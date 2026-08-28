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
| `REQ-UI-PR-TASK-STATUS-SUMMARY-001` | [Disclosure data flow](#disclosure-data-flow), [Author presentation](#author-presentation), [Mobile behavior](#mobile-behavior), [Failure and recovery](#failure-and-recovery) |

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

The compact task-row indicator remains passive on coarse pointers. The task row
keeps its existing primary navigation action and touch geometry.

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

## Related designs

- [Bounded Task Status Delivery](../../platform/system-design/bounded-task-status-delivery.md)
