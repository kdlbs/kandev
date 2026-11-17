---
id: "02-summary-retries"
title: "Stop missing summary retries"
status: completed
wave: 2
depends_on:
  - 01-terminal-settlement
plan: "plan.md"
requirements:
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-007
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-003.6
  - AC-TASKS-TURN-CHANGES-007.1
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
---

# Task 02: Stop missing summary retries

## Summary

Finish summary processing when there is no captured turn-change set, while
retaining retries for actual transient publication or processing failures.

## In scope

- Test the backendapp processor with a missing set, a present set and a
  transient publisher failure using the real error contract.
- Ensure missing-set processing does not schedule recurring warnings, and
  actual pending work can still be retried and published.

## Out of scope

- Creating historical change sets or changing capture policy or Git data.

## Acceptance

1. An absent change set is a terminal no-op and does not retry indefinitely.
2. A real publisher error remains retryable and a present summary publishes.

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/backendapp ./internal/agent/runtime/lifecycle -run 'Test.*TurnChange' -count=1)
bash scripts/lint-go-changed apps/backend/internal/backendapp/turn_changes_runtime.go
```

## Files likely touched

- `apps/backend/internal/backendapp/turn_changes_runtime.go`
- `apps/backend/internal/backendapp/turn_changes_runtime_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_turn_changes_test.go`

## Dependencies

Task 01.

## Risks

Do not suppress transient storage errors that are distinct from a missing set.

## Parallelism

sequential

## Inputs

- `docs/specs/tasks/requirements/turn-changed-files.md`
- `docs/specs/tasks/system-design/turn-changed-files.md`

## Results

Implemented the missing-set publication no-op without suppressing processing
or transient publication errors. The focused regression failed before the fix
and passes afterward, including publication succeeding on retry. Targeted
backendapp/lifecycle checks and scoped lint passed.
