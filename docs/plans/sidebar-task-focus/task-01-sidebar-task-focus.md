---
id: "01-sidebar-task-focus"
title: "Strengthen active sidebar task focus"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-TASK-FOCUS-001
acceptance_criteria:
  - AC-UI-SIDEBAR-TASK-FOCUS-001.1
  - AC-UI-SIDEBAR-TASK-FOCUS-001.2
  - AC-UI-SIDEBAR-TASK-FOCUS-001.3
  - AC-UI-SIDEBAR-TASK-FOCUS-001.4
  - AC-UI-SIDEBAR-TASK-FOCUS-001.5
system_design:
  - ../../specs/ui/system-design/sidebar-task-focus.md
---

# Task 01: Strengthen active sidebar task focus

## Summary

Give the active shared task row a stronger theme-aware background with only a
top and bottom active-task border. Preserve the user's task-color marker when a
color is assigned, omit the default marker when no color is assigned, and
preserve existing row state attributes, the separate multi-selection ring, and
current desktop/mobile interaction behavior.

## In scope

- Update the active branch of `taskItemRowClassName` in `task-item.tsx`.
- Keep the task-color `SelectionBar` visible only when a task color is set.
- Add desktop and mobile E2E assertions for colored and uncolored active rows.

## Out of scope

- Task-color data, APIs, persistence, or picker UI.
- Changes to task selection, task actions, task metadata, or mobile layout.

## Acceptance

- An active desktop row has the stronger background with 1px top and bottom
  borders only, while an inactive row does not.
- Active rows retain their custom color marker when set, render no default
  leading marker when uncolored, and retain existing `data-active` /
  `aria-current` semantics.
- The mobile task-switcher shows the same active treatment without horizontal
  overflow or loss of the row's primary tap behavior.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/task/task-item.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm e2e:run --project chromium tests/task/sidebar-task-open.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-task-actions.spec.ts -- --grep "active task"
```

## Files likely touched

- `apps/web/components/task/task-item.tsx`
- `apps/web/e2e/tests/task/sidebar-task-open.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts`

## Dependencies

None.

## Risks

- Tailwind utility order could let the generic hover class override the active
  surface. Keep active and active-hover utilities together in the selected
  branch and verify the rendered row.
- The horizontal border must not add side borders or affect the existing
  multi-selection ring or current row padding.

## Parallelism

`sequential`

## Inputs

- [Sidebar Task Focus Requirements](../../specs/ui/requirements/sidebar-task-focus.md)
- [Sidebar Task Focus System Design](../../specs/ui/system-design/sidebar-task-focus.md)
- Existing shared row path in `task-switcher-row.tsx` and mobile path in
  `session-task-switcher-sheet.tsx`.

## Results

Implemented the shared active-row surface without an active-task ring in
`TaskItem`, while preserving the custom task-color marker and existing
multi-selection ring. The desktop and mobile E2E flows now assert the
background-only treatment for a red task color.

- Red desktop E2E failed on the expected old `bg-primary/10` class.
- Focused `TaskItem` suite: 49 tests passed.
- Web typecheck passed.
- Desktop sidebar E2E: 2 tests passed.
- Mobile active-task E2E: 1 test passed, including the no-horizontal-overflow
  assertion.

For the background-only comparison variant, the desktop regression first failed
on the existing active ring, then desktop and mobile E2E passed after removing
that ring. The focused unit test suite (49 tests), typecheck, lint, and
specification checks also passed.

For the top-and-bottom border comparison variant, the desktop regression first
failed on the missing top border, then passed after adding the horizontal border
utilities. The mobile assertion is run against the same shared row styling.

For the conditional marker refinement, the desktop regression first failed on
the default marker in an uncolored active row. `SelectionBar` now renders only
when a supported task color is assigned. Desktop E2E passed with 2 tests, and
the mobile colored and uncolored active-row flows passed with 2 tests, including
the no-horizontal-overflow assertions. The focused unit suite (49 tests), web
typecheck, web lint, and specification checks also passed.
