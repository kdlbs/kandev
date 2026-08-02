---
id: "03-frontend-title-flow"
title: "Settings and task/subtask dialogs"
status: done
wave: 3
depends_on: ["01-backend-title-lifecycle", "02-mcp-title-tool"]
plan: "plan.md"
spec: "../../specs/tasks/agent-generated-titles.md"
---

# Task 03: Settings and task/subtask dialogs

## Acceptance

- Task Actions exposes a self-documenting, manually saved setting that defaults false and stays synced
  through boot, HTTP, and WebSocket settings paths.
- With the setting enabled, create-mode task and subtask dialogs omit title controls, require/focus the
  prompt, and send `auto_title:true`; disabled and edit modes retain current title behavior.
- Desktop and phone reuse the same state, validation, and submit logic while preserving their existing
  dialog geometry, scroll ownership, touch targets, and non-title actions.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/settings/agent-generated-task-title-settings.test.tsx lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts components/task-create-dialog.test.tsx components/task-create-dialog-footer.test.ts components/task-create-dialog-form-body.test.tsx components/task-create-dialog-helpers.test.ts components/task-create-dialog-setup.test.ts components/task/new-subtask-form-parts.test.tsx components/task/use-subtask-submit.test.ts
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm run typecheck
```

The focused Vitest suite passed 108 tests across 10 files; frontend lint (`cd apps && pnpm --filter
@kandev/web lint`) and TypeScript typecheck also passed.

## Files likely touched

- `apps/web/components/settings/general-settings.tsx`
- `apps/web/components/settings/agent-generated-task-title-settings.tsx`
- its focused component test
- `apps/web/lib/state/slices/settings/types.ts`
- `apps/web/lib/state/slices/settings/settings-slice.ts`
- `apps/web/lib/types/http-user-settings.ts`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/ssr/user-settings.ts`
- `apps/web/lib/ssr/user-settings.test.ts`
- `apps/web/lib/ws/handlers/users.ts`
- `apps/web/lib/ws/handlers/users.test.ts`
- `apps/web/components/task-create-dialog.tsx`
- `apps/web/components/task-create-dialog-types.ts`
- `apps/web/components/task-create-dialog-form-body.tsx`
- `apps/web/components/task-create-dialog-footer.tsx`
- `apps/web/components/task-create-dialog-submit.tsx`
- `apps/web/components/task-create-dialog-helpers.ts`
- `apps/web/components/task-create-dialog-prop-builders.ts`
- `apps/web/lib/api/domains/kanban-api.ts`
- focused dialog/helper tests
- `apps/web/components/task/new-subtask-dialog.tsx`
- `apps/web/components/task/new-subtask-form-parts.tsx`
- `apps/web/components/task/use-subtask-submit.ts`
- focused subtask tests

## Dependencies

Tasks 01 and 02 define the HTTP and agent-completion contracts the UI opts into.

## Parallelism

Sequential. The task spans shared user-settings and task-create types and should remain one coherent
frontend change.

## Inputs

- Spec sections: **What**, **User settings**, desktop/mobile scenarios.
- Plan sections: **User setting and hydration**, **New Task dialog**, **New Subtask dialog and mobile
  contract**.
- Mobile exemplars: current `TaskCreateDialog` and `NewSubtaskDialog` phone compositions.

## Risks

- Avoid a title-field flash by relying on boot-hydrated settings before the dialog opens.
- Do not let hidden Jira/Linear/GitHub autofill values accidentally become submitted manual titles.
- Empty-prompt gating must cover voice submit, split actions, plan mode, and create-only paths.

## Output contract

Report behavior implemented, files changed, the exact test/typecheck commands and results, blockers or
risks, and update this task plus `plan.md` status in the same conversation.
