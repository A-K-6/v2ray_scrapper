#!/usr/bin/env python3
import asyncio
import base64
from contextlib import asynccontextmanager
from typing import List, Optional

from fastapi import FastAPI, HTTPException, Query, Response, Depends, Body
from fastapi.middleware.cors import CORSMiddleware
import uvicorn

from core.config import settings
from core.logger import logger
from models.server import ServerResponse, ProxyServer
from service.xray_service import XrayService
from service.subscription_manager import SubscriptionManager

# --- Service Initialization ---
xray_service = XrayService(settings)
manager = SubscriptionManager(settings, xray_service)

@asynccontextmanager
async def lifespan(app: FastAPI):
    # Startup logic
    asyncio.create_task(manager.start_periodic_update())
    yield
    # Shutdown logic (optional)
    await manager.storage_service.close()

# --- FastAPI App Setup ---
app = FastAPI(
    title="High-Speed V2Ray Server Tester",
    description="Aggregates, validates, and distributes high-performance V2Ray server configurations.",
    version="4.0",
    lifespan=lifespan
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["GET"],
    allow_headers=["*"],
)

@app.get("/health", summary="Check if the service is running")
def health_check():
    return {"status": "ok"}

async def get_top_servers_dep() -> List[ProxyServer]:
    servers = await manager.get_top_25()
    if not servers:
        raise HTTPException(
            status_code=503,
            detail="Cache not initialized. Please wait for the first update cycle.",
        )
    return servers

@app.get("/servers/live", summary="Trigger a live test and get top 25", response_model=ServerResponse)
async def get_servers_live():
    # Enqueue a new update cycle task
    await manager.enqueue_update()
    
    # Return what we currently have in cache, or a message that update is in progress
    top_25 = await manager.get_top_25()
    
    if not top_25:
        return {"count": 0, "servers": [], "message": "Update task enqueued. Please wait for the first cycle to complete."}
    
    return {"count": len(top_25), "servers": top_25}

@app.get("/cache", summary="Get cached top 25 servers", response_model=ServerResponse)
async def get_cached_servers(cached_servers: List[ProxyServer] = Depends(get_top_servers_dep)):
    return {"count": len(cached_servers), "servers": cached_servers}

@app.get("/cache/raw", summary="Get cached top 25 servers as raw subscription links")
async def get_cached_raw(cached_servers: List[ProxyServer] = Depends(get_top_servers_dep)):
    raw_links = [s.raw_uri for s in cached_servers]
    return Response("\n".join(raw_links), media_type="text/plain")

def filter_servers(servers: List[ProxyServer], countries: Optional[str] = None) -> List[ProxyServer]:
    if not countries:
        return servers
    target_countries = {c.strip().upper() for c in countries.split(",") if c.strip()}
    return [s for s in servers if s.country_code.upper() in target_countries]

@app.get("/cache/base64", summary="Get cached top 25 as a Base64 encoded subscription")
async def get_cached_base64(
    country: Optional[str] = Query(None, description="Filter by country codes (e.g., US,DE)"),
    cached_servers: List[ProxyServer] = Depends(get_top_servers_dep)
):
    filtered = filter_servers(cached_servers, country)
    raw_links = [s.raw_uri for s in filtered]
    combined = "\n".join(raw_links)
    encoded = base64.b64encode(combined.encode()).decode()
    return Response(encoded, media_type="text/plain")

@app.get("/cache/all/base64", summary="Get ALL cached servers as a Base64 subscription")
async def get_cached_all_base64(
    country: Optional[str] = Query(None, description="Filter by country codes (e.g., US,DE)")
):
    cached_all = await manager.get_all_cached()
    if not cached_all:
        raise HTTPException(status_code=503, detail="Cache not initialized.")
    
    filtered = filter_servers(cached_all, country)
    raw_links = [s.raw_uri for s in filtered]
    combined = "\n".join(raw_links)
    encoded = base64.b64encode(combined.encode()).decode()
    return Response(encoded, media_type="text/plain")

@app.get(
    "/subscription/site-specific",
    summary="Get a subscription for a specific site",
    description="Tests all cached servers against a target URL and returns a Base64 subscription.",
)
async def get_site_specific_subscription(
    url: str = Query(..., description="The target URL to test against (e.g., https://www.google.com)")
):
    successful_servers = await manager.get_site_specific_servers(url)

    if successful_servers is None:
         if manager.is_processing():
             raise HTTPException(status_code=429, detail="A test is already in progress.")
         else:
             raise HTTPException(status_code=503, detail="Cache is empty.")
    
    if not successful_servers:
        raise HTTPException(status_code=404, detail=f"No servers could access {url}.")

    raw_links = [s.raw_uri for s in successful_servers]
    combined = "\n".join(raw_links)
    encoded = base64.b64encode(combined.encode()).decode()
    return Response(encoded, media_type="text/plain")

@app.post(
    "/subscription/test",
    summary="Test submitted subscription content",
    response_model=ServerResponse,
    description="Accepts raw or Base64-encoded V2Ray subscription text and returns the working servers.",
)
async def test_subscription_content(content: str = Body(..., media_type="text/plain")):
    working_servers = await manager.test_subscription_content(content)

    if working_servers is None:
        raise HTTPException(status_code=429, detail="A test is already in progress.")

    if not working_servers:
        return {"count": 0, "servers": []}

    return {"count": len(working_servers), "servers": working_servers}

if __name__ == "__main__":
    uvicorn.run(app, host=settings.UVICORN_HOST, port=settings.UVICORN_PORT)
