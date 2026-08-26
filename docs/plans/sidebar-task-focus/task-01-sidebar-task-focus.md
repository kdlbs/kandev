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
system_design:
  - ../../specs/ui/system-design/sidebar-task-focus.md
---

# Task 01: Strengthen active sidebar task focus

## Summary

Give the active shared task row a stronger theme-aware background and inset
ring. Preserve the user's task-color marker, existing row state attributes,
and current desktop/mobile interaction behavior.

## In scope

- Update the active branch of `taskItemRowClassName` in `task-item.tsx`.
- Keep the task-color `SelectionBar` visible for active rows.
- Add desktop and mobile E2E assertions for the shared active treatment.

## Out of scope

- Task-color data, APIs, persistence, or picker UI.
- Changes to task selection, task actions, task metadata, or mobile layout.

## Acceptance

- An active desktop row has the stronger surface and inset ring, while an
  inactive row does not.
- Active rows retain their custom color marker and existing `data-active` /
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
- A visible border could change row dimensions. Use an inset ring and retain
  the current padding.

## Parallelism

`sequential`

## Inputs

- [Sidebar Task Focus Requirements](../../specs/ui/requirements/sidebar-task-focus.md)
- [Sidebar Task Focus System Design](../../specs/ui/system-design/sidebar-task-focus.md)
- Existing shared row path in `task-switcher-row.tsx` and mobile path in
  `session-task-switcher-sheet.tsx`.

## Results

Implemented the shared active-row surface and inset ring in `TaskItem`, while
preserving the custom task-color marker and existing multi-selection ring. The
desktop and mobile E2E flows now assert the treatment for a red task color.

- Red desktop E2E failed on the expected old `bg-primary/10` class.
- Focused `TaskItem` suite: 49 tests passed.
- Web typecheck passed.
- Desktop sidebar E2E: 2 tests passed.
- Mobile active-task E2E: 1 test passed, including the no-horizontal-overflow
  assertion.
