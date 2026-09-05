---
id: "03-stop-reaches-registered-execution"
title: "Reach a registered execution from a task stop"
status: done
wave: 3
depends_on: ["02-terminal-stall-teardown"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/task-stop-reachability.md"
system_design: "../../specs/tasks/system-design/task-stop-reachability.md"
requirements:
  - REQ-TASKS-TASK-STOP-REACHABILITY-001
acceptance_criteria:
  - AC-TASKS-TASK-STOP-REACHABILITY-001.1
  - AC-TASKS-TASK-STOP-REACHABILITY-001.2
  - AC-TASKS-TASK-STOP-REACHABILITY-001.3
  - AC-TASKS-TASK-STOP-REACHABILITY-001.4
  - AC-TASKS-TASK-STOP-REACHABILITY-001.5
  - AC-TASKS-TASK-STOP-REACHABILITY-001.6
  - AC-TASKS-TASK-STOP-REACHABILITY-001.7
---

# Task 03: Reach a registered execution from a task stop

## Outcome

`Executor.StopByTaskID` resolves what to halt from the union of the
active-session query and the in-memory execution registry. A session that is
terminal in the database but still holds a registered execution is stopped, and
its terminal state and error message survive.

## In scope

- Add a read-only task-scoped lookup to the execution registry:
  `ExecutionStore` gains a method returning the session IDs of executions whose
  `TaskID` matches, and `Manager` exposes it. `AgentExecution.TaskID` already
  exists; no new index is required.
- Add that method to `executor.AgentManagerClient`.
- In `StopByTaskID`, after the existing `ListActiveTaskSessionsByTaskID` call,
  add registry-only sessions by loading each session row with
  `repo.GetTaskSession`. Skip and log a row that cannot be loaded. Return
  `ErrExecutionNotFound` only when the union is empty.
- Stop a registry-recovered session without the session-state transition, so its
  terminal state and error message are preserved. Extract the shared
  execution-resolution and stop-scheduling part of `stopWithSession` rather than
  duplicating it.
- Log recovered sessions distinctly from query-resolved sessions, naming the
  session and execution.

## Exclusions

- Do not widen the SQL active-state set in
  `apps/backend/internal/task/repository/sqlite/session.go`. Every other caller
  of `ListActiveTaskSessionsByTaskID` depends on its current meaning.
- Do not change `stopSession`, `StopExecution`, the `agent.cancel` path, or
  `StopTaskForCoordinator`.
- Do not change the behavior for a session that is in an active persisted state.
- Do not add startup or background reconciliation of orphans.

## Acceptance conditions

1. A task whose only session is `FAILED` with a registered execution is stopped
   successfully, the execution is stopped once, and the session stays `FAILED`
   with its original error message.
2. A task with an active session behaves exactly as today, including the
   cancellation transition and its events.
3. A task with neither an active session nor a registered execution still
   returns `ErrExecutionNotFound` and changes nothing; a repeated stop after the
   executions are gone creates no duplicate transition.

## Verification

```sh
cd apps/backend && go test ./internal/orchestrator/... -count=1
cd apps/backend && go test ./internal/agent/runtime/lifecycle/... -count=1
```

New regressions:

- `apps/backend/internal/orchestrator/executor/executor_interaction_test.go`:
  `TestStopByTaskID_StopsFailedSessionWithRegisteredExecution` — seeds a
  `FAILED` session plus a registered execution in the fake agent manager;
  asserts no error, one stop for that execution, and an unchanged session state.
  It must first fail with `ErrExecutionNotFound` and zero stops.
  `TestStopByTaskID_ActiveSessionKeepsCancellationTransition` and
  `TestStopByTaskID_NoActiveSessionAndNoRegisteredExecutionReportsNotFound`
  pin the unchanged paths.
- `apps/backend/internal/agent/runtime/lifecycle/execution_store_test.go`:
  `TestExecutionStore_ListsSessionIDsForTask` — covers matching, non-matching,
  empty-task, and post-`Remove` cases.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/execution_store.go`
- `apps/backend/internal/agent/runtime/lifecycle/execution_store_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`
- `apps/backend/internal/orchestrator/executor/executor.go`
- `apps/backend/internal/orchestrator/executor/executor_interaction.go`
- `apps/backend/internal/orchestrator/executor/executor_interaction_test.go`

## Dependencies

Task 02. Its regression constructs the orphan that Task 02 stops producing, so
the fake agent manager, not the stall path, seeds the registered execution.

## Results

Added `ExecutionStore.ListSessionIDsForTask(taskID) []string`
(`execution_store.go`) — a read-only in-memory scan of registered executions
by `TaskID`, independent of session persisted state — and exposed it as
`Manager.ListSessionIDsForTask` (`manager_execution.go`). Added it to
`executor.AgentManagerClient` (`executor.go`) and wired the real path through
`lifecycleAdapter.ListSessionIDsForTask` (`backendapp/adapters.go`).

In `executor_interaction.go`:
- Extracted `resolveExecutionIDForStop` (execution-ID lookup + not-found
  classification, previously inlined in `stopWithSession`) and
  `registerAndScheduleStop` (advisory ownership registration + detached
  teardown scheduling, previously the tail of `stopWithSession`). Both are now
  shared rather than duplicated, per the work order.
- Added `stopRegistryRecoveredSession`, which resolves and stops an
  execution via those two shared helpers but skips the CANCELLED
  transition — the session's terminal state and error message stay
  authoritative.
- Added `recoverRegistryOnlySessions`, called from `StopByTaskID` after the
  existing `ListActiveTaskSessionsByTaskID` call: it asks the registry for
  taskID's registered session IDs, skips any already covered by the active
  query, loads each remaining row via `repo.GetTaskSession` (skip + log on
  failure), and returns the loaded rows for `stopRegistryRecoveredSession`.
  `StopByTaskID` now returns `ErrExecutionNotFound` only when both the active
  query and the recovered set are empty; the SQL active-state set in
  `sqlite/session.go` was not touched.

Added regressions:
- `execution_store_test.go`:
  `TestExecutionStore_ListsSessionIDsForTask` — matching/non-matching/empty-
  task/post-`Remove` cases.
- `executor_interaction_test.go`:
  `TestStopByTaskID_StopsFailedSessionWithRegisteredExecution` (new
  behavior — a FAILED session with a registry-only execution is stopped,
  forced, once, with its FAILED state and error message unchanged),
  `TestStopByTaskID_ActiveSessionKeepsCancellationTransition` and
  `TestStopByTaskID_NoActiveSessionAndNoRegisteredExecutionReportsNotFound`
  (pin the unchanged paths, including that a repeated stop after both sets
  are empty stays `ErrExecutionNotFound` with no duplicate transition).

RED verification for the primary test: reverted the interface/store/executor
production and mock plumbing via a scoped `git stash` (unique tag, applied by
SHA, dropped after) and confirmed the package fails to *compile* without it
(`unknown field listSessionIDsForTaskFunc in struct literal of type
mockAgentManager`) — the registry-recovery capability does not exist on
`main` at all, so there is no lesser "wrong behavior" RED state to assert
against; a compile failure is the correct RED here. Re-applied the stash and
confirmed all three new tests pass.

Adding `ListSessionIDsForTask` to `executor.AgentManagerClient` required
updating every other implementer to keep the codebase compiling: the mock
`GetExecutionIDForSession` implementations in
`internal/orchestrator/event_handlers_test.go`,
`internal/orchestrator/scheduler/scheduler_test.go`, and
`internal/orchestrator/executor/executor_mocks_test.go` (all gained a real
`listSessionIDsForTaskFunc`-backed method), plus
`internal/integration/simulated_agent_manager_test.go`'s
`SimulatedAgentManagerClient` (gained a real implementation filtering its
`simulatedInstance` map by `taskID`, matching its other lookups).

Verification: `go test ./internal/orchestrator/... -count=1` — all packages
green. `go test ./internal/agent/runtime/lifecycle/... -count=1` — green
except the same 7 pre-existing, environment-caused failures documented in
Task 02's results (disk at 99% capacity on this host); none touch this
change. `go build ./...` — clean. `gofmt -l` on every touched file — clean.

### Hardening: recovery-path fail-open on a transient load error

Self-review of this task's own recovery path (walking its failure branches,
not the happy path) found a real fail-open bug: `recoverRegistryOnlySessions`
logged and silently dropped a registered session when `repo.GetTaskSession`
returned an error. If that was the *only* registered session, `StopByTaskID`
then returned the same `ErrExecutionNotFound` it returns when nothing is
registered at all — collapsing "a transient DB read failed" (e.g. `database
is locked`, which this environment demonstrably produces under load) into
"there is no orphan," exactly the false all-clear this task exists to
eliminate.

Fixed: `recoverRegistryOnlySessions` now returns `([]*models.TaskSession,
error)`, keeping the last load error. `StopByTaskID` no longer reports
`ErrExecutionNotFound` when a registered session exists but couldn't be
loaded — it wraps and returns the real error instead, so a transient failure
is never indistinguishable from "genuinely nothing to stop."

New regression (`executor_interaction_test.go`):
`TestStopByTaskID_RegistryRecoveryLoadFailureIsNotReportedAsNotFound` — a
registered session whose `GetTaskSession` load fails; asserts the returned
error is not `ErrExecutionNotFound` and wraps the load failure. Failed first
with `error = execution not found, want the load failure surfaced instead of
ErrExecutionNotFound`, then passed after the fix.

Verification: `go test ./internal/orchestrator/executor/... -count=1` —
green (including the 3 existing `TestStopByTaskID_*` tests, unaffected).
`go build ./...` — clean. `gofmt -l` — clean. `golangci-lint run
./internal/orchestrator/executor/... --new-from-rev=<merge-base>` — 0 issues.

### RUNNING-orphan investigation (requested before extending defect 3)

Asked to establish, before trusting defect 3's FAILED-only scope: does
defect 1 (pre-fix) leave a stalled session at RUNNING instead of FAILED, and
if so, is `StopByTaskID` blind to *that* case too, or only to FAILED?

**Pre-fix, a metadata-only stream leaves the session at RUNNING forever, not
FAILED.** `handleAgentStalled` (`event_handlers_stall.go`) is the only code
path that transitions a stalled session to FAILED (via
`recordSessionLaunchFailure`, gated on `payload.NeverStarted`). Pre-Task-01,
`recordActivity` kept resetting `lastActivityAt` for metadata frames, so
`waitForPromptDone`'s 5-minute condition never tripped, `handleAgentStalled`
was never called, and the session simply stayed in whatever state it already
had — RUNNING, since that's the state a session is in while a prompt is
in flight. This is the same defect already named as defect 1, stated in
terms of its state consequence.

**`StopByTaskID` was never blind to a RUNNING orphan.**
`ListActiveTaskSessionsByTaskID`'s SQL set is `CREATED, STARTING, RUNNING,
WAITING_FOR_INPUT` — RUNNING has always been in it, so a session stuck at
RUNNING with a still-registered execution was always reachable through the
pre-existing, unmodified `stopWithSession` path. Defect 3's blindness is
specific to FAILED, a state that only becomes reachable-but-excluded once
defect 2 exists to produce it. There is no second, RUNNING-side blindness for
Task 03 to also close.

**Conclusion: no fourth defect shape.** Task 01 (honest clock) + Task 02
(forced teardown on the now-honestly-detected stall) together eliminate the
"stuck at RUNNING forever" failure mode at its source — the watchdog now
fires and the process is torn down, rather than needing a second reachability
path for a state that should no longer occur. Nothing new to build here.

**The two cited live specimens (d8631f7a, 7e8f19cd) are a different, out-of-
scope shape**, exactly as flagged when raised: both are long-running
sessions with thousands of real messages, not never-started ones, and the
description of their stuckness (a workflow-board move deferred to turn-end on
a session the board still calls RUNNING, so no move takes effect) points at
the Kandev workflow/board layer, not at `StopByTaskID`'s session resolution.
Not folded into this card's fix; not investigated further, per the explicit
scope instruction to name rather than silently absorb a fourth shape.

**Third data point (this card's own never-started launch, no session/output
for 10 minutes):** consistent with defect 1 exactly as modeled above. The fix
is implemented in this worktree but cannot be observed live against that
specific already-stuck launch from here — doing so needs the actual running
backend process for this Kandev instance restarted onto the fixed code, which
is a deployment action outside this card's scope (a source change, not an
operational one).
