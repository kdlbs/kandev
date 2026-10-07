---
status: current
system: integrations
created: 2026-10-06
requirements:
  - REQ-INTEGRATIONS-GITHUB-BROWSE-STATUS-001
owners:
  - kandev
---

# GitHub Browse Status Batches System Design

## Purpose and boundaries

Associate the local `usePRStatuses` result with the existing requested batch key
and enforce eligibility when returning it. Integrations already owns provider
state and browse outcomes. The [dashboard design](github-dashboard-mobile.md)
owns queries and phone composition; the
[reverse-association design](github-pr-task-association-reads.md) owns linked
tasks. Neither contract should absorb this independent status read.

This applies existing workspace/batch identity locally. No architectural choice
requires an ADR, new system boundary, system README edit or shared coordinator.
The existing backend transport and cache remain authoritative for response data;
this frontend correction makes no new permission or freshness claim.

## Requirement mapping

| Criteria for REQ-INTEGRATIONS-GITHUB-BROWSE-STATUS-001 | Design section |
| --- | --- |
| AC .1, .2, .3 | [Result identity and immediate reads](#result-identity-and-immediate-reads) |
| AC .4, .5, .6 | [Scheduling and completion](#scheduling-and-completion) |
| AC .7 | [Consumers and mobile parity](#consumers-and-mobile-parity) |

## Components and existing contracts

- `apps/web/components/github/my-github/use-pr-statuses.ts` owns local state,
  `completedKey`, request effects, `prStatusKey` and the public
  `usePRStatuses(workspaceId, prs): Map<string, GitHubPRStatus>` signature.
- The key is the existing workspace ID followed by ordered
  `prStatusKey(repo_owner, repo_name, number)` values. Absent workspace or empty
  PRs produces the existing empty key. Preserve its construction and ordering;
  do not add HEAD, timestamps, navigation generations or array identity.
- The hook imports `getPRStatusesBatch` and `PRStatusRef` through
  `lib/api/domains/github-api.ts`. Their implementation is in
  `lib/api/domains/github-pr-api.ts`. No API client changes are needed.
- `PRListBody` calls the hook and looks up each row by `prStatusKey`. `PRRow`
  passes that result to `PRStatusBadges` and `pickPRForLaunch`. Missing status
  already renders no badges and falls back to the current query PR for launch.
- `PRStatusBadges` owns existing check/review/mergeability display. The actual
  list, shared `ChangeRequestRow`, native router, `StateProvider`, tooltip and
  locale setup form the immediate integration boundary.

## Result identity and immediate reads

The local result snapshot contains the producer key and its Map. These publish
together only from an uncancelled
successful request. Do not derive producer identity from the current props,
`completedKey`, a shared PR key or a transport's pending state.

At render time, return the stored Map only when the requested key is nonempty
and equals the snapshot's producer key. Otherwise return a stable per-hook empty
Map. Avoid recreating an eligible Map or copying status objects on reads. The
fallback remains local to the instance; no module-level mutable Map is needed.

This read fence must work before passive effects run. Clearing state in the
request effect alone is insufficient: it can expose a stale badge in the first
commit and destroys the retained A result used by existing A-to-B-to-A reuse.
Keep the old snapshot internally while another batch is pending; it is unreadable
in that other batch. Do not filter the Map by overlapping PR membership.

## Scheduling and completion

Keep `completedKey` separate from the result snapshot. Preserve the current
effect dependencies `[key, workspaceId]`, ordered refs and request initiation.
Equal-content new arrays do not clean up or restart the active read.

For absent workspace or empty PRs, preserve the effect's reset of `completedKey`
and result clearing, while the render fence already returns empty. A successful
uncancelled response sets `completedKey` and publishes its key plus
`new Map(Object.entries(resp.statuses ?? {}))` atomically.

Keep the closure-local `cancelled` guard on both success and failure. Cleanup
marks the originating effect cancelled. The producer key cannot replace this
guard: a late A1 response would otherwise match A again after A-to-B-to-A.
StrictMode's first effect is cancelled; its second effect can request because
no completed key was recorded by the cancelled read.

Preserve the existing failure outcome: do not update `completedKey`; clear a
nonempty retained result, and retain an already empty result. Preserve snapshot
ownership when retaining an empty result. Failure cannot reveal another batch.
The existing comment about retrying on the next render is inaccurate: a render
with unchanged key and workspace does not rerun this effect. Descriptive comments
reflect that dependency behavior; no retry trigger is added.

Retained completed A -> pending B -> A returns the same A Map and uses the
existing completed-key skip. Pending A1 -> B -> pending A2 must ignore A1 success
and failure. If B completed or failed, existing retention/scheduling determines
whether A has data and whether another request occurs. In particular, a B
failure can clear the retained A Map while `completedKey` still remembers A;
returning to A can therefore remain empty without a new request. Do not repair
this adjacent recovery behavior as part of the read fence.

## Consumers and mobile parity

No consumer production change is causal in the accepted proof. `PRList` and
`PRStatusBadges` already honor missing Map entries. Their layout, copy, actions,
navigation, scrolling and responsive/touch behavior remain the shipped surface.
Any newly discovered consumer defect requires ROOT review before widening scope.

Desktop and phone use the same list and derived Map. This is purely local state
eligibility, with no viewport-dependent change. The mobile-parity pure-state
exception is satisfied by real hook/component evidence; no browser, Vite build
or Playwright run is justified. Existing badge strings remain localized.

## Verification

The [single sequential work order](../../../plans/github-pr-status-batch-scope/task-01-scope-status-results.md)
owns exact commands and the regression matrix. Use the real hook and actual list,
with only `getPRStatusesBatch` partially mocked using controlled native Promises.
Record first-render hook values and the first scoped list commit before passive
effects, so an effect-only clear cannot pass. Prove current ACK, equal-content
reuse, current failure behavior, cancellation, independent instances and both
A-to-B-to-A paths. Keep unchanged key/launch/badge controls.

The accepted proof is read-only baseline evidence. It is not a new test run or
the permanent regression implementation.
