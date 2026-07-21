---
id: "02-task-create-selector"
title: "Task-create local repository selector"
status: done
wave: 2
depends_on: ["01-backend-initialization"]
plan: "plan.md"
spec: "../../specs/create-local-repository/spec.md"
---

# Task 02: Task-Create Local Repository Selector

## Acceptance

- A task-create repository picker exposes **Create new repository**, and success selects the returned
  repository with `main` only in the originating row, moves the first task to a compatible direct
  local executor, explains that change, and preserves the rest of the task draft.
- Multi-repository tasks do not expose the action because they require worktree execution.
- The shared directory browser drives name/parent validation, target preview, retryable errors, and
  idempotent active-workspace cache insertion without changing the **None** mode picker.
- Desktop Dialog and mobile Drawer satisfy the spec's focus, scroll, safe-area, touch-target, and
  no-hover mobile contract; Quick Chat does not gain the action, and no compatible direct local
  profile blocks creation before the API call.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run \
  components/create-local-repository-surface.test.tsx \
  components/task-create-dialog-pill.test.tsx \
  components/task-create-dialog-repo-chips.test.tsx \
  components/task-create-dialog-handlers.test.ts
cd apps/web && pnpm run typecheck && pnpm run lint
```

## Files Likely Touched

- `apps/web/lib/api/domains/workspace-api.ts`
- `apps/web/components/folder-picker.tsx`
- `apps/web/components/create-local-repository-surface.tsx`
- `apps/web/components/create-local-repository-surface.test.tsx`
- `apps/web/components/task-create-dialog-pill.tsx`
- `apps/web/components/task-create-dialog-pill.test.tsx`
- `apps/web/components/task-create-dialog-workspace-repo-chips.tsx`
- `apps/web/components/task-create-dialog-repo-chips.tsx`
- `apps/web/components/task-create-dialog-repo-chips.test.tsx`
- `apps/web/components/task-create-dialog-handlers.ts`
- `apps/web/components/task-create-dialog-handlers.test.ts`
- `apps/web/components/task-create-dialog-types.ts`
- `apps/web/components/task-create-dialog-prop-builders.ts`
- `apps/web/components/task-create-dialog.tsx`

## Dependencies

Task 01's endpoint and response/error contract.

## Inputs

- Spec: What, API Surface, Mobile Design Contract, Scenarios.
- Shared browser pattern: `apps/web/components/folder-picker.tsx`.
- Mobile geometry exemplar: `apps/web/components/task/mobile/mobile-picker-sheet.tsx`.
- Selector constraint: the cmdk-only note in `apps/web/components/task-create-dialog-pill.tsx`.
- Repository cache: `apps/web/lib/state/slices/workspace/workspace-slice.ts` and
  `apps/web/hooks/domains/workspace/use-repositories.ts`.

## Output Contract

Report desktop/mobile behavior, state and focus handling, tests run, files touched, blockers, and
remaining visual risks. Mark this file `done` and update the plan entry only after targeted tests pass.
