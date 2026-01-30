# Idea: Database Persistence

## Context
The application currently uses in-memory lists (`_cached_all`) and simple JSON file serialization (`StorageService`) for persistence. This limits advanced querying, historical data analysis, and scalability.

## Proposal
Migrate to a proper database solution. **SQLite** is sufficient for single-instance deployments, while **Redis** is ideal for high-performance caching if scaling horizontally.

## Implementation Steps
1.  **Phase 1: SQLite**
    -   Use `SQLAlchemy` (async) or `Tortoise-ORM`.
    -   Define models: `Server` (url, protocol, country, last_tested, success_count, fail_count).
    -   On test completion, update the DB instead of a JSON file.
    -   API queries the DB directly.

2.  **Phase 2: Data Retention**
    -   Implement a cleanup job to remove servers not seen for X days.
    -   Track "Uptime" by logging test results over time.

## Benefits
-   **Persistence:** No data loss on crash (beyond the last transaction).
-   **Querying:** Efficient filtering by country, protocol, or latency without iterating over lists in Python.
-   **Analytics:** Ability to track "most reliable providers" or "best countries" over time.
