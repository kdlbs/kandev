---
created: 2026-08-27
status: done
requirements:
  - REQ-UI-RESPONSIVE-PLAN-FORMATTING-001
system_design:
  - ../../specs/ui/system-design/responsive-plan-formatting.md
legacy_specs: []
---

# Implementation Plan: Stop the Plan Bubble Menu Transaction Loop

## Overview

Stop the desktop Plan editor from running a continuous React and Tiptap
transaction loop. Implement one focused TDD work order because the fault and
its regression boundary are in one component.

The trace shows approximately 44,800 React scheduler callbacks in 3.56
seconds. The DOM stays at 4,862 elements. The source-mapped stack shows a
Tiptap BubbleMenu option update followed by an editor transaction and another
PlanBubbleMenu render.

## Scope

### In scope

- Keep the desktop BubbleMenu option identity stable across unchanged renders.
- Publish a new editor snapshot only when its derived values change.
- Add a focused regression that models the real BubbleMenu option-update
  transaction.
- Preserve the desktop selection bubble and the mobile docked formatting
  strip.

### Out of scope

- Change Plan formatting commands, layout, touch geometry, or viewport
  behavior.
- Change Tiptap, ProseMirror, task persistence, or WebSocket code.
- Add performance telemetry or a general React render monitor.
- Change the public plugin editor contract.

## Technical approach

### Stable BubbleMenu inputs

Define the placement options once in
`apps/web/components/editors/tiptap/plan-bubble-menu.tsx`. Pass that stable
object to the desktop BubbleMenu.

### Deduplicated editor snapshots

Compare the next `EditorSnapshot` with the current snapshot. Return the
current state when all derived values are equal. Publish the next snapshot
only after focus, selection, context, or active marks change.

### Regression boundary

Extend `plan-bubble-menu.test.tsx` with a controlled BubbleMenu fake. The fake
models Tiptap's option dependency and its metadata transaction. The test fails
on the current code because the transaction repeats after each render.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-UI-RESPONSIVE-PLAN-FORMATTING-001.1` | The focused Vitest regression proves that the desktop BubbleMenu settles after one option update. Focus, selection, active-mark, and desktop/mobile code-block transitions remain reactive. |
| `AC-UI-RESPONSIVE-PLAN-FORMATTING-001.2` through `.8` | The existing mobile component tests and `mobile-plan-formatting-toolbar.spec.ts` prove that the docked presentation keeps its current behavior. |

## E2E tests

This fix changes render-state normalization only. It does not change layout,
touch behavior, scrolling, navigation, or viewport-dependent interaction.

The existing `mobile-plan-formatting-toolbar.spec.ts` remains the mobile
parity evidence. Run its `mobile-chrome` scenario after the focused unit test.

## Work orders

- [x] [Task 01: Stop the BubbleMenu transaction loop](task-01-stop-bubble-menu-transaction-loop.md) (done)

## Verification results

- Red regression: the controlled BubbleMenu fake observed four option-update
  transactions before the fix and failed the settlement assertion.
- `cd apps/web && pnpm exec vitest run components/editors/tiptap/plan-bubble-menu.test.tsx`
  passed, 13 tests, including active-mark and code-block transaction changes.
- `cd apps/web && pnpm run typecheck` passed.
- `cd apps/web && pnpm exec eslint components/editors/tiptap/plan-bubble-menu.tsx
  components/editors/tiptap/plan-bubble-menu.test.tsx` passed.
- `cd apps/web && pnpm e2e:run --project mobile-chrome
  tests/task/mobile-plan-formatting-toolbar.spec.ts` passed, 1 test.
- Fresh desktop and mobile PR screenshots were captured, inspected, validated,
  and compressed. The disposable desktop capture spec was removed.

## Risks

- An incomplete snapshot comparison can leave pressed formatting state stale.
- An unstable BubbleMenu prop can reintroduce metadata transactions on each
  render.
- A weak menu mock can hide the Tiptap effect that caused the fault.

## Public documentation

None. This fix changes no public command, configuration, API, workflow, or
plugin contract.

## Decisions

No ADR is required. The fix completes the current component design without a
new ownership or public contract boundary.
