---
status: current
system: ui
requirements:
  - REQ-UI-THREADS-SAVED-VIEWS-001
  - REQ-UI-THREADS-SAVED-VIEWS-002
  - REQ-UI-THREADS-SAVED-VIEWS-003
  - REQ-UI-THREADS-SAVED-VIEWS-004
  - REQ-UI-THREADS-SAVED-VIEWS-005
---

# Threads Saved Views System Design

## Purpose and boundaries

This design adds user-owned task queries to `/threads`. A query controls the
task scope, filter clauses, sort, and maximum admitted columns. The existing
Threads deck continues to own session selection, viewport activation, layout
composition, and composer disclosure. This design owns persistence of its
presentation preferences with each view.

Threads and the task sidebar share query primitives. They keep separate saved
view collections because their presentation fields and active-view lifecycles
are different. The backend-owned user settings JSON remains the durable source.

The task and workflow systems remain authoritative for every candidate field.
Stored task IDs never grant access to a task or cause a direct task fetch.

## Requirement mapping

| Requirement                      | Design section                                                                                                                    |
| -------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-THREADS-SAVED-VIEWS-001` | [Saved view state](#saved-view-state), [Persistence and synchronization](#persistence-and-synchronization)                        |
| `REQ-UI-THREADS-SAVED-VIEWS-002` | [Candidate projection](#candidate-projection), [Filter catalog](#filter-catalog), [Query pipeline](#query-pipeline)               |
| `REQ-UI-THREADS-SAVED-VIEWS-003` | [Sort, admission, and stable order](#sort-admission-and-stable-order), [Deep links](#deep-links)                                  |
| `REQ-UI-THREADS-SAVED-VIEWS-004` | [Top-bar controls](#top-bar-controls), [Responsive behavior](#responsive-behavior), [Failure and recovery](#failure-and-recovery) |
| `REQ-UI-THREADS-SAVED-VIEWS-005` | [Presentation preferences](#presentation-preferences) |

## Saved view state

The frontend UI slice adds a separate `threadViews` state:

```ts
type ThreadTaskScope =
  | { mode: "all"; taskIds: [] }
  | { mode: "selected"; taskIds: string[] };

type ThreadView = {
  id: string;
  name: string;
  taskScope: ThreadTaskScope;
  filters: ThreadFilterClause[];
  sort: ThreadSortSpec;
  maxColumns: number | null;
  layout: "columns" | "grid";
  autoHideComposer: boolean;
};

type ThreadViewDraft = Omit<ThreadView, "id" | "name"> & {
  baseViewId: string;
};
```

`threadViews` contains saved views, the active view ID, one active draft, the
last sync error, and an order-reset generation. The built-in view uses ID
`view-all-threads`, name `All threads`, task scope `all`, attention sort, and
`maxColumns: 5`. New view definitions use the same limit. A saved `null` value
continues to select no limit.

The saved collection is independent from `sidebarViews`. Shared frontend query
types contain filter operators, values, and sort direction. Sidebar and
Threads define separate dimension and sort-key unions. Existing sidebar wire
keys remain unchanged.

The store reuses the sidebar action semantics:

- view selection persists immediately when there is no active draft;
- edits create or update a persisted draft;
- Save replaces the active saved definition and clears the draft;
- Save as creates and activates another view;
- Discard restores the active saved definition;
- create, rename, and delete use optimistic updates with rollback;
- the 50-view limit is checked in the UI and backend.

While a draft exists, both view pickers disable saved-view selection and show
a localized explanation directing the user to View settings. The existing
Save/Discard actions clear the draft before switching. `setThreadActiveView`
also rejects selection while a draft exists, including same-view selection,
so a stale UI event cannot clear an unsaved draft.

## Persistence and synchronization

The existing `users.settings` JSON object adds these portable fields:

```json
{
  "thread_views": [],
  "thread_active_view_id": "view-all-threads",
  "thread_view_draft": null
}
```

`ThreadView` uses snake-case wire fields:

```json
{
  "id": "view-needs-attention",
  "name": "Needs attention",
  "task_scope": {
    "mode": "selected",
    "task_ids": ["task-a", "task-b"]
  },
  "filters": [],
  "sort": { "key": "attention", "direction": "asc" },
  "max_columns": 3,
  "layout": "grid",
  "auto_hide_composer": true
}
```

The update continues to use `PATCH /api/v1/user/settings`. It adds no table and
no endpoint. Omitted fields keep their stored values. An explicit null draft
clears the draft. Replacing `thread_views` uses complete-list semantics.

Backend models, DTOs, service updates, repository JSON mapping, boot payloads,
and `user.settings.updated` carry the three fields. The backend supplies the
canonical view when stored settings omit the fields or contain an empty list.
It keeps `thread_active_view_id` referentially valid.

The backend rejects these invalid values:

- more than 50 views;
- an empty or duplicate view ID;
- an empty view name;
- more than 20 filter clauses in one view;
- more than 200 selected task IDs in one view;
- a numeric column limit outside 1 through 30;
- an active or draft base ID that does not identify a saved view.

The user-settings revision orders boot hydration, PATCH responses, and live
events. The frontend rejects older settings snapshots. A queued settings sync
serializes rapid view mutations and preserves unrelated user settings.

## Presentation preferences

`layout` and `auto_hide_composer` belong to both saved views and drafts. The
backend `models.ThreadView` and `models.ThreadViewDraft` add these fields; DTOs
already carry those model types. Extend the existing service validation and
store normalization rather than creating another settings endpoint or table.
The request/response shape remains the same through PATCH, boot hydration,
and `user.settings.updated`.

The native boot projection in `backendapp/boot_thread_views.go` also maps
`layout` and `autoHideComposer` for both views and drafts. It emits the
frontend's camel-case state directly, so the API wire mapper cannot repair
fields omitted there.

The backend supplies `columns` and `false` for omitted presentation fields in
stored definitions, canonical defaults, new views, and legacy draft payloads.
It normalizes unknown stored presentation values independently without losing
the rest of the view. Decode these new stored fields tolerantly at the existing
`scanUserSettings` JSON boundary before constructing their typed values; a
malformed new field must not make the whole user-settings read fail. Normal
request decoding and service validation reject explicitly unsupported new
values before persistence. Preserve existing top-level PATCH semantics:
omitted collections/drafts are unchanged, a null draft clears it, and supplied
view lists are complete replacements. Missing nested fields in such a
replacement use the compatibility defaults; they are not a partial view patch.

Keep `DefaultThreadViews` as the backend default owner. Extend the one frontend
wire mapper in `thread-view-wire.ts` for `layout` and `autoHideComposer`, using
the same compatibility fallbacks. `createDefaultThreadView` supplies only the
matching render/new-view shape; it does not write defaults during hydration.
No browser storage fallback or DB schema migration is introduced.

Update every view/draft constructor and clone in `thread-view-actions.ts`,
including optimistic before/after snapshots, failed-write retry payloads,
deferred server state, overwrite, Save as, and Duplicate. Extend
`updateThreadViewDraft` and its exported slice type. A filter-only draft must
carry its base view's presentation fields; a presentation-only patch must
retain all existing query fields. The draft merge in
`thread-view-query.ts:cloneViewWithDraft` returns the effective presentation.
Its existing `queryFingerprint` explicitly selects query fields; keep the new
presentation fields out so editing, saving, rollback, and remote presentation
updates do not reorder task tiles. Real view selection/query edits retain
their existing reset behavior.

Add a Display section to `ThreadsViewEditor`, reusing shared Select/Switch and
the existing Save/Save as/Discard footer. Fields are Layout (Columns/Grid) and
Auto-hide composer. Descriptions explain full-height reading, two-row
monitoring, CI-only collapsed footers, hover/keyboard disclosure, and the
visible-composer touch fallback. Keep layout selection exclusively inside
Display in the existing configurator, with its draft and Save/Discard flow.
The top bar does not duplicate the layout selector.
Keep ordinary desktop controls at 28px and touch controls at least 44px.

Phone/tablet use the existing view drawer and its internal editor page. Phone
layout controls edit the saved wider-screen choice and explain why the phone
continues to show one chat. Auto-hide's helper explains that phone and coarse
pointers retain their normal composer without changing the saved preference.
The grid-height fallback is derived by the board,
with a reason made available to the layout control/editor; it never changes
the draft. Preserve one inset drawer, its header/back controls, one body scroll
owner, safe-area padding, and existing error/retry presentation.

Relabel the editor's `maxColumns` field Maximum chats across all locales,
without changing the internal or wire name. The helper explains that five
means five task tiles in either layout, with the existing hidden-match count.
No automatic new view, limit increase, global preference, or default auto-hide
activation accompanies choosing Grid.

All copy uses `threads` translations. Update English, Portuguese, Simplified
Chinese, the generated Traditional Chinese pair, and pseudo output with the
repository translation tools. Existing synchronized settings revisions,
authorization, rollback, and retry remain the failure/consistency boundary.
The implementation package must test a second client's update and a rejected
rapid edit, not only reload serialization.

## Candidate projection

`selectThreadCandidates` replaces direct construction of the final deck list.
It reads all workflow snapshots for the active workspace. It does not read the
global active workflow filter.

Each `ThreadCandidate` contains only bounded task-list data:

- task ID, title, state, priority, blocked state, and task type;
- task labels, origin, session count, and active-error presence;
- workflow and step IDs and labels;
- linked repository IDs and labels;
- primary session ID, lifecycle state, and pending action;
- primary agent profile ID and label, and executor type;
- review outcome and last activity;
- active subagent and queued prompt counts;
- Git-change, pull-request, and pull-request-attention booleans.

The task-picker projection also keeps task-level foreground activity,
interruption state, final-step state, and compact pull-request information.
These values come from the same WebSocket-backed snapshots as the task title.

The projection uses workflow snapshots, workspace repository/profile stores,
and `TaskStatusSummary`. It does not load a transcript or the session list.
The bounded task DTO adds the primary session's agent profile ID beside the
existing `primary_agent_name`. The filter stores the stable profile ID and
renders the current name from the workspace profile store. Missing data
produces an `unknown` value.

A candidate must satisfy `isThreadTaskEligible`, belong to the active
workspace, be non-archived, and have a primary session ID. This keeps the
canonical view compatible with the current Threads deck.

## Filter catalog

Threads uses the existing clause operators where their value kinds apply:
`is`, `is_not`, `in`, `not_in`, `matches`, and `not_matches`.

The initial dimension registry is:

| Dimension             | Values or meaning                                                          |
| --------------------- | -------------------------------------------------------------------------- |
| `threadStatus`        | `needs_action`, `running`, `waiting`, `ready_for_review`                   |
| `pendingAction`       | `clarification`, `permission`, `none`                                      |
| `taskState`           | Exact persisted task state                                                 |
| `workflow`            | Workflow ID                                                                |
| `workflowStep`        | Workflow-step ID                                                           |
| `repository`          | Any linked repository ID                                                   |
| `primaryAgent`        | Primary agent profile ID, rendered with its current name                   |
| `executorType`        | Primary executor type                                                      |
| `priority`            | Persisted task priority                                                    |
| `blocked`             | Task dependency-blocked boolean                                            |
| `hasQueuedPrompts`    | Queue count is more than zero                                              |
| `hasActiveSubagents`  | Active subagent count is more than zero                                    |
| `hasDiff`             | Bounded Git summary has a change                                           |
| `hasPR`               | Bounded pull-request count is more than zero                               |
| `prNeedsAttention`    | Bounded pull-request attention is true                                     |
| `taskType`            | Standard, pull-request review, or issue-watch task                         |
| `titleMatch`          | Case-insensitive title substring                                           |
| `hasActiveError`      | Compact task summary contains an active error                              |
| `taskLabel`           | Any normalized value in the bounded task label array                       |
| `taskOrigin`          | Persisted manual, agent-created, routine, onboarding, or automation origin |
| `hasMultipleSessions` | Bounded session count is more than one                                     |

All clauses use AND. An `in` clause uses OR across its selected values. A
repository clause matches when any linked repository satisfies the clause.

The editor injects a Threads dimension registry into shared clause editor
primitives. It does not add Threads dimensions to the sidebar registry.

The task picker can clear every checkbox while the user edits a draft. An
empty selected scope is not valid saved data. The editor disables Save and
identifies the task-scope error until the user selects a task or changes the
scope to `all`.

## Query pipeline

The effective view is the active saved view merged with its matching draft.
The page applies this pipeline:

1. Scope workflow snapshots to the active workspace.
2. Project eligible task candidates.
3. Apply the `all` or `selected` task scope.
4. Apply every filter clause.
5. Sort the matching candidates.
6. Reserve one admission slot for a valid deep-link target when necessary.
7. Apply `maxColumns` when it is not null.
8. Reconcile the admitted IDs with stable column order.

The result includes admitted threads, total matching count, hidden matching
count, and the effective view name. `ThreadsBoard` receives admitted threads
only. Hidden candidates do not mount task shells, session-list hooks, or chats.

## Sort, admission, and stable order

Sort keys include attention, last activity, updated time, created time, title,
task state, workflow then step, priority, and primary agent label. Direction
applies to every key. Each comparator uses task ID as its final tie-breaker.

Each sort option supplies a localized description to the shared sort-picker
primitive. The description states the primary order before the user selects
the option.

Attention sort preserves the current deck precedence. It puts explicit person
action first, then running or ready-for-review work, and then waiting work. It
uses activity recency inside each group. Plain `WAITING_FOR_INPUT` does not
become a person-action rank.

The query fingerprint contains the active view ID, effective scope, filters,
sort, and column limit. A fingerprint change or explicit Reapply sort action
increments the order-reset generation. The next render uses the complete
sorted order.

Task and session events do not reset that generation. Existing admitted tasks
that still match keep their positions. Removed tasks leave the order. New
matches fill free admission slots in current sorted order. This rule prevents
a reply from moving the column that owns keyboard focus.

When a cap hides matches, the control shows admitted and hidden counts. Reapply
sort replaces the admitted set with the current sorted prefix and does not save
or change the view definition.

## Deep links

A valid `taskId` URL target can be outside the active task scope or sorted
prefix. The page inserts that candidate into the admitted set and counts it
toward `maxColumns`. If the set is full, the target replaces the last ordinary
admission for that reconciliation pass.

The temporary admission stays until the URL target changes, the user switches
views, or the task becomes ineligible. It does not add the task ID to the saved
scope. Existing `sessionId` validation then runs after the target column loads
session membership.

## Top-bar controls

`KanbanHeader` gains optional task-listing control slots. Desktop and tablet
place the slot before `ViewToggleGroup`. Phone groups the page label and active
view in the title control, with inline pagination and no scrolling action strip.

Threads supplies the existing view selector/settings controls:

```text
[All threads v] [View settings] [Listing views]
```

The first control switches saved views and exposes New view. The second opens
the editor and shows a dirty indicator for a draft. Its accessible name
includes the active view and admitted count.

Layout selection stays inside Display as described in
[Presentation preferences](#presentation-preferences). Touch layouts keep
Display in the existing view drawer.

The existing `KanbanDisplayDropdown` does not render on Threads. Workflow,
repository, and task filters on that surface create a second query owner.
Other task-listing pages keep their current header controls.

The editor reuses view header, clause, sort, and responsive surface primitives
after they accept a surface-owned registry and state adapter. It omits sidebar
grouping, collapsed groups, and task-row presentation.

The desktop popover uses the shared popover background with a stronger border,
ring, and shadow. This treatment separates the editor from dark page surfaces.

Each task-picker row composes the shared `TaskStateIcon` and `PRTaskIcon`.
`TaskStateIcon` uses the bounded task and session state from the candidate.
`PRTaskIcon` receives compact summary data and keeps its shared color and
pointer or touch disclosure. The row shows the current workflow-step label.

## Responsive behavior

Desktop uses an anchored popover with a bounded height and one vertical scroll
owner. The task-scope row opens a task-picker page inside the same popover.
The picker has title search, Select all, Clear all, and checkbox rows.

Tablet uses the same top-bar position with 44-pixel triggers. Its trigger opens
the mobile drawer because a touch pointer does not use the desktop popover.

Phone groups the Threads page label and active view in one unboxed, stacked
button in the top bar, outside a scrolling action strip. The button opens one
inset bottom drawer. The drawer has three internal
pages: saved-view selection, view editor, and task picker. Back navigation
changes the drawer page instead of opening another overlay.

The drawer header stays fixed. Its body uses
`min-h-0 flex-1 overflow-y-auto overscroll-contain`. The body clears the bottom
safe area. Rows and standalone actions have 44-pixel hit areas. Long view and
task names truncate without horizontal document overflow.

The task-column pager remains unchanged. `maxColumns` limits pager items, while
the viewport activation contract keeps only the nearest phone column
detail-active.

## Failure and recovery

- Before user settings hydrate, the page uses the canonical view as a render
  placeholder and does not write it back automatically.
- A save error rolls back views, active ID, and draft to the last backend
  snapshot. The existing toast bridge shows a recoverable error.
- On phones, a compact status icon beside the view control announces sync
  failure. Retry and Dismiss live inside the saved-view drawer's scrolling body,
  not the fixed-height page title row. Desktop and tablet keep inline recovery.
- An unknown filter dimension, operator, sort key, or task ID is ignored during
  frontend normalization. Known valid fields remain available for editing.
- If the active ID is invalid, normalization selects the canonical view. It
  does not erase other valid saved views.
- A selected task that leaves eligibility disappears without removal from the
  saved scope. Normal task events can make it reappear.
- If a filter removes every candidate, the empty deck keeps the view controls
  and reports the effective view name.
- Workspace changes cancel stale snapshot derivations through the existing
  workspace generation and scope checks.

## Security and privacy

User-settings handlers authorize the current user before reading or writing
views. Stored task IDs are preferences only. They never bypass workspace task
authorization or trigger a task fetch by ID.

View names and task IDs are not emitted in logs. Validation logs can include
the field name, count, and stable error code. Filter evaluation uses bounded
task metadata already authorized for the active workspace.

## Verification design

Backend tests cover defaults, limits, active-ID validation, partial PATCH
semantics, JSON round trips, revision updates, and settings broadcasts.

Frontend unit tests cover wire mapping, normalization, optimistic rollback,
scope semantics, every filter dimension, sort ties, admission limits, deep-link
inclusion, hidden counts, and stable-order resets.

Desktop component and Playwright tests cover top-bar placement, saved-view
switching, task metadata, sort descriptions, a three-column cap, persistence,
and deep links. Tests also prove that hidden matches mount no session-list or
chat consumer.

Mobile Playwright covers the top-bar entry, one-drawer navigation, 44-pixel
rows, task selection, filter and limit parity, safe-area clearance, focus
return, and zero document overflow.

## Related decisions

- [Surface-owned Saved Task Views](../../../decisions/2026-08-31-surface-owned-saved-task-views.md)
- [Backend-owned Portable User Settings](../../../decisions/0041-backend-owned-portable-user-settings.md)
- [Viewport Activation Owns Threads Session Streams](../../../decisions/2026-08-28-viewport-activation-owns-thread-streams.md)

## Implementation plans

- [Threads layouts and composer disclosure](../../../plans/threads-layouts/plan.md)
  implements presentation preferences under `REQ-UI-THREADS-SAVED-VIEWS-005`.
- [Threads saved views](../../../plans/threads-saved-views/plan.md) records the
  completed query, editor, and persistence foundation. Its completed results
  do not claim coverage of the later presentation additions.
