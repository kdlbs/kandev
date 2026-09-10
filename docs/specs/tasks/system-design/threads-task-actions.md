---
status: current
system: tasks
requirements:
  - REQ-TASKS-THREADS-ACTIONS-001
  - REQ-TASKS-THREADS-ACTIONS-002
  - REQ-TASKS-THREADS-ACTIONS-003
  - REQ-TASKS-THREADS-ACTIONS-004
---

# Threads Task Actions System Design

## Ownership and dependencies

The task system owns the action target and delegates execution to the same
task operations used by existing task menus. Threads supplies task IDs and
presentation anchors. Its conversation selection remains local to each column.
No endpoint, schema, permission, or plugin contract changes are needed.

The [Threads deck](../../ui/system-design/threads-conversation-deck.md) and
[saved views](../../ui/system-design/threads-saved-views.md) own task admission,
stable order, and viewport activation. The parent mobile Threads/shared-header
implementation is integrated without recreating its header, picker or swiper.
The [plan](../../../plans/threads-task-actions/plan.md) records the pinned parent
commit, merge, work orders and verification results.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-THREADS-ACTIONS-001` | Shared task operations; menu composition |
| `REQ-TASKS-THREADS-ACTIONS-002` | Target lifetime; shared task operations; failure behavior |
| `REQ-TASKS-THREADS-ACTIONS-003` | Admission and selection recovery; state delivery |
| `REQ-TASKS-THREADS-ACTIONS-004` | Header entry points; phone composition; dismissal and focus |

## Existing implementation boundaries

| Existing owner | Reuse in this feature |
| --- | --- |
| `components/task/task-switcher-context-menu.tsx` | Shared context items, movement dispatch, eligibility, and open/close behavior |
| `components/task/task-item-menu-button.tsx` | Visible ellipsis, localized Task actions name, keyboard and touch entry |
| `task-priority-context-menu.tsx`, `hooks/use-update-task-priority.ts` | Four tokens/current marker, persistence, event-based display, failure toast |
| `task-move-context-menu.tsx`, `hooks/use-task-workflow-move.ts` | Workflow/step choices, no-step/current/auto-start states, existing move transport and errors |
| `task-switcher-link-menu.tsx`, `task-session-sidebar-link-actions.ts`, `task-session-sidebar-task-linking.ts` | First-party availability, captured link target, immutable plugin context, provider icons and dialog handoff |
| `task-switcher-action-items.tsx`, `task-archive-confirmation.tsx`, `task-delete-confirm-dialog.tsx` | Archive preference/classification and existing deletion consent |
| `hooks/use-task-actions.ts`, `hooks/use-task-removal.ts` | Domain mutation APIs and shared successful-removal cleanup |
| `app/threads/threads-page-client.tsx`, `lib/threads/stable-order.ts` | Workspace-scoped snapshots, saved-view query, stable admitted order |

Paths without a prefix in this table are under `apps/web/components/task/`;
all other source paths are relative to `apps/web/`.

## Target lifetime

`useTaskManagementFlow` is mounted once above the removable column list by
`ThreadTaskActionsProvider`. The task-owned `TaskManagementSurface` composes
existing hooks and confirmation/link components. It does not import Threads,
derive task IDs from sessions, or implement API requests. `useTaskMenuActions`
owns the shared archive/delete lifecycle and is consumed by this surface and
the existing task-session sidebar. Threads contains no mutation handlers.

The flow stores an immutable `{ taskId, workspaceId }` and a small presentation
stage; its entry adapter stores the originating trigger. `resolveTaskMenuTarget`
reads the target's latest task record and workflow metadata by that ID from the
scoped workflow snapshots, using the existing task lookup conventions. It
derives the required `TaskSwitcherItem` fields
from the real task record: priority, workflow and step IDs, archive state,
repository links, primary executor type, and task-wide foreground activity.
`ActiveThread` is a summary and lacks several of these fields; it is not a
stand-in for the task record. No background transcript or per-column task fetch
is introduced to populate a menu.

Opening A captures A. Re-rendering the page with B visible cannot replace the
captured identity. Display values and eligibility can refresh for A. Before
dispatch or confirmation, resolve A again and check current availability. Close
an unsubmitted flow when its workspace changes or authoritative state removes
or archives A. Mere admission/filter loss does not destroy the flow. A response
already in flight retains its captured target. Provider forms live in a child
keyed by workspace/task identity, while the pending mutation owner lives above
that child. An old link completion cannot close or populate a newer target's
form. Invalid workspace/target state closes the stage permanently, including
across a later return to that workspace.

Store the pending target/operation separately from the visible stage: deletion
confirmation currently closes when submitting. Clearing a dialog must not
clear the identity needed by its asynchronous result. Archive closes its stage
before awaiting the shared operation, so its late result cannot close a newer
menu. Shared ref guards prevent duplicate pending dispatch. Never pass sidebar
`selectedTaskIds` into the Threads entry point.

## Shared task operations

Use the existing movement fallback from `TaskMoveItems` for both destinations:
`moveTasks([taskId], workflowId, stepId, destination)`, obtained from
`useTaskWorkflowMove()`. That is already
the shared task-menu path for consumers without the sidebar's optimistic
same-workflow adapter. It preserves the server's workflow transition and WIP
rules and avoids adding an optimistic Threads-only list mutation. Retain
`useMoveToStep` for existing consumers that already use it.

Priority uses `useUpdateTaskPriority`; the returned Promise alone is not proof
of persistence, because the hook owns its error toast. Current values follow
the confirmed store/event path. Move failures already toast and rethrow; catch
the rejection at the shared menu boundary without a duplicate toast.

Link choices use `selectTaskLinkActions`, `useSidebarLinkActions`,
`useSidebarTaskLinking`, and `useTaskPluginLinkActions`. Reuse `SidebarLinkDialogs`
or extract its existing provider-only composition without cloning forms.
Capture the task's repository context and close the menu before launching a
provider or plugin surface. Plugin registrations and visibility stay reactive.
A workspace change dismisses the old flow before a new plugin context can run.

Archive uses `TaskArchiveConfirmation`, which already owns the user preference,
subtask classification, in-flight warning, and consent. On phone, the simple
confirmation occupies the task drawer body via its `inline` and `renderInline`
support. A
classification requiring the existing full dialog closes the drawer before
opening that dialog. Classification remains in the shared confirmation adapter.
If filtering removes its header while classification loads, the confirmation
anchor falls back to the surviving trigger without changing the captured task.
Delete closes the menu and opens `TaskDeleteConfirmDialog` for the captured
task. Keep existing cascade/discard options and backend dirty-worktree errors.

The domain APIs remain in `useTaskActions`. Reuse `useTaskRemoval` for cleanup
of successful archive/delete operations and cascade trees. Its explicit
`stayOnListing` option suppresses
task-detail routing and session loading, not shared cache cleanup. Defaults
preserve existing sidebar/task-detail navigation, including archive rollback.
Threads must not call `useArchiveAndSwitchTask` with its task-detail navigation
behavior or temporarily rewrite global selection to suppress that behavior.
Capture descendants before a cascade can prune the snapshots. Reuse existing
task lifecycle events for recent/sidebar/session cleanup.

## Menu composition

Extract a focused shared task-management item composition containing exactly
the requested six groups. Reuse the existing priority, movement, link, archive,
and delete items and their action callbacks. Keep the full switcher menu's
default composition for its other consumers. A small explicit composition
choice is sufficient; a new action registry or plugin API is unnecessary.

Desktop uses the existing ContextMenu primitives. `TaskContextMenuSubContent`
portals and bounds shared task submenus to the viewport with an internal
scroller, avoiding a transformed parent menu as their containing block. Phone
uses a presentation adapter over those same option/availability sources. Where
current components mix option derivation with Radix markup, extract only that
derivation for both renderers. Do not duplicate workflow filtering, link
eligibility, priority tokens, confirmation rules, or mutation dispatch in the
phone component. Disabled current-step and no-step states remain visible.

## Header entry points

Attach the fine-pointer context trigger only to the task header's title/metadata
region. Exclude session controls and interactive/editor descendants. The chat
body, composer, and deck scroll container are outside that trigger. Menu portal
events must not reach header navigation handlers, while normal board touch
scroll remains untouched. Threads has no task-row drag sensor to cancel.

Reuse `TaskMenuButton` with `visible` enabled in the existing header row next to
Open task. Extend it with a controlled opener/ref and sizing hooks if required,
preserving its existing default behavior for other callers. Keyboard opening
anchors to the button's bounds rather than synthetic pointer coordinates
`0,0`. Mobile uses a dialog popup name/state; desktop uses menu semantics.

The parent `MobileThreadColumnHeader` retains its title picker, Open task,
status, and session control. Its title remains flexible and bounded to two
lines beside the two explicit icon controls. The page's compact shared topbar
and `ThreadsBoard.renderHeader` inline pagination are unchanged.

## Phone composition

Use `useResponsiveBreakpoint`: phone or a coarse-pointer menu entry uses the
task-domain drawer adapter; fine-pointer desktop keeps compact Radix menus.
This explicitly covers the 640-767px gap in the existing global menu CSS,
whose bottom-sheet overrides only apply below 640px. Keep the new behavior
scoped to the shared task-management surface.

The curated exemplars are `components/kanban/mobile-menu-sheet.tsx` for its
inset card, fixed header, `min-h-0` body and safe-area clearance, and
`components/task/mobile/mobile-picker-sheet.tsx` for short task-domain choices
and the parent's `onCloseAutoFocus` support. Reuse the Drawer primitive's inset
geometry. Bound the whole surface using `dvh` and safe-area offsets; override
legacy `vh` caps only on this surface. Use intrinsic height for short menus and
one `overflow-y-auto overscroll-contain` body for long ones.

This is a temporary task decision, so an inset drawer fits the depth. Its
title identifies the target task. A small page state contains root, priority,
current-workflow steps, workflow list, selected workflow's steps, or link
choices. Back moves up one page; selecting a workflow keeps the same drawer
open for its steps. No portaled submenu mounts inside this phone adapter.
Nested pages reset body scroll on entry and restore the originating row on
Back. Long labels wrap or truncate within `min-w-0` while keeping their full
accessible name and visible state marker.

Touch triggers and navigation/choice rows have 44px minimum hit areas. Apply
these via the phone/coarse-pointer variant so the fine-pointer base remains
compact. The drawer owns vertical drag/scroll only while open. No new gesture
listener, pointer capture, touch cancellation, or `touch-action: none` is added
to the native horizontal swiper.

## Admission and selection recovery

Let `queryThreadView` and `useStableThreadOrder` process confirmed shared state
as today. Changes to priority can affect existing saved-view filters; moves,
archive, delete, external updates, and manual filter changes use the same
membership reconciliation. Do not invent a parallel admitted list or sort.

`resolveRemainingThreadId(previousOrder, nextOrder, currentTaskId)` implements
the requirement's successor/predecessor rule. `useThreadSelectionRecovery`
keeps the last committed order and viewport anchor in the board. Phone scroll
events read the same nearest-center geometry as the parent's mobile position.
Desktop retains the interacted column while visible, otherwise the nearest
visible column. This anchor supports recovery; it does not replace viewport
activation or session selection.

When the anchor survives, preserve it and the local scroll offset if preceding
columns disappear. When it leaves, select the deterministic survivor and
position its shell through the existing board navigation path. A new user
swipe/focus action before a delayed response wins if its task survives. Keep
the parent's pagination and detail activation derived from resulting scroll
geometry, including during a held swipe. Reconcile once per actual membership
change, without render/effect loops or a live rerank.

After authoritative loading yields no tasks, render `ThreadsEmptyState` with a
programmatic focus target. Preserve loading separately. The page and its URL
resolution are unchanged: existing temporary deep-link admission remains
authoritative, and a filtered-but-temporarily-admitted task is still a survivor.
The existing page resolver ignores unavailable targets; no additional URL
rewrite is required for this action entry point.

## Dismissal and focus

Escape on a nested phone page goes Back; Escape at root closes. Capture the
nested Escape before the Drawer primitive closes the entire surface. The
visible Back control has the same behavior, and outside/backdrop or drawer
dismissal closes the flow without dispatch. Existing provider dialogs own
their own cancel and Escape behavior after handoff.

Focus return runs after the final surface closes, not during a menu-to-dialog
handoff. Return to the original connected, visible trigger if it is still the
reader's context. Otherwise focus the current surviving thread's overflow; if
the deck is empty, focus its empty-state container. Use `preventScroll` so an
offscreen originating column cannot pull the phone back. On breakpoint changes,
resolve the surviving responsive trigger by task identity. If a newer user
interaction already owns focus, leave it there.

## State delivery and failure behavior

Use existing task mutation clients and workspace task events to update the
single/multi-workflow snapshots and existing linked-reference stores. The menu
does not add a store, subscription, polling loop, or task/session persistence.
Shared removal cleanup is idempotent if a task event beats the HTTP response.
Failure paths preserve confirmed values, release pending state, and display the
shared localized error. Add generic error feedback at the shared action boundary
only where the reused API currently propagates an error without visible UI.

Server authorization remains final. Re-check availability for the captured
task before dispatch, and exercise rejection and provider disappearance in
tests. No new permission grants or remote writes beyond the selected existing
operation are introduced.

## Verification design

Focused TDD covers target changes during menus/confirmations, live eligibility,
shared mutation success/failure, pending-operation guards, and deterministic
recovery. Browser tests use real isolated task fixtures and perform all six
actions through the UI on both desktop and mobile, asserting the target result,
an unaffected sibling, and persisted state after reload. Existing mocked
integrations exercise link submission without production credentials.

Geometry tests cover 360px and 320px widths, long labels/lists, the 640-767px
phone range, and a coarse-pointer tablet. Assert active-surface containment,
one body scroller, hitboxes and actual hit testing, safe-area clearance, and no
document horizontal overflow. Exercise nested Back/Escape, cancelled destructive
actions, dialog handoff, removed-trigger focus, and real swiping before and
after menu use. Inspect newly rendered phone screenshots during implementation.
Exact test names and commands belong to the linked work orders.

## Related decisions

- [Viewport Activation Owns Threads Session Streams](../../../decisions/2026-08-28-viewport-activation-owns-thread-streams.md)
- [Separate Task Summary and Session Stream Traffic](../../../decisions/2026-08-01-separate-task-summary-session-stream-traffic.md)

These boundaries are reused. This delivery does not require a new ADR.
