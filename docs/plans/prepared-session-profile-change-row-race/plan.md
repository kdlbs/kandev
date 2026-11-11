---
created: 2026-10-20
status: implemented
requirements:
  - REQ-OFFICE-SESSION-IDENTITY-001
  - REQ-OFFICE-ROUTING-001
system_design:
  - ../../specs/office/system-design/task-session-identity-01.md
  - ../../specs/office/system-design/routing-02.md
legacy_specs: []
---

# Implementation Plan: Prepared-Session Profile Change Row Race

## Overview

Let `LaunchPreparedSession` replace a prepared execution whose profile differs
from the launch profile. The replacement stop must not fail the launch when it
has already removed the session's `executors_running` row, and must not end the
session the launch is about to reuse. Fixes
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

- The same synchronous stop also runs `Service.handleAgentStopped`, which
  moves a non-terminal session to `CANCELLED`. With the row deletion tolerated,
  the launch then fails at `verify session before registering execution:
  session ... is CANCELLED: session is terminal`, with the same
  `unknown_provider_error` classification and parked run. Confirmed on a dev
  instance after Task 01.
- `handleAgentStopped` already treats a stop as an acknowledgement when an
  explicit teardown owner holds the exact `(session, execution)` pair
  (`hasExecutionTeardownOwner`). The executor reaches that registry through
  `registerStopOwner` (`SetOnExecutionStopOwnerRegistration` →
  `Service.RegisterExecutionStopOwner`), but the profile-change stop does not
  register before stopping.
- Task 01's unit test mocked the agent manager, so the stop never ran the real
  stopped-event handler. That is why it passed while the end-to-end launch still
  failed.

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
- Register explicit stop ownership for the exact prepared execution before the
  profile-replacement stop, so the stopped event is an acknowledgement and the
  session keeps its state.
- Orchestrator integration test on a real SQLite repository through
  `NewService`, with an agent manager whose stop runs the real
  `handleAgentStopped` synchronously.
- Prepare Office sessions from the task workbench with the execution profile
  Office routing would select, so the common launch reuses the prepared
  runtime; profile drift keeps using the replacement path.
- Office Playwright E2E test with the mock agent covering the issue's flow:
  blocked assignment, prepared session from the task page, blocker completion,
  and a launched run.

### Out of scope

- Office scheduler error classification of unrelated prestart errors.
- Changing lifecycle stop semantics or making the stop asynchronous.
- Changing the shared executor mock repository's default delete behavior.
- The separate observations from manual testing: the Office status board's
  "done" gate, and an Office routine run that stayed claimed after a restart.

## Work orders

| ID | Title | Depends on | Status |
| -- | ----- | ---------- | ------ |
| [01](task-01-tolerate-cleared-executor-row.md) | Tolerate a cleared executor row on profile change | none | done |
| [02](task-02-keep-session-on-profile-replacement.md) | Keep the session alive across a profile-replacement stop | 01 | done |
| [03](task-03-prepare-with-routed-profile.md) | Prepare Office sessions with the routed execution profile | 02 | done |

## Verification

```bash
# From apps/backend:
go test ./internal/orchestrator/executor -run 'TestLaunchPreparedSession_ProfileChange' -count=1 -race
go test ./internal/orchestrator/executor/... -count=1
go test -trimpath ./internal/orchestrator -run 'TestLaunchPreparedSession_ProfileReplacement|TestEnsureSession_OfficeTask' -count=1 -race
go test -trimpath ./internal/backendapp -run TestOfficeLaunchProfilePreview -count=1
# From repo root:
bash scripts/lint-go-changed <changed-go-files>
# From apps/web:
pnpm e2e:run --host --shards 1 --project chromium tests/office/prepared-session-profile-replacement.spec.ts
```

## Risks

- Swallowing too broadly would hide the environment recovery claim refusal.
  Match only `models.ErrExecutorRunningNotFound` with `errors.Is`, and test that
  another error still fails the launch.
