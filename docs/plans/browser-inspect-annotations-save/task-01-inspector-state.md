---
id: "01-inspector-state"
title: "Preserve inspector pending state"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/browser-inspect-annotations-save.md"
---

# Task 01: Preserve inspector pending state

## Acceptance

- A pin or area selection remains available to `commitAnnotation` after the
  popup is opened, so Save/Enter can emit `annotation-added` and close the
  popup.
- Cancel and Escape still clear the pending selection without emitting an
  annotation.
- The existing API package tests and JavaScript syntax check pass.

## Verification

- Add and run the focused regression test:
  `cd apps/backend && go test -run TestInspectorScript_PreservesPendingAnnotationAcrossPopupCleanup ./internal/agentctl/server/api`
- Run the full affected package:
  `cd apps/backend && go test ./internal/agentctl/server/api`
- Validate the embedded script parses:
  `cd apps/backend && node --check internal/agentctl/server/api/scripts/inspector.js`
- Run `git diff --check`.

## Files likely touched

- `apps/backend/internal/agentctl/server/api/scripts/inspector.js`
- `apps/backend/internal/agentctl/server/api/html_injector_test.go`

## Dependencies

None.

## Parallelism

Sequential. The test and the embedded script share the same behavior contract.

## Inputs

- `docs/specs/ui/requirements/browser-inspect-annotations-save.md`, especially the Save,
  Enter, and dismissal scenarios.
- `docs/plans/browser-inspect-annotations-save/plan.md`.
- Existing `closePopup`, `openCommentPopup`, `startPending`, and
  `commitAnnotation` implementations in `inspector.js`.

## Output contract

Report the changed files, the red/green test results, exact commands, any
remaining E2E dependency, and synchronized task/plan statuses.

## Results

- RED: `cd apps/backend && go test -run TestInspectorScript_PreservesPendingAnnotationAcrossPopupCleanup ./internal/agentctl/server/api` failed at the new assertion before the production fix.
- GREEN: the same focused test passed after preserving `pendingAnnotation` across `closePopup()`.
- `cd apps/backend && go test ./internal/agentctl/server/api` — passed.
- `cd apps/backend && node --check internal/agentctl/server/api/scripts/inspector.js` — passed.
- `git diff --check` — passed.
