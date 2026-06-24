# Development Log - V2Ray Scrapper & Tester

This file tracks major changes, workflow executions, and project milestones.

## [2026-06-24] - Unified Go Core Service (Planned)
- **Task:** Draft design plan to migrate the FastAPI API server, Redis caching layer, and ARQ task worker entirely into a single Go-based service.
- **Changes:**
  - Created [014-unified-go-core.md](file:///home/aeen/Aeen/Code/PProjects/v2ray_scrapper/ideas/todo/014-unified-go-core.md) containing structural goals, migration steps, and execution path.
- **Goal:** Eliminate Python, Redis, and ARQ to make the core backend compile to a single, dependency-free binary with very low RAM and container footprint.
- **Status:** Planning.

## [2026-06-24] - Custom Testing & Settings Dashboard
- **Task:** Implement custom subscription and site testing via backend Core API, and add a dynamic configuration dashboard to the Jetpack Compose Android client to control subscription URLs, check targets, and latency/config limits.
- **Changes:**
  - Added new dynamic `/subscription/test-custom` endpoint in [main.py](file:///home/aeen/Aeen/Code/PProjects/v2ray_scrapper/src/main.py) accepting subscription list, target test URL, max delay, and config limit.
  - Extended `fetch_urls` dynamic scraping support in [scraper_service.py](file:///home/aeen/Aeen/Code/PProjects/v2ray_scrapper/src/service/scraper_service.py).
  - Updated `run_cycle` in [tester_service.py](file:///home/aeen/Aeen/Code/PProjects/v2ray_scrapper/src/service/tester_service.py) and `run_test_batch` in [xray_service.py](file:///home/aeen/Aeen/Code/PProjects/v2ray_scrapper/src/service/xray_service.py) to use dynamic test URLs and delay limits.
  - Refactored [MainActivity.kt](file:///home/aeen/Aeen/Code/PProjects/v2ray_scrapper/android-client/app/src/main/java/com/example/v2rayupdater/MainActivity.kt) into a 4-tab dashboard UI:
    - **Nodes View:** Triggers dynamic server-side Core tests or local socket checks.
    - **Subs View:** Toggles active subscriptions and supports adding custom URLs.
    - **Sites View:** Manages target check URLs (Google, YouTube, custom sites).
    - **Settings View:** Manages Core endpoint, maximum latency delay slider, and config limits slider.
  - Wrote unit tests in [test_custom_test.py](file:///home/aeen/Aeen/Code/PProjects/v2ray_scrapper/tests/test_custom_test.py).
  - Fixed Python 3.14 virtual environment dependency builds in [requirements.txt](file:///home/aeen/Aeen/Code/PProjects/v2ray_scrapper/requirements.txt) by updating Pydantic and typing-inspection packages.
- **Goal:** Give client users comprehensive control to check arbitrary configurations against selected censored websites on the Core.
- **Status:** Completed.

## [2026-06-23] - Android Client & GitHub Actions Compilation
- **Task:** Create a lightweight Native Android client app (using Kotlin + Jetpack Compose) to retrieve working proxy lists from the scraper API or GitHub, test latencies locally via TCP ping, and quickly open/copy configurations. Add CI/CD build actions.
- **Changes:**
  - Created [android-client](file:///home/aeen/Aeen/Code/PProjects/v2ray_scrapper/android-client/) containing a native Kotlin project using Jetpack Compose.
  - Implemented [MainActivity.kt](file:///home/aeen/Aeen/Code/PProjects/v2ray_scrapper/android-client/app/src/main/java/com/example/v2rayupdater/MainActivity.kt) for fetching subscription files, parsing VMess, VLESS, Trojan, and Shadowsocks URIs, displaying them in a dark theme list, copying configs, launching external V2Ray apps (v2rayNG, Nekobox, Sing-Box), and testing server latency directly via TCP Socket connections in parallel.
  - Configured Gradle dependencies and Android project structures via [build.gradle](file:///home/aeen/Aeen/Code/PProjects/v2ray_scrapper/android-client/build.gradle), [app/build.gradle](file:///home/aeen/Aeen/Code/PProjects/v2ray_scrapper/android-client/app/build.gradle), and [settings.gradle](file:///home/aeen/Aeen/Code/PProjects/v2ray_scrapper/android-client/settings.gradle).
  - Added a GitHub Actions workflow in [android.yml](file:///home/aeen/Aeen/Code/PProjects/v2ray_scrapper/.github/workflows/android.yml) to automatically compile debug (pre-signed) and release APKs on tag pushes (`v*`) or manual triggers and publish them to GitHub Releases.
- **Goal:** Provide a simple, download-and-use mobile client to sync and test working proxy subscriptions locally.
- **Status:** Completed.

## [2026-06-21] - Caching Overhaul & Dynamic Site Caching
- **Task:** Implement Redis caching for site-specific API responses, track dynamic site requests, and test them during background update cycles to provide fresh subscription configs.
- **Changes:**
  - Added `record_site_request` and `get_active_site_requests` to [storage_service.py](file:///home/aeen/Aeen/Code/tools/v2ray_scrapper/src/service/storage_service.py) for Redis-backed tracking of requested URL timestamps. Included automatic `HDEL` cleanup of expired/invalid fields.
  - Updated configuration settings in [config.py](file:///home/aeen/Aeen/Code/tools/v2ray_scrapper/src/core/config.py): increased `SITE_CACHE_TTL_SECONDS` default to 86400 (24 hours), introduced `SITE_REQUEST_MAX_AGE_DAYS` (default 5), and added `MAX_CONCURRENT_SITE_CHECKS` (default 3).
  - Updated [.env.sample](file:///home/aeen/Aeen/Code/tools/v2ray_scrapper/.env.sample) with the new configuration properties.
  - Refactored [subscription_manager.py](file:///home/aeen/Aeen/Code/tools/v2ray_scrapper/src/service/subscription_manager.py):
    - Cleaned up duplicate method definitions for `get_top_25`, `get_all_cached`, `get_site_specific_servers`, and `is_processing`.
    - Updated `refresh_cache_from_storage` to clear the local in-memory site cache only when the working server fingerprints change.
    - Updated `get_site_specific_servers` to query/store from Redis first before executing slow on-demand test runs.
    - Integrated background dynamic testing in `_run_integrations_background` for site URLs queried via API within the last 5 days, utilizing an `asyncio.Semaphore` based on the configured concurrency limit.
  - Created [test_caching.py](file:///home/aeen/Aeen/Code/tools/v2ray_scrapper/tests/test_caching.py) to run unit tests verifying the caching, Redis request tracking, and auto-cleanup logic.
- **Goal:** Cache site-specific subscriptions for a day, and automatically refresh them in background cycles if queried recently to serve them instantly.
- **Status:** Completed.

## [2026-06-16] - Performance Overhaul: High-Parallelism Go Tester
- **Task:** Optimized the Go tester to handle large volumes of servers (12k+) with "Go speed".
- **Changes:**
  - **Dynamic Concurrency:** Removed hardcoded limits in Go core. Now accepts `BatchSize` and `MaxParallelBatches` from Python.
  - **Parallelism Boost:** Increased default concurrency from 2 to 20 parallel batches (2,000 concurrent tests).
  - **Enhanced Diagnostics:** Implemented Xray `stderr` capture during port binding failures to identify root causes (e.g., port collisions or resource limits).
  - **Robust Port Waiting:** Reduced port binding timeout to 20s with more aggressive polling for faster recovery.
  - **Build System Fix:** Updated `Makefile` to correctly build the entire Go package.
- **Goal:** Reduce testing time for 12k servers from >15 minutes to ~2 minutes.
- **Status:** Completed.

## [2026-06-16] - Architectural Overhaul: Unified Go Core
- **Task:** Successfully implemented the three core roadmap items to improve performance, reliability, and stability.
- **Changes:**
  - **Go Scraping:** Moved subscription fetching to Go, implementing parallel goroutines for ultra-fast processing of multiple sources.
  - **Go Database Persistence:** Implemented a state management system in Go (`state.json`) that tracks server fail counts and history across restarts.
  - **Go-Git Integration:** Migrated all Git operations (clone, pull, rebase, push) to a robust Go-managed system, eliminating "resource busy" errors on Docker mount points.
  - **Unified Command Interface:** The Go tester now supports complex commands like `scrape-and-test` and `git-push`, reducing Python's role to orchestration and API serving.
- **Goal:** Create a high-performance, stable, and production-ready tool with minimal external dependencies.
- **Status:** Completed.

## [2026-06-16] - Optimization: Native Go Parsing for Xray Tester
- **Task:** Migrated URI parsing and Xray configuration generation from Python to the Go-based `xray-tester`.
- **Changes:**
  - Implemented `parseRawURI` in Go, supporting VMess, VLESS, Trojan, Shadowsocks, and Hysteria2.
  - Refactored `xray_service.py` to pass raw URIs to the Go subprocess instead of dynamically building large JSON payloads in Python.
  - The Go tester now internally generates the unified Xray `inbounds`, `outbounds`, and `routing` configuration for batch testing.
- **Goal:** Significantly improve testing performance and reduce Python memory overhead by leveraging Go's speed for parsing and structure generation.
- **Status:** Completed.

## [2026-06-16] - Bug Fix: Git Uploader Mount Point & Corruption Resilience
- **Task:** Resolved critical failures in `GitUploader` where corrupted repositories on Docker mount points caused "Device or resource busy" errors.
- **Changes:**
  - Implemented `_clear_repo_dir` to delete directory contents instead of the directory itself, respecting Docker mount points.
  - Enhanced repository validation using `git rev-parse --is-inside-work-tree` to reliably detect corrupted `.git` states.
  - Replaced all instances of `shutil.rmtree(self.repo_dir)` with the safe `_clear_repo_dir` method.
- **Goal:** Ensure the automated push cycle can recover from any git state without manual intervention or worker crashes.
- **Status:** Completed.

## [2026-02-28] - Optimization: Decoupled Integrations & Task Concurrency
- **Task:** Prevented slow GitHub pushes and site checks from blocking the next update cycle.
- **Changes:**
  - Modified `SubscriptionManager.update_cycle` to release the `_processing_lock` immediately after the testing phase.
  - Moved GitHub pushes and site accessibility checks into background tasks (`asyncio.create_task`).
  - Added an `asyncio.Lock` to `IntegrationService` to prevent concurrent Git operations from conflicting on the same repository directory.
- **Goal:** Improve worker throughput and ensure the scheduled cron jobs are never skipped due to slow network operations.
- **Status:** Completed.

## [2026-02-28] - Bug Fix: Xray Port Binding Stability & Diagnostics
- **Task:** Addressed "failed to bind port" errors during parallel testing.
- **Changes:**
  - Reduced `BATCH_SIZE` from 500 to 100 for better stability.
  - Increased `TesterService` concurrency to 10 (1,000 parallel tests total).
  - Enhanced `go-tester` to capture and report Xray `stderr` on failure.
  - Increased Xray startup timeout in `go-tester` from 10s to 30s.
- **Goal:** Resolve resource contention issues and improve error visibility.
- **Status:** Completed.

## [2026-02-28] - Bug Fix: arq Timeout & Performance Optimization
- **Task:** Fixed `TimeoutError` in `run_update_cycle_task` and optimized testing performance.
- **Changes:**
  - Increased `arq` `job_timeout` from 300s to 1200s in `WorkerSettings`.
  - Parallelized batch testing in `TesterService` using `asyncio.Semaphore` and unique port ranges.
  - Parallelized site-specific accessibility checks in `IntegrationService`.
  - Modified `XrayService` to support configurable `base_port` for parallel execution.
- **Goal:** Prevent task cancellation during long-running update cycles and reduce overall processing time.
- **Status:** Completed.

## [2026-02-28] - Idea Implemented: Distributed Task Architecture & Resilient Scraping
- **Task:** Successfully transitioned to a distributed, task-based architecture using `arq`.
- **Goal:** Decoupled the update cycle, implemented resilient scraping with exponential backoff (tenacity), and introduced a standalone task worker to scale testing.
- **Reference:** `ideas/done/012-distributed-task-architecture.md`.
- **Status:** Completed.

## [2026-02-28] - Idea Created: Distributed Task Architecture & Resilient Scraping
- **Task:** Created a new idea for transitioning to a distributed, task-based architecture.
- **Goal:** Decouple the update cycle, implement resilient scraping with exponential backoff, and allow standalone Go-based tester workers for high-scale testing.
- **Reference:** `ideas/todo/012-distributed-task-architecture.md`.
- **Status:** Completed.

## [2026-02-27] - Makefile Creation
- **Task:** Created a `Makefile` to simplify development, testing, and deployment.
- **Goal:** Provide easy-to-use commands for environment setup, building Go components, running the FastAPI app, and Docker management.
- **Targets Added:** `setup`, `build-go`, `run`, `test`, `docker-up`, `lint`, `fmt`, `clean`.
- **Status:** Completed.

## [2026-02-27] - Idea Implemented: Comprehensive Codebase Cleanup
- **Task:** Executed a major architectural refactoring to improve modularity and type safety.
- **Goal:** Split monolithic `SubscriptionService` into specialized services (`ScraperService`, `TesterService`, `IntegrationService`, `SubscriptionManager`), unified configuration into a Pydantic-based manager, and implemented protocol-specific Pydantic models.
- **Runtime:** Upgraded to Python 3.13-slim (stable for source builds without Rust toolchain complexity).
- **Reference:** `ideas/done/011-codebase-cleanup.md`.
- **Status:** Completed.

## [2026-02-27] - Idea Created: Comprehensive Codebase Cleanup
- **Task:** Proposed a major architectural refactoring to improve modularity and type safety.
- **Goal:** Split monolithic `SubscriptionService`, unify configuration, and implement protocol-specific models for parsing and Xray config generation.
- **Reference:** `ideas/done/011-codebase-cleanup.md`.
- **Status:** Completed.

## [2026-02-26] - Idea Implemented: Go-based Xray Tester
- **Task:** Implemented a Go-based testing utility (`xray-tester`) to orchestrate proxy testing.
- **Goal:** Replaced Python `aiohttp_socks` concurrent latency testing with efficient Go goroutines using `net/http` to resolve bottlenecking and simplify subprocess management.
- **Reference:** `ideas/done/010-go-xray-tester.md`.
- **Status:** Completed.

## [2026-02-26] - Idea Created: Go-based Xray Tester
- **Task:** Created a new idea for a Go-based testing utility.
- **Goal:** Improve performance and scalability of server testing.
- **Reference:** `ideas/todo/010-go-xray-tester.md`.
- **Status:** Planning.

## [2026-02-26] - GEMINI.md Update
- **Task:** Updated `GEMINI.md` to follow the new modular context and workflow structure.
- **Changes:**
    - Standardized "Idea Generation" and "Implementation" workflows.
    - Defined `PRD.md` as the Single Source of Truth.
    - Added mandatory `devlog.md` updates to all workflows.
- **Status:** Completed.
