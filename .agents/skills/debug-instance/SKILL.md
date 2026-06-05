---
name: debug-instance
description: Debug a running kandev development instance. Use when the user reports a bug, unexpected behavior, or asks to investigate an issue while kandev is running via `make dev`. Triages the bug class first (backend-logic → Go test, live-instance → /debug/export, UI → browser), launches an ISOLATED parallel instance when a running app is needed, and tears down only what it started.
allowed-tools: Bash(curl:*) Bash(jq:*) Bash(npx:*) Bash(scripts/kandev-instances:*) Bash(scripts/kandev-logs:*) Bash(scripts/dev-isolated:*) Bash(scripts/kandev-kill:*) Bash(go:*)
---

# Debug Running Kandev Instance

Diagnose issues efficiently and safely. The two cardinal rules:

1. **Triage the bug class BEFORE launching anything.** A browser is the slowest,
   least faithful tool — reach for it last.
2. **Never touch the user's live instance.** Launch your own isolated instance,
   and tear down only what you started. Never `pkill kandev`.

## Immediately: Create Task Pipeline

As soon as this skill is invoked, create a task list:

1. **Triage** — classify the bug (decision gate below) and pick the cheapest faithful path
2. **Reproduce / gather evidence** — Go test, `/debug/export`, or browser (whichever the gate selects)
3. **Diagnose & report** — correlate findings, present root cause + suggested fix

Mark each `in_progress` when starting, `completed` when done.

---

## Phase 0 — Decision Gate (do this first)

Classify the bug, then take the matching path. Most bugs are class A.

### A) Backend-logic bug → targeted Go test (NO UI)

Validation, dedup, data shaping, error mapping, identifier assignment, workflow
routing — anything reproducible from inputs. Reproduce with a Go test against the
**real service path**. This is dramatically faster and more faithful than a browser.

The office/task service has a ready-made harness (`setupOfficeTest` builds a real
`Service` + SQLite repo). Write a throwaway `*_test.go` next to it that drives the
real call and asserts the buggy behavior:

```go
func TestRepro_DuplicateTitleRejected(t *testing.T) {
    svc, repo := setupOfficeTest(t)
    ctx := context.Background()
    _ = repo

    // Drive the real service method the bug lives in.
    task, err := svc.CreateTask(ctx, &CreateTaskRequest{
        WorkspaceID: "ws-1",
        Title:       "Office Task",
        ProjectID:   "proj-1",
    })
    if err != nil {
        t.Fatalf("CreateTask: %v", err)
    }
    // Assert the behavior you expect — failure here IS the reproduction.
    if task.Identifier == "" {
        t.Fatalf("expected identifier to be assigned, got empty")
    }
}
```

Run just that test:

```bash
cd apps/backend && go test -tags fts5 -run TestRepro ./internal/task/service/ -v
```

Once you understand the root cause, hand off to `/fix` or `/tdd` to turn the repro
into a permanent regression test. **Do not launch the UI for class-A bugs.**

### B) Live-instance bug → fetch logs, no relaunch

Something is *already misbehaving in the user's running instance* and you need its
state/logs. Don't relaunch anything — read the user's instance's debug export
(read-only). Jump to **Phase 2**.

### C) UI / interaction bug that needs a browser

A rendering, layout, focus, WS-driven, or click-flow bug you genuinely can't
reproduce from inputs. Only now launch an isolated instance (**Phase 1**) and drive
a browser (**Phase 3**). Never drive a browser against the user's live instance —
it mutates their data.

---

## Phase 1 — Identify instances & launch your own (isolated)

### List what's running

```bash
scripts/kandev-instances
```

Columns: `PID  BACKEND_PORT  WEB_PORT  AGENTCTL_PORT  HOME_DIR  REPO_PATH`.

The **user's** live instance is the one with `HOME_DIR=/home/<user>` (or a repo path
under the user's home, typically backend port **38429**). **Never act on it.**

### Launch an isolated parallel instance

`dev-isolated` auto-picks non-colliding ports (never 38429/37429/39429), creates a
throwaway `HOME` + fresh SQLite DB, builds the backend binaries a live instance
actually needs (kandev + agentctl + mock-agent) if stale, waits for health, and
prints the URLs, log paths, and the exact teardown command. On a **clean checkout**,
pass `--install` (or run `make install` once) so `node_modules` + deps are present —
the `--web` frontend needs them.

```bash
# Backend only (enough for class-A/B work and API probing):
scripts/dev-isolated

# Also start the Next.js frontend (only for class-C browser work):
scripts/dev-isolated --web
```

It prints a `READY` block with the backend URL (e.g. `http://localhost:48429`), the
pidfile, and:

```text
Teardown : scripts/kandev-kill --pidfile /tmp/kandev-dev-isolated-<port>.pid --yes
```

Note your instance's port — every command below uses it.

---

## Phase 2 — Fetch & analyze logs

Use `kandev-logs` against the relevant port — **your** isolated port for class-C,
or the **user's** port (read-only) for class-B.

```bash
# Full structured export (logs + runtime metadata), pretty-printed:
scripts/kandev-logs <port> --export

# Error-level only:
scripts/kandev-logs <port> --export --level error

# Tail the backend stderr log of an isolated instance you launched:
scripts/kandev-logs <port> --tail --lines 120
```

> The export endpoint (`/api/v1/system/debug/export`) returns raw JSON. If output
> looks schema-converted (types instead of values), a proxy is compressing it — the
> script uses plain `curl`, so prefer the script over an MCP fetch tool.

Summarize: uptime/version/goroutines/memory (metadata), error count + recent errors
with stack traces, notable warning patterns. **Filter aggressively** — hundreds of
entries is normal; lead with `--level error`. Correlate with the reported issue.

---

## Phase 3 — Browser interaction (class C only)

Drive the browser with **`npx playwright-cli`** (there is no bare `playwright-cli`
binary in this repo — `playwright-cli ...` alone fails). Confirm availability once:

```bash
npx --no-install playwright-cli --version
# Discover commands if unsure:
npx playwright-cli --help
```

### Reuse an existing session

```bash
npx playwright-cli list
```

- **If a session exists**, reuse it — do NOT open a new browser.
- **If none exists**, open one against YOUR isolated web port (never the user's):

```bash
npx playwright-cli open http://localhost:<your_web_port>
```

### Common debugging commands

```bash
# Page state (accessibility snapshot with refs)
npx playwright-cli snapshot
npx playwright-cli snapshot "#main"
npx playwright-cli snapshot --depth=4

# Frontend console errors
npx playwright-cli console
npx playwright-cli console error

# Network activity
npx playwright-cli network

# Dump the frontend log buffer
npx playwright-cli eval "JSON.stringify(window.__kandevLogBuffer?.snapshot?.() ?? [])"

# Navigate to a specific page
npx playwright-cli goto http://localhost:<your_web_port>/some/path
```

### Correlate frontend + backend

Cross-reference browser console / frontend-log-buffer timestamps with backend error
timestamps (`scripts/kandev-logs <your_port> --export --level error`) to trace the
full request lifecycle.

---

## Phase 4 — Diagnose, report & TEAR DOWN

1. Present a structured summary:
   - **Issue**: what was reported
   - **Evidence**: log entries, console errors, test failures, DOM state
   - **Root cause hypothesis**: most likely cause from the evidence
   - **Suggested fix**: file paths + change; offer to delegate to `/fix` or `/tdd`
2. **Tear down ONLY your instance** using the command `dev-isolated` printed:

```bash
scripts/kandev-kill --pidfile /tmp/kandev-dev-isolated-<your_port>.pid --yes
# or, by port:
scripts/kandev-kill <your_port> --yes
```

   `kandev-kill` refuses port 38429 without `--force`, prints exactly what it will
   kill, and cascades to the agentctl child + spawned agents. Remove any throwaway
   `*_test.go` repro file you created.
3. If you opened a browser, close it (`npx playwright-cli close`) unless the user
   wants it left open.

---

## Rules

- **Triage first** (Phase 0). Class-A bugs get a Go test, not a browser.
- **Never touch the user's live instance** — identify it via `scripts/kandev-instances`,
  launch your own with `scripts/dev-isolated`, and only ever `scripts/kandev-kill`
  the port YOU launched. **Never `pkill kandev`. Never kill a port you didn't start.**
- **Logs before browser** — `scripts/kandev-logs` is faster and often sufficient.
- **Filter aggressively** — start with `--level error`.
- **No orphans** — always tear down the instance you launched and delete throwaway repro files.
- **`npx playwright-cli`, never bare `playwright-cli`.**
- **Create the task pipeline immediately.**
