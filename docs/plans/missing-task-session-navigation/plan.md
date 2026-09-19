---
spec: docs/specs/tasks/requirements/missing-task-route-recovery.md
created: 2026-08-12
status: complete
---

# Implementation Plan: Missing Task Session Navigation

## Overview

Guard route initialization so that stale route props cannot replace a later sidebar selection. Add unit coverage for the guard and a session-backed browser regression.

## Root cause

The missing route keeps its task ID in `TaskPageContent` after an in-place sidebar switch. The sidebar updates the store and URL without a route remount.

A loaded sibling session then changes `agent.taskSessionId`. This change runs the route synchronization effect again. The effect pairs the sibling session with the missing route ID and restores the error state.

The current browser test creates a sibling without a session. That setup does not enter the failing fast path.

## Frontend

### Route-to-store synchronization

- Extend `syncActiveTaskSession` with `activeTaskId` and `previousRouteTaskId` inputs.
- Change its return type to `boolean`. The value reports whether the function applied the route state.

```typescript
export function syncActiveTaskSession(params: {
  initialTaskId: string | undefined;
  fallbackTaskId: string | null | undefined;
  initialSessionId: string | null;
  activeTaskId: string | null;
  previousRouteTaskId: string | null | undefined;
  setActiveSessionAuto: (taskId: string, sessionId: string) => void;
  setActiveTask: (taskId: string) => void;
}): boolean;
```

- Return `true` for the first route initialization and for a changed route task ID.
- Return `true` when the active store task still matches the unchanged route. This rule permits delayed session adoption for the current route.
- Return `false` when the route ID is unchanged and the active store task differs. This rule preserves an in-place sidebar selection.
- Track the prior route task ID with `useRef<string | null | undefined>(undefined)` in `useTaskPageData`.
- Update the ref only when `syncActiveTaskSession` returns `true`.
- Read the latest active task ID through a ref, but do not include it as a
  reactive effect dependency. Sidebar task selection owns preferred-session
  restoration and must not be followed by route synchronization that reapplies
  the route's primary session.

### URL and Dockview boundaries

- Keep `replaceTaskUrl` as a non-navigating URL update.
- Keep the existing Dockview environment switch and session selection paths.
- Do not add a route remount or a hard refresh. Both changes can discard the fast workbench transition.

### Mobile design contract

- The change only guards shared task and session state. It does not change layout, controls, touch behavior, or scrolling.
- The phone error state keeps the existing task-overview action.
- The existing phone scenario continues to cover recovery from an unavailable route.
- No new phone browser test is necessary for this desktop-sidebar path.

## Tests

- **What:** The first route initialization can select its route task.
- **File:** `apps/web/components/task/task-page-content-helpers.test.ts`.
- **How:** Call `syncActiveTaskSession` with no prior route task ID.

- **What:** A changed route task ID can replace a stale active task.
- **File:** `apps/web/components/task/task-page-content-helpers.test.ts`.
- **How:** Call `syncActiveTaskSession` with different prior and current route task IDs.

- **What:** The current route can adopt a session that becomes available later.
- **File:** `apps/web/components/task/task-page-content-helpers.test.ts`.
- **How:** Use the same route and active task ID.

- **What:** An unchanged stale route cannot replace an in-place sibling selection.
- **File:** `apps/web/components/task/task-page-content-helpers.test.ts`.
- **How:** Use the same route ID and a different active store task ID. This assertion must fail before the production change.

## E2E Tests

- **Scenario:** Given a missing task route and a sibling with a loaded primary session, selecting the sibling keeps its workbench active.
- **File:** `apps/web/e2e/tests/task/task-loading-state.spec.ts`.
- **Setup:** Create the sibling with `createTaskWithAgent` and the existing mock-agent profile.
- **What to validate:** The URL selects the sibling. The error state disappears. The Dockview workbench appears. The sibling row stays active after session updates.
- Keep the existing no-session sibling scenario. It covers the prepare-session path.
- **Scenario:** Given task A has a primary and a non-primary session, switching
  A → B → A preserves the selected non-primary session.
- **What to validate:** The route returns to A while the active-session store
  value remains the non-primary session.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Guard missing-route task synchronization](task-01-guard-route-task-sync.md)

The task is sequential. The unit guard and browser regression describe one state transition.

## Risks and out of scope

- The guard must still let a real route change replace the active task.
- The guard must still let the current route adopt a session that arrives later.
- A ref must distinguish the first route synchronization from an unchanged missing route.
- Changes to `replaceTaskUrl`, Dockview layouts, backend task lookup, and archive behavior are out of scope.
- The repair adds no API, WebSocket, persistence, permission, or user-interface contract.

## Verification Results

- RED browser run: the focused session-backed sibling E2E reproduced the visible
  task error state after sibling selection.
- RED unit run: the stale-route case failed because `syncActiveTaskSession` returned
  `undefined` and re-applied stale route state.
- GREEN unit run: the focused helper suite passed, 22 tests.
- GREEN browser run: the focused production-build E2E passed, 1 test.
- `pnpm run typecheck` passed.
- Targeted Prettier check passed.
- Targeted ESLint passed.
- Review fixup RED E2E: the A → B → A secondary-session regression returned
  task A's primary session after returning from task B.
- Review fixup GREEN E2E: the same production-build regression passed after
  removing the active-task-only effect trigger and reading the current active
  task from a ref.
- Full task-loading-state production-build E2E passed, 4 tests.
