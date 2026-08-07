---
spec: docs/specs/improve-kandev/spec.md
created: 2026-08-01
status: implemented
---

# Implementation Plan: Improve Kandev Workspace Isolation

## Overview

Improve Kandev tasks currently land in the user's active workspace, mixing
contribution work with their regular tasks. This change makes the
`improve-kandev` bootstrap endpoint find-or-create a dedicated, idempotently
reused workspace named `Improve Kandev`, scope the repository registration and
both hidden workflows to it, and return its `workspace_id` so the frontend
creates the task there. Backend contract first, then frontend wiring, then E2E
evidence of isolation.

---

## Backend

### `internal/improvekandev/handler.go`
- `BootstrapRequest.WorkspaceID` becomes optional (`json:"workspace_id,omitempty"`).
  Remove the `req.WorkspaceID == ""` 400 validation and the
  `canonicalWorkspaceID` call; delete `canonicalWorkspaceID` (and its unit
  test). The field is accepted and ignored for backward compatibility.
- Add constants `improveWorkspaceName = "Improve Kandev"` and
  `improveWorkspaceDesc`.
- Add `ensureImproveWorkspace(ctx) (*taskmodels.Workspace, error)`, mirroring
  the existing `ensureWorkflow` idempotency pattern:
  1. `taskSvc.ListWorkspaces(ctx)` → match by exact `Name == "Improve Kandev"`.
  2. Miss → `taskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{
     Name: improveWorkspaceName, Description: improveWorkspaceDesc,
     BootstrapKanbanWorkflow: true })`.
  3. Create failure → re-list and match by name (concurrent-bootstrap race);
     still missing → return the error.
- `httpBootstrap`:
  - Call `ensureImproveWorkspace` first; use its ID everywhere the old
    `workspaceID` was used (`resolveOrCloneRepo`, `ListWorkflows`,
    `ensureWorkflow` ×2).
  - Add `WorkspaceID string \`json:"workspace_id"\`` to `BootstrapResponse`,
    set to the dedicated workspace's ID.

### Tests
- `internal/improvekandev/handler_test.go`: remove `TestCanonicalWorkspaceID`
  (function deleted).
- `internal/integration/improve_kandev_test.go`:
  - Update `TestImproveKandevBootstrapCreatesBothHiddenWorkflowsIdempotently`:
    drop the pre-created workspace; bootstrap with an empty
    `WorkspaceID`; assert the response `WorkspaceID` is non-empty, a workspace
    named `Improve Kandev` exists, both hidden workflows live in it, and a
    second bootstrap returns the same workspace + workflow IDs.
  - Add `TestImproveKandevBootstrapReusesExistingImproveWorkspace`: pre-create
    a workspace named `Improve Kandev`, bootstrap with a *different* request
    `workspace_id`, assert the response's workspace is the pre-created one.

---

## Frontend

### `lib/api/domains/improve-kandev-api.ts`
- Add `workspace_id: string` to `ImproveKandevBootstrapResponse`. Keep the
  request `workspaceId` parameter (still sent; backend ignores it).

### `components/improve-kandev-dialog.tsx`
- `useBootstrapKandev`: after bootstrap, call `listRepositories` and
  `setRepositories` with `data.workspace_id` instead of the active
  `workspaceId` so the locked-repo chip resolves a label for the dedicated
  workspace's repo.

### `components/improve-kandev-dialog-create.tsx`
- `CreateModeView`: pass `workspaceId={ready ? ready.data.workspace_id :
  props.workspaceId}` to `TaskCreateDialog`. Submit stays blocked until
  bootstrap is ready, so no task can be created in the wrong workspace.
  The sidebar `workspaceId` prop remains the fallback while loading and is
  still used by `useGitHubAuthCheck` for the fix URL.

---

## Tests

- **What:** bootstrap creates the dedicated workspace on first use
  (spec scenario). **File:** `internal/integration/improve_kandev_test.go`.
  **How:** integration test through the real handler + task service.
- **What:** bootstrap reuses the existing workspace and workflow IDs
  (spec scenario). **File:** same. **How:** second bootstrap call, compare IDs.
- **What:** a different request `workspace_id` is ignored.
  **File:** same. **How:** pre-create the dedicated workspace, pass another ID.
- **What:** frontend uses the bootstrap's `workspace_id` for repo listing and
  task creation. **File:** `apps/web/components/*` + E2E. **How:** E2E test
  seeds a real dedicated workspace and asserts the task lands there.
- **What:** E2E mocks stay contract-complete (`workspace_id` present).
  **File:** `apps/web/e2e/tests/improve-kandev.spec.ts`,
  `mobile-improve-kandev.spec.ts`. **How:** update mock bodies.

---

## E2E Tests

- **Scenario:** GIVEN the user's active workspace is not the dedicated
  workspace, WHEN the dialog submits a task, THEN the task appears in the
  dedicated workspace and not the active one.
  **File:** `apps/web/e2e/tests/improve-kandev.spec.ts`.
  **What to verify:** `apiClient.listTasks(dedicatedWorkspaceId)` contains the
  task; `apiClient.listTasks(seedData.workspaceId)` does not.
  Seed the dedicated workspace with `apiClient.createWorkspace("Improve
  Kandev")`, a workflow via `createWorkflow(workspace.id, ...)`, and a
  repository via `createRepository(workspace.id, <seed repo dir>)`; mock
  bootstrap to return those IDs.

---

## Verification Results

Completed for all 7 tasks (see each task's `## Results`). Summary of the final
verification run on the merged branch (`68b8a57a4`, PR #2347):

- Backend: `go test -tags fts5 ./internal/improvekandev/... ./internal/integration/...`
  pass; full backend suite passes with `KANDEV_*` vars unset and `umask 022`.
- Frontend: `tsc --noEmit` and `eslint --max-warnings 0` clean on all touched
  files; unit tests pass (improve-kandev dialog, repository-card).
- E2E: `playwright test --config e2e/playwright.config.ts improve-kandev.spec` →
  14 passed (13 desktop + 1 mobile). Required rebuilding the stale
  `apps/backend/bin/kandev` e2e binary (missing upstream `agent_generated_task_titles`)
  and seeding `agent_generated_task_titles: false` in the submit-flow tests.
- CI on PR #2347: all checks green (`mergeStateStatus: CLEAN`); CodeRabbit
  threads resolved.
- Demo instance on ubu4.in.fiere.fr (backend :25208, vite :58449) rebuilt with
  the merged code: bootstrap with `create_workspace:true` created workspace
  `0dfe5db8-c82d-4c6e-803a-2eec41bcb3fc` and carried the Default workspace's
  mock GitHub connection; 409s and read-only settings pages verified.

Artifacts: `docs/specs/improve-kandev/spec.md` (spec amendment for Phase 2),
this plan, and `docs/plans/improve-kandev-workspace/task-*.md`.
Cleanup/teardown: no temporary artifacts shipped; e2e DIAG probes removed before
committing.

---

## Implementation Waves And Parallel Candidates

Wave 1 (sequential):
- [x] [task-01-backend-dedicated-workspace](task-01-backend-dedicated-workspace.md)

Wave 2 (depends on 01):
- [x] [task-02-frontend-bootstrap-workspace](task-02-frontend-bootstrap-workspace.md)

Wave 3 (depends on 02):
- [x] [task-03-e2e-workspace-isolation](task-03-e2e-workspace-isolation.md)

## Phase 2 — Dedicated workspace immutability + GitHub carry-over

Follow-up requirements: the dedicated workspace is configuration-immutable
(workflows and repositories read-only) and, on first creation only, inherits
the GitHub workspace connection from the user's default workspace — nothing
else (no other integrations, no automations).

### Waves

Wave 4 (backend guards — sequential):
- [x] [task-04-backend-workspace-guards](task-04-backend-workspace-guards.md)

Wave 5 (depends on 04):
- [x] [task-05-backend-github-carryover](task-05-backend-github-carryover.md)

Wave 6 (depends on 04, 05):
- [x] [task-06-frontend-settings-readonly](task-06-frontend-settings-readonly.md)

Wave 7 (depends on 06):
- [x] [task-07-e2e-settings-guards](task-07-e2e-settings-guards.md)

The default is sequential execution in the primary conversation. No subagents
unless the user explicitly asks after selecting the implementation model.

---

## Open Questions

None.
