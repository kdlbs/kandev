---
id: "01-stabilize-archive-confirmation-selection"
title: "Stabilize archive confirmation selection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CONFIRMATION-SURFACE-002
  - REQ-UI-TASK-CLEANUP-CONFIRMATION-001
acceptance_criteria:
  - AC-TASKS-CONFIRMATION-SURFACE-002.1
  - AC-TASKS-CONFIRMATION-SURFACE-002.3
  - AC-TASKS-CONFIRMATION-SURFACE-002.4
  - AC-UI-TASK-CLEANUP-CONFIRMATION-001.7
  - AC-UI-TASK-CLEANUP-CONFIRMATION-001.8
system_design:
  - ../../specs/ui/system-design/confirmation-warning-hierarchy.md
---

# Task 01: Stabilize Archive Confirmation Selection

## Summary

Make descendant classification choose the standard fine-pointer archive shell
before any confirmation UI mounts. Add component and desktop browser
regressions for the delayed positive-count path while preserving every existing
final confirmation route and the contained phone dialog.

## In scope

- Remove the provisional fine-pointer loading popover from
  `TaskArchiveConfirmation` without changing its controlled request state.
- Preserve Escape and outside-pointer dismissal while the pending request has
  no rendered confirmation shell.
- Add a RED component regression that holds descendant classification pending,
  then resolves it to a positive count and observes only the cascade dialog.
- Add a RED desktop Kanban E2E regression that deterministically holds the
  subtask-count response and proves the provisional popover never appears.
- Run the existing zero-descendant desktop and contained phone confirmation
  scenarios after the correction.

## Out of scope

- Backend, archive API, user-settings, cascade, navigation, or descendant-count
  changes.
- New caching, prefetching, loading copy, animations, or confirmation surfaces.
- Changes to bulk, inline, coarse-pointer row, delete, or mobile presentation.

## Acceptance

- While a standard fine-pointer descendant classification is unresolved, no
  archive confirmation shell or actionable archive control is rendered; a
  positive result mounts only the existing full cascade dialog.
- Escape restores focus to the configured trigger, outside pointer intent
  dismisses without being blocked, and a late result cannot mount a surface
  after either dismissal.
- Resolved zero counts still mount the anchored popover, classification errors
  still fail safe to the full dialog, and callbacks/focus/preference behavior
  remain unchanged.
- The focused component, desktop E2E, existing mobile E2E, typecheck, and
  touched-file lint commands pass.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/task/task-archive-confirmation.test.tsx
cd apps/web && pnpm exec eslint components/task/task-archive-confirmation.tsx components/task/task-archive-confirmation.test.tsx e2e/tests/kanban/cascade-subtasks-toggle.spec.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm e2e:run --project mobile-chrome tests/kanban/mobile-card-archive-confirmation.spec.ts
cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --no-build tests/kanban/cascade-subtasks-toggle.spec.ts
```

The component and desktop E2E regressions must be run and observed failing for
the provisional popover before the production correction, then rerun green.

## Files likely touched

- `apps/web/components/task/task-archive-confirmation.tsx`
- `apps/web/components/task/task-archive-confirmation.test.tsx`
- `apps/web/e2e/tests/kanban/cascade-subtasks-toggle.spec.ts`

## Dependencies

None.

## Risks

- The pending branch is shared by several presentation modes; keep the change
  after the existing force-dialog/bulk decision and inside the standard
  fine-pointer path.
- A browser absence assertion can race ahead of the intercepted request; the
  test must wait for the route handler to observe the request before asserting.

## Parallelism

`sequential`

## Inputs

- `AC-TASKS-CONFIRMATION-SURFACE-002.4` and the classification-gated surface
  selection section in the UI confirmation system design.
- `apps/web/components/task/task-archive-confirmation.tsx` and its component
  test suite.
- `apps/web/e2e/tests/kanban/cascade-subtasks-toggle.spec.ts` and the existing
  held-route patterns in task E2E coverage.
- The coarse-pointer unresolved branch, which already avoids mounting a
  temporary popup.

## Results

- Added deferred descendant-count component and Chromium regressions. Before
  the implementation, both failed because the fine-pointer loading popover was
  visible while classification remained pending.
- Removed the provisional fine-pointer loading popover and its loader import.
  A non-rendering controller preserves Escape, trigger-focus, and outside
  pointer dismissal while the request waits, then renders the existing
  resolved-zero popover or safe dialog only if it remains open.
- Review-remediation RED cases exposed the missing hidden-state dismissal at
  both component and Chromium boundaries before the controller was added.
- Component suite: 12 passed. Touched-file ESLint and web typecheck passed.
- Existing Mobile Chrome confirmation spec: 2 passed. Chromium cascade spec:
  6 passed, with a fresh desktop capture of the final cascade dialog.
