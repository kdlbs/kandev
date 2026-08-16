---
spec: docs/specs/pr-outcome-attribution/spec.md
created: 2026-08-13
amended: 2026-08-15
status: draft
---

# Implementation Plan: Pull request outcome attribution

## Overview

**Amended 2026-08-15 — narrowed to upstream observation only.** The reviewer of
PR #2614 cut the Kandev-recorded closure reason from core; a plugin owns that
workflow. See the spec's Amendment history. Tasks 04, 05 and 06 are
**withdrawn**, and [task 07](task-07-narrow-to-upstream-scope.md) removes the
code they produced. The sections below describe the surviving design; where they
described the disposition endpoint, control, or E2E, that text is gone
deliberately, not lost.

Five nullable columns land on `github_task_prs` in one fail-loud migration that
also stamps a durable activation instant in `kandev_meta`. All five are then
sourced from the syncs Kandev already performs (GraphQL batched poll, gh CLI
single-PR, REST single-PR), and all five have exactly one writer: the sync
writer. Order is schema first (every read projects an explicit column list, so
nothing works until the columns exist and the projection is updated), then
client field acquisition, then the sync writer. There is no UI and no E2E: the
feature is a data-layer and API-layer addition with no core screen consuming it
(spec AC-30b).

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

- `createTablesSQL`, `github_task_prs` DDL (`store.go:107`) — append the five
  columns so fresh installs get them without an `ALTER`:
  `is_draft BOOLEAN`, `changed_files INTEGER`, `merged_by_login TEXT`,
  `closed_by_login TEXT`, `auto_merge_observed_at DATETIME`. No `NOT NULL`, no
  `DEFAULT` (AC-01).
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
  (`store.go:1492`) — append all five, qualified with `gtp.` in the second
  (AC-07). `store_taskpr_schema_drift_test.go` documents why both lists exist.
- `CreateTaskPR` (`store.go:1456`) and `ReplaceTaskPR` (`store.go:1738`) —
  append all five to the INSERT column list, placeholders, and args (AC-07).
- `UpdateTaskPR` (`store.go:1782`) — append all five columns. The sync writer is
  their only writer (spec Concurrency), so there is no second write path and no
  disjoint-column guarantee to maintain.

**`apps/backend/internal/github/models.go`** — `TaskPR` (`models.go:413`) gains
five pointer fields. No `omitempty`: AC-30 requires the keys to be present, and
`null` versus absent is the distinction the whole feature exists to preserve.

```go
IsDraft             *bool      `json:"is_draft" db:"is_draft"`
ChangedFiles        *int       `json:"changed_files" db:"changed_files"`
MergedByLogin       *string    `json:"merged_by_login" db:"merged_by_login"`
ClosedByLogin       *string    `json:"closed_by_login" db:"closed_by_login"`
AutoMergeObservedAt *time.Time `json:"auto_merge_observed_at" db:"auto_merge_observed_at"`
```

`ClosedByLogin` carries a doc comment stating the AC-15 gap verbatim: a PR whose
only post-closure sync came through REST or gh CLI keeps `NULL` permanently,
because terminal rows are excluded from the orphan sweep
(`service_pr_unwatched.go:52`). `AutoMergeObservedAt` carries a doc comment
stating it is a latched observation and never a merge cause.

Also in `models.go`: the writer-health invariants AC-36/AC-37/AC-39 recorded as
a doc comment on the column group so a downstream extract author reads them next
to the fields.

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
	taskPROutcomeSyncsTotal = expvar.NewMap("github_task_pr_outcome_syncs_total")
)
```

with the same `k=v;k=v` label helper: `populated=true|false`. Process-local and
dev-mode visible only;
AC-36 and AC-37 remain the durable signals.

### Disposition endpoint (task 04) — WITHDRAWN

Cut from scope 2026-08-15. The design that lived here described the
`PATCH .../disposition` endpoint, its service, and its store method. It is
removed rather than archived because it specifies a contract that no longer
exists (spec AC-41). See [task 04](task-04-disposition-endpoint.md); the code is
removed by [task 07](task-07-narrow-to-upstream-scope.md).

---

## Frontend

### `lib/types/github.ts` (was task 05, narrowed by task 07)

`TaskPR` (`lib/types/github.ts:220`) gains the **five** retained fields, each
declared `field?: T | null` — optional and nullable.

Optionality is deliberate and was arrived at the hard way: declaring them
strictly required broke 24 pre-existing frontend test files that construct
`TaskPR` literals without every field. The backend never omits the key (spec
AC-30), so the wire guarantee holds regardless; `?:` only relaxes what a
hand-written test literal must include. The reasoning is documented inline in
`github.ts` so a future reader does not "fix" it back to required.

`closed_by_login` carries a comment recording the AC-15 gap: a PR whose only
post-closure sync came through REST or gh CLI keeps `null` permanently.
`auto_merge_observed_at` carries a comment recording that it is a latched
observation, never a merge cause.

**No component consumes these fields** (spec AC-30b). There is no API client
method, no control, and no translation key for this feature. The frontend change
is the type alone, verified by `pnpm run typecheck`.

---


## Tests

- **Migration replay and no-backfill** — *What:* AC-01 through AC-07.
  *File:* `apps/backend/internal/github/store_task_pr_outcome_migration_test.go`.
  *How:* open an isolated SQLite DB, create `github_task_prs` **without** the
  five columns, seed a terminal row, run `initSchema` twice through the real
  `NewStore` path, then assert: all five columns exist, the seeded row is
  `NULL` in all five after both runs, `kandev_meta` holds
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
  *How:* the existing drift assertion should fail if any of the five is missing
  from `taskPRColumns` / `taskPRColumnsQualified`; extend it to cover the
  `CreateTaskPR` / `ReplaceTaskPR` INSERT lists too.
- **Removal pinning** — *What:* AC-40, AC-41. *File:*
  `apps/backend/internal/github/store_taskpr_schema_drift_test.go` (extend).
  *How:* assert the `github_task_prs` DDL and every column projection contain no
  `disposition` substring, and that no registered route path contains
  `disposition`. This is what keeps the 2026-08-15 narrowing from silently
  regressing.
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
- **Frontend** — *What:* AC-30, AC-30b. *File:* none. `TaskPR` gains five typed
  fields and no component consumes them, so `pnpm run typecheck` is the
  verification. No component test and no store-slice test are added.

---

## E2E Tests

**None.** The narrowed feature has no user-visible surface (spec AC-30b), so
there is nothing on screen to assert. The two specs written against the removed
disposition control (`pr-disposition.spec.ts`, `mobile-pr-disposition.spec.ts`)
are deleted by [task 07](task-07-narrow-to-upstream-scope.md) and are not
replaced.

---

## Verification Results

All six tasks done (see each task file's `## Results` for per-task detail).
Definition-of-done gauntlet run from the repo root on 2026-08-13:

- **`make fmt`** — clean, no diffs beyond what was already staged.
- **`make typecheck`** — clean (`tsc --noEmit` across web/desktop/theme/types/ui workspaces).
- **`make lint`** — `0 issues` (`golangci-lint run ./...`), `eslint --max-warnings 0` clean, harness lint (118 files) clean, architecture lint clean.
- **`make lint-format`** — `prettier --check` clean across web/cli/packages.
- **`cd apps/web && pnpm run i18n:ratchet`** — clean at the time of the original
  (un-narrowed) run. Task 07 changes this surface: the guard-allowlist entry for
  `pr-disposition-row.tsx` is dropped alongside the component (spec AC-41b), so
  the whole gauntlet must be re-run after the narrowing lands.
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
- [~] [task-04-disposition-endpoint](task-04-disposition-endpoint.md) — withdrawn 2026-08-15

Wave 4:
- [~] [task-05-frontend-disposition-control](task-05-frontend-disposition-control.md) — withdrawn 2026-08-15

Wave 5:
- [~] [task-06-e2e-disposition](task-06-e2e-disposition.md) — withdrawn 2026-08-15

Wave 6:
- [ ] [task-07-narrow-to-upstream-scope](task-07-narrow-to-upstream-scope.md)
```

Tasks 01, 02 and 03 remain done and are not re-opened. Task 07 is subtractive
with exactly one exception: it removes what the withdrawn tasks built and must
leave every retained column's behaviour byte-identical, EXCEPT for the AC-43 fix
added on 2026-08-16.

**The one additive item (AC-43).** Spec Review round 1 found a live AC-17
violation: `ReplaceTaskPR`'s `DELETE` + `INSERT` writes `auto_merge_observed_at`
as a plain value, so the `COALESCE` guard that protects `UpdateTaskPR` cannot
apply to the new row and a replace carrying `nil` silently clears a latched
observation. The human directed the fix be made here rather than split out. Task
07 therefore also carries the latch forward in `ReplaceTaskPR` and `RestoreTaskPR`
with tests; see that task file's "ADDITIVE" section. Related: the spec's
"exactly one writer" claim was false and is corrected — AC-07 now names four
writers (`CreateTaskPR`, `ReplaceTaskPR`, `RestoreTaskPR`, `UpdateTaskPR`), so
the `disposition*` columns must be stripped from all four column lists.

---

## Open Questions

None. One decision was made in this plan rather than deferred, recorded above
with its rationale: the new outcome fields live on `PR` (not `PRStatus`) because
`newPRStatus` is the single REST/gh convergence point.

The 2026-08-15 narrowing raised one question and answered it in the spec rather
than leaving it here: a developer database that booted an earlier revision of
this branch keeps three unused nullable columns, and no `DROP COLUMN` is emitted
to remove them (spec AC-42).
