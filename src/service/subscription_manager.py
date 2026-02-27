import asyncio
import time
from typing import List, Dict, Optional, Tuple
from loguru import logger

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

    async def initialize(self):
        """Initializes services and loads state from storage."""
        await self.geoip_service.initialize()
        await self.storage_service.initialize()
        
        # Load working servers
        working_data = await self.storage_service.load_servers("working_servers")
        if working_data:
            from models.server import VlessServer, VmessServer, TrojanServer, ShadowsocksServer, Hysteria2Server
            servers = []
            for d in working_data:
                try:
                    # Generic validation would be better but let's be explicit if needed
                    # Actually ProxyServer is a Union, Pydantic should handle it
                    from pydantic import TypeAdapter
                    adapter = TypeAdapter(ProxyServer)
                    servers.append(adapter.validate_python(d))
                except Exception as e:
                    logger.warning(f"Failed to load server from storage: {e}")
            
            async with self._cache_lock:
                self._cached_all = servers
                self._cached_top25 = servers[:25]
            logger.info(f"Loaded {len(servers)} working servers from storage.")

        # Load candidates
        candidate_data = await self.storage_service.load_servers("candidate_servers")
        if candidate_data:
            from pydantic import TypeAdapter
            adapter = TypeAdapter(ProxyServer)
            for d in candidate_data:
                try:
                    s = adapter.validate_python(d)
                    self._candidate_servers[s.fingerprint] = s
                except Exception as e:
                    logger.warning(f"Failed to load candidate from storage: {e}")
            logger.info(f"Loaded {len(self._candidate_servers)} candidate servers from storage.")

    async def update_cycle(self):
        """Runs a full update cycle: scrap -> merge -> test -> enrich -> store -> push."""
        if self._processing_lock.locked():
            logger.info("Skipping update, a test is already in progress.")
            return

        async with self._processing_lock:
            try:
                # 1. Scrape
                new_servers = await self.scraper.fetch_all()
                
                # 2. Merge with candidates
                for s in new_servers:
                    fp = s.fingerprint
                    if fp not in self._candidate_servers:
                        self._candidate_servers[fp] = s
                
                # 3. Test
                working, updated_candidates_list = await self.tester.run_cycle(list(self._candidate_servers.values()))
                
                # 4. Update State
                async with self._cache_lock:
                    self._cached_all = working
                    self._cached_top25 = working[:25]
                
                self._candidate_servers = {s.fingerprint: s for s in updated_candidates_list}
                
                # 5. Persist
                await self.storage_service.save_servers("working_servers", [s.model_dump() for s in working])
                await self.storage_service.save_servers("candidate_servers", [s.model_dump() for s in updated_candidates_list])
                
                # 6. Integrations
                await self.integrator.push_to_github(working)
                await self.integrator.handle_site_checks(working)

            except Exception as e:
                logger.exception(f"Critical error in update cycle: {e}")

    async def start_periodic_update(self):
        await self.initialize()
        while True:
            await self.update_cycle()
            await asyncio.sleep(self.settings.CACHE_INTERVAL_SECONDS)

    async def get_top_25(self) -> List[ProxyServer]:
        async with self._cache_lock:
            return self._cached_top25

    async def get_all_cached(self) -> List[ProxyServer]:
        async with self._cache_lock:
            return self._cached_all

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
