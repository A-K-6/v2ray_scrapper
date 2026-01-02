# V2Ray Scrapper & Tester

## Project Overview

The **V2Ray Scrapper & Tester** is a robust, automated service designed to aggregate, validate, and distribute high-performance V2Ray server configurations. It addresses the issue of unreliable public proxy lists by actively scraping subscription links and performing real-world latency tests using the **Xray core**. This ensures that only functional, high-speed servers are delivered to end-users or downstream applications.

### Key Features
*   **Multi-Protocol Parsing:** Supports VLESS, VMess, Trojan, and Shadowsocks.
*   **Real-Latency Testing:** Uses the actual `xray` binary to establish connections and measure real delay (HTTP HEAD requests), providing accuracy superior to simple ICMP pings.
*   **Automated Health Checks:** Continuously monitors server health in the background, removing dead nodes.
*   **Site-Specific Validation:** Verifies if servers can access specific targets (e.g., Google, YouTube).
*   **Distribution:** Exposes results via a REST API (JSON, Base64, Raw) and supports automated pushing to GitHub/GitLab.
*   **Containerized:** Built for easy deployment with Docker.

### Technical Architecture
*   **Language:** Python 3.11+
*   **Framework:** FastAPI (ASGI)
*   **Core Engine:** Project Xray (Golang binary)
*   **Concurrency:** `asyncio`, `aiohttp` for efficient batch processing.
*   **Validation:** Pydantic for data validation and settings management.

## Building and Running

### Using Docker (Recommended)

The project is optimized for Docker Compose.

1.  **Configure Environment:**
    Copy the sample environment file and configure your settings (especially `SUB_URLS`).
    ```bash
    cp .env.sample .env
    ```

2.  **Start the Service:**
    ```bash
    docker compose up -d
    ```

The API will be accessible at `http://localhost:8084`.

### Manual Local Setup (Development)

1.  **Prerequisites:**
    *   Python 3.11+
    *   [Xray Core](https://github.com/XTLS/Xray-core) installed and available in your system PATH (or specified in `XRAY_PATH`).

2.  **Install Dependencies:**
    ```bash
    python3 -m venv .venv
    source .venv/bin/activate
    pip install -r requirements.txt
    ```

3.  **Run Application:**
    ```bash
    # Ensure XRAY_PATH is set correctly in .env or environment
    uvicorn src.main:app --host 0.0.0.0 --port 8084 --reload
    ```

## Configuration

Configuration is handled via environment variables, loaded through Pydantic `BaseSettings`. See `.env.sample` and `src/core/config.py` for all options.

**Key Variables:**

*   `SUB_URLS`: Comma-separated list of subscription URLs to scrape.
*   `CACHE_INTERVAL_SECONDS`: Refresh interval for server lists (default: 900s).
*   `MAX_DELAY_MS`: Maximum latency to consider a server "working" (default: 8000ms).
*   `TEST_TIMEOUT`: Timeout for individual connection tests (default: 10s).
*   `XRAY_PATH`: Path to the Xray binary (default: `/usr/local/bin/xray`).
*   `GITHUB_PUSH_ENABLED`: Set to `True` to enable Git integration.

## Development Conventions

*   **Code Structure:**
    *   `src/main.py`: Entry point and API definition.
    *   `src/core/`: Configuration and core utilities.
    *   `src/models/`: Pydantic data models (e.g., `Server`, `ServerResponse`).
    *   `src/service/`: Business logic (`SubscriptionService`, `XrayService`, `GitUploader`).
*   **Asynchronous Programming:** Heavy use of `asyncio` for non-blocking I/O during scraping and testing. Ensure any new I/O operations are async.
*   **Type Hinting:** Strictly use Python type hints for better code quality and IDE support.
*   **Linting/Formatting:** Follow standard Python PEP 8 guidelines.

## API Endpoints

*   `GET /health`: Service health check.
*   `GET /servers/live`: Trigger immediate test and return top results.
*   `GET /cache`: Retrieve currently cached top 25 servers.
*   `GET /cache/base64`: Get cached servers in standard Base64 subscription format.
*   `GET /subscription/site-specific?url=...`: Get servers that work for a specific URL.
