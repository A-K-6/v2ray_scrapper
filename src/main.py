#!/usr/bin/env python3
import asyncio
import base64
import sys
from typing import List, Dict

from fastapi import FastAPI, HTTPException, Query, Response, Depends
from fastapi.middleware.cors import CORSMiddleware
import uvicorn

from core.config import settings
from models.server import ServerResponse
from service.xray_service import XrayService
from service.subscription_service import SubscriptionService

# --- Service Initialization ---
xray_service = XrayService(settings)
subscription_service = SubscriptionService(settings, xray_service)

# --- FastAPI App Setup ---
app = FastAPI(
    title="High-Speed V2Ray Server Tester",
    description="Fetches V2Ray servers, performs real-delay tests, and exposes the fastest servers via an API.",
    version="3.2",
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["GET"],
    allow_headers=["*"],
)

@app.on_event("startup")
async def startup_event():
    asyncio.create_task(subscription_service.start_periodic_update())

@app.get("/health", summary="Check if the service is running")
def health_check():
    return {"status": "ok"}

async def get_top_servers_dep() -> List[Dict]:
    servers = await subscription_service.get_top_25()
    if servers is None:
        raise HTTPException(
            status_code=503,
            detail="Cache not initialized. Please wait or try the /servers/live endpoint.",
        )
    return servers

@app.get("/servers/live", summary="Get top 25 servers (live test)", response_model=ServerResponse)
async def get_servers_live():
    if subscription_service.is_processing():
        raise HTTPException(status_code=429, detail="A test is already in progress.")
    
    # Trigger an update manually? Or just return current?
    # The original code called compute_top_servers() directly which updates nothing but returns result.
    # To keep behavior:
    async with subscription_service._processing_lock:
        top_servers = await subscription_service.compute_top_servers()
        
    if not top_servers:
        raise HTTPException(status_code=503, detail="No servers available or all tests failed.")
    top_25 = top_servers[:25]
    return {"count": len(top_25), "servers": top_25}

@app.get("/cache", summary="Get cached top 25 servers", response_model=ServerResponse)
async def get_cached_servers(cached_servers: List[Dict] = Depends(get_top_servers_dep)):
    return {"count": len(cached_servers), "servers": cached_servers}

@app.get("/cache/raw", summary="Get cached top 25 servers as raw subscription links")
async def get_cached_raw(cached_servers: List[Dict] = Depends(get_top_servers_dep)):
    raw_links = [s["raw_uri"] for s in cached_servers]
    return Response("\n".join(raw_links), media_type="text/plain")

@app.get("/cache/base64", summary="Get cached top 25 as a Base64 encoded subscription")
async def get_cached_base64(cached_servers: List[Dict] = Depends(get_top_servers_dep)):
    raw_links = [s["raw_uri"] for s in cached_servers]
    combined = "\n".join(raw_links)
    encoded = base64.b64encode(combined.encode()).decode()
    return Response(encoded, media_type="text/plain")

@app.get("/cache/all/base64", summary="Get ALL cached servers as a Base64 subscription")
async def get_cached_all_base64():
    cached_all = await subscription_service.get_all_cached()
    if cached_all is None:
        raise HTTPException(status_code=503, detail="Cache not initialized.")
    raw_links = [s["raw_uri"] for s in cached_all]
    combined = "\n".join(raw_links)
    encoded = base64.b64encode(combined.encode()).decode()
    return Response(encoded, media_type="text/plain")

@app.get(
    "/subscription/site-specific",
    summary="Get a subscription for a specific site",
    description="Tests all cached servers against a target URL and returns a Base64 subscription of the servers that can access it.",
)
async def get_site_specific_subscription(
    url: str = Query(..., description="The target URL to test against (e.g., https://www.google.com)")
):
    successful_servers = await subscription_service.get_site_specific_servers(url)

    if successful_servers is None:
         # This means either cache empty or processing locked
         if subscription_service.is_processing():
             raise HTTPException(status_code=429, detail="A test is already in progress. Please wait.")
         else:
             raise HTTPException(status_code=503, detail="Cache is empty. Please wait for it to populate.")
    
    if not successful_servers:
        raise HTTPException(status_code=404, detail=f"No servers could successfully access {url}.")

    print(f"Found {len(successful_servers)} servers that can access {url}.")
    raw_links = [s["raw_uri"] for s in successful_servers]
    combined = "\n".join(raw_links)
    encoded = base64.b64encode(combined.encode()).decode()
    return Response(encoded, media_type="text/plain")

if __name__ == "__main__":
    # Basic check
    import os
    if not os.path.exists(settings.XRAY_PATH):
        print(f"WARNING: Xray executable not found at '{settings.XRAY_PATH}'. App may not function correctly.", file=sys.stderr)
    
    uvicorn.run(app, host=settings.UVICORN_HOST, port=settings.UVICORN_PORT)
