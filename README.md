# V2Ray Scrapper

A small, single-process Go service that scrapes proxy subscriptions, tests them through Xray, enriches working nodes with GeoIP data, and serves ready-to-use subscriptions.

The backend has one binary and one required companion executable: `v2ray-scrapper` and a checksum-verified, pinned Xray release. It does not require Python, Redis, ARQ, or a separate worker.

## What it provides

- VLESS, VMess, Trojan, Shadowsocks, and Hysteria 2 parsing
- Real HTTP latency tests through Xray-managed SOCKS listeners
- Bounded batch concurrency
- In-memory top/all/site-specific caches
- Atomic JSON persistence across restarts
- Periodic background refreshes
- Optional GeoIP remarks and Git publishing
- The existing REST API used by the Android client

## Quick start

```bash
make up
curl http://localhost:8084/health
```

`make up` runs initialization and performs a Docker rebuild before starting the service, so a stale Python-era image cannot be reused. It also runs the container as the current host user so `./data/state.json` remains writable on Linux bind mounts. `make init` asks for the host port, refresh interval, and maximum candidates, then creates `.env`, `config.yaml`, and the local runtime directories. Press Enter at every prompt to accept the production defaults. Existing custom sources and Git credentials are preserved; Python-era performance settings are migrated to the bounded Go defaults. Use `make init ARGS=--force` only when you intend to regenerate the files completely.

For unattended setup, use `make init ARGS=--non-interactive`. Run `make init ARGS=--help` to see the supported environment overrides.

The API listens on port `8084` by default. Persistent state is written to `./data/state.json`.

Interactive Swagger documentation is available at `http://localhost:8084/swagger`. The machine-readable OpenAPI document is served at `/openapi.json`.

## Local development

Go 1.23+ and an Xray binary are required.

```bash
make init
make setup
make test
make run
```

Environment files are not loaded by the binary itself. Export the variables you need, or use Docker Compose, which reads `.env` automatically.

## API

| Method | Endpoint | Result |
| --- | --- | --- |
| GET | `/health` | Liveness response |
| GET | `/swagger` | Interactive Swagger UI |
| GET | `/openapi.json` | OpenAPI 3.1 specification |
| GET | `/servers/live` | Starts a refresh and returns the current top 25 |
| GET | `/cache` | Top 25 as JSON |
| GET | `/cache/raw` | Top 25 as newline-separated URIs |
| GET | `/cache/base64` | Top 25 as a Base64 subscription |
| GET | `/cache/all/base64` | All working nodes as Base64 |
| GET | `/subscription/site-specific?url=...` | Nodes that can access the target URL |
| POST | `/subscription/test` | Tests a raw or Base64 text subscription |
| POST | `/subscription/test-custom` | Tests supplied URLs/content with custom limits |

The Base64 endpoints accept an optional comma-separated country filter, for example `?country=US,DE`.

## Configuration

All runtime settings use environment variables. See [.env.sample](.env.sample) for the complete, documented set. `config.yaml` remains optional and is only used for named site checks and Git output filenames.

Useful defaults:

- `CACHE_INTERVAL_SECONDS=600`
- `TEST_TIMEOUT=6`
- `MAX_DELAY_MS=10000`
- `BATCH_SIZE=20`
- `MAX_CONCURRENT_BATCHES=3`
- `MAX_CANDIDATES=60`
- `STATE_FILE_PATH=/data/state.json` in Docker

## Commands

```bash
make help
make init
make test
make e2e-public
make lint
make build
make up
make logs
make down
```

The Android client remains under `android-client/` and continues using the same API routes.

`make e2e-public` performs the network-intensive release check against the three default public GitHub feeds. It requires Docker, curl, and Python 3, enforces a first usable cache within 120 seconds, exercises the public API formats and live proxy testing, and verifies persisted-cache availability after restart.
