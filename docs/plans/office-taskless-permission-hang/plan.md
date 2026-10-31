---
created: 2026-10-20
status: done
requirements:
  - REQ-OFFICE-TASKLESS-001
system_design:
  - ../../specs/office/system-design/taskless-run-sessions.md
legacy_specs: []
---

# Implementation Plan: Taskless Office runs hang on permission requests

## Overview

A taskless Office run (for example the coordinator heartbeat routine) stays
`claimed` indefinitely when its agent asks for a tool permission. The run holds
the agent's concurrency slot, so with `max_concurrent_sessions = 1` every later
run for that agent is deferred with `gate: agent_ceiling`.

Confirmed root cause:

1. `lifecycle.EventPublisher.PublishPermissionRequest` builds the subject from
   `execution.SessionID`. A run-owned execution has an empty task session ID,
   so the subject is `permission_request.received.` (empty last token).
2. The only subscriber, the orchestrator watcher, subscribes to
   `permission_request.received.*`. `bus.compilePattern` turns `*` into
   `[^.]+`, which never matches an empty token, so the event reaches nobody.
   The orchestrator handler would also drop it (`missing session_id`), and all
   resolution APIs (`ResolveAgentPermission`, `ResolvePermissionBySessionID`)
   are keyed by task session.
3. agentctl keeps the request pending. Because the backend is attached, the
   5-second delivery fallback does not fire. Auto-approve profiles also hang:
   agentctl marks the request `AutoApprovalPending` and waits for the backend to
   claim the decision.
4. Nothing else ends the turn: no AgentReady (turn never completes), no failure,
   no initial-prompt timeout (`appctx.Detached(ctx, stopCh, 0)`). The 30-minute
   `RecoverStale` sweep requeues the run but does not stop the execution.

Evidence: reproduced twice on an isolated dev instance (`v0.94.0-884`,
claude-acp CEO, coordinator heartbeat routine): once on a clean start and once
on the first run after restarting against the same database. In both, the
backend log stops after agentctl's `received permission request`, 7 to 8
seconds into the turn, while the run stays `claimed` and its run session stays
`running`. Restart is incidental. A temporary lifecycle unit test reproduced
the dropped event (`subject "permission_request.received."` delivered to no
subscriber).

Implementation order: route the event and expose execution-keyed resolution
first (Task 01), then add the Office policy that answers it (Task 02), then
prove the whole path end to end (Task 03). Each step is a prerequisite for the
next.

## Scope

### In scope

- Run-owned permission event subject and payload owner fields.
- Explicit orchestrator ignore of run-owned permission requests.
- Execution-keyed permission resolve/cancel on the shared runtime facade.
- Office taskless permission policy: approve when the launched execution profile auto-approves,
  otherwise reject (or cancel), with a run event per decision.
- Unit, integration and Playwright regression coverage.

### Out of scope

- A general turn-duration watchdog for taskless runs, and fixing
  `RecoverStale` requeuing without stopping the execution. Both belong to the
  outstanding `AC-OFFICE-TASKLESS-001.5`/`.6` lifecycle work.
- Graceful shutdown recording an unfinished taskless turn as `finished`
  (observed during reproduction: `AgentStopped` reaches
  `handleTasklessAgentCompleted`). Separate defect under `.6`.
- Any UI for answering taskless permissions interactively.

## Technical approach

### Runtime (Task 01)

- `internal/agent/runtime/lifecycle/events.go`: `PublishPermissionRequest`
  uses the same subject rule as `agentStreamSubjectID` (run session ID for
  run-owned executions) and adds `owner_kind`, `workspace_id`, `run_id`,
  `run_session_id`, `run_attempt`, `agent_profile_id` and `agent_execution_id`
  to `PermissionRequestEventPayload`.
- `internal/agent/runtime/lifecycle/manager_interaction.go`: add
  `ResolvePermissionByExecutionID` and `CancelPermissionByExecutionID`,
  sharing a helper with the existing `...BySessionID` variants.
- `internal/agent/runtime/runtime.go` and `facade.go`: add
  `ResolveRunPermission(ctx, executionID, requestID, pendingID, optionID)` and
  `CancelRunPermission(ctx, executionID, requestID, pendingID)`. Both reject a
  non-run owner.
- `internal/orchestrator/watcher/watcher.go` (`PermissionRequestData` gains
  `OwnerKind`) and `internal/orchestrator/event_handlers_git.go`:
  `handlePermissionRequest` returns early for `owner_kind == "run"` before
  any task-session read or write.

### Office (Task 02)

- New `internal/office/service/taskless_permissions.go`:
  `handleTasklessPermissionRequest` subscribed in
  `event_subscribers.go` to `events.BuildPermissionRequestWildcardSubject()`.
  It reuses `exactRunSessionEvent` / `resolveLifecycleRun` admission, decides
  per the design table, appends a `permission.decision` run event via
  `AppendRunEvent`, then calls the new `RunPermissionResolver` seam.
- `internal/office/service/service.go`: `RunPermissionResolver` interface and
  `SetRunPermissionResolver`; `internal/backendapp/main.go` wires
  `runtimeapi.New(lifecycleMgr)` next to `SetRunExecutionStopper`.

### Compatibility

| Execution | Launched execution profile `auto_approve` | Result |
| --- | --- | --- |
| task-owned | any | unchanged (orchestrator path) |
| run-owned | on | approved by Office; turn continues |
| run-owned | off, reject offered | rejected by Office; turn continues |
| run-owned | off, no reject | cancelled by Office; turn continues |
| run-owned, superseded attempt | any | dropped; not answered |

## Tests

| AC | Evidence |
| --- | --- |
| AC-OFFICE-TASKLESS-001.9 | `lifecycle/events_permission_run_owner_test.go`: `TestPublishPermissionRequest_RunOwnedReachesWildcardSubscriber` (fails today) |
| AC-OFFICE-TASKLESS-001.9 | `lifecycle/manager_run_permission_test.go`, `runtime/run_permission_test.go`: resolve/cancel by execution ID |
| AC-OFFICE-TASKLESS-001.8 | `orchestrator/event_handlers_permission_run_owner_test.go`: `TestHandlePermissionRequest_IgnoresRunOwned` |
| AC-OFFICE-TASKLESS-001.9, .4 | `office/service/taskless_permissions_test.go`: auto-approve, reject, cancel, superseded attempt, unclaimed run, run event recorded |
| AC-OFFICE-TASKLESS-001.1, .9 | `office/service/taskless_permissions_test.go`: `TestTasklessPermission_RejectsAndRunFinishesOnReady` (real bus and Office subscribers; answered request, then AgentReady finishes the run) |
| AC-OFFICE-TASKLESS-001.1 | `cmd/mock-agent/office_scenario_test.go`: Office prompt directive selects a scenario |

## E2E tests

| Flow | AC | Spec |
| --- | --- | --- |
| Taskless routine whose mock agent requests a permission finishes, frees the agent, and records the decision | AC-OFFICE-TASKLESS-001.1, .9 | `apps/web/e2e/tests/office/taskless-routine-permission.spec.ts` (project `chromium`) |

## Work orders

- [x] [Task 01: Route run-owned permission requests](task-01-route-run-owned-permissions.md)
- [x] [Task 02: Office answers taskless permission requests](task-02-office-taskless-permission-policy.md)
- [x] [Task 03: End-to-end taskless permission regression](task-03-taskless-permission-e2e.md)

## Verification results

- Unit and integration: lifecycle, runtime, orchestrator, Office service,
  watcher, mock-agent and event bus tests for this change pass; details per
  work order.
- E2E: `tests/office/taskless-routine-permission.spec.ts` 2/2 pass; both cases
  fail with the fix removed.
- Lint: `make lint` 0 issues. Specs: `list-docs.py validate` and
  `lint-spec-files.py --all` pass.
- Host note: full-package runs were slowed by ~30 ms fsync latency; two
  timing-sensitive tests failed only under that load and pass alone.

## Risks

- Once the subject is non-empty, the orchestrator watcher starts receiving
  run-owned requests. Task 01 must land the explicit ignore in the same change.
- Rejecting a permission can make the agent end its turn with less work done.
  That is the intended unattended behavior and matches coordinator unattended
  task turns and utility agents.
- The mock agent must receive a directive inside an Office taskless prompt
  (Task 03); if the routine description is not part of the prompt, carry the
  directive in the agent instructions instead.
