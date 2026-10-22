---
id: "02-office-taskless-permission-policy"
title: "Office answers taskless permission requests"
status: done
wave: 2
depends_on: ["01-route-run-owned-permissions"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-TASKLESS-001
acceptance_criteria:
  - AC-OFFICE-TASKLESS-001.1
  - AC-OFFICE-TASKLESS-001.4
  - AC-OFFICE-TASKLESS-001.9
system_design:
  - ../../specs/office/system-design/taskless-run-sessions.md
---

# Task 02: Office answers taskless permission requests

## Summary

Office subscribes to permission requests, admits only those from the current
attempt of a claimed taskless run, and answers them: approve when the request
is an auto-approval candidate, otherwise reject, otherwise cancel. Each
decision is a run event written before delivery.

## In scope

- `internal/office/service/taskless_permissions.go` handler and subscription.
- `RunPermissionResolver` seam on the Office service and its backendapp wiring.
- `permission.decision` run event (decision, option kind, title).

## Out of scope

- Turn-duration watchdog and `RecoverStale` behavior.
- Graceful-shutdown outcome of unfinished taskless turns.

## Acceptance

- Unit tests cover: auto-approval pending resolves with the offered option;
  non-auto request resolves with the first reject option; no reject option
  cancels; superseded attempt, unclaimed run and task-owned events are
  ignored; the run event is appended even when delivery fails; a transient
  delivery failure is retried; an undeliverable answer stops the execution; a
  failed decision record still answers and logs the decision; an auto-approval
  agentctl already delivered is recorded and not answered again.
- `TestTasklessPermission_RejectsAndRunFinishesOnReady` uses the real event bus
  and Office subscribers: the request is answered, and the following
  AgentReady finishes the run.

## Verification

```bash
(cd apps/backend && go test ./internal/office/service/ -run 'TasklessPermission' -count=1)
(cd apps/backend && go build ./...)
(cd apps/backend && make lint)
```

## Files likely touched

- `apps/backend/internal/office/service/taskless_permissions.go` (new)
- `apps/backend/internal/office/service/taskless_permissions_test.go` (new)
- `apps/backend/internal/office/service/event_subscribers.go`
- `apps/backend/internal/office/service/service.go`
- `apps/backend/internal/backendapp/main.go`

## Dependencies

Task 01.

## Risks

- Event handlers run async by default (`maybeAsync`); tests use
  `SetSyncHandlers(true)` like `taskless_lifecycle_test.go`.

## Parallelism

`sequential`

## Inputs

- Design: taskless-run-sessions.md, "Permission requests".
- Patterns: `handleTasklessAgentReady` (taskless_completion.go),
  `ResolveUnattendedPermission` and `pickRejectChoice` (orchestrator/agent_permissions.go).

## Results

Done 2026-10-20.

- `go test ./internal/office/service/ -run 'TasklessPermission' -count=1`: 5/5 pass
  (written red first: the resolver seam did not exist).
- `go test -tags fts5 ./internal/office/service/`: pass.
- `go test -tags fts5 ./internal/backendapp/ -run 'Office|Routine|Taskless|RunSession|Permission'`: pass (437 s).
  The whole backendapp package exceeds 20 minutes on this host; it had 415
  passing tests and no failures before the timeout.
- `go build ./...` and `make lint`: pass, 0 issues.
- PR review remediation (Greptile, 2026-10-22): added retry-then-stop delivery and
  the strict decision record; `go test ./internal/office/service/ -run TasklessPermission`
  8/8 pass (the three new tests failed first), `golangci-lint run ./internal/office/service/...` 0 issues.
- PR review remediation (Claude review, 2026-10-22): already-delivered auto-approvals are
  recorded only; `TestTasklessPermission_RecordsAlreadyAnsweredAutoApproval` failed first,
  9/9 now pass.
