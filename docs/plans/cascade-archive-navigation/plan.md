---
spec: docs/specs/tasks/requirements/archive-confirmation.md
created: 2026-08-12
status: complete
---

# Implementation Plan: Cascade Archive Navigation

## Overview

Make the shared archive switch logic aware of the full task tree selected by a
cascade archive. The client will select only a live task outside that tree. If
none exists, it will wait for archive success before it opens Home. Focused unit
tests and desktop and phone E2E tests will protect URL, rendered state, failure
recovery, and later navigation.

## Root cause

`useArchiveAndSwitchTask` calls `removeTaskFromBoard` with `switchOnly: true`
before it sends the archive request. `selectNextTaskAfterRemoval` excludes only
the parent task ID. It can therefore select a recent child even when
`cascade: true` will archive that child in the same request.

The diagnostic bundle records this sequence on 2026-08-12:

1. The client selected child task `ef8...` while parent `a077...` was active.
2. The archive request then archived the parent and two children.
3. The WebSocket archive event redirected the now-active child to Home.

The pre-switch uses `history.replaceState`, while the WebSocket handler uses
the application navigation path. These competing transitions can leave the
address bar and rendered route out of sync. The archive API completed without
an error, which explains why a hard refresh repaired the page.

## Frontend

### Cascade-aware candidate selection

- Extend the options accepted by `removeTaskFromBoard` so the caller can mark
  the removed task tree as excluded from candidate selection.
- Build the excluded task IDs from the parent links in the cached workflow and
  Kanban projections. Use a transitive walk so the logic matches the backend's
  recursive cascade contract even if deeper trees appear in cached data.
- Keep non-cascade behavior unchanged. An active child remains a valid next task
  when only its parent is archived.
- Continue to order safe candidates by recent use and board order. Continue to
  validate each candidate with the task API before switching.

### Deterministic archive transition

- Pass the cascade exclusion option to both the pre-request switch and the
  post-request cleanup in `useArchiveAndSwitchTask`.
- When `switchOnly` finds no safe task, return without changing the route. The
  successful cleanup or WebSocket event can then open Home after the archive
  succeeds.
- Keep the current rollback rule. If the archive request fails after a safe
  pre-switch, restore the original task, session, and URL. If no safe pre-switch
  occurred, leave the original task rendered.
- Do not change `replaceTaskUrl`, the archive endpoint, or WebSocket redirect
  ownership. The repair removes the doomed navigation target that creates the
  race.

## Mobile design contract

- **Shared behavior:** desktop sidebar and mobile task switcher actions already
  call `useArchiveAndSwitchTask`. The repair stays in this shared hook and its
  removal helper.
- **Nearest shipped exemplar:** keep the current task action and
  `TaskArchiveConfirmDialog` flow in
  `apps/web/components/task/mobile/session-task-switcher-sheet.tsx`.
- **Composition:** no surface, drawer, dialog, control, scroll owner, or
  responsive layout changes.
- **Phone outcome:** after a cascade archive, the phone renders a safe remaining
  task whose ID matches the URL. The task drawer can then open another task
  without a page refresh.
- **Input and accessibility:** existing task action and cascade checkbox
  semantics remain unchanged. The E2E test uses the shipped touch path.

## Tests

- In `apps/web/hooks/use-task-removal.test.ts`, prove that cascade selection
  skips all descendants and selects an unrelated live task.
- Prove that non-cascade selection can still select a child.
- Prove that a switch-only pass with no safe candidate does not open Home, while
  the final successful removal does.
- In `apps/web/hooks/use-task-actions.test.ts`, prove that cascade exclusion is
  used before and after the API request. Prove that failure rollback still
  restores the original task.

## E2E tests

- Extend `apps/web/e2e/tests/task/archive-task-redirect.spec.ts`. Create a
  parent, two children, and one unrelated task. Visit a child before the parent
  so the old recent-task policy selects that child. Archive the parent with the
  cascade checkbox and assert that the unrelated task ID and content render.
- Add `apps/web/e2e/tests/task/mobile-archive-task-redirect.spec.ts` for the same
  recent-child setup through the phone task drawer. Assert the safe task route
  and rendered mobile layout. Then create another live task and open it from the
  drawer without reloading.
- Count main-frame document requests around each archive transition. The count
  must not increase, which proves that the fix does not depend on a hard refresh.
- Extend `SessionPage.archiveTaskInSidebar` with an optional cascade input so
  desktop archive tests use one stable dialog interaction.

## Verification

Run the workspace install once first if `apps/node_modules` is absent:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web exec vitest run hooks/use-task-removal.test.ts hooks/use-task-actions.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run tests/task/archive-task-redirect.spec.ts -- --grep 'skips cascaded descendants')
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-archive-task-redirect.spec.ts -- --grep 'keeps navigation responsive')
```

Confirm that Playwright discovers and runs the intended test for each focused
E2E command. `No tests found` is not passing evidence.

## Implementation waves and parallel candidates

Wave 1:

- [x] [Task 01: Make archive switching cascade-aware](task-01-cascade-aware-switch.md)

Wave 2, after Task 01:

- [x] [Task 02: Add archive navigation E2E coverage](task-02-archive-navigation-e2e.md)

The tasks are sequential. The E2E regression requires the shared selection fix,
and both E2E files use the same managed production build. No subagent execution
is authorized by this plan.

## Risks and out of scope

- Cached projections can contain duplicate rows. Descendant collection must
  deduplicate IDs and stop on malformed parent cycles.
- WebSocket events can arrive before the archive response. The existing
  `wasActiveTaskId` guard must keep user navigation authoritative.
- The change must not filter children from non-cascade archives.
- Backend archive behavior, persistence, archive confirmation settings, delete
  behavior, and route infrastructure remain out of scope.

## Verification results

All implementation waves are complete. Focused unit tests pass (23 tests
across the removal and action hook suites), the web typecheck passes, and the
desktop and phone archive navigation scenarios pass without main-frame
document requests during the SPA transitions.

Verification:

- `(cd apps && pnpm --filter @kandev/web exec vitest run hooks/use-task-removal.test.ts hooks/use-task-actions.test.ts)` passed with 23 tests after review remediation.
- `cd apps/web && pnpm run typecheck` passed.
- `cd apps/web && pnpm e2e:run tests/task/archive-task-redirect.spec.ts -- --grep 'skips cascaded descendants'` passed with 1 test.
- `cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-archive-task-redirect.spec.ts -- --grep 'keeps navigation responsive'` passed with 1 test.
- `cd apps/web && pnpm e2e:run tests/task/archive-task-redirect.spec.ts` passed with 2 tests.
- Managed E2E runners cleaned up their isolated runtime and database after each run.
