---
id: "11-shared-local-remote-selector"
title: "Shared Local and Remote selector"
status: done
wave: 7
depends_on:
  - "06-shared-source-picker"
plan: "plan.md"
spec: "../../specs/tasks/attach-workspace-sources.md"
---

# Task 11: Shared Local and Remote selector

## Acceptance

- Task creation and Add sources consume one controlled segmented source-mode component. Task
  creation retains **Repo / Remote / None**, its state semantics, and existing test IDs; Add sources
  renders **Local / Remote** with 44px options on phone viewports.
- Add sources reuses the combined saved/discovered local repository picker in **Local** and the
  provider-backed/pasted-URL picker in **Remote**. The local-folder/manual-path affordance remains
  available only when the executor supports it.
- Switching modes never removes configured rows, validation state, or mixed-batch payload data.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run \
  components/task-create-dialog-repo-chips.test.tsx \
  components/task/add-workspace-sources/add-workspace-sources-dialog.test.tsx \
  components/workspace-source-picker/workspace-source-state.test.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web exec eslint \
  components/task-create-dialog-source-mode.tsx \
  components/task-create-dialog-repo-chips.tsx \
  components/task/add-workspace-sources/add-workspace-sources-dialog.tsx \
  --max-warnings=0
```

## Files likely touched

- `apps/web/components/task-create-dialog-source-mode.tsx`
- `apps/web/components/task-create-dialog-repo-chips.tsx`
- `apps/web/components/task-create-dialog-remote-repo-chips.tsx`
- `apps/web/components/task/add-workspace-sources/add-workspace-sources-dialog.tsx`
- `apps/web/components/task/add-workspace-sources/add-workspace-sources-dialog.test.tsx`
- `apps/web/components/workspace-source-picker/`
- `apps/web/components/task-create-dialog-*.test.tsx`

## Dependencies

Task 06.

## Inputs

- Spec: **What** source-mode requirements and Local/Remote scenarios.
- Plan: **Frontend → Shared source picker** and **Mobile design contract**.
- Patterns: `WorkspaceRepoChips`, `RemoteRepoChip`, `useDiscoverReposEffect`, and the existing
  create-task `SourceModeSwitch`.

## Output contract

- Update only this task file to `in_progress` when starting and `done` after acceptance and
  verification pass.
- Return a compact handoff capsule: accepted behaviors, base/head SHA, changed files and entry
  points, targeted command results, risk tags, and any uncertainty.
- Do not update `plan.md`; the planner serializes shared-plan status.
