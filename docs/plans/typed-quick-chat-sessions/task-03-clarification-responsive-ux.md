---
id: "03-clarification-responsive-ux"
title: "Clarification responsive UX"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/quick-chat-expiration.md"
---

# Task 03: Clarification Responsive UX

## Acceptance

- A pending clarification remains an inline, bounded, scrollable bottom region while message history
  retains meaningful visible space.
- An accessible collapse/expand control works without resolving or dismissing the question and is
  usable at narrow mobile widths.
- Existing resize and answer behavior remains intact.

## Verification

`cd apps && pnpm --filter @kandev/web test -- --run components/task/chat/clarification-custom-input.test.tsx`

`cd apps/web && pnpm run typecheck`

## Files Likely Touched

- `apps/web/components/quick-chat/quick-chat-content.tsx`
- `apps/web/components/task/chat/clarification-input-overlay.tsx`
- `apps/web/hooks/use-resizable-clarification-overlay.ts`
- Focused adjacent test files for extracted logic, if needed.

## Inputs

- Spec: Clarification and mobile scenarios.
- Existing `ResizeHandle`, `useResizableClarificationOverlay`, and clarification overlay patterns.

## Dependencies

None. Coordinate any shared `quick-chat-content.tsx` edit before Task 02 integration.

## Output Contract

Report behavior changed, files touched, red/green test commands, blockers, residual risks, and mark
this task `done` in this file and `plan.md`.

## Result

- Quick Chat clarifications now have a 44px accessible collapse/expand control. Collapsing preserves
  the pending question, selected answers, custom drafts, and the prior user-resized height.
- The expanded header and scrollable body share a `50vh` cap, leaving message history visible on
  desktop and mobile. Hidden carousel keyboard shortcuts are disabled while collapsed.
- Focused clarification tests (23), frontend typecheck, zero-warning focused lint, and diff checks
  pass. Desktop/mobile rendered verification remains in Task 04.
