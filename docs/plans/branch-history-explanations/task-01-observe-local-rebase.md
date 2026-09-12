---
id: "01-observe-local-rebase"
title: "Observe local rebase evidence"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-003
acceptance_criteria:
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-003.2
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-003.3
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-003.7
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-003.8
system_design:
  - ../../specs/tasks/system-design/branch-history-explanations.md
---

# Task 01: Observe local rebase evidence

## Summary

Provide bounded, read-only local-rebase evidence through the existing executor Git path. Preserve existing relation and mutation operations.

## In scope

- Implement the typed WS, handler, agentctl client, API, and Git observation contract.
- Resolve linked-worktree reflogs and exact graph counts with conservative fallback.
- Add temporary-repository tests and transport validation tests, including cancellation and repository isolation.

## Out of scope

Automatic reconciliation, provider writes outside existing confirmed operations,
and unrelated Git or UI refactors.

## Acceptance

- Matching completed rebase evidence returns the exact identity and independent counts.
- Missing, ambiguous, stale, or bounded-out evidence returns neutral results without Git mutation.
- Authorized session routing reaches the selected executor repository. Requests cannot supply arbitrary paths or commands.

## Verification

Run from the repository root. Install workspace dependencies once before pnpm.
Add failing tests first, implement, then rerun these exact checks.

```bash
(cd apps/backend && go test ./internal/agentctl/server/process -run 'TestContributionHistory' -count=1)
(cd apps/backend && go test ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/handlers -run 'TestContributionHistory' -count=1)
(cd apps/backend && go test -race ./internal/agentctl/server/process -run 'TestContributionHistory' -count=1)
git diff --check
```

## Files likely touched

- New `apps/backend/internal/agentctl/server/process/git_contribution_history.go` and `_test.go`
- `apps/backend/internal/agentctl/server/api/git.go`, `server.go`, and focused tests
- `apps/backend/internal/agent/runtime/agentctl/git.go` and tests
- `apps/backend/internal/agent/handlers/git_handlers.go` and tests
- `apps/backend/pkg/websocket/actions.go`

## Dependencies

None.

## Risks

Unsupported reflog formats must not produce a guessed cause. A slow Git process must terminate within the observation budget.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/tasks/requirements/remote-contribution-tasks.md#amendment-branch-history-explanations)
- [System design](../../specs/tasks/system-design/branch-history-explanations.md)
- [Plan](plan.md)
- Existing contribution resolution tests and branch-scoped selection patterns.

## Results

Implemented the bounded, read-only contribution history observer and wired it
through the existing WS, agentctl, executor, and Git operation paths. Matching
linked-worktree rebases return separate task, published, and newer-base counts;
ambiguous, stale, unavailable, and bounded observations remain neutral.

Verification passed:

- `(cd apps/backend && go test ./internal/agentctl/server/process -count=1)` (full package, including expired and ambiguous reflogs, missing objects, post-rebase and conflict-resolution histories, merge ranges, 200-entry and 2000-commit bounds, and a running-process timeout)
- `(cd apps/backend && go test ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/agent/handlers -run 'TestContributionHistory' -count=1)`
- `(cd apps/backend && go test -race ./internal/agentctl/server/process -run 'TestContributionHistory' -count=1)`
- `git diff --check`
