---
id: "05-run-dev"
title: "runDev orchestration"
status: done
wave: 2
depends_on: ["01-dev-paths", "02-db-backup", "03-web-child", "04-dev-command"]
plan: "plan.md"
spec: "../../specs/platform/requirements/go-dev-launcher.md"
---

# Task 05: `runDev` orchestration

Assemble tasks 01–04 into the working `kandev dev` command.

## Acceptance

- `runDev` performs, in order: repo-root discovery → dev port selection → dev backend env
  resolution → production-DB backup (abort on failure) → startup banner → supervisor and
  signal handlers → backend launch → health wait (600s dev timeout, dumping captured
  backend output on failure) → Vite launch → URL readiness wait → browser open →
  `waitForAppExit`. Every failure path shuts the supervisor down before returning.
- The backend child is `make -C apps/backend dev` with CWD at the repo root, launched
  through `launchRestartableBackend` so the restart control socket and launch manifest
  behave exactly as they do in `start`.
- `launchRestartableBackend` takes the supervisor home directory as an explicit
  parameter instead of calling `resolveHomeDir()` internally, so dev's supervisor state
  lands under `<repoRoot>/.kandev-dev/supervisor/` and not in the ambient
  `KANDEV_HOME_DIR` (which, inside a Kandev task workspace, is the parent backend's).
  `runStart` and `runInstalled` pass `resolveHomeDir()` and are behaviorally unchanged.
- `--headless` skips the browser open, as in `start`.

## Verification

Use TDD with the existing seam pattern in `start.go:12-21` (package-level function vars
swapped in tests) so `runDev` is exercised without spawning real processes. Then:

~~~bash
cd apps/backend && go test ./internal/launcher/ -race && go build ./... && golangci-lint run ./internal/launcher/... --timeout=5m
~~~

## Files

- `apps/backend/internal/launcher/dev.go` (new)
- `apps/backend/internal/launcher/dev_test.go` (new)
- `apps/backend/internal/launcher/supervisor.go`, `supervisor_test.go` — homeDir
  parameter
- `apps/backend/internal/launcher/start.go`, `start_test.go` — updated call site
- `apps/backend/internal/launcher/run.go` — updated call site
- `apps/backend/internal/launcher/launcher.go` — real `CommandDev` dispatch

## Inputs

- `apps/cli/src/dev.ts:26-110` — the orchestration order and the exact log lines
  (`[kandev] starting backend...`, `[kandev] backend ready at ...`,
  `[kandev] starting web...`, `[kandev] open: ...`). Keep them; people grep for these.
- `apps/backend/internal/launcher/start.go:86-124` — `runManagedApp`, the closest
  existing shape. Consider whether `runDev` can reuse it with a web-child hook rather
  than duplicating the backend/health/exit sequence.
- `docs/decisions/0019-restart-supervisor.md` — the manifest and control-socket contract.

## Risks

- `launchRestartableBackend` already takes eight parameters; adding `homeDir` pushes it
  past readability and the lint limits. Introduce a `backendLaunchConfig` struct in the
  same change rather than appending an argument.
- The dev backend child is `make`, not the launcher binary, so `dumpLogs` captures
  make's output too. Verify the failure dump is still readable when the Go build itself
  fails — that is the most common dev startup failure and the output must make it
  obvious.
- Do **not** re-exec `self __backend` for dev. That would drop rebuild-on-restart; see
  the plan's design decisions.
