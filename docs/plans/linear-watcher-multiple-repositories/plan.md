---
spec: docs/specs/linear-watcher-multiple-repositories/spec.md
created: 2026-08-02
status: pending
area: backend, frontend
related_prs: ["#1491 (single-repo watcher binding)", "#1978 (Sentry multi-project)", "ADR 0008 (DB upgrade safety)"]
---

# Implementation Plan: Linear Watcher Multiple Repositories

## Problem

A Linear issue watch binds **at most one** `(repository_id, base_branch)` pair (PR #1491). Teams whose issues span several repositories must duplicate watches. Sentry already supports selecting multiple projects per watch (PR #1978, dropdown-checklist + chips); the Linear watcher should let the create/edit dialog select and add **multiple repositories**, each with its own base branch.

The multi-repo task transport already exists and needs no change: `IssueTaskRequest.Repositories []IssueTaskRepository` (`apps/backend/internal/orchestrator/event_handlers_github.go:83-97`) is a slice; the executor launches one worktree per entry (`executor_execute.go:1241-1250`, `executor_multi_repo_test.go`); the dispatch self-heal already iterates every entry (`watcher_dispatch.go:257-282`). The Linear source currently hard-codes a one-element slice (`source_linear.go:83-88`) — that is the only place that must learn to carry N entries.

## Goal & Non-Goals

**Goal.** Let a Linear watch bind **N** `(repository_id, base_branch)` pairs, persisted in a new `repositories_json` column (legacy columns stay as a mirror/fallback), carried through the event into `IssueTaskRequest.Repositories`, and editable in the watcher dialog as an add/remove chip list with a per-repo base branch.

**Non-Goals.** No Jira/Sentry changes (single-repo pickers stay). No GitHub watcher changes. No executor/worktree changes. No watcher-table changes (it shows no repo column today). No new endpoints.

## Design

### Wire & storage shape

```go
// models.go
type IssueWatchRepository struct {
    RepositoryID string `json:"repositoryId"`
    BaseBranch   string `json:"baseBranch"`
}
```

`IssueWatch` gains `Repositories []IssueWatchRepository` (canonical; `json:"repositories,omitempty"`). The legacy `RepositoryID string` / `BaseBranch string` fields stay on the struct (they are still scanned from the DB columns), and a custom `UnmarshalJSON` on `IssueWatch` fills `Repositories` from the legacy fields when `repositories` is absent (plural wins when both present — the Sentry `SearchFilter.UnmarshalJSON` precedent). Marshalling is plain (both keys emitted; the service keeps the legacy fields in sync with `Repositories[0]`).

**Invariant (backward-compat, mandatory):** `len(Repositories) == 0` ⇒ `IssueTaskRequest.Repositories == nil` ⇒ blank-scratch launch, byte-for-byte today's repo-less behaviour. Covered by a dedicated source test.

### Schema + migration (`store.go`)

- `createTablesSQL`: add `repositories_json TEXT NOT NULL DEFAULT ''` to the `linear_issue_watches` block.
- New migration `addIssueWatchRepositoriesJSONColumn()`: `PRAGMA table_info` guard (house pattern, `store.go:163-192`), `ALTER TABLE ... ADD COLUMN repositories_json TEXT NOT NULL DEFAULT ''`, then **backfill**: `UPDATE linear_issue_watches SET repositories_json = json_array(json_object('repositoryId', repository_id, 'baseBranch', base_branch)) WHERE repository_id != '' AND repositories_json = ''`. Register in `initSchema` after `addIssueWatchRepositoryColumns`.
- Legacy columns are **not dropped**: the row scan keeps them; every write mirrors `Repositories[0]` into them (`''` when unbound). This keeps downgrade/older readers correct and makes the store read fallback simple.

### Store (`store_issue_watch.go`)

- `issueWatchRow` gains `RepositoriesJSON string db:"repositories_json"`.
- `toIssueWatch()`: parse `RepositoriesJSON` (empty ⇒ try legacy `RepositoryID`/`BaseBranch` → one-entry list, then empty). `encodeRepositories`/`decodeRepositories` helpers mirror `encodeFilter`/`decodeFilter`.
- `issueWatchInsertColumns` / `issueWatchSelectColumns`: add `repositories_json` (SELECT keeps `COALESCE(repositories_json, '') AS repositories_json`).
- `CreateIssueWatch` / `UpdateIssueWatch`: write `repositories_json` plus the mirrored legacy columns (already synced by the service; the store encodes `Repositories` and also persists `w.RepositoryID`/`w.BaseBranch` — synced there or in the service; pick one place — the **service** — and have the store encode only, to keep the store a dumb projection).

### Model + DTOs (`models.go`)

- `CreateIssueWatchRequest`: add `Repositories []IssueWatchRepository json:"repositories"`. Legacy `RepositoryID`/`BaseBranch` stay. Plural wins when `len > 0`.
- `UpdateIssueWatchRequest`: add `Repositories []IssueWatchRepository json:"repositories"`. **Tri-state via slice nil-ness:** absent key ⇒ `nil` ⇒ unchanged; `[]` ⇒ clear; non-empty ⇒ replace. Legacy singular `*string` fields stay for old callers; plural wins when non-nil.
- `NewLinearIssueEvent`: replace `RepositoryID`/`BaseBranch` with `Repositories []IssueWatchRepository` (internal bus — clean cutover; grep for other constructors: `publishNewLinearIssueEvent` in `service_issue_watch.go` and the source + tests are the only consumers).

### Service (`service_issue_watch.go`)

- Replace `resolveRepositoryBinding` (singular) with `resolveRepositoryBindings(ctx, workspaceID string, reqs []IssueWatchRepository) ([]IssueWatchRepository, error)`:
  - nil/empty input ⇒ `nil` (unbound).
  - Trim both fields per entry; drop entries with empty `RepositoryID` (mirrors `normalizeFilter` dropping empty ids).
  - Reject a non-empty `BaseBranch` that fails `securityutil.IsValidBaseBranchRef` (existing guard, per entry).
  - **Dedupe by `RepositoryID`, keep first occurrence** (mirrors `normalizeFilter` priority dedupe; the UI also prevents duplicates).
  - When `repoLookup` is wired: `GetRepository` per entry — missing ⇒ `ErrInvalidConfig`, cross-workspace ⇒ `ErrInvalidConfig`; fill empty `BaseBranch` with the repo's `DefaultBranch`. The lookup is **always wired in production** (`backendapp/helpers.go`), so the workspace check always runs in the API path; the unwired branch exists only for unit tests that construct the service directly.
- `CreateIssueWatch`: normalize the request's bindings first (`plural if len>0 else singular→one-entry`), run `resolveRepositoryBindings`, then set `w.Repositories`, and sync `w.RepositoryID`/`w.BaseBranch` from `Repositories[0]` before persist.
- `UpdateIssueWatch`: capture `prevRepositories`; in `applyIssueWatchPatch` handle the new field (plural present ⇒ replace; else legacy singular logic unchanged, converting the result into `Repositories`); re-resolve only when the binding changed (same edit-friendly rule as today: unchanged binding with a since-deleted repo must not block prompt/filter edits).
- `applyIssueWatchPatch`: plural present ⇒ `w.Repositories = req.Repositories` and legacy fields are derived from it; singular present + plural absent ⇒ existing rebind/reset-branch semantics, then `w.Repositories = [w.RepositoryID, w.BaseBranch]` (or empty when RepositoryID == ""); **no repository field present ⇒ the binding is left untouched** (an unrelated PATCH must never rebuild the list from the legacy mirror, which holds only the first entry).
- `publishNewLinearIssueEvent`: emit `Repositories: w.Repositories` (drop singular).

### Source (`source_linear.go`)

`BuildTaskRequest`:

```go
if len(e.Repositories) > 0 {
    req.Repositories = make([]IssueTaskRepository, 0, len(e.Repositories))
    for _, r := range e.Repositories {
        req.Repositories = append(req.Repositories, IssueTaskRepository{RepositoryID: r.RepositoryID, BaseBranch: r.BaseBranch})
    }
}
```

Empty ⇒ leave `req.Repositories` nil (invariant). `WatchMetadataKey`, throttle gate, `preflightDeletedRepository` unchanged (already list-aware).

### Frontend types & API client

- `apps/web/lib/types/linear.ts`: add `export interface LinearWatchRepositoryBinding { repositoryId: string; baseBranch: string }`. `LinearIssueWatch` gains `repositories?: LinearWatchRepositoryBinding[]` (legacy `repositoryId`/`baseBranch` stay for read-compat). `CreateLinearIssueWatchInput` / `UpdateLinearIssueWatchInput` gain `repositories?: LinearWatchRepositoryBinding[]`.
- `linear-api.ts`: no signature change (payload flows through `JSON.stringify`).

### Frontend form (`linear-issue-watch-form.ts`)

- `FormState`: replace `repositoryId`/`baseBranch` with `repositories: LinearWatchRepositoryBinding[]`.
- `formStateFromWatch`: `w.repositories ?? (w.repositoryId ? [{ repositoryId: w.repositoryId, baseBranch: w.baseBranch }] : [])`.
- `buildWatchPayload`: emit `repositories: form.repositories.filter(r => r.repositoryId)` (drop half-filled rows) instead of the singular pair. `isWatchFormReady` unchanged (a binding is optional).
- `clearWorkspaceScopedForm` (`apps/web/lib/watcher-repository-default.ts`) is shared with Jira/Sentry (whose forms still carry `repositoryId`/`baseBranch`). Extend it to clear `repositories` **when present** without breaking the singular-only callers (conditional spread guarded by `"repositories" in prev` / optional fields). Update its doc comment (no longer "shared across Linear/Jira/Sentry" verbatim).

### Frontend picker — new component `apps/web/components/watcher-repository-multi-fields.tsx`

Sentry-style add/remove, plus a per-repo branch:

- Props: `workspaceId`, `bindings: LinearWatchRepositoryBinding[]`, `onChange(next: LinearWatchRepositoryBinding[])`.
- Loads workspace repos via `useRepositories(workspaceId, !!workspaceId, true)` (same hook as `watcher-repository-fields.tsx`).
- Renders one row per binding: repository **label** + base-branch `SelectField` (branches via `useBranches` — fetch inside a per-row sub-component `RepoBindingRow` so hook call count is stable) + remove button.
- "Add repository" dropdown (Radix `Select` or `Popover`+`Command`, mirroring `sentry-issue-watch-multiselect.tsx` `ProjectMultiSelect`): lists repos **not already bound**; selecting one appends `{ repositoryId: <id>, baseBranch: "" }` ("" = repo default branch at save). Disabled when every workspace repo is already bound.
- Branch select options: `(repository default branch)` sentinel mapping to `""` (reuse `DEFAULT_BRANCH`/`DEFAULT_BRANCH_LABEL` from `watcher-repository-default.ts`) + branch names; dedupe branch names (the `new Set(branches.map(b => b.name))` trick already in `watcher-repository-fields.tsx`).
- Empty selection ⇒ renders just the add dropdown and the "(no repository)" explanation — no sentinel needed since there is no empty option.
- **i18n (ratchet-enforced):** this is a NEW file — every user-facing literal MUST go through `t()`; add a new `linear` namespace (`apps/web/src/locales/en/linear.json` + run `pnpm run i18n:pseudo` to sync `pseudo/linear.json`). Keys exist ⇒ `check-i18n-keys.mjs` stays green.

### Frontend dialog (`linear-issue-watch-dialog.tsx`)

- Replace the `<WatcherRepositoryFields …/>` block in `AutomationFields` with `<WatcherRepositoryMultiFields workspaceId={form.workspaceId} bindings={form.repositories} onChange={…} />`.
- Update the `DialogDescription` copy ("Optionally bind one or more repositories…") — **changed lines must use `t()`** per the ratchet.
- Workspace switch: the dialog's `WorkspacePicker onChange` calls `clearWorkspaceScopedForm`; ensure `repositories` is cleared with the rest of the workspace-scoped state (helper extension above).
- `watcher-repository-fields.tsx` stays untouched (still used by Jira/Sentry).

### Table

No change (`linear-issue-watch-table.tsx` shows no repository column today).

## Tests

Backend (all in `apps/backend/internal/linear/`, SQLite-backed, table-driven where practical):

- **Store round-trip (multi):** create watch with `Repositories = [{repo-123, develop}, {repo-456, main}]`; read back equal; update to a 2nd set; update to `[]` clears (legacy columns also cleared). Extend `store_issue_watch_repository_test.go`.
- **Store migration:** old schema (pre-`repositories_json`) seeded with `repository_id='repo-123', base_branch='develop'`; `NewStore` runs `initSchema`; row reads back with `Repositories == [{repo-123, develop}]` and `repositories_json` backfilled; a repo-less row stays `[]`; second `initSchema` idempotent. Extend the existing migration test with a second old-schema constant (pre-multi-repo, WITH repo columns).
- **Store legacy fallback:** a row whose `repositories_json` is `''` but legacy columns are set reads back as a one-entry list (defensive; covers any DB that skipped backfill).
- **Service create:** plural accepted; singular-only create ⇒ one-entry list; unbound ⇒ `Repositories == nil`; cross-workspace repo ⇒ `ErrInvalidConfig`; missing repo ⇒ `ErrInvalidConfig`; empty base branch filled with `DefaultBranch`; duplicate entries dedupe to one; invalid branch ref ⇒ `ErrInvalidConfig`. Extend `service_issue_watch_repository_test.go` / `service_issue_watch_test.go`.
- **Service update:** omitted `Repositories` ⇒ unchanged; `[]` ⇒ cleared; replace ⇒ new set re-resolved; singular `RepositoryID` PATCH still works (old tests keep passing); unchanged binding + soft-deleted repo does not block unrelated edits.
- **Source:** event with 2 repos ⇒ `len(req.Repositories) == 2`, order preserved, branches carried; unbound event ⇒ `req.Repositories == nil` (the invariant). Extend `source_linear_test.go` (update the existing bound/unbound tests to the new event shape).
- **Event round-trip:** `publishNewLinearIssueEvent` → source path covered via the source tests; `NewLinearIssueEvent` JSON marshals `repositories`.

Frontend:

- `linear-issue-watch-form.test.ts`: `formStateFromWatch` maps plural (and legacy singular) correctly; `buildWatchPayload` emits `repositories` array and drops empty rows; unbound ⇒ `repositories: []`.
- New `watcher-repository-multi-fields.test.tsx`: add a repo appends a row; add dropdown excludes already-bound repos; branch change updates that row only; remove deletes the row; empty state renders the add control.
- `linear-slice.test.ts` fixtures updated if they construct watches with `repositoryId`/`baseBranch`.
- i18n: `pnpm run i18n:ratchet` and `pnpm run i18n:check` clean; keys present in `en` and `pseudo` `linear.json`.

## E2E

Extend `apps/web/e2e/tests/integrations/linear-settings.spec.ts` (mock Linear provider, real SQLite store — watch CRUD exercises the real service):

- **Scenario:** seed two repositories in the workspace (`apiClient.createRepository`), open New Watcher, add both via the repo dropdown, set a branch on one, save; assert the watch persists via `apiClient.rawRequest("GET", "/api/v1/linear/watches/issue?workspace_id=…")` → `repositories` has 2 entries in order.
- **Scenario (invariant):** create a watch without touching the repository picker; assert the stored watch **omits** `repositories` (the unbound GET contract — the key is absent, never an empty array) and a triggered issue still creates a repo-less task (assert via the created task's repository association, mirroring the existing watcher trigger helpers in this spec file's sibling flows).
- **Scenario:** reopen the dialog for the saved watch; both repo rows render with the saved branches (the second repo is a real git repo registered through the API, since the backend now requires local repos to exist).
- Keep it to one spec file; run with `KANDEV_E2E_MOCK=true` (e2e fixture already sets `KANDEV_MOCK_LINEAR`).

## Rollout

- **No feature flag** — additive, empty default (`repositories_json = ''` = today's behaviour), same rationale as PR #1491.
- Migration is expand-only + backfill, idempotent, runs on first boot (ADR 0008 snapshot path).
- Backend and frontend ship in the same release, and the SPA is served by the backend, so a new UI can never reach an old backend in a supported deployment — there is no window in which `repositories` could be silently discarded. The only cross-version direction is an old UI against a new backend, which stays fully compatible via the accepted singular fields.

## Risks

- **Dual representation** (JSON column + legacy mirror columns). Mitigated: the legacy columns are derived from the canonical list by the store at write time (single derivation at the persistence boundary, so they cannot drift); the service mirrors the same values on the in-memory response object. Neither layer holds an independent value.
- **i18n ratchet on new UI.** The new component and any changed dialog lines must use `t()` with keys added to both catalogs; forgetting fails pre-commit/CI. Mitigated: keys + `i18n:pseudo` step listed in the frontend task.
- **Patch tri-state of the array.** `repositories: []` (clear) vs absent (unchanged) relies on Go slice nil-ness through JSON — `nil` slice on absent key. Covered by the update tests.
- **Scope creep to Jira/Sentry.** Explicitly out of scope; the shared helper change is guarded to be backward compatible.

## Implementation Waves

```
Wave 1 — Backend (sequential within the package; files are tightly coupled):
- [ ] models.go: IssueWatchRepository + Repositories on IssueWatch/DTOs/event + UnmarshalJSON
- [ ] store.go: column + migration + backfill; store_issue_watch.go: row/encode/CRUD
- [ ] service_issue_watch.go: resolveRepositoryBindings + create/update/patch/publish
- [ ] source_linear.go: BuildTaskRequest list mapping
- [ ] backend tests (store migration/round-trip, service, source)

Wave 2 — Frontend (sequential):
- [ ] types/linear.ts + watcher-repository-default.ts helper extension
- [ ] linear-issue-watch-form.ts (FormState + payload + tests)
- [ ] watcher-repository-multi-fields.tsx (new, t()-localized) + test
- [ ] linear-issue-watch-dialog.tsx wiring (+ dialog copy via t())

Wave 3 — E2E:
- [ ] linear-settings.spec.ts multi-repo scenarios
```

## Verification Commands

Format first (formatters may split lines and trip the complexity linter):

```bash
make fmt
```

Targeted backend:

```bash
cd apps/backend && go test ./internal/linear/... ./internal/orchestrator/...
```

Frontend:

```bash
cd apps && pnpm --filter @kandev/web typecheck
cd apps/web && pnpm vitest run src/components/linear/linear-issue-watch-form.test.ts components/watcher-repository-multi-fields.test.tsx
cd apps/web && pnpm run i18n:ratchet && pnpm run i18n:check
```

E2E:

```bash
cd apps/web && pnpm e2e linear-settings.spec.ts
```

Full gate:

```bash
make typecheck test lint
```

## Open Questions

None.
