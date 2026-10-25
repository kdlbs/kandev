---
id: "01-persist-pr-auto-fix-attempt-state"
title: "Persist PR auto-fix attempt state"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CI-PR-AUTOMATION-001
acceptance_criteria:
  - AC-UI-CI-PR-AUTOMATION-001.9
  - AC-UI-CI-PR-AUTOMATION-001.10
  - AC-UI-CI-PR-AUTOMATION-001.11
  - AC-UI-CI-PR-AUTOMATION-001.12
  - AC-UI-CI-PR-AUTOMATION-001.13
  - AC-UI-CI-PR-AUTOMATION-001.14
system_design:
  - ../../specs/ui/system-design/ci-pr-automation-01.md
  - ../../specs/ui/system-design/ci-pr-automation-02.md
---

# Task 01: Persist PR Auto-Fix Attempt State

## Summary

Add the durable state and compare-and-set operations needed to distinguish an
in-flight auto-fix prompt from an acknowledged or retryable attempt.

## In scope

- Add replayable SQLite/Postgres-compatible columns for attempt state and
  identity to `github_task_ci_pr_state`.
- Extend GitHub automation models and API projections with non-sensitive status
  fields needed by the existing error and round surfaces.
- Add atomic store/service methods to record dispatch, bind a turn, accept the
  first matching outcome, mark a matching terminal turn retryable, reconcile a
  progress deadline, and reset all new fields on disable/re-enable.
- Include PR head and normalized check execution timestamps in provider
  generation.
- Add startup reconciliation for queued and terminal-turn states.

## Out of scope

- MCP registration and prompt instructions.
- Calling the store from agent lifecycle handlers.
- GitLab persistence changes.
- New UI components.

## Acceptance

- Every state mutation is conditional on the current task, repository, PR,
  session, turn, and signature where applicable.
- Replayed migrations preserve existing checkpoints and default legacy rows to
  a safe acknowledged state, while new attempts use the explicit lifecycle.
- Restart reconciliation preserves a live queued attempt and makes a terminal
  undispositioned running attempt retryable.

## Verification

```bash
make -C apps/backend test ARGS='./internal/github/...'
make -C apps/backend lint
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/service_ci_automation.go`
- `apps/backend/internal/github/store_ci_automation_test.go`
- `apps/backend/internal/github/service_ci_automation_test.go`
- `apps/backend/internal/github/store_postgres_test.go`

## Dependencies

None.

## Risks

- Legacy rows have no turn identity. Migration must not reinterpret already
  acknowledged feedback as an open attempt.
- Conditional updates must use driver-portable SQL and remain replayable.

## Parallelism

`sequential`

## Results

Implemented durable attempt lifecycle columns, models, service methods, provider
generation tracking, reset behavior, and startup reconciliation. Added store
tests for migrations, compare-and-set identity, lifecycle transitions, progress
deadlines, outcome handling, and reset behavior.
