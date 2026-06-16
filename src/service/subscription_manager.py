import base64
import asyncio
import binascii
import time
from typing import List, Dict, Optional, Tuple
from loguru import logger
from arq import create_pool
from arq.connections import RedisSettings

from core.config import Settings
from models.server import ProxyServer
from service.scraper_service import ScraperService
from service.tester_service import TesterService
from service.integration_service import IntegrationService
from service.storage_service import StorageService
from service.geoip_service import GeoIPService
from service.xray_service import XrayService

class SubscriptionManager:
    def __init__(self, settings: Settings, xray_service: XrayService):
        self.settings = settings
        self.xray_service = xray_service
        self.geoip_service = GeoIPService(settings.GEOIP_DB_PATH)
        self.storage_service = StorageService(settings)
        
        self.scraper = ScraperService(settings)
        self.tester = TesterService(settings, xray_service, self.geoip_service)
        self.integrator = IntegrationService(settings, xray_service)
        
        # State
        self._cached_all: List[ProxyServer] = []
        self._cached_top25: List[ProxyServer] = []
        self._candidate_servers: Dict[str, ProxyServer] = {}
        
        self._cache_lock = asyncio.Lock()
        self._processing_lock = asyncio.Lock()
        
        self._site_cache: Dict[str, Tuple[float, List[ProxyServer]]] = {}
        self._site_cache_lock = asyncio.Lock()
        
        self._arq_pool = None

    async def initialize(self):
        """Initializes services and loads state from storage."""
        await self.geoip_service.initialize()
        await self.storage_service.initialize()
        
        self._arq_pool = await create_pool(RedisSettings(
            host=self.settings.REDIS_HOST,
            port=self.settings.REDIS_PORT,
            database=self.settings.REDIS_DB,
            password=self.settings.REDIS_PASSWORD or None
        ))
        
        # Load working servers
        await self.refresh_cache_from_storage()

    async def refresh_cache_from_storage(self):
        """Reloads the in-memory cache from Redis."""
        working_data = await self.storage_service.load_servers("working_servers")
        if working_data:
            from pydantic import TypeAdapter
            adapter = TypeAdapter(ProxyServer)
            servers = []
            for d in working_data:
                try:
                    servers.append(adapter.validate_python(d))
                except Exception as e:
                    logger.warning(f"Failed to load server from storage: {e}")
            
            async with self._cache_lock:
                self._cached_all = servers
                self._cached_top25 = servers[:25]
            logger.info(f"Refreshed cache with {len(servers)} working servers from storage.")

        # Load candidates
        candidate_data = await self.storage_service.load_servers("candidate_servers")
        if candidate_data:
            from pydantic import TypeAdapter
            adapter = TypeAdapter(ProxyServer)
            async with self._cache_lock:
                self._candidate_servers = {}
                for d in candidate_data:
                    try:
                        s = adapter.validate_python(d)
                        self._candidate_servers[s.fingerprint] = s
                    except Exception as e:
                        logger.warning(f"Failed to load candidate from storage: {e}")
            logger.info(f"Refreshed {len(self._candidate_servers)} candidate servers from storage.")

    async def enqueue_update(self):
        """Enqueues an update cycle task."""
        if self._arq_pool:
            await self._arq_pool.enqueue_job('run_update_cycle_task')
            logger.info("Enqueued update cycle task.")
        else:
            logger.error("ARQ pool not initialized. Cannot enqueue task.")

    async def update_cycle(self):
        """Runs a full update cycle. This is meant to be called by the worker."""
        if self._processing_lock.locked():
            logger.info("Skipping update, a test is already in progress.")
            return

        async with self._processing_lock:
            try:
                # 1. Scrape and Test in Go
                logger.info("Starting unified Go scrape-and-test cycle...")
                results = await self.xray_service.scrape_and_test(self.settings.SUB_URLS)
                
                if not results:
                    logger.warning("No results from Go tester.")
                    return

                # 2. Process results back into models
                working_servers = []
                all_candidates = []
                
                for res in results:
                    uri = res.get("uri")
                    if not uri: continue
                    
                    # Parse URI in Python just to get the model (already validated by Go)
                    server = self.scraper.parser.parse(uri)
                    if not server: continue
                    
                    server.delay = res.get("delay", -1)
                    all_candidates.append(server)
                    
                    if not res.get("failed") and server.delay > 0:
                        # Enrich with GeoIP, Remark, etc.
                        self.tester.enrich_server(server, server.delay)
                        working_servers.append(server)

                if not all_candidates:
                    logger.warning("No valid candidate servers found after Go cycle.")
                    return
                
                # Sort by delay
                working_servers.sort(key=lambda s: s.delay if s.delay > 0 else float("inf"))
                
                # 3. Update local state
                async with self._cache_lock:
                    self._cached_all = working_servers
                    self._cached_top25 = working_servers[:25]
                    self._candidate_servers = {s.fingerprint: s for s in all_candidates}
                
                # 4. Persist to Storage (for API)
                await self.storage_service.save_servers("working_servers", [s.model_dump() for s in working_servers])
                await self.storage_service.save_servers("candidate_servers", [s.model_dump() for s in all_candidates])
                
                logger.info(f"Update cycle complete. Working: {len(working_servers)}, Total Candidates: {len(all_candidates)}")
                
                # 5. Integrations
                if self.integrator.has_enabled_integrations():
                    asyncio.create_task(self._run_integrations_background(working_servers))

            except Exception as e:
                logger.exception(f"Critical error in update cycle: {e}")

    async def _run_integrations_background(self, working: List[ProxyServer]):
        """Helper to run integrations without holding the main processing lock."""
        try:
            if self.integrator.is_main_push_enabled():
                await self.integrator.push_to_github(working)
            else:
                logger.info("Main subscription publishing is disabled. Skipping publish.")

            if self.integrator.is_site_push_enabled():
                await self.integrator.handle_site_checks(working)
            else:
                logger.info("Site-specific publishing is disabled. Skipping site checks.")

            logger.info("Background integrations completed.")
        except Exception as e:
            logger.error(f"Error in background integrations: {e}")

    async def start_periodic_update(self):
        """API Process: Periodically refreshes cache and enqueues updates."""
        await self.initialize()
        
        # Incremental Sync Task: Keep Redis in sync with Go's state.json
        asyncio.create_task(self._incremental_sync_loop())
        
        while True:
            # Refresh local cache from storage (in case worker updated it)
            await self.refresh_cache_from_storage()
            
            # Enqueue a new update if needed
            await self.enqueue_update()
            
            await asyncio.sleep(self.settings.CACHE_INTERVAL_SECONDS)

    async def _incremental_sync_loop(self):
        """Syncs state.json to Redis frequently to expose Go's partial progress to the API."""
        while True:
            try:
                import os
                import json
                if os.path.exists(self.settings.STATE_FILE_PATH):
                    with open(self.settings.STATE_FILE_PATH, "r") as f:
                        state_data = json.load(f)
                    
                    servers_data = state_data.get("servers", {})
                    if servers_data:
                        # Extract all URIs that are working
                        working_servers = []
                        all_candidates = []
                        
                        for fp, s in servers_data.items():
                            uri = s.get("uri")
                            if not uri: continue
                            
                            # Parse URI to model
                            server = self.scraper.parser.parse(uri)
                            if not server: continue
                            
                            server.delay = s.get("last_delay", -1)
                            server.fail_count = s.get("fail_count", 0)
                            all_candidates.append(server.model_dump())
                            
                            if server.delay > 0 and server.fail_count == 0:
                                self.tester.enrich_server(server, server.delay)
                                working_servers.append(server.model_dump())
                        
                        # Sort working by delay
                        working_servers.sort(key=lambda s: s.get("delay", 99999))
                        
                        # Sync to Redis
                        await self.storage_service.save_servers("working_servers", working_servers)
                        await self.storage_service.save_servers("candidate_servers", all_candidates)
                        # logger.debug(f"Incrementally synced {len(working_servers)} servers to Redis.")
            except Exception as e:
                logger.error(f"Incremental sync error: {e}")
            
            await asyncio.sleep(10) # Sync every 10 seconds

    async def get_top_25(self) -> List[ProxyServer]:
        await self.refresh_cache_from_storage()
        async with self._cache_lock:
            return list(self._cached_top25)

    async def get_all_cached(self) -> List[ProxyServer]:
        await self.refresh_cache_from_storage()
        async with self._cache_lock:
            return list(self._cached_all)

    async def get_site_specific_servers(self, url: str) -> Optional[List[ProxyServer]]:
        # Refresh from storage first to get any incrementally found servers
        await self.refresh_cache_from_storage()

        async with self._site_cache_lock:
            if url in self._site_cache:
                cache_time, servers = self._site_cache[url]
                if (time.time() - cache_time) < self.settings.SITE_CACHE_TTL_SECONDS:
                    return servers

        async with self._cache_lock:
            # Use ALL discovered servers (including those found in current background run)
            base_list = self._cached_all

        if not base_list: return None
        
        # We allow site-specific checks even if a background update is running.
        # This gives the user immediate results from partially discovered servers.
        valid = await self.xray_service.evaluate_site_accessibility(url, base_list)
        async with self._site_cache_lock:
            self._site_cache[url] = (time.time(), valid)
        return valid

    def parse_subscription_content(self, content: str) -> List[ProxyServer]:
        raw_content = content.strip()
        if not raw_content:
            return []

        parsed_servers = self._parse_lines(raw_content)
        if parsed_servers:
            return parsed_servers

        try:
            decoded = base64.b64decode(raw_content).decode("utf-8", errors="ignore")
        except (binascii.Error, ValueError):
            return []

        return self._parse_lines(decoded)

    def _parse_lines(self, content: str) -> List[ProxyServer]:
        seen_fingerprints: Dict[str, ProxyServer] = {}
        for line in content.splitlines():
            parsed = self.scraper.parser.parse(line)
            if not parsed:
                continue
            seen_fingerprints[parsed.fingerprint] = parsed
        return list(seen_fingerprints.values())

    async def test_subscription_content(self, content: str) -> Optional[List[ProxyServer]]:
        candidates = self.parse_subscription_content(content)
        if not candidates:
            return []

        if self._processing_lock.locked():
            return None

        async with self._processing_lock:
            working, _ = await self.tester.run_cycle(candidates)
            return working

    def is_processing(self) -> bool:
        return self._processing_lock.locked()

    async def get_top_25(self) -> List[ProxyServer]:
        await self.refresh_cache_from_storage()
        async with self._cache_lock:
            return list(self._cached_top25)

    async def get_all_cached(self) -> List[ProxyServer]:
        await self.refresh_cache_from_storage()
        async with self._cache_lock:
            return list(self._cached_all)

    async def get_site_specific_servers(self, url: str) -> Optional[List[ProxyServer]]:
        async with self._site_cache_lock:
            if url in self._site_cache:
                cache_time, servers = self._site_cache[url]
                if (time.time() - cache_time) < self.settings.SITE_CACHE_TTL_SECONDS:
                    return servers

        async with self._cache_lock:
            base_list = self._cached_all

        if not base_list: return None
        if self._processing_lock.locked(): return None

        async with self._processing_lock:
            valid = await self.xray_service.evaluate_site_accessibility(url, base_list)
            async with self._site_cache_lock:
                self._site_cache[url] = (time.time(), valid)
            return valid

    def is_processing(self) -> bool:
        return self._processing_lock.locked()
