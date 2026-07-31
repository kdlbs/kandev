---
status: pending-approval
---

# Task 01: Clear the active visit marker

## Scope

Repair the unread-divider lifecycle only. Do not change the read-cursor schema, API, feature setting, pagination behavior, or unrelated transcript scrolling.

## Steps

1. Extend `use-session-read-tracking.test.ts` with a deferred `markSessionRead` response: capture a prior cursor on visibility, assert its divider while the response is held, release the current response, and assert no divider remains. Add the matching rejection assertion that retains the marker. Run the focused test and observe failures before implementation.
2. Update `useSessionReadTracking` so a successful, current `markSessionRead` response clears its matching visit anchor after updating the store. Keep the existing stale-dispatch guard so an old response cannot clear a newer session or visit's marker.
3. Update the desktop and mobile unread-divider Playwright specs to delay the mark-read response. Assert the placement while pending, then release it and assert no divider remains in the active chat. Remove assertions that require a persistent active-session marker or its stale scroll position.
4. Run the focused unit tests, both E2E specs, and `pnpm run typecheck` from `apps/web`.

## Acceptance criteria

- A successful mark-read response clears the divider for the currently visible session without waiting for navigation.
- An old/out-of-order response cannot clear another session or a newer visit's marker.
- A failed mark-read response leaves the marker available for retry on a later visibility transition.
- Desktop and mobile prove the marker is transient under a delayed response and absent after acknowledgment.
