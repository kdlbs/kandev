---
spec: docs/specs/office/requirements/unread-divider.md
created: 2026-07-31
status: pending-approval
---

# Implementation Plan: Suppress markers created during active visits

## Root cause

`useSessionReadTracking` captures a frozen visit anchor even when the
persisted cursor already equals the transcript tail. `findUnreadDividerItemId`
correctly returns no divider at that instant because there is no following
message. When the user sends a prompt or the agent emits a message while the
same panel remains visible, that stale tail anchor gains a following row and
the renderer places a "New" divider before it. No visibility transition or
late session metadata fetch is required.

## Decision

Determine divider eligibility from the immutable cursor captured at visit
entry and the first fully loaded rendered transcript. A visit that begins at
the current tail, or without a cursor, is permanently divider-ineligible
until the user leaves and returns. Preserve a frozen anchor only when unread
content existed in that initial, loaded transcript. The live read cursor
continues advancing independently. Readiness must be a one-time
initial-message-load signal, not the broad `messagesLoading` flag, which also
covers pagination and refreshes.

## Frontend

- `apps/web/hooks/domains/session/use-session-messages.ts`: expose the
  existing `isWaitingForInitialMessages` state as a dedicated
  initial-message readiness result without conflating later pagination or
  background fetches.
- `apps/web/components/task/chat/use-chat-panel-state.ts` and
  `apps/web/components/task/task-chat-panel.tsx`: propagate that dedicated
  initial-load signal to read tracking alongside the latest rendered message
  id.
- `apps/web/components/task/chat/use-session-read-tracking.ts`: retain the
  prior cursor at the visibility transition and latch eligibility exactly once
  when the initial message load settles. Create an anchor only if that cursor
  precedes the initial tail; make no anchor for an empty cursor, an empty
  initial transcript, or a cursor equal to the tail. Keep the stale-response
  and leave/re-enter behavior. Do not derive a new divider from later
  `latestMessageId` changes.
- `apps/web/components/task/chat/use-session-read-tracking.test.ts`: add
  regressions for (a) cursor-at-tail followed by a user prompt while
  continuously visible, and (b) a delayed initial message load that settles
  at the captured cursor. Both must remain divider-ineligible. Retain the
  existing true-unread and stale-response tests.

## E2E Tests

- `apps/web/e2e/tests/chat/unread-divider.spec.ts`: seed a session whose
  persisted cursor is already the visible tail, open it, send a prompt while
  staying on that chat panel, and assert no divider appears before the prompt.
  Retain the existing rewound-cursor flow to prove real pre-existing unread
  messages still render the divider.
- `apps/web/e2e/tests/chat/mobile-unread-divider.spec.ts`: add the same
  tail-then-prompt assertion under the native mobile chat layout. This is the
  same user outcome on the shared hook, with a distinct mobile composition.

## Verification

1. Run the focused hook test file during Red-Green-Refactor.
2. Run the desktop and mobile unread-divider Playwright specs through
   `pnpm e2e:run` after a production rebuild.
3. Run `pnpm run typecheck` from `apps/web`.

## Implementation wave

1. [task-01-clear-active-visit-marker](task-01-clear-active-visit-marker.md)
   — sequential; one readiness-aware shared-hook change with desktop and
   mobile regression coverage.
