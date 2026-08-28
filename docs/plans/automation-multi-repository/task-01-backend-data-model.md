---
id: "01-backend-data-model"
title: "Backend data model: automation_repositories join table"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/office/requirements/automations-settings.md"
---

# Task 01: Backend data model — `automation_repositories`

## Inputs

- `docs/specs/office/requirements/automations-settings.md` — Data model, API surface,
  Failure modes sections (multi-repository additions).
- `docs/plans/automation-multi-repository/plan.md` — Backend section.
- `apps/backend/internal/automation/models.go` — `Automation`,
  `CreateAutomationRequest`, `UpdateAutomationRequest`.
- `apps/backend/internal/automation/store.go` — schema, CRUD, migration
  pattern (`initSchema`, `migrateRepositoryIDSQL`).
- `apps/backend/internal/automation/service.go` — `CreateAutomation`,
  `UpdateAutomation`.
- `apps/backend/internal/task/repository/sqlite/base_migrations.go` —
  `migrateTaskRepositoriesAllowMultiBranch` as the recreate-table migration
  pattern to mirror for dropping `automations.repository_id`.
- **Open question to resolve first:** find where `automation.NewService` is
  constructed (likely `apps/backend/cmd/...` or an app-wiring file) and
  determine whether a repository-lookup dependency (something satisfying
  `GetRepository(ctx, id) (*models.Repository, error)` scoped by workspace) is
  already reachable there. If not, add a small interface + `SetXxx` setter on
  `Service` following the existing `SetTaskDeleter`/`SetWorkspaceAuthorizer`
  pattern, and wire it at construction time.

## Change

1. **Schema** (`store.go`):
   - Add `automation_repositories` table + index to `createTablesSQL`.
   - Remove `repository_id` from the canonical `automations` CREATE TABLE.
   - Replace `migrateRepositoryIDSQL` with a migration that: (a) creates
     `automation_repositories` (already covered by `createTablesSQL` running
     first), (b) if `automations.repository_id` still exists (check via
     `PRAGMA table_info(automations)`), backfills every non-empty
     `repository_id` into `automation_repositories` at position 0, then (c)
     recreates the `automations` table without that column.
   - **FK hazard:** `automation_triggers.automation_id` and
     `automation_runs.automation_id` both `REFERENCES automations(id) ON
     DELETE CASCADE`, and this DB opens with `_foreign_keys=on` (see
     `apps/backend/internal/db/sqlite.go`). Recreating `automations`
     (rename-out/create-new/copy/drop-old) with FK enforcement on will fail
     or orphan those two children's constraints. Mirror
     `apps/backend/internal/task/repository/sqlite/base_migrations.go`'s
     `recreateTable` helper exactly: wrap the whole rename/create/copy/drop
     sequence in `PRAGMA foreign_keys=OFF` before the transaction and
     `defer` a `PRAGMA foreign_keys=ON` after — that helper already proves
     the pattern works for a table with FK children (`migrateTasksRemoveWorkflowFK`
     recreates `tasks`, which `task_sessions`/`task_repositories`/etc.
     reference the same way). The `automation` package doesn't have this
     helper yet; port a minimal equivalent rather than reaching across
     packages.
   - Must be idempotent — safe to run `initSchema` repeatedly against an
     already-migrated DB.
2. **Model** (`models.go`):
   - `Automation.RepositoryID string` → `RepositoryIDs []string
     json:"repository_ids" db:"-"`.
   - `CreateAutomationRequest.RepositoryID string` → `RepositoryIDs []string
     json:"repository_ids"`.
   - `UpdateAutomationRequest.RepositoryID *string` → `RepositoryIDs []string
     json:"repository_ids,omitempty"` (nil = untouched, non-nil = replace,
     matching the `dto.TaskRepositoryInput` convention).
3. **Store CRUD** (`store.go`):
   - `CreateAutomation`: insert the `automations` row (no `repository_id`
     column), then insert one `automation_repositories` row per entry in
     `a.RepositoryIDs` (position = index), in the same transaction.
   - Add `listRepositoryIDsForAutomations(ctx, automationIDs []string)
     (map[string][]string, error)` mirroring
     `listTriggersForAutomations`; wire into `GetAutomation`,
     `ListAutomations`, `ListAllEnabled` to hydrate `RepositoryIDs`.
   - `UpdateAutomation`: when `req.RepositoryIDs != nil`, inside the same
     transaction as the `automations` UPDATE, `DELETE FROM
     automation_repositories WHERE automation_id = ?` then re-insert the new
     ordered list. Do not rely on `ON DELETE CASCADE` for this path (it only
     fires on automation deletion).
   - `applyAutomationUpdate`: remove the `RepositoryID` branch.
4. **Service validation** (`service.go`):
   - `CreateAutomation` and `UpdateAutomation` (when `RepositoryIDs != nil`):
     reject duplicate IDs in the request, and reject any ID that isn't a
     repository belonging to the automation's `WorkspaceID`. Return a sentinel
     error (e.g. `ErrRepositoryNotInWorkspace`) distinguishable from generic
     errors so callers can map it to a validation (not internal) error code.
5. **WS handler error mapping** (`handlers.go`): map the new sentinel error
   from `wsCreate`/`wsUpdate` to `ws.ErrorCodeValidation` instead of falling
   through to `ws.ErrorCodeInternalError`.

## Acceptance

- `Automation`, `CreateAutomationRequest`, `UpdateAutomationRequest` expose
  `RepositoryIDs []string` (JSON `repository_ids`); no remaining
  `RepositoryID`/`repository_id` singular field in `internal/automation`.
- Creating/updating/listing/getting an automation round-trips an ordered
  multi-repository list correctly, including through the batch-hydration path
  used by `ListAutomations`/`ListAllEnabled`.
- A pre-existing DB with a non-empty legacy `repository_id` column value ends
  up with exactly one matching `automation_repositories` row after
  `NewStore`, and the `automations` table no longer has a `repository_id`
  column. Running `NewStore` again against the same (already migrated) DB
  file does not error and does not duplicate rows.
- `CreateAutomation`/`UpdateAutomation` reject a `repository_ids` entry that
  belongs to a different workspace, and reject a duplicate ID within one
  request, with a validation-class error (not an internal-error class one).

## Verification

```
make -C apps/backend test
```

## Files likely touched

- `apps/backend/internal/automation/models.go`
- `apps/backend/internal/automation/store.go`
- `apps/backend/internal/automation/store_test.go`
- `apps/backend/internal/automation/service.go`
- `apps/backend/internal/automation/service_test.go`
- `apps/backend/internal/automation/handlers.go`
- Whatever file constructs `automation.NewService` (locate first)

## Dependencies

None.

## Parallelism

`sequential` (wave 1) — tasks 02 and 03 depend on this contract.

## Output contract

Summary of changes, exact test command output, any new sentinel error names
and where the repository-lookup dependency was wired, plus a note updating
`plan.md`'s Wave 1 checkbox and this file's `status` to `done`.
