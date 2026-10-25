---
id: "02-frontend-title-limit"
title: "Frontend title inputs and remote prefill"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/title-length-limit.md"
---

# Task 02: Frontend title inputs and remote prefill

## Acceptance

- Every task-title editor clamps typed and pasted input to 60 characters on desktop and mobile.
- Remote initial values, restored drafts, GitHub URL suggestions, and Jira/Linear imports longer than 60 characters become 59 characters plus `…` without changing the full generated description or remote association.
- Existing create/edit/rename/subtask/Office dialog composition, focus behavior, scrolling, and actions remain unchanged.

## Verification

```bash
(
  cd apps
  pnpm --filter @kandev/web test -- --run lib/task-title.test.ts components/task-create-dialog-state.test.ts components/task-create-dialog-submit.test.tsx
)
(
  cd apps/web
  pnpm run typecheck
)
```

## Files likely touched

- `apps/web/lib/task-title.ts`
- `apps/web/lib/task-title.test.ts`
- `apps/web/components/task-create-dialog-selectors.tsx`
- `apps/web/components/task-create-dialog-state.ts`
- `apps/web/components/task-create-dialog-state.test.ts`
- `apps/web/components/task-create-dialog.tsx`
- `apps/web/components/task-create-dialog-form-body.test.tsx`
- `apps/web/components/task/task-rename-dialog.tsx`
- `apps/web/components/task/task-rename-dialog.test.tsx`
- `apps/web/components/task/new-subtask-form-parts.tsx`
- `apps/web/components/task/new-subtask-form-parts.test.tsx`
- `apps/web/app/office/components/new-task-dialog.tsx`
- `apps/web/app/office/components/new-task-dialog.test.tsx`

## Dependencies

None. The numeric limit mirrors Task 01's backend contract; if either task changes the agreed value, update both before completion.

## Parallelism

Parallel-safe with Task 01 because frontend files are disjoint from backend/MCP files. Execute sequentially unless the user explicitly authorizes subagents.

## Inputs

- Spec sections: **What**, **Failure modes**, and the UI/remote-prefill scenarios.
- Plan sections: **Frontend**, **Mobile design contract**, and **Tests**.
- Existing patterns: `InlineTaskName`, `resolveFormDefaults`, `useTitleAutofillFromPrimaryGitHubInfo`, `TaskRenameDialog`, `SubtaskFormBody`, and Office `NewTaskDialog`.

## Risks

- Apply ellipsis only to system/remote-prefilled values; manual input is capped without silently rewriting it to an ellipsis form.
- Keep the provider's full title in descriptions and associations; truncate only the task-title state.

## Output contract

Report the changed files, exact test commands/results, rendered mobile check or exact blocker, and remaining risks, then mark this task `done` and update its checkbox in `plan.md`.
