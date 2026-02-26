# V2Ray Scrapper & Tester - Gemini Context

## Overview
The **V2Ray Scrapper & Tester** is an automated service for aggregating, validating, and distributing high-performance V2Ray server configurations using the Xray core.
- **Phase:** Active / Maintenance (Version 1.1).
- **Core Tech:** Python 3.11+, FastAPI, Xray Core, asyncio, aiohttp, Pydantic, Docker.
- **Key Features:** Multi-protocol parsing (VLESS, VMess, Trojan, SS), real-latency testing, Geo-IP enrichment, and automated Git distribution.

## Documentation Structure
- **Product Requirements:** `PRD.md` (Includes Roadmap & Specs)
- **Ideas (Todo):** `ideas/todo/`
- **Ideas (Done):** `ideas/done/`
- **Development Log:** `devlog.md`
- **Configuration Samples:** `.env.sample`, `config.yaml.sample`

---

## 🚀 Workflows

### 1. Idea Generation Workflow
Use this when exploring new features or improvements.
1.  **Request:** User asks for an idea or a search for an idea.
2.  **Research:** Search the codebase and documents (especially `PRD.md` and `ideas/`) to understand the context.
3.  **Refinement:** Propose a plan, apply critical thinking, and ask for user feedback.
4.  **Documentation:** Create a detailed plan in `ideas/todo/XXX-ideaname.md`.
5.  **Log:** Update `devlog.md`.

### 2. Implementation Workflow
Use this for executing changes or fixing issues.

#### Case A: Implementing an Idea
1.  **Review:** Read the idea file from `ideas/todo/` carefully.
2.  **Clarify:** Think through the logic and ask for any missing details.
3.  **Execute:** Implement the code, ensuring async-first I/O and strict type hinting.
4.  **Finalize:** Move the idea file from `ideas/todo/` to `ideas/done/`.
5.  **Document & Log:** Update `PRD.md` if necessary and update `devlog.md`.

#### Case B: Simple Task / Bug Fix
1.  **Analyze:** Read relevant code and `PRD.md` carefully.
2.  **Execute:** Implement the requested fix or change.
3.  **Document & Log:** Update `devlog.md`.

---

## Engineering Mandates
- **Single Source of Truth:** `PRD.md`.
- **Async First:** Heavy use of `asyncio` for all I/O operations (scraping, testing).
- **Type Safety:** Strict use of Python type hints and Pydantic models.
- **Code Style:** Adhere to PEP 8; ensure `xray` subprocesses are properly terminated.
- **DevLog:** Update `devlog.md` after every major workflow step or implementation.
