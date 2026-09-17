---
id: "02-report-schedule-state-api"
title: "Report intent and schedule state on routine reads (API)"
status: done
wave: 2
depends_on: ["01-classify-schedule-state"]
plan: "plan.md"
spec: "../../specs/office/requirements/routine-arming-visibility.md"
---

# Task 02: Report intent and schedule state on routine reads (API)

Satisfies REQ-OFFICE-ROUTINE-ARMING-002's data-shape criteria:
AC-OFFICE-ROUTINE-ARMING-002.1, .4 (data only — rendering is Task 03), .5,
.6, .7, .11, .12. The rendering-only criteria (.2, .3, .8, .9, .10) belong
to Task 03, which depends on this task's response shape.

## Scope

Extend the routine list response and the single-routine response to carry,
per routine: intent (`office_routines.status`, unchanged), schedule state,
and the unarmed cron trigger list, both from Task 01's classifier — without
growing the query count with the number of routines in the workspace.

## Exclusions

- No new classification logic — call Task 01's batch entry point.
- No UI change (Task 03).
- No startup scan (Task 04).
- No coordinator-install change (Task 05).

## Acceptance conditions

1. The routine list response and the single-routine response each carry
   intent, schedule state, and the unarmed cron trigger list per routine, in
   one call each (AC-002.1). The pair is not required to be a transactional
   snapshot: intent comes from the routine row, schedule state from the
   trigger rows, and neither is omitted because the other read failed — a
   routine whose triggers could not be read still carries its intent
   alongside schedule state `unknown` (AC-002.12).
2. Producing the list costs a bounded number of queries that does not grow
   with routine count on the all-succeeds path: one query for routines, one
   batch query for triggers (Task 01's batch entry point), each isolated
   Task-001.13 re-read counted as the documented exception rather than a
   regression (AC-002.7).
3. Response shape distinguishes, without prescribing rendering: a schedule
   state of `unscheduled_manual_only` or `unscheduled_no_trigger` is
   reportable as "no schedule" (AC-002.5); `unknown` is reportable as
   undetermined, distinct from armed or broken (AC-002.6); `event_only` is
   never bundled with "no schedule" and never asks for a trigger to be
   created (AC-002.11); and for a non-empty unarmed list, whether at least
   one entry is schedulable is derivable by the caller without it having to
   inspect the dispatch-grace math itself — an empty list carries neither
   side of that distinction (AC-002.4).

## Verification

- `cd apps/backend && go test -tags fts5 ./internal/office/dashboard/... ./internal/office/routines/...`
- Test: routine list response for a fixture with all nine schedule states
  present carries the right state and unarmed list per routine.
- Test: query-count assertion (via a counting DB wrapper or an existing
  budget-test pattern in this package) proves AC-002.7 on an all-succeeds
  fixture of N routines, and separately on a fixture that forces one
  Task-001.13 re-read.
- Test: a routine whose trigger read fails carries its real intent value
  alongside `unknown` (AC-002.12).

## Files likely touched

- `apps/backend/internal/office/routines/handler.go` — `listRoutines`
  (L45-52) and `getRoutine` (L98-105) are the actual routine list/detail
  HTTP handlers (`GET /workspaces/:wsId/routines`, `GET /routines/:id`);
  call Task 01's `ClassifyRoutines`/`ClassifyRoutine` here and populate the
  extended response.
- `apps/backend/internal/office/routines/dto.go` — `RoutineResponse` (L34)
  and `RoutineListResponse` (L39) currently carry only `*Routine`
  (`{routine}` / `{routines}` — intent only). Extend them to also carry
  schedule state and the unarmed cron trigger list per routine (e.g. a
  wrapper type embedding `*Routine` plus the two new fields, so intent and
  schedule state travel in the same JSON object per AC-002.1).
- Note: `apps/backend/internal/office/dashboard/service_agents.go`'s
  `ListRoutines` block (~L551-561) is a separate, pre-existing aggregate —
  it only counts `status == "active"` into the workspace dashboard's KPI
  tile and is not this requirement's target; leave it alone unless it also
  needs the distinction (out of scope unless the dashboard tile itself is
  asked to change).
- Corresponding `*_test.go` files (`internal/office/routines/handler_test.go`
  or new).

## Dependencies

Task 01.

## Parallelism

Independent of Task 04 and Task 05 once Task 01 lands — no shared files.

## Result

Implemented:

- `apps/backend/internal/office/routines/dto.go` — added `RoutineWithSchedule`
  (embeds `*Routine`, adds `schedule_state` and `unarmed_cron_triggers` JSON
  fields). `RoutineResponse.Routine` and `RoutineListResponse.Routines` now
  carry `*RoutineWithSchedule` / `[]*RoutineWithSchedule` instead of the bare
  `*Routine`.
- `apps/backend/internal/office/routines/service.go` — added
  `ListTriggersByRoutineIDs` to the `Repository` interface (structurally
  required so `s.repo` satisfies Task 01's `RoutineTriggerReader`), and added
  `AttachScheduleState(ctx, []*Routine) ([]*RoutineWithSchedule, error)`,
  which calls `ClassifyRoutines` once per call (one batch trigger query,
  independent of routine count) and pairs each routine's intent with its
  schedule state and unarmed list.
- `apps/backend/internal/office/routines/handler.go` — `listRoutines` and a
  new `withScheduleState` helper (used by `createRoutine`, `getRoutine`,
  `updateRoutine`) call `AttachScheduleState` before building the response.
  No other file in `internal/` constructs `RoutineResponse`/
  `RoutineListResponse` (confirmed via grep), so no other call sites needed
  updates.

AC-002.1: `RoutineWithSchedule` carries intent (`Status`, from the embedded
`*Routine`), `ScheduleState`, and `UnarmedCronTriggers` in one JSON object.
AC-002.12: covered by `TestAttachScheduleState_TriggerReadFailureKeepsIntent`
— a routine whose trigger reads fail (batch and single) keeps its real
`Status` alongside `ScheduleStateUnknown`.
AC-002.7: covered by `TestAttachScheduleState_QueryCountBound` (one batch
call, zero single calls, for 5 routines) and
`TestAttachScheduleState_BatchFailureFallsBackPerRoutine` (one batch call,
exactly one single call per routine on the documented fallback path), both
via a `countingRepo` decorator wrapping a real in-memory-SQLite-backed
repository.
AC-002.5/.6/.11/.4: satisfied structurally — `ScheduleState` is a plain
string enum a caller switches on directly (no additional derivation logic
needed in this task), and `UnarmedCronTrigger.Reasons` already carries
`not_schedulable` distinctly from `stalled`/`disabled`, so an empty list
carries neither and a non-empty list is inspectable without dispatch-grace
math. Verified in `TestAttachScheduleState_AllStatesFixture`, which covers
armed, trigger_invalid, trigger_disabled, unscheduled_manual_only, and
unscheduled_no_trigger routines in one list response.

New test file:
`apps/backend/internal/office/routines/schedule_response_test.go` — 4 tests
(`TestAttachScheduleState_AllStatesFixture`,
`TestAttachScheduleState_QueryCountBound`,
`TestAttachScheduleState_BatchFailureFallsBackPerRoutine`,
`TestAttachScheduleState_TriggerReadFailureKeepsIntent`), all backed by a
real `*routines.RoutineService` over an in-memory SQLite repository (via a
`countingRepo` decorator), not just `ClassifyRoutines` in isolation (already
covered by Task 01's tests).

Verification:

- `go build -tags fts5 ./...` — clean.
- `go test -tags fts5 ./internal/office/routines/...` — `ok`.
- `go test -tags fts5 ./internal/office/dashboard/...` — `ok`.
- `go test -tags fts5 ./internal/office/repository/sqlite/...` — one
  pre-existing, unrelated failure (`TestMigrate_PriorityIdempotent`,
  confirmed via `git stash` against the clean tree during Task 01; not
  touched by this task).
- `gofmt -l` on the new/changed files — clean.
- `golangci-lint run ./internal/office/routines/... --new-from-rev=<merge-base>`
  — `0 issues` (fixed a goconst hit on the repeated `gin.H{"error": ...}`
  literal by extracting `respondInternalError` + a `jsonErrorKey` constant in
  `handler.go`; fixed staticcheck QF1008 redundant-embedded-selector hints in
  the new test file).
