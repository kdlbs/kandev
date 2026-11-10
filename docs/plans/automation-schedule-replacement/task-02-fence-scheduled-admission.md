---
id: "02-fence-scheduled-admission"
title: "Serialize automatic admission with edits"
status: completed
wave: 2
depends_on:
  - "01-persist-schedule-boundary"
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATIONS-SETTINGS-001
acceptance_criteria:
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.18
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.20
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.21
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.22
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.23
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.24
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.25
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.26
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.27
system_design:
  - ../../specs/office/system-design/automations-settings-03.md
---

# Task 02: Serialize automatic admission with edits

## Summary

Prevent an automatic observation from admitting work after an edit invalidates
that observation. Keep the cursor write inside the admission lock and event
publication outside it. Prove the complete public update-to-scheduler behavior.

## In scope

- Pass the scheduler's observed trigger revision through an internal automatic
  admission path. Preserve public manual firing behavior.
- Under `automationRunLock`, reload and validate the automation, trigger,
  revision, enabled state, plugin-event exclusion, and current due status.
- Reuse existing dedup, capacity, and `admitTriggerLocked` logic. Move the
  successful cursor update under the same lock without changing its error policy.
- Add calendar, stale-observation, ordering, catch-up, and WebSocket integration
  regressions with the real store and event bus.
- Update the Schedule and troubleshooting paragraphs in
  `docs/public/automation-and-mcp.md` after the behavior passes.
- Record both work orders' results and reconcile the design-package status.

## Out of scope

- New cron semantics, a new scheduler, distributed claims, or admission policy.
- Cancelling admitted runs, provider launches, retries, or historical cleanup.
- UI changes, browser instrumentation, localization, or managed-schedule replacement changes.

## Acceptance

1. `TestCronScheduler_StaleSnapshotAfterEdit` fails before the correction with
   one run and one event. After the correction, obsolete observations have
   no side effects, while the next valid saved occurrence admits one run.
2. Deterministic ordering tests prove both lock orderings and cursor monotonicity
   across edits. Existing dedup, capacity-skip pacing, failure, and manual behavior
   pass without nested locking or publication under the admission mutex.
3. Public-action integration, calendar controls, and documentation checks pass.
   Record actual command results and retain pending status for any unresolved
   acceptance condition.

## Verification

Run from the repository root. Capture the focused Red result before the
production change, then run these commands after Green:

```bash
(cd apps/backend && go test -trimpath -race ./internal/automation -run '^(TestCronScheduler_(Replacement|Unchanged|Stale|DisabledOrDeleted|EditAdmission)|TestAutomationTriggerUpdate_ScheduleReplacement)' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/automation -count=1)
bash scripts/lint-go-changed apps/backend/internal/automation/service.go apps/backend/internal/automation/scheduler.go apps/backend/internal/automation/scheduled_admission.go apps/backend/internal/automation/scheduler_schedule_replacement_test.go apps/backend/internal/automation/handlers_schedule_replacement_test.go
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/automation-schedule-replacement
```

Run the repository documentation-coverage preflight with every referenced
document and the complete changed file set. The planned source paths also make
this command useful before implementation:

```bash
node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const directory = 'docs/plans/automation-schedule-replacement/';
const documents = [
  directory + 'plan.md',
  directory + 'task-01-persist-schedule-boundary.md',
  directory + 'task-02-fence-scheduled-admission.md',
  'docs/specs/office/requirements/automations-settings.md',
  'docs/specs/office/system-design/automations-settings-03.md',
];
const modified = execFileSync('git', ['diff', '--name-only', 'HEAD', '-z'], { encoding: 'utf8' });
const added = execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' });
const planned = ['store.go', 'scheduler.go', 'service.go'].map(name => 'apps/backend/internal/automation/' + name);
const changedFiles = [...new Set([...modified.split('\0'), ...added.split('\0'), ...planned].filter(Boolean))];
const fileContents = Object.fromEntries(documents.map(path => [path, fs.readFileSync(path, 'utf8')]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

Record its accepted work orders and errors.
Include Task 01's PostgreSQL result or its explicit environment
limitation. Do not count a skipped PostgreSQL test as executed coverage.

## Files likely touched

- `apps/backend/internal/automation/scheduler.go`
- `apps/backend/internal/automation/service.go`
- `apps/backend/internal/automation/scheduled_admission.go` (new)
- `apps/backend/internal/automation/scheduler_schedule_replacement_test.go` (new)
- `apps/backend/internal/automation/handlers_schedule_replacement_test.go` (new)
- `docs/public/automation-and-mcp.md`
- This plan, both work-order Results sections, and the paired design lifecycle.

## Dependencies

Task 01 must provide the persistent replacement boundary first.

## Risks

- Fresh-row validation outside the admission lock leaves the save-overlap gap open.
- Advancing a cursor after unlocking can overwrite a later replacement boundary.
- A stale cursor can survive without a revision change after another firing.
  Due status must also use the fresh cursor.
- Event subscribers can call service mutations. Publication under the admission
  mutex can deadlock those callbacks.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/office/requirements/automations-settings.md), linked criteria.
- [Design](../../specs/office/system-design/automations-settings-03.md), Admission
  ordering, Calendar and recovery, and Verification and documentation.
- `CronScheduler.evaluate`, `shouldFire`, and `fire`.
- `fireTriggerWithMatchedTriggers`, `admitTrigger`, and `admitTriggerLocked`.
- Existing `scheduler_test.go`, `scheduler_cron_test.go`, and
  `scheduler_impossible_cron_test.go` controls and event subscriptions.
- Existing `handlers_test.go` WebSocket request pattern.
- Task 01 Results and `docs/public/automation-and-mcp.md#schedule`.

## Results

Completed 2026-11-10 after Task 01.

- Added `scheduled_admission.go`. `CronScheduler.fire` now passes the observed
  trigger revision, automation ID, trigger ID, evaluation instant, trigger
  data, and dedup binding through an internal automatic-admission path.
- Under `automationRunLock`, admission reloads the automation and trigger,
  requires matching identity, scheduled type, observed revision, enabled state,
  no `plugin_event` condition, and a fresh due check. Obsolete observations
  create no run, skipped row, event, dedup effect, or cursor write.
- The successful evaluation-cursor write moved into `admitTriggerLocked`, so an
  admitted firing or capacity skip advances it while the lock is held. Dedup
  and admission errors still do not. Publication and subscriber callbacks run
  after the lock is released.
- Added regressions for replacement calendar cases (weekly, Monday New York,
  spring-forward, fall-back, exact edit instant), unchanged-schedule catch-up,
  stale revision, stale cursor, disabled and deleted snapshots, both edit
  orderings, an edit during post-admission publication, and the public
  WebSocket update action through the real scheduler and event bus.
- Adapted the existing daily capacity-pacing control to seed a due evaluation
  anchor, because automatic admission now rechecks due status from fresh state.
- Red: the stale-snapshot regression admitted one run and one event, and the
  stale-cursor regression admitted two runs. Green: the focused race command
  passed in 1.344 seconds and the whole package passed in 10.150 seconds.
- Changed-file lint reported 0 issues. Public docs passed 63 tests and 55
  pages; catalog validation, 36 specification-validator tests, all-spec lint,
  documentation coverage (`covered`, `ok: true`, `errors: []`), and
  `git diff --check` all passed.
- Review remediation: the post-admission publication regression now advances
  the fake clock to a distinct later edit instant inside the subscriber. The
  assertion distinguishes that replacement boundary from the older evaluation
  timestamp, so a delayed cursor rollback would fail. The focused ordering and
  disabled-schedule race run passed in 1.146 seconds; changed-file lint reported
  0 issues.
- PR #5245 fixup: added a two-goroutine admission/edit barrier regression that
  pauses admission after its fresh read and proves the edit waits for the
  admission lock. Separated edit and observation instants in the stale-snapshot
  and edit-wins cursor assertions, split the public schedule paragraph into a
  lead, bullets, and the clearing rule, and added docstrings to the new helpers
  and tests. The focused race suite passed in 2.373 seconds and the whole
  package passed in 11.869 seconds.
- No acceptance condition remains pending. PostgreSQL evidence remains the
  Task 01 environment limitation recorded there.
