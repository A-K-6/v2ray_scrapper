import asyncio
import base64
import aiohttp
from typing import List, Optional
from loguru import logger
from tenacity import retry, stop_after_attempt, wait_exponential, retry_if_exception_type

from core.config import Settings
from service.parse_uri import ProxyParser
from models.server import ProxyServer

class ScraperService:
    def __init__(self, settings: Settings):
        self.settings = settings
        self.parser = ProxyParser()

    @retry(
        stop=stop_after_attempt(5),
        wait=wait_exponential(multiplier=2, min=2, max=30),
        retry=retry_if_exception_type((aiohttp.ClientError, asyncio.TimeoutError)),
        before_sleep=lambda retry_state: logger.warning(f"Retrying fetch ({retry_state.attempt_number}/5) after error: {retry_state.outcome.exception()}"),
        reraise=True
    )
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
        except (aiohttp.ClientError, asyncio.TimeoutError) as e:
            # Re-raise for tenacity to handle retries
            raise
        except Exception as e:
            logger.error(f"Failed to fetch {url}: {e}")
            return []

    async def fetch_all(self) -> List[ProxyServer]:
        logger.info("Fetching subscriptions...")
        tasks = []
        async with aiohttp.ClientSession(trust_env=False) as session:
            for url in self.settings.SUB_URLS:
                tasks.append(self._fetch_single_url(session, url))
            
            results = await asyncio.gather(*tasks, return_exceptions=True)
        
        # Flatten and Deduplicate
        seen_fingerprints = {}
        total_found = 0
        failed_sources = 0
        for url, batch in zip(self.settings.SUB_URLS, results):
            if isinstance(batch, Exception):
                failed_sources += 1
                logger.error(f"Skipping failed subscription source {url}: {batch}")
                continue
            total_found += len(batch)
            for server in batch:
                fp = server.fingerprint
                if fp not in seen_fingerprints:
                    seen_fingerprints[fp] = server
        
        final_list = list(seen_fingerprints.values())
        logger.info(
            f"Total servers found: {total_found}. Unique servers: {len(final_list)}. "
            f"Failed sources: {failed_sources}/{len(self.settings.SUB_URLS)}"
        )

        if self.settings.LOW_INTERNET_CONS:
            logger.info(f"Low Internet Consumption Mode ON: Limiting to top {self.settings.LOW_INTERNET_LIMIT} servers.")
            final_list = final_list[:self.settings.LOW_INTERNET_LIMIT]

        return final_list

    async def fetch_urls(self, urls: List[str]) -> List[ProxyServer]:
        logger.info(f"Fetching {len(urls)} custom subscriptions...")
        tasks = []
        async with aiohttp.ClientSession(trust_env=False) as session:
            for url in urls:
                tasks.append(self._fetch_single_url(session, url))
            
            results = await asyncio.gather(*tasks, return_exceptions=True)
        
        seen_fingerprints = {}
        total_found = 0
        failed_sources = 0
        for url, batch in zip(urls, results):
            if isinstance(batch, Exception):
                failed_sources += 1
                logger.error(f"Skipping failed subscription source {url}: {batch}")
                continue
            total_found += len(batch)
            for server in batch:
                fp = server.fingerprint
                if fp not in seen_fingerprints:
                    seen_fingerprints[fp] = server
        
        final_list = list(seen_fingerprints.values())
        logger.info(
            f"Custom fetch complete. Found {total_found} servers. Unique servers: {len(final_list)}. "
            f"Failed: {failed_sources}/{len(urls)}"
        )
        return final_list

