import sys
import os
import unittest
from unittest.mock import AsyncMock, MagicMock, patch
import time

sys.path.append(os.path.abspath(os.path.join(os.path.dirname(__file__), '../src')))

from service.storage_service import StorageService
from service.subscription_manager import SubscriptionManager
from core.config import Settings
from models.server import ProxyServer

class TestStorageServiceCaching(unittest.IsolatedAsyncioTestCase):
    async def test_record_and_get_active_site_requests(self):
        # Create a mock settings object
        settings = Settings()
        
        # Instantiate StorageService and mock redis
        storage = StorageService(settings)
        storage.redis = AsyncMock()
        
        # Test record_site_request
        await storage.record_site_request("https://google.com")
        storage.redis.hset.assert_called_once()
        args = storage.redis.hset.call_args[0]
        self.assertEqual(args[0], "site_requests")
        self.assertEqual(args[1], "https://google.com")
        
        # Test get_active_site_requests
        now = time.time()
        storage.redis.hgetall.return_value = {
            "https://google.com": str(now - 1000),      # active
            "https://facebook.com": str(now - 600000),  # expired (600k sec is ~7 days)
        }
        
        active = await storage.get_active_site_requests(5 * 86400) # 5 days
        self.assertIn("https://google.com", active)
        self.assertNotIn("https://facebook.com", active)

class TestSubscriptionManagerCaching(unittest.IsolatedAsyncioTestCase):
    async def test_get_site_specific_servers_uses_cache(self):
        settings = Settings()
        settings.SITE_CACHE_TTL_SECONDS = 3600
        xray_service = MagicMock()
        
        manager = SubscriptionManager(settings, xray_service)
        manager.storage_service = AsyncMock()
        
        # Mock load_servers to simulate Redis cache hit
        test_server = {
            "protocol": "vless",
            "address": "127.0.0.1",
            "port": 443,
            "vless_id": "uuid",
            "remark": "CachedServer"
        }
        manager.storage_service.load_servers.return_value = [test_server]
        
        # Call get_site_specific_servers
        servers = await manager.get_site_specific_servers("https://google.com")
        
        # Assertions
        manager.storage_service.record_site_request.assert_called_with("https://google.com")
        manager.storage_service.load_servers.assert_called_with("site_servers:https://google.com")
        self.assertEqual(len(servers), 1)
        self.assertEqual(servers[0].remark, "CachedServer")
        
        # Check that it's cached in memory too
        self.assertIn("https://google.com", manager._site_cache)

if __name__ == '__main__':
    unittest.main()
