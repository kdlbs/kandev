---
name: debug-instance
description: Debug a running kandev development instance. Use when the user reports a bug, unexpected behavior, or asks to investigate an issue while kandev is running via `make dev`. Fetches backend logs, analyzes errors, and optionally inspects the UI via an existing Playwright browser session.
allowed-tools: Bash(curl:*) Bash(jq:*) Bash(playwright-cli:*)
---

# Debug Running Kandev Instance

Diagnose issues in a running kandev development instance by fetching structured logs and optionally inspecting the UI via an existing Playwright browser session.

## Immediately: Create Task Pipeline

As soon as this skill is invoked, create a task list to track progress:

1. **Detect running instance** — probe health endpoint, determine ports
2. **Fetch & analyze backend logs** — get debug export, summarize errors
3. **Browser inspection** (if UI-related) — check for existing playwright session, inspect DOM/console
4. **Diagnose & report** — correlate findings, present root cause hypothesis

Mark each task as `in_progress` when starting and `completed` when done. Skip task 3 if the issue is purely backend.

---

## Phase 1 — Detect Running Instance

1. Probe the default backend port:
   ```bash
   curl -sf http://localhost:38429/api/v1/system/health
   ```
2. If that fails, try ports 38430–38435 (kandev auto-assigns nearby ports if default is busy).
3. If still not found, ask the user which port kandev is running on.
4. Derive the web port: typically `backend_port - 1000` (e.g., 38429 → 37429).

## Phase 2 — Fetch & Analyze Logs

**IMPORTANT**: The debug export returns raw JSON. If output appears schema-converted (types instead of values), the response is being compressed by a proxy tool — use `curl` directly or check tool configuration.

1. Fetch the debug export:
   ```bash
   curl -s http://localhost:<backend_port>/api/v1/system/debug/export | jq .
   ```
2. Summarize findings:
   - **Metadata**: uptime, version, goroutine count, memory usage
   - **Errors**: count of error-level entries, most recent errors with stack traces
   - **Warnings**: count and notable patterns
3. Filter by level if needed:
   ```bash
   curl -s "http://localhost:<backend_port>/api/v1/system/debug/export?level=error" | jq .logs
   ```
4. Correlate errors with the user's reported issue.

## Phase 3 — Browser Interaction (conditional)

Only enter this phase if:
- The issue is UI-related
- The user explicitly asks to inspect the browser
- Log analysis alone is insufficient

### Check for existing Playwright session

```bash
playwright-cli list
```

- **If a session exists**: use it directly. Do NOT open a new browser.
- **If no session exists** and browser inspection is needed:
  ```bash
  playwright-cli open http://localhost:<web_port> --headed
  ```

### Browser debugging commands

```bash
# Page state
playwright-cli snapshot

# Console logs (frontend errors)
playwright-cli console
playwright-cli console error

# Network activity
playwright-cli network

# Execute JS to extract frontend log buffer
playwright-cli eval "JSON.stringify(window.__kandevLogBuffer?.snapshot?.() ?? [])"

# Navigate to a specific page
playwright-cli goto http://localhost:<web_port>/some/path

# Inspect specific elements
playwright-cli snapshot "#main"
playwright-cli snapshot --depth=4
```

### Correlate frontend + backend

After gathering browser console errors or frontend log buffer entries, cross-reference with backend error timestamps to identify the full request lifecycle.

## Phase 4 — Diagnose & Report

1. Present a structured summary:
   - **Issue**: what was reported
   - **Evidence**: relevant log entries, console errors, DOM state
   - **Root cause hypothesis**: most likely cause based on evidence
   - **Suggested fix**: what code to change (with file paths)
2. If the fix is clear, offer to implement it (delegate to `/fix` or `/tdd`).

---

## Rules

- **Always probe health first** — never assume ports.
- **Always check `playwright-cli list` before opening a browser** — reuse existing sessions.
- **Present log summary before browser inspection** — logs are faster and often sufficient.
- **Don't leave browsers open** — if you opened one, close it when done (unless user wants to keep it).
- **Filter aggressively** — 2000 log entries is a lot; use `?level=error` or grep for keywords.
- **Create the task pipeline immediately** — track progress from the start.
