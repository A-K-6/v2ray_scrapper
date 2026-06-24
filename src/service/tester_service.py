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

    async def run_cycle(self, candidates: List[ProxyServer], test_url: Optional[str] = None, max_delay_ms: Optional[int] = None) -> Tuple[List[ProxyServer], List[ProxyServer]]:
        """
        Runs a test cycle. 
        Returns (working_servers, updated_candidates)
        """
        if not candidates:
            return [], []

        logger.info(f"Testing {len(candidates)} candidates...")

        # 1. Batch Test (Parallelized with Slot-based Port Recycling)
        all_results = []
        max_concurrent = self.settings.MAX_CONCURRENT_BATCHES
        
        # Use a Queue to manage available port "slots" to prevent port overflow
        available_slots = asyncio.Queue()
        for i in range(max_concurrent):
            available_slots.put_nowait(i)
        
        total_batches = (len(candidates) + self.settings.BATCH_SIZE - 1) // self.settings.BATCH_SIZE
        logger.info(f"Starting test cycle with {total_batches} batches (concurrency: {max_concurrent})")

        async def test_batch(batch, batch_idx):
            slot_idx = await available_slots.get()
            try:
                # Use a safe offset (200 ports per batch) and recycle ports based on slot
                batch_base_port = self.settings.BASE_PORT + (slot_idx * 200)
                
                # Log progress every 10 batches or every 10%
                log_interval = max(1, total_batches // 10)
                if (batch_idx + 1) % log_interval == 0 or (batch_idx + 1) == total_batches:
                    logger.info(f"Processing batch {batch_idx + 1}/{total_batches}...")
                
                return await self.xray_service.run_test_batch(batch, base_port=batch_base_port, test_url=test_url)
            finally:
                # Small cooldown to let OS release ports (avoid TIME_WAIT issues)
                await asyncio.sleep(2)
                available_slots.put_nowait(slot_idx)

        tasks = []
        for i in range(0, len(candidates), self.settings.BATCH_SIZE):
            batch = candidates[i : i + self.settings.BATCH_SIZE]
            batch_idx = i // self.settings.BATCH_SIZE
            tasks.append(test_batch(batch, batch_idx))

        batches_results = await asyncio.gather(*tasks)
        for res in batches_results:
            all_results.extend(res)

        # 2. Process Results
        currently_working = []
        updated_candidates = []
        limit_delay = max_delay_ms if max_delay_ms is not None else self.settings.MAX_DELAY_MS

        for server, delay in all_results:
            if delay <= limit_delay:
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
