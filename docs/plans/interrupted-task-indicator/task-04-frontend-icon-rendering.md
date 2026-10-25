---
id: "04-frontend-icon-rendering"
title: "Frontend interrupted icon rendering"
status: done
wave: 4
depends_on: ["03-frontend-data-plumbing"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/interrupted-task-indicator.md"
---

# Task 04: Frontend interrupted icon rendering

## Acceptance

- `TaskItem` renders a red alert icon (`IconAlertCircle`, `text-red-500`,
  `data-testid="task-state-interrupted"`) when `interrupted` is true and no
  higher-priority affordance applies: pending permission, pending
  clarification, generating/background activity, preparing, and the running
  spinner all win; the review/done affordances lose to the red icon.
- `getTaskStateIcon` / `getTaskStateIconConfig` accept an `interrupted`
  parameter and return the red icon for non-terminal states; every caller
  (kanban card, graph node, swimlane node, rich task-list row, task state
  actions) passes the task's field.
- The icon has a focusable tooltip and accessible label with externalized copy
  ("Interrupted by restart") through `t()`; `pnpm run i18n:ratchet` passes for
  the changed files.
- `COMPLETED`, `FAILED`, and `CANCELLED` tasks never render the red icon.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/task/task-item.test.tsx lib/ui/state-icons.test.tsx
cd apps/web && pnpm run typecheck
cd apps && pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/components/task/task-item.tsx` (+ `task-item.test.tsx`)
- `apps/web/lib/ui/state-icons.tsx` (+ `state-icons.test.tsx`)
- `apps/web/components/kanban-card-content.tsx`
- `apps/web/components/kanban/graph2-step-node.tsx`
- `apps/web/components/kanban/swimlane-graph-content.tsx`
- `apps/web/app/tasks/rich-task-list-row.tsx`
- `apps/web/components/task/task-state-actions.tsx`
- `apps/web/src/locales/en/*.json` (new copy key)

## Dependencies

Task 03 (the field must reach `TaskItem` and the kanban task rows).

## Inputs

- Spec: `What` (icon precedence), `Scenarios` 1 and 6, `Out of scope`.
- Plan: `Frontend > Icon rendering`.
- Existing pattern: `BackgroundWorkTaskIcon` in `task-item.tsx` (focusable
  tooltip wrapper, `aria-label`, `data-testid`) and the icon config map in
  `state-icons.tsx` (`STYLE_ERROR = "text-red-500"`).

## Risks

- Positional-argument growth in `getTaskStateIcon` — keep the existing
  argument order and update every caller and test; the signature is public to
  the surfaces listed above (grep for `getTaskStateIcon(` to catch stragglers).
- The interrupted branch must sit after the activity/preparing/running checks
  in `TaskStateIcon` or a resumed task briefly shows red over its spinner.
- The i18n ratchet judges changed lines: the tooltip/aria copy must use `t()`
  with a real locale key, never a literal.
- Tooltip text must be stable and never compared with `===` elsewhere.

## Output contract

Report the precedence order implemented, all `getTaskStateIcon` callers
updated, the i18n key added, focused test and ratchet results, then mark this
task `done` and update `plan.md`.
