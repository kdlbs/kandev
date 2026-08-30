---
spec: docs/specs/ui/requirements/transcript-navigation-settings.md
created: 2026-07-30
status: implemented
---

# Implementation Plan: Transcript Navigation Settings

## Overview

Extend the backend-owned user-settings contract with visibility for the existing transcript
auto-scroll control, then carry the setting through boot hydration, websocket refreshes, settings
drafts, and transcript chrome. Change the two requested absent-value defaults without overwriting
explicit saved values. Finally, reserve enough scroll clearance for the floating Save action and
prove the settings flow and geometry on desktop and mobile.

## Backend

### Portable user-settings contract

- Add `ShowTranscriptAutoScrollControl` / `show_transcript_auto_scroll_control` to
  `internal/user/models.UserSettings`, DTO response and PATCH request types, controller mapping,
  service request/application, settings-event payload, SQLite JSON marshal/scan, and the Go boot
  state.
- In `apps/backend/internal/user/store/sqlite.go`, default missing
  `show_anchored_prompt_bar` and `show_scroll_to_start` to `false`; keep
  `show_scroll_to_last_prompt` and the new `show_transcript_auto_scroll_control` defaulted to
  `true`. Pointer-backed scan fields preserve explicit false values without a schema migration.
- Extend store and service tests for empty/legacy JSON, explicit values, PATCH application, and
  marshal/scan round trips.

## Frontend

### Hydration and live settings

- Add `showTranscriptAutoScrollControl` to the frontend user-settings state and HTTP/websocket
  wire types.
- Update the root defaults, SSR/boot response mapping, websocket handler, display-settings
  carry-forward mapping, and nested settings-state response mapping. Missing anchored-prompt and
  scroll-to-start values fall back to `false`; missing scroll-to-last-prompt and auto-scroll-control
  values fall back to `true`.
- Update focused mapping/handler tests and E2E API helper types/fixture baselines.

### Settings card and transcript chrome

- Extend `apps/web/components/settings/anchored-prompt-bar-settings.tsx` with a fourth,
  self-explaining **Show transcript auto-scroll control** switch in the same manual-save
  contributor and PATCH payload.
- Update `TaskActionsSettings` copy so it describes optional transcript navigation controls rather
  than claiming the scroll buttons are always available.
- In `apps/web/components/task/chat/auto-scroll-toggle-button.tsx`, read the saved visibility preference
  before rendering `AutoScrollToggleButton`; do not change
  `useTranscriptAutoScrollEnabled`, session storage, or the default enabled auto-scroll behavior.
- Extend component tests to prove the new default states, draft-before-Save behavior, independent
  PATCH fields, hidden control behavior, and unchanged per-session auto-scroll semantics.

### Settings scroll clearance and mobile contract

- Increase the bottom padding on `settings-scroll-container` in
  `apps/web/components/settings/settings-layout-client.tsx` so its last control can scroll above
  the floating action's bottom offset, action height, status bar, and safe-area inset.
- Desktop and phone use the existing settings page and shared floating action. The settings
  container remains the single vertical scroll owner; no new overlay or mobile-only presentation
  is introduced.
- The nearest mobile exemplars are the existing settings shell and
  `mobile-general-settings.spec.ts`: a touch-reachable fixed primary action, safe-area clearance,
  internal vertical scrolling, and no document horizontal overflow.

## Tests

- **What:** absent versus explicit transcript-navigation values and the new preference survive
  marshal/scan and PATCH application.
  **Files:** `apps/backend/internal/user/store/sqlite_test.go`,
  `apps/backend/internal/user/service/service_test.go`.
  **How:** focused table tests for defaults, explicit booleans, round trip, and request application.
- **What:** boot/SSR and websocket settings mapping use the same defaults and preserve explicit
  values.
  **Files:** `apps/web/lib/ssr/user-settings.test.ts`,
  `apps/web/lib/ws/handlers/users.test.ts`.
  **How:** mapping tests with omitted and explicit boolean fields.
- **What:** all four settings participate independently in the route-level draft and the
  auto-scroll control visibility does not change its per-session enabled default.
  **Files:** `apps/web/components/settings/anchored-prompt-bar-settings.test.tsx`,
  `apps/web/components/task/chat/auto-scroll-toggle-button.test.tsx`, and a focused
  `chat-input-area` test if needed for cluster rendering.
  **How:** component tests through the shared Save provider and transcript status bar.

## E2E Tests

- **Scenario:** settings changes remain local until Save and restore independently after reload.
  **File:** `apps/web/e2e/tests/settings/settings-manual-save.spec.ts`.
  **What to verify:** default switch states, dirty markers, no pre-Save persistence, saved values
  after reload, and auto-scroll control visibility in a seeded transcript.
- **Scenario:** the final switch and Save action remain independently reachable at 390px.
  **File:** `apps/web/e2e/tests/settings/mobile-general-settings.spec.ts`.
  **What to verify:** last-control/floating-action bounding boxes do not intersect, the Save button
  has a touch-sized hitbox, the settings container scrolls, and document horizontal overflow is
  absent.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Portable settings contract](task-01-portable-settings-contract.md) — done.

Wave 2:

- [x] [Task 02: Settings UI and transcript control](task-02-settings-ui-and-transcript-control.md)
  — done.

Wave 3:

- [x] [Task 03: Responsive clearance and E2E](task-03-responsive-clearance-and-e2e.md) —
  done; depends on Task 02.

No task is marked parallel-safe because Tasks 01–03 share the user-settings contract and test
fixtures.

## Risks

- Default changes must apply only when a field is absent; explicit saved values must not be
  reinterpreted.
- User settings are mapped through backend DTOs, boot state, frontend SSR, websocket refreshes, and
  a nested settings state; missing one path can make a saved value appear to revert.
- E2E workers persist user settings across tests, so fixture baselines and test cleanup must reset
  all four transcript-navigation preferences.
- Reserved padding must cover the floating action without creating a second scroll owner or
  viewport-width overflow.
