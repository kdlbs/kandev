---
created: 2026-11-10
status: implemented
requirements:
  - REQ-OFFICE-AUTOMATIONS-SETTINGS-001
system_design:
  - ../../specs/office/system-design/automations-settings-03.md
legacy_specs: []
---

# Implementation Plan: Safe automation schedule replacement

## Overview

Make a replacement cron definition start after its successful save. Preserve
pending occurrences for unchanged definitions. Deliver the persistent boundary
first, then serialize automatic admission against edits.

This package addresses [issue #5220](https://github.com/kdlbs/kandev/issues/5220).
The issue is assigned to `carlosflorencio`. Implementation is complete. Both
work orders ran sequentially in the primary session.

## Evidence and root cause

Investigation used repository revision
`974831c8ae3f99ddac601fac28972786a9a48351` and Go 1.26.0 on Linux.
Two temporary tests exercised the real automation service and SQLite store.
The tests used a fixed `testing/synctest` clock and no live automation data.

`TestRepro5220_ScheduleEditReusesPreviousCalendarHistory` failed with the
expected assertion. Its state was:

| State | UTC value |
| --- | --- |
| Original definition | `0 9 * * 6` |
| Last evaluation | 2026-11-07 09:00:00 |
| Replacement save | 2026-11-09 09:19:03 |
| Replacement definition | `9 9 * * 6` |
| Next occurrence from retained cursor | 2026-11-07 09:09:00 |
| Scheduler check | 2026-11-09 09:19:25 |
| Correct next occurrence after save | 2026-11-14 09:09:00 |

`Service.UpdateTrigger` delegates to `Store.UpdateTrigger`, which retains
`last_evaluated_at`. `CronScheduler.shouldFire` applies the replacement expression
to that old cursor. It reports the historical Saturday occurrence as due on
Monday.

`TestRepro5220_StaleSchedulerSnapshotFiresAfterEdit` also failed as expected.
A snapshot of Monday `19 9 * * 1` was due at 09:19:03. After the service saved
Saturday `9 9 * * 6`, `CronScheduler.fire` admitted one run and published one
`AutomationTriggered` event from the old snapshot. Resetting the cursor alone
cannot close this save-overlap case.

Temporary reproduction files are removed before the design handoff. Permanent
regressions belong to the implementation work orders.

## Scope

### In scope

- Amend AC-OFFICE-AUTOMATIONS-SETTINGS-001.20 through .27 in the
  [existing requirement](../../specs/office/requirements/automations-settings.md).
- Replace schedule history atomically when its definition changes.
- Preserve no-op, enabled-only, and non-scheduled updates.
- Reject obsolete automatic observations before admission side effects.
- Preserve calendar semantics, ordinary catch-up, dedup, capacity pacing,
  and already admitted runs.
- Add focused save, persistence, scheduler, ordering, and public-action tests.
- Document the result when implementation ships.

### Out of scope

- Historical run cleanup, retroactive correction of past edits, or production
  database changes during investigation.
- Catch-up controls, new columns, migrations, API fields, runtime flags,
  distributed scheduler ownership, or a general admission rewrite.
- Office routines, model discovery in #5218, retries, provider launch behavior,
  new UI controls, copy, localization, or browser clock hooks.
- Changing managed-schedule trigger replacement or plugin webhook gate semantics.

## Technical approach

The [schedule replacement design](../../specs/office/system-design/automations-settings-03.md)
defines the persistent and admission boundaries.

Task 01 changes `Store.UpdateTrigger` in
`apps/backend/internal/automation/store.go`. A small scheduled-config helper
compares decoded definitions with the design's bounded normalization. A changed
scheduled definition writes `last_evaluated_at` and `updated_at` with one edit
timestamp in the existing update. Other updates omit the cursor column.
Keep `trigger_revision` increments and service validation intact.

Task 02 adds an internal automatic admission path to the existing service.
`CronScheduler.fire` supplies its observed revision. Under `automationRunLock`,
reload and validate the current trigger, then reuse existing admission logic.
The cursor write completes under that lock. Event publication stays outside it.
Extract small helpers in `scheduled_admission.go` and `scheduler.go` as needed.
Manual and other trigger callers keep their current interfaces and behavior.

No schema expansion is needed. PostgreSQL uses the same portable update through
`Rebind`. Managed schedule updates already create fresh trigger IDs and anchors.
Rejecting their deleted snapshots requires no change to that replacement path.

## Tests

All paths below are under `apps/backend/internal/automation/`. Tests use fixed
instants or a fake clock, real persistence, and deterministic ordering barriers.

| Criteria | Planned tests and evidence |
| --- | --- |
| .20, .18 | `trigger_schedule_update_test.go`: `TestScheduledTriggerUpdate_ReplacementBoundary`. Cover historical and nil cursors, weekly/daily/hourly/descriptor/interval edits, and timezone changes. Reload the stored boundary. |
| .21, .26 | `TestScheduledTriggerUpdate_UnchangedDefinition`. Cover omitted config, equivalent JSON, surrounding whitespace, default UTC, ignored timezone fields, and an enabled-only save. Preserve a pending occurrence. |
| .22, .23 | `TestScheduledTriggerUpdate_DisabledAndEmpty`. Cover disabled edits, re-enabling, clearing an expression, and a later non-empty replacement. |
| .24, .16 | `TestScheduledTriggerUpdate_RejectedEditPreservesState`. Cover invalid expressions, invalid effective timezones, and a failed persistence statement. Assert unchanged config, enabled state, revision, timestamps, and cursor. |
| .20, .21 | `trigger_schedule_update_postgres_test.go`: `TestScheduledTriggerUpdate_PostgresParity`. Use `testutil.OpenIsolatedPostgres` for changed and unchanged writes with nil and populated cursors. |
| .20, .18, .26 | `scheduler_schedule_replacement_test.go`: `TestCronScheduler_ReplacementWaitsForNextOccurrence` and `TestCronScheduler_UnchangedScheduleCatchUp`. Assert no early rows/events, the expected first run, and no immediate repeat. Include Monday New York, spring-forward, fall-back, and exact-edit-instant cases. |
| .25 | `TestCronScheduler_StaleSnapshotAfterEdit`, `TestCronScheduler_DisabledOrDeletedSnapshot`, and `TestCronScheduler_StaleCursorSnapshot`. Obsolete automatic work creates no rows, skipped rows, dedup effects, events, or cursor changes. |
| .25, .27 | `TestCronScheduler_EditAdmissionOrdering`. Exercise both lock orderings and an edit during post-admission publication. Preserve an admitted run and prevent its older cursor write from overwriting the edit. |
| .20, .21, .25 | `handlers_schedule_replacement_test.go`: `TestAutomationTriggerUpdate_ScheduleReplacement`. Drive `wsUpdateTrigger`, reload through the store, and evaluate/fire through the actual scheduler and event bus. |
| .21, .27 | `TestScheduledTriggerUpdate_NonScheduledCursor` plus existing scheduler capacity/dedup/manual tests. Preserve adjacent admission behavior. |

New tests use `@covers` annotations for the acceptance criteria they prove.
Place tests in new files instead of extending oversized legacy test files.

## End-to-end evidence and UI assessment

The public-action integration crosses WebSocket update, service validation,
persistence, scheduler due evaluation, admission, and event publication. It
proves the changed backend contract without an external agent or browser.
Desktop and phone use the same public update action. No rendered composition,
navigation, touch control, or viewport-specific state changes in this package.

## Documentation assessment

Task 02 updates `docs/public/automation-and-mcp.md#schedule` and its
troubleshooting entry for missing scheduled runs. The existing how-to page
already explains creation anchors and single catch-up execution. Add the edit
boundary beside those rules, and preserve their distinction.

Public documentation remains unchanged during this planning turn. The root
README and screenshot catalog need no changes. This local correction needs no
separate ADR because its requirement, design, and tests retain the rationale.

The implemented `automation-impossible-cron` package owns schedule validation.
Its scope and results remain valid. This package adds replacement semantics
and references its existing validation and calendar controls.

## Work orders

- [x] [Task 01: Persist replacement schedule boundaries](task-01-persist-schedule-boundary.md)
- [x] [Task 02: Serialize automatic admission with edits](task-02-fence-scheduled-admission.md)

Task 02 depends on Task 01. Execute sequentially in the primary session.

## Verification results

Design checkpoint:

- Both temporary reproduction commands failed on their intended assertions.
- Existing calendar and validation controls passed with exit 0:
  `(cd apps/backend && go test -trimpath ./internal/automation -run '^(TestNextCronFire_|TestCronScheduler_(PinnedExpression|DSTFallBack|ScheduleOccurrenceControls)|TestScheduledConfig_)' -count=1)`.
  Package time was 0.108 seconds. This was a focused diagnostic control run,
  without the race detector.
- `python3 scripts/list-docs.py validate`: passed, with 447 decisions and
  1969 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, with 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Documentation coverage preflight from Task 02: `covered`, `ok: true`, and
  `errors: []` for both work orders, including planned backend source paths.
- `git diff --check -- docs/specs docs/decisions docs/plans/automation-schedule-replacement`:
  passed. Explicit whitespace and final-newline checks covered all untracked
  package documents.
- Temporary reproduction tests were removed. The worktree contains only the
  requirement amendment and four new package documents, all unstaged.

Implementation checkpoint:

- Task 01 Red: the focused replacement-boundary test failed all eight cases
  before the store change. Historical and nil cursors were retained instead of
  the edit boundary.
- Task 01 Green: `(cd apps/backend && go test -trimpath -race ./internal/automation -run '^(TestScheduledTriggerUpdate_|TestScheduledConfig_)' -count=1)` passed in 1.620 seconds.
  `TestScheduledTriggerUpdate_PostgresParity` is present but skipped because
  `KANDEV_TEST_POSTGRES_DSN` is unset. The skipped test is not PostgreSQL
  behavior evidence.
- Task 02 Red: the stale-snapshot regression admitted one run and one event;
  the stale-cursor regression admitted two runs.
- Task 02 Green: `(cd apps/backend && go test -trimpath -race ./internal/automation -run '^(TestCronScheduler_(Replacement|Unchanged|Stale|DisabledOrDeleted|EditAdmission)|TestAutomationTriggerUpdate_ScheduleReplacement)' -count=1)` passed in 1.344 seconds.
- Whole owning package: `(cd apps/backend && go test -trimpath -race ./internal/automation -count=1)` passed in 10.150 seconds.
- `go run ./cmd/sqlguard ./internal` and
  `go test -trimpath -race ./internal/persistence/storeconformance -count=1`
  passed. `go build -trimpath ./...` passed.
- `bash scripts/lint-go-changed` reported 0 issues for every changed Go file.
- Public docs regression passed 63 tests and validated 55 pages. Catalog
  validation passed with 447 decisions and 1969 specifications; all-spec lint
  passed 36 tests and the full file set.
- Documentation coverage preflight returned `covered`, `ok: true`, and
  `errors: []` with both work orders accepted.
- Review remediation: the disabled/cleared schedule fixture now uses a fixed
  fake clock, and the post-admission publication case advances to a later edit
  instant so an older cursor write cannot satisfy its assertion. The focused
  race run for both regressions passed in 1.146 seconds.
- `git diff --check` passed.

## Risks

- Resetting every save would suppress overdue work. Definition identity must
  exclude no-op formatting and ignored timezone fields.
- `UpdatedAt` is not a schedule-only timestamp. Using it as every anchor
  would change enabled-only and unchanged-save behavior.
- A cursor write outside the admission lock can overwrite a newer edit.
  Publishing under that lock can deadlock subscriber callbacks.
- Old edits cannot be reconstructed safely. This package protects future
  replacements and does not rewrite existing history on startup.
- PostgreSQL coverage requires `KANDEV_TEST_POSTGRES_DSN`. A skipped test is
  not PostgreSQL behavior evidence.
