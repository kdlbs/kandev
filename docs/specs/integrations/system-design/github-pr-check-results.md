---
status: current
system: integrations
created: 2026-09-30
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001
  - REQ-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-002
owners:
  - kandev
---

# GitHub Pull Request Check Results System Design

## Purpose and boundaries

The integration selects current provider checks before it derives feedback,
task status, and automation input. The web application consumes those results.
This design repairs selection and cancellation handling within the existing
GitHub integration. It does not create another source of task or merge readiness.

The design reuses the
[workflow attention collector and cache](github-workflow-attention.md),
[task PR synchronization](github-task-pr-sync-coordination.md), and existing
[automation guards](../../ui/system-design/ci-pr-automation-02.md).

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-001` | Provider identity; Current check selection; Collection and refresh; Failure behavior |
| `REQ-INTEGRATIONS-GITHUB-PR-CHECK-RESULTS-002` | Conclusion policy; Status and automation; Desktop and phone presentation |

## Components and responsibilities

| Boundary | Responsibility |
| --- | --- |
| `gh_client.go`, `pat_client.go`, `client_helpers.go` | Decode identity and preserve it through transport conversion and merging |
| New `check_selection.go` in `internal/github` | Select retained checks from PR, raw checks, and workflow evidence |
| `workflow_attention.go`, `workflow_attention_cache.go` | Share workflow reads, matching primitives, ordering, cache scope, and invalidation |
| `getPRFeedback`, `getPRStatus`, `service_pr_feedback_sync.go` | Select checks before deriving or persisting result state |
| `service_pr_watch_batched.go` and its workflow enrichment | Keep native GraphQL rollup authoritative; apply normalized detail counts only when populated |
| `service_pr_status.go`, `client_helpers.go` | Apply cancellation semantics to the existing reducers and issue predicate |
| Orchestrator `event_handlers_github_ci_automation.go` | Consume retained checks and exclude cancellation from repair deltas |
| Web `check-buckets.ts`, `pr-ci-popover.tsx`, PR details | Share selected results while preserving provider-specific presentation |

## Provider identity

Extend `ghCheckRun` and `CheckRun` with optional provider identity:
`id`, `app_id`, `app_slug`, and `check_suite_id`.
Extend `ghWorkflowRun` and `WorkflowRun` with `check_suite_id`.
The shared raw shapes serve both GH CLI and PAT clients, including App-backed PAT clients.
Retain these fields in mock fixtures and the corresponding TypeScript `CheckRun`.

The workflow reader already supplies run ID, attempt, workflow ID, event,
source repository, branch, head SHA, and creation time.
Join checks to workflow runs through check-suite identity, not an HTML job URL.
Carry optional `workflow_id`, `workflow_name`, and `workflow_run_id` on selected
checks for grouping. Include event and source identity in the internal group key.
The frontend groups known checks by application and selected workflow-run ID,
which also separates events and source identities with equal display names.
Display labels remain provider data.

GitHub documents check-suite identity on both
[check runs](https://docs.github.com/en/rest/checks/runs#list-check-runs-for-a-git-reference)
and [Actions runs](https://docs.github.com/en/rest/actions/workflow-runs#list-workflow-runs-for-a-repository).
These fields permit the join without a request for every job.

New JSON fields are optional. This repair needs no database migration.
The existing TaskPR summary fields and populated flags retain their meanings.
Add optional `checks_state` to `PRFeedback` for the status of its selected snapshot.
Use presence to distinguish an observed empty state from a legacy omitted field.
It is derived evidence, not an independent merge-readiness rule.

## Current check selection

Use one pure selector with the PR, check runs/status contexts, and the available
workflow snapshot as inputs. Do not discard suite identity before this selector.

1. Preserve distinct check-suite/application/name entries through `ListCheckRuns`.
   Keep status contexts distinguishable from check runs.
2. Match Actions runs to the current PR head. Reuse explicit PR-association
   and repository-identity conflict checks from `workflow_attention.go`.
3. For unassociated `pull_request_target` runs, require exact head SHA, source
   repository identity, and branch. The check selector adds this event-specific
   fallback without changing the attention classifier's fork-approval rule.
4. Group matched runs by workflow ID, event, source repository, and branch.
   Reuse `workflowRunGroupKey` and `workflowRunNewer` where their identity is complete.
   Order distinct runs by creation time, then run ID. Order attempts of one run
   by attempt number. Late `updated_at` changes cannot supersede newer runs.
5. For distinct runs with different suites, remove checks joined to an older
   matched suite. Remove the whole superseded suite before job-name reduction.
   The replacement suite does not need to contain every dependent job yet.
6. Within each selected logical job, choose the newest check-run ID when both
   IDs are present. Use start time only for legacy rows without comparable IDs.
   A queued newer ID can replace a completed older ID without `started_at`.
7. Within one run/suite, retain observed results for jobs absent from a partial
   rerun. Do not assume that absence means cancellation or supersession.
8. Preserve independent workflows and applications through the final merge.
   Legacy rows keep existing name-based behavior only within their unknown
   identity partition; they cannot replace a known different application/workflow.
   Preserve existing check-run/status-context shadowing for the same logical
   check. Equal names alone cannot shadow known distinct producers.

Third-party checks without a matched Actions workflow use latest-job selection
within their application. Their suite IDs alone do not establish a workflow lineage.
An unknown workflow, missing suite ID, unsupported event, or conflicting identity
does not authorize removal of that check.

## Collection and refresh

Selection and workflow attention consume one Actions snapshot per feedback or
status read. Expose the existing cached workflow reader through a narrow context
hook, following `withWorkflowAttentionCollector`; avoid a second cache or client.
Direct client tests can use the uncached reader once for the same request.

`getPRFeedback` and `getPRStatus` perform selection before `newPRStatus`,
`hasFailingChecks`, or feedback publication. Keep the same observation available
for workflow-attention classification.
Derive snapshot state from retained checks plus selected active workflows.
A selected active workflow keeps an otherwise successful or empty reduction pending,
even before any job exists. A retained failure still takes precedence.
This changes summary state without adding a check or incrementing running counts.
Carry that state through `PRFeedback.checks_state` and its persistence path;
legacy feedback without the field retains the existing reduction fallback.

Preserve native GraphQL `statusCheckRollup` in batch reads. Do not expand every
background watch into a full check scan. A subsequent populated feedback/status
read updates counts from retained checks. GraphQL and detailed reads must not
alternate failure/pending solely because of superseded cancellations.

Preserve existing Actions cache lifetimes, credential generations, singleflight,
and explicit-refresh invalidation. The selected result is scoped to the supplied
PR head. Never store it under a different head or credential namespace.
No job fan-out is required to repair distinct-run suite supersession.

## Conclusion policy

Cancellation is excluded from the three hover buckets and completed-check counts.
`groupChecksByWorkflow` omits groups with zero counted jobs.
Retained cancellations remain available to PR details with their raw conclusion.

| Observation | Hover | REST rollup/counts | Feedback/repair |
| --- | --- | --- | --- |
| Queued or running current check | Running | Pending; outside completed denominator | Wait under existing settled-check gate |
| Current `failure` or `timed_out` | Failed | Failure; completed failed count | Issue and eligible repair evidence |
| Retained `cancelled` | Omitted | Ignored; zero completed count | No failure or repair delta |
| Cancellation-only result | Existing non-success empty copy | No derived success | No cancellation-only merge or repair |
| `action_required` | Preserve existing check/attention behavior | Preserve existing attention gate | Preserve non-cancellation repair policy |
| Success, neutral, skipped, stale | Preserve existing policy | Preserve existing policy | Preserve existing policy |

Introduce a small exported Go cancellation/failure predicate if needed so
`hasFailingChecks` and the orchestrator agree on eligible terminal conclusions.
This predicate does not replace the separate pending, attention, or merge guards.
Update existing tests that currently assert cancellation equals failure.

## Status and automation

`computeOverallCheckStatus` and `countCheckResults` skip cancellations before
their existing reductions. A list containing only cancellations cannot derive
`success`. A genuine retained failure still wins over running or cancelled siblings.

Remove `cancelled` from `ciAutomationCheckConclusionNeedsFix` and ensure existing
checkpoint refresh removes resolved/superseded cancellation entries. Do not rewrite
stored historical prompts or increment repair rounds for a prompt-free refresh.
Independent review comments, conflicts, and current failures still contribute deltas.

Keep `ciAutomationReadyToMerge`, fresh provider reads, expected-head validation,
workflow-attention blockers, review gates, and GitHub mergeability authoritative.
Ignoring cancelled rows is not a replacement for those guards. A cancellation-only
REST snapshot has no success state. Required cancelled checks remain subject to
GitHub's fresh rollup and mergeability policy.

## Desktop and phone presentation

Use provider workflow identity for `WorkflowGroup` keys when available. Keep
the legacy name split as a fallback. Row IDs must distinguish equal display names
from independent workflows/events. No new overlay or check-state enum is needed.

The desktop surface remains the anchored `PRCIPopover`. The phone entry remains
the tap-open `PRStatusChipDrawer` using the existing `useTouchDrawer` branch.
Both share the same buckets, progress bar, and selected feedback.
The nearest mobile exemplar is `pr-status-chip.tsx` with
`ChangeRequestStatusDrawerContent`: fixed header, internal vertical scroller,
safe-area clearance, and existing dismissal/focus behavior.

When counted checks are absent but retained cancellations exist, use the existing
localized `github:checksNotSuccessful` empty label. Do not show “No checks have
started” or fabricate a running group. The existing details action exposes the
retained checks and their cancelled conclusions.
Normal loading and unavailable-feedback fallbacks remain in place.
Loaded feedback with an empty check array is a populated zero-count observation.
Do not fall back to inferred aggregate bucket counts merely because it is empty.
Use aggregates while feedback is absent, and keep snapshot state distinct from
the number of observed running jobs.

No new product copy is planned. Any changed copy uses `t()` and all required catalogs.
Do not change shared GitLab pipeline interpretation or touch-target geometry.

## Failure behavior

- An Actions read error preserves unclassified raw checks. It does not prove
  that a suite is superseded or that checks passed.
- Missing positive identity prevents suite removal. Real retained failures remain
  visible. Cancellation semantics still apply to raw `cancelled` conclusions.
- An incomplete check-page read fails the existing strict feedback/status read.
  It cannot publish a successful empty list.
- Cache errors do not become cached absence. Existing same-head attention
  preservation and credential-denial behavior remain effective.

## Verification boundaries

Provider transport tests cover pagination and identity. Selector tests cover
different suites, equal timestamps, late cancellation, nil start time,
independent producers, missing identity, and partial reruns.
Reducer and automation tests cover cancellation-only and mixed-state collections.
Rendered desktop/phone tests use raw mock provider data through the real feedback
path. They verify current links, counts, real failures, refresh, and reload.

## Related delivery records

- [Repair plan](../../../plans/github-pr-check-results/plan.md)
- [Completed workflow-attention package](../../../plans/github-workflow-attention/plan.md)

This repair uses existing integration ownership and provider-evidence boundaries.
The paired contract and this selection design contain its rationale; no new ADR is required.
