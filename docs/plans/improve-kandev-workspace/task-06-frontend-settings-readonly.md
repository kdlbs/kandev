# task-06: Frontend read-only settings for the dedicated workspace

Spec: `docs/specs/workspaces/requirements/improve-kandev.md` — "Dedicated workspace immutability"
Plan: `docs/plans/improve-kandev-workspace/plan.md` — Phase 2, Wave 6.
Depends on: task-04, task-05.

## Goal

The workspace settings pages for the dedicated `Improve Kandev` workspace are
read-only: the workflows page lists the hidden `improve-kandev` (Improve →
Test → PR) and `report-kandev-issue` workflows with no create/edit/delete/
import/export/reorder controls; the repositories page lists the kandev repo
with no add/edit/delete controls. Other workspaces keep the current UI.

## Target

- `apps/web/app/settings/workspace/workspace-workflows-client.tsx` (+ its
  dialogs `workspace-workflows-dialogs.tsx` and any actions file).
- `apps/web/app/settings/workspace/workspace-repositories-client.tsx`.
- `apps/web/src/settings-routes.tsx` (route loaders pass workspace name).
- `apps/web/app/actions/workspaces.ts` — `listWorkflowsAction` gains an
  `include_hidden` option (the backend endpoint already supports
  `?include_hidden=true`).

## Change

1. Determine the improve workspace client-side by exact name
   `IMPROVE_KANDEV_WORKSPACE_NAME` (already exported from
   `components/improve-kandev-dialog-model.ts`). The route loaders already
   fetch the workspace (`getWorkspaceAction`) — thread `isImproveWorkspace`
   into the client components (route-loader → component props, mirroring how
   the workspace name is already passed).
2. Workflows page in improve mode:
   - Fetch workflows with `include_hidden=true` so the hidden improve
     workflows appear (verify the list response includes steps or fetch steps
     per workflow with the existing steps action so the card can show
     Improve → Test → PR).
   - Hide the Add/Import/Export/GitHub-sync actions and the reorder
     affordances; render cards non-interactive (no edit/delete).
   - Show a short read-only notice ("This workspace's workflows are managed by
     Improve Kandev and cannot be changed.").
3. Repositories page in improve mode: hide Add/Discover; render repository
   cards non-interactive (no edit/delete); same notice style.
4. Keep the generic task-create dialog behavior unchanged (the standard New
   Task flow in the improve workspace routes to the Improve Kandev dialog per
   the earlier change — this task only touches settings pages).

## Acceptance

- Unit tests for the read-only rendering (component tests in the settings
  workspace dir; follow existing test conventions for these pages — if none
  exist, add at least a render-mode test for the workflows page: improve
  workspace hides the Add control and lists the hidden workflow; normal
  workspace unchanged).
- `cd apps/web && pnpm typecheck && pnpm lint` and the settings-related
  vitest suites pass.

## Results

Shipped in `cc7b66550` (PR #2347). `WorkspaceWorkflowsClient`/`WorkspaceRepositoriesClient`
render read-only when `isImproveWorkspace` (`WorkflowCard readOnly`,
`RepositoryCard readOnly`), with a shared `WorkspaceSettingsHeader` and a badge
distinguishing "managed by Improve Kandev" from GitHub-sync managed; `include_hidden=true`
lists hidden workflows in settings. The merged upstream i18n pass replaced literals
with `t()` keys (`workflowsReadOnlyImprove`, `repositoriesReadOnlyImprove`, ...);
repository-card max-lines was fixed by extracting `RepositoryPreview` into
`components/settings/repository-card-preview.tsx`. Verified: `tsc --noEmit`,
`eslint --max-warnings 0`, `vitest run components/settings/repository-card.test.tsx`
(17 tests across the touched files).
