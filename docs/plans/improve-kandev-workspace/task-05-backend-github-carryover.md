# task-05: Backend GitHub carry-over on Improve Kandev workspace creation

Spec: `docs/specs/improve-kandev/spec.md` — "Workspace creation semantics"
Plan: `docs/plans/improve-kandev-workspace/plan.md` — Phase 2, Wave 5.
Depends on: task-04.

## Goal

When bootstrap creates the `Improve Kandev` workspace for the **first time**
only, copy the GitHub workspace connection (and its PAT secret where
applicable) from the user's default workspace. Copy nothing else: no other
integration configs, no automations, no extra workflows/repos.

## Target

- `apps/backend/internal/github/service_connections.go` (or a new
  `copy_connection.go`) — new `CopyWorkspaceConnectionToWorkspace`.
- `apps/backend/internal/github/store.go` — any needed read/write helper.
- `apps/backend/internal/improvekandev/handler.go` — creation-path hook.
- `apps/backend/internal/backendapp` — wire the copier + default-workspace
  resolver into the improvekandev handler.

## Change

1. GitHub service: add
   `CopyWorkspaceConnectionToWorkspace(ctx, srcWorkspaceID, dstWorkspaceID) error`:
   - Read the source `github_workspace_connections` row; if none, return nil
     (no-op).
   - For `pat` sources: also copy the PAT secret (stored under the
     workspace-scoped secret key `WorkspacePATSecretKey(src)` → write under
     `WorkspacePATSecretKey(dst)`); increment/recompute nothing — the copied
     row keeps its `credential_generation`/status as-is.
   - For `gh_cli` sources: the CLI account is host-global; copy the row
     (source/login/host) without a secret.
   - For app-installation sources: copy the row (installation id/account/app
     registration reference).
   - Upsert into `github_workspace_connections` for the destination
     (ON CONFLICT(workspace_id) — same as `SetWorkspaceConnection`).
   - Use the store's transaction surface if one exists; keep it best-effort:
     a copy failure must not fail bootstrap (log + continue) — decide and
     document.
2. Default workspace resolution: reuse
   `integrations/workspacescope.DefaultResolver.Resolve(ctx)` (active
   workspace in user settings → first by created_at → literal `default`).
   Wire it in `backendapp` where the improvekandev handler is constructed.
3. Improve handler: add a small interface to the handler (mirroring the
   `Cloner` pattern), e.g.
   `GitHubWorkspaceCopier interface { CopyWorkspaceConnectionToWorkspace(ctx, src, dst string) error }`,
   and a `DefaultWorkspaceResolver func(ctx) (string, error)` (or interface).
   In `ensureImproveWorkspace`, after a successful **create** (miss path
   only), resolve the default workspace and copy the GitHub connection into
   the new workspace. The reuse path must NOT copy.
4. `NewHandler` signature grows by the copier + resolver; update
   `cmd/kandev` wiring and the test constructors (`handler_test.go`,
   `internal/integration/improve_kandev_test.go`) with fakes.

## Acceptance

- Unit test (`internal/github`): copying a `pat` connection duplicates the row
  and the secret under the destination key; `gh_cli`/app rows copy without a
  secret; no source row → nil.
- Integration test: bootstrap on a fresh DB where the default workspace has a
  GitHub connection (seed via the github service or store) → the new Improve
  Kandev workspace has the same connection; a second bootstrap does not change
  it. Bootstrap with no source connection → no connection row.
- Existing improve-kandev integration tests stay green (constructors updated
  with fakes).
- `go test -tags fts5 ./internal/github/... ./internal/improvekandev/... ./internal/integration/...` and `golangci-lint run ./...` pass.

## Results

Shipped in `37597867d` (PR #2347). `github.Service.CopyWorkspaceConnectionToWorkspace`
copies the default workspace's GitHub connection row + PAT secret at workspace
creation only; `gh_cli`/app rows without secrets are copied as rows.
Wired via `improvekandev.GitHubWorkspaceCopier` / `DefaultWorkspaceResolver`
(uses `workspacescope.ResolveMigrationTarget`); `dbPool` threaded through
`buildHTTPServer` + `routeParams`. Best-effort — failures are logged and never
fail bootstrap. Verified by integration tests and on the ubu4 demo instance
(workspace `0dfe5db8-...` received the Default workspace's mock GitHub
connection; re-seed needed after restart since mock connections are in-memory:
`PUT /api/v1/github/mock/workspace-connections/<ws-id>`).
