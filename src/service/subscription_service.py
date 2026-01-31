import asyncio
import base64
import os
import sys
import time
from typing import Dict, List, Optional, Tuple
from urllib.parse import urlparse

import aiohttp
from loguru import logger

from core.config import Settings
from core.yaml_config import load_yaml_config
from service.git_uploader import GitUploader
from service.parse_uri import ProxyParser
from service.xray_service import XrayService
from service.geoip_service import GeoIPService
from service.storage_service import StorageService
from service.uri_generator import UriGenerator

class SubscriptionService:
    def __init__(self, settings: Settings, xray_service: XrayService):
        self.settings = settings
        self.xray_service = xray_service
        self.parser = ProxyParser()
        self.geoip_service = GeoIPService(settings.GEOIP_DB_PATH)
        self.storage_service = StorageService(settings)
        self.app_config = load_yaml_config()
        
        # State
        self._cached_all: Optional[List[Dict]] = None
        self._cached_top25: Optional[List[Dict]] = None
        self._cache_lock = asyncio.Lock()
        self._processing_lock = asyncio.Lock()
        
        self._site_cache: Dict[str, Tuple[float, List[Dict]]] = {}
        self._site_cache_lock = asyncio.Lock()

    def _generate_fingerprint(self, server: Dict) -> int:
        """Generates a unique hash for a server based on its connection details."""
        protocol = server.get("protocol")
        # Common fields for identity: address, port
        common = (server.get("address"), server.get("port"))
        
        if protocol == "vless":
            # Identity: protocol, address, port, uuid, flow, type, security, path
            return hash((
                "vless", *common, 
                server.get("vless_id"), 
                server.get("flow"),
                server.get("type"), 
                server.get("security"), 
                server.get("path")
            ))
        elif protocol == "vmess":
             # Identity: protocol, address, port, uuid, type, security, path, tls, aid
             return hash((
                 "vmess", *common, 
                 server.get("vmess_id"), 
                 server.get("type"), 
                 server.get("security"), 
                 server.get("path"), 
                 server.get("tls"),
                 server.get("aid")
             ))
        elif protocol == "trojan":
             # Identity: protocol, address, port, password
             return hash(("trojan", *common, server.get("password")))
        elif protocol == "shadowsocks":
             # Identity: protocol, address, port, method, password
             return hash(("shadowsocks", *common, server.get("method"), server.get("password")))
        elif protocol == "hysteria2":
             # Identity: protocol, address, port, password, obfs
             return hash(("hysteria2", *common, server.get("password"), server.get("obfs")))
        
        # Fallback for unknown protocols: use raw_uri (less optimal but safe)
        return hash(server.get("raw_uri"))

    async def _fetch_single_url(self, session: aiohttp.ClientSession, url: str) -> List[Dict]:
        url = url.strip()
        if not url: return []
        
        logger.info(f"Fetching from: {url}")
        try:
            async with session.get(url, timeout=30) as resp:
                resp.raise_for_status()
                raw_text = await resp.text()
                if raw_text.strip().startswith("<"):
                    logger.error(f"Error: content from {url} is HTML. Skipping.")
                    return []
                try:
                    decoded = base64.b64decode(raw_text).decode("utf-8", errors="ignore")
                except Exception:
                    decoded = raw_text
                
                lines = [line.strip() for line in decoded.splitlines() if line.strip()]
                parsed_batch = [p for p in (self.parser.parse(line) for line in lines) if p]
                logger.info(f"  Found {len(parsed_batch)} servers from {url}.")
                return parsed_batch
        except Exception as e:
            logger.error(f"Failed to fetch {url}: {e}")
            return []

    async def fetch_subscription_servers(self) -> List[Dict]:
        logger.info("Fetching subscriptions...")
        tasks = []
        async with aiohttp.ClientSession(trust_env=False) as session:
            for url in self.settings.SUB_URLS:
                tasks.append(self._fetch_single_url(session, url))
            
            results = await asyncio.gather(*tasks)
        
        # Flatten and Deduplicate
        seen_fingerprints = {}
        total_found = 0
        for batch in results:
            total_found += len(batch)
            for server in batch:
                fp = self._generate_fingerprint(server)
                if fp not in seen_fingerprints:
                    seen_fingerprints[fp] = server
        
        final_list = list(seen_fingerprints.values())
        logger.info(f"Total servers found: {total_found}. Unique servers: {len(final_list)}")

        if self.settings.LOW_INTERNET_CONS:
            logger.info(f"Low Internet Consumption Mode ON: Limiting to top {self.settings.LOW_INTERNET_LIMIT} servers.")
            final_list = final_list[:self.settings.LOW_INTERNET_LIMIT]

        return final_list

    async def compute_top_servers(self) -> List[Dict]:
        if not os.path.exists(self.settings.XRAY_PATH):
             # This might happen if Xray is not installed yet or path is wrong
             logger.warning(f"Warning: Xray executable not found at {self.settings.XRAY_PATH}")
             
        servers = await self.fetch_subscription_servers()
        if not servers:
            return []

        all_results = []
        for i in range(0, len(servers), self.settings.BATCH_SIZE):
            batch = servers[i : i + self.settings.BATCH_SIZE]
            logger.info(f"Testing batch {i // self.settings.BATCH_SIZE + 1}...")
            batch_results = await self.xray_service.run_test_batch(batch)
            all_results.extend(batch_results)

        successful = sorted([(s, d) for s, d in all_results if d <= self.settings.MAX_DELAY_MS], key=lambda item: item[1])
        logger.info(f"Found {len(successful)} working servers.")
        
        enriched_servers = []
        for server, delay in successful:
            s_copy = server.copy()
            s_copy["delay"] = round(delay)
            
            # GeoIP Lookup
            ip = s_copy.get("address")
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
            logger.info("Skipping update, a test is already in progress.")
            return
            
        async with self._processing_lock:
            try:
                top_servers = await self.compute_top_servers()
                async with self._cache_lock:
                    self._cached_all = top_servers
                    self._cached_top25 = top_servers[:25]
                logger.info(f"Cache updated with {len(top_servers)} servers.")

                # Persist to Redis
                await self.storage_service.save_servers("working_servers", top_servers)

                await self._handle_github_push(top_servers)
                await self._handle_precheck_sites(top_servers)

            except Exception as e:
                logger.error(f"Error during cache update: {e}")

    async def _handle_github_push(self, top_servers: List[Dict]):
        if self.settings.GITHUB_PUSH_ENABLED and self.settings.GITHUB_TOKEN and self.settings.GITHUB_REPO_URL and top_servers:
            logger.info("Starting GitHub push for main subscription...")
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
                logger.error(f"Main GitHub push failed: {e}")

    async def _handle_precheck_sites(self, top_servers: List[Dict]):
        # Priority: YAML config > ENV variable
        sites_to_check = []
        
        if self.app_config.sites:
             sites_to_check = [s for s in self.app_config.sites if s.enabled]
        elif self.settings.PRECHECK_SITES:
             # Backward compatibility: create dummy site configs from env
             from core.yaml_config import SiteConfig
             for url in self.settings.PRECHECK_SITES:
                 parsed = urlparse(url)
                 safe_name = parsed.hostname.replace(".", "_") if parsed.hostname else "site"
                 sites_to_check.append(SiteConfig(url=url, filename=f"{safe_name}.txt"))

        if sites_to_check and top_servers:
            logger.info(f"Pre-warming site cache for {len(sites_to_check)} sites...")
            for site in sites_to_check:
                logger.info(f"  Pre-checking {site.url}...")
                try:
                    valid_servers = await self.xray_service.evaluate_site_accessibility(site.url, top_servers)
                    
                    async with self._site_cache_lock:
                        self._site_cache[site.url] = (time.time(), valid_servers)
                    logger.info(f"  Cached {len(valid_servers)} servers for {site.url}")

                    if self.settings.GITHUB_PUSH_ENABLED and valid_servers:
                        # Use the specific filename from config
                        await self._push_site_specific_list(site.filename, valid_servers)

                except Exception as e:
                    logger.error(f"  Failed to pre-check {site.url}: {e}")

    async def _push_site_specific_list(self, filename: str, valid_servers: List[Dict]):
        try:
            site_content = "\n".join([s["raw_uri"] for s in valid_servers])
            
            # Use git branch from YAML if available, else env
            branch = self.app_config.git.branch if self.app_config.git.branch else self.settings.GITHUB_BRANCH

            uploader = GitUploader(
                repo_url=self.settings.GITHUB_REPO_URL,
                token=self.settings.GITHUB_TOKEN,
                user_name=self.settings.GITHUB_USER,
                user_email=self.settings.GITHUB_EMAIL,
                repo_dir=self.settings.GITHUB_REPO_DIR,
                branch=branch
            )
            logger.info(f"  Pushing {filename} to GitHub branch {branch}...")
            await asyncio.to_thread(uploader.update_file_and_push, filename, site_content)
        except Exception as push_err:
            logger.error(f"  Failed to push file {filename}: {push_err}")

    async def start_periodic_update(self):
        await self.geoip_service.initialize()
        await self.storage_service.initialize()
        
        # Try to load from cache first
        cached = await self.storage_service.load_servers("working_servers")
        if cached:
             async with self._cache_lock:
                 self._cached_all = cached
                 self._cached_top25 = cached[:25]
             logger.info(f"Loaded {len(cached)} servers from persistent storage.")

        while True:
            logger.info("Periodic cache update started...")
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
             return None

        async with self._processing_lock:
            successful_servers = await self.xray_service.evaluate_site_accessibility(url, servers_to_test)

        async with self._site_cache_lock:
            self._site_cache[url] = (time.time(), successful_servers)
            
        return successful_servers

    def is_processing(self) -> bool:
        return self._processing_lock.locked()