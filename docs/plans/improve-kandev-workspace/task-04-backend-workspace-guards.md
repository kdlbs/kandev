# task-04: Backend workspace guards (read-only Improve Kandev workspace)

Spec: `docs/specs/workspaces/requirements/improve-kandev.md` — "Dedicated workspace immutability"
Plan: `docs/plans/improve-kandev-workspace/plan.md` — Phase 2, Wave 4.

## Goal

The dedicated `Improve Kandev` workspace is configuration-immutable: workflow
mutations (create/update/delete/reorder and step mutations) and repository
mutations (create/update/delete/initialize-local) scoped to it are rejected
with HTTP 409 before any write. Listing/reading stays available.

## Target

- `apps/backend/internal/improvekandev/handler.go` — export the improve
  workspace name check used by guards.
- `apps/backend/internal/task/service/service_resources.go` — workflow CRUD +
  repository mutation guards.
- `apps/backend/internal/task/handlers/*` — wire the guard error to 409.
- `apps/backend/internal/workflow/service/readonly.go` +
  `apps/backend/internal/workflow/controller/controller.go` — step-mutation
  guard.
- `apps/backend/internal/backendapp` — wire a workspace-name provider into the
  workflow service (only if the workflow service cannot resolve the workspace
  itself).

## Change

1. In `internal/improvekandev`, add an exported predicate
   `IsImproveWorkspaceName(name string) bool` (exact match against
   `improveWorkspaceName`). Keep the unexported const private; export only the
   predicate + a doc comment.
2. Task service:
   - Add `ErrWorkspaceReadOnly = errors.New("workspace is managed by Improve
     Kandev and is read-only")` (mirror `workflow/service.ErrWorkflowReadOnly`
     wording style).
   - Add a private helper `ensureWorkspaceMutable(ctx, workspaceID)` that
     resolves the workspace (reusing the existing workspace resolver path used
     by `authorizeWorkspaceID`) and returns `ErrWorkspaceReadOnly` when
     `IsImproveWorkspaceName(workspace.Name)`.
   - Call it at the top of: `CreateWorkflow`, `UpdateWorkflow`, `DeleteWorkflow`
     (and any reorder method the service exposes), and the repository mutation
     methods (`CreateRepository`, `UpdateRepository`, `DeleteRepository`,
     `InitializeLocalRepository` — exact names per the actual service).
3. Handlers: map `ErrWorkspaceReadOnly` → HTTP 409 with a clear message
   (mirror how `rejectReadOnlyWorkflowHTTP` / `writeStepMutationError` map
   `ErrWorkflowReadOnly`). Find all workflow/repo handler mutation paths and
   ensure they surface the 409 (WS paths included if they mutate).
4. Workflow steps: extend `workflow/service.EnsureWorkflowMutable` so that,
   in addition to the GitHub-source check, it resolves the workflow's
   workspace and rejects when `IsImproveWorkspaceName(workspace.Name)`. The
   service needs a workspace lookup — add a minimal `workspaceProvider`
   interface (GetWorkspace) to the workflow service if it doesn't already have
   one, wire it in `backendapp`, keep the guard fail-open on resolution errors
   (same policy as today).
5. Do NOT guard: workspace rename/delete, task creation, kanban board, hidden
   workflow healing by the improve bootstrap itself (the bootstrap's
   `ensureWorkflow` upsert must keep working — it writes through the same
   service; ensure the guard doesn't break the bootstrap path. If
   `ensureWorkflow` uses taskSvc.UpdateWorkflow, either exclude the
   improvekandev caller or guard at the HTTP/controller layer only — decide
   and document in the code).

## Acceptance

- Unit tests in `internal/task/service` asserting create/update/delete workflow
  and repository mutations return `ErrWorkspaceReadOnly` for the improve
  workspace and succeed for a normal workspace.
- Unit tests in `internal/workflow` asserting step mutations reject for a
  workflow in the improve workspace (workflow's workspace resolved by name).
- Integration test: HTTP mutations against the improve workspace return 409;
  the same mutations on a normal workspace succeed. The bootstrap still works
  after the guards (existing integration tests stay green).
- `go test -tags fts5 ./internal/task/... ./internal/workflow/... ./internal/improvekandev/... ./internal/integration/...` and `golangci-lint run ./...` pass.

## Results

Shipped in `37597867d` + merge `68b8a57a4` (PR #2347). Guards at the HTTP/WS
handler layer reject mutations in the "Improve Kandev" workspace with HTTP 409
(`workspaceReadOnlyMsg`); workflow step mutations go through
`workflow/service.EnsureWorkflowMutable` (`ErrWorkflowWorkspaceReadOnly` → 409)
and task mutations via `task.Service.ErrWorkspaceReadOnly`. Identity is name-based
(`(*Workspace).IsImproveKandev()`), so the bootstrap's internal service calls keep
working. Verified by `TestImproveKandevBootstrap*` in
`internal/integration/improve_kandev_test.go` (synthetic-identity router) and by
the read-only settings e2e tests; full backend suite passes with
`KANDEV_*` vars unset and `umask 022`.
