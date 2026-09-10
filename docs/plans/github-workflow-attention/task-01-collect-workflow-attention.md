---
id: "01-collect-workflow-attention"
title: "Collect workflow attention"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.1
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.2
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.4
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.5
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.6
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.7
system_design:
  - ../../specs/integrations/system-design/github-workflow-attention.md
---

# Task 01: Collect Workflow Attention

## Summary

Collect current-head workflow evidence with the workspace GitHub client.
Persist the observation through every status writer without manufacturing check failures.

## In scope

- Add the workflow-run and candidate-job readers, shared classifier, and matching client doubles.
- Enrich REST status, feedback, numbered watches, and branch-discovered batches before cache publication. Keep the unwatched lifecycle sweep free of new Actions reads so active watches do not incur an additional workflow API sweep.
- Add the JSON observation column and cover scan, update, restore, migration, and workspace events.
- Preserve compact task-status projections and update their bounded attention category where necessary.
- Prove approval-only observations cannot start CI repair or automatic merging.
- Extend the mock provider/controller so browser fixtures can represent a jobless approval gate.

## Out of scope

- Rendered UI and locale changes.
- New permission grants, workflow mutations, and independent polling infrastructure.

## Acceptance

1. A regression named `TestWorkflowAttention_JoblessForkApproval` fails before the change and passes after collection.
2. REST, batched sync, feedback, reload, and multi-PR storage agree on head-scoped evidence, including stale and superseded observations.
3. Mixed real failures retain their counts and repair behavior. Approval-only evidence consumes no repair round and never satisfies merge readiness.

## Verification

Run from the repository root. Use TDD for the new named regressions.

```bash
(cd apps/backend && go test ./internal/github -count=1)
(cd apps/backend && go test ./internal/orchestrator -run 'GitHub|Github|PRCI|PRCIAutomation' -count=1)
python3 scripts/lint-spec-files.py --all
git diff --check
```

The orchestrator command must discover every changed automation test. Add a specific test name if its existing name differs.

## Files likely touched

- `apps/backend/internal/github/client.go`, `models.go`, `gh_client.go`, `pat_client.go`
- New `apps/backend/internal/github/workflow_attention.go` and `workflow_attention_test.go`
- `apps/backend/internal/github/client_helpers.go`, `service_pr_watch_batched.go`, `service_pr_watch.go`, `service_pr_feedback_sync.go`
- GitHub `store*.go`, matching migration tests, `mock_client.go`, `noop_client.go`, `mock_controller.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation.go` and its test file
- The existing bounded task-status projection where it derives GitHub attention

## Dependencies

None.

## Risks

The collector must distinguish permission failure, no runs, and no jobs.
Current-head matching needs head repository identity when GitHub omits PR associations.
Extra reads must stay inside existing credential scopes and context budgets.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-workflow-attention.md)
- [Design](../../specs/integrations/system-design/github-workflow-attention.md)
- Existing `newPRStatus`, `persistPRFeedbackState`, and `SyncTaskPR` paths
- Existing `store_taskpr_schema_drift_test.go` and `store_merge_queue_recovery_test.go` patterns

## Results

Implemented and verified on 2026-09-10. The GitHub clients now collect current-head workflow runs and jobs, classify jobless fork approvals separately from ordinary checks, and persist the bounded observation through REST, feedback, batched, watch, restore, and mock-provider paths. The unwatched lifecycle sweep preserves same-head stored evidence and clears it on a new head without issuing new Actions reads. Approval-only evidence does not count as a failed check or satisfy CI automation readiness.

Validation passed:

- `go test ./internal/github -count=1`: 1,741 tests passed.
- `go test ./internal/orchestrator -run 'GitHub|Github|PRCI|PRCIAutomation' -count=1`: 51 tests passed.
- `python3 scripts/lint-spec-files.py --all`.
- `git diff --check`.

PR-fixup validation also passed `go test -race ./internal/github -count=1` (1,741 tests) and `make -C apps/backend lint` with zero issues. The unwatched lifecycle and mock workflow mutation regressions are covered by the added tests.

The exact-head review remediation also covers the actual REST `pull_requests[].head.repo` shape, including
matching and mismatching repository identities and a newer associated success superseding an older unassociated
approval. The final focused workflow-attention tests and backend lint passed after this change.
