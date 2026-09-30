---
id: "01-select-current-check-executions"
title: "Select current provider check executions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001.1
  - AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001.2
  - AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001.3
  - AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001.4
  - AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001.5
  - AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001.6
  - AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001.7
  - AC-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001.8
system_design:
  - ../../specs/integrations/system-design/github-pr-check-results.md
---

# Task 01: Select Current Provider Check Executions

## Summary

Preserve check identity and remove superseded Actions suites before deriving
status or feedback. This task delivers the selected-check contract consumed by
Task 02, without changing cancellation classification yet.

## In scope

- Shared raw/domain identity fields, optional web check/feedback types, and mock seed fields.
- Pure selector in `check_selection.go`, with matching and deterministic ordering.
- GH CLI and PAT pagination/conversion, suite-aware merging, and safe legacy fallback.
- One reused workflow snapshot for selection and attention per status/feedback read.
- Current-head/cache scope and explicit refresh behavior.
- Selected snapshot state, including active workflows with no concrete jobs.
- Service wiring before reducers, persistence, or automation feedback publication.
- Mock provider behavior that exercises the same selector.

## Out of scope

- Cancellation bucket/reducer/prompt changes owned by Task 02.
- New polling/cache infrastructure, provider mutations, or database migrations.
- A per-job Actions request for every selected workflow.
- Changes to workflow approval classification or native GraphQL rollup semantics.

## Acceptance

1. The suite regression fails before the fix because cancelled dependent jobs
   remain despite a verified replacement run with no dependent jobs yet.
2. Selection preserves independent failures, queued newer checks, partial-rerun
   successes, legacy input, and unavailable-evidence behavior.
3. Real clients and mock services use the same current-head selection contract,
   without duplicate Actions reads or cross-credential reuse.

## TDD and test design

Start with `TestSelectCurrentPRChecksSupersededSuiteWithoutReplacementJobs` in
`apps/backend/internal/github/check_selection_test.go`.
Use the IDs, equal creation times, unassociated target event, source repository,
and branch recorded in the [plan](plan.md#confirmed-defect).
Provide old deploy/description checks and a replacement packaging check only.
Assert the old suite is absent and the running check remains.
The RED failure must concern retained superseded checks, not a missing selector.

Add the other selector cases listed in the plan. Extend both client tests to
prove identity survives page two. Include status contexts and unrelated same-name
check applications. Service tests must use raw provider observations.

## Verification

Run from the repository root. Install dependencies once in a fresh worktree
before the first pnpm command.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/github -count=1)
(cd apps/backend && go test -race ./internal/github -run '^TestSelectCurrentPRChecks' -count=1)
(cd apps/web && pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Task 02 owns the rendered verification of the selected-check contract.
Record RED and GREEN evidence, actual command results, and changed test names.
Update both this work order and the plan after completion.

## Files likely touched

- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/gh_client.go`
- `apps/backend/internal/github/pat_client.go`
- `apps/backend/internal/github/client_helpers.go`
- New `apps/backend/internal/github/check_selection.go` and `check_selection_test.go`
- `apps/backend/internal/github/workflow_attention.go`
- `apps/backend/internal/github/workflow_attention_cache.go`
- `apps/backend/internal/github/service_pr.go`
- `apps/backend/internal/github/service_pr_watch.go`
- `apps/backend/internal/github/service_pr_watch_batched.go`
- `apps/backend/internal/github/service_pr_feedback_sync.go`
- `apps/backend/internal/github/mock_client.go` and `mock_controller.go`
- Client/helper/feedback/cache tests beside these files
- `apps/web/lib/types/github.ts`
- `apps/web/e2e/helpers/api-client.ts`

## Dependencies

None. Read the investigation evidence in the plan before implementation.

## Risks

- The old and new example runs have identical timestamps. A time-only selector fails.
- A suite can contain observed jobs from a partial rerun. Do not remove valid
  successes based on absent current-attempt jobs.
- `ListCheckRuns` currently reduces by name before PR context is available.
  Preserve identity before that irreversible reduction.
- Existing attention context hooks expose classified observations. Reuse their
  reader boundary without creating a second cache or broadening approval rules.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-pr-check-results.md), REQ-001 and its criteria.
- [System design](../../specs/integrations/system-design/github-pr-check-results.md), Provider identity through Collection and refresh.
- `workflowRunNewer`, `workflowRunGroupKey`, and repository matching in `workflow_attention.go`.
- Existing client pagination and `service_pr_feedback_sync_test.go` patterns.
- Existing [workflow-attention work order](../github-workflow-attention/task-01-collect-workflow-attention.md), as completed neighboring context.

## Results

Completed. The initial integration regression failed because feedback still
returned the cancelled `deploy-fork` and `update-description-fork` checks while
the replacement run contained only its in-progress packaging check.

Implemented optional check-run/application/suite identity, workflow-suite
identity, identity-preserving transport merging, PR-head-scoped workflow matching,
current-suite selection before job reduction, the unassociated
`pull_request_target` fallback, and shared cached Actions observations for
feedback, status, and workflow attention. Active selected workflows with no jobs
now publish `pending` with zero fabricated check counts. GH CLI, PAT, mock, and
web transport types carry the identity.

Verification:

- `(cd apps/backend && go test ./internal/github -count=1)`: passed.
- `(cd apps/backend && go test -race ./internal/github -run '^TestSelectCurrentPRChecks' -count=1)`: passed.
- `(cd apps/web && pnpm run typecheck)`: passed.
- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.

Task 02 owns cancellation reductions and rendered desktop/phone verification.

### Review remediation

Follow-up review found that same-name GitHub Actions checks from different
check suites could collapse to the newest check ID when no workflow run could
be joined. This hid a real failure if the workflow snapshot was absent or its
read failed.

Added `TestGetPRFeedbackPreservesUnmatchedActionsSuitesWhenWorkflowEvidenceUnavailable`.
Its red phase reproduced the issue for App ID plus slug and slug-only Actions
identity, with both missing and failed workflow evidence. It asserts that both
suites remain, feedback state is `failure`, and `HasIssues` stays true. The
selector now partitions unmatched Actions by suite and producer, and retains
suite partitions when producer identity is unavailable. Third-party
application-level selection and legacy name handling remain unchanged.

Validation after the fix:

- `go test ./internal/github -run '^(TestGetPRFeedbackPreservesUnmatchedActionsSuitesWhenWorkflowEvidenceUnavailable|TestSelectCurrentPRChecksSupersededSuiteWithoutReplacementJobs|TestSelectCurrentPRChecksQueuedReplacementAndPartialRerun)$' -count=1`: passed.
- `go test ./internal/github -count=1`: passed.
- `go test ./internal/orchestrator -run 'Test.*(CIAutomation|CIFix|CIMerge)' -count=1`: passed.
