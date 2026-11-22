---
id: 02-graphql-workflow-phase
title: Read monitored workflow evidence through GraphQL
status: complete
wave: 2
depends_on:
  - 01-job-evidence-unknown
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-002
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003
  - REQ-INTEGRATIONS-GITHUB-PR-POLLING-002
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.1
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.5
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.6
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.9
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.10
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-002.1
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-002.4
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.1
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-002.4
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-002.5
system_design:
  - ../../specs/integrations/system-design/github-workflow-attention.md
  - ../../specs/integrations/system-design/github-pr-discovery-health.md
---

# Task 02: Read monitored workflow evidence through GraphQL

## Summary

Replace the batched path's concurrent REST Actions enrichment with a third
workflow phase. Two GraphQL steps run after status apply and classify every
open known PR, with no per-head REST reads. Local deferral leaves stored
evidence untouched and reports the PRs as unfinished workflow work.

## In scope

- `graphql_workflow_suites.go` (new):
  - The step-1 builder, at 20 PRs per request, grouped by repository, with
    `checkSuites(first: 100)` and `workflowRun` fields exactly as in the design.
  - The step-2 builder: `nodes(ids:)` chunks of 50 with a LATEST `checkRuns` `totalCount`.
  - Decoders, plus conversion of Actions suites (non-null `workflowRun`) into
    `WorkflowRun`, with head identity from the PR and attempt 0.
- `service_pr_workflow_phase.go` (new):
  - The phase runner, keyed by credential scope, repository, PR number, and stored head.
  - The `batched-workflow:` singleflight and progress key.
  - Step-2 selection: only `COMPLETED` / `ACTION_REQUIRED` `pull_request`
    suites on fork PRs need counts.
  - Classification through `classifyWorkflowAttentionWithJobs`, with a reader
    over step-2 counts.
  - A head mismatch or `hasNextPage` yields `unknown`.
- A head-guarded store write of `workflow_attention` through
  `resolveTaskPRWorkflowAttention`. Publish `github.task_pr.updated` only on a
  semantic change (`workflowAttentionSemanticEqual`). Add a PostgreSQL-gated
  test if the store method's SQL is dialect-sensitive.
- `service_pr_watch_batched.go`:
  - Stop calling `enrichBatchedWorkflowAttention` from
    `fetchBatchedNumberedPhase` and `fetchBatchedSearchingPhase`.
  - Run the workflow phase after both phases apply, for open, non-terminal,
    non-detached rows.
  - Remove the dead enrichment helpers.
- `prWatchSyncOutcome` carries unfinished workflow work and its typed
  `AdmissionDeferredError`. Passive per-task sync returns the deferral.
- On an unsupported-GraphQL or invalid-query classification, use the REST
  collector for that workspace serially, stopping at the first local deferral.
- Update the "Batched phase progress and continuation" section of
  `apps/backend/internal/github/AGENTS.md` for the third phase.

## Out of scope

- Monitor timer resumption of workflow work (Task 03).
- Admission policy, UI, projection, schema columns, single-PR REST paths, and REST cache TTLs.

## Acceptance

1. Thirty open fork PRs with jobless action-required suites all store
   `approval_required` after one batched sync. The sync issues zero REST
   `ListWorkflowRuns`/`ListWorkflowRunJobs` calls, two step-1 queries, and one
   step-2 query. A completed non-gated head stores `none`.
2. A step-1 or step-2 `AdmissionDeferredError` leaves every affected row
   byte-identical: not stale, no event. Those PRs are returned as unfinished
   workflow work, and status writes from the earlier phases still publish.
3. These cases store `unknown`, preserving prior same-head evidence as stale:
   a head that changed between apply and step 1, a suite page with
   `hasNextPage`, and a missing step-2 count. Unsupported GraphQL uses the
   serial REST collector.

## TDD and verification

Run from the repository root. Write
`TestWorkflowPhase_ClassifiesManyHeadsWithoutRESTActions` first and record its
failure. Current code issues REST Actions calls per head; under real
non-blocking `RateAdmission` most rows stay `unknown`.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/github -run '^TestWorkflowPhase_ClassifiesManyHeadsWithoutRESTActions$' -count=1 -v)
```

Add to `service_pr_workflow_phase_test.go`:

- `TestWorkflowPhase_ClassifiesManyHeadsWithoutRESTActions`: fake
  `GraphQLExecutor` with call counts, fake client counting REST Actions calls,
  and a mixed set of gated forks, a same-repository `action_required` run with
  jobs, and a completed success head.
- `TestWorkflowPhase_DeferralLeavesStoredObservationAndReturnsPending`.
- `TestWorkflowPhase_HeadChangeAndPageOverflowAreUnknown`.
- `TestWorkflowPhase_UnsupportedGraphQLUsesSerialREST`.

Add decoder and builder tests to `graphql_workflow_suites_test.go`. Use fixtures
shaped like the 2026-10-29 probe of PR #4720: 19 suites, 10 jobless
`ACTION_REQUIRED` `pull_request` suites, and one suite without `workflowRun`.

After the correction:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/github -run 'TestWorkflowPhase_|TestWorkflowAttention|TestGraphQL|Test.*Batched|TestPRStatusProgress_|TestPRMonitorContinuation_|TestTaskPRWorkflowAttention' -count=1 -timeout=8m)
bash scripts/lint-go-changed $(git diff --name-only -- 'apps/backend/*.go')
git diff --check
```

## Files likely touched

- `apps/backend/internal/github/graphql_workflow_suites.go` (new)
- `apps/backend/internal/github/graphql_workflow_suites_test.go` (new)
- `apps/backend/internal/github/service_pr_workflow_phase.go` (new)
- `apps/backend/internal/github/service_pr_workflow_phase_test.go` (new)
- `apps/backend/internal/github/service_pr_watch_batched.go`
- `apps/backend/internal/github/service_pr_watch.go` (outcome plumbing only)
- `apps/backend/internal/github/store.go` (head-guarded attention write)
- `apps/backend/internal/github/AGENTS.md`

## Dependencies

Task 01 supplies the incomplete-job-evidence rule that the step-2 reader relies on.

## Inputs

- [Monitored workflow phase design](../../specs/integrations/system-design/github-workflow-attention.md#monitored-workflow-phase)
- [Independent PR status publication](../../specs/integrations/system-design/github-pr-discovery-health.md#independent-pr-status-publication)
- `graphql.go` (`buildBatchedPRQuery`, `chunkRefs`) and `batch_progress.go` as builder and progress examples.

## Results

Implemented. `graphql_workflow_suites.go` builds and decodes the dedicated
20-PR suite query and the 50-suite job-count query. `service_pr_workflow_phase.go`
runs the phase after both status phases apply, retains step-1 evidence and
step-2 counts through a `batched-workflow:` progress key, falls back to the
serial REST collector for unsupported or rejected GraphQL, and writes through
the new head-guarded `Store.UpdateTaskPRWorkflowAttention`. A changed head, an
overflowing suite page, or a missing job count is unavailable evidence. The
batched status phases no longer issue concurrent REST Actions reads.

Tests cover thirty heads without REST Actions reads, deferral leaving stored
rows untouched and returning pending work, head/overflow unknown handling, and
the REST fallback. The full `internal/github` package passed.

Review follow-ups are also implemented:

- A path-bearing GraphQL error now marks only the aliases or suite nodes it
  names unavailable. The resolved siblings keep their evidence; an error
  without a usable path still fails the whole chunk. Numeric `path` segments
  from `nodes(ids:)` are decoded correctly.
- Each converted suite carries the PR association, so `pull_request_target`,
  `push`, and `workflow_run` action-required suites on the PR head match the PR
  instead of being dropped as unassociated runs.
- GraphQL rate-limit boundaries from `rateLimit.resetAt` are retained with
  `withPRDiscoveryRetryAt` on the returned error.
- Empty-head rows are skipped before the phase, and the store rejects an empty
  expected head, so every `workflow_attention` write stays head-scoped.
- The singleflight now shares only the evidence fetch. Every caller applies it
  to its own task rows and builds its own unfinished watches, so two tasks that
  watch the same PR both receive the observation.
- The REST fallback retains the fetched PR identity and a job-read deferral
  across wakes, so non-blocking admission can advance instead of repeating
  `GetPR` forever.

Verification after the review follow-ups:

- `go test -trimpath -tags fts5 ./internal/github -count=1` passed.
- `go test -trimpath -tags fts5 -race ./internal/github -run 'TestWorkflowPhase_|TestWorkflowAttention|TestGraphQL|Test.*Batched|TestPRStatusProgress_|TestPRMonitorContinuation_|TestTaskPRWorkflowAttention' -count=1 -timeout=8m`
  passed.
- `bash scripts/lint-go-changed` reported zero issues.
