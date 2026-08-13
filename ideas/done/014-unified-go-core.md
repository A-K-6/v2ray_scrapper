# Unified Go Core Service

## Status

Implemented on 2026-08-13.

The backend now runs as one Go process responsible for the HTTP API, scheduled refreshes, scraping, parsing, Xray testing, GeoIP enrichment, local atomic persistence, site-specific caching, and optional Git publishing.

Python, FastAPI, ARQ, Redis, and the separate Go helper executable were removed. Deployment is a single container plus a local `/data` volume.
