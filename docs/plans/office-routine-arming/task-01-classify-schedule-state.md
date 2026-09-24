---
id: "01-classify-schedule-state"
title: "Classify a routine's schedule state"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-ROUTINE-ARMING-001
acceptance_criteria:
  - AC-OFFICE-ROUTINE-ARMING-001.1
  - AC-OFFICE-ROUTINE-ARMING-001.2
  - AC-OFFICE-ROUTINE-ARMING-001.3
  - AC-OFFICE-ROUTINE-ARMING-001.4
  - AC-OFFICE-ROUTINE-ARMING-001.5
  - AC-OFFICE-ROUTINE-ARMING-001.6
  - AC-OFFICE-ROUTINE-ARMING-001.7
  - AC-OFFICE-ROUTINE-ARMING-001.8
  - AC-OFFICE-ROUTINE-ARMING-001.9
  - AC-OFFICE-ROUTINE-ARMING-001.10
  - AC-OFFICE-ROUTINE-ARMING-001.11
  - AC-OFFICE-ROUTINE-ARMING-001.12
  - AC-OFFICE-ROUTINE-ARMING-001.13
system_design:
  - ../../specs/office/system-design/routine-schedule-state.md
---

# Task 01: Classify a routine's schedule state

Satisfies REQ-OFFICE-ROUTINE-ARMING-001 (AC-OFFICE-ROUTINE-ARMING-001.1 through
.13).

## Scope

A pure, total classification of one routine's schedule state from its trigger
rows alone, plus the batched-read-with-per-routine-fallback semantics that
AC-001.12 and AC-001.13 require, plus the unarmed-cron-trigger list AC-001.8
requires alongside it. This task defines the dispatch-grace constant
(AC-001.10) that Task 04 (startup scan) also consumes — define it once here.

## Exclusions

- No HTTP response shape, no dashboard change, no UI (Task 02/03).
- No startup scan (Task 04).
- No coordinator-install change (Task 05) — it consumes this task's output but
  this task does not touch `routines/service.go`'s coordinator-install path.
- No repair: this task's function set never writes `office_routine_triggers`
  or `office_routines`.

## Acceptance conditions

1. `ClassifyRoutine(triggers []models.RoutineTrigger, now time.Time) (ScheduleState, []UnarmedCronTrigger)`
   (exact name/package left to Build; suggested home:
   `internal/office/routines`, since that package already owns trigger
   dispatch logic and is already imported by `office/agents` and
   `office/onboarding`) implements the nine-rule table in rule order,
   stopping at first match (AC-001.1), never reads `office_routines.status`
   (AC-001.2), and treats a past-due or exactly-at-`now` `next_run_at` as
   rule 1 (AC-001.3, AC-001.4). It returns every applicable unarmed-cron
   reason per trigger, in the fixed order (`enabled=false`, then
   "not schedulable", then "stalled past dispatch grace"), in trigger order
   (`created_at` ascending, `id` ascending) (AC-001.8, AC-001.9, AC-001.11).
   A `dispatchGrace` constant of exactly `60 * time.Second`, unexported and
   with no environment/config/flag override, backs rule 2 and the unarmed
   list's third reason (AC-001.10).
2. A batch classification entry point reads every named routine's trigger
   rows in one query, captures one `now` for the whole call, and on a batch
   read failure (or one undecodable row) re-reads exactly the affected
   routines individually — once each — reporting `unknown` only for those
   whose individual re-read also fails, reusing the batch call's captured
   `now` rather than recapturing it per routine (AC-001.5, AC-001.12,
   AC-001.13). A routine whose reads never fail counts as one query
   contribution regardless of how many other routines are classified in the
   same call (feeds Task 02's AC-002.7 query-count bound).
3. Table-driven tests cover: every one of the 9 rules in isolation; the
   rule-1/rule-9 interaction of AC-001.9 (armed on one trigger, unarmed list
   non-empty from a sibling); the never-fired exclusion of AC-001.11; the
   batch-then-per-routine-fallback path of AC-001.13 with a fixture that
   forces one row in a multi-routine batch to fail decode; and identical
   output on SQLite and PostgreSQL for the same rows (AC-001.6, via the
   `storeconformance`/`KANDEV_TEST_POSTGRES_DSN` pattern already used
   elsewhere in this package's tests).

## Verification

- `cd apps/backend && go test -tags fts5 ./internal/office/routines/... ./internal/office/repository/sqlite/...`
- New tests must first fail against the current tree, where no classifier
  exists.
- `KANDEV_TEST_POSTGRES_DSN=<dsn> go test -tags fts5 ./internal/office/repository/sqlite/... -run TestClassify` (or
  equivalent name) for the PostgreSQL leg of AC-001.6.

## Files likely touched

- new: `apps/backend/internal/office/routines/arming.go`
- new: `apps/backend/internal/office/routines/arming_test.go`
- `apps/backend/internal/office/repository/sqlite/routines.go` — add the
  batch trigger-read query and the single-routine fallback read.
- `apps/backend/internal/office/repository/sqlite/routines_test.go`

## Dependencies

None. This is the foundation for Tasks 02, 04, and 05.

## Parallelism

Must land before Wave 2. Nothing in Wave 1 can run in parallel with it.

## Result

Implemented in `internal/office/routines/arming.go` (unexported `dispatchGrace`,
`ClassifyRoutine`, `ClassifyRoutines`, `RoutineTriggerReader`) plus
`ListTriggersByRoutineIDs` and an `, id` trigger-order tiebreak on
`ListTriggersByRoutineID` in `internal/office/repository/sqlite/routines.go`.
Tests: `arming_test.go` (all 9 rules, precedence, unarmed-reason ordering, the
armed/unarmed-sibling interaction, trigger order, batch-then-fallback),
`arming_dialect_postgres_test.go` (AC-001.6, gated on
`KANDEV_TEST_POSTGRES_DSN`, not run in this environment), and a new
`TestListTriggersByRoutineIDs_Batch` in `routines_test.go` covering the id
tiebreak and the empty-routine/empty-input cases.

Verification: `gofmt -l` clean; `go test -tags fts5
./internal/office/routines/... ./internal/office/repository/sqlite/...`
passes except the pre-existing, unrelated `TestMigrate_PriorityIdempotent`
failure (reproduced on the unmodified tree before this task's changes, via
`git stash`); `golangci-lint run` clean on both packages.
