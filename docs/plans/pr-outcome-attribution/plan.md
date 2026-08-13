---
spec: docs/specs/pr-outcome-attribution/spec.md
created: 2026-08-13
status: draft
---

# Implementation Plan: Pull request outcome attribution

## Overview

Eight nullable columns land on `github_task_prs` in one fail-loud migration that
also stamps a durable activation instant in `kandev_meta`. Five of them are then
sourced from the syncs Kandev already performs (GraphQL batched poll, gh CLI
single-PR, REST single-PR); three are written only by a new workspace-scoped
`PATCH .../disposition` endpoint and a control in the CI popover. Order is
schema first (every read projects an explicit column list, so nothing works
until the columns exist and the projection is updated), then client field
acquisition, then the sync writer, then the disposition endpoint, then the UI,
then E2E. Frontend and E2E follow the API they consume.

---

## Backend

### Schema and activation (task 01)

**`apps/backend/internal/persistence/meta.go`** — the `kandev_meta` read/write
helpers are currently unexported (`readKey`, `writeKey`), so no repository
package can reach them. Add two exported wrappers:

```go
// ReadMetaKey returns the value for key, or "" when the key is absent.
func ReadMetaKey(db *sqlx.DB, key string) (string, error)

// WriteMetaKeyIfAbsent inserts key=value only when key is not already
// present, and reports whether this call performed the insert.
func WriteMetaKeyIfAbsent(db *sqlx.DB, key, value string) (bool, error)
```

`WriteMetaKeyIfAbsent` uses `INSERT INTO kandev_meta (key, value) VALUES (?, ?)
ON CONFLICT(key) DO NOTHING` and reports `RowsAffected() > 0`. The
`DO NOTHING` form is what makes AC-06 race-free without a read-modify-write.
`internal/persistence` depends only on `common/ports`, `profiles`,
`common/config`, `common/logger`, and `internal/db` (verified with
`go list -deps`), so `internal/github` importing it introduces no cycle.

**`apps/backend/internal/github/store.go`**

- `createTablesSQL`, `github_task_prs` DDL (`store.go:107`) — append the eight
  columns so fresh installs get them without an `ALTER`:
  `is_draft BOOLEAN`, `changed_files INTEGER`, `merged_by_login TEXT`,
  `closed_by_login TEXT`, `auto_merge_observed_at DATETIME`,
  `disposition TEXT`, `disposition_superseded_by_url TEXT`,
  `disposition_recorded_at DATETIME`. No `NOT NULL`, no `DEFAULT` (AC-01).
- New `func (s *Store) addTaskPROutcomeColumns() (bool, error)`, called from
  `initSchemaUpgrades()` (`store.go:487`). It follows the fail-loud idiom of
  `addTaskCIRoundColumns` (`store.go:1008`): `s.tableColumns("github_task_prs")`
  precheck, then one `ALTER TABLE ... ADD COLUMN` per missing column. On Exec
  error it tolerates only `dbutil.IsDuplicateColumnError(err)` (ADR 0027) and
  returns every other error wrapped (AC-03). Returns whether any column was
  added. It must **not** go in `applyIdempotentSchemaColumns` (`store.go:484`) —
  that function swallows every error with `_, _ = s.db.Exec(...)`, and these
  columns enter `taskPRColumns`, so a silent failure turns every task-PR read
  into a scan error.
- `initSchemaUpgrades()` returns the added-columns bool up to `initSchema`,
  which on `true` calls `persistence.WriteMetaKeyIfAbsent(s.db,
  "github_task_pr_outcome_activated_at", time.Now().UTC().Format(time.RFC3339))`
  and returns the error (AC-05; a meta-write failure aborts startup per the
  spec's failure table). No `UPDATE` against `github_task_prs` anywhere in this
  path (AC-04).
- `taskPRColumns` (`store.go:1484`) and `taskPRColumnsQualified`
  (`store.go:1492`) — append all eight, qualified with `gtp.` in the second
  (AC-07). `store_taskpr_schema_drift_test.go` documents why both lists exist.
- `CreateTaskPR` (`store.go:1456`) and `ReplaceTaskPR` (`store.go:1738`) —
  append all eight to the INSERT column list, placeholders, and args (AC-07).
- `UpdateTaskPR` (`store.go:1782`) — append **only the five sync-owned**
  columns: `is_draft`, `changed_files`, `merged_by_login`, `closed_by_login`,
  `auto_merge_observed_at`. It SHALL NOT gain any `disposition*` column; the
  disjoint-writer guarantee in the spec's Concurrency section depends on it.
- New `func (s *Store) UpdateTaskPRDisposition(ctx context.Context,
  associationID string, disposition, supersededByURL *string, recordedAt
  *time.Time) error` — one statement touching exactly
  `disposition, disposition_superseded_by_url, disposition_recorded_at,
  updated_at WHERE id = ?`, and no sync-owned column.

**`apps/backend/internal/github/models.go`** — `TaskPR` (`models.go:413`) gains
eight pointer fields. No `omitempty`: AC-30 requires the keys to be present, and
`null` versus absent is the distinction the whole feature exists to preserve.

```go
IsDraft                    *bool      `json:"is_draft" db:"is_draft"`
ChangedFiles               *int       `json:"changed_files" db:"changed_files"`
MergedByLogin              *string    `json:"merged_by_login" db:"merged_by_login"`
ClosedByLogin              *string    `json:"closed_by_login" db:"closed_by_login"`
AutoMergeObservedAt        *time.Time `json:"auto_merge_observed_at" db:"auto_merge_observed_at"`
Disposition                *string    `json:"disposition" db:"disposition"`
DispositionSupersededByURL *string    `json:"disposition_superseded_by_url" db:"disposition_superseded_by_url"`
DispositionRecordedAt      *time.Time `json:"disposition_recorded_at" db:"disposition_recorded_at"`
```

`ClosedByLogin` carries a doc comment stating the AC-15 gap verbatim: a PR whose
only post-closure sync came through REST or gh CLI keeps `NULL` permanently,
because terminal rows are excluded from the orphan sweep
(`service_pr_unwatched.go:52`). `AutoMergeObservedAt` carries a doc comment
stating it is a latched observation and never a merge cause.

Also in `models.go`: the five permitted disposition values as a package
constant set plus a `validTaskPRDisposition(string) bool` helper, and the
writer-health invariants AC-36/AC-37/AC-39 recorded as a doc comment on the
column group so a downstream extract author reads them next to the fields.

### Upstream field acquisition (task 02)

**`apps/backend/internal/github/models.go`** — `PR` (`models.go:248`) already
has `Draft`. Add `ChangedFiles int`, `MergedByLogin string`,
`AutoMergeEnabled bool`. These go on `PR`, not `PRStatus`, because
`newPRStatus(pr, reviews, checks)` (`client_helpers.go:142`) is the single
convergence point for both REST and gh CLI single-PR paths and it only receives
a `*PR`.

`PRStatus` (`models.go:340`) gains, following the existing `ChecksPopulated` /
`ReviewCountsPopulated` precedent:

```go
OutcomeFieldsPopulated      bool   `json:"outcome_fields_populated,omitempty"`
ClosedByLogin               string `json:"closed_by_login,omitempty"`
ClosureAttributionPopulated bool   `json:"closure_attribution_populated,omitempty"`
```

**`graphql.go`** — `prFieldsBlock()` (`graphql.go:371`) gains
`changedFiles`, `mergedBy { login }`, `autoMergeRequest { enabledAt }` and
`timelineItems(last: 1, itemTypes: CLOSED_EVENT) { nodes { ... on ClosedEvent
{ actor { login } } } }` (AC-08). `isDraft` is already selected. Because both
`buildBatchedPRQuery` and `buildBatchedBranchQuery` call this one function, the
batched PR query and the batched branch query return the same fields for free.
`batchedPRResult` (`graphql.go:165`) gains the matching decode fields, with
`MergedBy` / `AutoMergeRequest` / the timeline actor as pointer or nullable
structs — GitHub returns `null` for all three routinely.
`convertBatchedPRResult` (`graphql.go:398`) sets `OutcomeFieldsPopulated: true`
unconditionally, and sets `ClosedByLogin` + `ClosureAttributionPopulated: true`
only when the timeline node carries a non-nil actor with a non-empty login
(AC-14).

**`gh_client.go`** — `GetPR`'s `--json` list (`gh_client.go:250`) gains
`changedFiles,mergedBy,autoMergeRequest`; `ghPR` and `convertGHPR` decode them.
`closedBy` is **not** requested — it is not in the gh CLI's PR field set
(AC-09). The `pr list` field lists in `FindPRByBranch` and `ListAuthoredPRs` are
deliberately left alone: those paths do not build a `PRStatus` through
`newPRStatus`, so they never mark the group populated (AC-11).

**`pat_client.go`** — `patPR` (`pat_client.go:975`) gains
`changed_files`, `merged_by { login }`, `auto_merge` (any non-null object means
armed); the REST-to-`PR` converter copies them (AC-09). `closed_by` is not
requested; it is absent from the pulls endpoint.

**`client_helpers.go`** — `newPRStatus` (`client_helpers.go:142`) sets
`OutcomeFieldsPopulated: true`, mirroring the existing `ChecksPopulated: true`
rationale: this path fetched a full single PR, so a zero is a real zero (AC-10).
It leaves `ClosureAttributionPopulated` false — neither REST caller can see the
closing actor (AC-15).

**`noop_client.go`** — unchanged; its `GetPRStatus` returns a bare value with
every populated flag false (AC-11). Confirm `mock_client.go`'s `GetPRStatus`
routes through `newPRStatus`; if it constructs a `PRStatus` literal, leave the
new flags false there so the mock cannot mark a group populated it never
observed.

### Sync writer (task 03)

**`apps/backend/internal/github/service_pr_watch.go`**

- `taskPRSyncState` (`service_pr_watch.go:692`) gains
  `isDraft *bool`, `changedFiles *int`, `mergedByLogin *string`,
  `closedByLogin *string`, `autoMergeObservedAt *time.Time`.
- `prepareTaskPRSyncState` (`service_pr_watch.go:717`) applies the same
  populated/preserve dance the checks and review counts already use:
  - `status.OutcomeFieldsPopulated` false → carry `tp.IsDraft`,
    `tp.ChangedFiles`, `tp.MergedByLogin` through unchanged, including `NULL`
    (AC-13). True → write `&status.PR.Draft`, `&status.PR.ChangedFiles`, and
    `&status.PR.MergedByLogin` — or `nil` for the login when upstream reports
    no merger, never `""` (AC-12).
  - `status.ClosureAttributionPopulated` false → carry `tp.ClosedByLogin`
    through unchanged (AC-15). True → write the observed login (AC-14).
  - Auto-merge latch: set `auto_merge_observed_at` to `time.Now().UTC()` only
    when `status.OutcomeFieldsPopulated && status.PR.AutoMergeEnabled &&
    tp.AutoMergeObservedAt == nil` (AC-16); otherwise carry the stored value
    forward verbatim, including when a later sync sees auto-merge disarmed
    (AC-17).
  - `changed_files == 0` is a real observation and is written as `0`, distinct
    from `NULL`.
- `SyncTaskPR` (`service_pr_watch.go:786`) — extend the `changed` expression
  with the five new fields (AC-18) using nil-safe pointer comparisons
  (`intPtrEqual` and `timeEqual` already exist; add `boolPtrEqual` and
  `stringPtrEqual` beside them), assign them onto `tp` before
  `store.UpdateTaskPR`, and leave the existing publish gate untouched. The
  function already carries `//nolint:cyclop`; if the new branches push another
  limit, extract the outcome-field reconciliation into a helper rather than
  widening the nolint.
- `mergeable_state = 'draft'` derivation is untouched (AC-19).

**New `apps/backend/internal/github/metrics_vars.go`** (AC-38), following
`internal/office/scheduler/metrics_vars.go`:

```go
var (
	taskPROutcomeSyncsTotal        = expvar.NewMap("github_task_pr_outcome_syncs_total")
	taskPROutcomeDispositionsTotal = expvar.NewMap("github_task_pr_outcome_dispositions_total")
)
```

with the same `k=v;k=v` label helper: `populated=true|false` for syncs,
`action=set|clear` for dispositions. Process-local and dev-mode visible only;
AC-36 and AC-37 remain the durable signals.

### Disposition endpoint (task 04)

**New `apps/backend/internal/github/service_task_pr_disposition.go`**, modelled
on `service_task_pr_detach.go`:

```go
var ErrInvalidDisposition = errors.New("invalid task PR disposition")

func (s *Service) SetTaskPRDisposition(
	ctx context.Context, workspaceID, associationID string,
	disposition, supersededByURL *string,
) (*TaskPR, error)
```

Order of operations, chosen so AC-26 has the association's identity available
and so authorization matches the detach endpoint:

1. Blank workspace or association id → `ErrTaskPRNotFound`.
2. `store.GetTaskPRByID`; nil, or non-empty `WorkspaceID` that differs → 
   `ErrTaskPRNotFound` (AC-28 — one error for both cases, no existence leak).
   A `detached_at` value is **not** checked: a detached association is exactly
   a PR someone walked away from (AC-27). `state` is not checked either
   (AC-29b).
3. `authorizeWorkspaceAccess(ctx, workspaceID)`.
4. Trim `supersededByURL`; an empty string is treated as absent.
5. Non-nil `disposition` outside the five permitted values →
   `ErrInvalidDisposition` (AC-23).
6. `supersededByURL` present while the resulting disposition is not
   `superseded` → `ErrInvalidDisposition` (AC-24). This also covers the
   clear-plus-URL body.
7. `parsePRURL(supersededByURL)` (`service_pr_watch.go:495`) failure →
   `fmt.Errorf("%w: %w", ErrInvalidPRURL, err)` (AC-25).
8. Parsed `(owner, repo, number)` equal to the association's, case-insensitively
   on owner/repo → `ErrInvalidDisposition` (AC-26).
9. Compare the desired triple against the stored one. Identical → return `tp`
   unchanged, write nothing, publish nothing, do not advance
   `disposition_recorded_at` (AC-29).
10. Otherwise `store.UpdateTaskPRDisposition` with `recordedAt =
    time.Now().UTC()` when disposition is non-nil, and all three set to `nil`
    when it is nil (AC-21, AC-22), then re-read and publish
    `events.GitHubTaskPRUpdated` with the row (AC-29, AC-30). Bump the
    disposition expvar counter.

A nil `disposition` — whether the JSON key was absent or explicitly `null` —
means clear. Distinguishing the two would add a `map[string]json.RawMessage`
decode for no behavioural gain: the only body where the difference could matter
(`{"superseded_by_url": "..."}` with no disposition) is already a 400 under
AC-24. Recorded here because it is a contract, not an accident.

A sync never clears a disposition (AC-29c): `UpdateTaskPR` does not name those
columns, so reopen-then-merge leaves the recorded value intact by construction.

**`controller.go`** — register beside the existing task-PR routes
(`controller.go:72`-`75`):

```go
api.PATCH("/task-prs/:associationId/disposition", c.httpSetTaskPRDisposition)
```

**`handlers.go`** — `httpSetTaskPRDisposition` reads `workspace_id` from the
query, binds `{disposition *string, superseded_by_url *string}`, maps
`ErrTaskPRNotFound` → 404 and `ErrInvalidDisposition` / `ErrInvalidPRURL` → 400
with the message surfaced to the client (the UI shows it verbatim, AC-32), and
returns the updated association on success.

---

## Frontend

### `lib/types/github.ts` (task 05)

`TaskPR` (`lib/types/github.ts:220`) gains the eight fields as
`T | null` — never optional, matching the backend's no-`omitempty` choice so
"absent" cannot be confused with "nobody looked":

```ts
export type TaskPRDisposition =
  | "unknown" | "superseded" | "duplicate" | "exploratory" | "withdrawn";
```

with a comment on `disposition` recording that `null` and `"unknown"` are
different facts and must never be collapsed, and a comment on
`closed_by_login` recording the AC-15 gap.

### `lib/api/domains/github-api.ts` (task 05)

`patchTaskPRDisposition(associationId, workspaceId, body, options?)` — a PATCH
to `/api/v1/github/task-prs/{id}/disposition?workspace_id=...` returning
`TaskPR`, shaped like the neighbouring `deleteTaskPR` (`github-api.ts:92`).

### `components/github/pr-disposition-row.tsx` (task 05, new)

Rendered from `PRCIPopover` (`components/github/pr-ci-popover.tsx:575`) between
`PRMergeabilityRow` and `PRCIAutomationControls`. `PRCIPopover` is the body the
multi-PR popover renders for the selected tab
(`multi-pr-ci-popover.tsx:216`), so placing the control there satisfies AC-31
and additionally covers the single-PR popover — a superset of what the spec
requires, with no separate code path to keep in sync.

- Renders nothing unless `pr.state === "closed" && !pr.merged_at` (AC-31,
  AC-34).
- A select over the five values plus a clear affordance; when the current value
  is non-null it is shown as selected and can be changed or cleared (AC-33).
- Selecting `superseded` reveals a URL input; the server's 400 message is
  surfaced verbatim in an inline error (AC-32).
- On success, applies the returned row with the store's `setTaskPR(pr.task_id,
  updated)` (`lib/state/slices/github/github-slice.ts:158`). The
  `github.task_pr.updated` event delivers the same row to other clients.
- All copy through `t()` in the `github` namespace (AC-35).

### i18n (task 05)

New keys in `src/locales/en/github.json` and the three sibling catalogs
(`pseudo`, `pt-pt`, `zh-cn`). No U+2014. `pnpm run i18n:pseudo` regenerates the
pseudo catalog. `components/github/pr-disposition-row.tsx` must be appended to
`i18nGuardFiles` in `eslint.i18n.options.mjs` in the same change — both popover
files are already on the list (lines 1396 and 1409) and a new sibling that is
not would be a silent hole.

---

## Tests

- **Migration replay and no-backfill** — *What:* AC-01 through AC-07.
  *File:* `apps/backend/internal/github/store_task_pr_outcome_migration_test.go`.
  *How:* open an isolated SQLite DB, create `github_task_prs` **without** the
  eight columns, seed a terminal row, run `initSchema` twice through the real
  `NewStore` path, then assert: all eight columns exist, the seeded row is
  `NULL` in all eight after both runs, `kandev_meta` holds
  `github_task_pr_outcome_activated_at` exactly once and the second run does not
  advance it, and a fresh-install DB (columns already in `createTablesSQL`) also
  replays clean. Modelled on
  `internal/task/repository/sqlite/task_external_id_migration_test.go` and the
  ADR-0027 fresh-plus-replay requirement. Postgres replay is not required: this
  store is not Postgres-capable today (`DATETIME` DDL, `PRAGMA table_info`), as
  recorded in the spec's Out of scope.
- **Fail-loud classification** — *What:* AC-03. *File:* same. *How:* inject an
  Exec error that is not a duplicate-column error and assert `initSchema`
  returns it; assert a duplicate-column error is tolerated. No local
  error-string classifier is introduced.
- **Meta helpers** — *What:* write-once semantics. *File:*
  `apps/backend/internal/persistence/meta_test.go` (extend). *How:* table-driven:
  absent key inserts and reports `true`; a second call leaves the value and
  reports `false`; `ReadMetaKey` on an absent key returns `""`, nil.
- **Column-list drift** — *What:* AC-07. *File:*
  `apps/backend/internal/github/store_taskpr_schema_drift_test.go` (extend).
  *How:* the existing drift assertion should fail if any of the eight is missing
  from `taskPRColumns` / `taskPRColumnsQualified`; extend it to cover the
  `CreateTaskPR` / `ReplaceTaskPR` INSERT lists too.
- **Disjoint writers (pinning)** — *What:* the Concurrency guarantee. *File:*
  `apps/backend/internal/github/store_task_pr_disposition_test.go`. *How:*
  assert `UpdateTaskPR`'s SQL contains no `disposition` substring, and that
  `UpdateTaskPRDisposition`'s SQL contains none of the five sync-owned columns.
  A behavioural companion: write a disposition, run a sync that changes state,
  assert the disposition survives (AC-29c); and the reverse.
- **GraphQL field block and decode** — *What:* AC-08, AC-14. *File:*
  `apps/backend/internal/github/graphql_test.go` (extend). *How:* assert the
  built query text contains `changedFiles`, `mergedBy`, `autoMergeRequest`, and
  the `CLOSED_EVENT` timeline selection, for both `buildBatchedPRQuery` and
  `buildBatchedBranchQuery`; decode fixtures with (a) a merged PR carrying
  `mergedBy`, (b) a closed PR with a `ClosedEvent` actor, (c) a PR with a null
  actor, asserting `ClosureAttributionPopulated` only in case (b).
- **gh and REST field lists** — *What:* AC-09. *File:*
  `gh_client_reads_test.go` and `pat_client_reads_test.go` (extend). *How:*
  assert the `--json` argument contains the three new fields and does **not**
  contain `closedBy`; decode a REST fixture with `changed_files`, `merged_by`,
  and a non-null `auto_merge` and assert the converted `PR`.
- **Populated flags** — *What:* AC-10, AC-11. *File:*
  `client_helpers_test.go`, `noop_client_test.go` (extend). *How:*
  `newPRStatus` sets `OutcomeFieldsPopulated` and leaves
  `ClosureAttributionPopulated` false; the noop client sets neither.
- **Sync writer** — *What:* AC-12, AC-13, AC-16, AC-17, AC-18. *File:*
  `apps/backend/internal/github/service_pr_outcome_sync_test.go`. *How:*
  table-driven over a real store: populated sync writes all three; unpopulated
  sync preserves `NULL` and preserves a previously written non-`NULL`;
  `merged_by = null` writes `NULL` not `""`; `changed_files = 0` writes `0` and
  reads back non-nil; the latch sets once, survives a disarmed observation, and
  is not overwritten by a second armed one; an unchanged sync publishes no
  `github.task_pr.updated` while a changed one does.
- **Disposition endpoint (integration, handler → service → store)** — *What:*
  AC-20 through AC-29c. *File:*
  `apps/backend/internal/github/controller_task_pr_disposition_test.go`. *How:*
  gin test server over a real store: happy path persists all three and returns
  the row; each 400 case (unknown enum, URL without `superseded`, unparseable
  URL, self-supersession); 404 for missing and for cross-workspace; accepted on
  a detached association; accepted on an `open` row (AC-29b); clearing to
  `null` nulls all three in one statement; re-PATCH with an identical body
  leaves `disposition_recorded_at` byte-identical and publishes nothing.
- **Frontend unit** — *What:* AC-31, AC-32, AC-33, AC-34. *File:*
  `apps/web/components/github/pr-disposition-row.test.tsx`. *How:* render with
  a closed-unmerged `TaskPR` and assert the control appears; with `open`,
  `merged`, and closed-with-`merged_at` and assert it does not; a rejected
  PATCH surfaces the server message verbatim; an existing value renders as
  selected and can be cleared.
- **Store slice** — *What:* the PATCH response reaches the store. *File:*
  `apps/web/lib/state/slices/github/github-slice.test.ts` (extend). *How:*
  `setTaskPR` with a row carrying a disposition replaces the prior row in place.

---

## E2E Tests

- **Scenario:** GIVEN a task with a closed, unmerged PR, WHEN the user opens the
  CI popover and records `superseded` with a superseding PR URL, THEN the
  recorded value persists across a reload.
  **File:** `apps/web/e2e/tests/pr/pr-disposition.spec.ts`.
  **What to verify:** the disposition control is visible for the closed row; after
  selecting `superseded`, entering a valid URL, and saving, the control shows the
  recorded value; after `page.reload()` it still does.
- **Scenario:** GIVEN a task with a merged PR, WHEN the user opens the CI
  popover, THEN no disposition control is offered.
  **File:** same spec.
  **What to verify:** the control's testid is absent for the merged row.

Seeding uses the existing `POST /api/v1/github/mock/task-prs`
(`mock_controller.go:56`), which already accepts `state`. A `closed` row leaves
`merged_at` nil, and a `merged` row is excluded by the state check, so both
fixtures are reachable without extending `associateTaskPRRequest`. Confirm this
during implementation; if the merged fixture needs `merged_at`, add the field to
`associateTaskPRRequest` and `buildTaskPRFromRequest` rather than weakening the
UI gate.

---

## Verification Results

All six tasks done (see each task file's `## Results` for per-task detail).
Definition-of-done gauntlet run from the repo root on 2026-08-13:

- **`make fmt`** — clean, no diffs beyond what was already staged.
- **`make typecheck`** — clean (`tsc --noEmit` across web/desktop/theme/types/ui workspaces).
- **`make lint`** — `0 issues` (`golangci-lint run ./...`), `eslint --max-warnings 0` clean, harness lint (118 files) clean, architecture lint clean.
- **`make lint-format`** — `prettier --check` clean across web/cli/packages.
- **`cd apps/web && pnpm run i18n:ratchet`** — clean (0 added + 3 modified files clean; guard allowlist intact, 1 added for `pr-disposition-row.tsx`).
- **`make test`** — `test-backend`, `test-web`, `test-cli`, `test-scripts` all run. Every failure encountered is proven pre-existing/environmental, not caused by this change (evidence below); every suite that touches this feature's code is fully green.

### Pre-existing/environmental failures encountered (not this change)

All of the following were reproduced identically against `git merge-base HEAD origin/main` (`2089e7c92`) in a separate scratch worktree, or root-caused to a host characteristic independent of any file this PR touches. None of the listed files appear in `git diff --stat 2089e7c92` for this branch.

| Failure | Root cause | Evidence |
|---|---|---|
| `internal/orchestrator` (workflow_e2e_test.go, several subtests) | Host CPU contention during the full `make test` run (many concurrent unrelated sessions on this shared host) | Re-ran `go test ./internal/orchestrator/...` in isolation on this branch: 8/8 packages pass, 94.7s. Not a merge-base issue — a flake under load. |
| `internal/system/storage/workspaces` (3 subtests, `TestRemoveDependencyDirectoryDoesNotFollowWorkspaceReplacement` etc.) | macOS `/var/folders` vs `/private/var/folders` symlink resolution in a `os.Open`-based directory-root check | Identical failure, same error text, reproduced in a scratch worktree at `2089e7c92`. |
| `apps/web/lib/http-git-server.test.ts` (3 subtests) | No live Docker daemon in this sandbox (`e2e/helpers/http-git-server.ts` requires one to resolve the bridge gateway) | `docker info` confirms `Cannot connect to the Docker daemon`; file untouched since commit `75abf779e`, long before this branch. |
| `scripts/pr-state.test.sh` ("threads count") | macOS's bundled bash is 3.2.57, which raises `unbound variable` on `${empty_array[@]}` under `set -u` (fixed in bash 4.4+); a modern bash 5.3.3 is present at `/opt/homebrew/bin/bash` but not first in this shell's resolution for `env bash` | `bash --version` confirms 3.2.57; minimal repro (`bash -c 'set -u; a=(); echo "${a[@]}"'`) reproduces the exact error; file untouched since `58e7be4b7`. |
| `scripts/opencode-code-review.test.sh` ("Harness requires trusted producer only for the dedicated OpenCode App") | Pre-existing bug, unrelated to bash version | Reproduced identically (`exit=1`, same failing case) in a full scratch worktree at `2089e7c92`. |
| `.github/scripts/release-workflow-contract_test.py` (2 subtests) + `scripts/release/publish-npm.test.mjs` (3 subtests) | `scripts/release/npm-packages.sh` uses `declare -A` (bash 4+ only); same bash-3.2-default cause as `pr-state.test.sh` | `/opt/homebrew/bin/bash -c 'source scripts/release/npm-packages.sh; echo ok'` loads cleanly under bash 5.3.3; all failures show the identical `npm-packages.sh: line 13: linux: unbound variable`. |

Cleanup: all scratch worktrees (`/tmp/kandev-scratch-baseline`, `/tmp/kandev-scratch-baseline2`) and temp comparison files were removed after use; `git worktree list` shows none remaining for this investigation.

---

## Implementation Waves And Parallel Candidates

Sequential execution in the primary conversation is the default. Waves show
dependency order only; they do not authorize subagents.

```
Wave 1:
- [x] [task-01-schema-and-activation](task-01-schema-and-activation.md)

Wave 2:
- [x] [task-02-upstream-field-sourcing](task-02-upstream-field-sourcing.md)

Wave 3:
- [x] [task-03-sync-writer](task-03-sync-writer.md)
- [x] [task-04-disposition-endpoint](task-04-disposition-endpoint.md)

Wave 4:
- [x] [task-05-frontend-disposition-control](task-05-frontend-disposition-control.md)

Wave 5:
- [x] [task-06-e2e-disposition](task-06-e2e-disposition.md)
```

Tasks 03 and 04 both depend on task 01 and are file-disjoint from each other
(`service_pr_watch.go` + `metrics_vars.go` versus
`service_task_pr_disposition.go` + `controller.go` + `handlers.go`), but they
share `store.go` only through changes task 01 already landed. Task 03
additionally depends on task 02 for the fields it writes.

---

## Open Questions

None. Two decisions were made in this plan rather than deferred, both recorded
above with their rationale: the new outcome fields live on `PR` (not `PRStatus`)
because `newPRStatus` is the single REST/gh convergence point; and an absent
`disposition` key is treated identically to an explicit `null`.
