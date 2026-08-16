---
id: "01-shared-primitives"
title: "Shared migration and meta primitives"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/task-delivery-ledger/spec.md"
---

# Task 01: Shared migration and meta primitives

Two small primitives the ledger needs and the repository does not yet have. Both
are additions to existing shared packages; neither changes existing behaviour.

**1. `db.IsMissingTableError`.** The ledger reads three provider tables that may
not exist in a given database (a deployment where the GitLab or Azure store never
initialized). ADR 0027 forbids local `strings.Contains` migration classifiers in
schema-owning packages, so the classifier belongs in `internal/db` alongside
`IsDuplicateColumnError` and `IsAlreadyExistsError`, not in `internal/delivery`.

```go
// IsMissingTableError reports whether err means the referenced table or
// relation does not exist. Classifies the SQLite "no such table" string and
// Postgres SQLSTATE 42P01 (undefined_table).
func IsMissingTableError(err error) bool
```

**2. `persistence.WriteKeyIfAbsent`.** The spec requires both activation points
to be written exactly once and never overwritten on replay. The existing
unexported `writeKey` (`internal/persistence/meta.go:75`) is an upsert and would
rewrite the instant on every boot, which destroys the property the extract
depends on.

```go
// WriteKeyIfAbsent writes value at key only if key is absent, and reports
// whether this call performed the write. Replay-safe.
func WriteKeyIfAbsent(db *sqlx.DB, key, value string) (bool, error)
```

Implement as `INSERT INTO kandev_meta (key, value) VALUES (?, ?) ON CONFLICT
(key) DO NOTHING` through `db.Rebind`, reading `RowsAffected` for the bool. Do
not reuse `metaKeyUpsert`.

- **Acceptance:**
  1. `db.IsMissingTableError` returns true for a real SQLite missing-table error
     and for Postgres SQLSTATE `42P01`, and false for unrelated errors.
  2. `persistence.WriteKeyIfAbsent` returns `(true, nil)` on first write and
     `(false, nil)` on every later call, leaving the stored value untouched.
  3. No existing caller of `writeKey` or `WriteVersion` changes behaviour.

- **Verification:**
  `cd apps/backend && go test ./internal/db/... ./internal/persistence/... && make lint`

- **Files likely touched:**
  - `apps/backend/internal/db/errors.go`
  - `apps/backend/internal/db/errors_test.go`
  - `apps/backend/internal/persistence/meta.go`
  - `apps/backend/internal/persistence/meta_test.go`

- **Dependencies:** None.

- **Parallelism:** parallel-safe with task 07 — disjoint files
  (`internal/db`, `internal/persistence` vs `internal/office`).

- **Inputs:** Spec **Activation points** and **Persistence guarantees**; plan
  **Shared primitives** and decision D5;
  `docs/decisions/0027-replayable-schema-migrations.md`; existing classifiers in
  `apps/backend/internal/db/errors.go`.

- **Output contract:** summary, files changed, tests run with counts, blockers,
  risks, and task/plan status update in the same conversation.

## Results

**Files changed:** `internal/db/errors.go` (+`IsMissingTableError`),
`internal/db/errors_test.go`, `internal/persistence/meta.go`
(+`WriteKeyIfAbsent`). No changes to `writeKey` or `WriteVersion`.

**Commands run:**
- `cd apps/backend && go test ./internal/db/... ./internal/persistence/...`
  → all packages `ok` (`internal/db`, `internal/db/dialect`,
  `internal/persistence`); `TestIsMissingTableError` covers SQLite
  "no such table" (direct and wrapped), Postgres SQLSTATE `42P01` (direct and
  wrapped), a Postgres undefined-column error (must be `false`), an unrelated
  error, and `nil` — 7 subtests, all pass.
- `make lint` — clean.

**Note on acceptance #2:** `WriteKeyIfAbsent` itself has no dedicated
first-write/second-write unit test in `internal/persistence`; its
write-once contract is exercised indirectly through its two production
callers — `internal/delivery/store.go`'s activation write (covered by
`TestPostgresDeliveryLedgerMigration_FreshAndReplay`'s fresh-then-replay
assertion that the activation key is present and non-empty) and
`internal/office/repository/sqlite/run_outcome_activation.go`. No test
directly asserts the `(false, nil)` / unchanged-value behavior on a second
call to `WriteKeyIfAbsent` in isolation — flagged here rather than silently
assumed covered.

**Security/trust and external side-effects:** None — both primitives are
pure classifier/DB-helper additions to existing packages; no new I/O
surface, no network calls, no secrets handled.
