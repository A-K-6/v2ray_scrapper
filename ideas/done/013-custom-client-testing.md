# Idea: Custom Client Testing and Settings Management

## Status
**Implemented** on 2026-06-24.

## Context
Currently, the Android Client has hardcoded preset urls and only allows local TCP latency tests. The Core API does not support dynamic server-side proxy testing against custom targets or custom subscription links with server-side Xray execution.

## Proposal
Add a custom testing endpoint in the Core FastAPI service and a settings dashboard in the Jetpack Compose Android client to allow users to specify custom target URLs, custom subscription URLs, latency limits, and maximum config limits, triggering high-performance Xray-based verification directly on the Core.

## Implementation Steps

### 1. Core (Backend)
- Add a new endpoint `POST /subscription/test-custom` in [main.py](file:///home/aeen/Aeen/Code/PProjects/v2ray_scrapper/src/main.py).
- Enhance `ScraperService` in [scraper_service.py](file:///home/aeen/Aeen/Code/PProjects/v2ray_scrapper/src/service/scraper_service.py) to support dynamic multi-URL scraping.
- Refactor `TesterService` in [tester_service.py](file:///home/aeen/Aeen/Code/PProjects/v2ray_scrapper/src/service/tester_service.py) and `XrayService` in [xray_service.py](file:///home/aeen/Aeen/Code/PProjects/v2ray_scrapper/src/service/xray_service.py) to support dynamic latency test URLs and dynamic timeout limits per request.

### 2. Android Client App
- Refactor [MainActivity.kt](file:///home/aeen/Aeen/Code/PProjects/v2ray_scrapper/android-client/app/src/main/java/com/example/v2rayupdater/MainActivity.kt) with a dynamic dashboard interface (using Compose Tabs/Navigation):
  - **Nodes View:** Shows the current list of active tested nodes, with details, copy, and quick launch client options.
  - **Subscriptions View:** Management UI to add, enable/disable, or delete custom subscription URLs.
  - **Sites View:** Toggle or add target test websites (Google, YouTube, etc.) to evaluate proxies against.
  - **Settings View:** Manage configuration limits (Max Configs, Max Latency Timeout).
- Implement API call logic targeting `/subscription/test-custom` on the Core.

## Benefits
- **Flexibility:** Allows testing custom lists of subscriptions against specific sites.
- **Accurate Server-side Filtering:** Core does full protocol/xray testing, preventing raw TCP checks from showing "working" but censored configs.
- **Improved UX:** Gives users complete control over what is parsed and tested.
