---
id: "01-backend-desktop-launch-contract"
title: "Backend desktop launch contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/desktop-tauri-app/spec.md"
---

# Task 01: Backend Desktop Launch Contract

## Acceptance

- The backend honors `server.host` / `KANDEV_SERVER_HOST` when binding the HTTP listener.
- Desktop/headless launch can force `127.0.0.1` without changing normal CLI/service defaults.
- `kandev --headless --port <port>` remains a stable process contract for a parent desktop app to supervise.

## Verification

```bash
rtk bash -lc 'cd apps/backend && go test -tags fts5 ./internal/backendapp ./internal/launcher ./internal/common/config'
```

## Files Likely Touched

- `apps/backend/internal/common/config/config.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/helpers_test.go`
- `apps/backend/internal/launcher/env.go`
- `apps/backend/internal/launcher/start.go`
- `apps/backend/internal/launcher/start_test.go`
- `apps/backend/internal/launcher/run_test.go`

## Dependencies

None.

## Inputs

- Spec sections: API surface > Desktop launch contract; API surface > Backend bind contract; Failure modes.
- Existing patterns: native launcher in `apps/backend/internal/launcher/`, backend HTTP server setup in `apps/backend/internal/backendapp/main.go`.
- ADR: `docs/decisions/0026-tauri-desktop-shell.md`.

## Output Contract

When complete, update this file's `status` to `done`, update the Wave 1 checkbox in `plan.md`, and report changed files, tests run, blockers, and residual risks.
