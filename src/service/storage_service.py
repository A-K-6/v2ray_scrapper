import json
import sys
import time
from typing import Dict, List, Optional
import redis.asyncio as redis
from loguru import logger

from core.config import Settings

class StorageService:
    def __init__(self, settings: Settings):
        self.settings = settings
        self.redis: Optional[redis.Redis] = None

    async def initialize(self):
        """Initializes the Redis connection."""
        try:
            self.redis = redis.Redis(
                host=self.settings.REDIS_HOST,
                port=self.settings.REDIS_PORT,
                db=self.settings.REDIS_DB,
                password=self.settings.REDIS_PASSWORD or None,
                decode_responses=True
            )
            await self.redis.ping()
            logger.info(f"Connected to Redis at {self.settings.REDIS_HOST}:{self.settings.REDIS_PORT}")
        except Exception as e:
            logger.error(f"Failed to connect to Redis: {e}")
            self.redis = None

    async def save_servers(self, key: str, servers: List[Dict], ttl: int = 0):
        """Saves a list of servers to Redis."""
        if not self.redis:
            return

        try:
            json_data = json.dumps(servers)
            if ttl > 0:
                await self.redis.setex(key, ttl, json_data)
            else:
                await self.redis.set(key, json_data)
        except Exception as e:
            logger.error(f"Error saving to Redis (key={key}): {e}")

    async def load_servers(self, key: str) -> List[Dict]:
        """Loads a list of servers from Redis."""
        if not self.redis:
            return []

        try:
            data = await self.redis.get(key)
            if data:
                return json.loads(data)
        except Exception as e:
            logger.error(f"Error loading from Redis (key={key}): {e}")
        return []

    async def record_site_request(self, url: str):
        """Records that a site-specific URL was requested, with current timestamp."""
        if not self.redis:
            return
        try:
            await self.redis.hset("site_requests", url, str(time.time()))
        except Exception as e:
            logger.error(f"Error recording site request for {url}: {e}")

    async def get_active_site_requests(self, max_age_seconds: float) -> List[str]:
        """Gets all site-specific URLs requested within max_age_seconds, and cleans up expired ones."""
        if not self.redis:
            return []
        try:
            all_requests = await self.redis.hgetall("site_requests")
            now = time.time()
            active_urls = []
            expired_urls = []
            for url, ts_str in all_requests.items():
                try:
                    ts = float(ts_str)
                    if now - ts <= max_age_seconds:
                        active_urls.append(url)
                    else:
                        expired_urls.append(url)
                except ValueError:
                    expired_urls.append(url)
            
            # Clean up expired fields from Redis
            if expired_urls:
                try:
                    await self.redis.hdel("site_requests", *expired_urls)
                    logger.info(f"Cleaned up {len(expired_urls)} expired site-specific requests from Redis.")
                except Exception as ex:
                    logger.error(f"Failed to delete expired site requests from Redis: {ex}")
                    
            return active_urls
        except Exception as e:
            logger.error(f"Error getting active site requests: {e}")
            return []

    async def close(self):
        if self.redis:
            await self.redis.close()
