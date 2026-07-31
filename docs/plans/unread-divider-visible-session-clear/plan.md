---
spec: docs/specs/office/unread-divider.md
created: 2026-07-31
status: pending-approval
---

# Implementation Plan: Clear unread markers during active sessions

## Root cause

`useSessionReadTracking` deliberately freezes the visit-start `anchor` until a visibility transition. A successful `markSessionRead` updates only the persisted cursor in the store; it never clears that local anchor. Consequently, a divider captured for a rewound cursor remains rendered after the reader is actively viewing, and even as later messages are acknowledged.

## Decision

Treat the divider as a pending visit-start marker. Capture it before the first live acknowledgment so the initial unread boundary can be identified, then clear it when the non-stale `markSessionRead` response for the newest visible message succeeds. Do not recreate or move a marker while the same panel remains visible. A failed request retains the marker because the visible transcript was not acknowledged.

## Frontend

- `apps/web/components/task/chat/use-session-read-tracking.ts`: in the existing non-stale successful mark-read response path, clear the anchor only when it still belongs to that dispatch's visible session. Preserve the current dispatch identity guard, monotonic backend cursor handling, feature gate, and leave/re-enter capture behavior.
- `apps/web/components/task/chat/use-session-read-tracking.test.ts`: add deferred-response coverage that proves the marker is visible only while the visit-start request is pending, clears on the current response, and remains on request failure. Retain the stale-response regression test.

## E2E Tests

- `apps/web/e2e/tests/chat/unread-divider.spec.ts`: make the desktop response timing deterministic by intercepting/delaying `mark-read`; assert the divider's pending visit-start position, release the response, then assert the active transcript no longer contains a divider. Replace the now-invalid persistent-divider/scroll contract with the active-session-clear contract.
- `apps/web/e2e/tests/chat/mobile-unread-divider.spec.ts`: apply the same delayed-response flow under the native mobile chat layout. This verifies parity for the shared hook and the mobile renderer without adding desktop controls to the phone UI.

## Verification

1. Run the focused hook test file during Red-Green-Refactor.
2. Run the desktop and mobile unread-divider Playwright specs through `pnpm e2e:run` after a production rebuild.
3. Run `pnpm run typecheck` from `apps/web`.

## Implementation wave

1. [task-01-clear-active-visit-marker](task-01-clear-active-visit-marker.md) — sequential; one shared hook change with its deterministic desktop and mobile regression coverage.
