---
id: "01-persist-schedule-boundary"
title: "Persist replacement schedule boundaries"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATIONS-SETTINGS-001
acceptance_criteria:
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.16
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.18
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.20
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.21
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.22
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.23
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.24
system_design:
  - ../../specs/office/system-design/automations-settings-03.md
---

# Task 01: Persist replacement schedule boundaries

## Summary

Write a schedule replacement boundary with the trigger config. Preserve the
cursor for unchanged definitions and updates outside scheduled triggers.
Use the existing service validation and portable store update.

## In scope

- Apply the design's decoded definition comparison in a small
  `scheduled_config.go` helper. Cover omitted, null, empty, and valid configs.
- Update `Store.UpdateTrigger` atomically with the edit timestamp on schedule
  changes. Preserve creation time and the existing revision behavior.
- Omit cursor writes for unchanged, enabled-only, and non-scheduled updates.
- Add real SQLite service/store regressions and isolated PostgreSQL parity.
- Preserve the existing invalid-schedule validation behavior and error paths.

## Out of scope

- Scheduler observation fencing, admission refactoring, and public documentation.
- New columns, migrations, trigger deletion/recreation, or calendar algorithms.
- General cron equivalence or timezone alias canonicalization.

## Acceptance

1. `TestScheduledTriggerUpdate_ReplacementBoundary` fails before the correction
   because it retains the previous cursor. After the correction, reloaded
   boundaries match each successful replacement save, including nil cursors.
2. Unchanged, disabled, empty, rejected, and non-scheduled cases satisfy the
   linked criteria without losing pending occurrences or changing unrelated state.
3. SQLite and configured PostgreSQL fixtures prove the same update behavior.
   Persistence checks pass, and skipped PostgreSQL evidence is recorded explicitly.

## Verification

Run from the repository root. Capture the focused Red result before the
production change, then run these commands after Green:

```bash
(cd apps/backend && go test -trimpath -race ./internal/automation -run '^(TestScheduledTriggerUpdate_|TestScheduledConfig_)' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
bash scripts/lint-go-changed apps/backend/internal/automation/store.go apps/backend/internal/automation/scheduled_config.go apps/backend/internal/automation/trigger_schedule_update_test.go apps/backend/internal/automation/trigger_schedule_update_postgres_test.go
```

The focused command includes `TestScheduledTriggerUpdate_PostgresParity`.
Configure `KANDEV_TEST_POSTGRES_DSN` to obtain real PostgreSQL evidence. Reuse
the isolated schema fixture and bounded timestamp assertions for that test.
Do not use a fake clock across PostgreSQL network calls.

## Files likely touched

- `apps/backend/internal/automation/store.go`
- `apps/backend/internal/automation/scheduled_config.go` (new)
- `apps/backend/internal/automation/trigger_schedule_update_test.go` (new)
- `apps/backend/internal/automation/trigger_schedule_update_postgres_test.go` (new)

## Dependencies

None. Task 02 consumes this persistent boundary.

## Risks

- Writing a copied cursor on the no-op path loses concurrent evaluation progress.
- Comparing raw JSON resets unchanged definitions.
- Clearing the cursor to nil reuses the old creation anchor.
- A failed write must preserve the old config and cursor together.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/office/requirements/automations-settings.md), linked criteria.
- [Design](../../specs/office/system-design/automations-settings-03.md), Definition
  identity and Persistent boundary.
- `Store.UpdateTrigger`, `Service.UpdateTrigger`, and `validateScheduledConfig`.
- Existing `impossible_schedule_validation_test.go` and `store_postgres_test.go` fixtures.
- `apps/backend/AGENTS.md` and `.agents/skills/tdd/references/backend-tests.md`.

## Results

Completed 2026-11-10.

- Added `scheduled_config.go` with the bounded decoded-definition identity.
  Formatting, member order, surrounding expression whitespace, the empty
  timezone versus `UTC`, and timezone fields ignored by `@every` or an inline
  `TZ=`/`CRON_TZ=` prefix do not replace a schedule.
- `Store.UpdateTrigger` now writes `last_evaluated_at` and `updated_at` from one
  UTC edit timestamp when a scheduled definition changes. Unchanged,
  enabled-only, omitted-config, and non-scheduled updates omit the cursor
  column. Revision increments and validation behavior are unchanged.
- Added SQLite service/store regressions for historical and nil cursors, weekly,
  daily, hourly, descriptor, interval, timezone-field and inline-prefix
  replacements, unreadable saved configs, no-op saves, disabled and cleared
  schedules, rejected validation and persistence failures, and non-scheduled
  triggers.
- Added `TestScheduledTriggerUpdate_PostgresParity` for changed and unchanged
  writes with nil and populated cursors. `KANDEV_TEST_POSTGRES_DSN` is unset in
  this environment, so the test skipped. This is an explicit environment
  limitation, not PostgreSQL behavior evidence.
- Red: `TestScheduledTriggerUpdate_ReplacementBoundary` failed all eight cases
  against the previous store update. Green: the focused race command passed in
  1.620 seconds. SQL guard, store conformance, `go build ./...`, and changed-file
  lint passed.
- Review remediation: the disabled/re-enabled and cleared/non-empty subtests
  now use `testing/synctest` with fixed edit instants, so their expected
  occurrences no longer depend on the wall clock. The focused
  `TestScheduledTriggerUpdate_DisabledAndEmpty` race run passed in 1.146 seconds
  together with the Task 02 ordering regression.
