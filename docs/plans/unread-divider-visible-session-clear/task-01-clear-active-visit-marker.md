---
status: pending-approval
---

# Task 01: Suppress markers created during active visits

## Scope

Repair the unread-divider eligibility lifecycle only. Do not change the
read-cursor schema, API, feature setting, pagination behavior, or unrelated
transcript scrolling.

## Steps

1. Extend `use-session-read-tracking.test.ts` with the exact regression:
   start a visible visit where the stored cursor and latest rendered message
   are both `m1`, then add `m2` while visibility never changes. Assert the
   derived divider remains absent. Add the delayed-initial-load variant and
   run the focused test to observe both failures before implementation.
2. Expose the existing `isWaitingForInitialMessages` state through
   `useSessionMessages` and `useChatPanelState` as a dedicated initial-load
   signal. Do not reuse `messagesLoading`, because it also covers pagination
   and later background refreshes.
3. Pass that signal to `useSessionReadTracking`. Retain the prior cursor at
   the visibility transition, then latch visit eligibility once the initial
   load settles: empty cursor, empty initial transcript, and cursor-equals-tail
   must remain divider-ineligible for the visit. Preserve frozen anchors for
   genuine pre-existing unread content and the stale-response guard.
4. Add deterministic desktop and mobile Playwright coverage for opening at
   the current tail, sending a prompt without leaving the chat panel, and
   observing no divider before that prompt. Keep the existing genuine-unread
   visit-start coverage.
5. Run the focused unit tests, both E2E specs, and `pnpm run typecheck` from
   `apps/web`.

## Acceptance criteria

- A session opened at its current tail does not render a divider when a user
  prompt or agent message is subsequently added during the same visible visit.
- A true pre-existing unread boundary still renders after navigation into the
  session.
- An initially loading or empty transcript cannot later activate a stale
  captured cursor as a divider during the same visit.
- Desktop and mobile prove the tail-then-prompt flow has no divider.
