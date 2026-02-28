# Development Log - V2Ray Scrapper & Tester

This file tracks major changes, workflow executions, and project milestones.

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
