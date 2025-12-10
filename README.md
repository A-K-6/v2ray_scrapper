# V2Ray Scrapper & Tester

> **Automated V2Ray Server Aggregator, Tester, and Distributor**

This tool actively scrapes V2Ray subscription links, validates them, tests their real-world latency using the **Xray core**, and exposes the working servers via a REST API. It ensures you always have a fresh list of high-speed, functional proxies.

## 🚀 Features

-   **Multi-Protocol Support:** Parses VLESS, VMess, Trojan, and Shadowsocks.
-   **Real-World Testing:** Uses the actual `xray` binary to establish connections and measure "Real Delay" (not just Ping).
-   **Automated Health Checks:** continuously tests servers in the background and removes dead ones.
-   **Site-Specific Testing:** Verify if servers can access specific targets (e.g., Google, YouTube).
-   **Smart Caching:** In-memory caching for high performance and reduced load.
-   **Git Integration:** Automatically push working servers to a GitHub/GitLab repository.
-   **Dockerized:** Easy deployment with Docker Compose.

---

## 🛠 Prerequisites

-   **Docker** & **Docker Compose** (Recommended)
-   *OR* Python 3.11+ and [Xray Core](https://github.com/XTLS/Xray-core) installed locally.

---

## ⚡ Quick Start (Docker)

The easiest way to run the service is using Docker Compose.

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/yourusername/v2ray-scrapper.git
    cd v2ray-scrapper
    ```

2.  **Configure Environment:**
    Copy the sample environment file and edit it.
    ```bash
    cp .env.sample .env
    ```
    *Add your subscription URLs to `SUB_URLS` in `.env`.*

3.  **Run the Service:**
    ```bash
    docker compose up -d
    ```

The API will be available at `http://localhost:8084`.

---

## 🔧 Configuration

Configuration is managed via environment variables (or the `.env` file).

| Variable | Description | Default |
| :--- | :--- | :--- |
| `SUB_URLS` | Comma-separated list of subscription URLs to scrape. | *Default Internal List* |
| `CACHE_INTERVAL_SECONDS` | How often (in seconds) to re-test servers. | `900` (15 min) |
| `MAX_DELAY_MS` | Max latency to consider a server "working". | `8000` |
| `TEST_TIMEOUT` | Timeout for each connection test (seconds). | `10` |
| `LOW_INTERNET_CONS` | Limit number of servers tested to save bandwidth. | `False` |
| `GITHUB_PUSH_ENABLED` | Enable pushing results to a Git repo. | `False` |
| `GITHUB_TOKEN` | GitHub Personal Access Token (if enabled). | - |
| `GITHUB_REPO_URL` | Target Git repository URL. | - |

---

## 📡 API Endpoints

Once running, you can access the following endpoints:

| Endpoint | Method | Description |
| :--- | :--- | :--- |
| `/health` | `GET` | Service health check. |
| `/servers/live` | `GET` | Trigger immediate test and return top servers. |
| `/cache` | `GET` | Get currently cached top 25 servers (JSON). |
| `/cache/raw` | `GET` | Get cached servers as a text list (URI format). |
| `/cache/base64` | `GET` | Get cached servers as Base64 subscription string. |
| `/cache/all/base64` | `GET` | Get **ALL** working servers as Base64. |
| `/subscription/site-specific` | `GET` | Get servers that work for a specific URL (param: `url`). |

**Example Usage:**
```bash
# Get a working subscription link
curl http://localhost:8084/cache/base64
```

---

## 📦 Manual Installation (Dev)

If you prefer running without Docker:

1.  **Install Xray Core:**
    ```bash
    # Example for Linux
    bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install
    ```
    *Ensure `xray` is in your PATH or update `XRAY_PATH` in `.env`.*

2.  **Setup Python Environment:**
    ```bash
    python3 -m venv .venv
    source .venv/bin/activate
    pip install -r requirements.txt
    ```

3.  **Run the Application:**
    ```bash
    uvicorn src.main:app --host 0.0.0.0 --port 8084 --reload
    ```

---

## 🤝 Contributing

Pull requests are welcome. For major changes, please open an issue first to discuss what you would like to change.

## 📄 License

[MIT](LICENSE)
