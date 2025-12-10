# Product Requirement Document (PRD)
## High-Speed V2Ray Server Scrapper & Tester

| Metadata | Details |
| :--- | :--- |
| **Project Name** | V2Ray Scrapper & Tester |
| **Version** | 1.0 |
| **Status** | Active / Maintenance |
| **Last Updated** | 2025-12-02 |
| **Owner** | Engineering Team |

---

## 1. Introduction

### 1.1 Purpose
The **V2Ray Scrapper & Tester** is an automated service designed to aggregate, validate, and distribute high-performance V2Ray server configurations. It solves the problem of unreliable public proxy lists by actively testing servers against real-world endpoints using the Xray core, ensuring that only functional and fast servers are delivered to the end-user.

### 1.2 Target Audience
*   **Privacy Advocates:** Users seeking reliable tools to bypass censorship.
*   **Network Administrators:** Teams needing to monitor or aggregate proxy performance.
*   **Automated Systems:** Downstream applications requiring a dynamic, high-quality list of active proxies.

### 1.3 Scope
The system handles the entire lifecycle of a proxy configuration:
1.  **Ingestion:** Fetching subscription links (VLESS, VMess, Trojan, Shadowsocks).
2.  **Validation:** Parsing and deduplicating server URIs.
3.  **Testing:** Performing real-latency tests using the `xray` binary (not just ICMP ping).
4.  **Distribution:** Exposing results via a REST API (JSON, Base64, Raw) and optionally pushing to a Git repository.

---

## 2. Product Overview

### 2.1 Problem Statement
Public V2Ray subscription links often contain hundreds of servers, but a significant percentage are dead, slow, or incompatible with specific websites. Users waste time manually selecting servers until they find one that works.

### 2.2 Solution
A self-hosted containerized application that runs in the background, continuously polishing the server list. It provides a "set and forget" mechanism where the user simply points their V2Ray client to this application's endpoint to get a guaranteed working list.

### 2.3 Key Value Propositions
*   **Real-World Accuracy:** Uses the actual Xray core for testing, ensuring protocol compatibility.
*   **Site-Specific Optimization:** Can test if servers work for specific blocked domains (e.g., Google, YouTube).
*   **Bandwidth Efficiency:** Includes a "Low Internet Consumption" mode to limit testing overhead.
*   **Persistence:** Automated Git integration allows the verified list to be hosted on GitHub/GitLab for easy external access.

---

## 3. Functional Requirements

### 3.1 Subscription Management
*   **FR-01:** The system MUST accept multiple source URLs via environment variables (`SUB_URLS`).
*   **FR-02:** The system MUST support parsing of the following protocols:
    *   VLESS (standard and Reality security)
    *   VMess (standard and authenticated)
    *   Trojan
    *   Shadowsocks
*   **FR-03:** The system MUST filter out duplicate servers based on their raw URI.

### 3.2 Server Testing Engine
*   **FR-04:** The system MUST use the `xray` binary to establish actual proxy connections for testing.
*   **FR-05:** The system MUST measure the "Real Delay" (time to establish connection + HTTP HEAD request) to a target URL (default: `http://www.google.com/generate_204`).
*   **FR-06:** The system MUST enforce a configurable timeout (default: 10s) per test.
*   **FR-07:** The system MUST support batch testing to manage system resource usage (CPU/RAM).

### 3.3 Caching & Scheduling
*   **FR-08:** The system MUST maintain an in-memory cache of the "Top 25" and "All" working servers.
*   **FR-09:** The cache MUST auto-refresh on a configurable interval (default: 15 minutes).
*   **FR-10:** The system MUST support "Site-Specific Caching," storing results of servers that work for specific target URLs (e.g., `google.com`) for a longer TTL (default: 1 hour).

### 3.4 API Interface
The system MUST expose a RESTful API with the following endpoints:

| Endpoint | Method | Description |
| :--- | :--- | :--- |
| `/health` | GET | Service health check. |
| `/servers/live` | GET | Triggers an immediate test and returns top 25 servers (JSON). |
| `/cache` | GET | Returns currently cached top 25 servers (JSON). |
| `/cache/raw` | GET | Returns cached servers as a newline-separated list of URIs. |
| `/cache/base64` | GET | Returns cached servers as a Base64 encoded string (standard subscription format). |
| `/cache/all/base64` | GET | Returns ALL working servers (not just top 25) as Base64. |
| `/subscription/site-specific` | GET | Query param `url`. Returns Base64 subscription of servers capable of accessing the specific URL. |

### 3.5 External Integration (Git)
*   **FR-11:** If enabled, the system MUST automatically push the updated server list to a specified Git repository.
*   **FR-12:** The system MUST support site-specific file generation (e.g., `www_google_com.txt`) in the Git repo.
*   **FR-13:** The system MUST handle authentication via Personal Access Tokens (PAT).

---

## 4. Non-Functional Requirements

### 4.1 Performance
*   **NFR-01:** The system should handle concurrent testing of 500+ servers without crashing (batch processing required).
*   **NFR-02:** API response time for cached endpoints should be under 50ms.

### 4.2 Reliability
*   **NFR-03:** The system must gracefully handle network timeouts and upstream API failures (404/500 from subscription providers).
*   **NFR-04:** The `xray` subprocess must be properly terminated/killed after timeouts to prevent zombie processes.

### 4.3 Deployment & Configuration
*   **NFR-05:** The application MUST be containerized (Docker).
*   **NFR-06:** All configuration MUST be managed via Environment Variables (following 12-Factor App methodology).

---

## 5. Technical Architecture

### 5.1 Tech Stack
*   **Language:** Python 3.11+
*   **Web Framework:** FastAPI (ASGI)
*   **Concurrency:** `asyncio`, `aiohttp`
*   **Proxy Core:** Project Xray (Golang binary)
*   **Validation:** Pydantic
*   **Settings:** Pydantic-Settings

### 5.2 Data Flow
1.  **Startup:** `SubscriptionService` initializes and starts the background loop.
2.  **Fetch:** `aiohttp` pulls data from `SUB_URLS`.
3.  **Parse:** `ProxyParser` converts raw base64/text into `Server` models.
4.  **Test:** `XrayService` generates a temporary JSON config, spins up `xray`, and attempts `aiohttp` requests through the local SOCKS5 port.
5.  **Cache:** Successful results are sorted by latency and stored in memory.
6.  **Serve:** API endpoints read from memory and return formatted responses.

---

## 6. Configuration Parameters (Environment Variables)

| Variable | Default | Description |
| :--- | :--- | :--- |
| `SUB_URLS` | (Default Internal URL) | Comma-separated list of subscription sources. |
| `XRAY_PATH` | `/usr/local/bin/xray` | Path to the Xray executable. |
| `CACHE_INTERVAL_SECONDS` | `900` | How often to refresh the server list. |
| `MAX_DELAY_MS` | `8000` | Maximum allowed latency to consider a server "working". |
| `GITHUB_PUSH_ENABLED` | `false` | Enable/Disable Git integration. |
| `GITHUB_REPO_URL` | - | Target repository HTTPS URL. |
| `GITHUB_TOKEN` | - | GitHub PAT for authentication. |
| `LOW_INTERNET_CONS` | `false` | If true, limits the number of servers tested to save bandwidth. |

---

## 7. Roadmap / Future Improvements
*   **Database Persistence:** Switch from in-memory caching to Redis or SQLite to persist server stats across restarts.
*   **Geo-IP Filtering:** Integrate `maxmind` or similar to allow users to request servers by country (e.g., `/servers?country=US`).
*   **Protocol Converter:** Functionality to convert between protocols (e.g., VLESS -> Clash YAML).
*   **Web Dashboard:** A simple React/HTML frontend to visualize server health and manually trigger updates.
