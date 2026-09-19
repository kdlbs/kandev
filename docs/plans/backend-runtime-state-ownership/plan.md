---
spec: docs/specs/executors/requirements/port-collision-safety.md
created: 2026-08-09
status: completed
---

# Implementation Plan: Backend Runtime-State Ownership

## Overview

This repair prevents a second backend from changing the runtime state of a live Kandev instance.
It also makes startup session recovery publish authoritative state before backend readiness.

The confirmed reproduction used this command in a task worktree while another backend was active:

```bash
make -C apps/backend dev
```

The second process opened the shared database and reconciled sessions. It then failed to bind the
occupied HTTP port. The first backend later saw an already-settled session, so it did not clear the
stored `generating` task summary.

This plan implements
[ADR-2026-08-09-exclusive-runtime-state-ownership](../../decisions/2026-08-09-exclusive-runtime-state-ownership.md).
It also updates the status-delivery and interrupted-task contracts.

## Backend

### Runtime-state ownership locks

Add `apps/backend/internal/backendapp/ownershiplock/` with a small cross-platform lock API. The
package owns target selection, canonical paths, lock acquisition, and lock release.

- `lock.go` defines the owner type, conflict error, target metadata, and all-or-nothing acquisition.
- `lock_unix.go` uses a non-blocking exclusive file-handle lock.
- `lock_windows.go` uses the equivalent non-blocking Windows file lock through
  `golang.org/x/sys/windows`, which is already a backend dependency.
- `targets.go` selects the Kandev-home lock. It adds a database lock when SQLite uses a path outside
  the canonical home.
- Lock files remain after release. The open file handle owns the lock.

Update `apps/backend/internal/backendapp/main.go`. Acquire ownership after `config.Load()` and flag
overrides, but before `logger.NewBackendLogger()`. Hold ownership until `Run` completes all service
cleanup. On any acquisition error, write one clear stderr error and return exit code 1.

The rejected process must not create backend logs, open a database, take a backup, apply a
migration, launch agentctl, reconcile a session, or start an HTTP listener.

Add the native lock package to `apps/backend/Makefile` and
`.github/workflows/backend-tests.yml` Windows-sensitive package lists.

### Startup state publication

Update `apps/backend/internal/orchestrator/service.go`. Route a successful startup transition from
`STARTING` or `RUNNING` to `WAITING_FOR_INPUT` through the existing semantic session-state
publication path.

The transition must keep these existing startup behaviors:

- Abandon an orphan turn.
- Move an eligible task from `IN_PROGRESS` to `REVIEW`.
- Add the interrupted-task marker.
- Preserve the executor row and resume data.
- Repair confirmed-dead local process data.

The semantic state event must contain the authoritative primary-session state and an explicit empty
foreground-activity value. The existing task activity publisher remains the settle fallback for a
`RUNNING` session. A session that is already `WAITING_FOR_INPUT` remains a no-op.

No status-summary schema or frontend change is necessary. The current projector consumes semantic
session events and explicit task activity values. The current sidebar already shows the interrupted
icon when `generating` is absent.

## Public Documentation

Update these public pages:

- `docs/public/backend-development.md`
- `docs/public/contributing.md`
- `docs/public/configuration.md`

State that one backend owns a Kandev home at a time. Explain that raw backend commands use the
normal home unless `KANDEV_HOME_DIR` is set. Keep the existing isolated command example.

Do not document lock-file removal as a recovery step. A stale file does not hold an operating-system
lock.

## Tests

### Runtime-state ownership

- **What:** A second process cannot acquire the same Kandev-home lock.
- **File:** `apps/backend/internal/backendapp/ownershiplock/lock_test.go`
- **How:** Use a subprocess helper and temporary directories. Assert conflict, release after normal
  exit, release after forced exit, and independent-home success.

- **What:** Separate homes cannot open the same external SQLite database.
- **File:** `apps/backend/internal/backendapp/ownershiplock/targets_test.go`
- **How:** Build target sets for default and custom database paths. Assert canonical deduplication
  and external-database ownership.

- **What:** Backend startup stops before shared-state initialization when ownership is unavailable.
- **File:** `apps/backend/internal/backendapp/main_test.go`
- **How:** Hold a temporary-home lock, invoke the backend entry seam, and assert exit code 1. Assert
  that no database, backup, backend log, agentctl child, or listener appears.

### Startup state publication

- **What:** Startup recovery publishes `RUNNING -> WAITING_FOR_INPUT` through the semantic session
  event path.
- **File:** `apps/backend/internal/orchestrator/reconcile_restart_test.go`
- **How:** Seed a running session, task, orphan turn, executor row, and recording event bus. This
  regression must fail before the production change because the state event is absent.

- **What:** A stale `RUNNING` and `generating` summary converges after startup recovery.
- **File:** `apps/backend/internal/task/statussummary/projector_test.go`
- **How:** Seed a stored summary, send the authoritative recovery event, and assert a newer revision
  with `WAITING_FOR_INPUT`, empty foreground activity, and zero active subagents.

- **What:** Existing task review, interruption, resume, and executor-row behavior does not change.
- **File:** `apps/backend/internal/orchestrator/reconcile_restart_test.go` and
  `apps/backend/internal/orchestrator/turn_lifecycle_test.go`
- **How:** Extend the current startup-reconciliation cases. Assert the existing database outcomes
  with the new event assertions.

### Public docs

- **What:** Published docs describe one-home ownership and isolated direct backend commands.
- **Files:** `docs/public/backend-development.md`, `docs/public/contributing.md`, and
  `docs/public/configuration.md`
- **How:** Run the public-doc tests and validator.

## Verification Results

- Focused ownership tests: `go test -tags fts5 ./internal/backendapp/ownershiplock ./internal/backendapp` — 304 tests passed.
- Startup recovery, task service, and projector tests: `go test -tags fts5 ./internal/orchestrator ./internal/task/service ./internal/task/statussummary` — 2,658 tests passed.
- Public documentation tests: `node --test scripts/validate-public-docs.test.mjs` — 58 tests passed; `node scripts/validate-public-docs.mjs` — 41 pages validated.

## Implementation Waves And Parallel Candidates

Wave 1 contains two parallel candidates. User authorization is required before delegation.

- [x] [Task 01: Enforce runtime-state ownership](task-01-runtime-state-ownership.md) —
  `parallel-safe`
- [x] [Task 02: Publish startup recovery state](task-02-startup-recovery-publication.md) —
  `parallel-safe`

The tasks use disjoint source packages. Neither task changes a schema, generated contract,
dependency file, or shared package configuration.

Wave 2:

- [x] [Task 03: Document runtime ownership](task-03-runtime-ownership-docs.md) — depends on Task 01

Implementation remains sequential in the primary conversation unless the user explicitly
authorizes subagents.

## Risks

- File-lock behavior differs between Unix and Windows. Native Windows coverage is required.
- A process test can leak a child and retain a lock after an early test error. Each test must
  register cleanup before its first assertion.
- A launcher restart must wait for the old backend to release ownership. Existing supervisors
  already stop and join the old process before replacement startup. Regression coverage must keep
  this behavior.
- Network filesystems can have different advisory-lock guarantees. This repair targets supported
  local Kandev homes and local SQLite files.

## Out of Scope

- Automatic `KANDEV_HOME_DIR` selection for raw backend commands.
- Active-active backend support for Postgres or NATS.
- A sidebar or debug-panel redesign.
- A database migration or status-summary schema change.

## Open Questions

None.
