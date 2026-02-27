import asyncio
import base64
import aiohttp
from typing import List, Optional
from loguru import logger

from core.config import Settings
from service.parse_uri import ProxyParser
from models.server import ProxyServer

class ScraperService:
    def __init__(self, settings: Settings):
        self.settings = settings
        self.parser = ProxyParser()

    async def _fetch_single_url(self, session: aiohttp.ClientSession, url: str) -> List[ProxyServer]:
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

    async def fetch_all(self) -> List[ProxyServer]:
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
                fp = server.fingerprint
                if fp not in seen_fingerprints:
                    seen_fingerprints[fp] = server
        
        final_list = list(seen_fingerprints.values())
        logger.info(f"Total servers found: {total_found}. Unique servers: {len(final_list)}")

        if self.settings.LOW_INTERNET_CONS:
            logger.info(f"Low Internet Consumption Mode ON: Limiting to top {self.settings.LOW_INTERNET_LIMIT} servers.")
            final_list = final_list[:self.settings.LOW_INTERNET_LIMIT]

        return final_list
