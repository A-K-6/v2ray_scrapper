# Idea: YAML Configuration for Site Partitioning (Completed)

## Status
**Implemented** on 2026-01-30.

## Context
Currently, the application uses `.env` variables (`PRECHECK_SITES`) to define which sites to test. The filename logic is hardcoded (replacing dots with underscores).

The user is running multiple worker instances and wants to avoid Git merge conflicts by partitioning the workload (Worker A handles Site A, Worker B handles Site B). The current "worker-per-branch" strategy causes data fragmentation.

## Proposal
Introduce a `config.yaml` file to define "Business Logic" (sites to test, output filenames) separately from "Infrastructure Secrets" (.env). This allows multiple workers to push to the **same branch** safely by writing to **different files**.

## Proposed YAML Structure
```yaml
worker:
  # Unique identifier for this worker (optional, for logging)
  id: "worker-01"

git:
  branch: "main"  # All workers can now share 'main' safely

# Define the tasks for this specific worker instance
sites:
  - url: "https://www.google.com"
    filename: "google_valid.txt"
    # Future feature: specific requirements (e.g., must be US IP)
    # requirements:
    #   country: "US"

  - url: "https://www.netflix.com"
    filename: "netflix_valid.txt"
```

## Implementation Steps

1.  **Dependencies:** Add `PyYAML` to `requirements.txt`. (Done)
2.  **Configuration Model:**
    -   Create `src/core/yaml_config.py`. (Done)
    -   Define Pydantic models for the YAML structure (for validation). (Done)
    -   Load `config.yaml` at startup. (Done)
3.  **Refactor `SubscriptionService`:**
    -   Remove `settings.PRECHECK_SITES` logic. (Done - kept as fallback)
    -   Iterate over the loaded YAML `sites` list. (Done)
    -   Pass the specific `filename` to the `GitUploader`. (Done)
4.  **Refactor `GitUploader`:**
    -   Ensure `pull --rebase` is robust to handle updates from other workers on different files. (Done)

## Benefits
-   **Scalability:** You can spin up 10 workers, each with a different `config.yaml` handling a different set of sites.
-   **Unified Data:** All results end up in a single Git branch/repo.
-   **Flexibility:** Precise control over output filenames (e.g., `google.txt` instead of `www_google_com.txt`).