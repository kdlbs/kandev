---
created: 2026-09-06
status: done
requirements:
  - REQ-TASKS-CONFIRMATION-SURFACE-002
  - REQ-UI-TASK-CLEANUP-CONFIRMATION-001
system_design:
  - ../../specs/ui/system-design/confirmation-warning-hierarchy.md
legacy_specs: []
---

# Implementation Plan: Stable Archive Confirmation Surface

## Overview

Prevent a parent-task archive request from showing an anchored loading popover
and then replacing it with the cascade dialog. Gate the standard fine-pointer
surface until descendant classification settles, then mount only the existing
final popover or dialog. One sequential work order keeps the rendering change,
regression tests, and desktop/mobile compatibility evidence together.

## Scope

### In scope

- Keep the confirmation request open while descendant classification is
  pending without mounting a provisional fine-pointer confirmation, while
  retaining Escape and outside-pointer dismissal.
- Preserve the anchored popover for a resolved zero-descendant task and the
  full cascade dialog for positive or unavailable descendant counts.
- Prove the async transition at component and desktop Kanban E2E boundaries.
- Re-run the existing phone Kanban dialog scenario as mobile-parity evidence.

### Out of scope

- Changing archive, cascade, descendant-count, settings, or navigation APIs.
- Prefetching or caching descendant counts before an archive request.
- Changing bulk, forced-dialog, explicit inline, coarse-pointer row, delete, or
  confirmation-preference behavior.
- Changing confirmation copy, localization catalogs, geometry, or styling.

## Technical approach

### Fine-pointer classification gate

In `apps/web/components/task/task-archive-confirmation.tsx`, remove the
fine-pointer `ArchiveClassifyingPopover` presentation. While the shared
`useSubtaskCountState` result is `idle` or `loading`, the standard fine-pointer
branch returns no confirmation shell. The controlled `open` state and current
request remain intact, so the existing resolved-zero popover, positive-count
dialog, and error dialog mount normally once classification settles. Removing
the provisional branch also removes its loader-only import and markup.

A non-rendering pending controller retains dismissal behavior without
reintroducing a temporary confirmation. Escape closes the request and restores
focus to the configured trigger; a new pointer interaction closes it without
blocking that target. Closing disables descendant classification, so a late
response cannot mount a confirmation after dismissal.

The existing coarse-pointer pending branch already renders no popup and is the
nearest implementation precedent. `forceDialog`, bulk, inline, preference
bypass, mutation-pending, and final confirmation branches remain unchanged.

### Component regression

Update `apps/web/components/task/task-archive-confirmation.test.tsx` so a
deferred descendant-count promise proves the full state transition. Before the
promise resolves, neither `task-archive-confirm-popover` nor an alert dialog or
archive action is present. Resolving it with a positive count renders the full
cascade dialog and never exposes the popover. Existing resolved-zero, error,
forced-dialog, callback, and cleanup-content tests continue to cover the other
branches. Separate pending-dismissal cases cover Escape with trigger focus
restoration and outside pointer intent.

### Browser regression and mobile parity

Extend `apps/web/e2e/tests/kanban/cascade-subtasks-toggle.spec.ts` with a
fine-pointer parent-task scenario that holds the exact
`GET /api/v1/tasks/:id/subtask-count` response behind a deterministic promise
gate. After the request is observed, assert that no provisional confirmation is
visible; dismiss it with Escape, release the response, and prove no late surface
mounts. Reopen Archive, then assert that only the cascade alert dialog appears.
Release and unregister the route in `finally` so failures cannot leak fixture
state.

Phone Kanban remains on its contained forced-dialog branch, with the full
dialog as the single scroll owner and its existing stacked 44px actions,
viewport insets, focus return, and no document overflow. No mobile markup or
interaction changes; the existing
`apps/web/e2e/tests/kanban/mobile-card-archive-confirmation.spec.ts` is the
mobile-parity check.

## Tests

| Acceptance criterion                              | Evidence                                                                                                                                                                                         |
| ------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `AC-TASKS-CONFIRMATION-SURFACE-002.4`             | Deferred-promise regressions in `task-archive-confirmation.test.tsx`: no pending shell, then only the positive-count dialog; Escape restores trigger focus and outside pointer intent dismisses. |
| `AC-TASKS-CONFIRMATION-SURFACE-002.1`, `.3`, `.4` | Desktop `cascade-subtasks-toggle.spec.ts`: delayed classification never exposes the popover and resolves to the cascade dialog; existing zero-count coverage retains the anchored popover.       |
| `AC-UI-TASK-CLEANUP-CONFIRMATION-001.7`           | Existing component cases retain archive callbacks, failure fallback, cascade choice, and mutation-pending behavior.                                                                              |
| `AC-UI-TASK-CLEANUP-CONFIRMATION-001.8`           | Existing `mobile-card-archive-confirmation.spec.ts` retains the contained phone dialog and archive outcome.                                                                                      |

## E2E tests

- Chromium: extend `apps/web/e2e/tests/kanban/cascade-subtasks-toggle.spec.ts`
  with the delayed parent-task classification scenario.
- Mobile Chrome: re-run
  `apps/web/e2e/tests/kanban/mobile-card-archive-confirmation.spec.ts`; no new
  mobile scenario is needed because the changed branch is fine-pointer-only and
  the existing spec exercises the intentionally different phone composition.

## Work orders

- [x] [Task 01: Stabilize archive confirmation selection](task-01-stabilize-archive-confirmation-selection.md) — done

## Verification results

- RED component regression: failed on the provisional
  `task-archive-confirm-popover` before the production change.
- RED Chromium regression: failed with the same provisional popover while the
  intercepted descendant-count request remained pending.
- Review-remediation RED component cases: 2 failed because the hidden pending
  request did not retain Escape or outside-pointer dismissal.
- Review-remediation RED Chromium regression: failed because a late positive
  result mounted the cascade dialog after Escape dismissal.
- Focused component suite: 12 passed.
- Touched-file ESLint: passed with no findings.
- Web typecheck: passed.
- Mobile Chrome confirmation E2E: 2 passed.
- Chromium cascade archive/delete E2E: 6 passed, including the delayed
  classification case and a fresh desktop PR screenshot.

## Risks

- A slow descendant-count request gives no confirmation shell until the final
  presentation is known. The request remains active and normally targets the
  local backend; adding another loading overlay would recreate the shell-swap
  defect.
- Over-broadly changing unresolved classification could regress forced mobile,
  bulk, or inline callers. Keep the correction inside the standard
  fine-pointer branch and retain focused coverage for the existing routes.
- The E2E regression must wait until the intercepted request is observed before
  asserting absence, or it could pass before React enters the pending state.
