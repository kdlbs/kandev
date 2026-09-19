---
id: "15-final-review-remediation"
title: "Final lifecycle review remediation"
status: done
wave: 8
depends_on:
  - "14-lifecycle-delivery-review-remediation"
plan: "plan.md"
spec: "../../specs/ui/requirements/ci-pr-automation.md"
---

# Task 15: Final lifecycle review remediation

## Acceptance

- PR refresh failure and stale auto-merge freshness disable only the CI action
  for that pass; lifecycle evaluation still runs independently with the
  available linked-PR fact and records the CI error.
- Lifecycle dispatcher tests prove duplicate busy observations keep one stable
  coalesced entry, distinct PR/events stay distinct and ordered, and
  `WAITING_FOR_INPUT`/`IDLE` sessions enqueue then drain immediately.
- A handler test proves queue acceptance occurs before the lifecycle checkpoint
  is recorded.
- `UpdateTaskCIOptions` is back within the backend function-size limit using
  transaction-local helpers without weakening the single atomic transaction.

## Verification

```bash
cd apps/backend && go test -race ./internal/github ./internal/task/repository/... ./internal/orchestrator
```

## Files likely touched

- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/store_ci_automation_test.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation_test.go`
- lifecycle queue tests under `apps/backend/internal/orchestrator/`
- This task file

## Constraints

- Use TDD and preserve Tasks 13/14 behavior.
- Lifecycle errors/checkpoints remain on the existing per-PR store and durable
  queue; do not add a poller, queue, schema, session, or workflow transition.
- Keep refresh/freshness errors user-visible without using them to gate
  independent lifecycle options.
- Do not edit `plan.md`, commit, or spawn subagents.

## Output contract

- Set this task to `done` only after all four acceptance items and the targeted
  race run pass.
- Report files, exact tests, divergence, blockers, and residual risk.
