---
id: "04-stabilize-contribution-props"
title: "Stabilize contribution props"
status: done
wave: 4
depends_on:
  - "03-stabilize-sidebar-rows"
plan: "plan.md"
spec: "../../specs/ui/requirements/sidebar-task-row-presentation.md"
system_design: "../../specs/ui/system-design/task-surface-render-isolation.md"
---

# Task 04: Stabilize contribution props

## Acceptance

- Plugin `sessionIds` keeps its identity while the ordered identifiers remain equal.
- The plugin contribution does not rerender for session-state changes outside its slot props.
- Pull-request hydration returns one stable aggregate while its semantic inputs remain equal.
- Pull-request indicators reuse a stable empty list and still update when pull-request data changes.
- Plugin host APIs, provider status derivation, and visible task-row output remain unchanged.

This task preserves `AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.8` and `.12` for pull-request
indicators. The plugin portion is an internal render refinement with no public contract change.

## TDD sequence

1. Add a failing plugin render-count test for a session-object update with unchanged identifiers.
2. Add a failing hook identity test for unchanged pull-request hydration inputs.
3. Add a failing pull-request icon test for the stable empty-list path.
4. Stabilize each derived value at its owning component or hook boundary.
5. Run the focused tests, typecheck, and frontend lint.

## Verification

```bash
cd apps
rtk pnpm --filter @kandev/web test components/task/task-top-bar-plugin-actions.test.tsx hooks/domains/github/use-task-pr-tooltip-hydration.test.tsx components/github/pr-task-icon.render.test.tsx
rtk pnpm --filter @kandev/web typecheck
rtk pnpm --filter @kandev/web lint
```

## Files likely touched

- `apps/web/components/task/task-top-bar-plugin-actions.tsx`
- `apps/web/components/task/task-top-bar-plugin-actions.test.tsx`
- `apps/web/hooks/domains/github/use-task-pr-tooltip-hydration.ts`
- `apps/web/hooks/domains/github/use-task-pr-tooltip-hydration.test.tsx`
- `apps/web/components/github/pr-task-icon.tsx`
- `apps/web/components/github/pr-task-icon.render.test.tsx`

## Dependencies

- Task 03 completes sidebar callback stabilization before shared row indicators change.

## Inputs

- `REQ-UI-SIDEBAR-TASK-ROW-PRESENTATION-001`
- Task Surface Render Isolation system design
- Existing plugin host API and pull-request status behavior
- Trace evidence for equal `sessionIds`, hydration, and pull-request props

## Output contract

Report each stable-value strategy, render-count results, changed files, focused test results, final
frontend checks, and remaining risks. Do not change plugin or integration contracts.
