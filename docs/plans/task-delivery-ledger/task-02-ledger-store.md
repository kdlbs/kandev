---
id: "02-ledger-store"
title: "Delivery ledger table, migration and lattice upsert"
status: done
wave: 2
depends_on: ["01-shared-primitives"]
plan: "plan.md"
spec: "../../specs/task-delivery-ledger/spec.md"
---

# Task 02: Delivery ledger table, migration and lattice upsert

Create the `internal/delivery` package and everything that touches persistence:
the vocabularies, the table, its replay-safe migration, its activation point, and
the single upsert statement that enforces the promotion lattice.

Do **not** implement classification, evidence queries, ancestry or the sweep here
— they are tasks 03-06 and depend on the types this task defines.

## What to build

**`models.go`** — the four `Outcome` constants, the seven `delivery_basis`
constants and the four `reached_default_basis` constants exactly as the spec's
**Basis vocabulary** tables, plus `rank(Outcome) int` implementing the lattice
(`"" -> 0, unknown -> 1, no_delivery_observed -> 2, direct_commit -> 3,
pr_merge -> 4`) and a `LedgerRow` struct with pointer/`sql.Null*` fields for
every nullable column so a `NULL` round-trips as `nil`, never as `0` or `""`.

**`store.go`** — the DDL in the plan's **`store.go`** section, applied through
`db.MigrateLogger`. Every classification and observation column is nullable with
**no** `DEFAULT`. A `DEFAULT 0` on `observed_branch_commits` or a `DEFAULT ''` on
a basis column would make "never evaluated" read identically to "evaluated and
found nothing", which is the exact discontinuity the spec forbids.

Immediately after the DDL, write the activation point once:

```go
persistence.WriteKeyIfAbsent(writer, "telemetry.delivery_ledger.activated_at",
    time.Now().UTC().Format(time.RFC3339))
```

**The lattice upsert.** One `INSERT ... ON CONFLICT (task_id, repository_id) DO
UPDATE` statement, per the plan. The rank comparison lives in the SQL `SET`
clause, never in a Go read-then-write: two evaluators racing on the same pair
must converge on the higher-ranked outcome regardless of arrival order.

Points that are easy to get wrong and are individually tested:

- `delivery_basis` uses `>=` where outcome and ref use `>`, so a re-evaluation at
  the same outcome can refine the basis. The spec's fifth Classification
  scenario (`branch_commits_unmerged` becoming `reached_default_unattributed`
  once ancestry lands) depends on this and on nothing else.
- `reached_default_at` / `_basis` / `_ref` are write-once: `COALESCE` the
  timestamp and gate the other two on the *stored* timestamp being NULL.
- `updated_at` advances only when a classification or observation column actually
  changes. `last_evaluated_at` and `evaluation_seq` advance on every persisted
  evaluation including a no-change one.
- `MAX(a, b)` is SQLite's two-argument scalar form; Postgres needs `GREATEST`.
  Select through the existing `internal/db` dialect seam, not a driver-name
  string comparison in this package.

Also expose `MaxLastEvaluatedAt(ctx) (time.Time, error)` for the stall signal
(task 06 consumes it) and an `UpsertResult` carrying whether a demotion was
suppressed, so task 06 can increment the counter without re-reading the row.

- **Acceptance:**
  1. Against a database seeded before this feature, the table and index exist,
     every new column reads `NULL` on a pre-existing row, and no pre-existing
     value changes.
  2. Running the migration twice applies cleanly the second time and
     `telemetry.delivery_ledger.activated_at` keeps its first-boot value.
  3. A stored `pr_merge` survives an upsert computing `unknown`, in either
     commit order, and the result reports a suppressed demotion.
  4. Re-upserting unchanged inputs leaves every classification and observation
     column byte-identical and `updated_at` unchanged, while `last_evaluated_at`
     and `evaluation_seq` advance.
  5. `reached_default_at` set through one basis is not overwritten by a later
     observation through a different basis.

- **Verification:**
  `cd apps/backend && go test ./internal/delivery/... && KANDEV_TEST_POSTGRES_DSN="$KANDEV_TEST_POSTGRES_DSN" go test -run Postgres ./internal/delivery/... && make lint`

  (The Postgres leg is env-gated per ADR 0027; if the DSN is unset, record that
  explicitly in Results rather than reporting the leg as passed.)

- **Files likely touched:**
  - `apps/backend/internal/delivery/models.go`
  - `apps/backend/internal/delivery/models_test.go`
  - `apps/backend/internal/delivery/store.go`
  - `apps/backend/internal/delivery/store_migration_test.go`
  - `apps/backend/internal/delivery/store_lattice_test.go`
  - `apps/backend/internal/delivery/store_idempotency_test.go`
  - `apps/backend/internal/delivery/store_postgres_test.go`

- **Dependencies:** Task 01 (`persistence.WriteKeyIfAbsent`).

- **Parallelism:** sequential. It creates the package every later ledger task
  imports.

- **Inputs:** Spec **Data model**, **Basis vocabulary**, **Activation points**,
  **Persistence guarantees**, **Ordering, idempotency, concurrency**, and the
  **Migration and activation** + **Idempotency, ordering and concurrency**
  scenarios; plan decisions D1 and D2;
  `docs/decisions/0027-replayable-schema-migrations.md`;
  `apps/backend/internal/task/repository/sqlite/task_external_id_migration_test.go`
  as the migration-test pattern; `apps/backend/CLAUDE.md` "Schema & migrations".

- **Output contract:** summary, files changed, tests run with counts, whether the
  Postgres leg ran, blockers, risks, and task/plan status update in the same
  conversation.

## Results

**Files changed:** `internal/delivery/models.go`, `internal/delivery/store.go`,
plus migration/lattice/idempotency test files under `internal/delivery/`
(`store_test.go`, `upsert_test.go`, and siblings — organized by concern rather
than exactly the `store_migration_test.go` / `store_lattice_test.go` /
`store_idempotency_test.go` split named in the plan).

**Commands run:**
- `cd apps/backend && go test ./internal/delivery/...` → `ok`, 99 subtests
  pass, 0 fail, 3.9s (full package run, not just this task's scope — the
  package now also contains tasks 03-06's code).
- `KANDEV_TEST_POSTGRES_DSN=... go test -run Postgres ./internal/delivery/...`
  — **not run**. This sandbox has no reachable Postgres instance:
  `psql` against the local Postgres.app rejects trust auth (requires an
  interactive GUI permission grant unavailable here), and `docker run
  postgres:16` fails with a broken Docker daemon
  (`error creating temporary lease: ... input/output error`). The env-gated
  tests (`TestPostgresDeliveryLedgerMigration_FreshAndReplay`,
  `TestPostgresUpsert_RankGuardedBehavior`) compile and `t.Skip` cleanly
  without the DSN; they have not been run against a live Postgres in this
  environment. Per ADR 0027 this is recorded as not run, not passed.
- `make lint` — clean.

**Acceptance verification:** #1 (pre-existing DB, NULL columns) and #2
(replay-safe migration + activation instant) are covered by the SQLite
migration tests; #3 (suppressed demotion, either commit order) and #4
(unchanged-input re-upsert leaves classification/observation columns
byte-identical, `updated_at` frozen, `last_evaluated_at`/`evaluation_seq`
advance) and #5 (write-once `reached_default_at` across bases) are each
covered by dedicated `upsert_test.go` cases exercised in the 99-subtest run
above.

**Security/trust and external side-effects:** None — new table only, no
network calls, no secrets. The rank comparison lives entirely in the SQL
`SET` clause (per the plan), so no read-then-write race exists in Go.
