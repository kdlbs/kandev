---
id: "01-move-approver-guard"
title: "Human step moves respect Office approvers"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-HUMAN-DONE-002
acceptance_criteria:
  - AC-OFFICE-HUMAN-DONE-002.1
  - AC-OFFICE-HUMAN-DONE-002.2
  - AC-OFFICE-HUMAN-DONE-002.3
  - AC-OFFICE-HUMAN-DONE-002.4
system_design:
  - ../../specs/office/system-design/human-done-final-step.md
---

# Task 01: Human step moves respect Office approvers

## Summary

A human move of an Office task into a completing step is refused before any
write when an approver is pending. Today `syncTaskStateForWorkflowMove` writes
`COMPLETED` in the move transaction, and the Office approver seam skips tasks
that are already complete.

## In scope

- `OfficeApprovalGuard`, `PendingApprover`, `ApprovalsPendingMoveError`, and
  `SetOfficeApprovalGuard` in `task/service`.
- The guard call in `MoveTaskWithOptions`, under the conditions in the system
  design: human actor, `IsFromOffice`, the sync set `COMPLETED`.
- The 409 `task_approvals_pending` mapping in `handleSelectedMoveError`.
- A `backendapp` adapter over the dashboard's pending-approver read with names.
- `task_approvals_pending` -> `task:taskMoveErrorApprovalsPending` in every
  catalog.

## Out of scope

- The Office PATCH Done path (Task 02).
- Engine, agent and automation moves.

## Acceptance

- The regression test
  `TestMoveTask_OfficeHumanIntoCompletingStep_ApproversPending_RefusedBeforeWrite`
  fails on current main (the task ends `COMPLETED`) and passes after: step,
  state and step-entry markers are unchanged, and no event is published.
- Engine moves, non-Office tasks, non-completing targets, and a nil guard keep
  the current behavior.
- `POST /api/v1/tasks/:id/move` returns 409
  `{code: "task_approvals_pending", pending_approvers: [...]}`.

## Verification

```bash
cd apps/backend && go test -tags fts5 -count=1 -run 'TestMoveTask_Office|TestMoveTask_NonOfficeTask_SkipsApprovalGuard|TestHandleSelectedMoveError_ApprovalsPending409' ./internal/task/...
cd apps/backend && go test -tags fts5 -count=1 ./internal/task/... ./internal/orchestrator/... ./internal/office/dashboard/...
cd apps/web && pnpm exec vitest run components/task/task-move-error-message.test.ts
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
make -C apps/backend lint
```


## Files likely touched

- `apps/backend/internal/task/service/service_workflow.go`
- `apps/backend/internal/task/service/office_approval_guard.go` (new)
- `apps/backend/internal/task/service/office_approval_guard_test.go` (new)
- `apps/backend/internal/task/handlers/errors.go`
- `apps/backend/internal/task/handlers/move_approvals_error_test.go` (new)
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/office/dashboard/decisions.go` (`PendingApproversForCompletion`) and `pending_approvers_completion_test.go`
- `apps/backend/internal/backendapp/adapters_office_completion.go` and its test
- `apps/web/components/task/task-move-error-message.ts` and its test
- `apps/web/src/locales/{en,pseudo,pt-pt,ja,ko,zh-cn,zh-hk,zh-tw}/task.json`

## Dependencies

None.

## Risks

- `task/service` must not import `office/dashboard` (dashboard imports
  `task/service`). Keep the interface in `task/service`; the adapter lives in
  `backendapp`.

## Results

Done. Verification commands pass; see [plan verification results](plan.md#verification-results).
