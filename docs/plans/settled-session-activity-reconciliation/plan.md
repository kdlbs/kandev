---
created: 2026-09-01
status: done
requirements:
  - REQ-PLATFORM-BACKGROUND-WORK-LIVENESS-001
system_design:
  - ../../specs/platform/system-design/background-work-liveness.md
legacy_specs: []
---

# Implementation Plan: Reconcile Settled Session Activity After Restart

## Overview

Fix the stale `Background work is running` status that can survive a backend
restart even though the resumed agent has no active turn. The backend already
omits `foreground_activity` from the settled authoritative session record. The
frontend currently merges that complete record with the same omission semantics
as a partial WebSocket update, so it keeps the pre-restart `background` value.

Keep partial-event merging unchanged. Make complete session-list hydration
clear omitted activity while using a per-session client activity epoch to keep
a newer WebSocket projection that arrives during the HTTP request. Extend the
existing backend-restart browser flow with the operator-visible outcome.

## Scope

### In scope

- Reconcile the complete task-session list against retained client activity.
- Preserve newer WebSocket activity across an in-flight list refresh.
- Prove the operator-visible backend-restart outcome in desktop Playwright.

### Out of scope

- Backend activity detection, persistence, prompt admission, and agent resume.
- Status wording, component layout, navigation, or responsive interaction.
- Changes to the Claude background prompt handoff experiment.

## Technical approach

### Frontend state and hydration

- Add a process-local activity epoch per session in
  `apps/web/lib/state/slices/session/types.ts` and `session-slice.ts`.
- Increment the epoch whenever a partial session event explicitly carries
  `foreground_activity`, including repeated values and explicit clears.
- Capture activity epochs through one shared helper whenever a client-side
  asynchronous session-list loader starts an authoritative request, including
  task hydration, task selection/removal, and Office task detail loading.
- Require the request-start epoch map in the complete-snapshot action contract
  and serialize forced hydration calls before React rerenders loading state.
- When the response arrives, normalize an omitted activity field to a clear if
  the session epoch is unchanged. If the epoch advanced, preserve the newer
  live projection (`foreground_activity`, `active_subagent_count`, and
  `supports_steering`) while merging the response's durable fields.
- Retain the current partial-update behavior in `mergeTaskSession` and delete a
  session's activity epoch during session cleanup.

## Tests

- **AC `.3` and `.9`, authoritative clear:** change the regression in
  `apps/web/lib/state/slices/session/session-slice.upsert.test.ts` so a complete
  settled snapshot clears omitted activity while still merging durable fields.
- **AC `.7` and `.9`, event ordering:** add store and hook coverage in
  `session-slice.upsert.test.ts` and `apps/web/hooks/use-task-sessions.test.ts`
  that holds a list request open, applies a newer activity event, and proves
  that the response preserves the complete newer activity projection.
- **Partial-event compatibility:** retain or add a session-slice assertion that
  an omitted field on `upsertTaskSessionFromEvent` does not clear current
  activity. Keep the existing explicit activity, explicit null,
  added-during-hydration, cancellation, and route tests green.

## E2E tests

- **AC `.9`, restart recovery:** extend
  `apps/web/e2e/tests/session/session-resume.spec.ts` in the desktop Chromium
  project. Seed or establish a retained background projection, restart the
  backend, allow the session to resume, and open or refresh the task after the
  resume transition. Verify the session is `WAITING_FOR_INPUT`, the composer is
  ready, and the background-work status is absent.

This is a shared client-state repair with no component, layout, navigation,
scroll, or touch changes. Desktop Playwright covers the user-visible restart
flow. A separate mobile test would repeat the same store transition without a
mobile-specific interaction, so mobile coverage remains in the shared unit
tests.

## Work orders

- [x] [Task 01: Reconcile authoritative session activity](task-01-reconcile-authoritative-session-activity.md)
- [x] [Task 02: Prove restart recovery in the browser](task-02-prove-restart-activity-recovery.md)

Execution is sequential in the primary conversation. Task 02 depends on Task
01's reconciliation behavior.

## Verification results

- Focused Vitest: 58 tests passed across the session slice, hydration hook,
  task selection/removal loader, and Office task detail loader.
- Web TypeScript typecheck passed.
- Focused desktop Chromium restart E2E passed after demonstrating the expected
  timeout against the pre-fix merge behavior.
- Specification lint and Markdown whitespace checks passed.

## Risks

- The fix must not reinterpret omitted fields on partial WebSocket events.
- A delayed HTTP response must not overwrite an activity event received after
  the request started.
- The activity epoch is client-local ordering state. It is not persisted and
  does not become a backend API field.
- Public documentation does not change because this restores the specified
  status behavior and adds no operator-facing configuration or API field.
