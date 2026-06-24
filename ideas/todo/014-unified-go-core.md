# Idea: Unified Go Core Service

## Status
**Proposed** (Approved by user on 2026-06-24)

## Context
Currently, the backend consists of a Python FastAPI API web server, a Redis instance for caching and job storage, an ARQ task worker, and a Go-based tester binary (`xray-tester`). This multi-runtime configuration increases the Docker image footprint (~500MB), resource consumption (~150MB RAM), and introduces compilation dependencies (e.g. C-extensions and Python 3.14 compatibility).

## Proposal
Consolidate the entire backend architecture into a single Go-based compiled service. By migrating the FastAPI HTTP endpoints, background scheduling, memory caching, and GeoIP enrichment entirely into Go, we can eliminate the Python runtime, virtual environments, Redis, and ARQ task workers.

## Implementation Steps

### 1. Go Web Service
- Implement a native Go HTTP server (`net/http` or a lightweight router like `chi`) to expose the subscription API:
  - `GET /health`
  - `GET /servers/live`
  - `GET /cache` (JSON, Raw, Base64)
  - `POST /subscription/test-custom`
- Use native Go channels to queue and limit concurrent site-specific and custom test runs.

### 2. Native Job Scheduling
- Replace Redis and ARQ workers with native Go tickers (`time.Ticker`) to trigger scheduled background scrapes and test runs.

### 3. Local Cache & Database Persistence
- Replace Redis storage with standard local memory caching using thread-safe structures (`sync.Map`) and persistent SQLite or local JSON files (`state.json`).
- Implement native Go GeoIP lookups using `github.com/oschwald/geoip2-golang`.

### 4. Codebase & Deployment Cleanup
- Remove the `/src` Python folder, `requirements.txt`, `Dockerfile` Python installations, and Redis configuration from `docker-compose.yml`.
- Build the final deployment as a single Go executable inside a minimal Alpine-based container.

## Benefits
- **Zero Runtime Dependencies:** No Python virtual environments, pip, or external Redis server.
- **Drastically Lower Footprint:** Memory usage reduced to <15MB RAM and Docker image size reduced to ~30MB.
- **Fast Startup:** Instant compilation and daemon startup.
