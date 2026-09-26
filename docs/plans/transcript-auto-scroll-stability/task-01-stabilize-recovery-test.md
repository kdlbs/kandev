---
id: "01-stabilize-recovery-test"
title: "Stabilize clarification recovery test"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/transcript-auto-scroll.md"
---

# Task 01: Stabilize clarification recovery test

## Acceptance

- The regression waits for recovery completion after retry acceptance without a
  zero-wait channel receive.
- The test still proves recovery completes before the intentionally blocked
  prompt finishes.

## Verification

```bash
cd apps/backend && go test -race -run '^TestClarificationRecovery_ReleasesGuardAfterRetryDispatch$' -count=30 ./internal/orchestrator
```

## Files likely touched

- `apps/backend/internal/orchestrator/task_operations_test.go`

## Dependencies

None.

## Parallelism

Sequential.

## Output contract

Report the synchronization change, test result, files changed, and updated task
and plan status.
