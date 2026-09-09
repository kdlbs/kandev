---
status: current
system: office
requirements:
  - REQ-OFFICE-RUN-HISTORY-RETENTION-001
  - REQ-OFFICE-RUN-HISTORY-RETENTION-002
  - REQ-OFFICE-RUN-HISTORY-RETENTION-005
---

# Office Run History Retention System Design

## Purpose and boundaries

This design adds one scheduled sweep that deletes aged Office run history from
five tables. It changes no run lifecycle, no routine dispatch decision, and no
task, session, or checkout.

Office owns the sweep because the eligibility rules are defined by Office
primitives. The settings record, the health check, the reporting value, and the
System page surface are the operator half of the same capability and are
designed in [run history retention
operations](run-history-retention-operations.md), which is where every
requirement in REQ-OFFICE-RUN-HISTORY-RETENTION-003 and -004 is satisfied.

Adjacent contracts read and constrained but not owned:

- `internal/health` — `Checker`, `Issue`, and the `/api/v1/system/health`
  response the System page's health card renders.
- `internal/system/settings.Store` — the key/value settings table, already used
  by storage maintenance under one JSON key with normalization on read.
- `internal/runs/repository/sqlite` — the `runs` queue and `run_events` access
  methods.
- `internal/office/repository/sqlite` — the schema owner for `runs`,
  `run_events`, `office_run_route_attempts`, `office_run_skills`, and
  `office_routine_runs`.

## Measured starting state

Read from the reference install's SQLite database on 2026-09-09. These numbers
set the defaults and are the baseline any regression test can be written
against.

| Table | Rows | Notes |
|---|---|---|
| `office_routine_runs` | 326 | 323 `coalesced`, 3 `task_created` |
| `runs` | 53 | |
| `run_events` | 340 | across 53 runs, mean 6.4 per run |
| `office_run_route_attempts` | 55 | |
| `office_run_skills` | 382 | |

The 323 `coalesced` rows span 2026-08-03 to 2026-08-31 and belong to a single
routine that is now `paused`. Orphan `run_events` today: zero, because nothing
has ever deleted a `runs` row.

## Schema facts this design depends on

Verified by reading the schema owners, not assumed.

- `office_routine_runs` (`office/repository/sqlite/base.go`) has
  `FOREIGN KEY (routine_id) REFERENCES office_routines(id) ON DELETE CASCADE`
  on both engines, and SQLite opens with `_foreign_keys=on`
  (`internal/db/sqlite.go`). Its two indexes are partial and serve the dispatch
  gate, not an age scan: `idx_office_routine_runs_active_fingerprint`
  (`WHERE status = 'task_created'`) and `idx_office_routine_runs_linked_task`
  (`WHERE linked_task_id != ''`).
- `run_events`, `office_run_route_attempts`, and `office_run_skills` declare
  **no foreign key to `runs`** on either engine. Confirmed in
  `office/repository/sqlite/base.go` and in the PostgreSQL conformance snapshot
  `internal/persistence/storeconformance/testdata/upgrades/v0.93.0/postgres.sql`,
  where `run_events` has only a primary key and one index. A cascade would
  therefore delete nothing; satellite deletion must be explicit
  (AC-OFFICE-RUN-HISTORY-RETENTION-005.2).
- `run_events.seq` is assigned by `AppendRunEvent` as
  `COALESCE(MAX(seq) + 1, 0)` scoped to the run. Deleting a live run's whole
  timeline restarts its sequence at zero, and the run detail view's incremental
  tail reads `WHERE seq > afterSeq`. This is why
  AC-OFFICE-RUN-HISTORY-RETENTION-001.6 forbids event deletion outside the
  transaction that deletes the run.
- `runs` indexes are `idx_run_status_requested (status, requested_at)` and the
  partial unique `idx_run_idempotency`. Nothing indexes `finished_at`.
- `ScheduleRetry` (`runs/repository/sqlite/runs.go`) sets
  `status = 'queued', finished_at = NULL` on an existing run, keyed by id with
  **no status guard in its `WHERE` clause**. Any terminal run can therefore
  become live again at any moment, which is the race
  AC-OFFICE-RUN-HISTORY-RETENTION-002.4 closes. Because the guard is absent, a
  `cancelled` row is resurrectible on exactly the same terms as a `failed` one,
  so admitting `cancelled` to the history set adds no new race — it is covered
  by the same re-assertion.
- `CancelRunsWhere` (`runs/repository/sqlite/cancel.go`) is documented as "the
  single writer of the terminal cancel state on the runs table". It sets
  `status = 'cancelled', cancel_reason = ?, finished_at = ?` and is guarded by
  `status IN ('queued', 'claimed')`, so a cancelled row is always terminal and
  always carries a completion timestamp. It is production-reachable through
  `CancelRunsForTasks` from `office/service/tree_controls.go` and
  `office/repository/sqlite/participants.go`. It writes the literal string, so
  `office/models/enums.go`'s four-value `RunStatus` block does not enumerate it:
  the database has five `runs` statuses, not four. This is why
  AC-OFFICE-RUN-HISTORY-RETENTION-001.2 classifies `cancelled` as history and
  AC-OFFICE-RUN-HISTORY-RETENTION-001.10 makes any sixth value fail safe.
- Every production writer of a terminal `runs` status stamps the completion
  timestamp: `FinishRun` sets `finished_at = now`, `MarkRunFailed` sets
  `finished_at = COALESCE(finished_at, now)`, and `CancelRunsWhere` sets it
  outright. Only the test helper `SetRunStatusForTest` can leave a terminal row
  with a null `finished_at`. The `COALESCE(finished_at, requested_at)` fallback
  in AC-OFFICE-RUN-HISTORY-RETENTION-001.3 is therefore defensive rather than a
  routine path — but it is also what makes the ordering key non-null, which is
  what keeps the floor deterministic across engines (see below).
- `runs.requested_at` and `office_routine_runs.created_at` are both
  `TIMESTAMP NOT NULL`, so the completion-time expression is total. This matters
  for parity, not just tidiness: SQLite and PostgreSQL differ in where they sort
  nulls by default, so an `ORDER BY` over a nullable timestamp would rank the
  floor differently on the two engines from identical data.
- `CleanExpired` (`runs/repository/sqlite/runs.go`) already deletes terminal
  `runs` rows older than a cutoff and **has no production caller** — only
  tests reach it. It deletes only `runs`, so wiring it as-is would orphan every
  satellite row. It is superseded by the batched, satellite-aware delete below
  rather than reused.

## Component: `internal/office/retention`

One package holding policy, settings, the sweep, and the health check.

### Ownership and lifecycle

A single goroutine owner modelled on `internal/system/storage.Scheduler`: a
`Start(ctx)` that is a no-op when already running, a `Stop()` that cancels and
joins, and a buffered wake channel so a settings change re-arms the interval
without interrupting a sweep in progress
(AC-OFFICE-RUN-HISTORY-RETENTION-004.5). `Stop` is joined from the same place
that stops the Office scheduler.

The loop is deliberately not the Office run-processing tick
(`office/service/scheduler_integration.go`, `DefaultTickInterval = 5s`). That
tick already carries `RecoverStale` and `ReapStaleCheckouts` unthrottled and is
the run-claim hot path; on SQLite a bulk delete there contends with the single
writer that claims runs, 17,280 times a day, for an input that changes on a
scale of days (AC-OFFICE-RUN-HISTORY-RETENTION-002.1).

A `sweeping bool` guarded by the same mutex makes a due sweep a skip rather than
a second goroutine (AC-OFFICE-RUN-HISTORY-RETENTION-002.2). That guard is
process-local, which is sufficient on SQLite, where the database file is owned
by one backend process. On PostgreSQL, where several backends can share one
database, it is not: two backends would each see `sweeping == false` and sweep
the same rows concurrently.

The sweep therefore also takes a PostgreSQL advisory lock, and it must be a
**session-scoped, non-blocking** one, which is a different shape from every
existing advisory lock in this repository:

```
conn := db.Conn(ctx)                        // one dedicated connection
SELECT pg_try_advisory_lock(:retention_key) // boolean, returns immediately
... whole sweep, every table, every batch, on the pool ...
SELECT pg_advisory_unlock(:retention_key)   // on that same connection
conn.Close()                                // deferred
```

Both properties are load-bearing and neither is optional:

- **Session-scoped, not transaction-scoped.** The sweep is multi-transaction by
  construction: AC-OFFICE-RUN-HISTORY-RETENTION-002.5 makes each batch its own
  transaction, and AC-OFFICE-RUN-HISTORY-RETENTION-002.7 requires one table's
  failure to leave another table's committed deletions intact, which forbids
  wrapping the sweep in a single transaction. A `pg_advisory_xact_lock` is
  released when its transaction ends, so it would protect one batch and then let
  a second backend in between tables — the interleaving
  AC-OFFICE-RUN-HISTORY-RETENTION-002.12 exists to prevent. The lock is held on a
  connection checked out for the sweep and released in a `defer`; the batches
  themselves continue to use the pool.
- **`try`, not the blocking form.** AC-OFFICE-RUN-HISTORY-RETENTION-002.12 says a
  backend that cannot acquire the lock *skips*. `pg_advisory_xact_lock` and
  `pg_advisory_lock` wait instead of failing, which would convert a concurrent
  sweep into a queued one and eventually stall the scheduler behind a long sweep.
  `pg_try_advisory_lock` returns `false` immediately; on `false` the backend
  records a skip and returns, exactly as for a local concurrent sweep.

This deliberately departs from the established repo pattern
`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`
(`office/repository/sqlite/participants.go`, `internal/secrets/sqlite_store.go`,
`internal/workflow/repository/phase2_sqlite.go`), and the departure is the point:
each of those call sites performs its entire unit of work inside the one
transaction that holds the lock, and each *wants* to wait rather than skip. The
sweep can do neither. The key is derived the same way, `hashtextextended` over a
constant distinct from every key those sites use, so retention never contends
with participant-seat, secret-transfer, or workflow-phase locking.

**If the lock connection drops mid-sweep**, PostgreSQL releases the session's
advisory locks as part of ending the session, so a crashed or partitioned backend
cannot wedge retention permanently — that self-healing is the reason for a
session lock rather than a lease row in `system_settings`, which would need its
own expiry and its own stale-holder rule. The cost is that the surviving sweep no
longer holds exclusivity without knowing it, so the sweep **verifies the lock
connection is still alive between tables** and, if it is not, stops before the
next table, records that table as skipped rather than failed, and returns. It
does not attempt to re-acquire mid-sweep: a re-acquisition after another backend
has taken the lock would produce exactly the concurrent sweep this protects
against. Batches already committed stay committed, which
AC-OFFICE-RUN-HISTORY-RETENTION-002.5 permits.

A skip is recorded as its own value — a skip counter and a last-skip timestamp —
and does **not** overwrite `LastSweep`. `LastSweep` holds the last sweep that
actually ran, so a burst of skips cannot blank the operator's view of the last
real result (AC-OFFICE-RUN-HISTORY-RETENTION-002.2, and
AC-OFFICE-RUN-HISTORY-RETENTION-004.7, which forbids rendering a
never-swept-looking surface when a sweep has in fact run).

The first sweep after `Start` is armed at a fixed 5-minute delay rather than a
full interval (AC-OFFICE-RUN-HISTORY-RETENTION-002.10). Arming at the interval,
as the storage scheduler does with its 24-hour default, means an install
restarted more often than the interval never sweeps at all; 5 minutes keeps
startup clear of schema init and run recovery without depending on uptime. Each
sweep computes `time.Now().UTC()` once and passes that instant to every table,
so two tables in one sweep cannot disagree about the cutoff
(AC-OFFICE-RUN-HISTORY-RETENTION-002.11).

Scheduling is **fixed-delay, not fixed-rate**: the next sweep is armed when the
previous one returns, so a sweep that overruns its interval delays its successor
instead of causing an immediate second one (which the concurrency guard would
only skip anyway, turning a slow sweep into a stream of skips). A settings
change re-arms the delay from the moment of the change rather than from the last
sweep, and enabling retention that was disabled arms at the same 5-minute delay
as a fresh start rather than at a full interval — otherwise an operator who
enables retention on a 168-hour interval waits a week to see whether it works
(AC-OFFICE-RUN-HISTORY-RETENTION-002.13).

### Eligibility, expressed once

Two predicates, each defined in exactly one place and reused by the count, the
preview, and the delete.

**Routine runs.** History statuses are `skipped`, `coalesced`, `failed`, `done`,
`cancelled`. `received` and `task_created` are absent by construction, which is
what makes AC-OFFICE-RUN-HISTORY-RETENTION-005.3 hold: the dispatch gate
`GetActiveRunForFingerprint` reads only `status = 'task_created'`, so no sweep
can change its answer.

```
DELETE FROM office_routine_runs
WHERE id IN (
  SELECT id FROM (
    SELECT id,
           ROW_NUMBER() OVER (
             PARTITION BY routine_id
             ORDER BY COALESCE(completed_at, created_at) DESC, id DESC
           ) AS rn
    FROM office_routine_runs
    WHERE status IN (<history statuses>)
  ) ranked
  WHERE rn > :floor
    AND completion_time < :cutoff
  ORDER BY completion_time ASC, id ASC
  LIMIT :batch
)
AND status IN (<history statuses>)
AND COALESCE(completed_at, created_at) < :cutoff
```

where the ranked subquery also projects
`COALESCE(completed_at, created_at) AS completion_time`.

Two orderings appear here and they are not the same ordering; conflating them is
the defect this section exists to prevent.

- The **`ORDER BY` inside `ROW_NUMBER()`** ranks rows *within* a partition so the
  floor keeps the newest per owner: `completion_time DESC, id DESC`
  (AC-OFFICE-RUN-HISTORY-RETENTION-001.4).
- The **`ORDER BY` on the outer select** decides *which* eligible rows a
  batch-limited sweep takes: `completion_time ASC, id ASC`, oldest first
  (AC-OFFICE-RUN-HISTORY-RETENTION-002.3). The window function's ordering does
  not reach the outer `LIMIT`, so without this clause the engine is free to
  return any subset and the two engines may drain a backlog differently from
  identical data — which would contradict
  AC-OFFICE-RUN-HISTORY-RETENTION-005.1 and make the parity test below flake for
  a reason unrelated to a genuine engine difference.

`id` is a UUID and so is not chronological; it is used only as a total, stable
tiebreak for equal timestamps, in both orderings.

The trailing `AND` clauses are the re-assertion required by
AC-OFFICE-RUN-HISTORY-RETENTION-002.4. For this table the whole ranked subquery
is re-evaluated inside the `DELETE`, so the floor is re-asserted atomically along
with status and age; the two-phase `runs` path below has to do that explicitly.

Window functions are available on both engines: SQLite 3.54.0 through
`github.com/mattn/go-sqlite3 v1.14.33`, verified by running this exact
`ROW_NUMBER() OVER (PARTITION BY routine_id ...)` against the reference
database.

**Runs.** History is `status IN ('finished','failed','cancelled')`, partitioned
by `agent_profile_id`, ranked `COALESCE(finished_at, requested_at) DESC, id DESC`
and batch-ordered `COALESCE(finished_at, requested_at) ASC, id ASC`. There is no
`finished_at IS NOT NULL` conjunct: requiring one would make a terminal row with
an unset stamp immortal and unobservable, which
AC-OFFICE-RUN-HISTORY-RETENTION-001.3 forbids. `queued` and `claimed` are
absent, which covers a routing-parked run whose `earliest_retry_at` is far in
the future (AC-OFFICE-RUN-HISTORY-RETENTION-001.2).

A status in neither set is treated as live state and raises
`office_retention_unknown_status:<table>`
(AC-OFFICE-RUN-HISTORY-RETENTION-001.10). Both sets are closed and asserted
against `office/models/enums.go` plus the literal-SQL writers in a test, so a
sixth status cannot enter the database without failing that test — the failure
mode that hid `cancelled` in the first place.

**That warning needs a producer, and the eligibility predicate cannot be it.**
The predicate selects `status IN (<history statuses>)`, so a row holding an
unrecognized status is never selected, never counted, and never seen: a fail-safe
whose only detector is a CI test fires on the developer's machine and stays
silent on the install that actually has the row. The producer is instead the
**status census** — one

```
SELECT status, COUNT(*) FROM <swept table> GROUP BY status
```

per swept table, run by the retained-count evaluation described in [run history
retention operations](run-history-retention-operations.md#reporting). Three
consequences follow from siting it there rather than in the sweep, and each one
closes a hole:

- The census **subsumes the retained count** rather than adding a second scan:
  the retained count is the sum of the census rows, so one `GROUP BY` yields
  both. (`run_events` is thresholded but not swept and has no status column, so
  it keeps a plain `COUNT(*)`.)
- It runs **whether or not retention is enabled**, because the count evaluation
  does (AC-OFFICE-RUN-HISTORY-RETENTION-003.11). A disabled install is precisely
  where an unrecognized status would otherwise never be noticed, since
  AC-OFFICE-RUN-HISTORY-RETENTION-002.8 means no sweep runs there at all.
- It sees a status **because the row exists**, not because the row was
  selectable, which is the property the eligibility predicate structurally
  cannot have.

One issue per table, not one per status: a table with several unrecognized
statuses raises the single id `office_retention_unknown_status:<table>` whose
message lists every unrecognized status **in ascending lexicographic order** with
its row count. Ordering is named because the message is compared across health
polls; an unordered list would make a stable condition look like a changing one.

### Deleting a run

Per batch, one transaction, satellites first, run last:

1. Select up to `batch_limit` eligible run ids.
2. `DELETE FROM run_events WHERE run_id IN (...)`
3. `DELETE FROM office_run_route_attempts WHERE run_id IN (...)`
4. `DELETE FROM office_run_skills WHERE run_id IN (...)`
5. `DELETE FROM runs WHERE id IN (:selected_ids) AND id IN (<ranked eligible
   subquery>)` — the re-assertion is the **whole ranked subquery**, not just the
   status and cutoff conjuncts, so the retention floor is re-evaluated at delete
   time along with them. Unlike the single-statement `office_routine_runs`
   delete, this path selected its ids in a separate earlier statement, so a
   `ScheduleRetry` in between can re-rank a partition and push a row that was
   `rn > floor` at selection to `rn <= floor` now. Re-asserting only status and
   age would delete a row that has since become floor-protected
   (AC-OFFICE-RUN-HISTORY-RETENTION-002.4).

Step 5 can delete fewer rows than steps 2 to 4 addressed, when a `ScheduleRetry`
resurrected a run between selection and delete. That is the correct outcome for
AC-OFFICE-RUN-HISTORY-RETENTION-002.4 only if the whole batch rolls back rather
than leaving a live run without its timeline. **The transaction is therefore
rolled back and retried once with a fresh selection when the step 5 row count
does not match the selected id count**; a second mismatch rolls back again and
abandons the batch.

An abandoned batch is recorded as a **failure** for that table, raising
`office_retention_failed:<table>`, not as backlog
(AC-OFFICE-RUN-HISTORY-RETENTION-002.7). The distinction is not cosmetic:
backlog means work correctly deferred by the batch limit and is expected on a
large install, so routing this case there would file the one genuinely dangerous
outcome under the one routine one. The table is retried on the next scheduled
sweep either way, but only the failure classification tells an operator that a
deletion could not be applied consistently.

Step 5 can only ever delete *fewer* rows than steps 2 to 4 addressed, never
more, because it is bounded by the same selected id set. This is the one place
where the naive implementation silently corrupts a live run, and it is the
reason satellite deletion is not a separate statement outside the transaction.

`run_events` is never addressed by any predicate other than membership in this
id set (AC-OFFICE-RUN-HISTORY-RETENTION-001.6). There is no age-based delete on
`run_events`.

### Indexes to add

Neither table has an index serving the sweep.

- `idx_office_routine_runs_retention ON office_routine_runs(routine_id, completed_at DESC, id DESC)`
- `idx_runs_retention ON runs(agent_profile_id, finished_at DESC, id DESC)`

Added through the existing `office/repository/sqlite` schema path so both the
fresh-install `CREATE` and the upgrade path get them, and recorded in the
conformance fixtures.

## Ordering, concurrency, and failure

| Question | Answer | AC |
|---|---|---|
| Sweep ordering across tables | `office_routine_runs`, then `runs` with its satellites. Independent sets; the order is fixed only so results and logs are reproducible. | 002.6 |
| Floor ordering | `COALESCE(completed_at, created_at) DESC, id DESC` / `COALESCE(finished_at, requested_at) DESC, id DESC` | 001.4 |
| Two sweeps due at once | Second is skipped, not queued, and recorded as skipped | 002.2 |
| Sweep vs. live writer | Selection is a snapshot; status, age **and floor** are re-asserted in the delete; a batch whose step 5 count disagrees is rolled back | 002.4, 002.5 |
| Sweep interrupted | Each batch is one transaction; applied batches are whole, the interrupted one is not applied | 002.5 |
| First sweep after startup | Armed at a fixed 5-minute delay, not a full interval | 002.10 |
| Cutoff instant | Computed once per sweep, shared by every table | 002.11 |
| Re-run once no eligible rows remain | Deletes nothing further, reports zero. While backlog remains, the next sweep deletes the next batch by 002.3 — the two are not in tension because 002.6 is scoped to the drained state | 002.6, 002.3 |
| One table errors | That table is recorded failed; remaining tables continue; retried next sweep; startup unaffected | 002.7 |
| Empty tables | Zero deletions, no error, no warning | 002.9 |
| Deleted coalesce target | Allowed; `coalesced_into_run_id` is provenance, read by nothing | 001.9 |
| Routine paused | Irrelevant to eligibility | 001.7 |
| Cancelled run | History, like `finished`/`failed`; its writer stamps the completion timestamp | 001.2 |
| Terminal row, null completion stamp | Still history; dated by `created_at` / `requested_at` | 001.3 |
| Status in neither set | Treated as live state, never deleted, warned | 001.10 |
| Which rows a batch-limited sweep takes | `completion_time ASC, id ASC` — oldest first, named columns | 002.3, 005.1 |
| Batch abandoned after retry | Recorded as that table's failure, not as backlog | 002.7 |
| Preview finds more eligible rows than the batch limit | Not backlog. A preview deletes nothing by design, so "retention is behind" would be false; it reports the full eligible count instead | 002.3, 003.6, 003.9 |
| Sweep skipped | Recorded separately; never overwrites the last real `LastSweep` | 002.2, 004.7 |
| Two backends, one PostgreSQL | Session-scoped `pg_try_advisory_lock` held on a dedicated connection for the whole sweep; the loser skips without waiting | 002.12 |
| Lock connection drops mid-sweep | PostgreSQL releases the lock with the session; the sweep stops before the next table and records it skipped, and does not re-acquire | 002.12 |
| Scheduling model | Fixed-delay from the end of the previous sweep; a settings change re-arms from the change | 002.13 |

## Testing

Unit and repository tests in `internal/office/retention` and
`internal/office/repository/sqlite`, plus the persistence gates.

Behaviors that must have a test, because each is a way the naive implementation
is wrong:

- A `task_created` routine run older than any window survives, and
  `GetActiveRunForFingerprint` still finds it (001.1, 005.3).
- A `queued` run with `finished_at` cleared by `ScheduleRetry` survives (001.2).
- A run resurrected between selection and delete leaves both the run and its
  full `run_events` timeline intact (002.4, and the rollback above).
- After a sweep, no `run_events`, `office_run_route_attempts`, or
  `office_run_skills` row references a missing `runs` row (001.5, 005.2).
- No `run_events` row of a surviving run is ever deleted (001.6).
- A routine with 3 history rows all older than the window keeps all 3 under a
  floor of 50; a routine with 200 keeps exactly 50 (001.4).

- Two sweeps triggered concurrently produce one sweep and one recorded skip
  (002.2).
- A sweep hitting the batch limit reports backlog and the next sweep continues,
  and its satellite rows are deleted in full rather than capped (002.3, 003.6).

- A terminal row whose completion timestamp is unset is dated by `created_at`
  or `requested_at` and ages out rather than being retained forever (001.3).
  This test is only satisfiable because AC-OFFICE-RUN-HISTORY-RETENTION-001.2
  classifies history by status alone; an implementation that also required
  `finished_at IS NOT NULL` would retain the row forever and fail here.
- A `cancelled` run older than the window is deleted together with its satellite
  rows, and a `cancelled` run inside the floor is retained (001.2). Seed it
  through the real cancel path, not by writing the status directly, so the test
  fails if that path stops stamping the completion timestamp.
- A `runs` row holding a status in neither the history set nor the live-state set
  survives every sweep at any age and raises
  `office_retention_unknown_status:runs` (001.10). Three companions: the same row
  raises the same issue with retention **disabled**, where no sweep runs at all;
  a table holding two unrecognized statuses raises **one** issue listing both in
  ascending order with their counts; and a test asserts the two status sets
  together cover every value in `office/models/enums.go` *and* every status
  literal written by SQL in `internal/runs/repository/sqlite`, so a future sixth
  value cannot be added silently — the exact gap through which `cancelled` was
  missed.
- With more eligible rows than the batch limit, the sweep deletes the oldest
  eligible rows first, and the same seed data yields the identical deleted set on
  SQLite and PostgreSQL (002.3, 005.1). Without the outer `ORDER BY` this test is
  the one that fails.
- A row that becomes floor-protected between selection and delete is not deleted
  by the `runs` path (002.4).

- A batch abandoned after its retry is reported as that table's failure and not
  as backlog (002.7).

- Two backends against one PostgreSQL run one sweep, and the loser records a
  skip (002.12). The seed must give the winner **more than one table** to sweep,
  so that a transaction-scoped lock — which would release between tables and let
  the loser in — fails this test rather than passing it. A companion test drops
  the winner's lock connection mid-sweep and asserts it stops before the next
  table and does not re-acquire.
- A `runs` batch abandoned after its retry reports zero rows deleted for
  `run_events`, `office_run_route_attempts` and `office_run_skills`, not the
  counts the rolled-back statements addressed (002.7, 004.6).

Commands:

```
cd apps/backend
go test ./internal/office/... ./internal/runs/... -race -count=1
go run ./cmd/sqlguard ./internal
go test -race ./internal/persistence/storeconformance -count=1
KANDEV_TEST_POSTGRES_DSN=<dsn> go test -race ./internal/office/... \
  ./internal/persistence/storeconformance -count=1
cd apps/web && pnpm run typecheck && pnpm run i18n:check
```

The PostgreSQL line is not optional. The repository's dialect-sensitive suites
self-skip when `KANDEV_TEST_POSTGRES_DSN` is unset, so a green local run without
it proves nothing about AC-OFFICE-RUN-HISTORY-RETENTION-005.1. The engine-parity
test asserts identical deleted sets, retained sets, and counts from identical
seed data on both engines.

## Rejected alternatives

- **Wire the existing `CleanExpired` and stop.** One line, and it orphans every
  `run_events` row it passes, permanently and invisibly, because no foreign key
  on either engine would clean up after it.
- **Add `ON DELETE CASCADE` to the satellite tables instead.** A schema change
  to three tables on two engines, requiring a table rebuild on SQLite, to avoid
  three `DELETE` statements. It also hides the deletion from the code that has
  to count it for the sweep report.
- **Age-prune `run_events` directly.** The obvious implementation, and the one
  that breaks the run detail view's incremental tail and can restart a live
  run's sequence at zero. Forbidden by 001.6.
- **Run the sweep on the 5s Office tick.** Rejected in 002.1.
- **Count-only retention, keep newest N per owner with no window.** Bounds the
  table but makes "how long is my history kept" unanswerable for an install
  with mixed routine frequencies. The floor covers the case count-only is good
  at; the window covers the case it is bad at.
- **Deletion off by default.** Safe, and it means the gap stays open on every
  install that never visits the settings page. The per-table preview, designed
  in [run history retention
  operations](run-history-retention-operations.md), gives the same protection
  without that outcome.
- **Persist sweep history.** Named in the operations requirement's exclusions.

- **Rely on the window function's `ORDER BY` to order the batch.** It ranks rows
  inside each partition; it does not order the rows the outer `LIMIT` draws
  from. Leaving the outer select unordered makes a backlog sweep's deleted set
  engine-dependent and quietly breaks
  AC-OFFICE-RUN-HISTORY-RETENTION-005.1 only on installs large enough to exceed
  the batch limit — the installs that need this feature most.

## Prior art, applied

**Wiki: unavailable** — receipt in the requirement document. Nothing here should
be read as departing from a wiki position, because none could be consulted.

**`internal/automation/run_retention.go`** is the closest in-repo precedent, and it
prunes worktrees rather than rows. Three things are taken from it: retention scoped
per owner rather than globally, so one noisy owner cannot evict a quiet one's
only record; a bounded sweep window so a backlog drains across sweeps instead
of walking the whole table each time; and re-checking liveness immediately
before the destructive act, which appears here as the re-asserted predicate and
the batch rollback. One thing is deliberately not taken: it hangs its sweep off
a finalization hook, which ties cleanup frequency to firing frequency and leaves
a stopped routine's history untouched forever. This design uses a clock.

**`internal/system/storage`** supplies the scheduler shape, the settings
storage and normalization pattern, the hours-based interval with min and max
bounds, and the `health.Checker` route to a production-visible warning.

**GitLab Duo** and **the Claude apps gateway** are surveyed in the requirement
document's Prior art. What this design takes from them: the 30-day default
window, and the history-vs-live-state split (their per-table windows with one
table marked "until deleted via the API" rather than given a window). Neither
previews before the first deletion; that addition is ours, and it exists because
this ships enabled by default onto installs that already hold history.
