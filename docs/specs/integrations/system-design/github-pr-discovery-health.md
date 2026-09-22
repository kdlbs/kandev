---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-DISCOVERY-001
  - REQ-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001
created: 2026-09-11
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
