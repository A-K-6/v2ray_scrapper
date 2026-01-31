# Idea: Web Dashboard (Frontend)

## Context
The current interface is purely API-based (JSON/Text). Users cannot easily visualize server health, distribution, or manually control the service without using CLI tools or raw API calls.

## Proposal
Add a lightweight web frontend served by FastAPI.

## Implementation Steps
1.  **Tech Stack:** Simple HTML/JS (Vue.js or Alpine.js) embedded in `src/templates`.
2.  **Endpoints:**
    -   `GET /`: Renders the dashboard.
3.  **Features:**
    -   **Live Stats:** Number of active servers, last update time.
    -   **World Map:** Visualization of server locations (using GeoIP data).
    -   **Table:** List of top servers with "Copy Link" buttons.
    -   **Controls:** "Update Now" button to trigger immediate testing.
4.  **Backend:** Use `Jinja2Templates` in FastAPI to serve the HTML.

## Benefits
-   **UX:** Much friendlier for end-users.
-   **Transparency:** Visual confirmation that the system is working.
-   **Ease of Use:** "Click to Copy" functionality.
