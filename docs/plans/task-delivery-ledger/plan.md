---
spec: docs/specs/task-delivery-ledger/spec.md
created: 2026-08-13
status: in-progress
---

# Implementation Plan: Task delivery ledger

**Implementation status:** tasks 01-07 (the ledger backend and the Office run
outcome backend) are done and have been through multiple Build/Testing/Review
rounds on `feature/tel-distinguish-dire-34b` — see the `Review round N` /
`R5-Fx` / `R6-Fx` comments across `apps/backend/internal/delivery/` and
`apps/backend/internal/office/service/` for the authoritative, in-code record
of what each round found and fixed. Tasks 08-09 (frontend + E2E) are still
pending, as their own task files state.

## Overview

Two delimited changes ship together. The **delivery ledger** is a new
`task_delivery_ledger` table plus a new `internal/delivery` package that owns its
schema, its evaluator, and a 5-minute sweep; it classifies each
`(task, repository)` pair as `pr_merge | direct_commit | no_delivery_observed |
unknown` and separately records whether the pair's work was observed reaching the
repository's default branch. The **Office run outcome** is a nullable
`runs.outcome` column written at the six active `FinishRun` call sites, so a
budget-blocked or idle-skipped run stops being counted as a success.

Order is dependency-driven: shared migration/meta primitives first, then the
ledger's persistence and its pure evaluator, then the evidence queries and the
git ancestry probe, then the sweep that drives them and the writer-health
signals. The Office half is independent of the ledger and runs in parallel from
wave 1, with its frontend and E2E following.

---

## Design decisions taken here

The spec fixes the contract; these are the implementation choices it left open.
Each is a decision, not a restatement.

**D1 — The ledger gets its own package, `internal/delivery/`.** It cannot live in
`internal/task/repository/sqlite` because the evaluator must read
`github_task_prs`, `gitlab_task_mrs` and `azure_devops_task_prs`, which that
package does not own; it cannot live in `internal/analytics` because that package
is a read-side stats + HTTP handler surface with no writer and no background
loop, and the ledger is a writer with a lifecycle. `internal/github/store.go` is
the ownership pattern being followed: a package that owns its own table DDL,
runs its own idempotent migration at boot, and exposes `Provide(...)`.

**D2 — `runs.outcome` is added by the office repository's own migration.** It is a
column on an existing table owned by `internal/office/repository/sqlite/base.go`,
so per `apps/backend/CLAUDE.md` it is added by an idempotent `ADD COLUMN` in
`base_migrations.go` and *also* listed inline in the `CREATE TABLE` so fresh
databases get it. The migration is the source of truth for evolution.

**D3 — Detachment is a GitHub-only predicate.** `detached_at` exists on
`github_task_prs` (`internal/github/store.go:137`) and on neither
`gitlab_task_mrs` nor `azure_devops_task_prs`. The spec's rule-1 "is not
detached" is therefore evaluated as `detached_at IS NULL` for GitHub and is
vacuously true for the other two providers. Do not add a `detached_at` column to
the GitLab or Azure tables: that is a provider-table change and this card does
not own it.

**D4 — Azure DevOps merge is read from `status`.** `azure_devops_task_prs` has no
`merged_at` (`internal/azuredevops/store.go:37-58`). `status = 'completed'` is
the merged signal; `abandoned` and `active` are not. Any other status value
leaves the pair unclassified by rule 1 and falls through to rules 3-5, per the
spec's failure mode. Because Azure rows carry no `merged_at`, they sort **last**
in the `delivery_ref` tiebreak (`merged_at` NULLs last), then by
`pull_request_id` ascending.

**D5 — Missing provider tables are tolerated, classified through `internal/db`.**
A database where a provider store never initialized has no such table. ADR 0027
forbids local `strings.Contains` migration classifiers, so this adds
`db.IsMissingTableError` alongside the existing `IsDuplicateColumnError` /
`IsAlreadyExistsError` rather than string-matching in `internal/delivery`.

**D6 — The snapshot-write trigger is served by the sweep, not a new event.** The
spec lists four evaluation triggers. Three have existing event types
(`task_session.state_changed`, `github.task_pr.updated`,
`gitlab.task_mr.updated`). There is **no** event published when a git snapshot is
written (`internal/task/repository/sqlite/git_snapshots.go:101` writes the row
and publishes nothing), and Azure has no PR-updated event. Rather than add a new
event type on the snapshot hot path, the sweep's freshness predicate covers both:
it evaluates every candidate pair whose `last_evaluated_at` is older than its
most recent input observation, and snapshot `created_at` is one of those
observations. Worst-case latency is one sweep interval, which the spec already
fixes at 5 minutes. See **Open Questions**.

---

## Backend

### Shared primitives (`internal/db`, `internal/persistence`)

**`apps/backend/internal/db/errors.go`** — add:

```go
// IsMissingTableError reports whether err means the referenced table or
// relation does not exist. Classifies the SQLite "no such table" string and
// Postgres SQLSTATE 42P01 (undefined_table).
func IsMissingTableError(err error) bool
```

Same shape and test placement as the existing `IsDuplicateColumnError` /
`IsAlreadyExistsError`. Required by D5 so `internal/delivery` never string-matches.

**`apps/backend/internal/persistence/meta.go`** — add:

```go
// WriteKeyIfAbsent writes value at key only if key is absent, and reports
// whether this call performed the write. Replay-safe: a second call against a
// key that already exists is a no-op and returns false.
func WriteKeyIfAbsent(db *sqlx.DB, key, value string) (bool, error)
```

Implemented as `INSERT INTO kandev_meta (key, value) VALUES (?, ?) ON CONFLICT
(key) DO NOTHING` through `db.Rebind`, reading `RowsAffected`. The existing
unexported `writeKey` (`meta.go:75`) is an upsert and would overwrite an
activation point on every boot, which the spec forbids. `kandev_meta` survives
`internal/system/database/reset.go` (`reset.go:111` keeps it), so activation
points survive a database reset — which is correct: the extract needs them.

### `internal/delivery` (new package)

```
apps/backend/internal/delivery/
├── models.go        # Outcome, Basis, LedgerRow, Observation types + rank()
├── store.go         # DDL, migration, activation point, lattice upsert, reads
├── observe.go       # candidate pairs + per-pair evidence queries
├── evaluator.go     # pure Classify(Observation) -> Classification
├── ancestry.go      # git merge-base --is-ancestor probe
├── sweep.go         # 5-minute loop + event subscribers, Start/Stop
├── metrics.go       # expvar maps
└── provider.go      # Provide(writer, reader, deps, log) (*Service, func() error, error)
```

**`models.go`** — the closed vocabularies as typed string constants:

```go
type Outcome string
const (
    OutcomeUnknown            Outcome = "unknown"
    OutcomeNoDeliveryObserved Outcome = "no_delivery_observed"
    OutcomeDirectCommit       Outcome = "direct_commit"
    OutcomePRMerge            Outcome = "pr_merge"
)

// rank orders the promotion lattice. A higher rank never demotes to a lower.
func rank(o Outcome) int  // "" -> 0, unknown -> 1, no_delivery_observed -> 2,
                          // direct_commit -> 3, pr_merge -> 4
```

`delivery_basis` and `reached_default_basis` constants exactly as the spec's
**Basis vocabulary** tables. `Outcome` and both basis types get a `Valid()`
method, and a table-driven test pins that the constant sets match the spec so a
typo cannot silently widen the enum.

**`store.go`** — schema, applied through `db.MigrateLogger` at `Provide`:

```sql
CREATE TABLE IF NOT EXISTS task_delivery_ledger (
    id                       TEXT PRIMARY KEY,
    task_id                  TEXT NOT NULL,
    repository_id            TEXT NOT NULL,
    workspace_id             TEXT NOT NULL,
    delivery_outcome         TEXT,
    delivery_basis           TEXT,
    delivery_ref             TEXT,
    reached_default_at       TIMESTAMP,
    reached_default_basis    TEXT,
    reached_default_ref      TEXT,
    observed_branch_commits  INTEGER,
    first_classified_at      TIMESTAMP,
    last_evaluated_at        TIMESTAMP NOT NULL,
    evaluation_seq           INTEGER NOT NULL DEFAULT 1,
    created_at               TIMESTAMP NOT NULL,
    updated_at               TIMESTAMP NOT NULL,
    UNIQUE(task_id, repository_id),
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_task_delivery_ledger_ws_evaluated
    ON task_delivery_ledger(workspace_id, last_evaluated_at);
```

Every column the feature adds is nullable except identity, sequence and
timestamps, exactly as the spec's **Data model** table. No `DEFAULT 0` and no
`DEFAULT ''` on any classification or observation column: a legacy or
never-evaluated row must read `NULL`, and a default would make "not observed"
indistinguishable from "observed as nothing".

Immediately after the DDL, write the activation point exactly once:

```go
persistence.WriteKeyIfAbsent(writer, "telemetry.delivery_ledger.activated_at",
    time.Now().UTC().Format(time.RFC3339))
```

**The lattice lives in SQL, not in Go.** One statement, so two racing evaluators
converge regardless of arrival order and neither can lose the other's write:

```sql
INSERT INTO task_delivery_ledger (
    id, task_id, repository_id, workspace_id,
    delivery_outcome, delivery_basis, delivery_ref,
    reached_default_at, reached_default_basis, reached_default_ref,
    observed_branch_commits, first_classified_at,
    last_evaluated_at, evaluation_seq, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
ON CONFLICT (task_id, repository_id) DO UPDATE SET
    -- Promotion only. :rank is the incoming outcome's lattice rank, computed
    -- in Go and bound as a parameter; the stored rank is recomputed in SQL so
    -- the comparison never depends on a prior read.
    delivery_outcome = CASE WHEN :rank > <stored_rank_expr>
                            THEN :outcome ELSE task_delivery_ledger.delivery_outcome END,
    delivery_basis   = CASE WHEN :rank >= <stored_rank_expr>
                            THEN :basis ELSE task_delivery_ledger.delivery_basis END,
    delivery_ref     = CASE WHEN :rank > <stored_rank_expr>
                            THEN :ref ELSE task_delivery_ledger.delivery_ref END,
    -- Write-once and monotonic.
    reached_default_at    = COALESCE(task_delivery_ledger.reached_default_at, :reached_at),
    reached_default_basis = CASE WHEN task_delivery_ledger.reached_default_at IS NULL
                                 THEN :reached_basis ELSE task_delivery_ledger.reached_default_basis END,
    reached_default_ref   = CASE WHEN task_delivery_ledger.reached_default_at IS NULL
                                 THEN :reached_ref ELSE task_delivery_ledger.reached_default_ref END,
    observed_branch_commits = MAX(COALESCE(task_delivery_ledger.observed_branch_commits, 0), :ahead),
    first_classified_at   = COALESCE(task_delivery_ledger.first_classified_at, :first_classified),
    -- Always advance.
    last_evaluated_at = :now,
    evaluation_seq    = task_delivery_ledger.evaluation_seq + 1,
    -- Advance only on a real change.
    updated_at = CASE WHEN <any classification/observation column changed>
                      THEN :now ELSE task_delivery_ledger.updated_at END
```

`<stored_rank_expr>` is a `CASE` over `task_delivery_ledger.delivery_outcome`
mapping the four values to 1-4 and `NULL` to 0. `delivery_basis` uses `>=` rather
than `>` so a re-evaluation at the same outcome can refine the basis
(`branch_commits_unmerged` becoming `reached_default_unattributed` once ancestry
lands), which the spec's fifth Classification scenario requires. When the stored
rank exceeds the incoming rank, the caller increments
`delivery_ledger_demotions_suppressed_total`.

`MAX(a, b)` is SQLite's scalar two-argument `MAX`; on Postgres it is `GREATEST`.
Select the dialect through the existing `internal/db` dialect seam used elsewhere
in the repository rather than branching on a driver-name string in
`internal/delivery`.

Also on `store.go`:

- `MaxLastEvaluatedAt(ctx) (time.Time, error)` — the data-side stall signal.
- `ListStalePairs(ctx)` — the sweep's work list; see **`observe.go`**.

**`observe.go`** — candidate pairs and per-pair evidence, all against the single
shared database (every table below lives in the same SQLite file / Postgres
schema).

Candidate pairs are the union of `task_repositories`, non-empty
`task_sessions.repository_id`, and the `task_id`/`repository_id` of all three
provider tables, restricted to `repository_id <> ''` and to repositories with
`deleted_at IS NULL` (a soft-deleted repository freezes evaluation per the spec).
A task with no repository yields no row. Ordering is `tasks.created_at ASC,
tasks.id ASC, repositories.id ASC` — deterministic, so an interrupted sweep
resumes on the same order.

Per pair, gather one `Observation`:

| Field | Source |
|---|---|
| `DefaultBranch` | `repositories.default_branch` (`internal/task/repository/sqlite/base_schema.go:325`) |
| `MaxAhead` | `MAX(ahead)` over the pair's `task_session_git_snapshots` |
| `DistinctHeads` | `COUNT(DISTINCT head_commit)` where `head_commit <> ''` |
| `DefaultBranchHeads` | distinct non-empty `head_commit` where `branch = default_branch` |
| `SnapshotCount` | row count; zero distinguishes rule 4 from rule 5 |
| `LastHeadCommit` | greatest-`created_at` snapshot's `head_commit`, for ancestry |
| `MergedPRs` | union across the three provider tables (below) |

Snapshots reach the pair through `task_session_git_snapshots.session_id ->
task_sessions.id`, matching on `task_sessions.task_id` **and**
`task_sessions.repository_id`. A session with an empty `repository_id`
contributes to no pair and increments
`delivery_ledger_sessions_unattributed_total`.

Merged provider rows, each query wrapped so `db.IsMissingTableError` yields zero
rows rather than failing the evaluation (D5):

```sql
-- GitHub: detachment applies here and only here (D3).
SELECT pr_url, merged_at, pr_number, base_branch FROM github_task_prs
 WHERE task_id = ? AND repository_id = ?
   AND merged_at IS NOT NULL AND detached_at IS NULL
-- GitLab: no detachment concept.
SELECT mr_url, merged_at, mr_iid, base_branch FROM gitlab_task_mrs
 WHERE task_id = ? AND repository_id = ? AND merged_at IS NOT NULL
-- Azure: no merged_at; status carries it (D4).
SELECT pull_request_url, NULL, pull_request_id, target_branch
  FROM azure_devops_task_prs
 WHERE task_id = ? AND repository_id = ? AND status = 'completed'
```

`delivery_ref` selection follows the spec exactly: earliest `merged_at` (NULLs
last, D4), then number ascending, then provider name ascending in the order
`azure_devops`, `github`, `gitlab`. `direct_commit` ref: among default-branch
snapshots sharing the greatest `created_at`, the lexicographically greatest
`head_commit`. Both orderings are total, so repeated evaluation selects the same
row — which is what makes the idempotency scenario testable.

**`evaluator.go`** — one pure function, no database and no git:

```go
func Classify(obs Observation) Classification
```

The spec's five rules in their exact order, first match wins. Pure means the
whole **Classification** and **Squash-merge** scenario set is a table-driven unit
test with no fixtures. `reached_default_at` is computed here too, from the
`provider_pr_merged` (merged PR whose base branch is the default branch),
`default_branch_commit` and `ancestor_of_default` bases; `push_webhook_default`
is defined in the vocabulary but never produced (spec, Out of scope).

When `DefaultBranch == ""`, rules 2 and the ancestry observation are skipped and
the basis records `default_branch_unknown`.

**`ancestry.go`** — at most one git call per pair per evaluation, 10-second
timeout:

```go
subproc.NewGitCommand(ctx, "-C", repo.LocalPath,
    "merge-base", "--is-ancestor", headCommit, defaultRef)
// run via subproc.RunGitClass(ctx, subproc.GitWorkClassBackground, cmd)
```

Production git must go through `subproc` with a work class
(`apps/backend/CLAUDE.md`); `background` is the correct class for a telemetry
sweep. **Only exit code 0 is evidence.** A non-zero exit, a missing checkout, an
absent object, or a timeout produces *nothing*: `reached_default_at` stays
`NULL`, the outcome is untouched, and `delivery_ledger_ancestry_errors_total`
increments on the error cases. This is not defensive coding — the spec's
finding 2 shows a negative ancestry result is a routine false negative under
squash-merge, receipted by PR #2514.

Skipped entirely when `repositories.local_path` is empty or
`repositories.default_branch` is empty.

**`sweep.go`** — the goroutine, following the repository's ownership rules
(single owner, `Start(ctx)` / `Stop()`, `sync.WaitGroup`, `time.NewTicker`
inside a `select` that also watches shutdown, idempotent on both ends,
restartable). 5-minute interval, no result cap. Plus bus subscribers for
`events.TaskSessionStateChanged`, `events.GitHubTaskPRUpdated` and
`events.GitLabTaskMRUpdated`, each resolving the affected pair and evaluating it
directly. `internal/delivery` gets `goleak.VerifyTestMain(m)`.

**`metrics.go`** — `expvar` maps published at package init, following
`internal/common/subproc/metrics.go`: `delivery_ledger_evaluations_total` (keyed
by outcome), `delivery_ledger_rows_written_total`,
`delivery_ledger_demotions_suppressed_total`,
`delivery_ledger_ancestry_errors_total`,
`delivery_ledger_sessions_unattributed_total`. Exposed through the existing
dev-mode `/debug/vars` handler; no new route.

Counters are necessary but not sufficient — they reset to zero on restart, so a
writer that died before the last restart reads healthy. `MaxLastEvaluatedAt`
is the signal that survives, compared against the most recent
`task_sessions` activity. Both are required.

**`provider.go`** — `Provide(writer, reader *sqlx.DB, bus events.Bus, log *logger.Logger)
(*Service, func() error, error)`, wired in `internal/backendapp` alongside the
other providers, cleanup calling `Stop()`.

### Office run outcome (`internal/office`)

**Schema.** `apps/backend/internal/office/repository/sqlite/base_migrations.go`:

```go
r.migrate.Apply("runs.outcome", `ALTER TABLE runs ADD COLUMN outcome TEXT`)
```

and `outcome TEXT` listed inline in the `runs` `CREATE TABLE`
(`base.go:301-…`) so fresh databases get it. Nullable, no default: pre-activation
rows read `NULL`. Activation point
`telemetry.run_outcome.activated_at` written once via
`persistence.WriteKeyIfAbsent`. **No backfill** — the spec is explicit that the
pre-activation series keeps its own bucket.

**Writer.** `FinishRun` gains an outcome parameter. The narrowest change that
keeps every existing caller honest: add
`FinishRunWithOutcome(ctx, id string, outcome models.RunOutcome) error` to
`internal/office/service/scheduler_runs.go` and
`internal/office/repository/sqlite`, and leave `FinishRun(ctx, id)` delegating
with `OutcomeProcessed`. The four scheduler call sites in
`internal/office/service/scheduler_integration.go`:

| Line | Outcome |
|---|---|
| `:218` agent not active | `agent_inactive` |
| `:247` idle skipped | `idle_skipped` |
| `:517` task-tree hold | `task_tree_held` |
| `:829` pre-execution budget block | `budget_blocked` |

`runs.status` is untouched: a blocked run still reaches `finished`, so every
existing reader of `status` keeps working. The two agent-completion callers
`event_subscribers.go:408` and `:512` keep `processed`. Checkout errors use the
normal retry or failure path, and checkout contention requeues the run; neither
path finishes a run or records completed work.

**Reader.** `internal/office/repository/sqlite/agent_summary.go:36` —
`RunCountsByDayForAgent` returns five buckets instead of three:

```sql
SUM(CASE WHEN status = 'finished' AND outcome = 'processed'  THEN 1 ELSE 0 END) AS succeeded,
SUM(CASE WHEN status = 'finished' AND outcome IS NOT NULL
                                  AND outcome <> 'processed' THEN 1 ELSE 0 END) AS skipped,
SUM(CASE WHEN status = 'finished' AND outcome IS NULL        THEN 1 ELSE 0 END) AS unclassified,
SUM(CASE WHEN status IN ('failed','timed_out')               THEN 1 ELSE 0 END) AS failed,
SUM(CASE WHEN status NOT IN ('finished','failed','timed_out') THEN 1 ELSE 0 END) AS other
```

`AgentRunDayRow` (`agent_summary.go:13`) gains `Skipped` and `Unclassified`.
Legacy rows land in `unclassified`, never `succeeded` — that is the whole point:
the discontinuity is visible in the data rather than silent.

**DTO.** `internal/office/dashboard/agent_summary.go` — `AgentRunActivityDay`
(`:44`) gains `skipped` and `unclassified`; `padAgentRunActivity` (`:306`) and
`buildSuccessRate` (`:366`) both change `Total` to
`Succeeded + Skipped + Unclassified + Failed + Other`. `AgentSuccessRateDay`
keeps its `succeeded`/`total` pair, so the success rate now correctly *falls*
when blocked runs stop counting as successes. That is the fix, not a regression.

---

## Frontend

The ledger adds no frontend surface. The Office run outcome does: two existing
charts read the response shape that changed.

### `apps/web/lib/state/slices/office/types.ts`

The run-activity day type (around `:308`) gains `skipped: number` and
`unclassified: number`.

### `apps/web/app/office/agents/[id]/dashboard/components/run-activity-chart.tsx`

The stacked bar gains two segments between `succeeded` and `failed`: `skipped`
and `unclassified`, with the legend extended to match. `total` already comes
from the backend, so the existing proportional math is unchanged.

### `apps/web/app/office/agents/[id]/dashboard/components/success-rate-chart.tsx`

No shape change — it reads `succeeded` and `total`, both of which still exist.
Verify the rendered percentage against the new `total`; no code change is
expected here beyond the test.

### i18n

Two new keys in the `office` namespace for the new legend labels, following the
existing `t("office:succeeded")` call at `run-activity-chart.tsx:52`. Both go
through `t()`; neither is compared with `===`; neither is called at module
scope. Add the English and pseudo catalog entries in the same change, and use
plain punctuation (no U+2014).

---

## Tests

### Backend — shared primitives

- **What:** `IsMissingTableError` classifies SQLite "no such table" and Postgres
  42P01, and rejects unrelated errors.
  **File:** `apps/backend/internal/db/errors_test.go`
  **How:** table-driven, alongside the existing duplicate-column cases.
- **What:** `WriteKeyIfAbsent` writes once and is a no-op on replay, returning
  `false` and leaving the original value.
  **File:** `apps/backend/internal/persistence/meta_test.go`
  **How:** real SQLite temp DB; call twice, assert value and returned bool.

### Backend — ledger

- **What:** all five classification rules in order, first-match-wins, including
  the `default_branch_unknown` fallthrough and the rule-4/rule-5 split
  (snapshots-all-empty vs no observations at all).
  **File:** `apps/backend/internal/delivery/evaluator_test.go`
  **How:** table-driven over `Observation` literals; no DB, no git. One case per
  Classification scenario in the spec.
- **What:** a negative ancestry result sets no column and does not change the
  outcome; a missing checkout increments the error counter and leaves
  `reached_default_at` NULL.
  **File:** `apps/backend/internal/delivery/ancestry_test.go`
  **How:** real `git init` fixture under `t.TempDir()` for the positive case;
  a temp path with no repository for the error case. Channel-based
  synchronization, not sleeps.
- **What:** migration applies to a database seeded before the feature, every new
  column reads NULL on the pre-existing row, no pre-existing value changes, and
  `runMigrations` called **twice** is clean; the activation point retains its
  first-boot value across the replay.
  **File:** `apps/backend/internal/delivery/store_migration_test.go`
  **How:** mirrors
  `internal/task/repository/sqlite/task_external_id_migration_test.go` — open a
  temp SQLite DB, construct, seed, assert NULLs, construct again, re-assert.
- **What:** the same fresh-then-replay matrix on Postgres, with duplicate-object
  errors classified through `internal/db`.
  **File:** `apps/backend/internal/delivery/store_postgres_test.go`
  **How:** env-gated on `KANDEV_TEST_POSTGRES_DSN`, per ADR 0027.
- **What:** promotion only. A stored `pr_merge` survives an evaluator that
  computes `unknown`, and `delivery_ledger_demotions_suppressed_total`
  increments.
  **File:** `apps/backend/internal/delivery/store_lattice_test.go`
  **How:** real SQLite; upsert `pr_merge`, upsert `unknown`, read back.
- **What:** two evaluators racing on one pair converge on `pr_merge` in either
  commit order.
  **File:** `apps/backend/internal/delivery/store_lattice_test.go`
  **How:** run both orders explicitly; assert the same terminal row. The lattice
  is in the SQL, so this is a real test of the statement, not of a Go mutex.
- **What:** re-evaluating unchanged inputs leaves all classification and
  observation columns byte-identical and `updated_at` unchanged, while
  `last_evaluated_at` and `evaluation_seq` advance.
  **File:** `apps/backend/internal/delivery/store_idempotency_test.go`
  **How:** real SQLite; evaluate twice; compare every column.
- **What:** `reached_default_at` is write-once — a later observation through a
  different basis changes neither it nor `reached_default_basis`.
  **File:** `apps/backend/internal/delivery/store_idempotency_test.go`
- **What:** `delivery_ref` tiebreaks: earliest `merged_at` wins; on an exact tie
  the lower number wins; Azure rows (no `merged_at`) sort last; two
  default-branch snapshots sharing `created_at` resolve to the
  lexicographically greatest `head_commit`, stably across repeated evaluation.
  **File:** `apps/backend/internal/delivery/observe_test.go`
  **How:** real SQLite with all three provider tables created.
- **What:** a GitLab merged MR classifies `pr_merge` on the same basis as a
  GitHub PR; a detached GitHub PR does not; an absent provider table yields zero
  rows instead of an error.
  **File:** `apps/backend/internal/delivery/observe_test.go`
  **How:** real SQLite; the missing-table case simply omits the `CREATE TABLE`.
- **What:** end-to-end — candidate discovery through classification to a
  persisted row, covering a task with no repository writing no row, and a
  session with an empty `repository_id` counting as unattributed.
  **File:** `apps/backend/internal/delivery/service_test.go`
  **How:** integration test over a real SQLite database with task, session,
  snapshot and provider rows seeded.
- **What:** the sweep starts, evaluates, and stops without leaking; `Stop` then
  `Start` restarts cleanly.
  **File:** `apps/backend/internal/delivery/sweep_test.go`
  **How:** `testing/synctest` for the ticker; `goleak.VerifyTestMain(m)` in a
  package `TestMain`.
- **What:** the stall signal reports the true last write instant with counters at
  zero (the post-restart case).
  **File:** `apps/backend/internal/delivery/sweep_test.go`
  **How:** write a row, construct a fresh `Service` against the same DB, assert
  `MaxLastEvaluatedAt` without re-evaluating.

### Backend — Office run outcome

- **What:** each of the six active `FinishRun` paths writes its documented outcome
  while `status` stays `finished`.
  **File:** `apps/backend/internal/office/service/scheduler_integration_test.go`
  **How:** drive each path and read the `runs` row. The agent-inactive path is
  explicitly included even though it writes no `office_activity_log` row — that
  gap is why the activity log cannot carry this.
- **What:** `runs.outcome` migration applies to a pre-existing database, reads
  NULL on old rows, and replays cleanly.
  **File:** `apps/backend/internal/office/repository/sqlite/runs_outcome_migration_test.go`
  **How:** the `task_external_id_migration_test.go` pattern; new file, not an
  append, per the 800-effective-line file limit.
- **What:** the five-bucket day rollup — one processed, one budget-blocked, one
  failed and one pre-activation NULL row return
  `succeeded=1, skipped=1, failed=1, unclassified=1`.
  **File:** `apps/backend/internal/office/repository/sqlite/agent_summary_test.go`
  **How:** real SQLite, seeded runs; this is the spec's final Office scenario.
- **What:** `padAgentRunActivity` and `buildSuccessRate` compute
  `total` over all five buckets.
  **File:** `apps/backend/internal/office/dashboard/agent_summary_test.go`
  **How:** table-driven over `AgentRunDayRow` literals.

### Frontend

- **What:** the run-activity bar renders five segments and the legend lists the
  new buckets.
  **File:** `apps/web/app/office/agents/[id]/dashboard/components/run-activity-chart.test.tsx`
  **How:** render with a fixture day; assert segment values and legend labels.

---

## E2E Tests

- **Scenario:** GIVEN an Office agent with runs on a day, WHEN the agent
  dashboard is opened, THEN the Run Activity legend shows the succeeded,
  skipped, unclassified, failed and other buckets.
  **File:** `apps/web/e2e/tests/office/agent-dashboard.spec.ts` (extend the
  existing spec; do not add a new file)
  **What to verify:** the legend labels are visible and the chart renders
  without error against a seeded agent.

No E2E is added for the ledger: it has no user-visible surface (spec, Out of
scope).

---

## Verification Results

Pending. On completion, synchronize this section with each task's `## Results`:
record exact commands and outcomes/counts, generated artifact paths, and
cleanup/teardown evidence.

---

## Implementation Waves And Parallel Candidates

The default is sequential execution in the primary conversation. Waves expose
possible parallelism only; they do not authorize subagents.

```
Wave 1 (parallel candidates — disjoint files — user authorization required):
- [ ] [task-01-shared-primitives](task-01-shared-primitives.md)
- [ ] [task-07-run-outcome-backend](task-07-run-outcome-backend.md)

Wave 2:
- [ ] [task-02-ledger-store](task-02-ledger-store.md)

Wave 3 (parallel candidates — disjoint files within internal/delivery):
- [ ] [task-03-evaluator](task-03-evaluator.md)
- [ ] [task-04-observations](task-04-observations.md)

Wave 4 (parallel candidates — disjoint files):
- [ ] [task-05-ancestry](task-05-ancestry.md)
- [ ] [task-08-run-outcome-frontend](task-08-run-outcome-frontend.md)

Wave 5:
- [ ] [task-06-sweep-and-health](task-06-sweep-and-health.md)

Wave 6:
- [ ] [task-09-e2e-agent-dashboard](task-09-e2e-agent-dashboard.md)
```

Task 07 touches only `internal/office/**` and task 01 only
`internal/db` + `internal/persistence`, so wave 1 is genuinely disjoint. Tasks 03
and 04 both live in `internal/delivery` but write different files and both
consume the types task 02 defines; neither edits a shared schema or migration.

---

## Open Questions

1. **Snapshot-write trigger (D6).** The spec lists "a git snapshot is written"
   as an evaluation trigger, but no such event type exists and adding one puts a
   publish on the snapshot write path. This plan serves that trigger through the
   sweep's freshness predicate, bounding latency at one 5-minute interval. If
   sub-interval latency on snapshot writes is actually required, that is a new
   `task_session.git_snapshot.created` event type and a task 06 subscriber —
   confirm before implementing.
