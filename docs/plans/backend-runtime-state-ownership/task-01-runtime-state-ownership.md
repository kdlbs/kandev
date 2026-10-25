---
id: "01-runtime-state-ownership"
title: "Enforce runtime-state ownership"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/executors/requirements/port-collision-safety.md"
---

# Task 01: Enforce Runtime-State Ownership

Add a cross-platform ownership lock before any backend shared-state initialization.

## Acceptance

- A second backend that uses the same canonical Kandev home exits with code 1 before persistent
  backend initialization.
- Separate homes that reference one external SQLite database cannot open that database at the same
  time.
- The operating system releases ownership after normal process exit or forced process exit. No
  lock-file removal is necessary.

## Verification

Run these commands from the repository root:

```bash
cd apps/backend && go test -tags fts5 ./internal/backendapp/ownershiplock ./internal/backendapp
make -C apps/backend test-windows
git diff --check
```

The first regression test must fail before the production change because no backend-home lock
exists.

## Files Likely Touched

- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/main_test.go`
- `apps/backend/internal/backendapp/ownershiplock/lock.go`
- `apps/backend/internal/backendapp/ownershiplock/lock_unix.go`
- `apps/backend/internal/backendapp/ownershiplock/lock_windows.go`
- `apps/backend/internal/backendapp/ownershiplock/lock_test.go`
- `apps/backend/internal/backendapp/ownershiplock/targets.go`
- `apps/backend/internal/backendapp/ownershiplock/targets_test.go`
- `apps/backend/Makefile`
- `.github/workflows/backend-tests.yml`

## Dependencies

None.

## Parallelism

`parallel-safe` with Task 02. This task owns `internal/backendapp`, the new lock package, and Windows
test-package configuration. Task 02 owns orchestrator and status-summary files.

## Inputs

- Spec section **Exclusive runtime-state ownership**.
- ADR `2026-08-09-exclusive-runtime-state-ownership`.
- Plan section **Runtime-state ownership locks**.
- Existing supervisor stop-before-start behavior in `internal/launcher/supervisor.go` and
  `apps/cli/src/supervisor/child.ts`.

## Risks

- Do not erase a lock file during release. Another process can then lock a different file identity.
- Register subprocess cleanup before assertions so a failed test cannot retain ownership.
- Acquire every required target as one operation. Release earlier targets if a later target fails.

## Output Contract

Report the behavior, changed files, exact test results, blockers, and risks. Update this task and
`plan.md` in the same conversation.

## Results

- Added cross-platform ownership locks for the canonical Kandev home and external SQLite paths.
- Acquired ownership before backend logging and shared-state initialization; conflicts return exit code 1.
- Added subprocess, target-selection, startup-order, and Windows-sensitive package coverage.
- Verification: `go test -tags fts5 ./internal/backendapp/ownershiplock ./internal/backendapp` passed (304 tests).
