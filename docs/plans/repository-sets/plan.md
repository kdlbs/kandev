---
spec: docs/specs/workspaces/requirements/repository-sets.md
created: 2026-08-17
status: completed
---

# Implementation Plan: Repository Sets

## Overview

Add a workspace-scoped `repository_sets` entity with an ordered membership table, expose it over the
existing repository REST/WebSocket surface and boot payload, then teach the shared task-creation
repository picker to apply a set as rows and to save the current selection as a new set. Manage sets
from the existing workspace **Repositories** settings tab rather than a new tab.

The feature is additive: with no sets defined, every existing surface renders and behaves exactly as
today. That is why it ships without a runtime feature flag. The registry in
`apps/backend/internal/runtimeflags/registry.go` holds five entries, all guarding genuinely risky or
mode-changing subsystems (office, auth, two Claude turn behaviors, dev mode); a new picker control
that is invisible until a user creates a set does not meet that bar, and adding a flag would mean a
retired identity to carry forever.

Two deliberate model choices, both from the spec:

- **Sets carry no branch.** The user's ask is explicitly that branches stay a per-task decision.
  Storing a branch would duplicate `task_repositories.base_branch` and go stale against the
  repository's real refs.
- **A real table with FK memberships, not a JSON column.** `office_projects.repositories` stores a
  JSON `[]string` of config-style names with no validation that the entries exist
  (`apps/backend/internal/office/projects/service.go:148`). Sets must resolve to selectable
  `repositories.id` values, so membership is a table with a foreign key and a uniqueness contract.

---

## Backend

### Persistence and models

- Add `repository_sets` and `repository_set_items` DDL to
  `apps/backend/internal/task/repository/sqlite/base_schema.go`, as a new `initRepositorySetsSchema`
  step appended to the `steps` slice in `initSchema` (`base_schema.go:16-48`) after the core schema,
  so `repositories` and `workspaces` exist. Model the tables on `task_repositories`
  (`base_schema.go:354-367`): `ON DELETE CASCADE` on both foreign keys,
  `UNIQUE(workspace_id, name)` on sets and `UNIQUE(repository_set_id, repository_id)` on items, plus
  a unique index on `(workspace_id, LOWER(name))` so the case-insensitive name rule the service
  enforces has a database backstop as well.
- Add the same tables idempotently in `runMigrations()`
  (`apps/backend/internal/task/repository/sqlite/base_migrations.go:124`) via
  `MigrateLogger.Apply("repository_sets.table", …)` and `"repository_set_items.table"`, plus
  `"idx_repository_sets_workspace_id"` and `"idx_repository_set_items_set_id"`. Migrations run after
  schema init, so the indexes are safe there; no existing table gains a column, so there is nothing
  to sequence against an `ADD COLUMN`.
- Add `RepositorySet` and `RepositorySetItem` to `apps/backend/internal/task/models/models.go`,
  next to `Repository` (`:1426`) and `TaskRepository` (`:978`).
- Add `RepositorySetRepository` to `apps/backend/internal/task/repository/interface.go` beside
  `RepositoryEntityRepository` (`:370-420`), implemented in a new
  `apps/backend/internal/task/repository/sqlite/repository_set.go`: create, get, get-by-name,
  list-by-workspace, update, delete, and a `ReplaceRepositorySetItems` that rewrites the whole
  membership list with contiguous positions in one transaction. Reads join `repositories` and filter
  `repositories.deleted_at IS NULL AND repositories.workspace_id = repository_sets.workspace_id`.
  List uses one batched query for items across the returned sets rather than one query per set.
- Extend both repository soft-delete paths, `DeleteRepository` and
  `DeleteRepositoryIfNoActiveTaskSessions`
  (`apps/backend/internal/task/repository/sqlite/repository_entity.go:143`, `:199`), to delete the
  repository's `repository_set_items` rows inside the existing transaction, exactly as they already
  delete `repository_secret_bindings`.
- Add `RepositorySetDTO` / `RepositorySetItemDTO` and a `FromRepositorySet` mapper to
  `apps/backend/internal/task/dto/dto.go`, beside `RepositoryDTO` (`:48`) and `FromRepository`
  (`:605`).

### Service, authorization, and events

- Add `apps/backend/internal/task/service/service_repository_sets.go` with `CreateRepositorySet`,
  `GetRepositorySet`, `ListRepositorySets`, `UpdateRepositorySet`, and `DeleteRepositorySet`,
  keeping `service_resources.go` under its file-size limit rather than growing it.
- Every method resolves the workspace and runs the workspace-access check used by the repository
  methods in `service_resources.go` (`internal/task/service/service_access.go`); item-route methods
  resolve the set, then authorize against the set's workspace.
- Validate before any write: trimmed name of 1 to 100 characters; case-insensitive name uniqueness
  within the workspace, reported as a conflict naming the existing set; non-empty `repository_ids`;
  no duplicate ids; and every id resolving to a live repository in the same workspace. Return typed
  errors so the handler can map `400` / `404` / `409` / `422`.
- Name, description, and membership changes for one request commit together.
- Add `RepositorySetCreated|Updated|Deleted = "repository_set.created|updated|deleted"` to
  `apps/backend/internal/events/types.go` near the repository events (`:125-127`), the matching
  `ActionRepositorySet*` constants to `apps/backend/pkg/websocket/actions.go` near `:233`, and the
  three `b.subscribe` lines to
  `apps/backend/internal/gateway/websocket/task_notifications.go` near `:60-62`. Publish from the
  service with a payload carrying `id` and `workspace_id` so the existing `extractWorkspaceID`
  routing (`task_notifications.go:316`) reaches `BroadcastToWorkspace` with no new routing branch.

### HTTP and WebSocket surface

- Add `apps/backend/internal/task/handlers/repository_set_handlers.go` following
  `repository_handlers.go` (`RegisterRepositorySetRoutes` + `registerHTTP` + `registerWS`), with the
  five REST routes from the spec and the five `repository_set.*` dispatcher actions. Collection
  routes use `:id` for the workspace, matching the sibling repository routes
  (`repository_handlers.go:38-65`); gin will not accept a second wildcard name at that position.
- Register it from `registerTaskRoutes` in `apps/backend/internal/backendapp/helpers.go:1064`, next
  to `RegisterRepositoryRoutes`.
- Extend `apps/backend/internal/task/handlers/route_registration_test.go` so the new paths and WS
  actions are pinned like every other route group.

### Boot payload

- Add `repositorySetsForState` next to `repositoriesForState`
  (`apps/backend/internal/backendapp/boot_state_routes.go:202`) emitting the workspace-keyed shape
  the store expects (`itemsByWorkspaceId` / `loadingByWorkspaceId` / `loadedByWorkspaceId`), and
  call it from `routeContextBootData` (`:143`), `tasksPageBootData` (`:77`), and
  `addHomeKanbanRouteState` (`apps/backend/internal/backendapp/boot_state.go:319`) so every surface
  that can open the create dialog hydrates sets. Include the key in the empty-workspace early
  returns so the shape is never absent, and add it to the `routeData` maps
  (`boot_state_routes.go:80-89`, `:145-150`).
- A listing failure is non-fatal: `logBootError` and an empty list, as the neighbours do.
- Add the two tables to the workspace-scoped delete list in
  `apps/backend/internal/backendapp/e2e_reset.go` (the FK cascade covers the database, but that list
  is explicit).

---

## Frontend

### Data layer

- Add `RepositorySet` / `RepositorySetItem` types to `apps/web/lib/types/http.ts` beside
  `Repository` (`:245`).
- Add `listRepositorySets`, `createRepositorySet`, `updateRepositorySet`, `deleteRepositorySet` to
  `apps/web/lib/api/domains/workspace-api.ts` beside `listRepositories` (`:28`).
- Add a `repositorySets` slice to `apps/web/lib/state/slices/workspace/` mirroring the
  `repositories` shape in `types.ts:20-24`, with `setRepositorySets`, `upsertRepositorySet`,
  `removeRepositorySet`, and `invalidateRepositorySets`.
- Add `apps/web/hooks/domains/workspace/use-repository-sets.ts` modelled on `use-repositories.ts:28`
  (store read, lazy fetch, `{ sets, isLoading, refresh }`).
- Add `apps/web/lib/ws/handlers/repository-sets.ts` and register it in `apps/web/lib/ws/router.ts`
  (`registerWsHandlers`, `:39`) so `repository_set.created|updated|deleted` update the slice. This is
  the first `repository`-family WS handler on the web side; repositories themselves have none, which
  is why sets need their own rather than extending an existing file.

### Applying a set in the picker

- Put the applier in `RepoChipsRow`
  (`apps/web/components/task-create-dialog-repo-chips.tsx:65`), not in `WorkspaceRepoChips`. Both the
  create dialog and the new-subtask form render `RepoChipsRow`
  (`apps/web/components/task/new-subtask-form-parts.tsx:240`), while Quick Chat renders
  `WorkspaceRepoChips` directly (`apps/web/components/quick-chat/quick-chat-setup.tsx:135`) - so
  that placement gives exactly the surfaces the spec asks for with no gating code.
- Add a pure `apps/web/components/task-create-dialog-repository-sets.ts` holding
  `applyRepositorySet(rows, set, repositories, nextKey)`: skip members already present, skip members
  missing from the live repository list, consume a single blank placeholder row, append the rest with
  empty branches, and report the skipped counts. Keeping it pure is what makes the apply semantics
  unit-testable without rendering the dialog.
- Allocate row keys with the taken-key loop already used by `addRemoteRepo`
  (`apps/web/components/task-create-dialog-repositories-state.ts:58-68`). `useRepositoriesState`'s
  plain counter (`:16`) would otherwise hand out a key that an injected row already holds.
- Apply through `fs.setRepositories`. No submit or payload change is needed:
  `buildRepositoriesPayload` (`apps/web/components/task-create-dialog-helpers.ts:279`) already
  handles N rows.
- Render the control next to `add-repository`
  (`apps/web/components/task-create-dialog-workspace-repo-chips.tsx:134-143`) as a new component
  file, passed down as a node so `WorkspaceRepoChips` keeps its current responsibilities. Hide it
  when the workspace has no sets or when `fs.useRemote` / `fs.noRepository` is on; disable it with
  visible reason text when `getMultiRepoExecutorDisabledReason`
  (`apps/web/components/task-create-dialog-multi-repo-guard.ts:20`) returns one.
- Verify the interaction with `useRepositoryAutoSelectEffect`
  (`apps/web/components/task-create-dialog-repository-autopick.ts:23`): its
  `canReplaceEmptyRepositoryPlaceholder` guard (`:148`) only fires with exactly one blank row, so an
  applied set of two or more is not clobbered, but a set applied before user settings load can race
  the `defer` branch (`:94`). Gate the control on `userSettingsLoaded`.
- **Mobile parity** (`/mobile-parity`): on phone widths the set list is a bottom sheet, not a hover
  dropdown, sized for touch; the disabled reason is visible text, not a tooltip.

### Saving the current selection as a set

- Add **Save as set** to the same control: a small dialog taking a name and optional description,
  submitting the form's currently selected workspace repository ids in row order, deduped, ignoring
  blank rows and branches. It reuses `createRepositorySet` and leaves the in-progress task
  untouched. Surface the `409` name conflict inline in the dialog.

### Managing sets in settings

- Add a **Repository sets** `SettingsSection` to
  `apps/web/app/settings/workspace/workspace-repositories-client.tsx:559` (below the existing
  Repositories section), with the list, create/edit/delete controls, and member reordering in a new
  component file. The client is already 624 lines against an 800-effective-line cap, so the section
  and its hooks live in their own files.
- No new settings tab: sets belong with repositories, and a new tab would mean touching
  `apps/web/lib/settings/workspace-settings-tabs.ts:48`,
  `apps/web/src/settings-routes.tsx:357-420`, the discovery catalog, and four tests for a section
  that has one natural home.
- Read-only for the Improve workspace, matching `isImproveWorkspace` handling on that page.

### Internationalization

Every string goes through `t()`. Add keys to `apps/web/src/locales/en/` and to `pt-pt`, `zh-cn`,
`zh-hk`, `zh-tw`; generate the Traditional pair with `cd apps/web && pnpm run i18n:zh-hant`. No
em dashes in any catalog. Repository set names are user data and are never translated. Verify with
`cd apps/web && pnpm run i18n:check` and `pnpm run i18n:ratchet`.

---

## Testing

| Layer | Coverage |
| --- | --- |
| Store | `repository_set_test.go` (new file): full-column round trip, name uniqueness, item uniqueness, contiguous positions after replace, soft-deleted-repository exclusion, cross-workspace member rejection, workspace cascade. Fixture style from `apps/backend/internal/office/repository/sqlite/projects_test.go:16-70`. |
| Schema | Extend the replay test (`sqlite/schema_replay_test.go:12`) and the Postgres schema test (`postgres_schema_test.go:26`). |
| Service | New `service_repository_sets_test.go`: each validation error code, published event types (pattern at `service_test.go:248`), workspace authorization. |
| Handlers | New `repository_set_handlers_test.go` over an httptest router; route + WS action pinning in `route_registration_test.go`. |
| Boot | Extend `boot_state_routes_test.go` for the new collection and its empty-workspace shape. |
| Web unit | `task-create-dialog-repository-sets.test.ts` (apply semantics: skip-present, skip-missing, placeholder consumption, key collisions, idempotence), the new store slice, the API client, the WS handler, `use-repository-sets.test.ts`, and the settings section. |
| E2E | `apps/web/e2e/tests/task/create-task-repository-sets.spec.ts` and `mobile-create-task-repository-sets.spec.ts`: apply a set and assert two `repo-chip` rows and the created task's repository ids, apply twice for idempotence, save a selection as a set, and settings CRUD. Follow `subtask.spec.ts:929` for the multi-repo assertion shape and use `causal-waits.ts` primitives. |

## Verification commands

```bash
# backend
cd apps/backend && make fmt
cd apps/backend && go test ./internal/task/... ./internal/backendapp/... ./internal/events/... ./internal/gateway/websocket/...
cd apps/backend && make lint

# frontend
cd apps/web && pnpm run typecheck
cd apps/web && pnpm vitest run components/task-create-dialog-repository-sets.test.ts lib/state/slices/workspace hooks/domains/workspace
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm e2e:raw --grep "repository set"
```

## Risks

- **Autopick race.** The repository auto-select effect writes `repositories` on open and after user
  settings load. Applying a set inside that window can be overwritten. Mitigated by gating on
  `userSettingsLoaded` and by a unit test that applies a set while the placeholder row is present.
- **Row-key collision.** Injecting rows via `setRepositories` while `useRepositoriesState`'s counter
  sits at zero produces duplicate React keys and broken uncontrolled inputs. Mitigated by reusing
  the existing taken-key loop and pinning it in a test.
- **Soft-delete drift.** Because repository deletion is soft, a membership row can outlive its
  repository. Mitigated by both deleting items in the delete transaction and filtering on read; the
  store test asserts the filter independently of the transaction.
- **Boot-payload field whitelisting.** The boot mappers are explicit camelCase whitelists
  (`boot_state_routes.go:631-634`), so a DTO field not listed is silently absent from hydration. The
  boot test asserts the hydrated shape, not just the status code.
- **File-size caps.** `workspace-repositories-client.tsx` (624 lines) and the task-create dialog
  modules are close to their limits; every addition lands in a new file.

## Dependency order

| Wave | Tasks |
| --- | --- |
| 1 | task-01 persistence contracts |
| 2 | task-02 service and events |
| 3 | task-03 HTTP and WebSocket surface |
| 4 | task-04 boot payload and web data layer |
| 5 | task-05 apply a set in the picker, task-07 settings management (parallel-safe) |
| 6 | task-06 save selection as a set |
| 7 | task-08 end-to-end coverage |
| 8 | task-09 public documentation |
| 9 | task-10 final verification |

## Task status

| Task | Status |
| --- | --- |
| [task-01-persistence-contracts](task-01-persistence-contracts.md) | done |
| [task-02-service-and-events](task-02-service-and-events.md) | done |
| [task-03-http-ws-surface](task-03-http-ws-surface.md) | done |
| [task-04-boot-and-web-data-layer](task-04-boot-and-web-data-layer.md) | done |
| [task-05-apply-set-in-picker](task-05-apply-set-in-picker.md) | done |
| [task-06-save-selection-as-set](task-06-save-selection-as-set.md) | done |
| [task-07-settings-management](task-07-settings-management.md) | done |
| [task-08-end-to-end-coverage](task-08-end-to-end-coverage.md) | done |
| [task-09-public-documentation](task-09-public-documentation.md) | done |
| [task-10-final-verification](task-10-final-verification.md) | done |
