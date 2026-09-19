---
id: "03-stabilize-sidebar-rows"
title: "Stabilize sidebar row inputs"
status: done
wave: 3
depends_on:
  - "02-virtualize-file-tree"
plan: "plan.md"
spec: "../../specs/ui/requirements/sidebar-task-row-presentation.md"
system_design: "../../specs/ui/system-design/task-surface-render-isolation.md"
---

# Task 03: Stabilize sidebar row inputs

## Acceptance

- `onBulkMove` keeps its identity during unrelated sidebar owner renders.
- `TaskSwitcher` skips an unrelated owner render when its semantic props remain equal.
- A task-row update rerenders the affected row and preserves current selection behavior.
- Fine-pointer trailing content and the mobile primary row action remain unchanged.

This task preserves `AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.8` and `.12`. It also preserves
`AC-UI-MOBILE-TASK-NAVIGATION-001.1`, `.2`, `.3`, and `.7`.

## TDD sequence

1. Add a failing identity test for `onBulkMove`.
2. Add a failing render-count test for an unrelated sidebar owner update.
3. Derive the callback from stable action functions and current selection data.
4. Add or refine memoized boundaries only where the failing test requires them.
5. Run the focused selection and task-switcher tests.

## Verification

```bash
cd apps
rtk pnpm --filter @kandev/web test components/task/task-session-sidebar-selection.test.ts components/task/task-switcher.test.tsx components/task/task-switcher-context-menu.test.tsx
rtk pnpm --filter @kandev/web typecheck
```

## Files likely touched

- `apps/web/components/task/task-session-sidebar-selection.tsx`
- `apps/web/components/task/task-switcher.tsx`
- `apps/web/components/task/task-switcher-row.tsx`
- `apps/web/components/task/task-session-sidebar-selection.test.ts`
- `apps/web/components/task/task-switcher.test.tsx`

## Dependencies

- Task 02 completes the higher-impact file-tree path before this sidebar refinement.

## Inputs

- `REQ-UI-SIDEBAR-TASK-ROW-PRESENTATION-001`
- `REQ-UI-MOBILE-TASK-NAVIGATION-001`
- Sidebar Task Row Presentation system design
- Trace evidence for `onBulkMove` and repeated task-row renders

## Output contract

Report the callback dependency change, memoized boundary, render-count result, changed files,
focused test results, and remaining risks.
