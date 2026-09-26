---
id: "13-lifecycle-state-review-remediation"
title: "Lifecycle state review remediation"
status: done
wave: 8
depends_on:
  - "09-backend-lifecycle-reliability"
plan: "plan.md"
spec: "../../specs/ui/requirements/ci-pr-automation.md"
---

# Task 13: Lifecycle state review remediation

## Acceptance

- Auto-fix can block auto-merge but never skips an independently enabled
  review-requested, merged, or closed lifecycle evaluation.
- Re-enabling merged or closed resets only that terminal event's matching
  checkpoint; it prompts once for an already-terminal PR and does not disturb
  the other terminal option's accepted checkpoint or watch cleanup.
- Any authenticated reviewer-login change, including a repeated
  `prompt_on_review_requested: true` PATCH/MCP call while already enabled,
  atomically resets every linked PR review baseline without changing CI or
  terminal checkpoints.

## Verification

```bash
cd apps/backend && go test -race ./internal/github ./internal/orchestrator
```

## Files likely touched

- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/store_ci_automation_test.go`
- `apps/backend/internal/github/service_ci_automation.go`
- `apps/backend/internal/github/service_ci_automation_test.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation_test.go`
- This task file

## Constraints

- Use TDD.
- Keep the five booleans independent and the reviewer update atomic.
- Do not add schema, polling, session, queue, UI, or workflow-step behavior.
- Preserve accepted checkpoints for unrelated automation events.
- Do not edit `plan.md`, commit, or spawn subagents.

## Output contract

- Set this task to `done` only after all three blocker regressions and the
  targeted race run pass.
- Report files, exact tests, divergence, blockers, and residual risk.
