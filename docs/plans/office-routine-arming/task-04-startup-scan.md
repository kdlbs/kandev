---
id: "04-startup-scan"
title: "Surface unarmed routines at startup"
status: done
wave: 2
depends_on: ["01-classify-schedule-state"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-ROUTINE-ARMING-003
acceptance_criteria:
  - AC-OFFICE-ROUTINE-ARMING-003.1
  - AC-OFFICE-ROUTINE-ARMING-003.2
  - AC-OFFICE-ROUTINE-ARMING-003.3
  - AC-OFFICE-ROUTINE-ARMING-003.4
  - AC-OFFICE-ROUTINE-ARMING-003.5
  - AC-OFFICE-ROUTINE-ARMING-003.6
  - AC-OFFICE-ROUTINE-ARMING-003.7
  - AC-OFFICE-ROUTINE-ARMING-003.8
  - AC-OFFICE-ROUTINE-ARMING-003.9
  - AC-OFFICE-ROUTINE-ARMING-003.10
  - AC-OFFICE-ROUTINE-ARMING-003.11
  - AC-OFFICE-ROUTINE-ARMING-003.12
  - AC-OFFICE-ROUTINE-ARMING-003.13
  - AC-OFFICE-ROUTINE-ARMING-003.14
system_design:
  - ../../specs/office/system-design/routine-schedule-state.md
---

# Task 04: Surface unarmed routines at startup

Satisfies REQ-OFFICE-ROUTINE-ARMING-003 (AC-OFFICE-ROUTINE-ARMING-003.1
through .14).

## Scope

A read-only pass at Office startup, run once reconciliation has returned,
that classifies every enumerable routine in every enumerable workspace
(via Task 01) and emits structured logs plus `expvar` counters. No
production behavior changes if the scan is skipped or fails.

## Exclusions

- No classification logic of its own — consumes Task 01.
- No repair, no retry, no periodic re-scan, no API.
- No coordinator-install change (Task 05).

## Acceptance conditions

1. The scan runs after `infra.Reconciler.ReconcileAll` (main.go:1378-1380)
   has returned, ordered by an explicit completion signal passed from the
   startup sequence — not a timer, not an inspection of trigger rows.
   Invoked without that signal (exercised only in a test), it classifies
   nothing, emits one warning-level record, and increments the skipped
   counter with a reason distinct from enumeration and per-routine
   classification failure (AC-003.1). It iterates workspaces by
   `workspaces.created_at` then `workspaces.id`, and within each, routines
   by `office_routines.created_at` then `office_routines.id` (AC-003.2).
   Office startup does not wait for the scan and no scan outcome, including
   a database error, changes startup's result (AC-003.10). It writes
   nothing and never enables, disables, creates, or deletes a trigger
   (AC-003.11).
2. Per routine reached, exactly one record: warning for `active` +
   {`trigger_disabled`, `trigger_unscheduled`, `trigger_invalid`} naming
   workspace, routine, state, and unarmed cron triggers in trigger order
   (AC-003.3); warning for `active` + {`unscheduled_manual_only`,
   `unscheduled_no_trigger`} naming workspace, routine, and state with no
   trigger list, since the routine owns no cron trigger (AC-003.14);
   informational, no warning, for not-`active` (AC-003.4); informational,
   no warning, for `active` + any other non-`unknown` state (AC-003.5,
   scoped to exclude the .3 and .14 cases); and for `unknown`, one
   informational record naming workspace/routine/state regardless of
   intent, which both counts once under the observation counter (AC-003.6)
   and once under the skipped counter (AC-003.7) — intentional double
   counting, not a bug (AC-003.9).
3. Enumeration failures fail closed and are counted distinguishably: failing
   to enumerate the workspace list at all increments the skipped counter
   with its own reason, emits one warning, and returns without classifying
   anything (AC-003.8); failing to enumerate one workspace's routines keeps
   every record already emitted for earlier workspaces, increments the
   skipped counter once for that workspace, emits one warning naming it,
   does not retry it, and continues to the next workspace in order
   (AC-003.13).
4. Running the scan twice over unchanged data emits the same records in the
   same order with no database change, where "same" excludes wall-clock
   fields, counters, and a trigger that crossed the dispatch-grace boundary
   between the two scans, and where the comparison only applies between two
   fully-successful scans (AC-003.12).

## Verification

- `cd apps/backend && go test -tags fts5 ./internal/office/routines/... ./internal/office/infra/... ./internal/backendapp/...`
- Test: scan invoked without the completion signal classifies nothing and
  increments the "no signal" skipped reason (AC-003.1's test branch).
- Test: fixture spanning all schedule states produces exactly the record
  level/content matrix of AC-003.3/.4/.5/.9/.14, one record per routine.
- Test: workspace-enumeration failure and mid-scan per-workspace
  enumeration failure each fail closed per AC-003.8/.13, and startup itself
  never fails or blocks on any scan outcome (AC-003.10).
- Test: two scans over unchanged fixture data produce identical records
  under the AC-003.12 equality (excluding wall-clock/counters and any
  grace-boundary-straddling trigger).

## Files likely touched

- new: `apps/backend/internal/office/routines/startup_scan.go`
- new: `apps/backend/internal/office/routines/startup_scan_test.go`
- new: `apps/backend/internal/office/routines/arming_metrics.go` (expvar
  counters, following `internal/orchestrator/office_stall_metrics.go`'s
  `expvar.NewMap` + label-builder shape)
- `apps/backend/internal/office/infra/reconcile.go` — emit or return the
  completion signal `ReconcileAll` currently has no way to expose.
- `apps/backend/internal/backendapp/main.go` (~L1378-1393) — wire the
  signal from `ReconcileAll` into the new scan, launched so it does not
  block startup.

## Dependencies

Task 01 (classification, and the shared `dispatchGrace` constant it
defines).

## Parallelism

Independent of Task 02 and Task 05 once Task 01 lands — no shared files.

## Result

Implemented:

- `apps/backend/internal/office/repository/sqlite/routine_arming_scan.go`
  (new) — `ListWorkspaceIDsOrdered` (`SELECT id FROM workspaces ORDER BY
  created_at, id`) and `ListRoutinesOrdered` (per-workspace, same ordering),
  separate from the UI's name-ordered `ListRoutines`/workspace listing.
- `apps/backend/internal/office/routines/arming_metrics.go` (new) — two
  `expvar.Map` counters (`office_routine_arming_scan_observations_total`,
  `office_routine_arming_scan_skipped_total`), following
  `internal/orchestrator/office_stall_metrics.go`'s label-string shape, with
  the four skip reasons (`no_signal`, `workspace_enum_failed`,
  `routine_enum_failed`, `classification_unknown`).
- `apps/backend/internal/office/routines/startup_scan.go` (new) —
  `ReconcileSignal` (only constructible via `SignalReconcileComplete`, so an
  unauthorized/test caller holds the refused zero value), the
  `StartupScanReader` interface (Task 01's `RoutineTriggerReader` plus the
  two new ordered listers), and `RunStartupScan`, which classifies every
  workspace's routines via Task 01's `ClassifyRoutines` (one batch trigger
  read per workspace) and emits exactly one structured log record per
  routine reached, per the AC-003.3/.4/.5/.9/.14 matrix.
- `apps/backend/internal/office/infra/reconcile.go` — `ReconcileAll` now
  returns `routines.ReconcileSignal` (via `SignalReconcileComplete()`)
  instead of nothing, so a caller has an explicit, non-timer, non-inspection
  way to know reconciliation returned. Existing callers that ignore the
  return value keep compiling unchanged.
- `apps/backend/internal/backendapp/main.go` (~L1377-1384) — after
  `reconciler.ReconcileAll(ctx)` returns, launches
  `go officeroutines.RunStartupScan(ctx, reconcileSignal, repos.Office, log,
  time.Now().UTC())`. `repos.Office` (`*sqlite.Repository`) satisfies
  `StartupScanReader` structurally (confirmed by `go build` succeeding with
  no adapter needed). The scan runs in its own goroutine so office startup
  does not wait for it (AC-003.10).

AC-003.1: `TestRunStartupScan_NoSignalClassifiesNothing` uses a
`panicIfCalledReader` (panics on any method call) to prove the no-signal
path makes zero reader calls, emits exactly one warning, and increments
`no_signal`.
AC-003.2: enumeration order comes from the two new `ORDER BY created_at, id`
repository methods; not independently retested here since Task 01/02 already
cover the tiebreak mechanics this reuses.
AC-003.3/.4/.5/.9/.14: `TestRunStartupScan_AllStatesFixtureRecordMatrix`
covers all five branches in one workspace (cannot-fire with a trigger list,
no-schedule without one, an active+armed "observed" info record, a
not-active routine regardless of its schedule state, and a forced-`unknown`
routine), and asserts both the `unknown` observation-counter increment and
the `classification_unknown` skip-counter increment fire together
(intentional double counting per the spec).
AC-003.6/.7: covered throughout via `armingScanObservationsTotal`/
`armingScanSkippedTotal` counter-delta assertions.
AC-003.8: `TestRunStartupScan_WorkspaceEnumerationFailure` — one warning,
`workspace_enum_failed` incremented, no routine ever reached.
AC-003.10: the scan never returns an error or a value main.go could act on;
launched via `go`, it cannot block or fail startup by construction.
AC-003.11: the scan only calls the two new read-only listers and Task 01's
read-only trigger reader; no write path exists on `StartupScanReader`.
AC-003.12: `TestRunStartupScan_IdempotentOverUnchangedData` runs the scan
twice with the same fixed `now` and compares each run's log records
(level, message, context fields) via `reflect.DeepEqual`, excluding
wall-clock fields by never logging any.
AC-003.13: `TestRunStartupScan_PerWorkspaceRoutineEnumerationFailureContinues`
— 3 workspaces, the middle one's routine enumeration forced to fail; asserts
the earlier and later workspaces' records are preserved, the scan continues
past the failure, and `routine_enum_failed` increments exactly once.

New files:
`apps/backend/internal/office/repository/sqlite/routine_arming_scan.go`,
`apps/backend/internal/office/routines/arming_metrics.go`,
`apps/backend/internal/office/routines/startup_scan.go`,
`apps/backend/internal/office/routines/startup_scan_test.go` (in-package
`routines` test file, needed to assert directly on the unexported expvar
counters, following `internal/orchestrator/office_stall_visibility_test.go`'s
precedent for the same reason).

Verification:

- `go build -tags fts5 ./...` — clean.
- `go test -tags fts5 ./internal/office/routines/... ./internal/office/infra/... ./internal/backendapp/...`
  — all `ok`.
- `go test -tags fts5 ./internal/office/repository/sqlite/...` — one
  pre-existing, unrelated failure (`TestMigrate_PriorityIdempotent`,
  confirmed during Task 01; not touched by this task).
- `gofmt -l` on the new/changed files — clean.
- `golangci-lint run ./internal/office/infra/... ./internal/backendapp/... ./internal/office/routines/... ./internal/office/repository/sqlite/... --new-from-rev=<merge-base>`
  — `0 issues`.
