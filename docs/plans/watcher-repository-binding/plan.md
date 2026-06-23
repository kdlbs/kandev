---
spec: none — self-contained PR plan (see "Problem" and "Design" below)
created: 2026-06-24
status: draft
area: backend, frontend
related_prs: ["#1094 (watcher self-heal)", "#1124 (use-step-default reset pattern)", "ADR 0013 (multi-branch tasks)", "ADR 0008 (DB upgrade safety)"]
---

# Implementation Plan: Issue-Watcher Repository Binding (Linear / Jira / Sentry)

## Problem

The Linear, Jira, and Sentry issue-watchers create tasks with **no repository
associated**. As a result their agents launch into a blank scratch git repo
instead of the target codebase ("started with no repository checkout").

Validated end-to-end against the current source:

- The watch models carry no repository field:
  - Linear `IssueWatch` — `apps/backend/internal/linear/models.go:190-213`
  - Jira `IssueWatch` — `apps/backend/internal/jira/models.go:162-185`
  - Sentry `IssueWatch` — `apps/backend/internal/sentry/models.go:133-156`
  All three carry only workspace/workflow/step, the per-integration filter
  (`Filter SearchFilter` for Linear/Sentry, `JQL string` for Jira),
  agent/executor profile, prompt, enabled, poll interval, max-inflight, and
  error stamps. (The models even carry an explicit comment: *"Linear/JIRA
  issues have no repository affinity"* — `linear/models.go:188-189`,
  `jira/models.go:159-161`.)

- The orchestrator sources build an `IssueTaskRequest` with no `Repositories`:
  - `LinearWatcherSource.BuildTaskRequest` — `apps/backend/internal/orchestrator/source_linear.go:60-81`
  - `JiraWatcherSource.BuildTaskRequest` — `apps/backend/internal/orchestrator/source_jira.go:57-80`
  - `SentryWatcherSource.BuildTaskRequest` — `apps/backend/internal/orchestrator/source_sentry.go:96-118`

- At launch, the worktree env-preparer branches on whether the launch request
  carries repositories. `EnvPrepareRequest.RepoSpecs()`
  (`apps/backend/internal/agent/runtime/lifecycle/env_preparer.go:86-91`) and
  `manager_launch.go:650-677` take the worktree path only when
  `len(req.Repositories) > 0`. With zero repositories the launch falls to
  `initGitRepo` (`apps/backend/internal/agent/runtime/lifecycle/manager_launch.go:1250-1300`,
  invoked at `manager_launch.go:305`), which `git init`s a blank scratch repo
  with an empty `.gitkeep` and an "Initial commit" authored by
  `Kandev Quick Chat <quickchat@kandev.local>`. The agent then runs with no
  source checkout.

The `workspace_path` override is **not** a fix:
- `MetaKeyWorkspacePath` (`apps/backend/internal/task/models/models.go:66`) is
  only written from `CreateTaskRequest.WorkspacePath`
  (`apps/backend/internal/task/service/service_requests.go:47`, applied at
  `service_tasks.go:250-254`) — the UI "starting folder" picker. Watchers never
  set it.
- When set, it runs the agent **directly in that folder with no per-task
  isolation** (`apps/backend/internal/orchestrator/executor/executor_execute.go:271`)
  — unacceptable for a fan-out watcher creating many concurrent tasks
  (e.g. a Linear watch with `maxInflightTasks: 10`).

## Goal & Non-Goals

**Goal.** Let each Linear / Jira / Sentry issue-watch optionally bind a
**repository** (`repository_id`) and a **base branch** (`base_branch`), so
watcher-created tasks carry `Repositories` and take the existing
`len(req.Repositories) > 0` path → an **isolated git worktree per task** cut
from the bound repo at the base branch. This mirrors the GitHub watcher.

**Non-Goals.**
- No change to GitHub watcher behavior — it is the reference template and is
  out of scope to modify.
- No multi-repo / multi-branch binding for watchers. A watch binds **at most
  one** `(repository_id, base_branch)`. (Multi-branch tasks — ADR 0013 — remain
  a manual/agent-driven flow.)
- No automatic re-pointing of existing repo-less watches; they keep working
  exactly as today.
- Deleted-repository self-heal is *scoped* here (see "Self-heal interplay") but
  its full implementation may land as a follow-up.

## Design

### Reference template — the GitHub watcher

The transport already exists. `IssueTaskRequest`
(`apps/backend/internal/orchestrator/event_handlers_github.go:83-91`) already
has `Repositories []IssueTaskRepository`, and `IssueTaskRepository`
(`event_handlers_github.go:94-97`) is exactly:

```go
type IssueTaskRepository struct {
    RepositoryID string
    BaseBranch   string
}
```

**No change to `IssueTaskRequest` or `IssueTaskRepository` is required** — the
three watcher sources simply need to populate the existing slice. The GitHub
source resolves owner/name → repo via a resolver; our watchers store a kandev
`repository_id` directly, so they can build the slice with no resolution step.

### Core change & the invariant

Each watch gains two optional columns/fields: `repository_id` and
`base_branch`. The single behavioural switch is in each source's
`BuildTaskRequest`:

```go
// only when bound; otherwise leave Repositories nil (today's behaviour)
if e.RepositoryID != "" {
    req.Repositories = []IssueTaskRepository{{
        RepositoryID: e.RepositoryID,
        BaseBranch:   e.BaseBranch,
    }}
}
```

> **Backward-compatibility invariant (mandatory).** An empty `repository_id`
> produces `Repositories == nil`, which preserves today's exact behaviour
> (repo-less → blank scratch via `initGitRepo`). Only a non-empty
> `repository_id` switches a watch to the worktree path. This invariant is
> covered by a dedicated test per integration (see Tests).

### Where `base_branch` is resolved

Resolve at **create/update time**, store the concrete branch:

- If `repository_id` is set and `base_branch` is empty, default it to the bound
  repository's `DefaultBranch` and persist that concrete value.
- This keeps `BuildTaskRequest` a pure projection of stored state (no I/O in the
  orchestrator hot path) and matches how the watcher already pre-resolves
  agent/executor at config time.
- Trade-off: if the repo's default branch later changes, the stored value is
  stale until the watch is edited. Acceptable and explicit; an empty stored
  `base_branch` also remains valid — the worktree manager already falls back to
  the repo default branch when `BaseBranch` is empty
  (`apps/backend/internal/worktree/manager_base_branch_fallback_test.go` exercises
  this), so a stale/blank value degrades safely rather than failing.

### Repository lookup dependency (validation + default-branch resolution)

Repositories live in the **task** package, not in the integration packages:
`models.Repository` (`apps/backend/internal/task/models/models.go:757`) carries
`WorkspaceID` and `DefaultBranch`; the store exposes
`GetRepository(ctx, id)` (`apps/backend/internal/task/repository/interface.go:200`)
and `ListRepositories(ctx, workspaceID)` (`interface.go:203`).

The Linear/Jira/Sentry services must **not** import the task service directly
(import-cycle risk). Mirror the existing post-construction wiring used for
`watchreset.TaskDeleter`:

- Each service already declares a `taskDeleter` field wired *after*
  construction via `SetTaskDeleter` to dodge the cycle
  (`linear/service.go:37-53`, `jira/service.go:40-53`, `sentry/service.go:49-57`),
  called from `apps/backend/internal/backendapp/helpers.go:520-526`.
- Add a parallel minimal interface, e.g.:

  ```go
  // package linear (and jira, sentry)
  type RepositoryLookup interface {
      GetRepository(ctx context.Context, id string) (workspaceID, defaultBranch string, ok bool)
  }
  func (s *Service) SetRepositoryLookup(rl RepositoryLookup) { s.repoLookup = rl }
  ```

- Wire it in `backendapp/helpers.go` (next to `SetTaskDeleter`) with a thin
  adapter over `services.Task` (the same `services.Task` already used by the
  `taskDeleterAdapter` in `backendapp/main.go:493`).
- When `repoLookup` is nil (e.g. unit tests that don't wire it), validation
  skips the membership/default-branch step — preserves testability and
  fail-open behaviour.

## Backend — file-by-file

The three integrations are near-identical; each bullet applies to all three
unless noted. Per-integration divergences: Jira stores its filter as
`jql TEXT`; Linear & Sentry store `filter_json TEXT`. Sentry's profile fields in
the UI don't use the `STEP_DEFAULT` sentinel (frontend note below). None of
these affect the repository columns.

### 1. Schema + migration (expand-only, nullable-by-default)

Add two columns to each watch table, matching the house style of the existing
optional id columns (`agent_profile_id`, `executor_profile_id`), which are
`TEXT NOT NULL DEFAULT ''` — **empty string = unbound**. This satisfies the
"nullable / optional / expand-only" requirement (the column is always present,
defaults to "no binding") while keeping scans `string`-typed (no `*string`).

- **Linear** — `apps/backend/internal/linear/store.go`
  - Add `repository_id TEXT NOT NULL DEFAULT ''` and
    `base_branch TEXT NOT NULL DEFAULT ''` to `createTablesSQL`
    (table block at `store.go:55-75`).
  - Add migration helper `addIssueWatchRepositoryColumns(db)` mirroring
    `addMaxInflightTasksColumn` (`store.go:114-129`) and
    `addIssueWatchLastErrorColumns` (`store.go:136-152`): use the `tableColumns`
    helper (`store.go:262-284`, `PRAGMA table_info`) to add each column only if
    missing.
  - Register it in `initSchema` (`store.go:94-108`) after the existing
    column-add steps.
- **Jira** — `apps/backend/internal/jira/store.go` (table block `59-93`): same
  two columns + a `addIssueWatchRepositoryColumns` migration in `initSchema`.
- **Sentry** — `apps/backend/internal/sentry/store.go` (table block `41-71`):
  same; Sentry already keeps its incremental column-adds in a dedicated
  migration method — extend it.

Rationale: this is the established Kandev migration pattern (idempotent
`PRAGMA table_info` guard + `ALTER TABLE ADD COLUMN`), and it is observed/logged
through the boot-time `MigrateLogger` snapshot path per ADR 0008. New databases
get the columns straight from `createTablesSQL`.

### 2. Store read/write columns

- **INSERT** lists: add `repository_id, base_branch`
  - Linear `store.go:80-84` (currently 16 placeholders → 18) and `CreateIssueWatch` (`store.go:94-117`).
  - Jira `store.go:400-404`.
  - Sentry `store_issue_watch.go:90-94`.
- **SELECT** lists / scan structs: add both columns
  - Linear `store.go:86-90` (keep the `COALESCE(...,'')` style for old DBs).
  - Jira / Sentry equivalents.
- **UPDATE**: add `repository_id = ?, base_branch = ?`
  - Linear `UpdateIssueWatch` (`store.go:194-214`); Jira/Sentry equivalents.
  `workspace_id` stays immutable (unchanged).

### 3. Model + request DTOs + patch

- Add `RepositoryID string` and `BaseBranch string` to each `IssueWatch`
  (`linear/models.go:190-213`, `jira/models.go:162-185`,
  `sentry/models.go:133-156`).
- Add the same to `CreateIssueWatchRequest`
  (`linear/models.go:248-259`, `jira/models.go:218-229`, `sentry/models.go:189-200`)
  and `UpdateIssueWatchRequest`
  (`linear/models.go:265-278`, `jira/models.go:235-248`, `sentry/models.go:204-217`)
  as pointers (`*string`) for tri-state PATCH, mirroring the existing optional
  fields.
- Patch them in `applyIssueWatchPatch`
  (`linear/service_issue_watch.go:367-398`, `jira/service.go:772-803`,
  `sentry/service_issue_watch.go:324-355`).

### 4. Validation + default-branch resolution

Extend `validateIssueWatchCreate`
(`linear/service_issue_watch.go:242-264`, `jira/service.go:729-750`,
`sentry/service_issue_watch.go:244-263`) and the update path:

- If `repository_id` is non-empty and `repoLookup` is wired:
  - Look up the repo; reject (`ErrInvalidConfig`) if it doesn't exist or its
    `workspaceID != req.WorkspaceID` (workspace-scoping / IDOR guard).
  - If `base_branch` is empty, set it to the repo's `DefaultBranch` before
    persisting.
- If `repository_id` is empty: `base_branch` is forced empty (no orphan branch
  without a repo) and all checks are skipped — the repo-less invariant.

### 5. Event payload

Add `RepositoryID string` and `BaseBranch string` to:
- `NewLinearIssueEvent` (`linear/models.go:232-245`)
- `NewJiraIssueEvent` (`jira/models.go:202-215`)
- `NewSentryIssueEvent` (`sentry/models.go:173-186`)

Populate them from the watch in the publishers:
- `publishNewLinearIssueEvent` (`linear/service_issue_watch.go:206-228`)
- `publishNewJiraIssueEvent` (`jira/service.go:691-713`)
- `publishNewSentryIssueEvent` (`sentry/service_issue_watch.go:220-242`)

### 6. Source `BuildTaskRequest`

In each source, after building the existing request, conditionally set
`Repositories` from the event (the snippet under "Core change"):
- `source_linear.go:60-81`
- `source_jira.go:57-80`
- `source_sentry.go:96-118`

No change to the `WatcherSource` interface
(`apps/backend/internal/orchestrator/watcher_dispatch.go:107-164`) — its
`BuildTaskRequest(evt any) (*IssueTaskRequest, error)` contract already returns
the struct that carries `Repositories`.

### 7. HTTP API

The create/update handlers already bind JSON into the request DTOs, so adding
the fields to the DTOs is most of the work:
- Linear `httpCreateIssueWatch` / `httpUpdateIssueWatch`
  (`linear/handlers_issue_watch.go:31-61`).
- Jira routes (`jira/handlers.go:41-48`), Sentry routes
  (`sentry/handlers_issue_watch.go:12-21`).
No new endpoints; existing workspace-ownership guards
(`assertWatchInWorkspace` / `workspaceMatches`) are unchanged.

## Frontend — file-by-file

### Types & API client (add `repositoryId?: string`, `baseBranch?: string`)

- Linear: `apps/web/lib/types/linear.ts:122-171` (`LinearIssueWatch`,
  `CreateLinearIssueWatchInput`, `UpdateLinearIssueWatchInput`);
  client `apps/web/lib/api/domains/linear-api.ts:136-196`.
- Jira: `apps/web/lib/types/jira.ts:101-150`; client
  `apps/web/lib/api/domains/jira-api.ts`.
- Sentry: `apps/web/lib/types/sentry.ts:85-125`; client
  `apps/web/lib/api/domains/sentry-api.ts`.

### Watcher dialogs — add a Repository + Base-branch picker

Place a new optional "Repository" subsection right after the Workflow/Step
("Automation") block in each dialog:
- Linear: `apps/web/components/linear/linear-issue-watch-dialog.tsx`
  (after the workflow/step + profile fields, ~`413-457`).
- Jira: `apps/web/components/jira/jira-issue-watch-dialog.tsx` (~`308-352`).
- Sentry: `apps/web/components/sentry/sentry-issue-watch-dialog.tsx` (~`341-375`).

**Recommended control:** a pair of `SelectField`s — a repository select and a
base-branch select — following the existing **"(use step default)"** reset
pattern from PR #1124 (`apps/web/lib/watcher-profile-default.ts`:
`STEP_DEFAULT`, `STEP_DEFAULT_LABEL`, `resolveProfileId`). A "(no repository —
use step default)" sentinel option maps back to `""`, preserving repo-less
watches. The branch select is disabled until a repository is chosen and
defaults to the repo's default branch.

- Source the workspace repositories from the workspace store slice
  (`apps/web/lib/state/slices/workspace/`) already available to these dialogs
  (they render a workspace picker).
- Source branches from the existing
  `apps/web/hooks/domains/workspace/use-repository-branches.ts` hook (the same
  hook the task-create repo picker uses).
- The heavier multi-repo chip component
  (`apps/web/components/task-create-dialog-repo-chips.tsx`, `RepoChipsRow`) is
  **not** reused wholesale — it is built for N-repo/N-branch task creation with
  on-machine discovery and fresh-branch toggles. A single repo+branch pair is
  the right fit; reuse only the `use-repository-branches` hook.

> Sentry's profile selects currently do **not** use the `STEP_DEFAULT` sentinel
> (they store the id/empty directly). Keep the new repository control consistent
> *within* the Sentry dialog (sentinel optional); unifying Sentry's profile
> fields is out of scope.

## Backward-compatibility invariant

Existing repo-less watches must behave exactly as today:
- Migration adds columns defaulting to `''` → every existing row is "unbound".
- Empty `repository_id` ⇒ `Repositories == nil` in `BuildTaskRequest` ⇒
  `initGitRepo` blank-scratch path, unchanged.
- A dedicated regression test per integration asserts an unbound watch yields a
  request with empty `Repositories` (Tests §).

## Self-heal interplay (PR #1094)

The dispatch coordinator self-heals when the bound **agent profile** is
soft-deleted: `preflightDeletedProfile`
(`apps/backend/internal/orchestrator/watcher_dispatch.go:170-206`) calls
`src.SelfHeal(...)` → `service.DisableIssueWatchWithError`
(`source_linear.go:136`, `source_jira.go:134`, `source_sentry.go:64`). There is
**no repository existence check** in the coordinator today.

**Decision for this PR:** *fail-open at dispatch, validate at config time.*
- Create/update validation rejects a non-existent or cross-workspace repo, so a
  bound repo is valid when stored.
- If a bound repo is later soft-deleted, the launch path already degrades
  safely: a missing/empty checkout falls back to repo-default/blank behaviour
  rather than crashing the dispatch loop.
- A symmetric "disable watch + stamp `last_error` when the bound repository is
  deleted" preflight (mirroring `preflightDeletedProfile`) is **documented as a
  follow-up**, not implemented here, to keep this PR focused. Tracked as an open
  question below.

## Tests

Backend (`*_test.go` beside each source; all three integrations):

- **Store CRUD round-trip** — bound watch persists/loads `repository_id` +
  `base_branch`. *Files:* `linear/store_issue_watch_test.go` (extend the
  existing `newTestIssueWatch` helper at `:9-18`), Jira/Sentry equivalents.
  *How:* SQLite-backed, table-driven.
- **Migration** — open a DB created from the *old* schema (no repo columns),
  run `initSchema`, assert both columns exist and existing rows default to `''`.
  *How:* `PRAGMA table_info` assertion, mirroring the existing
  `addIssueWatchLastErrorColumns` coverage.
- **Validation** — (a) bound repo in another workspace is rejected;
  (b) empty `base_branch` is filled from the repo's `DefaultBranch`;
  (c) empty `repository_id` skips all repo checks and forces empty branch.
  *How:* service test with a fake `RepositoryLookup`.
- **Source `BuildTaskRequest`** — bound event ⇒
  `Repositories == [{repoID, baseBranch}]`; **unbound event ⇒
  `Repositories == nil`** (the invariant). *Files:* `source_linear_test.go`,
  `source_jira_test.go`, `source_sentry_test.go`.
- **Dispatch parity / self-heal unchanged** — extend
  `watcher_dispatch_selfheal_test.go:81-113` coverage so a bound repo flows
  through `Dispatch` without disturbing the deleted-profile short-circuit.
- **Disable parity** — existing `store_issue_watch_disable_test.go` continues to
  pass with the new columns present.

Frontend:
- Type/compile parity (`pnpm --filter @kandev/web typecheck`); a small unit test
  on the create-payload builder asserting `repositoryId`/`baseBranch` are sent
  and that the "(no repository)" sentinel maps to `""`.

## E2E Tests

- **Scenario:** GIVEN a Linear issue-watch bound to repository R at branch B,
  WHEN the watcher creates a task, THEN the task launches in an isolated
  worktree of R at B (not a blank scratch repo).
  *File:* `apps/web/e2e/tests/integrations/linear-watch-repository.spec.ts`
  (mock Linear provider, `KANDEV_MOCK_LINEAR`). *Verify:* created task has a
  repository chip / worktree path under the task dir, not the `.gitkeep`
  scratch.
- **Scenario (invariant):** GIVEN an unbound watch, WHEN it creates a task,
  THEN behaviour is unchanged (repo-less task). Can be asserted at the
  source/service layer instead of full E2E if cheaper.
- Jira/Sentry mirror the Linear E2E (mock providers) — at least the bound-repo
  happy path for one of them plus unit parity for the others.

## Rollout

- **No feature flag.** This is additive and behind an empty default
  (`repository_id = ''` = today's behaviour); a flag would only gate a strictly
  opt-in field. (Per the repo's flag guidance in `CLAUDE.md`, flags are for
  toggling behaviour for *all* users — not the case here.)
- **Migration** is expand-only and idempotent; runs on first boot of the new
  binary and is captured by the pre-migration `VACUUM INTO` snapshot (ADR 0008).
- **Order:** ship backend (schema → model/DTO → store → validation/lookup wiring
  → event → source → API) and frontend in the same release; the UI degrades
  gracefully if pointed at an older backend (extra fields ignored), and an older
  UI simply never sends the new fields.

## Risks

- **Stale default branch.** Stored `base_branch` can drift from the repo's
  default. Mitigated by the worktree manager's empty-branch fallback and by
  re-resolution on edit. Low impact.
- **Deleted bound repository.** Until the follow-up preflight lands, a deleted
  repo degrades to fallback/blank rather than a clean disable+error. Documented;
  fail-open.
- **Cross-package coupling.** New `RepositoryLookup` dependency in three
  integration services. Mitigated by the established post-construction
  `Set...` wiring pattern (no import cycle) and nil-safe validation.
- **Three-integration drift.** The change is mechanical but triplicated; a
  shared test matrix and identical helper signatures reduce the chance of one
  integration diverging.
- **Sentry sentinel inconsistency.** Sentry profile fields don't use
  `STEP_DEFAULT`; keep the new control self-consistent and avoid scope creep.

## Implementation Waves

> Single self-contained plan (no sibling task files). Backend packages are
> independent and can be parallelised per integration; frontend is sequential
> (shared build/types); E2E last.

```
Wave 1 — Backend contracts & persistence (parallel per integration):
- [ ] Linear: schema+migration, model/DTO, store, validation+lookup, event, source
- [ ] Jira:   schema+migration, model/DTO, store, validation+lookup, event, source
- [ ] Sentry: schema+migration, model/DTO, store, validation+lookup, event, source
- [ ] backendapp wiring: SetRepositoryLookup adapter over services.Task (helpers.go)

Wave 2 — Frontend (sequential):
- [ ] types + api clients (linear/jira/sentry)
- [ ] repository + base-branch picker in the three watcher dialogs

Wave 3 — E2E:
- [ ] bound-repo worktree happy path (+ unbound invariant)
```

## Verification Commands

Format first (formatters may split lines and trip the complexity linter):

```bash
make fmt
```

Targeted backend tests:

```bash
cd apps/backend && go test ./internal/linear/... ./internal/jira/... ./internal/sentry/... ./internal/orchestrator/...
```

Frontend:

```bash
cd apps && pnpm --filter @kandev/web typecheck
cd apps/web && pnpm e2e linear-watch-repository.spec.ts
```

Full gate:

```bash
make typecheck test lint
```

## Open Questions

1. **Deleted-repository self-heal** — implement the symmetric
   `preflightDeletedRepository` (disable + `last_error`) in this PR, or ship it
   as the documented follow-up? Plan assumes follow-up.
2. **Sentry profile sentinel** — leave Sentry's profile selects without the
   `STEP_DEFAULT` sentinel (and add the repo control consistently within that
   style), or unify Sentry with Linear/Jira in a separate cleanup? Plan keeps it
   out of scope.
