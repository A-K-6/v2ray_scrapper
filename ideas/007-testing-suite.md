# Idea: Comprehensive Testing Suite

## Context
The project lacks a robust testing suite. There is `tests/test_parser.py`, but core logic (`XrayService`, `SubscriptionService`) is largely untested manually or implicitly.

## Proposal
Establish a proper testing culture with `pytest`.

## Implementation Steps
1.  **Unit Tests:**
    -   Test `UriGenerator` (ensure correct URI reconstruction).
    -   Test `ProxyParser` edge cases (malformed links, missing fields).
2.  **Mocking:**
    -   Mock `aiohttp.ClientSession` to test `fetch_subscription_servers` without real network calls.
    -   Mock `asyncio.create_subprocess_exec` to test `XrayService` without running the binary.
3.  **Integration Tests:**
    -   Spin up a local dummy SOCKS5 server (using python) and verify `XrayService` detects it.
4.  **CI:**
    -   Add a GitHub Action to run `pytest` on PRs.

## Benefits
-   **Confidence:** Refactoring (like Idea #2) becomes safe.
-   **Quality:** Catch regressions early.
