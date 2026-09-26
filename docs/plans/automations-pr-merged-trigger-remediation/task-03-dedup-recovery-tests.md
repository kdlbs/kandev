---
id: "03-dedup-recovery-tests"
title: "Lock dedup recovery semantics"
status: done
wave: 3
depends_on: ["02-bind-archive-target"]
plan: "plan.md"
spec: "../../specs/office/requirements/automations-pr-merged-trigger.md"
---

# Task 03: Lock Dedup Recovery Semantics

## Intent

Protect the trigger-specific rule that only a firing which creates a task and records its run consumes
the one-time merge dedup key.

## Acceptance

- A concurrency-cap skip for `github_pr_merged` writes an empty key; another event is admitted.
- Equivalent skips for existing trigger types retain their keys.
- A pre-task failure for `github_pr_merged` writes an empty key.
- A task-created run that later fails retains its key and blocks a duplicate.
- Failure to record the successful run deletes the abandoned task and leaves no row/key.

## TDD sequence

1. Add focused automation service tests for merged-PR cap skips and an existing-trigger control.
2. Add orchestrator tests for pre-task failure, post-task failure, and failed success-row persistence.
3. Run the focused packages. Change production logic only if a test exposes a mismatch; otherwise keep
   this task test-only and resist refactoring unrelated dedup code.

## Files likely touched

- `apps/backend/internal/automation/service_test.go`
- `apps/backend/internal/orchestrator/event_handlers_automation_test.go`
- production files only if the specified behavior is not already correct

## Dependencies

Task 02, because the task-created path now also carries the bound target metadata.

## Parallelism

`sequential` — these regressions share the automation run lifecycle exercised by Task 02.

## Verification

- `cd apps/backend && go test ./internal/automation ./internal/orchestrator`

## Risks

- Do not change `HasRunWithDedupKey`; the trigger-specific contract is on write paths.
- Do not accidentally make push or CI skipped/failed rows keyless.

## Completed validation

- RED: the new merged-PR cap-skip, pre-task failure, and task-created failure regressions failed or
  lacked coverage against the original test surface.
- GREEN: focused scheduler, orchestrator, and store tests pass for merged-PR keyless skips/failures,
  existing-trigger key retention, and failed task-run preservation.
- GREEN: `cd apps/backend && go test ./internal/automation ./internal/orchestrator`.
- No production dedup logic needed changing; this task remains regression coverage only.

## Output contract

Report which paths were already correct, RED/GREEN evidence for any discovered mismatch, focused test
results, and task/plan status updates.
