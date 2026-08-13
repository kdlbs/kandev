---
title: Pull request outcome attribution
status: draft
created: 2026-08-12
owner: kandev
---

# Pull request outcome attribution

## Problem

`github_task_prs` records that a pull request reached `closed`, but nothing about
**why** it ended that way or **who** ended it. Two consequences:

1. Four facts GitHub already hands us on every sync are simply never asked for:
   who merged the PR, who closed it, whether auto-merge was armed, and how many
   files it changed. A fifth, `draft`, arrives on every sync and is thrown away
   after being collapsed into `mergeable_state = 'draft'`.
2. There is no closure reason at all, and there cannot be one from upstream. A
   GitHub pull request carries no `state_reason` (that field exists on issues and
   is `null` for every PR, verified below), and Kandev never closes a PR: there is
   no `ClosePR`, no `gh pr close`, and the only `gh pr` subcommands in the code are
   `view` and `list` (`gh_client.go:248`, `:290`, `:310`). There is no action to
   hook and no upstream field to read.

Without this, "closed unmerged" cannot be called abandoned. The Ops Cost dashboard
made exactly that claim on n=2 and retracted it on 2026-08-12. This spec exists so
the retraction does not have to happen twice.

## Input inventory (sampled, not assumed)

Everything below was read from the code or fetched live from the GitHub API on
2026-08-12. Where a claim in the originating card turned out to be wrong, the
correction is marked.

### Upstream field availability

Live REST sample, `GET /repos/kdlbs/kandev/pulls/2554` (merged):

| Field | Present | Value |
|---|---|---|
| `draft` | yes | `false` |
| `merged_by` | yes | `{login: "carlosflorencio", ...}` |
| `changed_files` | yes | `8` |
| `auto_merge` | yes | `null` |
| `closed_by` | **key absent from the response** | — |

Live REST sample, `GET /repos/kdlbs/kandev/pulls/2476` (closed, unmerged):
`merged_by = null`, `changed_files = 114`, `auto_merge = null`, `draft = true`,
`closed_by` absent.

Live REST sample, `GET /repos/kdlbs/kandev/issues/2476` (the same PR, via the
issues endpoint): `closed_by = {login: "nova28", ...}`, and
**`state_reason = null`** — confirming that `state_reason` is serialized for PRs
but never populated, so it is not a usable closure reason.

`gh pr view --json` field list (gh 2.87.3, printed locally): includes
`autoMergeRequest`, `changedFiles`, `isDraft`, `mergedBy`. It does **not** include
`closedBy`.

**Correction to the originating card:** `merged_by`, `auto_merge` and
`changed_files` are near-free additions. `closed_by` is **not** — it is absent
from the pulls REST endpoint and from the gh CLI's PR field set, and the GraphQL
`PullRequest` type has no `closedBy` field either. Acquiring it requires either a
`timelineItems(itemTypes: CLOSED_EVENT)` selection on the GraphQL path, or a
second REST call to the issues endpoint. This spec picks the GraphQL route and
names the resulting gap (AC-14, AC-15).

### Existing code shape

- GraphQL field selection: `graphql.go:371` `prFieldsBlock()`; decoded into
  `batchedPRResult` (`graphql.go:165`) and converted at
  `convertBatchedPRResult` (`graphql.go:398`).
- gh CLI single-PR field list: `gh_client.go:250`. REST shape: `patPR`
  (`pat_client.go:975`).
- Table DDL: `store.go:107`. Column projection used by **every** read:
  `taskPRColumns` (`store.go:1484`) and `taskPRColumnsQualified` (`store.go:1492`).
  Reads use an explicit list, not `SELECT *`, so a new column is invisible until
  it is added to both lists (`store_taskpr_schema_drift_test.go` documents why).
- Writers: `CreateTaskPR` (`store.go:1456`), `UpdateTaskPR` (fixed column list),
  `SyncTaskPR` (`service_pr_watch.go:786`).
- Migration idiom in this store: `applyIdempotentSchemaColumns` (`store.go:484`)
  swallows every error via `_, _ = s.db.Exec(...)`; `addWatchSelfHealColumns`
  (`store.go`) is the fail-loud variant, used precisely because its readers scan
  the column unconditionally.
- `Populated`-flag precedent for partial sync paths: `PRStatus.ChecksPopulated`,
  `ReviewCountsPopulated`, `UnresolvedReviewThreadsPopulated` (`models.go:340`).
- Terminal rows are excluded from the orphan sweep
  (`taskPRNeedsUnwatchedSync`, `service_pr_unwatched.go:52`): once a row is
  `merged` or `closed` it is never re-fetched by that path.
- Write surfaces for a task-PR association: `POST /api/v1/github/task-prs`,
  `DELETE /api/v1/github/task-prs/:associationId` (`controller.go:72`-`75`).
  The UI control for detach lives in the multi-PR CI popover
  (`apps/web/components/github/multi-pr-ci-popover.tsx`, reached from
  `pr-status-chip.tsx` and `pr-topbar-button.tsx` via `useTaskPR().unlink`).
- Schema-metadata table: `kandev_meta` (`internal/persistence/meta.go:41`),
  created during `persistence.Provide` before any repository schema init, and
  deliberately preserved by the database reset path
  (`internal/system/database/reset.go:111`).

### Confirmed non-defects

`review_count` reading 0 everywhere is **correct**: it counts distinct reviewers
whose latest state is `APPROVED` (`client_helpers.go:155`,
`countApprovedReviewerNodes` at `graphql.go:464`) and is written on every sync.
`comment_count` genuinely is frozen (`service_pr_watch.go:829` — "CommentCount is
no longer updated from polling"). Neither is in scope here.

## Scope

**Half A — upstream-supplied outcome fields.** Persist `is_draft`,
`changed_files`, `merged_by_login`, `closed_by_login`, and an auto-merge
observation, sourced from the syncs Kandev already performs.

**Half B — Kandev-recorded disposition.** A human-recorded closure reason
(`disposition`) plus a superseding-PR identifier, because upstream has neither.

**Cross-cutting.** Nullable-only columns, no backfill, a published activation
point, and a writer-health invariant that a downstream extract can check.

## Data model

Eight nullable columns on `github_task_prs`. Every one is `NULL` on an existing
row and stays `NULL` until a post-activation observation or an explicit user
action writes it. None has a non-`NULL` default.

| Column | Type | Meaning when `NULL` |
|---|---|---|
| `is_draft` | BOOLEAN NULL | never observed by a populating sync |
| `changed_files` | INTEGER NULL | never observed (distinct from `0`, a real value) |
| `merged_by_login` | TEXT NULL | not merged, or never observed |
| `closed_by_login` | TEXT NULL | not closed, or closure never observed by the GraphQL path |
| `auto_merge_observed_at` | DATETIME NULL | auto-merge never observed armed while Kandev was watching |
| `disposition` | TEXT NULL | nobody has recorded a reason |
| `disposition_superseded_by_url` | TEXT NULL | no superseding PR recorded |
| `disposition_recorded_at` | DATETIME NULL | no disposition ever recorded |

`disposition` accepts exactly: `unknown`, `superseded`, `duplicate`,
`exploratory`, `withdrawn`.

`NULL` and `'unknown'` are **different facts and must never be merged**: `NULL`
means nobody looked; `'unknown'` means somebody looked and could not determine
why. A reader that collapses them destroys the only signal that says whether the
dataset is being maintained.

`auto_merge_observed_at` is a **latched observation**, not a merge cause. GitHub
clears `auto_merge` once it fires (`pulls/2554` is merged and reports
`auto_merge: null`), so a poller can only ever learn "auto-merge was armed at
some instant while we were looking." It must not be read, named, or charted as
"merged by auto-merge."

## Acceptance criteria

### Migration and activation

- **AC-01** — WHEN backend startup runs the GitHub store's schema
  initialization against a database that lacks any of the eight columns, THE
  SYSTEM SHALL add each missing column via `ALTER TABLE ... ADD COLUMN` with no
  `NOT NULL` and no `DEFAULT` clause.
- **AC-02** — WHEN schema initialization runs a second time against the same
  database, THE SYSTEM SHALL complete without error and without altering any row.
- **AC-03** — IF an `ADD COLUMN` statement fails for any reason other than the
  column already existing, THEN THE SYSTEM SHALL return that error from schema
  initialization and abort startup. Duplicate-column classification SHALL use
  `db.IsDuplicateColumnError` per ADR 0027; this path SHALL NOT use
  `_, _ = s.db.Exec(...)` and SHALL NOT add a new local error-string classifier.
  Rationale: the eight columns are added to `taskPRColumns`, so a silently failed
  migration turns every task-PR read into a scan error rather than a missing
  value.
- **AC-04** — WHEN the migration runs, THE SYSTEM SHALL NOT execute any `UPDATE`
  or backfill against `github_task_prs`. Pre-existing rows SHALL retain `NULL` in
  all eight columns permanently, including rows already in a terminal state.
- **AC-05** — WHEN the migration adds at least one of the eight columns for the
  first time on a given database, THE SYSTEM SHALL persist the current UTC
  instant in `kandev_meta` under key `github_task_pr_outcome_activated_at`,
  in RFC 3339 form.
- **AC-06** — WHILE `github_task_pr_outcome_activated_at` already has a value,
  THE SYSTEM SHALL leave that value unchanged on every subsequent startup,
  including startups that add a later column from the set.
- **AC-07** — THE SYSTEM SHALL include all eight columns in `taskPRColumns` and
  `taskPRColumnsQualified`, and in the column lists of `CreateTaskPR` and
  `ReplaceTaskPR`, so every existing read path returns them and no read path
  regresses to `SELECT *`.

### Sourcing outcome fields from upstream

- **AC-08** — THE SYSTEM SHALL request `isDraft`, `changedFiles`, `mergedBy`,
  `autoMergeRequest` and a closed-event actor selection in the shared GraphQL PR
  field block, so the batched PR query and the batched branch query return the
  same fields.
- **AC-09** — THE SYSTEM SHALL request `changedFiles`, `mergedBy` and
  `autoMergeRequest` in the gh CLI single-PR `--json` field list, and
  `changed_files`, `merged_by` and `auto_merge` on the REST single-PR path.
  `closedBy` SHALL NOT be requested on either path, because neither exposes it.
- **AC-10** — WHEN a sync path has fetched a full single pull request or a
  batched GraphQL result, THE SYSTEM SHALL mark the outcome-field group as
  populated on the resulting status value.
- **AC-11** — WHEN a sync path has not fetched those fields (list or search
  results, or the noop client), THE SYSTEM SHALL leave the group unpopulated.
- **AC-12** — WHILE the outcome-field group is marked populated, THE SYSTEM
  SHALL write `is_draft`, `changed_files` and `merged_by_login` from the observed
  values, writing `NULL` for `merged_by_login` when upstream reports no merger.
- **AC-13** — IF the outcome-field group is not marked populated, THEN THE
  SYSTEM SHALL leave all three columns at their stored values, including `NULL`.
- **AC-14** — WHEN a GraphQL sync observes a closed event actor for a pull
  request, THE SYSTEM SHALL mark closure attribution as populated and write that
  actor's login to `closed_by_login`.
- **AC-15** — IF closure attribution is not marked populated, THEN THE SYSTEM
  SHALL leave `closed_by_login` unchanged. A pull request whose only sync after
  closure came through the REST or gh CLI fallback therefore keeps
  `closed_by_login = NULL` permanently, because terminal rows are excluded from
  the orphan sweep (`service_pr_unwatched.go:52`). This gap is accepted and must
  be stated wherever the column is consumed.
- **AC-16** — WHEN a populating sync observes auto-merge armed on a pull request
  AND `auto_merge_observed_at` is `NULL`, THE SYSTEM SHALL set it to the current
  UTC instant.
- **AC-17** — WHILE `auto_merge_observed_at` is non-`NULL`, THE SYSTEM SHALL
  never clear it or overwrite it, including when a later sync observes auto-merge
  disarmed or absent.
- **AC-18** — WHEN a sync changes any of `is_draft`, `changed_files`,
  `merged_by_login`, `closed_by_login` or `auto_merge_observed_at`, THE SYSTEM
  SHALL treat the row as changed for the purposes of publishing
  `github.task_pr.updated`. WHEN a sync writes values identical to the stored
  ones, THE SYSTEM SHALL NOT publish the event.
- **AC-19** — THE SYSTEM SHALL continue to derive `mergeable_state = 'draft'`
  exactly as today. `is_draft` is additive and SHALL NOT change any existing
  mergeability, auto-merge, or CI-automation behaviour.

### Recording a disposition

- **AC-20** — THE SYSTEM SHALL expose
  `PATCH /api/v1/github/task-prs/:associationId/disposition?workspace_id=<id>`
  accepting a JSON body with optional `disposition` and optional
  `superseded_by_url`.
- **AC-21** — WHEN the request names a valid association in the given workspace
  and `disposition` is one of the five permitted values, THE SYSTEM SHALL persist
  `disposition`, persist `superseded_by_url` when supplied, set
  `disposition_recorded_at` to the current UTC instant, and return the updated
  association.
- **AC-22** — WHEN the request sets `disposition` to `null`, THE SYSTEM SHALL
  clear all three disposition columns to `NULL` in one statement, restoring the
  "nobody looked" state.
- **AC-23** — IF `disposition` is a string outside the five permitted values,
  THEN THE SYSTEM SHALL reject the request with HTTP 400 and write nothing.
- **AC-24** — IF `superseded_by_url` is supplied while the resulting
  `disposition` is anything other than `superseded`, THEN THE SYSTEM SHALL reject
  the request with HTTP 400 and write nothing.
- **AC-25** — IF `superseded_by_url` is not a parseable GitHub pull request URL,
  THEN THE SYSTEM SHALL reject the request with HTTP 400, reusing the existing
  PR-URL parser and its `ErrInvalidPRURL` sentinel.
- **AC-26** — IF `superseded_by_url` resolves to the same `(owner, repo, number)`
  as the association being updated, THEN THE SYSTEM SHALL reject the request with
  HTTP 400. A PR cannot supersede itself.
- **AC-27** — WHEN the association exists but is detached
  (`detached_at IS NOT NULL`), THE SYSTEM SHALL accept the disposition write.
  A detached association is precisely a PR someone walked away from.
- **AC-28** — IF the association does not exist, or exists outside the supplied
  workspace, THEN THE SYSTEM SHALL return HTTP 404 with no distinction between
  the two cases.
- **AC-29** — WHEN a disposition write changes the stored values, THE SYSTEM
  SHALL publish `github.task_pr.updated` for the row. WHEN it writes values
  identical to the stored ones, THE SYSTEM SHALL NOT publish, and SHALL NOT
  advance `disposition_recorded_at`.
- **AC-29b** — THE SYSTEM SHALL accept a disposition write regardless of the
  association's `state`. The state restriction in AC-31 and AC-34 is a UI
  affordance, not an API constraint: a user may record intent on a PR they are
  about to close, and the endpoint has no way to know that a state observed
  moments ago is still current.
- **AC-29c** — WHEN a pull request is reopened, or reopened and later merged,
  THE SYSTEM SHALL leave any recorded `disposition` and
  `disposition_recorded_at` unchanged, because the sync writer and the
  disposition writer own disjoint columns. The row therefore carries a
  disposition recorded against a closure that was later undone. Consumers SHALL
  scope disposition reporting to rows where `state = 'closed'` and
  `merged_at IS NULL`, and the UI SHALL allow the value to be cleared (AC-33).
  A sync SHALL NOT clear the disposition: silently discarding a human's recorded
  reason on a state change is worse than a stale value a reader can filter.
- **AC-30** — THE SYSTEM SHALL surface all eight columns on the task-PR JSON
  representation returned by the existing task-PR endpoints and carried by
  `github.task_pr.updated`, and in the frontend `TaskPR` type.

### User-visible surface

- **AC-31** — WHILE a task-PR row in the multi-PR CI popover is in state
  `closed` with `merged_at` unset, THE SYSTEM SHALL offer a control that records
  a disposition for that association, offering the five permitted values.
- **AC-32** — WHEN the user selects `superseded`, THE SYSTEM SHALL let them
  supply a superseding pull request URL, and SHALL surface the server's
  validation error verbatim when the URL is rejected.
- **AC-33** — WHILE a row already carries a disposition, THE SYSTEM SHALL show
  the recorded value and allow it to be changed or cleared.
- **AC-34** — THE SYSTEM SHALL NOT offer the control for `open` or `merged`
  rows.
- **AC-35** — All new user-facing copy SHALL go through `t()` / `<Trans>` with
  no hardcoded literals and no U+2014 em dash, per the repo i18n contract.

### Writer health

- **AC-36** — THE SYSTEM SHALL guarantee the invariant: for any
  `github_task_prs` row where `merged_at >= github_task_pr_outcome_activated_at`,
  `merged_by_login` is non-`NULL`. `merged_at` is only ever written by the sync
  writer, so a row that merged after activation was necessarily observed by a
  post-activation writer; a `NULL` there is a writer fault, not a data gap.
- **AC-37** — THE SYSTEM SHALL guarantee the invariant: for any row where
  `last_synced_at >= github_task_pr_outcome_activated_at`, `is_draft` is
  non-`NULL` whenever the row's most recent sync was a populating one. Because
  `is_draft` is supplied by every populating path on every sync, this is the
  primary canary for "the writer stopped."
- **AC-38** — THE SYSTEM SHALL increment process-local `expvar` counters, named
  under a `github_task_pr_outcome_` prefix, for populating syncs, non-populating
  syncs, and disposition writes, following the existing metrics-map idiom in
  `internal/office/scheduler/metrics_vars.go` and `internal/common/subproc`.
  These are process-local and dev-mode-visible only; AC-36 and AC-37 are the
  durable, snapshot-checkable signals.
- **AC-39** — THE SYSTEM SHALL NOT state or imply either invariant for rows whose
  `merged_at` / `closed_at` predates the activation instant. Those rows are
  legitimately and permanently `NULL`.

## Ordering, idempotency, concurrency, and boundaries

**Ordering.** This feature introduces no new ordered collection and no new list
endpoint. Where succession between a task's pull requests is derived downstream
(see Out of scope), the ordering is `github_task_prs` rows filtered by `task_id`,
ordered by `created_at ASC`, tiebroken by `pr_number ASC`, then by `id ASC`.
`created_at` alone is not unique — two PRs opened in the same second by a
multi-repo launch collide — and `id` is a random UUID, so it is the final
tiebreak only, never the primary key of the ordering.

**Idempotency.** Schema initialization is replay-safe (AC-02) and the activation
instant is written at most once per database (AC-06). A disposition PATCH
repeated with an identical body leaves the row byte-identical, including
`disposition_recorded_at` (AC-29) — re-saving a value must not rewrite when it
was decided. A sync that observes unchanged values performs its `UPDATE` but
publishes no event (AC-18), matching today's `SyncTaskPR` behaviour.

**Concurrency.** The disposition writer and the sync writer touch **disjoint
column sets** and each issues a single `UPDATE`. `UpdateTaskPR`'s column list
SHALL NOT include any `disposition*` column, and the disposition statement SHALL
NOT include any sync-owned column. Consequence: a poll landing between a user's
read and their PATCH cannot clobber the disposition, and a PATCH cannot revert a
freshly-synced state. Two concurrent PATCHes on the same association are
last-write-wins on a single statement; no read-modify-write spans the request
boundary, so no lost-update window exists beyond that. Two concurrent populating
syncs for the same PR are already serialized by the existing per-row `UPDATE ...
WHERE id = ?`; the `auto_merge_observed_at` latch is set only when the stored
value is `NULL`, so a race sets it once and the loser writes the same non-`NULL`
value.

**Nil, empty, and error.** Upstream `merged_by: null` writes `NULL`, never `''`.
Upstream `auto_merge: null` leaves the latch untouched, never clears it. An
absent closed-event actor leaves `closed_by_login` untouched. `changed_files = 0`
is a real observation and is written as `0`, distinct from `NULL`. An empty
`superseded_by_url` string is treated as absent, not as an invalid URL. A failed
disposition write returns an error to the caller and publishes nothing; a failed
sync-side write follows the surrounding best-effort logging convention and leaves
the row for the next attempt.

**Defaults and boundaries.** Every column defaults to `NULL` at the SQL level
(AC-01) — no `DEFAULT 0`, no `DEFAULT ''`. `disposition` has no default value.
`disposition_recorded_at` is only ever set alongside a non-`NULL` `disposition`.
The five enum values are the complete set; adding a sixth later is a new
contract, not an implementation detail, because the extract's time series would
otherwise change meaning mid-series.

## Persistence guarantees

- No column in this feature is ever populated by a backfill, an inference, or a
  heuristic. Every non-`NULL` value is either a direct upstream observation
  (Half A) or an explicit user action (Half B).
- The activation instant is durable in `kandev_meta`, which survives the database
  reset path and is therefore readable by any point-in-time snapshot without
  schema versioning. Any consumer that reports on these columns must read it and
  scope its window to rows at or after it.
- Terminal rows are never re-fetched by the orphan sweep, so a value absent at
  the moment a PR reached its terminal state is absent forever. This is
  deliberate: it keeps the columns a record of what was observed, not a
  best-effort reconstruction.

## Failure modes

| Failure | Behaviour |
|---|---|
| `ADD COLUMN` fails for a non-duplicate reason | Startup aborts with the error (AC-03) |
| `kandev_meta` write fails during activation | Startup aborts; a database with the columns but no activation instant is unreportable and must not be shipped |
| GraphQL returns no closed-event node | `closed_by_login` untouched (AC-15) |
| Sync path is the noop client | Nothing marked populated; no column written (AC-11, AC-13) |
| Disposition PATCH with an unknown enum value | HTTP 400, nothing written (AC-23) |
| Disposition PATCH on a detached association | Accepted (AC-27) |
| Disposition PATCH on a missing or cross-workspace association | HTTP 404, no existence leak (AC-28) |

## Permissions

The disposition endpoint is workspace-scoped and follows the existing task-PR
mutation pattern: it takes `workspace_id`, resolves the association within it,
and applies the same service-layer authorization the detach endpoint applies.
It adds no new capability beyond writing an annotation on an association the
caller can already detach.

## Out of scope

Each exclusion below is a contract, not an oversight.

- **Inferring supersession into `disposition`.** `task_id` plus `created_at`
  ordering is the only succession signal that exists, and it is implicit. It is
  fully derivable downstream from columns the extract already has
  (`task_id`, `created_at`, `pr_number`, `id`) using the ordering stated above,
  so it needs no column and no writer here. It SHALL NOT be written into
  `disposition`, and no derived succession field is added to the row: an
  inference persisted next to recorded facts is exactly how a column's meaning
  changes without anyone deciding to change it.
- **Backfilling any historical row.** Explicitly forbidden by AC-04. No row that
  predates activation acquires a value, and none is labelled `abandoned` — a word
  this feature does not define and does not use.
- **`closed_by_login` on the REST and gh CLI paths.** Would cost a second REST
  call per closed PR. The GraphQL path is the production path; the gap is named
  in AC-15 rather than closed.
- **`comment_count`.** Genuinely frozen (`service_pr_watch.go:829`) and a real
  defect, but a different one.
- **`review_count`.** Not a defect; see Confirmed non-defects.
- **`disposition_recorded_by`.** Auth is off by default in Kandev, so the column
  would be empty on most installs and would invite the same "0 means nobody"
  confusion this spec exists to prevent.
- **An MCP tool for recording disposition.** The HTTP endpoint and the popover
  control are the write surface.
- **Postgres support for `internal/github`.** This store is not Postgres-capable
  today, independently of this feature: its DDL uses `DATETIME` column types and
  its column prechecks use `PRAGMA table_info`, neither of which Postgres accepts.
  This feature does not fix that and does not make it worse: per ADR 0027 it uses
  the shared `db.IsDuplicateColumnError` classifier rather than adding a new
  dialect-specific one (AC-03), so the migration is portable the day the store is.
  A Postgres replay test is therefore not required for this change.
- **Any change to mergeability, auto-merge, or CI auto-fix behaviour.** AC-19.

## Verification surfaces

- **Backend unit** — `internal/github`: migration applied to a database seeded
  with a pre-migration row, `initSchema` invoked twice, asserting the columns
  exist, the seeded row is still `NULL` in all eight, and the activation key is
  written once and not advanced by the second run. Modelled on
  `internal/task/repository/sqlite/task_external_id_migration_test.go`.
- **Backend unit** — sync writer: populated and unpopulated paths, the
  `auto_merge_observed_at` latch including the disarm case, the `NULL` vs `0`
  distinction for `changed_files`, and event publication on change only.
- **Backend unit** — disposition endpoint: each 400 case, the 404 case, the
  detached-row acceptance, clearing to `NULL`, and idempotent re-PATCH not
  advancing `disposition_recorded_at`.
- **Backend unit** — a pinning test that `UpdateTaskPR`'s column list contains no
  `disposition*` column, so the disjoint-writer guarantee cannot regress silently.
- **Frontend unit** — the popover control's visibility rule (AC-31, AC-34) and
  its error surfacing.
- **E2E — required.** This change adds a user-visible control to the multi-PR CI
  popover, a surface with existing Playwright coverage and existing mock-GitHub
  task-PR seeding (`POST /api/v1/github/mock/task-prs`). One spec: seed a
  closed-unmerged task PR, record `superseded` with a URL, assert it persists
  across reload, and assert the control is absent for a merged PR.
