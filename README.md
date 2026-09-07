# V2Ray Scrapper — now a standalone `v2rays` CLI

A small Go service that scrapes proxy subscriptions, tests them through sing-box, enriches working nodes with GeoIP data, and serves ready-to-use subscriptions.

**New:** no Docker required. Install one binary (`v2rays`), run `v2rays tui`, and it auto-provisions sing-box, keeps a local registry/state, and serves the same REST API.

## Quick start (60 seconds)

```bash
# 1. Install
curl -fsSL https://raw.githubusercontent.com/A-K-6/v2ray_scrapper/main/install.sh | bash

# 2. First-time setup (config, token, sing-box — all automatic)
v2rays config init
v2rays doctor          # everything should be ✓

# 3. Easiest way to use it
v2rays tui             # interactive menu: refresh, browse nodes, manage feeds

# ...or go straight to working proxies:
v2rays refresh --out subscription.txt   # one scrape+test cycle, Base64 output
v2rays serve                            # HTTP API on :8084 (Swagger at /swagger)
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/A-K-6/v2ray_scrapper/main/install.ps1 | iex
v2rays config init
v2rays tui
```

> Pin a version with `V2RAYS_VERSION=vX.Y.Z`. Releases are built by `.goreleaser.yml` (linux/darwin/windows, amd64/arm64).

## Install details

## CLI usage

```bash
v2rays tui                                    # easiest: interactive menu
v2rays serve                                  # run the HTTP API (same as Docker)
v2rays refresh --out subscription.txt         # one scrape+test cycle, export base64
v2rays get --format raw --limit 25            # export cache without network
v2rays get --all --country US,DE -f base64
v2rays test --sub https://example.com/sub.txt --limit 50
v2rays sources list | add <url...> | rm <url...>
v2rays sites list | add <url> [file] | rm <url>
v2rays token gen | show                       # MANAGEMENT_TOKEN for management routes
v2rays config init | show | path
v2rays doctor                                 # sing-box, geoip, state, token health
```

Standalone defaults (override with env): config at `~/.config/v2rays/config.yaml`, state/registry/sing-box under `~/.local/share/v2rays/` (Windows: `%AppData%`/`%LocalAppData%`). `SING_BOX_PATH`, `STATE_FILE_PATH`, `GEOIP_DB_PATH`, `YAML_CONFIG_PATH`, `REDIS_URL` still honored. Without `REDIS_URL`, sources/sites persist in a local `registry.json` instead of Redis. Missing sing-box is auto-downloaded (pinned v1.13.12, checksum-verified on linux).

## Project layout

```text
cmd/v2rays/        entrypoint (v2rays binary)
internal/
  api/             REST API + OpenAPI/Swagger
  cli/             subcommands, TUI menu, config init, doctor
  config/          env config, XDG paths, YAML sites
  proxy/           protocol parsing + URI generation
  tester/          sing-box testing engine
  scraper/         subscription fetching
  service/         orchestration + Git publishing
  store/           JSON state, file registry, Redis backend
  singbox/         sing-box auto-provisioning
  geoip/           country enrichment
  xdg/             config/data directories
android-client/    Android updater app (same API)
docs/              GitHub Pages site
scripts/           init, e2e, helpers
assets/            GeoIP database
```

## Docker (legacy, still supported)

The backend is one binary (`v2rays`, with a `v2ray-scrapper` symlink kept for compatibility) plus a checksum-verified, pinned sing-box release — auto-provisioned for standalone installs, baked into the Docker image otherwise. Docker Compose also starts Redis for durable source/site management and site-check caches; standalone mode uses a local `registry.json` file instead. Neither mode requires Python, ARQ, or a separate worker.

## What it provides

- VLESS, VMess, Trojan, Shadowsocks, and Hysteria 2 parsing
- Two-pass HTTPS validation through sing-box-managed SOCKS listeners
- Bounded batch concurrency
- In-memory top/all caches plus site-specific caches (Redis in Docker, local file standalone)
- Managed subscription-source and preloaded-site storage (Redis in Docker, `registry.json` standalone)
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

Go 1.23+ is required for development. A sing-box binary is auto-downloaded on first run (pinned release, checksum-verified on Linux); set `SING_BOX_PATH` to use your own.

```bash
make init
make setup
make test
make run
```

The standalone binary auto-loads `~/.config/v2rays/.env` (see `v2rays config init`); Docker Compose reads the repo-local `.env` automatically. Plain `go run .` / `make run` additionally honor exported variables.

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
| GET/POST/DELETE | `/subscriptions` | Lists, adds, or removes managed source URLs |
| GET/POST/DELETE | `/sites` | Lists, adds/updates, or removes preloaded site checks |

The Base64 endpoints accept an optional comma-separated country filter, for example `?country=US,DE`.

Management routes require `MANAGEMENT_TOKEN` and accept either `Authorization: Bearer <token>` or `X-API-Key: <token>`. They remain disabled when the token is empty. Added sources are used by following refreshes; added sites are automatically checked after a refresh and their result sets are cached with `SITE_CACHE_TTL_SECONDS` (Redis in Docker, local file standalone).

The default high-volume profile runs 10 sing-box batches of 100 nodes at once (up to 1,000 in-flight probes). A 10,000-node cycle therefore needs 10 waves; failed nodes stop after their first probe, while accepted nodes must pass twice. Real elapsed time still depends on host limits, upstream latency, and the percentage of healthy nodes. Reduce the three concurrency settings on smaller machines.

Each subscription source is retried once. If any configured source still fails, the scheduled refresh is aborted and the previous working/candidate cache is preserved instead of being aged out by an incomplete scrape. Logs report per-source hostnames and parsed candidate counts without exposing URL paths or query credentials.

```bash
curl -H "Authorization: Bearer $MANAGEMENT_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"urls":["https://example.com/subscription"]}' \
  http://localhost:8084/subscriptions

curl -H "Authorization: Bearer $MANAGEMENT_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://www.openai.com","filename":"openai_valid.txt"}' \
  http://localhost:8084/sites
```

## Configuration

All runtime settings use environment variables. See [.env.sample](.env.sample) for the complete, documented set. `config.yaml` remains optional and is only used for named site checks and Git output filenames.

Useful defaults:

- `CACHE_INTERVAL_SECONDS=600`
- `TEST_TIMEOUT=6`, `TEST_ATTEMPTS=2`
- `MAX_DELAY_MS=10000`
- `BATCH_SIZE=100`
- `MAX_CONCURRENT_BATCHES=10`
- `MAX_CONCURRENT_TESTS=100`
- `MAX_CANDIDATES=10000`
- `REDIS_URL=redis://redis:6379/0`
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

Redis may print a `vm.overcommit_memory` warning because that kernel setting belongs to the Docker host/VM and cannot be changed safely by this unprivileged Compose stack. It does not mean Redis failed to start. On a native Linux host, an administrator can persist `vm.overcommit_memory=1` with the host's sysctl configuration; Docker Desktop users must change it in Docker Desktop's Linux VM if they want to suppress the warning.
