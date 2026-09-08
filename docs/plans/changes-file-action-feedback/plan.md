---
created: 2026-09-03
status: implemented
requirements:
  - REQ-UI-CHANGES-FILE-ACTION-FEEDBACK-001
system_design:
  - ../../specs/ui/system-design/changes-file-action-feedback.md
legacy_specs: []
---

# Implementation Plan: Keep Changes File Action Feedback Visible

## Overview

Make per-file stage and unstage pending feedback override the tree row's idle
hover swap. One focused work order first captures the failure by pausing the
worktree request in Chromium, then makes the minimum shared component change
and verifies the completed stage and unstage transitions.

## Confirmed root cause

- `StageButton` correctly renders `IconLoader2` whenever `FileRow.isPending` is
  true.
- In tree mode, `TreeModeFileActionSlot` wraps that button in a fine-pointer
  layer whose visibility is controlled only by `group-hover` classes.
- The adjacent `FileIcon` also follows only the inverse hover rule. It does not
  consider `isPending`.
- Leaving the row therefore makes the action layer, including its spinner,
  transparent and restores the file-type icon even though the repository/path
  pending key remains active.
- Coarse-pointer rows do not receive the hover-only classes, so their action
  and spinner already remain visible.

## Scope

### In scope

- Give pending state precedence over the fine-pointer file-icon/action hover
  swap.
- Keep the existing spinner visible after pointer leave for both stage and
  unstage operations.
- Preserve per-repository, per-path pending-state scoping.
- Add deterministic Chromium coverage that holds each WebSocket request open,
  leaves the row, and observes the pending and completed states.
- Preserve the existing coarse-pointer action visibility and touch sizing.

### Out of scope

- Git operation dispatch, status polling, WebSocket contracts, and pending-key
  lifecycle changes.
- Bulk actions, discard/edit feedback, cancellation, notifications, and new
  copy.
- Changes-panel layout, path truncation, navigation, or mobile composition.

## Technical approach

Update `TreeModeFileActionSlot` in
`apps/web/components/task/changes-panel-file-row.tsx` so the fine-pointer idle
hover classes apply only when `isPending` is false. While pending, keep the
file-type icon hidden and the spinner layer visible independent of the row's
hover state. Continue rendering the same `StageButton`; do not introduce
component-local request state.

Extend `apps/web/e2e/tests/git/git-changes-panel.spec.ts` with a narrow
WebSocket transport controller patterned after the existing request-pause
helpers. It shall pause only the armed `worktree.stage` or
`worktree.unstage` frame and forward all other frames. The regression creates a
target file (`pending-action.txt`) and a sibling file (`pending-sibling.txt`),
starts each action from the UI, waits until the request is paused, moves the
pointer to a neutral location, and asserts the existing spinner is still
visible only for the target row before releasing the request and observing the
file's new section.

## Tests

- **AC-UI-CHANGES-FILE-ACTION-FEEDBACK-001.1 through .4:** the focused
  Chromium scenario proves spinner visibility for both request types after
  pointer leave, per-row scoping, and the final staged/unstaged section.
- **AC-UI-CHANGES-FILE-ACTION-FEEDBACK-001.5:** the existing
  `changes-panel-file-row.test.tsx` coarse-pointer assertion and
  `mobile-changes-panel.spec.ts` stage/unstage path remain unchanged and green.

## E2E tests

- Add `keeps stage and unstage progress visible after pointer leaves` to
  `apps/web/e2e/tests/git/git-changes-panel.spec.ts` in the Chromium project.
- No new Pixel 5 case is planned because the coarse-pointer branch already
  keeps the action slot visible and this work changes neither its markup nor
  its interaction. Existing mobile coverage exercises both stage and unstage
  controls through the shared row.

## Work orders

- [x] [Task 01: Keep pending file actions visible](task-01-keep-pending-file-actions-visible.md)

## Dependency order

```text
Task 01
```

The browser regression and component correction share one presentation
boundary and proceed sequentially through Red-Green-Refactor.

## Verification results

- RED: the focused Chromium scenario failed after pointer leave because the
  pending action layer reached `opacity: 0` while `worktree.stage` remained
  paused.
- GREEN: the production-build Chromium scenario passed both the paused stage
  and paused unstage flows. A final no-build rerun after test formatting also
  passed.
- The existing `changes-panel-file-row.test.tsx` suite passes all 16 tests,
  including the coarse-pointer action visibility and touch-size assertion.
- Targeted ESLint, web typecheck, specification lint, and scoped diff checks
  pass.

## Risks

- A visibility change that hides the file icon without explicitly revealing
  the spinner layer could leave the slot blank after pointer leave. The E2E
  assertion observes the rendered spinner, not only class names.
- A WebSocket pause helper can disrupt unrelated live updates if it buffers a
  whole message instead of only the matching newline-delimited frame. The
  helper must partition and forward unrelated frames immediately.
- Completion can move the row between sections and detach its locator. The
  test must reacquire the row from the destination section before testing the
  inverse action.

## Open questions

None.
