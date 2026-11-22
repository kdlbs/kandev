---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-LINEAR-BLOCKERS-001
  - REQ-INTEGRATIONS-LINEAR-BLOCKERS-002
---

# Linear Watch Blocker System Design

## Purpose and boundaries

The integration system owns the blocker setting, the blocker projection on
`LinearIssue`, and the decision to hold an issue. The orchestrator's watcher
dispatch is unchanged: it never sees a held issue. The setting is stored as
JSON in the existing `filter_json` column, so old rows decode with the setting
off. Pending issues are recorded in a new `linear_issue_watch_pending_issues` table,
created by the store's idempotent schema setup like the other Linear tables.

## Requirement mapping

| Acceptance criteria | Design sections |
| --- | --- |
| `AC-INTEGRATIONS-LINEAR-BLOCKERS-001.1` | Blocker projection; Holding issues |
| `AC-INTEGRATIONS-LINEAR-BLOCKERS-001.2` | Holding issues; Pending issues; Pagination |
| `AC-INTEGRATIONS-LINEAR-BLOCKERS-001.3` | Blocker projection; Holding issues |
| `AC-INTEGRATIONS-LINEAR-BLOCKERS-001.4` | Blocker projection; Pending issues |
| `AC-INTEGRATIONS-LINEAR-BLOCKERS-001.5` | Setting; Pagination |
| `AC-INTEGRATIONS-LINEAR-BLOCKERS-002.1` | Watch dialog |

## Setting

`SearchFilter.RequireBlockersDone` (`requireBlockersDone` on the wire) is kept
by `normalizeFilter`. `buildIssueFilter` does not read it, so it never reaches
Linear's `IssueFilter`, and `filterIsEmpty` does not count it: a watch still
needs at least one real match condition.

## Blocker projection

Linear stores "A blocks B" as an `IssueRelation` with `issue` A,
`relatedIssue` B, and type `blocks`; on B it appears in
`Issue.inverseRelations`. When the filter sets the flag, `SearchIssues` uses
`searchIssuesWithBlockersQuery`, which adds

```graphql
inverseRelations(first: 25) {
  nodes { type issue { identifier state { type } } }
  pageInfo { hasNextPage }
}
```

to the issue fragment. `blockersFromRelations` keeps the `blocks` nodes as
`LinearIssueBlocker{Identifier, StateType}`, and `hasNextPage` maps to
`LinearIssue.BlockersTruncated`. A relation is read from the blocked issue, so
a blocker in another team or project is included even when the watch filter
would not match it. Searches without the flag keep the plain query.

The page size of 25 keeps the query within Linear's complexity limit: Linear
multiplies nested connection costs by their page size, and a watch search
reads 50 issues per page, so the relation block adds roughly 50 × 25 × 3
points to a plain search.

## Holding issues

`LinearIssueBlocker.Done` is true only for state type `completed`. This
matches Kandev task dependencies, where only a successful prerequisite
resolves a link; a canceled blocker means the planned work did not happen.

After `CheckIssueWatch` reads the search pages, `holdBlockedIssues` re-checks
pending IDs and holds issues with open blockers or incomplete relation lists.
Held issues use no in-flight slots. The service sorts eligible issues before
publication and logs the held count at debug level.

## Pending issues

Linear's search orders by `updatedAt`, and a watch reads at most
`issueWatchMaxPages` pages of 50. A held issue does not change when its blocker
completes, so in a busy team it can leave that window while it waits. A
released issue can also miss its start: when the watch's in-flight cap is
full, dispatch defers the event without reserving the issue. To start such
issues after they leave the search window, a blocker-aware watch stores their
Linear IDs in `linear_issue_watch_pending_issues (issue_watch_id, issue_id)`.
Reset and deletion clear this list.

On each poll, `holdBlockedIssues` takes the pending IDs that the paged search
did not return and re-checks them with the same search and filter plus
`SearchFilter.IssueIDs`, which `buildIssueFilter` maps to `id: { in: [...] }`.
`IssueIDs` is `json:"-"`, so it is never stored or read from a request.
The pending set holds up to 250 IDs. The service re-checks up to 50 IDs per
batch and follows the cursor within a shared budget of five requests per poll.
If the budget ends or a request fails, unchecked IDs stay pending. A
cancellation or deadline error aborts the poll.

Re-checked issues join the search results before the blocker check. A temporary
reservation suppresses publication and keeps the issue pending. A recorded task
ID or a confirmed filter mismatch removes the issue from the pending set.
Beyond 250 pending issues, additional matches depend on the search window.

The store adds `poll_revision` to `linear_issue_watches` during schema setup.
Each poll with the setting on reads this revision before its Linear requests.
One transaction advances the revision and replaces the pending set if the
revision still matches. A reset, a watch edit, or another committed poll can
invalidate the revision. If the revision check fails, the poll discards its
results and skips the poll timestamp update. A pending storage error aborts
the poll.

## Pagination

A watch with default order normally reads a single page, because sorting a
default watch changes nothing. With the setting on, the watch reads up to
`issueWatchMaxPages` pages like a sorted watch, so a first page full of held
issues cannot hide issues that can start. A watch with the setting off keeps
its current page count and requests. With the setting on, default dispatch
order sorts the combined results by most recent Linear update.

## Watch dialog

`FormState.requireBlockersDone` is read from and written to the filter, and
`buildFilterPayload` omits it when off. A `Switch` labelled "Only start issues
whose blockers are done" follows the query field in `FilterFields`. The team
selector and `switchWatchWorkspace` leave it unchanged because it is not an
ID from the Linear organization. The watch table summary adds
`blockers:done`. On coarse pointers the switch's hit area grows to about
44 px through its `::after` inset, without changing its look. Desktop and
phone share the same dialog composition. New
copy is localized in every shipped locale.

## Security and compatibility

Relations are read with the watch's existing credential and scope; no new
permission is needed. Existing watches have the setting off and send the same
requests. Turning the setting off returns a watch to its previous behavior;
issues that were held are then started by the next poll if they match.

## Verification boundary

Real `GraphQLClient` requests against `httptest.Server` cover the query
choice, the relation mapping, and truncation. Service tests cover holding,
release after completion, canceled, cross-team, and truncated blockers,
pagination past held issues, and the default path with the setting off. Pending
issue tests cover reservation release, stale polls, reset, partial re-checks,
request budgets, and default dispatch order across batches. SQLite and
PostgreSQL tests cover revision writes. Web unit tests
cover the form round trip and the switch. Playwright covers the setting across
team pick, save, and reopen on desktop and phone, and a mock-Linear dispatch
where a blocked issue gets a task only after its blocker completes. The mock
client returns blockers only for blocker-aware searches but does not apply
other filters, so mock flows do not verify Linear-side matching.
