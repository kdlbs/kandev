---
id: "02-explain-unmatched-reports"
title: "Explain unmatched outcome reports"
status: done
wave: 2
depends_on:
  - "01-scope-reporting-guidance"
plan: "plan.md"
requirements:
  - REQ-UI-CI-PR-AUTOMATION-001
acceptance_criteria:
  - AC-UI-CI-PR-AUTOMATION-001.10
  - AC-UI-CI-PR-AUTOMATION-001.17
system_design:
  - ../../specs/ui/system-design/ci-pr-automation-02.md
---

# Task 02: Explain unmatched outcome reports

## Summary

Return a scope explanation when the backend cannot match an unresolved attempt.
Prove that rejected reports leave automation settings and attempt state unchanged.

## In scope

- A separate error mapping for `ErrTaskCIAutoFixAttemptNotFound`.
- Handler and MCP-client coverage for rejection guidance and `VALIDATION_ERROR`.
- Real-store regression coverage for the context matrix in the plan.

## Out of scope

- Successful no-op responses, synthetic attempts, schema changes, and automatic retries.
- Changing the conditional store write or caller identity contract.

## Acceptance

- The error explains scope without asserting that auto-fix is disabled.
- Invalid reports create no attempt, consume no round, and change no settings.
- Valid bound reports retain all three outcomes and reject duplicate or foreign-turn reports.

## TDD sequence

First add `TestReportPRAutoFixOutcomeExplainsUnmatchedTurn` and record its expected failure.
Add the state matrix through the real temporary store.
Change only the missing-attempt error mapping, then run all commands below.
Extend the MCP client test to prove the explanation survives the tool transport.

## Verification

Run from the repository root:

```bash
(cd apps/backend && rtk go test ./internal/mcp/handlers -run 'Test.*Report.*PRAutoFix' -count=1)
(cd apps/backend && rtk go test ./internal/mcp/server -run 'TestReportPRAutoFixOutcome|TestTaskPRAutomationTools' -count=1)
(cd apps/backend && rtk go test ./internal/github -run 'TestStorePRAutoFixOutcome|TestStoreTaskCIAutoFixAttempt' -count=1)
rtk python3 scripts/lint-spec-files.test.py
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

## Files likely touched

- `apps/backend/internal/mcp/handlers/task_pr_automation.go`
- `apps/backend/internal/mcp/handlers/task_pr_automation_outcome_scope_test.go` (new)
- `apps/backend/internal/mcp/server/pr_auto_fix_outcome_scope_test.go`
- `apps/backend/internal/github/store_ci_outcome_scope_test.go` (new)

## Dependencies

Task 01 owns the shared new MCP test file and the guidance terminology.

## Risks

The sentinel covers missing, stale, and completed attempts.
The explanation must not recommend changing settings to repair a reporting error.
State assertions must compare rows before and after rejected calls, including absent rows.

## Parallelism

`sequential`

## Inputs

- Criteria `.10` and `.17`, and design section `Outcome reporting scope`.
- `handleReportTaskPRAutoFixOutcome`, `ReportTaskCIAutoFixOutcome`, and existing handler tests.
- `TestStoreTaskCIAutoFixAttemptLifecycleUsesExactIdentity`.
- [Incident evidence and test matrix](plan.md).

## Results

Done. Unmatched auto-fix outcome reports now keep `VALIDATION_ERROR` while
returning a scope explanation that covers missing, stale, and completed turns.
The explanation directs ordinary work to continue without retrying the report
or changing automation settings. The MCP transport and temporary-SQLite tests
prove that the existing task/session/turn guard and all three valid outcomes
remain intact.

- `rtk go test ./internal/mcp/handlers -run 'Test.*Report.*PRAutoFix' -count=1`: passed, 7 tests.
- `rtk go test ./internal/mcp/server -run 'TestReportPRAutoFixOutcome|TestTaskPRAutomationTools' -count=1`: passed, 5 tests.
- `rtk go test ./internal/github -run 'TestStorePRAutoFixOutcomeOrdinaryTurnHasNoSideEffects|TestStoreTaskCIAutoFixAttempt' -count=1`: passed, 9 tests.
- `rtk python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `rtk python3 scripts/lint-spec-files.py --all`: passed.
- `rtk git diff --check`: passed.
