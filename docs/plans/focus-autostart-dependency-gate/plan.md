---
created: 2026-08-25
status: done
requirements:
  - REQ-TASKS-TASK-DEPENDENCIES-001
system_design:
  - ../../specs/tasks/system-design/task-dependencies.md
legacy_specs: []
---

# Implementation Plan: Focus Auto-Start Dependency Gate

## Overview

Repair the `session.ensure` focus path so every `LaunchSession` request marked
as an automatic start observes the existing task-dependency gate. Add the
regression test first, then extend `launchStart`'s existing downgrade-to-prepare
condition without changing manual starts or dependency-resolution behavior.

## Scope

### In scope

- Gate `LaunchSession` requests with `IntentStart` and `AutoStart: true` on
  unresolved task dependencies.
- Fail closed when the dependency gate cannot be read.
- Preserve the workspace-only `CREATED` session used to inspect a blocked task.
- Preserve normal automatic starts after dependencies resolve.

### Out of scope

- Manual Start agent actions and direct manual `StartTask` calls.
- Existing dependency-resolution, deferred-launch, and WIP-promotion behavior.
- Frontend changes, API changes, persistence changes, and browser E2E coverage.

## Technical approach

Add the dependency predicate to the existing auto-start downgrade in
`apps/backend/internal/orchestrator/session_launch.go`. The request remains an
automatic request while `launchPrepare` runs, which suppresses passthrough
prepare upgrades and prevents a bounce back into `launchStart`.

Add focused table-driven coverage in a new orchestrator test file. Use a
minimal configurable `TaskDependencyReader` fake and the established launch
counter/service wiring to prove blocked and read-error requests prepare only,
while an unblocked request reaches the agent launch path.

## Tests

- `AC-TASKS-TASK-DEPENDENCIES-001.1`: `TestLaunchSession_AutoStartDependencyGate`
  covers an unresolved dependency, a resolved dependency, and a dependency-read
  error at the `LaunchSession` boundary.
- The blocked and read-error cases assert a `CREATED` workspace-only session,
  no agent launch, and no transition to `SCHEDULING` or `IN_PROGRESS`.
- The unblocked case asserts the normal launch path still reaches a running
  agent session.

## Work orders

- [x] [Task 01: Gate focus-induced auto-start](task-01-gate-focus-autostart.md)

## Verification results

- Focused regression passed:
  `go test -tags fts5 ./internal/orchestrator -run '^TestLaunchSession_AutoStartDependencyGate$' -count=1`.
- Focused race regression passed:
  `go test -race -tags fts5 ./internal/orchestrator -run '^TestLaunchSession_AutoStartDependencyGate$' -count=1`.
- `make -C apps/backend lint` passed with zero issues.
- `gofmt -l` reported no changes for the modified Go files.
- `make -C apps/backend test` passed for `internal/orchestrator` and the other
  packages reached before two unrelated environment-sensitive packages failed.
  `internal/common/config` and `internal/launcher` selected the existing
  `/root/.kandev/config.yaml` instead of each test's temporary home.

## Risks

- The downgrade must retain `AutoStart: true`; clearing it would allow a
  passthrough prepare to upgrade back into an agent start.
- The dependency reader is optional in some test/service constructions, so the
  nil-reader behavior must remain unchanged.
- Broad backend validation can expose unrelated repository failures; targeted
  orchestrator evidence remains the regression proof.
