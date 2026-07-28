---
spec: docs/specs/platform/background-work-liveness.md
created: 2026-07-28
status: completed
---

# Implementation Plan: Coarse Running Busy Signal

## Overview

Restore the pre-ADR-0049 admission and display contract at the orchestrator's
public policy seams while leaving its background-work accounting machinery
intact. Pin the contract with focused backend tests and desktop/mobile browser
coverage, then update the public API documentation.

## Backend

### Admission and exposed activity

- Update `apps/backend/internal/orchestrator/task_operations.go` so
  `checkSessionPromptable` rejects every `RUNNING` session with
  `ErrAgentPromptInProgress`.
- Update `apps/backend/internal/orchestrator/turn_activity.go` so
  `ForegroundActivity` and activity publications expose only the coarse
  generating policy while the private tracker retains its fine-grained value.
- Update `publishTaskSessionStateChanged` in
  `apps/backend/internal/orchestrator/event_handlers_streaming.go` to use the
  same public policy and omit activity after `RUNNING`.

## Frontend

No production frontend change is required. The existing composer, activity
store, and status components already consume the backend's
`foreground_activity`; the backend will no longer send the disabled
`background` tier.

## Tests

- **What:** a `RUNNING` background-idle session remains unpromptable.
  **File:** `apps/backend/internal/orchestrator/foreground_busy_signal_test.go`.
  **How:** drive the real tracker into background-idle, assert the private value
  remains available, then assert the public admission and activity seams remain
  coarse.
- **What:** live activity publications never advertise the dormant background
  tier. **File:**
  `apps/backend/internal/orchestrator/foreground_activity_signal_test.go`.
  **How:** publish a background-idle transition through the recording event bus
  and assert `generating`.

## E2E Tests

- **Scenario:** GIVEN a held-open background turn on desktop, WHEN the operator
  submits during its background-idle window, THEN the composer queues the
  message and remains generating across reload.
  **File:** `apps/web/e2e/tests/chat/busy-signal.spec.ts`.
- **Scenario:** the same user outcome through the existing mobile composer
  button on Pixel 5.
  **File:** `apps/web/e2e/tests/chat/mobile-busy-signal.spec.ts`.

No layout, navigation, scroll, or touch composition changes are required. Both
viewports reuse the shipped session composer and status surfaces.

## Implementation Waves

- [x] [Task 01: Restore coarse running policy](task-01-restore-coarse-running-policy.md)
