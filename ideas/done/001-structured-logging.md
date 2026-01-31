# Idea: Structured Logging (Completed)

## Status
**Implemented** on 2026-01-30.

## Context
Currently, the application uses `print()` statements and `sys.stderr` for logging. This makes it difficult to parse logs, control verbosity levels (DEBUG, INFO, ERROR), and integrate with centralized logging systems.

## Proposal
Replace all `print()` calls with a proper logging framework.

## Recommended Library
**`loguru`** is recommended for its simplicity and out-of-the-box features (coloring, serialization, exception catching). Alternatively, the standard `logging` module can be used.

## Implementation Steps
1.  Add `loguru` to `requirements.txt`. (Done)
2.  Create a central logger configuration in `src/core/logger.py`. (Done)
    -   Configure sink for `sys.stderr`.
    -   Configure file rotation (optional).
    -   Set format: `"{time} | {level} | {message}"`.
3.  Replace `print(...)` with `logger.info(...)`. (Done)
4.  Replace `print(..., file=sys.stderr)` with `logger.error(...)` or `logger.warning(...)`. (Done)
5.  Use `logger.exception` in `try/except` blocks to capture stack traces automatically. (Done)

## Benefits
-   **Debuggability:** Easier to filter logs by severity.
-   **Observability:** Structured logs (JSON) can be ingested by tools like ELK, Loki, or Datadog.
-   **Cleanliness:** Separates operational output from debugging info.