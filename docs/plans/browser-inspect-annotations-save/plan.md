---
spec: docs/specs/ui/requirements/browser-inspect-annotations-save.md
created: 2026-08-07
status: complete
---

# Implementation Plan: Browser inspect annotation submission

## Overview

Repair the injected inspector's pending-annotation lifecycle with the smallest
possible change: preserve the selection while `openCommentPopup` performs its
defensive popup cleanup. Add a script-level regression test and focused browser
coverage for both Save and plain-Enter submission, while keeping Cancel/Escape
cleanup unchanged.

The issue's root cause is confirmed in
`apps/backend/internal/agentctl/server/api/scripts/inspector.js`: `startPending`
sets `pendingAnnotation`, then `openCommentPopup` calls `closePopup`, which
unconditionally clears it before either submit handler runs.

## Backend

### Injected inspector state

- `apps/backend/internal/agentctl/server/api/scripts/inspector.js`
  - Save the current `pendingAnnotation` before the preventive
    `closePopup()` call in `openCommentPopup`.
  - Restore it immediately after cleanup and before wiring the popup handlers.
  - Leave direct Cancel/Escape calls to `closePopup()` unchanged so dismissal
    still discards the pending selection.

## Frontend

### Preview submission coverage

- `apps/web/e2e/tests/preview/inspector-submission.spec.ts`
  - Use the existing E2E fixtures and a local mock HTML server.
  - Configure the seeded repository's dev script so the Browser panel action is
    available, open the Browser panel, switch to Inspect mode, and interact
    with the real injected iframe popup.
  - Cover clicking Save and pressing plain Enter, asserting the popup closes,
    the iframe marker exists, and the parent annotations panel contains the
    submitted comment.
  - Restore any seeded repository settings in teardown.

This is a state-only repair in the existing preview surface. It does not alter
responsive composition, touch targets, scrolling, navigation, or viewport
geometry, so a separate mobile Playwright test is not required; the shared
injected state transition is the same on mobile-sized previews.

## Tests

- **What:** `openCommentPopup` does not erase the selection created by
  `startPending`, while the existing dismissal path still clears it.
  **File:** `apps/backend/internal/agentctl/server/api/html_injector_test.go`.
  **How:** assert the embedded inspector script preserves the pending value
  across the cleanup call and retains the direct Cancel/Escape cleanup paths.
- **What:** the proxy-served inspector script remains syntactically valid and
  the existing HTML injection behavior remains intact.
  **File:** existing API package tests.
  **How:** run the targeted Go package test and `node --check` on the injected
  script.

## E2E Tests

- **Scenario:** **GIVEN** a pin selection with the comment popup open, **WHEN**
  the user types a comment and clicks **Save**, **THEN** the popup closes, a
  marker is present in the iframe, and the Browser annotations panel shows the
  comment.
  **File:** `apps/web/e2e/tests/preview/inspector-submission.spec.ts`.
- **Scenario:** **GIVEN** a pin selection with the comment popup open,
  **WHEN** the user types a comment and presses plain **Enter**, **THEN** the
  same marker and annotations-panel outcomes occur.
  **File:** `apps/web/e2e/tests/preview/inspector-submission.spec.ts`.

## Verification Results

Implemented and verified:

- `cd apps/backend && go test ./internal/agentctl/server/api` — passed.
- `cd apps/backend && node --check internal/agentctl/server/api/scripts/inspector.js` — passed.
- `cd apps/backend && go test -run TestInspectorScript_PreservesPendingAnnotationAcrossPopupCleanup ./internal/agentctl/server/api` — passed after the expected red failure.
- `cd apps/web && pnpm e2e:run --project chromium tests/preview/inspector-submission.spec.ts` — 2 passed (Save and plain Enter).
- `git diff --check` — passed.

## Implementation Waves And Parallel Candidates

Wave 1 (sequential):

- [x] [task-01-inspector-state](task-01-inspector-state.md) — done

Wave 2 (sequential, depends on Wave 1):

- [x] [task-02-preview-submit-e2e](task-02-preview-submit-e2e.md) — done

## Open Questions

- The existing `preview-annotations.spec.ts` suite remains skipped because its
  preview fixture lacks a configured `dev_script`; the focused regression test
  sets that prerequisite locally rather than changing unrelated scenarios.
