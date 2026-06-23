# V2Ray Scrapper & Tester

> **Automated V2Ray Server Aggregator, Tester, and Distributor**

This tool actively scrapes V2Ray subscription links, validates them, tests their real-world latency using the **Xray core**, and exposes the working servers via a REST API. It ensures you always have a fresh list of high-speed, functional proxies.

## 🚀 Features

-   **Multi-Protocol Support:** Parses VLESS, VMess, Trojan, and Shadowsocks.
-   **Real-World Testing:** Uses the actual `xray` binary to establish connections and measure "Real Delay" (not just Ping).
-   **YAML-based Site Partitioning:** Assign specific target sites to different worker instances to avoid Git conflicts and data fragmentation.
-   **Structured Logging:** Professional logging using `loguru` for better observability and debugging.
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

### Environment Variables (.env)
Configuration is managed via environment variables (or the `.env` file).

| Variable | Description | Default |
| :--- | :--- | :--- |
| `SUB_URLS` | Comma-separated list of subscription URLs to scrape. | *Default Internal List* |
| `CACHE_INTERVAL_SECONDS` | How often (in seconds) to re-test servers. | `900` (15 min) |
| `MAX_DELAY_MS` | Max latency to consider a server "working". | `8000` |
| `TEST_TIMEOUT` | Timeout for each connection test (seconds). | `10` |
| `LOW_INTERNET_CONS` | Limit number of servers tested to save bandwidth. | `False` |
| `GITHUB_PUSH_ENABLED` | Enable pushing results to a Git repo. | `False` |
| `GITHUB_MAIN_PUSH_ENABLED` | Enable pushing the main `subscription.txt` export. | `True` |
| `GITHUB_SITE_PUSH_ENABLED` | Enable site-specific checks and pushes like `google_tci.txt`. | `True` |
| `GITHUB_TOKEN` | GitHub Personal Access Token (if enabled). | - |
| `GITHUB_REPO_URL` | Target Git repository URL. | - |

### Advanced Site Configuration (config.yaml)
For complex setups with multiple workers, use a `config.yaml` file in the root directory. This allows you to define exactly which sites each worker instance should check and what the output filenames should be.

```yaml
# config.yaml
git:
  branch: "main"  # Multiple workers can safely share a branch

sites:
  - url: "https://www.google.com"
    filename: "google_valid.txt"
    enabled: true
  
  - url: "https://www.netflix.com"
    filename: "netflix_valid.txt"
    enabled: true
```
*Note: Site configurations in `config.yaml` override the `PRECHECK_SITES` environment variable.*

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
| `/subscription/test` | `POST` | Test raw or Base64 subscription text and return working servers. |

**Example Usage:**
```bash
# Get a working subscription link
curl http://localhost:8084/cache/base64

# Test a local subscription file without changing the main cache
curl -X POST http://localhost:8084/subscription/test \
  -H "Content-Type: text/plain" \
  --data-binary @subscription.txt
```

## 📱 Android Client App (V2Ray Updater)

A native Android application (`android-client`) built with Kotlin and Jetpack Compose. It allows you to download and sync your scraper's live proxies directly on your mobile device.

### Features
- **Fetch & View List:** Enter your scraper API endpoint (e.g., `http://<your-vps-ip>:8084/cache/base64`) or GitHub raw subscription URL. The app decodes and displays VMess, VLESS, Trojan, and Shadowsocks nodes in a modern, dark-themed UI.
- **Local Latency Testing:** Run concurrent TCP connection checks (TCP Ping) directly from your phone to test real-time node availability on your mobile network.
- **Clipboard Sync:** One-click copy for individual proxy configs or the entire subscription payload.
- **Quick Client Launch:** Deep integration shortcuts to quickly open common Android V2Ray clients like `v2rayNG`, `Nekobox`, and `Sing-Box` to import configs.

### 📦 Automated GitHub Releases
The project features a CI/CD workflow that automatically compiles the Android client and attaches the installer files to GitHub Releases!

To build and publish:
1. Push a version tag to your GitHub repository:
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```
2. GitHub Actions will trigger, compiling two APKs:
   - **`app-debug.apk` (Recommended):** Pre-signed with a debug key. Ready to download, install, and run immediately on your Android device.
   - **`app-release-unsigned.apk`:** Unsigned release build suitable for signing with custom keys.
3. Download the generated APKs from the **Releases** page of your GitHub repository.

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
