---
id: "06-shared-source-picker"
title: "Shared source picker"
status: done
wave: 3
depends_on: ["02-attachment-service"]
plan: "plan.md"
spec: "../../specs/tasks/attach-workspace-sources.md"
---

# Task 06: Shared Source Picker

## Acceptance

- Reusable workspace-repository, remote-repository, branch, and folder selection leaves live under
  `components/workspace-source-picker/` with task-create behavior unchanged.
- Mixed source form state supports independent stable rows, validation, retry, and exact HTTP payload
  construction for repository, remote repository, and folder kinds.
- Executor capability hides the folder row kind for Docker, SSH, Sprites, and remote Docker while
  leaving local and remote repository choices available.
- Frontend API/types/store contracts model workspace folders and attachment responses, and the old
  Worktree-only multi-repository guard is removed only after runtime capability is present.

## Verification

```bash
cd apps/web
rtk pnpm test -- components/workspace-source-picker components/task-create-dialog lib/api/domains/kanban-api
rtk pnpm run typecheck
```

## Files likely touched

- `apps/web/components/workspace-source-picker/**` (new)
- `apps/web/components/task-create-dialog-workspace-repo-chips.tsx`
- `apps/web/components/task-create-dialog-remote-repo-chip.tsx`
- `apps/web/components/task-create-dialog-remote-repo-chips.tsx`
- `apps/web/components/folder-picker.tsx`
- `apps/web/components/task-create-dialog-computed.ts`
- `apps/web/components/task-create-dialog-multi-repo-guard.ts`
- `apps/web/lib/api/domains/kanban-api.ts`
- `apps/web/lib/types/http.ts`
- `apps/web/lib/state/slices/kanban/types.ts`

## Dependencies

Task 02 for the final request contract. This task may begin extraction in parallel with Tasks 03/04
but must not delete the executor guard until Task 05 is integrated.

## Inputs

- Existing task-create picker files named above.
- Spec: mixed batch inputs and executor parity.

## Output contract

Summary, files changed, tests run, blockers, risks, divergence, and task/plan status updates.

## Completion notes

- Added the shared typed source-row state, validation, deterministic request
  builder, and executor capability helper under
  `apps/web/components/workspace-source-picker/`.
- Re-exported the existing task-create repository, remote repository, branch,
  and folder picker leaves from the shared namespace without changing their
  behavior. Task 07 owns the Add sources desktop/dialog/drawer compositions.
- Mobile parity handoff: Task 07 must consume this same state/capability layer
  in a phone-native, viewport-contained `MobilePickerSheet` or Drawer with a
  single internal scroll owner and 44px touch controls; this task introduces
  no rendered outer surface, so no mobile E2E is appropriate yet.
- Kept the existing Worktree-only multi-repository guard in
  `task-create-dialog-computed.ts` and `task-create-dialog-multi-repo-guard.ts`.
  **Deferred handoff to Task 05:** remove or relax that create-task guard only
  after Task 05 wires the remote runtime multi-repository capability and its
  executor profile contract; this attachment-picker capability does not make
  create-task provisioning safe by itself.
