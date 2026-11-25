---
id: 01-job-evidence-unknown
title: Treat unavailable job evidence as unknown
status: complete
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.6
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.7
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.10
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.4
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.5
system_design:
  - ../../specs/integrations/system-design/github-workflow-attention.md
---

# Task 01: Treat unavailable job evidence as unknown

## Summary

When a job read fails for any selected `action_required` run, classify the head
as `unknown` instead of choosing the generic `action_required` reason. Storage
then keeps same-head evidence as stale, and the approval badge is retained or
absent, never replaced by a false generic claim.

## In scope

- `classifyWorkflowAttentionWithJobs` in `workflow_attention.go`: on any job-read
  error, return `workflowAttentionUnknown(pr.HeadSHA)`, with no runs.
- Keep the generic reason for an `action_required` run whose jobs were read
  successfully and are non-empty, or that is not a fork `pull_request` run.
- Keep `resolveTaskPRWorkflowAttention` unchanged; it already preserves the
  prior same-head observation as stale on `unknown`.

## Out of scope

Admission, batching, GraphQL, caching TTLs, the monitor, and any UI.

## Acceptance

1. Eight jobless fork `action_required` runs where one job read returns
   `AdmissionDeferredError` produce `unknown`. A prior same-head
   `approval_required` observation stays stored as stale approval.
2. Existing classifier, cache, and selection tests stay green, including
   `TestWorkflowAttention_ActionRequiredWithJobsIsGeneric` and
   `TestWorkflowAttention_DeniedReadIsUnknown`.

## TDD and verification

Run from the repository root. Add the regression first and record its failure:
on current code it returns `action_required` with eight runs.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/github -run '^TestWorkflowAttention_UnavailableJobEvidenceIsUnknown$' -count=1 -v)
```

Add `TestWorkflowAttention_UnavailableJobEvidenceIsUnknown` to
`workflow_attention_test.go`. Use a mixed state: seven successful empty job
reads and one deferred read. Also cover a stored prior approval through
`resolveTaskPRWorkflowAttention`.

After the correction:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/github -run 'TestWorkflowAttention|TestTaskPRWorkflowAttention|TestPRStatusAndFeedbackShareOneCachedWorkflowSnapshot|TestTerminalPRFeedbackSelectsCurrentWorkflowSuite' -count=1)
bash scripts/lint-go-changed apps/backend/internal/github/workflow_attention.go apps/backend/internal/github/workflow_attention_test.go
git diff --check
```

## Files likely touched

- `apps/backend/internal/github/workflow_attention.go`
- `apps/backend/internal/github/workflow_attention_test.go`

## Dependencies

None.

## Inputs

- [Workflow attention requirements](../../specs/integrations/requirements/github-workflow-attention.md)
- [Provider evidence design](../../specs/integrations/system-design/github-workflow-attention.md#provider-evidence)

## Results

Implemented. `classifyWorkflowAttentionWithJobs` now reads job evidence only
for the completed fork `pull_request` candidates that can qualify for approval.
Any failed job read returns `unknown` for the head, so
`resolveTaskPRWorkflowAttention` preserves same-head prior evidence as stale and
never turns missing evidence into the generic `action_required` claim.

Added `TestWorkflowAttention_UnavailableJobEvidenceIsUnknown` with eight
jobless fork approval gates, seven successful empty reads, and one deferred
read. The pre-change classifier failed the regression by returning
`action_required` with eight runs.

Verification: the workflow-attention suite passed, and
`bash scripts/lint-go-changed` reported zero issues.
