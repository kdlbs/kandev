---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-002
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

`REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003` maps to [Task approval badge](#task-approval-badge).
Criteria 003.1-003.6 use eligibility and visual precedence; 003.7 uses the bounded projection;
003.8 uses the existing disclosure and accessibility contracts.

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
All new copy uses locale keys in English, Portuguese, Japanese, and the three Chinese catalogs.

The phone entry remains task navigation, then the PR status chip.
Reuse `PRStatusChipDrawer` and its existing scroll owner, safe areas, and dismiss behavior.
The closest shipped exemplar is `e2e/tests/pr/mobile-pr-ci-chip.spec.ts` and its corresponding chip component.
The compact task-row icon uses the existing `PRTaskIconDrawer` on touch. No new drawer or nested overlay is necessary.
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

## Expiring Actions observations

`REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-002` maps to this section.
Share raw workflow-run observations across batched enrichment and service
feedback/status reads. Keep per-PR classification outside the shared cache.
Key entries by credential scope and generation, normalized repository identity,
and head SHA. Reuse the bounded `ttlCache` and its singleflight/invalidation
patterns; add per-entry expiry support if needed. Keep at most 512 run entries
and 512 job entries. Errors are not cached.

| Observation | TTL |
| --- | --- |
| Empty list or any pending/running/unknown status | 30 seconds |
| Any approval/action-required or ambiguous attention result | 30 seconds |
| Nonempty, all completed, no attention requirement | 5 minutes |

A SHA does not freeze the workflow run collection. Reruns and new runs can
arrive for the same SHA. Never use an infinite TTL. Cache job observations by
credential scope, repository, run ID, and attempt, with the same expiry classes.
A run-attempt change must not reuse jobs from an earlier attempt.

Keep existing batched enrichment limits: concurrency 4 and total budget
5 seconds. Preserve the outer status/search cache TTL of 30 seconds and the
feedback TTL of 8 seconds. Long Actions entries only affect workflow attention,
not checks, review status, or auto-merge decisions.

Explicit refresh invalidates the relevant Actions and outer response entries,
then coalesces concurrent refreshes. Generation checks prevent an older
in-flight request from repopulating invalidated data. Passive page polling
uses the cache. Route future approval/rerun mutations through this invalidation
seam; adding those mutations is outside this package.

External reruns on a completed SHA can take up to 5 minutes plus the next
1-minute poll tick to appear. Attention and running results use the short TTL.
Provider errors retain unknown/stale-positive semantics and existing retry
admission. Terminal PRs continue to skip Actions reads altogether.

See [the implementation plan](../../../plans/watch-task-cleanup/plan.md).

## Task approval badge

### Eligibility and visual precedence

This extension reuses the stored `TaskPR.workflow_attention` observation.
The provider collector, refresh cadence, persistence schema, check counts, and automation rules retain their existing contracts.

For full PR records, reuse `getTaskPRWorkflowAttention` and `isWorkflowApprovalRequired` from `pr-workflow-attention.ts`.
Eligible evidence requires an open PR, a nonempty current head SHA, and matching observation head SHA.
The observation state must equal `approval_required`; `action_required`, `unknown`, missing evidence, and `none` do not qualify.
Same-head stale positives remain eligible. Existing summaries explain their last-known status after disclosure hydration.

Aggregate eligibility across open PRs independently from the status-color calculation.
`PRTaskIconView` supplies the approval flag through `PRTaskIconGlyph` to `PRStatusGlyph`.
The new `hasWorkflowApprovalRequired` glyph prop defaults to false.
Other glyph callers remain compatible; adding the badge to the topbar is outside this package.

Use Tabler `IconLockFilled` in the existing upper-right warning wrapper.
Keep the wrapper at 10px and the glyph at 8px, matching the current conflict badge.
Use `text-[#D97706] dark:text-[#FBBF24]` and the existing `bg-background` halo.
Explicit colors preserve the accepted palette independently of Tailwind's amber token version.
Keep the PR color and automation dot positions intact. Do not add animation or a separate badge interaction target.
Render the red conflict triangle when both flags are true; retain both facts in text and accessible names.

### Bounded projection

The current `PullRequestSummary` does not carry workflow approval.
Add optional `workflow_approval_required` to `status_summary.pull_request` as a boolean, omitted when false.
It represents eligible approval on any open linked PR, independent of the representative PR and aggregate color.
It is a display fact, never merge permission or an authorization claim about the current user.
Old clients ignore it; old payloads default to false. Do not infer it from `attention` or `aggregate_state`.

Extend `statussummary.PullRequestInput` and its internal PR observation with three bounded strings:
`HeadSHA`, `WorkflowAttentionState`, and `WorkflowAttentionHeadSHA` (proposed field names).
The package derives approval with the eligibility predicate above, without importing GitHub's full provider model.
The `githubTaskStatusSummaryPRReader` adapter copies those fields from the stored observation.
`Projector.applyPREventLocked` extracts the same fields from the existing `github.task_pr.updated` payload.
Support the event payload normalization used by the projector; test malformed or omitted objects without approval inference.
Carry these fields through `applyPullRequestInputs` and compare them during event deduplication.
`derivePullRequestSummary` computes an OR over eligible open PRs.
Rebuild and live-event paths must agree. Authoritative head changes, terminal states, and empty PR lists clear the flag.
Unavailable source reads retain the existing summary baseline policy.

Extend `TaskStatusSummary` in `lib/types/task-status-summary.ts` and `TaskPRInfo` in `lib/task-pr-info.ts`.
`taskPRInfoFromSummary` maps the new boolean to `workflowApprovalRequired` (proposed field name).
Until full PR records exist, `PRTaskIcon` uses that projection for the badge and its accessible name.
Once records hydrate, their current-head evidence is authoritative, including an explicit absence of approval.
Never OR a stale compact flag into a hydrated negative result.
No additional API request, provider poller, database column, or session subscription is necessary.

### Disclosure and accessibility

Reuse `github:workflowAwaitingApproval` for visible text and the approval portion of the accessible name.
Its existing English value is "Awaiting maintainer approval". This preserves established terminology and six-language localization.
Do not add an approval-only tooltip over the icon's existing disclosure.
Hydrated disclosures reuse `PRTaskStatusSummary` with per-PR approval and conflict rows.
Compact disclosures show the projected approval reason alongside the existing hydration status until full details arrive.
Hydration failure must not leave a visible padlock with only a generic loading or unavailable message.

The nearest phone exemplar is `PRTaskIconDrawer`, exercised by `mobile-pr-sidebar-automation-indicators.spec.ts`.
The task picker keeps its existing navigation and scroll ownership.
Tapping the PR control opens its existing drawer and stops row navigation; dismissal restores focus to the opener.
Retain its touch hit area, safe-area behavior, internal scroller, and desktop density.
Verify phone rendering and a narrow fine-pointer viewport; no new overlay composition is needed.

### Delivery and verification

The [badge plan](../../../plans/github-workflow-approval-badge/plan.md) extends the completed workflow-attention package.
Its two sequential work orders own projection/component regressions, then rendered desktop/phone coverage and public explanation.
Existing workflow-attention and polling plans keep their recorded completion results; this extension does not reopen their delivery scope.
Unit tests cover all observation states, lifecycle cleanup, multi-PR aggregation, conflict priority, and hydration precedence.
Provider-backed browser fixtures prove visibility before disclosure, reload, refresh recovery, and the phone explanation.
