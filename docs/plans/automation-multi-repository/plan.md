---
spec: docs/specs/office/requirements/automations-settings.md
created: 2026-07-30
status: draft
---

# Implementation Plan: Multi-repository automation configuration

## Overview

Replace the automation's singular `repository_id` with an ordered `repository_ids`
list, backed by a new `automation_repositories` join table (mirrors
`task_repositories`). The orchestrator resolves N repositories per trigger
firing the same way `resolveExplicitRepository` resolves one today. The editor
gates multi-select on the selected executor profile's capability by reusing
`getMultiRepoExecutorDisabledReason` from the task-creation dialog. Order:
backend data model + contract first (everything else depends on the wire
shape), then orchestrator resolution and frontend UI in parallel (disjoint
files, both consume the same contract), then E2E last.

---

## Backend

### Schema (`apps/backend/internal/automation/store.go`)

- Add to `createTablesSQL`:
  ```sql
  CREATE TABLE IF NOT EXISTS automation_repositories (
      id TEXT PRIMARY KEY,
      automation_id TEXT NOT NULL REFERENCES automations(id) ON DELETE CASCADE,
      repository_id TEXT NOT NULL,
      position INTEGER NOT NULL DEFAULT 0,
      created_at DATETIME NOT NULL,
      UNIQUE(automation_id, repository_id)
  );
  CREATE INDEX IF NOT EXISTS idx_automation_repositories_automation ON automation_repositories(automation_id);
  ```
- Drop `repository_id` from the canonical `automations` CREATE TABLE.
- Replace the `migrateRepositoryIDSQL` ADD-COLUMN migration with a one-time
  recreate-and-backfill migration, following the exact pattern of
  `migrateTaskRepositoriesAllowMultiBranch` in
  `apps/backend/internal/task/repository/sqlite/base_migrations.go` (uses
  `r.migrate.Apply`/`recreateTableNamed`-equivalent helpers — automation's
  `Store` doesn't have that helper today; add a small
  `recreateAutomationsTableDroppingRepositoryID` following the same
  create-new/copy/drop/rename/reindex shape used there). Order of operations
  inside `initSchema`:
  1. Run `createTablesSQL` (creates `automation_repositories` for fresh installs).
  2. If the `automations` table still has a `repository_id` column (detect via
     `PRAGMA table_info(automations)`), backfill: for each automation row with
     non-empty `repository_id`, `INSERT OR IGNORE INTO automation_repositories
     (id, automation_id, repository_id, position, created_at) VALUES
     (<uuid>, id, repository_id, 0, created_at)`.
  3. Recreate `automations` without the `repository_id` column (new table,
     copy columns, drop old, rename, recreate indexes) — same shape as the
     task_repositories migration.
- This migration is idempotent: step 2 is skipped once the column check in
  step 3 no longer finds `repository_id` on `automations`.

### Model (`apps/backend/internal/automation/models.go`)

- `Automation`: remove `RepositoryID string db:"repository_id"`; add
  `RepositoryIDs []string json:"repository_ids" db:"-"` (hydrated separately,
  like `Triggers`).
- `CreateAutomationRequest.RepositoryID string` → `RepositoryIDs []string
  json:"repository_ids"`.
- `UpdateAutomationRequest.RepositoryID *string` → `RepositoryIDs []string
  json:"repository_ids,omitempty"`. Use the nil-vs-non-nil-slice convention
  already used by `dto.TaskRepositoryInput`/`task_ws_handlers.go` (`req.Repositories
  != nil` means "provided"): nil = untouched, `[]` = explicit clear, non-empty
  = replace.

### Store (`apps/backend/internal/automation/store.go`)

- `CreateAutomation`: after inserting the `automations` row (column list drops
  `repository_id`), insert one `automation_repositories` row per ID in
  `a.RepositoryIDs` (position = index) inside the same transaction.
- New `hydrateRepositoryIDs` batch helper mirroring
  `listTriggersForAutomations`: `listRepositoryIDsForAutomations(ctx,
  automationIDs) (map[string][]string, error)` — `SELECT automation_id,
  repository_id FROM automation_repositories WHERE automation_id IN (?) ORDER
  BY automation_id, position`. Wire into `GetAutomation`, `ListAutomations`,
  `ListAllEnabled` the same way `Triggers` is hydrated today.
- `UpdateAutomation`: when `req.RepositoryIDs != nil`, replace the automation's
  `automation_repositories` rows transactionally: `DELETE FROM
  automation_repositories WHERE automation_id = ?` then re-insert the new
  ordered list, inside the same `Tx` as the `UPDATE automations` statement (do
  not rely solely on `ON DELETE CASCADE`, which only fires on automation
  deletion, not on a repository-list replace for a still-live automation).
- `applyAutomationUpdate`: drop the `RepositoryID` branch (repositories are
  now handled directly by the store method above, not via the in-memory
  struct mutation, since they're a child table rather than a column).

### Service (`apps/backend/internal/automation/service.go`)

- `CreateAutomation`: before building `*Automation`, validate
  `req.RepositoryIDs` — reject duplicates, and reject any ID that doesn't
  resolve to a repository belonging to `req.WorkspaceID`. Needs a
  `repoStore`-shaped dependency capable of `GetRepository`/listing by
  workspace; check what's already injected into `Service` (`s.store` is the
  automation store only — repository lookups may need a new small interface
  injected via a setter, following the `SetTaskDeleter`/`SetWorkspaceAuthorizer`
  pattern). Confirm during implementation whether the automation package
  already has access to a repository lookup (it currently does not import the
  task/repository package) and add the minimal interface + setter needed;
  wire it from `apps/backend/cmd/.../main.go` (or wherever `automation.NewService`
  is constructed) the same way `SetTaskDeleter` is wired.
- `UpdateAutomation`: same validation before calling `s.store.UpdateAutomation`
  whenever `req.RepositoryIDs != nil`.
- Return a typed/sentinel error (e.g. `ErrRepositoryNotInWorkspace`) so the WS
  handler can map it to `ws.ErrorCodeValidation` instead of
  `ErrorCodeInternalError`.

### Orchestrator (`apps/backend/internal/orchestrator/event_handlers_automation.go`)

- `resolveAutomationRepository`: replace `if a.RepositoryID != ""` with `if
  len(a.RepositoryIDs) > 0`, calling a new `resolveExplicitRepositories`.
- Add `resolveExplicitRepositories(ctx, repositoryIDs []string)
  []ReviewTaskRepository`: loop over `repositoryIDs`, reuse the existing
  per-ID resolution logic from `resolveExplicitRepository` (load repo, default
  branch fallback to `automationDefaultBaseBranch`), append one
  `ReviewTaskRepository` per ID, skip (with a warning log, not a hard failure)
  any ID that fails to load so one bad ID doesn't sink the whole firing. Keep
  `resolveExplicitRepository` only if still referenced elsewhere after the
  change; otherwise delete it (check via `lsp references` before removing).
- No other changes needed in `createAutomationTask` — it already accepts
  `[]ReviewTaskRepository` and passes the full slice to
  `CreateReviewTask.Repositories`.

---

## Frontend

### Types (`apps/web/lib/types/automation.ts`)

- `Automation.repository_id: string` → `repository_ids: string[]`.
- `CreateAutomationRequest`/`UpdateAutomationRequest` (the `repository_id?:
  string` fields around lines 125/144): → `repository_ids?: string[]`.

### `apps/web/components/task-create-dialog-multi-repo-guard.ts`

- No changes — import `getMultiRepoExecutorDisabledReason` directly from this
  existing module (it's a pure function with zero task-creation-dialog
  dependencies). Do not duplicate the executor-type set.

### `apps/web/components/automations/config-section.tsx`

- `RepositorySelection` retains its `{kind:"none"}` variant — still needed by
  the single-picker fallback branch below — but the repeatable multi-row
  picker never produces one (each row is always fully resolved the moment
  it's added).
- `ConfigSectionProps`: `repositorySelection: RepositorySelection` →
  `repositorySelections: RepositorySelection[]`; `onRepositoryChange` →
  `onRepositoriesChange: (selections: RepositorySelection[]) => void`.
  `dirtyFields.repositorySelection` → `dirtyFields.repositorySelections`.
- Compute `selectedExecutorProfile = allExecutorProfiles.find(p => p.id ===
  executorProfileId)` and `multiRepoDisabledReason =
  getMultiRepoExecutorDisabledReason(selectedExecutorProfile?.executor_type)`.
- Executor Profile `SelectField`: extend the local `SelectField` component's
  `items` shape to `Array<{id, label, disabled?, disabledReason?}>` (small,
  additive change — render `<SelectItem disabled={item.disabled}
  title={item.disabledReason}>`). Build executor items with `disabled:
  repositorySelections.length > 1 &&
  getMultiRepoExecutorDisabledReason(p.executor_type) !== null`.
- Repository picker: branch on `multiRepoDisabledReason === null &&
  conditionType !== "github_pr"`:
  - compatible (`null` reason, non-PR trigger): render a new
    `AutomationRepositoryRows` component (new file
    `apps/web/components/automations/automation-repository-rows.tsx`) that
    renders one `SelectField` per entry in `repositorySelections` (options
    built via `buildRepositoryItems`, marking repos already chosen in
    another row analogous to the task-creation dialog's "Already added"
    marker) plus a trailing "Add repository" button. Each click appends a
    row pre-filled with the next not-yet-selected repository — `onChange`/
    `addRow` bail out on `kind === "none"` (an unreachable defensive guard
    here, since `pickSelectionFromOptionId` only returns it for the
    `"__none__"` sentinel or a stale ID, neither offered by this picker), so
    every row is always fully resolved. The button disables once every
    known repo is already selected. Removing the last remaining row is
    allowed (falls back to an empty list, which the orchestrator resolves
    to the workspace's first repository — stated in helper text beside the
    control).
  - incompatible (single-repo executor, or a `github_pr` trigger): render
    the existing single `SelectField` bound to `repositorySelections[0] ??
    {kind:"none"}`, `onChange` replaces the whole array with `[]` or
    `[selection]`. For `github_pr` specifically, additionally disable the
    dropdown and show "PR triggers always use the PR's own repository." —
    the backend's `resolveAutomationRepository` (orchestrator) checks the
    trigger type first and, for `github_pr`, resolves the repository from
    the PR's own trigger data unconditionally, never reading
    `RepositoryIDs` — so a stale multi-repo selection left over from before
    the trigger was switched to `github_pr` has no effect. Covered by
    `TestResolveAutomationRepository_GitHubPRIgnoresConfiguredRepositoryIDs`.
- `pickSelectionFromOptionId`/`buildRepositoryItems`/`selectionToOptionId`:
  unchanged signatures; `RepositorySelection` keeps its `{kind:"none"}`
  variant for the single-picker branch above.

### `apps/web/components/automations/automation-payload.ts`

- `FormState.repositorySelection: RepositorySelection` →
  `repositorySelections: RepositorySelection[]`.
- `resolveRepositoryId` → `resolveRepositoryIds(workspaceId, selections:
  RepositorySelection[]): Promise<string[]>` — map each selection through the
  existing per-selection resolution logic (register discovered repos,
  pass through registered ids) via `Promise.all`.
- `buildCreatePayload`/`buildUpdatePayload`: `repository_id: repositoryId` →
  `repository_ids: repositoryIds`.

### `apps/web/components/automations/automation-editor.tsx`

- `defaultForm.repositorySelection` → `defaultForm.repositorySelections: []`.
- `formFromAutomation`: `a.repository_id ? {kind:"registered",id:...} :
  {kind:"none"}` → `a.repository_ids.map(id => ({kind:"registered", id}))`.
- `useSaveHandler`: `resolveRepositoryId` → `resolveRepositoryIds`; the
  discovered→registered promotion logic (currently checks
  `form.repositorySelection.kind === "discovered"`) needs to promote each
  discovered row independently — map `form.repositorySelections` against the
  resolved `repositoryIds` array by index, promoting only the discovered ones.

### `apps/web/components/automations/automation-editor-sections.tsx`

- `repositorySelection` prop/dirty-field wiring (lines ~143-205) →
  `repositorySelections`.

### Tests

- `apps/web/components/automations/config-section.test.tsx`: update existing
  prop name; add cases per the Tests section below.
- `apps/web/components/automations/automation-trigger-drafts.test.ts` /
  other automation tests referencing `repositorySelection`: grep after the
  rename lands to confirm none were missed (`lsp references` on the removed
  export is not applicable across TS/Go, so use
  `grep -rnP '\brepositorySelection\b' apps/web/components/automations` as
  the completeness check).

---

## Tests

- **What:** `automation_repositories` schema exists and CRUD round-trips an
  ordered multi-repo list.
  **File:** `apps/backend/internal/automation/store_test.go`
  **How:** `TestCreateAutomation_PersistsRepositoryIDsInOrder`,
  `TestUpdateAutomation_ReplacesRepositoryIDs`,
  `TestGetAutomation_HydratesRepositoryIDs`,
  `TestListAutomations_BatchHydratesRepositoryIDs`.

- **What:** legacy `repository_id` backfills into `automation_repositories`
  and the column is dropped, exactly once, idempotently.
  **File:** `apps/backend/internal/automation/store_test.go`
  **How:** `TestInitSchema_BackfillsLegacyRepositoryID` — seed a raw SQLite DB
  with the pre-migration schema + a row carrying `repository_id`, run
  `NewStore`, assert `automation_repositories` has the backfilled row and
  `PRAGMA table_info(automations)` no longer lists `repository_id`. Run
  `initSchema` twice against the same DB to prove idempotency.

- **What:** `CreateAutomation`/`UpdateAutomation` reject repository IDs
  outside the automation's workspace, and reject duplicates.
  **File:** `apps/backend/internal/automation/service_test.go`
  **How:** `TestCreateAutomation_RejectsForeignRepositoryID`,
  `TestCreateAutomation_RejectsDuplicateRepositoryID`,
  `TestUpdateAutomation_RejectsForeignRepositoryID`.

- **What:** orchestrator resolves N repositories for scheduled/webhook
  triggers, skips a single bad ID without failing the whole firing, and falls
  back to the workspace repository when the list is empty.
  **File:** `apps/backend/internal/orchestrator/event_handlers_automation_test.go`
  (create if it doesn't exist; check first)
  **How:** `TestResolveAutomationRepository_MultipleExplicitRepositories`,
  `TestResolveAutomationRepository_SkipsUnloadableID`,
  `TestResolveAutomationRepository_EmptyListFallsBackToWorkspace`.

- **What:** `getMultiRepoExecutorDisabledReason` reuse is correct for the
  automation editor's executor-picker gate (the function itself is already
  covered by `task-create-dialog-multi-repo-guard.test.ts` — no new unit test
  for the predicate itself, just for its wiring below).
  **File:** `apps/web/components/automations/config-section.test.tsx`
  **How:**
  - `renders a repeatable repository list when the executor profile supports multi-repo`
  - `renders a single dropdown when the executor profile does not support multi-repo`
  - `disables incompatible executor profiles once two or more repositories are selected`
  - `keeps a compatible executor profile enabled with two or more repositories selected`

- **What:** save flow resolves and promotes multiple discovered repositories
  independently.
  **File:** `apps/web/components/automations/automation-payload.test.ts`
  (create if it doesn't exist; check first — no existing file was found for
  this module)
  **How:** `resolveRepositoryIds resolves a mix of registered and discovered selections`,
  `buildUpdatePayload sends repository_ids in row order`.

---

## E2E Tests

- **Scenario:** GIVEN the automation editor with a `worktree` executor
  profile selected, WHEN the user adds a second repository row and saves,
  THEN the saved automation shows two repository chips/rows on reload.
  **File:** `apps/web/e2e/automations/multi-repository.spec.ts`
  **What to verify:** "Add repository" button visible and enabled; second row
  appears; after save + reload, both repositories are pre-selected.

- **Scenario:** GIVEN the automation editor with a `local` executor profile
  selected, WHEN the user opens the repository picker, THEN it is a single
  dropdown with no "Add repository" control.
  **File:** `apps/web/e2e/automations/multi-repository.spec.ts`
  **What to verify:** `[data-testid="repository-selector"]` renders as one
  select; no add-repository button in the DOM.

- **Scenario:** GIVEN two repositories already selected, WHEN the user opens
  the Executor Profile dropdown, THEN incompatible executor profiles are
  visibly disabled.
  **File:** `apps/web/e2e/automations/multi-repository.spec.ts`
  **What to verify:** the incompatible option has `aria-disabled`/`disabled`
  and is not selectable via click.

---

## Implementation Waves And Parallel Candidates

```
Wave 1:
- [ ] [task-01-backend-data-model](task-01-backend-data-model.md)

Wave 2 (parallel candidates — user authorization required):
- [ ] [task-02-orchestrator-resolution](task-02-orchestrator-resolution.md)
- [ ] [task-03-frontend-multi-repo-picker](task-03-frontend-multi-repo-picker.md)

Wave 3:
- [ ] [task-04-e2e](task-04-e2e.md)
```

---

## Open Questions

- Where exactly `automation.NewService` is constructed and whether a
  repository-lookup dependency is already reachable there, or needs new
  plumbing — confirm during task-01 implementation (flagged in that task's
  Inputs).
