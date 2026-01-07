import json
import sys
from typing import Dict, List, Optional
import redis.asyncio as redis

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
            print(f"Connected to Redis at {self.settings.REDIS_HOST}:{self.settings.REDIS_PORT}")
        except Exception as e:
            print(f"Failed to connect to Redis: {e}", file=sys.stderr)
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
            print(f"Error saving to Redis (key={key}): {e}", file=sys.stderr)

    async def load_servers(self, key: str) -> List[Dict]:
        """Loads a list of servers from Redis."""
        if not self.redis:
            return []

        try:
            data = await self.redis.get(key)
            if data:
                return json.loads(data)
        except Exception as e:
            print(f"Error loading from Redis (key={key}): {e}", file=sys.stderr)
        return []

    async def close(self):
        if self.redis:
            await self.redis.close()
