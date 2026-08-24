---
id: "17-final-lifecycle-transition-coverage"
title: "Final lifecycle transition coverage"
status: done
wave: 8
depends_on:
  - "16-ci-error-precedence"
plan: "plan.md"
spec: "../../specs/ui/requirements/ci-pr-automation.md"
---

# Task 17: Final lifecycle transition coverage

## Acceptance

- A task deleted after initial automation validation but before lifecycle queue
  insertion creates no queued message or lifecycle checkpoint.
- A lifecycle edge observed with no promptable session persists `last_error`
  without a checkpoint; after an existing task session becomes promptable, the
  next evaluation accepts exactly one coalesced event, records the checkpoint,
  and clears/broadcasts the error.

## Verification

```bash
cd apps/backend && go test -race ./internal/orchestrator -count=1
```

## Files likely touched

- lifecycle tests under `apps/backend/internal/orchestrator/`
- This task file

## Constraints

- Test-only unless a deterministic regression exposes a production defect.
- Use real repository/message queue/store boundaries where practical and no
  sleeps.
- Preserve queue-first delivery and archive/delete behavior.
- Do not edit `plan.md`, commit, or spawn subagents.

## Output contract

- Set this task to `done` only after both transitions and the race run pass.
- Report exact tests, any production bug, and residual risk.
