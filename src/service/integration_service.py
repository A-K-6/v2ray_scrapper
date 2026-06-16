import asyncio
import os
import json
from typing import List, Optional
from loguru import logger

from core.config import Settings
from models.server import ProxyServer
from service.git_uploader import GitUploader
from service.xray_service import XrayService

class IntegrationService:
    def __init__(self, settings: Settings, xray_service: XrayService):
        self.settings = settings
        self.xray_service = xray_service
        self._git_lock = asyncio.Lock()

    def is_main_push_enabled(self) -> bool:
        return (
            self.settings.GITHUB_PUSH_ENABLED
            and self.settings.GITHUB_MAIN_PUSH_ENABLED
            and bool(self.settings.GITHUB_TOKEN)
            and bool(self.settings.GITHUB_REPO_URL)
        )

    def is_site_push_enabled(self) -> bool:
        return (
            self.settings.GITHUB_PUSH_ENABLED
            and self.settings.GITHUB_SITE_PUSH_ENABLED
            and bool(self.settings.GITHUB_TOKEN)
            and bool(self.settings.GITHUB_REPO_URL)
        )

    def has_enabled_integrations(self) -> bool:
        return self.is_main_push_enabled() or self.is_site_push_enabled()

    async def push_to_github(self, servers: List[ProxyServer], filename: Optional[str] = None):
        """Pushes the provided servers to GitHub."""
        if not servers:
            return

        is_main_file = not filename or filename == self.settings.GITHUB_FILENAME
        if is_main_file and not self.is_main_push_enabled():
            logger.info("Main GitHub push is disabled. Skipping publish.")
            return
        if not is_main_file and not self.is_site_push_enabled():
            logger.info(f"Site-specific GitHub push is disabled for {filename}. Skipping publish.")
            return

        async with self._git_lock:
            filename = filename or self.settings.GITHUB_FILENAME
            logger.info(f"Starting GitHub push for {filename}...")
        
        raw_links = [s.raw_uri for s in servers]
        content = "\n".join(raw_links)
        
        # Use top servers as potential proxies
        max_proxy_attempts = min(5, len(servers))
        
        for i in range(max_proxy_attempts):
            best_server = servers[i]
            proxy_port = 25000 + (1 if filename != self.settings.GITHUB_FILENAME else 0)
            
            try:
                async with self.xray_service.run_single_proxy(best_server, proxy_port):
                    request_payload = {
                        "command": "git-push",
                        "git": {
                            "repo_url": self.settings.GITHUB_REPO_URL,
                            "repo_dir": self.settings.GITHUB_REPO_DIR,
                            "branch": self.settings.GITHUB_BRANCH,
                            "token": self.settings.GITHUB_TOKEN,
                            "user_name": self.settings.GITHUB_USER,
                            "user_email": self.settings.GITHUB_EMAIL,
                            "file_updates": {filename: content},
                            "proxy_url": f"socks5h://127.0.0.1:{proxy_port}"
                        }
                    }
                    
                    go_binary_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "go-tester", "xray-tester")
                    process = await asyncio.create_subprocess_exec(
                        go_binary_path,
                        stdin=asyncio.subprocess.PIPE,
                        stdout=asyncio.subprocess.PIPE,
                        stderr=asyncio.subprocess.PIPE
                    )
                    stdout, stderr = await process.communicate(input=json.dumps(request_payload).encode('utf-8'))
                    
                    if process.returncode == 0:
                        logger.info(f"Push successful for {filename} via Go!")
                        return
                    else:
                        logger.error(f"Go git-push failed for {filename}: {stderr.decode()}")
            except Exception as e:
                logger.warning(f"Push attempt {i+1} failed for {filename}: {e}")
                if i == max_proxy_attempts - 1:
                    logger.error(f"All push attempts for {filename} failed.")

    async def push_batch_to_github(self, file_updates: dict[str, str], best_servers: List[ProxyServer]):
        """Pushes multiple files to GitHub in a single commit using a proxy."""
        if not file_updates or not best_servers:
            return

        async with self._git_lock:
            logger.info(f"Starting batch GitHub push for {len(file_updates)} files...")
            
            # Use top servers as potential proxies
            max_proxy_attempts = min(5, len(best_servers))
            
            for i in range(max_proxy_attempts):
                best_server = best_servers[i]
                proxy_port = 25005
                
                try:
                    async with self.xray_service.run_single_proxy(best_server, proxy_port):
                        request_payload = {
                            "command": "git-push",
                            "git": {
                                "repo_url": self.settings.GITHUB_REPO_URL,
                                "repo_dir": self.settings.GITHUB_REPO_DIR,
                                "branch": self.settings.GITHUB_BRANCH,
                                "token": self.settings.GITHUB_TOKEN,
                                "user_name": self.settings.GITHUB_USER,
                                "user_email": self.settings.GITHUB_EMAIL,
                                "file_updates": file_updates,
                                "proxy_url": f"socks5h://127.0.0.1:{proxy_port}"
                            }
                        }
                        
                        go_binary_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "go-tester", "xray-tester")
                        process = await asyncio.create_subprocess_exec(
                            go_binary_path,
                            stdin=asyncio.subprocess.PIPE,
                            stdout=asyncio.subprocess.PIPE,
                            stderr=asyncio.subprocess.PIPE
                        )
                        stdout, stderr = await process.communicate(input=json.dumps(request_payload).encode('utf-8'))
                        
                        if process.returncode == 0:
                            logger.info("Batch push successful via Go!")
                            return
                        else:
                            logger.error(f"Go git-push failed: {stderr.decode()}")
                except Exception as e:
                    logger.warning(f"Batch push attempt {i+1} failed: {e}")
                    if i == max_proxy_attempts - 1:
                        logger.error("All batch push attempts failed.")

    async def handle_site_checks(self, top_servers: List[ProxyServer]):
        """Handles site-specific tests and prepares batch push to GitHub."""
        if not self.is_site_push_enabled():
            logger.info("Site-specific checks are disabled because site publishing is off.")
            return

        sites_to_check = [s for s in self.settings.app_config.sites if s.enabled]
        if not sites_to_check or not top_servers:
            return

        logger.info(f"Checking {len(sites_to_check)} sites in parallel...")
        
        # 1. Parallel Testing
        async def test_site(site, idx):
            site_base_port = self.settings.BASE_PORT + 10000 + (idx * 1000)
            try:
                valid_servers = await self.xray_service.evaluate_site_accessibility(site.url, top_servers, base_port=site_base_port)
                return site, valid_servers
            except Exception as e:
                logger.error(f"  Failed to check {site.url}: {e}")
                return site, []

        tasks = [test_site(site, i) for i, site in enumerate(sites_to_check)]
        results = await asyncio.gather(*tasks)

        # 2. Collect updates for batch push
        batch_updates = {}
        # Also include the main subscription file in the same batch if needed
        # (Though update_cycle currently calls this after main push, we can unify them)
        
        for site, valid_servers in results:
            if valid_servers:
                raw_links = [s.raw_uri for s in valid_servers]
                batch_updates[site.filename] = "\n".join(raw_links)
        
        if batch_updates:
            logger.info(f"Triggering batch push for {len(batch_updates)} site results...")
            await self.push_batch_to_github(batch_updates, top_servers)

