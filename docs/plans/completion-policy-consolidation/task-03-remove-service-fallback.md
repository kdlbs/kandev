---
id: "03-remove-service-fallback"
title: "Remove the service-level approval fallback"
status: done
wave: 3
depends_on: ["02-completion-policy-hook"]
plan: "plan.md"
requirements:
  - REQ-TASKS-COMPLETION-005
acceptance_criteria:
  - AC-TASKS-COMPLETION-005.2
system_design:
  - ../../specs/tasks/system-design/task-completion.md
---

# Task 03: Remove the service-level approval fallback

## Summary

Delete the second, non-atomic approver check in the task service so that the
repository hook is the only guard path. Resolves #5252.

## In scope

- Delete `guardOfficeHumanCompletion`, `atomicOfficeCompletionConfigured`, and
  `officeCompletionGuardConfigurer`. Delete the legacy branch in
  `persistHumanTaskState`, which now always calls the repository's human state
  write.
- Replace `OfficeApprovalGuard` and `SetOfficeApprovalGuard` with
  `ApproverNameResolver` and `SetApproverNameResolver`, used only to name
  refused approvers.
- Move tests that rely on the fallback (`office_approval_guard_test.go` and
  any service test that sets the guard over a fake repository) to a real
  SQLite repository with the Office policy registered.

## Out of scope

- Changing the refusal error contract or HTTP mapping.

## Acceptance

- `grep -rn "PendingApproversForCompletion\|guardOfficeHumanCompletion" apps/backend`
  finds nothing.
- Every migrated test still asserts that the refusal leaves step, state, and
  events unchanged.

## Verification

```bash
(cd apps/backend && go test ./internal/task/service/... ./internal/task/handlers/... ./internal/backendapp/... ./internal/office/dashboard/...)
bash scripts/lint-go-changed $(git diff --name-only origin/main -- '*.go')
```

## Files likely touched

- `apps/backend/internal/task/service/office_approval_guard.go`, `office_approval_guard_test.go`, `service.go`
- `apps/backend/internal/backendapp/adapters_office_completion.go`, `adapters_office_completion_test.go`, `main.go`

## Dependencies

Task 02.

## Risks

- A non-SQLite repository implementation without the human state write would
  now fail closed instead of writing without a guard. List the
  `TaskRepository` implementations and confirm that only test fakes are
  affected.

## Parallelism

`sequential`

## Inputs

- System design: "Completion policy hook" (fallback paragraph).

## Results

Removed the service-level approval fallback and the obsolete approver lookup
path. The full `internal/task/service` suite and task handler tests passed.
