---
spec: docs/specs/ui/requirements/transcript-auto-scroll.md
created: 2026-07-30
status: implemented
---

# Implementation Plan: Transcript Auto-scroll Stability

## Overview

Stabilize the unrelated CI regressions found on PR #2053. First replace the
orchestrator test's zero-wait scheduling assertion with bounded synchronization.
Then disable native browser overflow anchoring on the native transcript scroll
owner while auto-scroll is off, and prove the user-visible behavior on desktop
and mobile.

## Backend

### Clarification-recovery test synchronization

Update `TestClarificationRecovery_ReleasesGuardAfterRetryDispatch` in
`apps/backend/internal/orchestrator/task_operations_test.go`. Keep its blocked
prompt and ownership assertions, but wait with a bounded receive for
`retryClarificationAfterCancel` to return after the retry prompt is accepted.
This changes test scheduling only; production orchestration code is not
modified.

## Frontend

### Native transcript overflow anchoring

Update `apps/web/components/task/chat/message-list-native.tsx` so the native
scroll container opts out of CSS overflow anchoring while transcript auto-scroll
is disabled. Preserve the existing bottom marker and enabled behavior. The
desktop and mobile chat layouts reuse this component, retain their existing
single scroll owner, and require no navigation or control-layout change.

Mobile contract: the existing chat view remains the full-height scroll surface;
the top-bar toggle remains the mobile entry point and its touch target and
hierarchy do not change. The closest shipped exemplar is
`mobile-auto-scroll-toggle.spec.ts`, which already proves the same toggle and
mid-transcript freeze behavior on Pixel 5. Add the bottom-anchor scenario to
that flow.

## Tests

- **What:** clarification recovery completes asynchronously after dispatch
  acceptance without relying on a zero-time scheduler race.
  **File:** `apps/backend/internal/orchestrator/task_operations_test.go`.
  **How:** run the existing named Go regression test repeatedly.
- **What:** disabled native auto-scroll does not move a bottom-anchored
  transcript when a message arrives.
  **Files:** `apps/web/e2e/tests/chat/auto-scroll-toggle.spec.ts` and
  `apps/web/e2e/tests/chat/mobile-auto-scroll-toggle.spec.ts`.
  **How:** exercise the real WebSocket message path in Chromium and
  `mobile-chrome`.

## E2E Tests

- **Scenario:** GIVEN a user disables auto-scroll at the bottom, WHEN a live
  message arrives, THEN `scrollTop` does not increase.
  **Files:** desktop and mobile transcript-toggle specs.
  **What to verify:** a real message appears and the visible transcript remains
  frozen in both viewport compositions.

## Implementation Waves

Wave 1 (sequential by default):

- [x] [task-01-stabilize-recovery-test](task-01-stabilize-recovery-test.md)
- [x] [task-02-freeze-native-transcript](task-02-freeze-native-transcript.md)

## Open Questions

None.
