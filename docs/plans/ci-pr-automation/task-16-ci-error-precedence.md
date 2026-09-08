---
id: "16-ci-error-precedence"
title: "Preserve CI error after lifecycle success"
status: done
wave: 8
depends_on:
  - "15-final-review-remediation"
plan: "plan.md"
spec: "../../specs/ui/requirements/ci-pr-automation.md"
---

# Task 16: Preserve CI error after lifecycle success

## Acceptance

- Refresh/freshness CI errors are persisted only after independent lifecycle
  evaluation completes, so a successful lifecycle checkpoint cannot clear the
  error from the shared per-PR `last_error`.
- The final CI error state is published after persistence for open clients.
- Store-backed handler coverage proves final `last_error` remains visible for
  PR refresh failure when lifecycle delivery succeeds, and for stale
  auto-merge freshness after the independent lifecycle evaluator runs. A stale
  merge-ready PR cannot also deliver a review-request lifecycle event in the
  same pass because merge readiness rejects pending reviews.

## Verification

```bash
cd apps/backend && go test -race ./internal/github ./internal/orchestrator
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation_test.go`
- store-backed lifecycle test fixture files if needed
- This task file

## Constraints

- Use TDD.
- Keep the shared error field and independent boolean behavior; do not add
  schema or a parallel error store.
- Do not weaken queue-first lifecycle checkpointing or error broadcasts.
- Do not edit `plan.md`, commit, or spawn subagents.

## Output contract

- Set this task to `done` only after both store-backed regressions and targeted
  race verification pass.
- Report final error precedence, files, exact tests, blockers, and residuals.
