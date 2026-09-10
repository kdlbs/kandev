---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001
---

# GitHub Workflow Attention System Design

## Purpose and boundaries

Workflow attention supplements check results. It does not rewrite GitHub mergeability or manufacture check runs.
The integration owns collection, storage, and interpretation. Existing shared components own presentation geometry.

## Requirement mapping

| Acceptance criteria | Design sections |
| --- | --- |
| 001.1, 001.6, 001.7 | Provider evidence |
| 001.2, 001.4 | Status and automation |
| 001.5 | Storage and recovery |
| 001.3, 001.8 | Presentation |

All criteria reference `AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION`.

## Provider evidence

Add a workflow-run reader to `Client`, `GHClient`, and `PATClient`, with matching mock and noop implementations.
Use the repository Actions runs endpoint with an exact `head_sha` and pagination.
Use the existing workspace credential and timeout policy. Never fall back to another identity after permission denial.

Keep workflow ID, run ID, attempt, event, head SHA, head repository, branch, conclusion, and HTML URL.
Select the newest run and attempt for each workflow, event, and source branch/repository identity.
Ignore earlier attempts and runs that a newer execution supersedes.
Do not filter to `action_required` before selecting the latest runs.

Match explicit pull-request associations when present. Use the association repository ID and canonical
repository URL when the REST payload omits `owner.login`, and compare every identity field that both
the association and pull-request transport provide. A mismatching non-empty association or top-level
head repository excludes the run.
GitHub can return an empty association list for fork workflows that require approval.
For that case, require matching head SHA, head repository, head branch, and the `pull_request` event.
Extend the PR transport shape with head repository identity where necessary.

For selected `action_required` runs, fetch current-attempt job counts.
A completed fork `pull_request` run with `action_required` and no jobs supplies the approval classification for this defect.
This is an interpretation of combined provider evidence, not an explicit approval-reason field.
Other action-required runs use the generic attention classification.
Unknown job counts must not become zero. Missing runs, `waiting`, and `UNSTABLE` alone never prove approval.

`getPRStatus` and `getPRFeedback` in `client_helpers.go` share the collector and classifier.
The batched service path in `service_pr_watch_batched.go` enriches statuses before caching and applying them.
Cover both numbered and branch-discovered watches. The existing poller owns refresh cadence.
Coalesce enrichment by credential scope, repository, and head identity within each sync.
Use bounded concurrency and the existing context budget. Do not add a frontend poller or one request per mounted surface.
The unwatched-task lifecycle sweep only reconciles PR state, the observed head, and stored workflow attention; it does not issue new Actions reads. Retained watches and REST feedback/status paths provide fresh workflow evidence, while a head change invalidates the stored observation.

The REST [workflow runs contract](https://docs.github.com/en/rest/actions/workflow-runs) defines collection and permission requirements.
GitHub documents the maintainer action in [Approving workflow runs from forks](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/approve-runs-from-forks).

## Data contract

Add an additive `workflow_attention` object to `PRStatus`, `PRFeedback`, and `TaskPR`:

```text
state: unknown | none | approval_required | action_required
head_sha: observation identity
observed_at: timestamp of successful observation
stale: whether the most recent attempted observation failed
runs: [{run_id, run_attempt, workflow_id, name, url, reason}]
```

The run list contains only selected workflows that need attention.
`none` requires complete provider evidence. Partial or unavailable evidence is `unknown` unless a prior same-head observation exists.
An internal populated marker distinguishes old callers that did not attempt collection from an observed `unknown` result.
When a surface has both stored TaskPR evidence and cached feedback, compare applicable same-head observations by
`observed_at` and retain `none` as an authoritative observation during that comparison. A changed-head or malformed
cached observation cannot hide valid current stored evidence; a newer unknown observation preserves a positive result
as stale.
Old payloads without this object remain compatible and cannot claim approval.

## Storage and recovery

Persist the object as one JSON text column on `github_task_prs`, with an empty default for existing rows.
Use the existing additive migration, column list, scan, upsert, update, and restore paths.
`SyncTaskPR` remains the writer and publishes the existing workspace-scoped `github.task_pr.updated` event.
Boot data, task-scoped reads, feedback persistence, and live events carry the same object.
Keep compact task-list projections bounded. Derive their attention category from the stored object rather than adding the run list.

An unavailable read preserves prior same-head evidence and marks it stale.
The UI shows "Last known" and a refresh-unavailable explanation for that evidence.
A new head invalidates old evidence immediately. Closed or merged PRs suppress active attention.
An authoritative complete read replaces the run list, including an explicit empty result.
Omitted legacy observations preserve same-head stored evidence and never clear it accidentally.

Actions read permission can be absent even when check reads succeed.
Permission errors, rate limits, and timeouts leave other PR data available.
They do not trigger permission changes or claim that CI passed.
Bounded debug logs name the operation and PR, without tokens or response bodies.

## Status and automation

Leave actual `checks_state`, `checks_total`, and `checks_passing` intact.
Do not insert workflow placeholders into `CheckRun` or existing failure snapshots.
Approval-only evidence produces no CI repair message and consumes no repair round.
Existing independent review/comment/conflict triggers retain their behavior.
Actual failed checks remain eligible for their existing repair path.

The strict merge predicate rejects current attention, including stale positive evidence for the same head.
Preserve the reviewed-head invariant in [the auto-merge ADR](../../../decisions/2026-08-28-bind-github-auto-merge-attempts-to-reviewed-head.md).
Display precedence remains terminal, active queue, draft, actual blocking failures, then workflow attention before success.
An approval row remains visible alongside independent failure rows.

## Presentation

`pr-task-status-summary.tsx` adds a warning CI row: "Awaiting maintainer approval".
For approval-only evidence, suppress the redundant raw `unstable` merge row.
For unexplained `unstable`, use localized "Checks not successful" without inventing a cause.
Retain raw fallback behavior for genuinely unrecognized provider values.

`pr-task-icon.tsx` and `pr-status-chip.tsx` consume one shared attention interpretation.
Audit the GitHub registered-provider adapter and compact task projection so active and inactive rows agree.
`pr-ci-popover.tsx` and `pr-detail-panel.tsx` show the workflow name, reason, and "View on GitHub" link.
Do not show "No checks" as the sole explanation or offer "Fix CI" for approval-only evidence.
Use the existing provider-neutral summary and detail anatomy. Avoid GitHub-specific slots in shared primitives.
All new copy uses locale keys in English, Portuguese, and the three Chinese catalogs.

The phone entry remains task navigation, then the PR status chip.
Reuse `PRStatusChipDrawer` and its existing scroll owner, safe areas, and dismiss behavior.
The closest shipped exemplar is `e2e/tests/pr/mobile-pr-ci-chip.spec.ts` and its corresponding chip component.
The compact task-row icon stays passive on touch. No new drawer or nested overlay is necessary.
The external link has a 44px minimum touch hit area, while desktop density stays unchanged.

## Verification

Provider fixtures reproduce an empty check rollup with a jobless action-required fork workflow.
Mixed-state tests cover an approval gate alongside a real failure and alongside a successful workflow.
Recovery tests cover newer attempts, changed heads, permission errors, reloads, and REST/GraphQL convergence.
Component tests cover summary copy, counts, status precedence, and strict merge readiness.
Desktop and mobile Playwright tests verify the existing entry points and the provider link.

## Related artifacts

- [Requirements](../requirements/github-workflow-attention.md)
- [Shared task summary design](../../ui/system-design/pr-task-status-summary.md)
- [Implementation plan](../../../plans/github-workflow-attention/plan.md)
