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

After `CheckIssueWatch` has collected the unseen issues of every page, and
before project descriptions and `sortIssues`, `dropBlockedIssues` removes each
issue with a blocker that is not done or with a truncated relation list. A
held issue is not published, so the orchestrator neither reserves it in the
dedup table nor counts it against `maxInflightTasks`. The next poll reads its
blockers again, and once they are completed it is published and sorted with
the other issues. Held counts are logged at debug level.

## Pending issues

Linear's search orders by `updatedAt`, and a watch reads at most
`issueWatchMaxPages` pages of 50. A held issue does not change when its blocker
completes, so in a busy team it can leave that window while it waits. A
released issue can also miss its start: when the watch's in-flight cap is
full, dispatch defers the event without reserving the issue. To start such
issues anyway, a blocker-aware watch records the Linear IDs of the issues it
has matched but not yet started (held issues and released issues not yet
reserved) in `linear_issue_watch_pending_issues (issue_watch_id, issue_id)`,
keyed by watch and cleared with it and on reset.

On each poll, `holdBlockedIssues` takes the pending IDs that the paged search
did not return and re-checks them with the same search and filter plus
`SearchFilter.IssueIDs`, which `buildIssueFilter` maps to `id: { in: [...] }`.
`IssueIDs` is `json:"-"`, so it is never stored or read from a request.
Re-checks run in batches of 50 IDs, and the pending set is capped at
`maxPendingIssues` (250), so a poll adds at most five requests. Re-checked
issues join the window's unseen issues before blockers are evaluated. The
stored set is then replaced by the unseen issues of this poll, held first:
an issue leaves it once dispatch has reserved it (it is then seen) or once it
no longer matches the filter (it is absent from the response). When a
re-check request fails, its unchecked IDs stay pending and the rest of the
poll continues; a canceled poll fails as a whole. When the pending set cannot
be read, the stored set is left unchanged. Beyond 250 pending issues, the
extra ones start only while they are inside the search window.

## Pagination

A watch with default order normally reads a single page, because sorting a
default watch changes nothing. With the setting on, the watch reads up to
`issueWatchMaxPages` pages like a sorted watch, so a first page full of held
issues cannot hide issues that can start. A watch with the setting off keeps
its current page count and requests.

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
pagination past held issues, the unchanged default path, re-checks of pending
issues outside the window (release, a release deferred until reservation,
filter mismatch, request failure, cancellation, and batch bounds), and the
pending table across reset and delete. Web unit tests
cover the form round trip and the switch. Playwright covers the setting across
team pick, save, and reopen on desktop and phone, and a mock-Linear dispatch
where a blocked issue gets a task only after its blocker completes. The mock
client returns blockers only for blocker-aware searches but does not apply
other filters, so mock flows do not verify Linear-side matching.
