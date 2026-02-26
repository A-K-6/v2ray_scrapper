# Idea: High-Performance Go-based Xray Tester

## Context
Currently, the `XrayService` in Python manages Xray subprocesses by generating JSON configurations, writing them to disk, and starting/stopping the `xray` binary. While functional, this approach has overhead:
- **Subprocess Overhead:** Frequent starting and stopping of external processes.
- **Python Concurrency:** While `asyncio` is efficient, managing hundreds of concurrent proxy connections and subprocesses pushes Python's limits.
- **Resource Management:** Risks of zombie processes if termination fails.

## Proposal
Develop a standalone Go-based testing utility (`xray-tester`) that handles the core latency testing logic. Python will remain the orchestrator (scraping, parsing, API, distribution) but will delegate the heavy-duty testing to this Go utility.

## Key Features
- **Xray Integration:** Potentially use `xray-core` as a library for even tighter integration.
- **Massive Parallelism:** Leverage Go's goroutines to test thousands of servers simultaneously.
- **Standardized Interface:**
    - **Input:** JSON file or STDIN containing server configurations.
    - **Output:** JSON results (Latency, Success/Failure, Error details).
- **Optimized Testing:** Specialized connection pooling and timeout management.

## Implementation Steps
1.  **Go Utility Design:**
    - Define the JSON input/output schema.
    - Implement the testing logic using `xray-core` modules.
2.  **Integration:**
    - Modify `src/service/xray_service.py` to call the Go binary instead of the standard `xray` binary.
    - Handle the data exchange between Python and Go.
3.  **Dockerization:**
    - Update `Dockerfile` to include the Go compiler (for build) or use a multi-stage build to include the pre-compiled Go tester.

## Risks
- **Complexity:** Managing a multi-language codebase.
- **Xray Versioning:** Ensuring the Go tester stays in sync with `xray-core` updates.

## Benefits
- **Performance:** Significant reduction in total testing time.
- **Stability:** More robust resource management within a single Go process.
- **Scalability:** Easily handle larger server lists (5000+).
