---
status: draft
system: ui
requirements:
  - REQ-UI-PR-TASK-STATUS-SUMMARY-001
---

# PR Task Status Summary System Design

## Purpose and boundaries

The UI system owns the shared task-indicator disclosure. The disclosure is used
by the sidebar, Kanban cards, and rich task lists.

The task status projection remains the bounded source for inactive task rows.
The GitHub integration remains the source for stored `TaskPR` records. This
design does not add full PR records to `TaskStatusSummary`.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-PR-TASK-STATUS-SUMMARY-001` | [Disclosure data flow](#disclosure-data-flow), [Disclosure scrolling](#disclosure-scrolling), [Author presentation](#author-presentation), [Mobile behavior](#mobile-behavior), [Failure and recovery](#failure-and-recovery) |

## Current data split

`TaskStatusSummary.pull_request` gives every task row compact PR data. The
frontend maps this data to `prInfo`, which contains the PR number, state, and
aggregate state.

`taskPRs.byTaskId` contains full `TaskPR` records. These records contain the
title, `author_login`, review state, CI state, and merge state. The cache also
records the workspace and workspace-context generation that owns its rows, so
task-detail surfaces cannot reuse records from an earlier context. Some routes
load all workspace records, while task-detail surfaces load records for the
active task.

The current fallback indicator uses `prInfo` without a tooltip. The full-data
indicator uses `PRTaskIcon` with `PRTaskStatusSummary`. This split causes the
inconsistent behavior.

## Components and responsibilities

- `TaskContributionIcons` passes compact `prInfo` to the GitHub task indicator.
- `PRTaskIcon` owns one visual trigger for compact, loading, unavailable, and
  full-data states.
- A task-scoped hydration hook reads the current workspace-scoped
  `taskPRs.byTaskId` entry and uses `listTaskPRs([taskId])` only when full data
  is absent.
- `useChangeRequestTaskTooltipState` opens on a mouse pointer or visible
  keyboard focus. Its `onOpen` callback starts hydration.
- `PRTaskStatusSummary` derives GitHub presentation data from each `TaskPR`.
- `ChangeRequestTaskStatusSummary` renders the shared summary structure and an
  optional author identity.
- `PRCIPopover` and `PRStatusChipDrawer` provide the existing detailed touch
  path after the user opens a task.

### Status-color precedence

`getPRStatusColor` in `apps/web/components/github/pr-task-icon.tsx` is the
single source for the task-row GitHub pull-request icon color. It preserves
terminal states first and active merge-queue membership next. For an open draft
without an active queue entry, it returns the muted color before non-terminal
review or check failures. The separate `PRStatusChip` keeps its CI-specific
status derivation and is not a source for the task-row icon color.

## Data and contracts

The design uses the existing `GET /api/v1/github/task-prs?task_ids=<id>`
endpoint. This endpoint returns persisted task associations. It does not start
the workspace-wide stale-record refresh.

The frontend store remains the cache. A successful load adds each returned
`TaskPR` through the existing store action. The client-only cache metadata
tracks workspace context and association deletion tombstones; the
implementation adds no database field, task-summary field, or public API.

Every `TaskPR` API and WebSocket payload includes its owning `workspace_id`.
The backend exposes that identity to the WebSocket broadcaster for typed
in-process events and routes PR updates and detachments through the
fail-closed workspace path when authentication is enforced. The frontend
applies an update only when its workspace ID matches the active workspace;
missing or mismatched updates are ignored before the cache changes.

`ChangeRequestTaskStatusSummaryData` gains an optional `author` value. GitHub
sets this value from `TaskPR.author_login`. Other providers can omit it.

## Disclosure data flow

1. The task row renders the compact PR indicator from `prInfo`.
2. A mouse pointer enters the indicator, or keyboard focus becomes visible.
3. The tooltip opens immediately and shows the PR identity with a loading state.
4. The hydration hook checks the current workspace-scoped
   `taskPRs.byTaskId` entry again. It stops if full data is available.
5. The hook acquires one in-flight request for the active store, workspace,
   workspace-context generation, and task. Other mounted indicators reuse that
   request.
6. The response is ignored after a workspace or task context change. Existing
   store records win over matching response records, and deletion tombstones
   prevent a late response from resurrecting an association. New response
   identities can fill missing multi-PR siblings.
7. The full store data replaces the loading content while the tooltip remains
   open.

The in-flight registry is scoped to the Zustand store instance. It removes a
request after settlement. The store is the only settled cache.

## Author presentation

The structured task summary shows `task:byAuthor` below the PR title when
`author_login` is non-empty. The author line uses normal text, so its meaning
does not depend on color or an icon.

The GitHub CI popover header receives the same optional author. This header is
shared by the desktop status popover and the coarse-pointer PR-status drawer.

## Disclosure scrolling

`PRTaskIconTooltip` owns the desktop summary shell. Its maximum height uses
`--radix-tooltip-content-available-height`, with a dynamic viewport limit and
space for the border and collision margin. A single inner body owns vertical
scrolling. It contains both the PR summaries and automation details.
The outer shell retains the tooltip arrow without clipping it.

The shell accepts pointer events locally. The shared Tooltip primitive remains
unchanged. An opt-in hoverable mode in `useTaskIconTooltipState` tracks trigger
and content presence. It reuses `useHoverPopover` for the short close delay
across the six-pixel gap. Other task indicators retain their current defaults.
Hydration remains trigger-owned and does not repeat when the pointer enters content.

The scroll body accepts keyboard focus and has an accessible name from existing
localized PR status copy. Focus can move from the trigger into the body without
closing the disclosure. Escape dismisses it until a new disclosure interaction.
Leaving both regions closes it after the gap delay. Unmount clears timers.

The existing `PRTaskIconDrawer` keeps its fixed header and single scrolling body.
Its `80dvh` limit and shared Drawer safe-area handling contain long summaries.
No new provider request, persistent state, or mobile surface is necessary.

## Failure and recovery

The tooltip does not show a toast for a passive disclosure error. It shows a
localized unavailable state and keeps the compact PR identity visible.

An error or empty response removes the in-flight entry. A later hover or focus
starts another request. Pointer leave does not cancel a request because the
result remains useful to other task surfaces.

HTTP responses cannot replace a matching store record that arrived through a
newer WebSocket path. A workspace or workspace-context change prevents response
application, and a deletion tombstone prevents a late HTTP row from restoring a
removed association.

## Accessibility

The compact fallback and full indicator use the same focusable semantic
trigger. The trigger remains mounted while compact content changes to the full
summary, so keyboard focus and an open tooltip survive hydration. Keyboard
focus starts the same load as mouse hover. Escape dismisses the tooltip, and
later focus can open it again.

Loading and unavailable content uses localized text. The author identity is
part of the visible summary and the accessible content.

## Mobile behavior

The task row keeps its primary navigation action and touch geometry. The
existing task-icon drawer also provides explicit coarse-pointer disclosure,
as specified by the [sidebar automation design](../../integrations/system-design/github-pr-merge-queue.md).
It uses the same summary content as the desktop tooltip.

After navigation, the existing `PRStatusChipDrawer` is the author-detail entry
point. The drawer keeps its current safe-area handling and internal scroll
owner. Its PR content shows the author below the title.

The closest shipped mobile surfaces are
`session-task-switcher-sheet.tsx` for task navigation and
`pr-status-chip.tsx` for the coarse-pointer drawer.

## Tests

Unit and component tests cover request deduplication, cache reuse, workspace
and task-context changes, WebSocket-before-HTTP races, deletion tombstones,
retry behavior, keyboard-focus continuity, and author omission.

Desktop Playwright coverage starts on an unrelated task detail route. It opens
an inactive task indicator that initially has only compact summary data.

Mobile Playwright coverage taps the inactive task row, opens the existing
PR-status drawer, and checks the author identity and page containment.

Scrolling coverage seeds five linked PRs with wrapped titles and automation
details. Desktop coverage measures viewport containment, crosses the trigger
gap, and uses real wheel input to reach the final entry. Keyboard coverage
moves focus into the scroll body and uses scroll keys before Escape.
Phone coverage taps the existing task-icon drawer and reaches the last entry.
Short-content coverage confirms that no unnecessary scroll region appears.

## Related designs

- [Bounded Task Status Delivery](../../platform/system-design/bounded-task-status-delivery.md)
