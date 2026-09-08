---
spec: docs/specs/ui/requirements/task-surface-refresh.md
created: 2026-07-27
status: building
---

# Implementation Plan: Task Surface Foreground Refresh and Mobile Create Action

## Overview

Introduce one frontend foreground lifecycle primitive, migrate task-detail
backfills to it, and make Kanban and List expose authoritative refresh
callbacks. Add a phone-native pull surface around the existing List and Kanban
scroll compositions, then reuse the shipped Kanban FAB geometry for task
creation from List. No backend route, WebSocket event, persistence, or public
API changes are required.

## Frontend

### Foreground lifecycle and request coalescing

- Add `apps/web/hooks/use-foreground-refresh.ts` to listen for visible
  `visibilitychange`, `window.focus`, `pageshow`, and `online`.
- Keep the latest callback in a ref, suppress overlapping calls, and deduplicate
  the burst of browser events emitted by one foreground transition. The hook
  must not depend on `connection.status`, because a suspended socket can still
  appear connected.
- Replace the visibility-only listeners in
  `apps/web/hooks/use-task-sessions.ts`,
  `apps/web/hooks/domains/session/use-session-messages.ts`, and
  `apps/web/hooks/domains/session/use-queue.ts` with the shared lifecycle hook.
  Preserve each hook's current forced-reload and error behavior.
- Refactor the task loader in
  `apps/web/components/task/task-page-content.tsx` into a stable forced-refresh
  callback. Foreground recovery reloads the active task without changing the
  selected task/session and discards a response if navigation changed its task
  identity before completion.

### Kanban and List authoritative refresh

- Refactor
  `apps/web/hooks/domains/kanban/use-all-workflow-snapshots.ts` to expose a
  promise-returning forced refresh for the current workspace. Keep its existing
  workspace-generation guard and in-flight task preservation.
- When refreshing the workflow selected as the active Kanban workflow, update
  both `kanbanMulti.snapshots[workflowId]` and the active `kanban` snapshot from
  the same guarded response. This keeps preview/create consumers consistent
  with the rendered board.
- Use the shared foreground hook from the Kanban data owner so Home refreshes
  whether or not WebSocket status changes. A partial or failed refresh retains
  prior snapshots and allows retry.
- Extend the List page's existing `fetchTasks` callback in
  `apps/web/app/tasks/tasks-page-client.tsx` with silent foreground failure
  handling while retaining toast feedback for explicit/manual loads. Reuse its
  request sequence and workspace guard so stale requests cannot commit.
- Invoke the same List callback after a successful List task creation so the
  new row appears only when it matches the current filters and page.

### Mobile pull surface

- Add a small pure gesture-state helper under
  `apps/web/lib/mobile/pull-to-refresh.ts` and a responsive component under
  `apps/web/components/mobile/pull-to-refresh.tsx`.
- The component activates only for one-touch, primarily vertical downward
  movement beginning with the active vertical scroll owner at `scrollTop ===
  0`. It cancels for horizontal movement, multi-touch, task drag interactions,
  and movement below the threshold.
- Keep rendered content mounted during refresh. Show an accessible top
  indicator for pull progress, release readiness, and refreshing; settle it
  after success or failure.
- Wrap the List scroll surface and the mobile Kanban content without adding a
  second vertical scroll owner. The current List `main` and current Kanban
  column remain authoritative scroll containers.

### Mobile List create action

- Reuse `apps/web/components/kanban/mobile-fab.tsx` from
  `apps/web/app/tasks/tasks-page-client.tsx`; do not introduce a second FAB
  style.
- Mount the existing `TaskCreateDialog` for List with the active workspace and
  workflow context. When no workflow filter is selected, preserve the existing
  dialog workflow-resolution behavior rather than persisting a new filter.
- Keep the control phone-only, fixed above the bottom safe area, at a 56px
  touch target, with the existing **Add task** accessible name.

## Mobile design contract

- **Desktop outcome / mobile entry:** foreground focus refresh works on all
  viewports. Phone users additionally pull the current task surface or tap the
  bottom-right **Add task** FAB in List.
- **Nearest exemplar:** `apps/web/components/kanban/mobile-fab.tsx` supplies FAB
  geometry and safe-area behavior; the current mobile Kanban column and mobile
  List `main` supply scroll ownership.
- **Hierarchy / primary action:** tasks remain the focal content. Create is the
  persistent thumb-reachable primary action; refresh is a top-edge recovery
  gesture with visible progress.
- **Presentation:** inline pull indicator plus the existing fixed FAB. A drawer
  or full-height surface would add unnecessary navigation for these frequent,
  shallow actions.
- **Scroll and viewport:** no new document scroll or nested vertical scroller.
  The gesture interrogates the touched view's existing vertical scroll owner,
  uses overscroll containment, and leaves the FAB clear of
  `env(safe-area-inset-bottom)`.
- **Shared state:** data loaders, filters, workspace/workflow selection, and
  create behavior remain shared. Only the pull gesture and FAB presentation
  are phone-specific.
- **Mobile proof:** Playwright creates a task through List's FAB, pulls List and
  Kanban to recover dropped updates, checks a mid-scroll/horizontal gesture
  does not refresh, and asserts no horizontal document overflow.

## Tests

- **What:** foreground events invoke one refresh independent of WebSocket
  status; burst events and in-flight requests coalesce; hidden visibility
  events do nothing.
  **File:** `apps/web/hooks/use-foreground-refresh.test.ts`.
  **How:** Vitest hook tests with controlled promises and synthetic browser
  lifecycle events.
- **What:** sessions, chat messages, and queued messages backfill on window
  focus as well as visibility without duplicate concurrent requests.
  **Files:** `apps/web/hooks/use-task-sessions.test.ts`,
  `apps/web/hooks/domains/session/use-visibility-backfill.test.ts`,
  `apps/web/hooks/domains/session/use-queue.test.ts`.
  **How:** existing hook fakes extended with focus/burst cases.
- **What:** forced Kanban refresh updates current workflow snapshots, preserves
  mutations arriving during an in-flight request, and rejects stale workspace
  results.
  **Files:**
  `apps/web/hooks/domains/kanban/use-all-workflow-snapshots.test.ts`,
  `apps/web/hooks/domains/kanban/use-all-workflow-snapshots-inflight.test.ts`.
  **How:** controlled API promises against the existing store harness.
- **What:** pull state crosses the threshold only for eligible vertical
  gestures and resets on cancellation, horizontal movement, multi-touch, and
  request settlement.
  **File:** `apps/web/lib/mobile/pull-to-refresh.test.ts`.
  **How:** table-driven Vitest tests of the pure gesture state transitions.

## E2E Tests

- **Scenario:** mobile List FAB opens the real create flow and a matching task
  appears in the current List after creation.
  **File:** `apps/web/e2e/tests/task/mobile-task-surface-refresh.spec.ts`.
  **What to verify:** 56px FAB, safe viewport containment, dialog context,
  created row, and no horizontal document overflow.
- **Scenario:** a dropped task update is recovered by pulling mobile List and
  mobile Kanban from scroll-top; mid-scroll and primarily horizontal gestures
  do not start a refresh.
  **File:** `apps/web/e2e/tests/task/mobile-task-surface-refresh.spec.ts`.
  **What to verify:** indicator state and authoritative task content after the
  pull while the current view/workflow/column remains selected.
- **Scenario:** Kanban/Home and List recover a deliberately dropped task event
  when the desktop page receives a foreground signal while its WebSocket still
  exists.
  **File:** `apps/web/e2e/tests/task/task-surface-foreground-refresh.spec.ts`.
  **What to verify:** stale row/card becomes current without `page.reload()`.
- **Scenario:** task-detail chat recovers a deliberately dropped persisted
  message on desktop focus.
  **File:** `apps/web/e2e/tests/task/task-surface-foreground-refresh.spec.ts`.
  **What to verify:** the message appears without navigation/reload and the
  selected task/session does not change.
- Extend `apps/web/e2e/helpers/ws-drop.ts` only as needed to drop named task or
  message notifications deterministically; the test must assert that the
  intended frame was actually dropped.

## Implementation Waves And Parallel Candidates

The tasks are sequential because Task 02 consumes the foreground primitive from
Task 01, and E2E in Task 03 covers both.

- [x] [task-01-foreground-recovery](task-01-foreground-recovery.md)
- [x] [task-02-listing-pull-and-create](task-02-listing-pull-and-create.md)
- [ ] [task-03-task-surface-e2e](task-03-task-surface-e2e.md)

## Risks

- Browser focus, visibility, page-cache, and online events commonly arrive in
  a burst; insufficient deduplication would multiply expensive snapshot and
  message requests.
- Kanban's horizontal Embla gesture and task drag-and-drop share the same touch
  surface as pull-to-refresh. Direction locking and scroll-top detection must
  happen before preventing default browser behavior.
- A snapshot refresh can race task creation, task WS updates, or workspace
  navigation. Existing generation and in-flight mutation guards must remain
  intact for both the multi-workflow and active snapshots.
- E2E must prove notifications were dropped; otherwise live WebSocket delivery
  could make foreground recovery tests pass without exercising the new path.
