---
status: draft
created: 2026-08-02
---

# Linear Watcher — Multiple Repository Binding

## Why

A Linear issue watch can currently bind **at most one** repository, so watcher-created tasks run against a single codebase. Teams whose issues span several repos (frontend + backend + shared lib) must create one watch per repo and duplicate the filter, or accept tasks that start with only one checkout. The Sentry watcher already supports selecting multiple projects per watch (PR #1978); the Linear watcher should offer the same "select/add multiple" capability for repositories.

## What

- A Linear issue watch SHALL bind **zero or more repositories**, each with its own base branch.
- The create/edit watcher dialog SHALL let the user **select and add multiple repositories** (and remove them), and pick a base branch per repository. No selection = unbound (repo-less task, the historical behaviour).
- Each watcher-created task SHALL carry **all** bound repositories (first entry = primary), so the agent launches in one isolated worktree per repo — the existing multi-repo task transport, unchanged.
- Existing single-repo and repo-less watches SHALL keep working after the upgrade: a saved single binding reads back as a one-entry list; an unbound watch reads back as no repositories and continues to create repo-less tasks.
- The watcher's repository count SHALL be visible in the edit dialog when reopening a saved watch (chips/rows restored from the stored bindings).
- Adding the same repository twice SHALL NOT create duplicate worktrees: the stored binding holds at most one entry per repository.

## Data model

`linear_issue_watches` gains one column; the legacy columns stay and mirror the first entry for downgrade safety.

```text
linear_issue_watches
  repositories_json  TEXT NOT NULL DEFAULT ''   # JSON array of IssueWatchRepository
  repository_id      TEXT NOT NULL DEFAULT ''   # legacy — mirrors first entry, read-compat fallback
  base_branch        TEXT NOT NULL DEFAULT ''   # legacy — mirrors first entry, read-compat fallback
```

`repositories_json` is the canonical store of the binding. Wire shape of one entry:

```text
IssueWatchRepository
  repositoryId   string   # kandev repository UUID; "" never stored
  baseBranch     string   # input "" = "use the repo default"; the CONCRETE branch is persisted
```

Rules:
- Empty `repositories_json` = unbound. The legacy `repository_id`/`base_branch` columns are derived from the first entry (or `''` when unbound) **by the store at write time** — the canonical list is the only source of truth, so the mirror cannot drift; they are read as a fallback only for rows written before the migration backfill.
- **An empty `baseBranch` in input is resolved to the repository's `DefaultBranch` and the concrete branch is persisted.** `""` never appears in a stored bound entry; it appears only transiently on the wire and for unbound watches.
- Migration backfills `repositories_json` from legacy columns for existing bound rows (`[{"repositoryId": <repository_id>, "baseBranch": <base_branch>}]`), expand-only and idempotent per the house pattern (ADR 0008).
- A stored entry's `repositoryId` is guaranteed to belong to the watch's workspace (validated at save time; production always wires the `RepositoryLookup`, so the workspace check always runs in the API path).
- **Duplicate `repositoryId` entries are collapsed to the first occurrence at save time — not rejected.** The dialog also prevents duplicates by excluding already-bound repositories from the add control.
- Entries are ordered; the first entry is the task's primary repository (matching the multi-repo task convention, e.g. `executor.go` populating legacy top-level fields from `Repositories[0]`).

## API surface

Reused shapes: `IssueTaskRequest.Repositories []IssueTaskRepository` (`apps/backend/internal/orchestrator/event_handlers_github.go`) — unchanged; the Linear source simply fills the existing slice.

`POST /api/v1/linear/watches/issue` (create) and `PATCH /api/v1/linear/watches/issue/:id` (update) — existing routes, request bodies gain the plural field:

```
repositories?: [{ repositoryId: string, baseBranch: string }]   # create: absent/[] = unbound
                                                                  # update: absent = unchanged, [] = clear
```

- Legacy singular `repositoryId` / `baseBranch` fields remain accepted; when both plural and singular are present, the plural wins. A create with only the singular fields stores a one-entry list (backward compatible with today's client).
- **Unbound GET response contract:** the watch GET response emits `repositories` (canonical) and the legacy singular keys (mirroring the first entry) **only when the watch is bound**; an unbound watch omits both — `repositories` is absent, never an empty array. The frontend treats a defined `repositories` array, including `[]`, as canonical and falls back to the legacy keys only when `repositories` is absent.
- `NewLinearIssueEvent` (internal bus) carries `Repositories []IssueWatchRepository` instead of the singular pair; the orchestrator source maps it onto `IssueTaskRequest.Repositories` directly. Empty list ⇒ `Repositories == nil` in the task request (the unbound invariant).

## Permissions

Unchanged from today: the query-only integration middleware authorizes the `workspace_id` param, and create/update additionally authorize via `authorizeWorkspaceAccess` on the workspace. Each bound `repositoryId` is validated against the watch's workspace through the existing `RepositoryLookup` (`GetRepository`) — a repository from another workspace, or one that does not exist, is rejected with `ErrInvalidConfig` at save time. The dispatch-time `preflightDeletedRepository` in the orchestrator already iterates `req.Repositories`, so a deleted repo self-heals the watch for any number of bindings.

## Failure modes

- **Deleted bound repository (post-save):** the orchestrator's `preflightDeletedRepository` checks every entry, disables the watch with a `last_error` stamp, and skips task creation (existing behaviour, already multi-entry aware).
- **Stale base branch:** a stored branch can drift from the repo's default; the worktree manager falls back to the repo default when the branch is empty/unknown (existing behaviour, unchanged).
- **Invalid config at save time** (missing repo, cross-workspace repo, invalid git ref in a base branch, duplicate entries collapsed): rejected with `ErrInvalidConfig` and surfaced by the settings UI as today.

## Persistence guarantees

The binding survives restart: it lives in `linear_issue_watches.repositories_json` (plus the legacy mirror columns), read through the same store used by the poller and the settings UI. The migration is expand-only and runs on first boot of the new binary.

## Scenarios

- **GIVEN** an unbound Linear watch, **WHEN** it creates a task, **THEN** the task request has `Repositories == nil` (blank-scratch launch, unchanged).
- **GIVEN** a watch bound to two repositories (repo-A@main, repo-B@develop), **WHEN** it creates a task, **THEN** the task request carries `[{repo-A, main}, {repo-B, develop}]` in order, and the task launches with one worktree per repo (repo-A primary).
- **GIVEN** a watch bound to repo-1 only (stored before this feature), **WHEN** the watch is loaded after upgrade, **THEN** it reads back with `repositories == [{repositoryId: repo-1, baseBranch: <saved or ''>}]`.
- **GIVEN** the user opens the watcher dialog and selects two repositories, **WHEN** they save, **THEN** the stored watch has both entries and reopening the dialog shows both chips with their branches.
- **GIVEN** the user selects the same repository twice in the dialog, **WHEN** they save, **THEN** the stored binding contains that repository once (no duplicate worktrees).
- **GIVEN** a create request with `repositoryId`/`baseBranch` singular fields only, **WHEN** saved, **THEN** it stores a one-entry `repositories` list (backward compatible).
- **GIVEN** a create request binding a repository from another workspace (or a non-existent one), **WHEN** saved, **THEN** it is rejected with a 400-family `ErrInvalidConfig` and nothing is stored.
- **GIVEN** a bound watch whose repository is later soft-deleted, **WHEN** the watcher dispatches, **THEN** the watch is disabled with a `last_error` and no task is created.
- **GIVEN** an update request omitting `repositories`, **WHEN** patched, **THEN** the binding is left unchanged; an update sending `repositories: []` clears it.
- **GIVEN** a watch bound to two repositories, **WHEN** an unrelated PATCH (e.g. prompt or enabled only) is applied, **THEN** both entries remain — the binding is never rebuilt from the legacy mirror, which holds only the first entry.

## Out of scope

- Jira and Sentry watcher dialogs keep their single-repo picker; this feature is Linear-only (Sentry already has multi-project, and its repository binding stays single for now).
- No change to the GitHub watcher (the reference multi-repo implementation), to the task-creation multi-repo transport, or to the executor/worktree layer.
- No UI change to the watcher list table (it does not display repository bindings today, single or multiple).
- No automatic migration of repo-less watches to bound ones; unbound stays unbound.

## Open questions

- None blocking. (Namespacing of new UI copy is a plan-level detail; the i18n ratchet governs it.)
