---
created: 2026-10-20
status: planned
requirements:
  - REQ-OFFICE-SESSION-IDENTITY-001
  - REQ-OFFICE-ROUTING-001
system_design:
  - ../../specs/office/system-design/routing-02.md
legacy_specs: []
---

# Implementation Plan: Prepared-Session Profile Change Row Race

## Overview

Let `LaunchPreparedSession` replace a prepared execution whose profile differs
from the launch profile even when stopping that execution has already removed
its `executors_running` row. Fixes
[#4611](https://github.com/kdlbs/kandev/issues/4611).

## Confirmed root cause

- `Executor.LaunchPreparedSession`
  (`apps/backend/internal/orchestrator/executor/executor_execute.go`) loads the
  session's `executors_running` row. When its `ExecutionProfileID` differs from
  the launch profile, it calls `StopAgentWithReason(..., "execution profile
  changed", true)` and then `DeleteExecutorRunningBySessionID`.
- The stop is handled synchronously; the agent-stopped path removes the
  session's row (lifecycle `persistence.go` delete). The following delete then
  returns `models.ErrExecutorRunningNotFound`.
- The delete has no not-found tolerance, so the launch returns
  `clear previous execution profile: executor running not found`. The Office
  scheduler classifies that as `unknown_provider_error`, degrades the provider,
  and parks the run as `blocked_provider_action_required`.
- The stop call already tolerates `lifecycle.ErrExecutionNotFound`. Peer
  callers (`reconcile_liveness.go`, `event_handlers_workflow.go`) already
  tolerate `ErrExecutorRunningNotFound` on the same delete.

Reproduction: an Office task whose first run is deferred by a blocker, opened in
the UI (which prepares an `inherit_parent` session with the parent's profile),
then unblocked so the queued run launches with the workspace profile.

## Requirement conformance

Classification: the implementation violates active contracts; no requirement
change.

- `routing-02.md` (Persistence guarantees, Session/executor binding): "A profile
  change clears or ignores incompatible resume state." The desired end state
  (no row bound to the old profile) already holds when the row is missing.
- `AC-OFFICE-SESSION-IDENTITY-001.9`: rebinding a reused row's execution profile
  is a permitted recovery write and must not fail the launch.
- `AC-OFFICE-ROUTING-001.3`: with routing disabled, Office uses only the first
  provider; a misclassified internal error here must not park the run as a
  provider failure.

No system-design change: the design already states the intended outcome.

## Scope

### In scope

- Treat `models.ErrExecutorRunningNotFound` from the clear-previous-profile
  delete as success.
- Keep every other delete error fatal, including the environment recovery
  claim refusal from `ensureExecutorRunningAvailableTx`.
- Backend regression test at the executor boundary.

### Out of scope

- Office scheduler error classification of unrelated prestart errors.
- Changing lifecycle stop semantics or making the stop asynchronous.
- Changing the shared executor mock repository's default delete behavior.
- E2E coverage: the defect is a backend race between two internal calls with no
  distinct UI surface; the recovery is observable only as "the run launches".

## Work orders

| ID | Title | Depends on | Status |
| -- | ----- | ---------- | ------ |
| [01](task-01-tolerate-cleared-executor-row.md) | Tolerate a cleared executor row on profile change | none | planned |

## Verification

```bash
# From apps/backend:
go test ./internal/orchestrator/executor -run 'TestLaunchPreparedSession_ProfileChange' -count=1 -race
go test ./internal/orchestrator/executor/... -count=1
make -C apps/backend lint   # from repo root
```

## Risks

- Swallowing too broadly would hide the environment recovery claim refusal.
  Match only `models.ErrExecutorRunningNotFound` with `errors.Is`, and test that
  another error still fails the launch.
