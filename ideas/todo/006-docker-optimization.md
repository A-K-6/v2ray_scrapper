# Idea: Docker Optimization

## Context
The current `Dockerfile` installs `git`, `wget`, `unzip` and does not clean up `apt` caches. It also downloads the "latest" Xray binary, which makes builds non-deterministic (reproducibility issue).

## Proposal
Optimize the Dockerfile for size, security, and reproducibility.

## Implementation Steps
1.  **Pin Versions:**
    -   Define `ARG XRAY_VERSION=1.8.6` (or current stable).
    -   Download specific version URL.
2.  **Multi-Stage Build:**
    -   **Builder Stage:** Install tools, download Xray, install Python deps into a venv.
    -   **Runner Stage:** Copy venv and Xray binary. minimal python-slim image.
3.  **Cleanup:**
    -   Combine `apt-get install` and `rm -rf /var/lib/apt/lists/*` in the same RUN instruction to keep layers small.
4.  **Non-Root User:**
    -   Create a user `appuser` and run the application as non-root for security.

## Benefits
-   **Size:** Smaller image (faster pull/deploy).
-   **Security:** Reduced attack surface (non-root).
-   **Stability:** Deterministic builds (same Xray version every time).
