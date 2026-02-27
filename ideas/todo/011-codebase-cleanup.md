# Idea: Comprehensive Codebase Cleanup and Architectural Refactoring

## Status
- **Status**: Todo
- **Priority**: High
- **Created**: 2026-02-27

## Problem Statement
The current codebase has several architectural bottlenecks:
0.  **Outdated Runtime**: Currently running on Python 3.11, missing out on significant performance improvements and modern `asyncio` features available in Python 3.14 (the current 2026 stable release).
1.  **Monolithic Service**: `SubscriptionService` acts as a "God Class," handling everything from scraping to GitHub pushes.
2.  **Scattered Protocol Logic**: Logic for URI parsing, URI generation, and Xray configuration is distributed across multiple services and static classes, often using primitive dictionaries.
3.  **Configuration Inconsistency**: Settings are split between `.env` (Pydantic) and `config.yaml` (custom loader), leading to overlap and confusion.
4.  **State Management**: Complex locking mechanisms are used to manage in-memory state, which could be simplified by leaning more on Redis.

## Proposed Solution
Refactor the application into a more modular, type-safe architecture using specialized protocol models and decoupled services.

### 1. Protocol Models & Specialized Parsing
- Define a base `ProxyServer` model using Pydantic.
- Implement specialized models (`VlessServer`, `VmessServer`, `TrojanServer`, `ShadowsocksServer`, `Hysteria2Server`).
- Move the following logic into these models:
    - `from_uri(uri: str)`: Parsing logic.
    - `to_uri() -> str`: Generation logic.
    - `to_xray_outbound() -> Dict`: Xray configuration fragment.
- Update `ProxyParser` to act as a factory that returns these model instances.

### 2. Service Decoupling
Split `SubscriptionService` into focused components:
- **`ScraperService`**: Responsible for HTTP fetching, raw decoding, and initial deduplication based on fingerprints.
- **`ProxyTester`**: Orchestrates testing cycles, manages GeoIP enrichment, and sorts results by latency.
- **`SubscriptionManager`**: High-level orchestrator that handles the periodic update loop, cache management, and triggers external integrations.
- **`IntegrationService`**: Dedicated service for GitHub pushes and other external notifications.

### 3. Unified Configuration & State
- Merge `core/config.py` and `core/yaml_config.py` into a single configuration manager.
- Standardize on Pydantic for all configuration validation.
- Make Redis the primary source of truth for "Working" and "Candidate" servers to reduce in-memory state complexity.

### 4. API & Lifecycle Modernization
- Transition from `@app.on_event("startup")` to the modern FastAPI `lifespan` context manager.
- Use the new protocol models for API request/response validation.

### 5. Testing Suite
- Expand unit tests in `tests/` to cover all protocol models (parsing and Xray config generation).
- Implement basic integration tests for the core scraping and testing workflow.

## Expected Benefits
- **Maintainability**: Easier to add new protocols by just implementing a new model.
- **Testability**: Decoupled services and models are much easier to unit test.
- **Robustness**: Better error handling and type safety throughout the data flow.
- **Performance**: Reduced locking overhead and better state management.

## Implementation Plan
0.  **Runtime Upgrade**: Update `Dockerfile` and CI/CD to Python 3.14. Verify compatibility of core dependencies (`aiohttp`, `pydantic`, `fastapi`).
1.  **Refactor Models**: Create `src/models/protocol.py` (or expand `server.py`).
2.  **Update Services**: Progressively migrate logic from `SubscriptionService` to new services.
3.  **Unify Config**: Create a new unified config loader.
4.  **Update API**: Refactor `main.py` and routes.
5.  **Finalize & Test**: Update the test suite and verify functionality.
