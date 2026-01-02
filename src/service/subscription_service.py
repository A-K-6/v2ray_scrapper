import asyncio
import base64
import os
import sys
import time
from typing import Dict, List, Optional, Tuple
from urllib.parse import urlparse

import aiohttp

from core.config import Settings
from service.git_uploader import GitUploader
from service.parse_uri import ProxyParser
from service.xray_service import XrayService
from service.geoip_service import GeoIPService
from service.uri_generator import UriGenerator

class SubscriptionService:
    def __init__(self, settings: Settings, xray_service: XrayService):
        self.settings = settings
        self.xray_service = xray_service
        self.parser = ProxyParser()
        self.geoip_service = GeoIPService(settings.GEOIP_DB_PATH)
        
        # State
        self._cached_all: Optional[List[Dict]] = None
        self._cached_top25: Optional[List[Dict]] = None
        self._cache_lock = asyncio.Lock()
        self._processing_lock = asyncio.Lock()
        
        self._site_cache: Dict[str, Tuple[float, List[Dict]]] = {}
        self._site_cache_lock = asyncio.Lock()

    async def fetch_subscription_servers(self) -> List[Dict]:
        print("Fetching subscriptions...")
        all_servers = []
        
        for url in self.settings.SUB_URLS:
            url = url.strip()
            if not url: continue
            
            print(f"Fetching from: {url}")
            try:
                async with aiohttp.ClientSession(trust_env=False) as session:
                    async with session.get(url, timeout=30) as resp:
                        resp.raise_for_status()
                        raw_text = await resp.text()
                        if raw_text.strip().startswith("<"):
                            print(f"Error: content from {url} is HTML. Skipping.", file=sys.stderr)
                            continue
                        try:
                            decoded = base64.b64decode(raw_text).decode("utf-8", errors="ignore")
                        except Exception:
                            decoded = raw_text
                        
                        lines = [line.strip() for line in decoded.splitlines() if line.strip()]
                        parsed_batch = [p for p in (self.parser.parse(line) for line in lines) if p]
                        all_servers.extend(parsed_batch)
                        print(f"  Found {len(parsed_batch)} servers from {url}.")
            except Exception as e:
                print(f"Failed to fetch {url}: {e}", file=sys.stderr)

        unique_servers = {s['raw_uri']: s for s in all_servers}.values()
        final_list = list(unique_servers)
        print(f"Total unique servers found: {len(final_list)}")

        if self.settings.LOW_INTERNET_CONS:
            print(f"Low Internet Consumption Mode ON: Limiting to top {self.settings.LOW_INTERNET_LIMIT} servers.")
            final_list = final_list[:self.settings.LOW_INTERNET_LIMIT]

        return final_list

    async def compute_top_servers(self) -> List[Dict]:
        if not os.path.exists(self.settings.XRAY_PATH):
             # This might happen if Xray is not installed yet or path is wrong
             print(f"Warning: Xray executable not found at {self.settings.XRAY_PATH}", file=sys.stderr)
             
        servers = await self.fetch_subscription_servers()
        if not servers:
            return []

        all_results = []
        for i in range(0, len(servers), self.settings.BATCH_SIZE):
            batch = servers[i : i + self.settings.BATCH_SIZE]
            print(f"Testing batch {i // self.settings.BATCH_SIZE + 1}...")
            batch_results = await self.xray_service.run_test_batch(batch)
            all_results.extend(batch_results)

        successful = sorted([(s, d) for s, d in all_results if d <= self.settings.MAX_DELAY_MS], key=lambda item: item[1])
        print(f"Found {len(successful)} working servers.")
        
        enriched_servers = []
        for server, delay in successful:
            s_copy = server.copy()
            s_copy["delay"] = round(delay)
            
            # GeoIP Lookup
            ip = s_copy.get("address")
            # If address is a domain, we might not get a country unless we resolve it.
            # GeoIP reader expects IP. Xray resolves it during test but we don't capture the resolved IP.
            # Optimization: If it's a domain, we could resolve it, but that adds latency.
            # Ideally XrayService could return the resolved IP if it tracks it.
            # For now, let's just try looking it up (geoip2 handles IP strings).
            # If it's a domain, get_country returns default.
            
            country_code, flag = self.geoip_service.get_country(ip)
            s_copy["country_code"] = country_code
            s_copy["flag"] = flag
            
            # Update Remark: "🇺🇸 US 78ms"
            new_remark = f"{flag} {country_code} {s_copy['delay']}ms"
            s_copy["remark"] = new_remark
            
            # Regenerate URI
            s_copy["raw_uri"] = UriGenerator.generate(s_copy)
            
            enriched_servers.append(s_copy)

        return enriched_servers

    async def update_cache(self):
        """Updates the cache with the top servers."""
        if self._processing_lock.locked():
            print("Skipping update, a test is already in progress.")
            return
            
        async with self._processing_lock:
            try:
                top_servers = await self.compute_top_servers()
                async with self._cache_lock:
                    self._cached_all = top_servers
                    self._cached_top25 = top_servers[:25]
                print(f"Cache updated with {len(top_servers)} servers.")

                await self._handle_github_push(top_servers)
                await self._handle_precheck_sites(top_servers)

            except Exception as e:
                print(f"Error during cache update: {e}", file=sys.stderr)

    async def _handle_github_push(self, top_servers: List[Dict]):
        if self.settings.GITHUB_PUSH_ENABLED and self.settings.GITHUB_TOKEN and self.settings.GITHUB_REPO_URL and top_servers:
            print("Starting GitHub push for main subscription...")
            try:
                raw_links = [s["raw_uri"] for s in top_servers]
                content = "\n".join(raw_links)
                
                uploader = GitUploader(
                    repo_url=self.settings.GITHUB_REPO_URL,
                    token=self.settings.GITHUB_TOKEN,
                    user_name=self.settings.GITHUB_USER,
                    user_email=self.settings.GITHUB_EMAIL,
                    repo_dir=self.settings.GITHUB_REPO_DIR,
                    branch=self.settings.GITHUB_BRANCH
                )
                await asyncio.to_thread(uploader.update_file_and_push, self.settings.GITHUB_FILENAME, content)
            except Exception as e:
                print(f"Main GitHub push failed: {e}", file=sys.stderr)

    async def _handle_precheck_sites(self, top_servers: List[Dict]):
        if self.settings.PRECHECK_SITES and top_servers:
            print(f"Pre-warming site cache for: {self.settings.PRECHECK_SITES}")
            for site_url in self.settings.PRECHECK_SITES:
                print(f"  Pre-checking {site_url}...")
                try:
                    valid_servers = await self.xray_service.evaluate_site_accessibility(site_url, top_servers)
                    
                    async with self._site_cache_lock:
                        self._site_cache[site_url] = (time.time(), valid_servers)
                    print(f"  Cached {len(valid_servers)} servers for {site_url}")

                    if self.settings.GITHUB_PUSH_ENABLED and valid_servers:
                        await self._push_site_specific_list(site_url, valid_servers)

                except Exception as e:
                    print(f"  Failed to pre-check {site_url}: {e}", file=sys.stderr)

    async def _push_site_specific_list(self, site_url: str, valid_servers: List[Dict]):
        try:
            parsed = urlparse(site_url)
            safe_hostname = parsed.hostname.replace(".", "_") if parsed.hostname else "unknown_site"
            site_filename = f"{safe_hostname}.txt"

            site_content = "\n".join([s["raw_uri"] for s in valid_servers])
            
            uploader = GitUploader(
                repo_url=self.settings.GITHUB_REPO_URL,
                token=self.settings.GITHUB_TOKEN,
                user_name=self.settings.GITHUB_USER,
                user_email=self.settings.GITHUB_EMAIL,
                repo_dir=self.settings.GITHUB_REPO_DIR,
                branch=self.settings.GITHUB_BRANCH
            )
            print(f"  Pushing {site_filename} to GitHub...")
            await asyncio.to_thread(uploader.update_file_and_push, site_filename, site_content)
        except Exception as push_err:
            print(f"  Failed to push file for {site_url}: {push_err}", file=sys.stderr)

    async def start_periodic_update(self):
        await self.geoip_service.initialize()
        while True:
            print("Periodic cache update started...")
            await self.update_cache()
            await asyncio.sleep(self.settings.CACHE_INTERVAL_SECONDS)

    # Accessors
    async def get_top_25(self) -> List[Dict]:
        async with self._cache_lock:
            return self._cached_top25

    async def get_all_cached(self) -> List[Dict]:
        async with self._cache_lock:
            return self._cached_all
    
    async def get_site_specific_servers(self, url: str) -> List[Dict]:
        # Check cache
        async with self._site_cache_lock:
            if url in self._site_cache:
                cache_time, cached_servers = self._site_cache[url]
                if (time.time() - cache_time) < self.settings.SITE_CACHE_TTL_SECONDS:
                    return cached_servers

        # If not cached or expired, we need to test
        # We need the base list of servers to test against
        async with self._cache_lock:
             servers_to_test = self._cached_all

        if not servers_to_test:
            return None

        if self._processing_lock.locked():
             # Avoid starting a heavy test if system is busy? 
             # Or maybe we just wait for lock if we want to prevent concurrent heavy loads?
             # The original code raised 429 if locked.
             # But here evaluate_site_accessibility uses run_test_batch logic which spawns xray.
             # We probably want to allow it but maybe limit concurrency.
             # For now, let's respect the lock to avoid overloading.
             return None

        async with self._processing_lock:
            successful_servers = await self.xray_service.evaluate_site_accessibility(url, servers_to_test)

        async with self._site_cache_lock:
            self._site_cache[url] = (time.time(), successful_servers)
            
        return successful_servers

    def is_processing(self) -> bool:
        return self._processing_lock.locked()
