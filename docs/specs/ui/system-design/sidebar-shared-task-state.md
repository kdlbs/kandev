---
status: draft
system: ui
requirements:
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-002
---

# Shared Task State for Sidebar Views

## Purpose and ownership

UI owns shared task overviews and saved-view evaluation. Tasks retain lifecycle and authorization ownership.
Keep Zustand and reuse homepage data instead of creating an independent sidebar task collection.
This extends [sidebar browsing](sidebar-archived-filter.md), which still owns the server query and native-memory correction.
The [decision](../../../decisions/2026-09-29-shared-sidebar-task-state.md) revises the earlier unconditional server-query policy.

| Acceptance | Design sections |
| --- | --- |
| 002.20 | Canonical overview records; HTTP and WebSocket reconciliation |
| 002.21–002.22 | Coverage; Source selection; Ordering parity |
| 002.23 | Initial display under invalidation |
| 002.16, 002.24 | Retention and context isolation |
| 002.1–002.13 | Shared page projection; Responsive surfaces; Tests |

All references use `AC-UI-SIDEBAR-ARCHIVED-FILTER-002`.

## Source evidence before implementation

`useWorkspaceSidebarTasks` renders only `useSidebarTaskPage.response.entries`.
`useAllWorkflowSnapshots` separately hydrates `kanbanMulti.snapshots`, whose task arrays feed homepage consumers.
`WorkflowSnapshotData` has placeholder/failure flags but no complete-coverage contract.
`httpGetWorkflowSnapshot` uses `ListTasks`, which excludes archived tasks, then optionally applies `task_limit`.
The snapshot DTO does not tell the client whether that truncation happened.
`useSidebarPageLoader` rejects a successful response whenever the sidebar query revision changed during its read.
These are source-confirmed facts; PR timing and an independently reported three-discard reproduction are not additional measurements in this package.

## Canonical overview records

Add a store-owned `taskOverview` slice with `byId` scoped by account and workspace context generation.
The canonical entity contains the lightweight task projection needed by homepage and sidebar, not transcripts or session detail.
It includes identity, relationships, workflow placement, archive state, timestamps, repository metadata, summary, and required view inputs.
Each entity records field availability and accepted source freshness. Missing fields in a partial payload are not explicit clears.
Apply placement and its displayed step metadata as one accepted projection to avoid mixed old/new badges.
Keep task-owned timestamps separate from summary revisions; a summary update must not advance task mutation time.

Homepage snapshots own ordered task IDs per workflow and their step metadata.
Sidebar views own ordered task IDs, group/continuation entries, counts, source, and page metadata.
Neither owns a second independently mutable overview record.
Selectors can expose compatible task arrays for existing consumers, using the canonical objects and stable references.
Move homepage and sidebar reads to those selectors within the normalization work order.
Do not leave a new cache beside independently writable legacy task arrays as the completed design.
Rich task/session detail remains under its existing owner and can extend an overview without being copied into list retention.

Route boot hydration, workflow snapshot settlement, sidebar responses, task mutations, and relevant WebSocket handlers through shared merge actions.
Preserve unchanged entity references and use narrow selectors so unrelated tasks do not rerender the whole sidebar.
Preserve existing board optimistic actions, archive/delete navigation, and Office consumers through compatibility selectors and regression tests.
This is a scoped overview normalization, not a migration of every application object into one global entity framework.

## Coverage

Add optional `task_coverage` metadata to workflow snapshot responses and the corresponding boot projection.
It declares workspace/workflow scope, active membership, total eligible count before truncation, completeness, and an ordering profile.
Set completeness only when the response covers every eligible active task in that workflow.
Failed, placeholder, truncated, or older-server responses without metadata are incomplete.
A complete active snapshot never establishes archived coverage.
Compute metadata alongside the existing snapshot read; do not add a second all-task fetch or count probe from the browser.

Use ordering profile `sqlite_nocase_v1` for the verified SQLite contract and `server_only` where local collation parity is not established.
An older or unknown profile uses server evaluation. Do not guess database collation from browser locale.
A future PostgreSQL local profile needs executable collation parity before it becomes eligible.
PostgreSQL still shares records even when server evaluation supplies membership/order.

Client coverage records include required workflow IDs, complete membership IDs, available view fields, context generation, and freshness state.
All required workflows must be covered, including eligible hidden workflows; a partial workflow list cannot establish workspace coverage.
Only proven positive workflow restrictions can reduce that set. Negative, empty, and contradictory filters follow existing truth tables.
Do not infer completeness from fewer than 100 cached records, a total for another view, or the union of visited pages.
An archived page remains query-page coverage. It does not establish complete archived workspace coverage.

A WebSocket disconnect, replay gap, unknown membership mutation, or missing required field makes affected coverage stale.
A complete accepted snapshot can restore it after reconciliation with concurrent live changes.
Known create/update/archive/delete/move events update records and affected complete membership atomically.
A move out of the covered scope removes membership without pretending that unrelated scopes are complete.
The additive `task_workflow_coverage` on existing workflow-list responses, and
`taskWorkflowCoverage` in boot state, identifies all scopes containing eligible active
tasks. It includes hidden/Office scopes and an empty identifier for unassigned tasks,
regardless of which workflows the navigation list displays. Authoritatively empty
scopes require no extra snapshot. Positive workflow filters can use their own complete
snapshots. This is one lightweight server identity query, not another browser task fetch.

Compatibility task arrays are derived atomically by `withTaskOverviewNormalization`:
legacy write inputs are merged before publication, and all arrays expose exactly the
canonical objects. They cannot publish an independently mutable task record. Page
memberships retain IDs and group metadata. Wire patches preserve own-field availability;
source eligibility additionally requires every field used by the requested evaluator.

No extra polling or all-workflow snapshot fetch is mounted solely for the sidebar.
Cold deep links can use the bounded server query immediately.

## Source selection and shared page projection

The shared controller chooses one source for a complete effective view key:

1. Use complete, current resident coverage when the evaluator supports its ordering and required fields.
2. Otherwise show an eligible retained page immediately, then refresh it on the server.
3. Otherwise request the first bounded page and show the existing initial loading state.

Archive state controls which coverage is needed, not whether pagination exists.
Unloaded archives are fetched only when selected or explicitly paged.
Do not request missing IDs from an unknown collection: the server must determine matching membership and order.
Merge fetched page entities into `taskOverview`; retain membership and metadata separately.

For local evaluation, filter the complete candidate set before resolving task trees and applying sort/group/pin/manual order.
Derive complete-tree activity and effective state before collapse and page slicing.
Return the same page projection as server results, including totals, headings, parent context, and WIP metadata.
Render at most 100 task rows and show pagination only above 100 displayable rows.
An already complete large resident collection can paginate locally without fetching it again.
The sidebar must never fetch every task merely to enable that path.
Memoize evaluation by relevant membership/entity/preferences revisions; do not repeat it on unrelated streaming events.

When complete coverage arrives while a server read runs, switch only for the same effective view and cancel the redundant request.
A source switch preserves the current page, selected conversation, and scroll position unless the user changed the view.
When coverage becomes stale, retain safe visible rows and request the current server page.
Do not alternate sources on every render or run both fetch strategies in parallel.

## Ordering parity

Reuse the existing client filter/tree utilities where their semantics match the server.
Do not call the old `applyView` engine unchanged and assume parity.
It uses browser-dependent text ordering and can differ in tie order and repository grouping.
Add a focused sidebar local evaluator against the common conformance fixtures.
Use the server's canonical input ordinal: updated time descending, title order, then task ID.
Preserve nanosecond activity comparison, timezone offsets, malformed-value fallback, state quantifiers, and deterministic cycle handling.
Include repository combinations, missing repositories, manual order, WIP queues, and page-boundary context.

For SQLite, implement the exact ASCII NOCASE/BINARY comparison rules used by the query, including UTF-8 ordering for non-ASCII ties.
Do not use `localeCompare` or JavaScript UTF-16 order as a substitute for SQLite BINARY order.
An unsupported ordering profile or unavailable WIP/repository field uses the server path.
A mixed-state tree fixture must include running, completed, and excluded descendants.
Shared fixtures must compare local results with actual SQLite pages at 100 and 101 rows.
PostgreSQL remains on server ordering until a verified local profile is available.

## HTTP and WebSocket reconciliation

Keep context generation, request identity, view identity, and authorization checks before every merge and display commit.
Use existing task freshness and summary revision rules to reject older field updates.
Track per-entity changes since request start so a delayed HTTP response cannot replace a newer live value or known deletion.
Do not infer task freshness from summary freshness or compare unrelated revision domains.

`recordTaskOverviewChange` coalesces each read's repeated partial patches with the
same merge rules as `mergeTaskOverview`, including when no `byId` record exists.
Task fields compare strict RFC3339Nano instants through
`parseStrictRfc3339Timestamp`, with the existing space-to-`T` normalization.
Reject task fields only when both timestamps parse and the incoming instant is
strictly older. Equal, missing, or malformed timestamps keep the existing
fallback semantics. Independently use `pickFreshestStatusSummary`: omit/null
preserves the accepted summary, lower revision loses, and equal revision accepts
the incoming projection. A summary-only patch does not advance `updatedAt`.
Keep journal entries typed as `TaskOverviewPatch`; share partial-safe merge logic
inside the existing merge module without asserting that a journal patch is a
complete resident overview or synthesizing missing task fields. HTTP settlement
then compares the coalesced patch with the returned row using those same clocks.
This protects the registered archived `task.updated` path before a bounded page
introduces its task. Ordinary active updates already normalize board residents.
Deletion remains a sticky `null` for each outstanding read; merging a patch
must not resurrect it. Measure journal bytes from the resulting stored patch
and preserve existing ID/byte limits, overflow generation, and read cleanup.

A complete snapshot response can replace membership only after reconciling live membership events during its read.
Use a bounded in-flight reconciliation journal, capped at 1,000 affected task IDs or 1 MiB per controller.
Coalesce entries by ID and retain deletion/removal tombstones until older in-flight reads settle.
On journal overflow, mark coverage incomplete and clear reusable membership.
Cancel and drain older reads before releasing their tombstones; never accept a response after its protection was discarded.
Retry through the server path with backoff and visible recovery state, rather than an unbounded log or immediate retry loop.
Use no persistent event journal.

## Initial display under invalidation

Separate hard rejection from a soft refresh request.
Account/workspace changes, access denial, superseded view/page requests, and invalid request identity are hard barriers.
They can clear or reject data. Ordinary membership/order invalidation within the same scope is soft.
Do not cancel the current read solely because a soft invalidation arrived.

On a successful same-scope response, merge entities without overwriting newer accepted values.
Remove known deleted, moved-out, archived-out, or otherwise ineligible rows before display.
Keep the remaining response order as a provisional server page; do not re-sort an incomplete set.
Mark page membership/counts as refreshing and do not put this provisional result into reusable page storage.
Do not claim an authoritative empty result when reconciliation removed every returned row.
Keep eligible prior rows if available; otherwise show loading/recovery until a safe page exists.
This does not require waiting for an event-free interval before any safe rows can appear.

Coalesce changes during one read into at most one trailing read, using the existing 250 ms/2-second scheduling bounds.
A second update while that trailing read runs can request the next refresh, but cannot clear the displayed rows.
Use backoff on failures and never overlap duplicate reads for the same view/page.
A stable response restores authoritative counts and reusable membership.
No stale context, deleted task, or denied record can appear merely to avoid a loading indicator.

## Retention and context isolation

Preserve five reusable first-page memberships, a combined 2 MiB budget, and five-minute fetch-age expiry.
The budget counts metadata and unique overview bytes attributable to those memberships, even when records are normalized.
An oversized page can be displayed without entering reusable retention.
Later pages replace the current page and do not accumulate memberships.

Entities are retained by explicit owners: existing homepage membership, displayed page, reusable page, or active detail.
Eviction releases an owner's IDs and removes entities with no remaining owner.
Archival removes active-board ownership. A later server page does not keep every previously visited archived task alive.
Release owners on disposal. Account/workspace generation changes clear reusable coverage and prevent cross-scope reads.
Temporary reconciliation journals and tombstones also have explicit settlement/overflow cleanup.

## Responsive surfaces and states

Desktop retains the Tasks sidebar. Phone retains `SessionTaskSwitcherSheet` and `MobileTaskList` in the existing drawer.
Both use the same source controller and page projection. No new navigation or controls are introduced.
Phone keeps safe-area handling, 44px touch targets, contained scrolling, and focus return.
Shared data appears immediately; background refresh uses the existing localized nonblocking status.
Uncovered initial loads use the existing loading state. Errors and Retry remain distinct from a successful empty list.
Any new copy uses all six locale catalogs. Do not hardcode provisional-state text.

## Tests and delivery

[The package](../../../plans/sidebar-query-memory/plan.md) owns implementation and exact commands.
Store tests cover single-record identity, compatible projections, field availability, live freshness, and owner-based eviction.
Coverage tests cover truncation, older servers, missing workflows, archived separation, reconnect, and concurrent membership changes.
Evaluator fixtures cover all supported SQLite filters/sorts/groups and local/server page parity.
Hook tests defer responses across three soft invalidations and require safe rows after the first successful response.
Hard-barrier, deletion, overflow, and access-denial tests must still reject unsafe results.
Desktop and phone E2E cover homepage-to-sidebar reuse without a duplicate query and cold archived paging with bounded retention.
The server memory correction and its native benchmarks remain required for cold views.

[Archived update freshness](../../../plans/archived-sidebar-update-freshness/plan.md)
owns the narrow journal regression repair. Its permanent test dispatches through
`registerTasksHandlers` into a real `createAppStore` and
`SidebarTaskPageCache`, deferring only `querySidebarTasks`. It covers repeated
archived updates and HTTP settlement in both freshness directions, resident and
nonresident records, separate summary revisions, partial patches, tombstones,
bounded journals, and read/context isolation. This data normalization introduces
no layout, navigation, touch, scrolling, or breakpoint changes; focused transport
integration and existing affected controls satisfy the data-only mobile exception.
