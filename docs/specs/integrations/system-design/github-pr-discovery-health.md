---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-POLLING-001
  - REQ-INTEGRATIONS-GITHUB-PR-POLLING-002
  - REQ-INTEGRATIONS-GITHUB-PR-DISCOVERY-001
  - REQ-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001
created: 2026-09-11
updated: 2026-10-28
owners:
  - kandev
---

# GitHub PR discovery health design

## Ownership and mapping

`REQ-INTEGRATIONS-GITHUB-PR-DISCOVERY-001` maps to query validation and association
below. `REQ-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001` maps to failure state,
admission, projection, and recovery. Integration services own both outcomes.
Existing workspace authentication and frontend synchronization coordination
remain authoritative for credential selection and consumer leases.

## Query validation and association

`internal/github/graphql.go:prFieldsBlock` supplies both branch and known-PR
batch queries. Select `headRepository { id name nameWithOwner url }`, not
`cloneUrl`. Decode `url` explicitly and derive the HTTPS clone URL through a
shared mapping that preserves host, owner, repository, and existing null rules.
Keep GH CLI JSON fields separate from GraphQL schema fields: CLI output is not
a schema authority. Do not rewrite checkout remotes or change clone policy.

Use the existing watch and association writers in `service_pr_watch_batched.go`,
`service_pr_watch.go`, and `poller.go`. Preserve fork-network resolution,
workspace-group attribution, explicit detached tombstones, and event publication.
The definitive-empty batch result must retain its existing no-repeat behavior.

Tests must inspect generated selections against a checked, minimal Repository
field contract, in addition to exercising response conversion and persistence.
Record the upstream schema reference in the fixture. A canned response that
accepts any query cannot detect this defect.

## Watch target reconciliation

Discovery AC .4 uses the same session repository/worktree target resolution
for watch creation and refresh. In `internal/github/poller.go`,
`TaskBranchProvider.ResolveBranchForWatch` carries session, repository, and
current branch identity. The orchestrator implementation lives in
`event_handlers_github_watch_reconciliation.go`; the poller does not choose a
task's primary branch for every watch.

The session inventory can contain one row per distinct worktree branch without
repository identity. Expand each session once and do not pass an inventory
row's branch as the primary repository fallback; load branches from the
session's per-repository worktrees instead.

Resolve targets from the source session's task through
`resolveSessionWatchTargets`, matching creation's fallback and worktree rules.
`watch.TaskID` may name a workspace-group owner and remains the association
destination, not a substitute source of checkout identity. Reuse target
resolution without invoking effective-owner redirection for branch selection.

Keep an exact repository/branch match. Only select a replacement when there
is exactly one distinct branch target for that repository and the current
branch is absent. Missing session or repository identity, lookup failure, no
matching targets, or multiple unmatched candidates leaves the watch unchanged.
Never use a sibling repository as a fallback. Preserve single-repository
checkout-branch precedence and real unambiguous rename handling.

Keep the existing `pr_number = 0` atomic writer and collision handling.
Numbered watches, associations, detached tombstones, and attribution remain
unchanged. No migration or live database cleanup is required. Reconciliation
must reach a stable set of watch IDs and branches across repeated cycles;
tests include mixed numbered/searching watches and multi-branch sessions.

Delivery: [watch reconciliation repair](../../../plans/github-pr-watch-reconciliation/plan.md).

## Failure state and admission

Add a service-owned runtime discovery-health component. Key each target by
workspace ID, resolved credential cache scope/generation, repository identity,
and branch. Multiple session watches for that target share admission and health.
Keep a monotonic attempt generation and one active attempt per target. A batch
claims each participating target; failure affects only targets in that batch.
Unknown or unresolved aliases remain failures, not definitive empty results.

Store a bounded, sanitized category (`invalid_query`, `rate_limited`, or
`unavailable`), failure time, attempt generation, and optional retry deadline.
Do not retain raw provider payloads, query bodies, or credentials in UI state.
Remove target entries when no live watch needs them; clear the connection's
entries on credential replacement or workspace deletion. Service shutdown owns
any cleanup mechanism. No SQL migration or new perpetual polling loop is needed.

Classify GraphQL errors even on HTTP 200 at the shared decoded-error boundary.
Do not rely only on GH CLI stderr or HTTP status. Keep generic authentication
state separate: a valid login does not prove discovery works.

Use provider Retry-After/reset evidence when available. If a rate-limit error
has no reliable deadline, allow a coalesced probe after 60 seconds, doubling
the pause on continued failure up to 15 minutes. Invalid-query failures retry
at most once per minute. The existing poller performs due probes; manual sync
obeys the same admission rule. Do not retry an invalid or throttled batch by
immediately issuing the same query once per watch. Transport fallback remains
available when it can succeed and admission permits it.

Quota snapshots remain numerical observations in `RateTracker`; they do not
own discovery recovery. A `/rate_limit` refresh may replace a reported number
but cannot clear the separate failure or retry deadline. This avoids asserting
that a full budget is proof of availability. A later successful query for the
same target clears its failure, including a definitive empty result. A success
from an older attempt cannot clear newer evidence. One target's success does
not clear another target's failure.

## Projection and presentation

Add optional `pr_discovery_health` to workspace GitHub status: `state`
(`unknown`, `healthy`, `degraded`), `failed_target_count`, `last_failure_at`,
`category`, and optional `retry_at`. Summarize the newest outstanding failure;
retain degraded state while any current target has an unresolved failure.
Absence means unknown, not confirmed healthy. Do not expose target details
outside existing repository authorization.

Publish a scoped discovery-health event with credential generation and a
monotonic revision. Update Go wire types and the corresponding frontend types
in `lib/types/github.ts` and `lib/types/backend.ts`. HTTP status and WS projections must share revisions;
an older HTTP refresh cannot overwrite a newer event. Reject mismatched workspace
or credential generations in `github-slice.ts` and `lib/ws/handlers/github.ts`.
Existing quota events keep their meaning.

Render a localized warning under the workspace identity summary in
`github-status.tsx`. Show the same operation, category, and timestamp before
quota numbers in `github-rate-limit.tsx`. During refresh, preserve existing
content and indicate busy state through the current refresh control. Do not
label authentication invalid merely because discovery failed.

Reuse `GitHubAccessHelp`: desktop focus/hover disclosure and coarse-pointer
Drawer. This is the closest shipped mobile exemplar and matches the mobile
language's temporary-details surface. Keep one internal scroll owner for long
drawer content, safe-area spacing, focus return, and 44px touch targets.
The inline warning wraps within the settings column on both viewports.

## Validation and operational boundaries

Use deterministic clocks and attempt gates to prove error, full quota refresh,
continued degradation, due probe, and actual recovery. Include mixed targets,
credential replacement, out-of-order HTTP/WS completion, and nullable fork data.
Backend integration tests prove persistence and event identity. Browser tests
prove a warning survives disclosure refresh and clears only with newer recovery.
They must assert the phone drawer, not a tooltip opened by a simulated tap.

No live task is repaired by this package. After deployment, normal discovery
can link eligible PRs. Waiting for quota reset cannot repair invalid code.

## Post-push diagnostic outcomes

Discovery ACs .3 and .5 also cover `event_handlers_github.go` in the
orchestrator. `detectPushAndAssociatePRWithIdentity` currently merges provider
errors and empty results. `searchPRForExistingWatch` silently discards lookup
errors. Both paths must distinguish `found`, `empty`, `failed`, and `canceled`.

For a failed lookup, log a warning with the operation, workspace, task, session,
resolved repository ID, owner, repository name, branch, and safe category.
Reuse `classifyPRDiscoveryError` through a narrow exported classifier in the
GitHub package. Do not log `err.Error()`, response bodies, or request URLs.
Retain `invalid_query`, `rate_limited`, and `unavailable` as categories.
Do not invent an authentication diagnosis from an unclassified error.

Each successful empty lookup is debug-level evidence. At retry exhaustion,
report attempt, error, and empty counts plus the last outcome at debug level.
Failed attempts already have warnings. An empty result after an error records
an empty final outcome. A found result follows the existing association path.
Cancellation returns without a failure or exhausted-retry warning.

Keep the production retry schedule, service admission, watch identity,
association ownership, and health recovery rules unchanged. Factor a private
retry runner with an injected wait function if needed for deterministic tests.
Production waits remain context-aware with delays of zero, 30, and 60 seconds.
Tests exercise the runner through the orchestration entry point without sleeps.
No public status fields or frontend changes are necessary.

Delivery: [Recovery diagnostic fixes](../../../plans/recovery-diagnostics/plan.md).
The completed discovery-health and watch-reconciliation packages retain their
historical scope and results.

## References

- [GitHub Repository schema](https://docs.github.com/en/graphql/reference/objects#repository)
- [Authentication design](github-authentication-02.md)
- [Sync coordination](github-task-pr-sync-coordination.md)
- [Authentication ownership ADR](../../../decisions/0047-github-authentication-ownership.md)
- [Implementation package](../../../plans/github-pr-discovery-health/plan.md)

## Adaptive discovery schedule

`REQ-INTEGRATIONS-GITHUB-PR-POLLING-001` maps to this section.
Keep `defaultPRPollInterval` at 1 minute. Select due searching targets before
batch construction. Known numbered watches keep the current cadence.

| Searching task state | Minimum interval |
| --- | --- |
| Running or activity age below 2 hours | 1 minute |
| Activity age from 2 hours to below 24 hours | 15 minutes |
| Activity age at least 24 hours | 30 minutes |

Use injected time and `PRWatch.LastCheckedAt` for due checks. Round execution
to the next 1-minute tick, so actual delay may include one extra tick.
Use watch creation as an activity baseline for the first 2 hours.
A never-checked watch is immediately eligible. Missing activity evidence
fails open to fast polling. No new random delay is required in this package.

Add a narrow task-activity projection, loaded in bulk for eligible watch tasks.
Reuse the bounded query pattern in
`task/repository/sqlite/task_status_summary.go:LoadTaskLastActivity`, but not
its generic task-update source. Preserve that existing UI query unchanged.
Use persisted session execution state and conversation timestamps, plus observed
commit/push activity. Do not use generic task or watch `UpdatedAt`: GitHub sync
can update those fields itself. Exclude automatic provider notifications from
conversation activity. Include human messages and agent execution activity.
Do not scan remote branches or invoke GitHub to calculate idleness.
Record observed branch activity from existing local Git event paths; absence of
such an event is not proof that no remote push occurred.

An activity transition restores next-tick eligibility even after a slow poll.
Persisted task activity and last-check timestamps survive restart. A runtime
branch-activity marker may reset on restart without stopping periodic discovery.
Expire runtime markers after 24 hours and remove them with their watch.
Reconciliation of an unchanged watch must preserve its age and last check.

Group equivalent discovery targets before admission. Any active member selects
the 1-minute schedule for that target; do not duplicate the provider query.
Numbered targets and searching targets remain separate groups even when they
share a repository and branch, because their polling cadences differ.
Passive task/page refresh must use the same admission rule or it defeats the
background savings. Explicit user refresh bypasses only idle admission, never
quota or auth gates. Preserve `PRSyncFreshnessWindow` at 30 seconds for existing
sync consumers and keep the distinction between automatic and explicit reads.
If a passive provider attempt is already active, an explicit refresh starts a
new admitted attempt instead of joining the passive result. The superseded
attempt cannot publish its result, and concurrent explicit refreshes share the
explicit batch singleflight key. A retry deadline from an authentication,
rate-limit, or invalid-query failure remains authoritative.
When a passive workspace read finds no stale task, admit workspace discovery
reads at most once per minute. A newly created or reset watch clears that
cooldown, and stale-task refreshes bypass it.

## Bounded fallback

`REQ-INTEGRATIONS-GITHUB-PR-POLLING-002` maps to this section.
Replace the poller's all-or-nothing boolean batch outcome with per-workspace
outcomes: completed, deferred, fallback-eligible. Publish successful results
without waiting for every workspace to succeed. Preserve existing shared
health admission for rate limits and invalid queries, and auth circuits.

Allow 5 fallback target checks per workspace, capped at 10 per cycle globally.
Apply this limit to unsupported-GraphQL clients too. Rotate workspace and target
selection between cycles so fixed ordering cannot starve later targets.
These are target-check limits, not HTTP-request limits: a check may paginate
or fetch several resources. Stop a workspace on the first authentication,
configuration, rate-limit, or invalid-query error and check cancellation before
each target. Never retry successful batches via REST.
Do not mark deferred targets as successfully checked.
Deduplicate equivalent watches before consuming either budget and apply one
fallback result to every watch in the selected target group.

Retain existing failure schedules: discovery retry starts at 1 minute and caps
at 15 minutes; auth/config circuits start at 2 minutes and cap at 6 hours with
25 percent jitter. Provider reset deadlines and credential changes retain
existing handling. Admission continuation uses the existing PR monitor owner,
as defined below. The five-minute issue/review loop retains its separate contract.

## Independent PR status publication

Polling ACs 001.6-001.8 and 002.5-002.7 map to this section and the next section.
The [rate coordination design](github-rate-limit-coordination.md) retains
principal-wide pacing, reserve, interactive priority, and provider retry rules.

Before this repair, `fetchBatchedWatchStatuses` fetched numbered PRs before
searching branches under one shared progress key. A deferred branch phase
returned no statuses to the application stage, and search membership changes
prevented reuse of completed numbered work.

Separate numbered and searching fetch phases inside the existing batch service.
Apply a complete numbered phase before a later branch phase can defer.
Keep one shared fetch/application implementation for poller, workspace refresh,
and task sync. Do not fork the provider decoding or association rules.

Introduce an internal outcome containing applied `PRWatchSyncResult` entries,
unfinished watch groups, an optional `AdmissionDeferredError`, and other errors.
The name of this new outcome is an implementation detail. Existing exported
service signatures can adapt it. HTTP and WebSocket response shapes stay unchanged.
Callers must preserve successful entries before handling an error.
Only unfinished groups can enter transport fallback.

Use separate progress keys for numbered references and searching references.
Retain credential cache scope, selected GitHub host, credential generation,
explicit-refresh identity, and exact references in each key.
Changes to searching references must not reset numbered progress.
Existing ordered-reference validation still rejects unsafe continuation reuse.

Only complete status evidence reaches `applyBatchedNumberedWatch` or
`applyBatchedSearchingWatch`. Review-thread pagination must complete before
publication of the affected status. Preserve unresolved-thread counts across
deferral. This repair does not publish a partially decoded aggregate.
Actions enrichment remains best effort under its existing cache and time budget.

Retain `applyPRWatchStatusResult`, its transaction, and its lifecycle guard.
Publish task-PR and feedback events only after their corresponding write succeeds.
Complete each discovery attempt once. Release deferred attempts without
recording provider failure, successful empty discovery, or a successful check.
An applied phase must not publish again when another phase resumes.

## Admission continuation in the PR monitor

`Poller.prMonitorLoop` remains the only owner of periodic PR-monitor scheduling.
Keep the one-minute inventory tick and adaptive searching intervals.
Add a bounded pending inventory and one reusable timer to that loop.
Do not add one goroutine per watch or wait for admission inside a workspace pass.

A pending entry records workspace, unfinished phase, watch IDs and target
identities, credential scope/generation, and the original retry boundary.
Coalesce equivalent pending targets. Keep entries proportional to active watches.
Prune obsolete entries during each inventory reconciliation and before resumption.
New inventory joins pending work without restarting completed phases.
Fresh one-minute ticks do not create a second attempt for a pending target.

For a deferred request, prefer its future `RetryAt`. Otherwise use its positive
`Delay` from the observation time. For undated busy or interactive-priority
deferrals, use a one-second retry floor. A timer wake always rechecks admission.
An expired deadline cannot create a zero-delay retry loop.
Provider deadlines remain lower bounds and are never reduced to that floor.

On each wake, process due pending work in rotating workspace order.
Retain later deadlines while other principals progress. A new deferral updates
only the unfinished work. Never restart all completed requests for that wake.
The normal tick still discovers new watches and prunes removed targets.
Cancellation stops both paths. `Stop` drains the monitor and clears pending state.
A restart reconstructs work from persisted watches through the normal initial pass.

Check `errors.As` for `AdmissionDeferredError` before fallback classification in
the poller, workspace refresh, and passive per-task sync paths.
Local pacing, busy, reserve, and interactive-priority deferrals do not trigger
REST fallback or authentication circuits. Keep the 5-target workspace and
10-target cycle limits for genuine transport or unsupported-GraphQL fallback.
Timer wakes do not replenish that cycle's fallback budget.

Before a resumed provider read or retained-result application, resolve current
authorization, host, credentials, active watch ID, repository, and target.
Discard mismatched progress. Keep explicit-refresh attempt fencing so a delayed
passive result cannot replace newer manual evidence.
Preserve retired lifetimes, detached tombstones, terminal-state guards, shared
target fan-out, and task-group association ownership.

## Sidebar delivery and verification

The backend remains the only writer of task PR status.
Use the existing `github.task_pr.updated` publication and task summary projector.
The resulting `task.status_summary.updated` event feeds inactive sidebar rows
through `updateTaskStatusSummaryInBothKanbans` and `useWorkspaceSidebarTasks`.
No per-row poller or task-selection refresh is added.

Desktop keeps `TaskSessionSidebar`. Phone keeps the existing task picker and
shared task-row icon. Shared state, icon semantics, navigation, and layout remain
unchanged. The phone task picker is the nearest shipped mobile exemplar.
New copy, controls, and responsive geometry are outside this repair.

Service regressions use real `RateAdmission`, fake provider responses, and
virtual time. Mix a known PR with searching targets that change each cycle.
Assert persisted status, event identity, and provider request counts.
Monitor regressions prove one-second continuation, long provider deadlines,
cross-principal fairness, cancellation, and bounded fallback.
Retirement, credential replacement, and explicit refresh invalidate held results.

A backend integration test follows monitor output through the real summary
projector to `task.status_summary.updated` for an inactive task.
Desktop and mobile Playwright tests then prove existing row consumption without
task selection, hover, tap, reload, or a target-task sync request.
Mock publication in those browser tests proves delivery only.
The service regression remains the RED test for pacing and batch coupling.

Delivery: [sidebar freshness repair](../../../plans/github-sidebar-pr-freshness/plan.md).

## Polling efficiency delivery

See [the implementation plan](../../../plans/watch-task-cleanup/plan.md).
Use deterministic clock tests for exact 2-hour and 24-hour boundaries,
restart, mixed groups, passive reads, explicit refresh, and fallback fairness.

## Explicit retirement admission

[Watch retirement](github-pr-watch-retirement.md) precedes adaptive scheduling and provider admission.
Only active members contribute to shared-target scheduling or receive provider results.
Explicit refresh bypasses idle timing only. It does not reactivate a retired watch.
The completed polling-efficiency package remains historical evidence.
The [retirement package](../../../plans/github-pr-watch-retirement/plan.md) owns this additional admission boundary.
