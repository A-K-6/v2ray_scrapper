import asyncio
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

    async def push_to_github(self, servers: List[ProxyServer], filename: Optional[str] = None):
        """Pushes the provided servers to GitHub."""
        if not self.settings.GITHUB_PUSH_ENABLED or not self.settings.GITHUB_TOKEN or not servers:
            return

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
                    uploader = GitUploader(
                        repo_url=self.settings.GITHUB_REPO_URL,
                        token=self.settings.GITHUB_TOKEN,
                        user_name=self.settings.GITHUB_USER,
                        user_email=self.settings.GITHUB_EMAIL,
                        repo_dir=self.settings.GITHUB_REPO_DIR,
                        branch=self.settings.GITHUB_BRANCH,
                        settings=self.settings,
                        proxy_url=f"socks5://127.0.0.1:{proxy_port}"
                    )
                    await asyncio.to_thread(uploader.update_file_and_push, filename, content)
                    return # Success!
            except Exception as e:
                logger.warning(f"Push attempt {i+1} failed for {filename}: {e}")
                if i == max_proxy_attempts - 1:
                    logger.error(f"All push attempts for {filename} failed.")

    async def handle_site_checks(self, top_servers: List[ProxyServer]):
        """Handles site-specific tests and pushes to GitHub."""
        sites_to_check = self.settings.app_config.sites
        if not sites_to_check or not top_servers:
            return

        logger.info(f"Checking {len(sites_to_check)} sites...")
        for site in sites_to_check:
            if not site.enabled: continue
            
            logger.info(f"  Testing {site.url}...")
            try:
                valid_servers = await self.xray_service.evaluate_site_accessibility(site.url, top_servers)
                if valid_servers:
                    logger.info(f"  {len(valid_servers)} servers can access {site.url}")
                    await self.push_to_github(valid_servers, site.filename)
            except Exception as e:
                logger.error(f"  Failed to check {site.url}: {e}")
