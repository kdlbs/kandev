---
id: "04-build-autopilot-ui"
title: "Build the autopilot task UI"
status: done
wave: 2
depends_on:
  - "01-persist-task-contract"
plan: "plan.md"
spec: "../../specs/tasks/requirements/autopilot-mode.md"
---

# Task 04: Build the Autopilot Task UI

## Acceptance

- Task and subtask creation modes offer an off-by-default, accessible autopilot
  switch and serialize it; edit and new-session modes do not imply mutability.
- Autopilot tasks render a yellow localized chip above the composer and a secondary
  localized sidebar icon while preserving the primary pending-question icon.
- Shared desktop/mobile components consume one mapped task property, remain usable
  at narrow viewports, and pass unit, type, i18n, and accessibility checks.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web test -- --run components/task-create-dialog-defaults.test.ts components/task-create-dialog-form-body.test.tsx components/task/new-subtask-form-parts.test.tsx components/task/new-subtask-form-state.test.ts components/task/use-subtask-submit.test.ts components/task/task-item.test.tsx components/task/chat/chat-input-area.test.tsx lib/kanban/map-task.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
```

## Files likely touched

- `apps/web/lib/types/http.ts`
- `apps/web/lib/api/domains/kanban-api.ts`
- `apps/web/lib/kanban/map-task.ts`
- `apps/web/components/task-create-dialog-types.ts`
- `apps/web/components/task-create-dialog-state.ts`
- `apps/web/components/task-create-dialog-defaults.ts`
- `apps/web/components/task-create-dialog-form-body.tsx`
- `apps/web/components/task-create-dialog-prop-builders.ts`
- `apps/web/components/task-create-dialog-submit.tsx`
- `apps/web/components/task/new-subtask-dialog.tsx`
- `apps/web/components/task/new-subtask-form-state.ts`
- `apps/web/components/task/new-subtask-form-parts.tsx`
- `apps/web/components/task/use-subtask-submit.ts`
- `apps/web/components/task/task-session-sidebar-item.ts`
- `apps/web/components/task/task-switcher-types.ts`
- `apps/web/components/task/task-item.tsx`
- `apps/web/components/task/chat/chat-input-area.tsx`
- `apps/web/src/locales/en/task.json`
- `apps/web/src/locales/pt-pt/task.json`
- `apps/web/src/locales/pseudo/task.json`
- `apps/web/src/locales/zh-cn/task.json`
- `apps/web/components/task-create-dialog-defaults.test.ts`
- `apps/web/components/task-create-dialog-form-body.test.tsx`
- `apps/web/components/task/new-subtask-form-parts.test.tsx`
- `apps/web/components/task/new-subtask-form-state.test.ts`
- `apps/web/components/task/use-subtask-submit.test.ts`
- `apps/web/components/task/task-item.test.tsx`
- `apps/web/components/task/chat/chat-input-area.test.tsx`
- `apps/web/lib/kanban/map-task.test.ts`

## Dependencies

- Task 01 supplies the HTTP/DTO field and compatibility behavior.

## Parallelism

Can run in parallel with Task 02. This task owns frontend API, dialog, task-row,
composer, and locale files; Task 02 owns backend runtime files.

## Inputs

- Spec sections `User experience`, `Creation API`, and the mobile acceptance scenario.
- Plan `Mobile design contract`.
- Existing create dialog boolean rows, `ChatStatusBar`, `TaskItem`, and
  `taskPendingAction` mapping.

## Output contract

Report the frontend field/default, create-mode visibility rules, component and
translation keys, icon precedence, keyboard/touch behavior, narrow-width overflow
result, tests/checks run, and screenshots or DOM evidence used for accessibility.

## Results

Done. Added create-only switches for tasks and subtasks, shared task mapping,
localized yellow sidebar/chat indicators, pending-question precedence, and mobile
sheet propagation. Focused tests, typecheck, lint, i18n checks, and narrow viewport
overflow assertions pass. The fixup also covers stale direct-task fallback and
primary-session-clear WebSocket preservation.
