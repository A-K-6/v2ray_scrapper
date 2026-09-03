# V2Ray Scrapper Context

## Architecture

The backend is a single Go service. Keep it that way unless a demonstrated scaling requirement needs an external queue or shared database.

- `cmd/v2rays/main.go`: CLI entrypoint, dispatches to `internal/cli`
- `internal/cli`: `v2rays` subcommands (serve/refresh/get/test/sources/sites/token/config/doctor/tui)
- `internal/service`: refresh/test orchestration, concurrency gate, Git publishing
- `internal/api`: stable REST API boundary (+ OpenAPI in `docs.go`)
- `internal/config`: environment, XDG paths, YAML config
- `internal/proxy`: protocol parsing, URI generation, sing-box outbounds
- `internal/tester`: sing-box process lifecycle and latency testing
- `internal/scraper`: bounded subscription fetching
- `internal/store`: atomic JSON state, file registry, Redis backend
- `internal/singbox`: pinned sing-box auto-provisioning
- `internal/geoip`: optional MaxMind enrichment
- `internal/xdg`: config/data directory resolution
- `service.go`: refresh/test orchestration and concurrency gate
- `tester.go`: sing-box process lifecycle and latency testing
- `proxy.go`: protocol parsing, URI generation, and sing-box outbounds
- `scraper.go`: bounded subscription fetching
- `state.go`: atomic local JSON persistence
- `geoip.go`: optional MaxMind enrichment
- `git.go`: optional publishing
- `config.go`: environment and optional YAML configuration

`PRD.md` is the product requirements source of truth. Update `devlog.md` for major changes.

## Engineering rules

- Preserve the existing API contracts used by the Android client.
- Keep runtime state synchronized through `Service.mu`.
- All sing-box child processes must be canceled and reaped.
- Keep network and process concurrency bounded by configuration.
- Prefer the standard library; add dependencies only for capabilities that are impractical to implement safely in-house.
- Run `make test`, `make lint`, and `make build` before release.
