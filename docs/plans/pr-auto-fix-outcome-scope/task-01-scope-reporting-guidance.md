---
id: "01-scope-reporting-guidance"
title: "Scope agent reporting guidance"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CI-PR-AUTOMATION-001
acceptance_criteria:
  - AC-UI-CI-PR-AUTOMATION-001.16
system_design:
  - ../../specs/ui/system-design/ci-pr-automation-02.md
---

# Task 01: Scope agent reporting guidance

## Summary

Explain the current-turn reporting requirement at discovery and dispatch.
Keep the GitHub task catalog stable across manual and automatic turns.

## In scope

- Tool description exclusions for manual fixup, sibling review, and historical instructions.
- Explicit expiry of the immutable protocol at the end of its dispatched turn.
- Structured, passthrough, and customized-prompt coverage.

## Out of scope

- Catalog filtering, new tool arguments, setting changes, and retry logic.

## Acceptance

- Discovery explains that availability or enabled settings do not establish a reporting obligation.
- A valid current auto-fix dispatch still requires exactly one outcome.
- Prior-turn protocol text does not instruct later manual turns to report.

## TDD sequence

Add the two guidance tests named in the plan. Run them and record the expected failures.
Then change the two instruction surfaces and rerun the tests.
Retain existing provider membership and protocol visibility coverage.

## Verification

Run from the repository root:

```bash
(cd apps/backend && rtk go test ./internal/mcp/server -run 'TestReportPRAutoFixOutcome|TestTaskPRAutomationTools|TestServerModeTask_ProviderMembership' -count=1)
(cd apps/backend && rtk go test ./internal/orchestrator -run 'TestCIAutomationOutcomeProtocol' -count=1)
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

## Files likely touched

- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/pr_auto_fix_outcome_scope_test.go` (new)
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation.go`
- `apps/backend/internal/orchestrator/ci_automation_outcome_scope_test.go` (new)

## Dependencies

None.

## Risks

Overly broad exclusions can suppress legitimate automatic reports.
Assert the positive reporting instruction as well as its scope.

## Parallelism

`sequential`

## Inputs

- Requirement `.16` and design section `Outcome reporting scope`.
- `TestCIAutomationOutcomeProtocolVisibility` and `newTaskModeServer`.
- [Incident evidence and test matrix](plan.md).

## Results

Done. The GitHub outcome tool description now scopes reporting to the current
Kandev-dispatched auto-fix turn and excludes manual fixup, sibling review, and
historical instructions. The immutable structured and passthrough dispatch
protocols state the same scope and expiry. Existing provider membership and
positive outcome visibility remain covered.

- `rtk go test ./internal/mcp/server -run 'TestReportPRAutoFixOutcome|TestTaskPRAutomationTools|TestServerModeTask_ProviderMembership' -count=1`: passed, 10 tests.
- `rtk go test ./internal/orchestrator -run 'TestCIAutomationOutcomeProtocol' -count=1`: passed, 4 tests.
- `rtk python3 scripts/lint-spec-files.py --all`: passed.
- `rtk git diff --check`: passed.
