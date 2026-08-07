---
id: "02-frontend-bootstrap-workspace"
title: "Frontend: use the bootstrap workspace"
status: completed
wave: 2
depends_on: ["01-backend-dedicated-workspace"]
plan: "plan.md"
spec: "../../specs/improve-kandev/spec.md"
---

# Task 02: Frontend — create tasks in the bootstrap workspace

The Improve Kandev dialog must create tasks in the dedicated workspace the
bootstrap response returns, not in the user's active workspace.

## Acceptance

1. `ImproveKandevBootstrapResponse` includes `workspace_id: string`.
2. `useBootstrapKandev` lists repositories and populates the store for
   `data.workspace_id` (not the active workspace), so the locked repo chip
   resolves a label for the dedicated workspace's repository.
3. `CreateModeView` passes `ready.data.workspace_id` to `TaskCreateDialog`
   when bootstrap is ready, so the created task lands in the dedicated
   workspace. While loading/error the active workspace id remains the fallback
   and submit stays blocked.

## Verification

```sh
cd apps && pnpm --filter @kandev/web typecheck
cd apps/web && pnpm vitest run components/improve-kandev-dialog.test.tsx
```

Unit coverage (added with the change): `components/improve-kandev-dialog.test.tsx`
asserts bootstrap is called with the workspace-choice flag, repositories are
listed and stored for the bootstrap's `workspace_id` (not the active one), and
the workspace-creation choice gates bootstrap when the dedicated workspace is
missing. The task-landing-in-the-dedicated-workspace behavior itself is covered
by the E2E isolation test in task 03.

## Files likely touched

- `apps/web/lib/api/domains/improve-kandev-api.ts`
- `apps/web/components/improve-kandev-dialog.tsx`
- `apps/web/components/improve-kandev-dialog-create.tsx`

## Dependencies

Task 01 (response contract: `workspace_id`).

## Parallelism

Sequential (wave 2).

## Inputs

- Spec: "API surface" (`workspace_id` in the bootstrap response) and
  "Scenarios" (task lands in the dedicated workspace).
- Plan: `docs/plans/improve-kandev-workspace/plan.md` Frontend section.
- Existing wiring: `useBootstrapKandev` in `improve-kandev-dialog.tsx`
  (currently `listRepositories(workspaceId)` + `setRepositories(workspaceId,
  ...)`), `CreateModeView` in `improve-kandev-dialog-create.tsx` (currently
  passes `props.workspaceId` to `TaskCreateDialog`).

## Output contract

Summary, files changed, typecheck result, blockers, risks, and task/plan
status update in the same conversation.

## Risks

- The dialog's `useGitHubAuthCheck` keeps using the active workspace for the
  fix URL — intentional; do not switch it to the dedicated workspace.
- If `data.workspace_id` is missing (stale backend), `listRepositories(undefined)`
  would 400 — the E2E mocks and the real backend both return it after task 01;
  treat a missing field as a bootstrap error surfaced by the existing catch.

## Results

Shipped in `09733d2f2`, `cc7b66550` (PR #2347). The app boot payload's
`workspace_id`/`workspaces` now drive the Improve Kandev dialog; the dialog's
`WorkspaceChoicePanel` + `CreateWorkspaceCheckbox` opt into workspace creation
(default checked) when the dedicated workspace does not exist, and
`useBootstrapKandev` sends `create_workspace` accordingly. "New Task" in the
dedicated workspace opens the Improve Kandev dialog via the shared
`appSidebar.improveDialogOpen` store flag. Unit coverage:
`components/improve-kandev-dialog.test.tsx` (bootstrap wiring + copy), all pass
with `pnpm vitest run components/improve-kandev-dialog.test.tsx`.
