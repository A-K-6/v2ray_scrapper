# Idea: Distributed Task Architecture & Resilient Scraping

## 1. Overview
Transition the current monolithic and synchronous update cycle into a decoupled, task-based distributed architecture. This will enable the system to handle thousands of servers across multiple workers, improve fault tolerance, and ensure high availability of the scraper even during intense testing phases.

## 2. Problem Statement
- **Monolithic Bottleneck:** The `SubscriptionManager.update_cycle` performs Scrape -> Test -> Enrich -> Store -> Push in a single sequence. If one stage fails or hangs, the entire cycle is impacted.
- **Scalability Limits:** Testing is limited by the resources of a single machine. Large subscription lists (5000+ servers) can take a long time to process.
- **Transient Failures:** Upstream subscription URLs often fail temporarily. The current scraper lacks a sophisticated retry mechanism with exponential backoff.
- **Resource Contention:** Heavy testing can impact the responsiveness of the FastAPI's management of the cache.

## 3. Proposed Solution

### 3.1 Task-Based Decoupling
Introduce a lightweight async task queue (e.g., `ARQ` or a custom Redis-based queue) to split the workflow:
1.  **Scraper Task:** Triggered by a cron/timer. Fetches and pushes "raw" servers to a `pending_test` queue.
2.  **Tester Worker(s):** Standalone processes (can be multiple instances) that pull batches from `pending_test`, run the `xray-tester`, and push results to a `test_results` queue.
3.  **Aggregator Task:** Pulls from `test_results`, performs Geo-IP enrichment, updates Redis/Database, and triggers Integrations.
4.  **Integration Task:** Handles Git pushes and site-specific checks as independent background jobs.

### 3.2 Resilient Scraping
- Implement the `tenacity` library or a custom `asyncio` backoff wrapper for `ScraperService`.
- **Strategy:** Exponential backoff (e.g., 2s, 4s, 8s, 16s) with jitter to prevent thundering herd issues on upstream providers.

### 3.3 Standalone Go Tester Worker
- Modify the `go-tester` to optionally run in "Worker Mode," where it listens to a Redis list instead of being called as a subprocess with JSON flags.
- This allows the tester to run on a separate, high-bandwidth machine while the Python API remains on a lightweight management node.

## 4. Technical Changes

### Backend (Python)
- Integrate `arq` or a similar library for task management.
- Refactor `SubscriptionManager` to act as a "Dispatcher" rather than an "Orchestrator."
- Update `ScraperService` with retry decorators.

### Tester (Go)
- Extend `src/go-tester/main.go` to include a `--worker` flag.
- Implement Redis client logic in Go to pull/push tasks.

### Infrastructure
- Update `docker-compose.yml` to support scaling the `tester` service.
- Ensure Redis is the central state coordinator.

## 5. Benefits
- **High Throughput:** Horizontal scaling of testers allows processing of massive server lists.
- **Reliability:** Failure in a Git push or a single scrape attempt won't block the rest of the system.
- **Responsiveness:** The API remains fast and lightweight as heavy lifting is moved to background workers.

## 6. Roadmap
1.  [ ] Implement exponential backoff in `ScraperService`.
2.  [ ] Setup `arq` worker infra in Python.
3.  [ ] Decouple `update_cycle` into independent tasks.
4.  [ ] (Phase 2) Implement "Worker Mode" in `go-tester`.
