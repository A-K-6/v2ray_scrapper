# Idea: Refactor Xray Execution (DRY) (Completed)

## Status
**Implemented** on 2026-01-30.

## Context
In `src/service/xray_service.py`, the logic to:
1.  Create a temporary configuration file.
2.  Start the Xray subprocess.
3.  Wait for ports to open (polling).
4.  Handle timeouts/errors.
5.  Terminate/Kill the process.
6.  Cleanup the temporary file.

Is duplicated between `run_test_batch` and `evaluate_site_accessibility`. This violation of DRY (Don't Repeat Yourself) increases maintenance burden and risk of bugs (e.g., fixing a resource leak in one place but forgetting the other).

## Proposal
Extract the Xray process lifecycle management into a Python Context Manager.

## Implementation Steps
1.  Create a class `XrayProcessContext` (async context manager). (Done)
    -   `__init__`: Takes configuration dict and settings.
    -   `__aenter__`: Writes config, starts process, waits for ports. Returns the process object (or just signals readiness).
    -   `__aexit__`: Terminates/kills process, cleans up temp file.
2.  Refactor `run_test_batch` to use: (Done)
    ```python
    async with XrayProcessContext(config, ports) as xray:
        # Run tests
    ```
3.  Refactor `evaluate_site_accessibility` to use the same context manager. (Done)

## Benefits
-   **Maintainability:** Single point of truth for Xray process management.
-   **Reliability:** Ensures cleanup always happens (even on error) via `__aexit__`.
-   **Readability:** Reduces method size and complexity.