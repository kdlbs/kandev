---
id: "05-coordinator-install-idempotency"
title: "Match the coordinator routine on identity, not mutable trigger state"
status: done
wave: 2
depends_on: ["01-classify-schedule-state"]
plan: "plan.md"
spec: "../../specs/office/requirements/coordinator-install-idempotency.md"
---

# Task 05: Match the coordinator routine on identity, not mutable trigger state

Satisfies REQ-OFFICE-COORDINATOR-INSTALL-001 (AC-OFFICE-COORDINATOR-INSTALL-001.1
through .15).

## Scope

Redefine coordinator-routine identity as workspace + assignee agent +
canonical routine name only, complete a half-finished install (routine with
no cron trigger) instead of duplicating it, handle the case where more than
one existing routine already matches that identity, and serialize
check-and-create against concurrent installs across processes sharing one
database.

## Exclusions

- Does not change the schedule-state definition (Task 01) — only reports it
  (AC-OFFICE-COORDINATOR-INSTALL-001.5).
- Does not merge, delete, or otherwise touch duplicate routines created by a
  prior buggy install beyond selecting one to complete
  (AC-OFFICE-COORDINATOR-INSTALL-001.3).
- Does not add `enabled` to the identity match — forbidden by
  AC-OFFICE-COORDINATOR-INSTALL-001.1, and explicitly the trap the source
  card's framing invites.
- Does not add a database uniqueness constraint over the identity columns
  (AC-OFFICE-COORDINATOR-INSTALL-001.9 forbids it; upgraded installs already
  violate it).

## Acceptance conditions

1. `findCoordinatorRoutine` matches on workspace + assignee + canonical name
   only — no trigger presence, expression, `enabled`, or `next_run_at` term
   (AC-001.1). A single match: return it, create nothing
   (AC-001.2). No cron trigger on the match: create the canonical cron
   trigger (canonical expression, canonical timezone, `enabled = true`,
   `next_run_at` = first canonical occurrence strictly after one instant
   captured once at install start) and return the existing routine; if that
   next-occurrence computation fails, reject and report without touching
   the routine (it is a defect in the installer's own constants, unreachable
   by any input); if trigger creation itself fails, reject, report, and
   leave the routine in place so a later install reaches this same branch
   (AC-001.4). Any cron trigger already present, canonical or not, enabled
   or not: leave every trigger unchanged, create no second one, report
   schedule state (AC-001.5). Never change `status` on an existing match
   (AC-001.6). Empty workspace, assignee, or canonical name: reject before
   the identity lookup, reported distinguishably from a read failure
   (AC-001.14). No install-call ever retries a failure internally — each
   rejection returns to the caller as an error, optionally alongside the
   routine found or created, matching what both callers already do today
   (AC-001.15).
2. More than one existing routine matches the identity: select the one with
   the earliest `office_routines.created_at`, ties by `id` ascending;
   delete none; report that more than one was found; then run the selected
   routine through the same .4/.5 branch a single match would take
   (AC-001.3). No match at all: create one routine with that identity and
   `status = 'active'` plus its canonical trigger; a routine-insert failure
   rejects and creates no trigger; a trigger-creation failure after a
   successful routine insert leaves the routine in place and reports the
   failure so a later install completes it via AC-001.4 (AC-001.12). An
   identity-lookup failure, or a failure reading the existing routine's
   triggers (the read that decides AC-001.4 vs. AC-001.5), rejects and
   creates nothing — never treated as "absent" (AC-001.7).
3. Two concurrent installs for the same workspace and assignee create at
   most one new coordinator routine and at most one new canonical trigger
   between them, on both SQLite and PostgreSQL, via a serialization
   mechanism whose domain is every process sharing the database — not an
   in-process guard alone (AC-001.8, AC-001.9, AC-001.11). Acquisition never
   blocks indefinitely: it acquires or fails within a single named
   compile-time 5-second bound, not configurable by environment, config, or
   flag (AC-001.9). Failure to acquire, or losing a serialized section to a
   concurrent install, rejects, creates nothing, and reports contention
   distinguishably from AC-001.7's failures and is not retried within the
   call (AC-001.13). Context cancellation while waiting likewise rejects and
   creates nothing, reported distinguishably from both contention and
   AC-001.7 (AC-001.13).
4. Every condition above that must be reported (AC-001.3's duplicate count
   and selection, AC-001.5's schedule state) is a structured log record plus
   a counter per condition — never an additional return value, since both
   production callers discard everything but the error today (AC-001.10).
   Each record names workspace, assignee, and the routine it concerns.
   Counters count calls in which a condition was *observed*, including
   calls that are ultimately rejected. The whole requirement behaves
   identically on SQLite and PostgreSQL (AC-001.11).

## Design constraint for the serialization mechanism (AC-001.9, AC-001.13)

Pick a mechanism satisfying: (a) blocks/serializes across processes sharing
one database, not merely goroutines in one process; (b) works identically on
SQLite and PostgreSQL; (c) bounds acquisition at a hard 5s; (d) distinguishes
"could not acquire" (contention) from "acquired, then lost the race inside
the critical section" from context cancellation. The requirement's own
`## Out of scope` names three mechanisms that individually fail one of
these and explicitly forbids treating them as interchangeable: a bare
default-isolation-level transaction (does not stop two callers both reading
"absent"), a PostgreSQL advisory lock (no SQLite equivalent), an in-process
mutex (fails the cross-process domain). A row-level lock strategy
(`SELECT ... FOR UPDATE`-equivalent per dialect, or a small dedicated lock
row keyed by workspace+assignee with a bounded wait) is a plausible
candidate Build should evaluate against all four constraints before
building it.

**If the chosen mechanism adds a new table or columns**, add it to
`internal/persistence/requiredstores` and
`internal/persistence/storeconformance` per the backend schema-owner rule,
and add the fresh/replay conformance tests that requires — this is a risk to
flag during implementation, not a decision to make in this work order.

## Verification

- `cd apps/backend && go test -tags fts5 ./internal/office/routines/... ./internal/office/agents/... ./internal/office/onboarding/...`
- Test: identity match ignores an edited or absent trigger (regression for
  the original card's bug — a routine with a non-canonical or deleted
  trigger still matches).
- Test: install against a triggerless existing match creates exactly the
  canonical trigger and returns the existing routine (AC-001.4), and running
  it again is a no-op via AC-001.5.
- Test: two or more matching routines — selects earliest `created_at`/`id`,
  deletes none, reports the count, and still completes the selected one's
  schedule (AC-001.3).
- Test: concurrent-install race — two goroutines (or two DB connections
  simulating two processes) racing `CreateDefaultCoordinatorRoutine` for the
  same identity produce exactly one routine and one trigger; run against
  both SQLite and `KANDEV_TEST_POSTGRES_DSN` PostgreSQL.
- Test: empty workspace/assignee/canonical-name is rejected before any read
  (AC-001.14).
- Test: contention and context-cancellation rejections are distinguishable
  from each other and from AC-001.7's failures in the returned error/report.
- If a new schema owner was added: `go run ./cmd/sqlguard ./internal` and
  `go test -race ./internal/persistence/storeconformance -count=1`.

## Files likely touched

- `apps/backend/internal/office/routines/service.go` —
  `CreateDefaultCoordinatorRoutine`, `findCoordinatorRoutine`,
  `hasCronTrigger` (L145-239).
- `apps/backend/internal/office/repository/sqlite/routines.go` — new
  identity-lookup query and whatever serialization primitive is chosen.
- new: `apps/backend/internal/office/routines/coordinator_install_metrics.go`
  (structured records + counters, following the same `expvar.NewMap`
  convention as Task 04).
- `apps/backend/internal/office/agents/service.go`,
  `apps/backend/internal/office/onboarding/service.go` — no behavior change
  expected (both already discard the return value except the error), but
  their tests should assert the discard contract still holds.
- Corresponding `*_test.go` files, plus a PostgreSQL-gated concurrency test.

## Dependencies

Task 01 (this task reports Task 01's schedule state via AC-001.5; the
requirement document states it "cannot ship before that one").

## Parallelism

Independent of Task 02 and Task 04 once Task 01 lands — no shared files
with either.

## Result

Implemented as two separately-locked, separately-committed phases rather
than one transaction, after TDD caught a real defect in the first cut: a
single all-or-nothing transaction covering both routine-creation and
trigger-creation would roll back an already-decided routine on a
trigger-creation failure, violating AC-001.12's "leaves the routine in
place" requirement. Splitting into two phases (each its own lock + tx)
fixes this — a trigger-creation failure can now only abort the trigger's
own transaction, never the routine's already-committed one.

**Serialization mechanism** (resolves the work order's open design
question): a real database transaction on both dialects, holding, on
PostgreSQL only, a `pg_try_advisory_xact_lock` polled every 25ms inside a
context bounded to a hard 5s (`coordinatorInstallLockTimeout`, not
configurable by env/config/flag). On SQLite the existing single
writer-connection pool (`internal/db.OpenSQLite`'s `SetMaxOpenConns(1)`)
already serializes writers process-locally; PostgreSQL needs the explicit
advisory lock because a plain transaction under READ COMMITTED does not
stop two callers from both observing "absent". This combines (rather than
picks one of) the three mechanisms the requirement's Out-of-scope section
individually rules out.

Files:
- `apps/backend/internal/office/models/coordinator_install.go` (new) —
  `models.ErrCoordinatorInstallContention` sentinel and the
  `models.CoordinatorInstallTx` interface, placed in the shared `models`
  package (rather than in `sqlite` or `routines`) so neither package has to
  import the other.
- `apps/backend/internal/office/repository/sqlite/coordinator_install.go`
  (new) — `WithCoordinatorInstallLock` (generic lock-key + tx runner),
  `CoordinatorInstallLockKey` (workspace+assignee+name), the separate
  `CoordinatorInstallTriggerLockKey` (routine id), `InstallCoordinatorRoutine`
  (routine-existence phase), `EnsureCoordinatorTrigger` (trigger-existence
  phase, its own lock/tx), `acquirePostgresAdvisoryLock`,
  `classifyCoordinatorInstallWaitErr`.
- `apps/backend/internal/office/repository/sqlite/routines.go` — added
  `CreateRoutineTriggerTx`, refactored `CreateRoutineTrigger` to share an
  `insertRoutineTrigger` helper.
- `apps/backend/internal/office/routines/coordinator_install_metrics.go`
  (new) — `coordinatorInstallConditionsTotal` expvar map, 12 condition
  constants, `coordinatorInstallObserved`.
- `apps/backend/internal/office/routines/service.go` — `Repository`
  interface gained `InstallCoordinatorRoutine` and
  `EnsureCoordinatorTrigger`. `CreateDefaultCoordinatorRoutine` now runs
  `ensureCoordinatorRoutine` (AC-001.2/.3/.12's routine half) then
  `EnsureCoordinatorTrigger`'s decide callback (AC-001.4/.5/.12's trigger
  half, via `decideCoordinatorTrigger`), each reported through the shared
  `reportCoordinatorLockFailure` (contention / cancelled / a
  caller-supplied fallback condition for the read-before-decide failing).

Tests (all new):
- `apps/backend/internal/office/routines/coordinator_install_test.go`
  (internal `package routines`, for `coordinatorInstallConditionsTotal`
  access) — empty identity (AC-001.14), no-match create (AC-001.12),
  single match with any cron trigger unchanged (AC-001.2/.5), no-trigger
  match completes install (AC-001.4) plus re-entrancy, duplicate matches
  select earliest and leave the rest untouched (AC-001.3), status never
  changes (AC-001.6), identity-lookup and trigger-read failures rejected
  and distinguishable (AC-001.7), routine-insert failure creates no
  trigger, trigger-create failure leaves the routine in place and a
  follow-up call completes it (AC-001.12), contention vs. cancellation vs.
  lookup-failure are counted on distinct counters (AC-001.13), and a real
  two-goroutine race against one shared SQLite writer connection creates at
  most one routine and one trigger (AC-001.8/.9).
- `apps/backend/internal/office/routines/coordinator_install_dialect_postgres_test.go`
  (external `package routines_test`, `testutil.PostgresDSNFromEnv` gated,
  skips without `KANDEV_TEST_POSTGRES_DSN`) — fresh install + idempotent
  repeat produce the same shape on SQLite and PostgreSQL (AC-001.11), and a
  genuine concurrent-install race against PostgreSQL (where the advisory
  lock actually runs) creates at most one routine and trigger (AC-001.8/.9).

No new database table or column was introduced, so
`internal/persistence/requiredstores`/`storeconformance` and `sqlguard`
were not needed.

Verification: `go build -tags fts5 ./...` clean; `gofmt -l` clean on every
touched/new file; `go vet -tags fts5 ./internal/office/...` clean;
`golangci-lint run ./internal/office/models/... ./internal/office/repository/sqlite/... ./internal/office/routines/... --new-from-rev=<merge-base> --timeout=5m`
→ 0 issues; `go test -tags fts5 -race -count=1 ./internal/office/routines/... ./internal/office/agents/... ./internal/office/onboarding/... ./internal/office/wakeup/...`
all pass, including the two pre-existing coordinator-install tests
(`TestCreateDefaultCoordinatorRoutine_Idempotent`,
`TestRoutine_EndToEnd_CoordinatorHeartbeatFire`) unchanged. The new
PostgreSQL-gated tests skip locally (`KANDEV_TEST_POSTGRES_DSN` unset). The
one pre-existing, unrelated failure (`TestMigrate_PriorityIdempotent` in
`internal/office/repository/sqlite`) is confirmed present before this
task's changes and out of scope.
