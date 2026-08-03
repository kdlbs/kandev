---
spec: docs/specs/ui/cancel-turn-progress.md
created: 2026-08-03
status: building
---

# Implementation Plan: Preserve cancel-turn progress across task navigation

## Overview

Move cancel-request progress out of the remounted button instance and into the application-level UI
store, keyed by session ID. Keep the existing `agent.cancel` request and visual treatment, then add
a component regression and a delayed-WebSocket Playwright flow that switches away from and back to
the cancelling task.

## Root cause

`SubmitButton` in `apps/web/components/task/chat/chat-input-toolbar-primitives.tsx` owns
`isCancelling` in component-local `useState` and guards duplicate requests with a component-local
ref. Task navigation remounts the chat toolbar while the original `agent.cancel` promise continues
waiting for its WebSocket response. The new button instance initializes both values to `false`, so
it renders the pause icon as enabled even though cancellation is still pending.

## Decision

Extend the existing `chatInput` UI slice with `cancellingBySessionId` and a
`setCancelTurnPending(sessionId, pending)` action. `SubmitButton` will select the entry for its
`sessionId`, use the store as the duplicate-request guard, set it before awaiting `onCancel`, and
delete it in `finally`. The root `StateProvider` already outlives SPA task-route remounts, so no
backend, hydration, local-storage, or protocol change is needed.

## Frontend

### Transient chat-input state

- `apps/web/lib/state/slices/ui/types.ts`: add the session-keyed cancellation map to
  `ChatInputState` and add the typed `setCancelTurnPending` action.
- `apps/web/lib/state/slices/ui/ui-slice.ts`: initialize the map empty; set an entry to `true` when a
  request begins and delete it when the request settles. Do not persist this map to browser storage
  or the boot payload.
- `apps/web/lib/state/slices/ui/ui-slice.test.ts`: prove per-session isolation and removal on clear.

### Shared cancel control

- `apps/web/components/task/chat/chat-input-toolbar-primitives.tsx`: give `SubmitButton` the current
  `sessionId`; replace its local state/ref with selectors and the UI-slice action; retain the
  existing spinner, disabled styling, tooltip, error handling, and single-request behavior.
- `apps/web/components/task/chat/chat-input-toolbar.tsx`: pass `sessionId` through the minimal
  toolbar as well as the existing responsive paths.
- `apps/web/components/task/chat/chat-input-toolbar-desktop.tsx` and
  `apps/web/components/task/chat/chat-input-toolbar-mobile.tsx`: pass their existing session ID to
  the shared `SubmitButton`.

### Mobile design contract

The desktop outcome and mobile entry point remain the existing inline cancel control in the chat
composer. The nearest shipped mobile exemplar is `MobileChatInputToolbar`, which already renders the
same `SubmitButton` as desktop; no surface, hierarchy, scroll owner, safe-area, touch-target, or
navigation behavior changes. The shared store selector and request guard remain viewport-neutral,
and component coverage will exercise both responsive branches.

## Tests

- **Store lifecycle and isolation:** in
  `apps/web/lib/state/slices/ui/ui-slice.test.ts`, set cancellation pending for one session, verify a
  second session is unaffected, and verify clearing removes the first entry.
- **Remount regression and duplicate guard:** in
  `apps/web/components/task/chat/chat-input-toolbar.test.tsx`, keep a `StateProvider` mounted while
  unmounting and remounting the toolbar around an unresolved `onCancel` promise. Assert the
  remounted control remains disabled with `GridSpinner`, additional activation does not duplicate
  the request, and resolution or rejection clears the state. Run this flow for desktop and compact
  mobile toolbar selection because both use the same control.

## E2E Tests

- **Scenario:** **GIVEN** task A has a running turn and its `agent.cancel` request is held before it
  reaches the backend, **WHEN** the user clicks cancel, switches to task B, and returns to task A,
  **THEN** task A's cancel control remains disabled and animated until the held request is released.
- **Files:** extend `apps/web/e2e/helpers/ws-drop.ts` with a controller that holds and later forwards
  one correlated `agent.cancel` request without dropping unrelated frames; add
  `apps/web/e2e/tests/chat/cancel-progress-task-switch.spec.ts` for the desktop task-switch flow.
- **What to verify:** one held request, visible spinner and disabled control before and after the
  route remount, no duplicate request, and normal idle rendering after release.

Separate mobile Playwright coverage is not required for this repair because it changes only the
session-keyed state source behind the existing shared control; it does not change mobile layout,
touch behavior, scrolling, navigation, or viewport-dependent interaction. The responsive component
regression covers both toolbar branches.

## Verification Results

- `cd apps && pnpm install --frozen-lockfile` — passed.
- `cd apps && pnpm --filter @kandev/web test -- --run lib/state/slices/ui/ui-slice.test.ts components/task/chat/chat-input-toolbar.test.tsx` — passed, 2 files / 52 tests.
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps && pnpm --filter @kandev/web run i18n:ratchet` — passed, 0 added / 7 modified files clean.
- `cd apps/web && pnpm e2e:run --host --project chromium -- tests/chat/cancel-progress-task-switch.spec.ts` — passed, 1 test in 13.7s.
- `git diff --check` — passed.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-session-scoped-cancel-state](task-01-session-scoped-cancel-state.md) — done;
  establishes the state contract and shared control behavior.

Wave 2:

- [x] [task-02-task-switch-regression-e2e](task-02-task-switch-regression-e2e.md) — done;
  depends on Task 01 and proves the complete navigation flow.

The tasks are not parallel-safe because Task 02 validates the state and component behavior added by
Task 01.

## Open Questions

None.
