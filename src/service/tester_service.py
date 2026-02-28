import asyncio
from typing import List, Tuple
from loguru import logger

from core.config import Settings
from models.server import ProxyServer
from service.xray_service import XrayService
from service.geoip_service import GeoIPService

class TesterService:
    def __init__(self, settings: Settings, xray_service: XrayService, geoip_service: GeoIPService):
        self.settings = settings
        self.xray_service = xray_service
        self.geoip_service = geoip_service

    def enrich_server(self, server: ProxyServer, delay: float) -> ProxyServer:
        """Enriches a server with GeoIP info, remark, and URI."""
        server.delay = round(delay)
        
        # GeoIP Lookup
        ip = server.address
        country_code, flag = self.geoip_service.get_country(ip)
        server.country_code = country_code
        server.flag = flag
        
        # Update Remark: "🇺🇸 US 78ms"
        server.remark = f"{flag} {country_code} {server.delay}ms"
        
        # Regenerate URI (optional if model does it)
        server.raw_uri = server.to_uri()
        return server

    async def run_cycle(self, candidates: List[ProxyServer]) -> Tuple[List[ProxyServer], List[ProxyServer]]:
        """
        Runs a test cycle. 
        Returns (working_servers, updated_candidates)
        """
        if not candidates:
            return [], []

        logger.info(f"Testing {len(candidates)} candidates...")

        # 1. Batch Test
        all_results = []
        for i in range(0, len(candidates), self.settings.BATCH_SIZE):
            batch = candidates[i : i + self.settings.BATCH_SIZE]
            logger.info(f"Testing batch {i // self.settings.BATCH_SIZE + 1}...")
            batch_results = await self.xray_service.run_test_batch(batch)
            all_results.extend(batch_results)

        # 2. Process Results
        currently_working = []
        updated_candidates = []

        for server, delay in all_results:
            if delay <= self.settings.MAX_DELAY_MS:
                # Success: reset fail count and mark as working
                server.fail_count = 0
                enriched = self.enrich_server(server, delay)
                currently_working.append(enriched)
                updated_candidates.append(server)
            else:
                # Failure: increment fail count
                server.fail_count += 1
                if server.fail_count < self.settings.MAX_FAIL_COUNT:
                    updated_candidates.append(server)
                else:
                    # logger.debug(f"Server {server.address} reached max fail count and was removed.")
                    pass

        # Sort working servers by delay
        currently_working.sort(key=lambda s: s.delay)
        
        logger.info(f"Test cycle complete. Working: {len(currently_working)}, Candidates: {len(updated_candidates)}")
        return currently_working, updated_candidates
