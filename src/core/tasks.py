import asyncio
from typing import List, Optional
from arq import create_pool, cron
from arq.connections import RedisSettings
from loguru import logger

from core.config import settings as app_settings
from service.subscription_manager import SubscriptionManager
from service.xray_service import XrayService

# Global service instances for the worker
xray_service = XrayService(app_settings)
manager = SubscriptionManager(app_settings, xray_service)

async def startup(ctx):
    """Worker startup logic."""
    logger.info("Starting task worker...")
    await manager.initialize()
    ctx['manager'] = manager

async def shutdown(ctx):
    """Worker shutdown logic."""
    logger.info("Shutting down task worker...")
    await manager.storage_service.close()

async def run_update_cycle_task(ctx):
    """Periodic task to trigger the full update cycle."""
    manager = ctx['manager']
    logger.info("Executing periodic update cycle task...")
    await manager.update_cycle()

async def run_site_check_task(ctx, url: str):
    """Task to check accessibility of a specific site."""
    manager = ctx['manager']
    logger.info(f"Executing site check task for {url}...")
    await manager.get_site_specific_servers(url)

class WorkerSettings:
    """arq worker configuration."""
    functions = [run_update_cycle_task, run_site_check_task]
    job_timeout = 1200 # 20 minutes
    cron_jobs = [
        cron(run_update_cycle_task, second=0, minute=list(range(0, 60, max(1, app_settings.CACHE_INTERVAL_SECONDS // 60))))
    ]
    redis_settings = RedisSettings(
        host=app_settings.REDIS_HOST,
        port=app_settings.REDIS_PORT,
        database=app_settings.REDIS_DB,
        password=app_settings.REDIS_PASSWORD or None
    )
    on_startup = startup
    on_shutdown = shutdown
