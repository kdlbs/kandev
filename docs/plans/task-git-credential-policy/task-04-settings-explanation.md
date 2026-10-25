---
id: "04-settings-explanation"
title: "Add task credential settings and method explanations"
status: done
wave: 2
depends_on: ["01-policy-persistence"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 04: Add Task Credential Settings And Method Explanations

## Acceptance

- Workspace GitHub settings load/save/discard the separate managed/executor task credential policy
  through the shared settings-save coordinator.
- PAT, named CLI, and App choices visibly explain storage/resolution and managed task delivery;
  policy copy visibly explains local/Worktree, remote executor, and profile-token precedence.
- Fine pointers get hover/focus help and coarse pointers get the same content in a 44px-target
  Drawer without horizontal overflow.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/github/github-task-credentials-section.test.tsx components/github/github-auth-method-list.test.tsx
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/types/github.ts`
- `apps/web/components/github/github-task-credentials-section.tsx`
- focused component/model tests beside it
- `apps/web/components/github/github-auth-method-list.tsx`
- `apps/web/components/github/github-connection-dialog.tsx`
- `apps/web/components/github/github-settings.tsx`
- this task file and `plan.md`

## Dependencies

Task 01.

## Parallelism

Parallel-safe with Task 02 after Task 01: it owns frontend settings files only.

## Inputs

- Spec `Choosing A Method`, `UX And Mobile Contract`, and policy scenarios.
- Task 01 workspace-settings API field.
- `GitHubRepoScopeSection` responsive information-help and save-contributor patterns.

## Output contract

Report visible option copy, help content, responsive primitive choice, save/discard behavior,
RED/GREEN/typecheck results, files changed, and update task/plan status.
