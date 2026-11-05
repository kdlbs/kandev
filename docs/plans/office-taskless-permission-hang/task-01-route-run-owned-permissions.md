---
id: "01-route-run-owned-permissions"
title: "Route run-owned permission requests"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-TASKLESS-001
acceptance_criteria:
  - AC-OFFICE-TASKLESS-001.8
  - AC-OFFICE-TASKLESS-001.9
system_design:
  - ../../specs/office/system-design/taskless-run-sessions.md
---

# Task 01: Route run-owned permission requests

## Summary

Publish a run-owned execution's permission request on a subject that wildcard
subscribers can match, with owner identity in the payload, and expose
execution-keyed resolve/cancel through the shared runtime. The orchestrator
explicitly ignores run-owned requests.

## In scope

- `PublishPermissionRequest` subject and payload owner fields.
- `Manager.ResolvePermissionByExecutionID` / `CancelPermissionByExecutionID`.
- `Runtime.ResolveRunPermission` / `CancelRunPermission` on the facade.
- `PermissionRequestData.OwnerKind` and the orchestrator early return.

## Out of scope

- Deciding how to answer (Task 02).
- Task-owned permission behavior.

## Acceptance

- `TestPublishPermissionRequest_RunOwnedReachesWildcardSubscriber` fails on
  the current code (`subject "permission_request.received."`) and passes after
  the change, asserting subject `permission_request.received.<run_session_id>`
  and the owner fields.
- Resolve/cancel by execution ID reach the execution's agentctl client and
  return `ErrNotFound` for an unknown execution or a task-owned one.
- `handlePermissionRequest` with `owner_kind = run` writes no task message and
  reads no task session.

## Verification

```bash
(cd apps/backend && go test ./internal/agent/runtime/... ./internal/orchestrator/... -run 'Permission' -count=1)
(cd apps/backend && make lint)
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/events.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/runtime.go`
- `apps/backend/internal/agent/runtime/facade.go`
- `apps/backend/internal/orchestrator/watcher/watcher.go`
- `apps/backend/internal/orchestrator/event_handlers_git.go`
- New tests: `lifecycle/events_permission_run_owner_test.go`,
  `lifecycle/manager_run_permission_test.go`, `runtime/run_permission_test.go`,
  `orchestrator/event_handlers_permission_run_owner_test.go`

## Dependencies

None.

## Risks

- The orchestrator starts receiving run-owned events once the subject is
  fixed; the ignore must ship in this task.
- Facade backends other than lifecycle (test fakes) must implement or return
  `ErrUnsupported` for the new methods.

## Parallelism

`sequential`

## Inputs

- Design: taskless-run-sessions.md, "Permission requests" and "Events and observation".
- Pattern: `agentStreamSubjectID` in `lifecycle/events.go`; `ResolvePermissionBySessionID`.

## Results

Done 2026-10-20.

- `TestPublishPermissionRequest_RunOwnedReachesWildcardSubscriber` failed before
  the change: a temporary version reported subject `permission_request.received.`
  with no subscriber. It passes after.
- `go test ./internal/agent/runtime/... ./internal/orchestrator/... -run 'Permission' -count=1`: pass.
- `make lint`: 0 issues.
- Full-package runs under heavy host disk-stall load: `internal/agent/runtime` passed;
  `internal/agent/runtime/lifecycle` failed one unrelated timing test
  (`TestPrunedDeliveryStartupReportsAuthenticatedGapBeforeWebSocketAndACP`),
  which passes 3/3 alone; `internal/orchestrator` hit its 30-minute timeout and
  reported `TestIdleReaper_TickFiltersRowsAndStopsPreservedRows`, which passes
  3/3 alone. Neither test touches permission routing.
