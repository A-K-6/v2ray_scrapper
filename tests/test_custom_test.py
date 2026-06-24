import sys
import os
import unittest
from unittest.mock import AsyncMock, MagicMock, patch

sys.path.append(os.path.abspath(os.path.join(os.path.dirname(__file__), '../src')))

from service.subscription_manager import SubscriptionManager
from core.config import Settings
from models.server import ProxyServer

class TestCustomSubscriptionTesting(unittest.IsolatedAsyncioTestCase):
    async def test_custom_subscription_logic(self):
        settings = Settings()
        xray_service = MagicMock()
        
        manager = SubscriptionManager(settings, xray_service)
        manager.scraper = AsyncMock()
        manager.tester = AsyncMock()
        
        # Mock fetch_urls
        dummy_server_1 = MagicMock(spec=ProxyServer)
        dummy_server_1.fingerprint = "fp1"
        dummy_server_1.delay = 100
        
        dummy_server_2 = MagicMock(spec=ProxyServer)
        dummy_server_2.fingerprint = "fp2"
        dummy_server_2.delay = 200
        
        manager.scraper.fetch_urls.return_value = [dummy_server_1, dummy_server_2]
        
        # Mock tester.run_cycle
        manager.tester.run_cycle.return_value = ([dummy_server_1, dummy_server_2], [])
        
        # Test custom subscription testing
        working = await manager.test_custom_subscription(
            subscription_urls=["https://custom-sub-url.com/sub.txt"],
            test_url="https://youtube.com",
            max_delay_ms=2000,
            limit=1
        )
        
        manager.scraper.fetch_urls.assert_called_with(["https://custom-sub-url.com/sub.txt"])
        manager.tester.run_cycle.assert_called_with(
            [dummy_server_1, dummy_server_2],
            test_url="https://youtube.com",
            max_delay_ms=2000
        )
        
        # Limit is 1, so only 1 server should be returned
        self.assertEqual(len(working), 1)
        self.assertEqual(working[0], dummy_server_1)

if __name__ == '__main__':
    unittest.main()
