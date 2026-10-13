---
created: 2026-10-13
status: done
requirements:
  - REQ-OFFICE-AUTOMATIONS-SETTINGS-001
system_design:
  - ../../specs/office/system-design/automations-settings-01.md
legacy_specs: []
---

# Implementation Plan: Reject automation schedules with no occurrence

## Overview

Make scheduled automation saves reject non-empty cron schedules with no next
occurrence, and prevent equivalent schedules already in storage from producing
automatic runs. Deliver one sequential work order through the existing cron
helper, save-validation routes, scheduler, and real automation store.

ROOT reviewed the complete four-artifact design package and explicitly released
implementation on 2026-10-13 with an exclusive local-heavy lease. The same
primary session completed this work; no delegates, recursive tasks, new
sessions, or model switches were used.

## Evidence and root cause

ROOT qualified the defect at `94182a41bd8d0abcd57108e3baf3dd2d7dd018f8` using a
disposable independent Go overlay. The actual Go 1.26 race run exited 1:
`Service.AddTrigger` accepted `0 0 31 2 *` in UTC, and the equivalent trigger
persisted directly through `Store.CreateTrigger`, with an active run at the
concurrency cap, produced three distinct persisted skipped runs across three
scheduler minutes. Five controls passed: future/due daily, future leap day,
the satisfiable DOM/DOW OR expression `0 0 31 2 MON`, and future interval.
The package took 0.122s. Evidence is in
`/tmp/kandev-root-impossible-cron-discovery-20261013-xzdzh381/qualified-proof.json`
and `repro.log`; those files are local supporting evidence, not repository
artifacts. Do not read or copy ROOT's private `candidate_test.go`.

`robfig/cron/v3` v3.0.1 `SpecSchedule.Next` returns zero when its bounded
five-year search finds no occurrence. `nextCronFire` currently passes that
result through with a nil error. `validateScheduledConfig` checks only the
error, and `CronScheduler.shouldFire` considers zero due because it is not
after the evaluation instant. Minute dedup permits a fresh skipped run on
each later minute; updating the evaluation cursor cannot make the impossible
schedule satisfiable.

## Scope

### In scope

- AC-OFFICE-AUTOMATIONS-SETTINGS-001.16 through .18 in the
  [owning requirement](../../specs/office/requirements/automations-settings.md).
- Shared no-occurrence error from `nextCronFire`, after existing timezone and
  DST handling, and propagation through existing save and scheduler callers.
- Non-mutating rejection for automation creation with initial triggers,
  trigger addition, and config-bearing trigger updates, including disabled
  triggers. Preserve omission-only enable/disable updates so an old invalid
  row can still be disabled or repaired explicitly.
- Real service/store regressions and a persisted scheduler reproduction;
  accepted schedules and empty configurations remain usable.
- A short public Schedule paragraph when the behavior ships.

### Out of scope

- Retry, lifecycle, admission, workers, concurrency/dedup policy, schema, or
  automatic migration/disable/delete of existing trigger rows.
- Frontend cron regex, calendar engine, preview, markup, copy, or localization
  changes; no preserve/save-race companion fix.
- Office routine scheduling or its distinct cron helper, manual Run now
  semantics, historical run cleanup, and schedules beyond the dependency's
  supported search horizon.
- Broad local backend lint, frontend builds, browser suites, speculative
  verification, and unsolicited review requests.

## Technical approach

The [owning design's What section](../../specs/office/system-design/automations-settings-01.md#what)
defines the shared contract. In `internal/automation/scheduler.go`, convert a
zero final calendar candidate into an error explaining that no occurrence was
found within the supported search horizon. A successful `nextCronFire` result
must be a nonzero occurrence strictly after its anchor. Keep parsing, timezone
prefix precedence, descriptors, intervals, and DST suppression in place.
Do not infer satisfiability with a second calendar algorithm or month/day
regex. In particular, `0 0 31 2 MON` is valid through cron's OR semantics.

`service.go` already calls `nextCronFire` from `validateScheduledConfig` and
preserves trimmed-empty expressions. `validateTriggerConfig` applies it before
`createAutomation` writes any initial automation/trigger rows, before
`AddTrigger` writes, and before config-bearing `UpdateTrigger` writes. Keep
those shared calls; change only relevant contract comments if needed. A mixed
initial trigger list containing one invalid schedule must reject the entire
request before writes. `UpdateAutomation` has no trigger-config field and is
not a new validation route. `managedScheduleTriggerSpecs` in
`managed_schedule_service.go` also consumes the same validator; preserve its
`ErrManagedScheduleInvalid` wrapper rather than adding a second validator.

`CronScheduler.shouldFire` already returns false on helper errors. Existing
persisted impossible schedules therefore take the same error path before
`fire`/`Service.FireTrigger`, with no new run, event, or evaluation-cursor write.
Do not add save validation to the store: direct persistence remains useful for
legacy-row tests. Do not rewrite or disable rows during evaluation; users can
update their config to a valid schedule. A valid sibling trigger continues
through the same scheduler inventory.

Update the old
`TestNextCronFire_UnsatisfiableExpressionReturnsZeroWithoutHanging` assertion to
expect the new zero-plus-error contract, retaining its bounded termination
guard with a fixed calendar anchor. Its earlier comment called this behavior
out of scope; the new requirement explicitly brings it into scope.

## Tests

Independently author permanent tests after implementation release. Use
`newTestService`, the actual SQLite store, explicit fixed timestamps, and the
actual scheduler methods, without a production clock seam or real sleeps.

| Criteria | Permanent regression and observable assertion |
| --- | --- |
| .16 | `impossible_schedule_validation_test.go`: `TestScheduledConfig_NoOccurrenceRejectedWithoutMutation`, subtests for `CreateAutomation`, `AddTrigger`, and `UpdateTrigger`. Assert errors and reload store state, including no partial mixed initial trigger set and unchanged config/enabled/revision/cursor/timestamps after rejected update. |
| .16, .18 | `TestScheduledConfig_CompatibleSchedulesAndEmptyConfig`: real service saves for leap day, DOM/DOW OR, descriptor, interval, valid timezone, empty expression, whitespace expression, `{}`, `null`, and absent config. Empty saves remain not due. `TestManagedScheduleTriggerSpecs_RejectsNoOccurrence` verifies the existing managed-schedule error wrapper. |
| .17 | `scheduler_impossible_cron_test.go`: `TestCronScheduler_PersistedImpossibleScheduleProducesNoRuns`. Persist the impossible trigger and a cap-holding active run; reload the trigger each of three distinct scheduler minutes, call actual `shouldFire` and conditionally `fire`, then inspect `Store.ListRuns` and stored trigger state. Only the seeded run remains; no skipped rows or events and no cursor change. Cover both creation-anchor and existing-evaluation-anchor states. |
| .17, .18 | `TestCronScheduler_ImpossibleTriggerDoesNotBlockValidSibling`: process a persisted impossible trigger alongside a due valid trigger through the production inventory/due/fire path; assert only the valid trigger produces its expected run. Use a fixed common anchor and bound event completion if needed. |
| .18 | `TestCronScheduler_ScheduleOccurrenceControls`: fixed anchors assert daily before/at due, future leap day, valid DOM/DOW OR before/at due, and future interval. Existing `scheduler_cron_test.go` descriptor, timezone-prefix, DST spring-forward/fall-back and interval tests run in the owning package. |
| .16, .17 | `scheduler_cron_test.go`: rename the old unsatisfiable test to `TestNextCronFire_UnsatisfiableExpressionReturnsErrorWithoutHanging`; assert bounded return and zero plus non-nil error for February 30/31. |

## End-to-end evidence and mobile assessment

The backend service/store/scheduler integration tests cross the actual save,
persistence, due, firing, and run-history boundaries. They provide end-to-end
evidence for the changed backend contract. No rendered layout, touch,
navigation, or viewport-dependent interaction changes; desktop and phone
consume the same save result and automation history. No browser E2E or ASCII
UI preview is warranted for this backend-only work order.

## Documentation assessment

`docs/public/automation-and-mcp.md#schedule` already explains arbitrary calendar
expressions and recurring behavior. Add one bounded operator-facing paragraph
in Task 01 explaining rejection of a non-empty schedule with no occurrence in
the supported search horizon and that old impossible schedules stop creating
automatic runs until corrected. Keep empty schedules, leap day, and DOM/DOW OR
semantics clear. The existing page is primarily a how-to guide; no new page,
navigation, screenshots, or broad rewrite is justified. Public docs remain
unchanged during this speculative design checkpoint. README and screenshots
need no change. This local correction uses the established parser boundary;
the pair and tests preserve its rationale, so no ADR is required.

## Work orders

- [x] [Task 01: Reject schedules with no occurrence and exclude persisted rows](task-01-reject-no-occurrence.md)

## Verification results

Design checkpoint, 2026-10-13:

- `python3 scripts/list-docs.py validate`: passed, 380 decisions and 1580 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Repository `validateCoverage` preflight with these four artifacts and the
  planned scheduler path: `covered`, `ok: true`, `errors: []`.
- `git diff --check`: passed; explicit newline/trailing-whitespace checks also
  passed for the two untracked plan files. The shared design is 32,722 bytes,
  below its 32,768-byte ceiling.

Implementation results, 2026-10-13:

- Frozen pnpm workspace installation: passed once, native session 63179 joined
  exit 0. `GOTOOLCHAIN=local go version` confirmed Go 1.26.0.
- The exact focused race command in Task 01: Red exit 1 (session 13894,
  0.448s package); create/add/update wrongly accepted impossible schedules,
  both persisted-anchor cases produced three extra skipped rows, and a mixed
  inventory produced an extra run. Green exit 0 (session 71517, 1.451s package).
- The whole owning `./internal/automation` race command: passed, session 90238
  joined exit 0, 8.388s package. Existing timezone/DST/descriptor controls ran.
- Initial staged lint command: exit 126 before lint startup because the helper
  has tracked mode 100644. This is no lint verdict. The corrected canonical
  command uses `xargs -0 -r bash scripts/lint-go-changed`, as the active hook
  does. It linted the complete automation package and joined exit 0, session
  54268. The work-order command was corrected without changing script mode.
- Catalog and all-spec lint: passed, 380 decisions and 1580 specifications.
- Public documentation validation: passed, 48 published pages.
- PR-documentation coverage preflight: `covered`, `ok: true`, `errors: []`.
- `gofmt -l` on staged Go paths reported no files; diff/plan whitespace and
  plan-status checks passed. Normal active commit hooks and the exact delivery
  receipt are recorded in the durable task Plan before publication.

No full local backend scan, frontend build, browser suite, or extra bot review
was requested. Logs are retained under the owned local evidence directory
`/tmp/kandev-child119-local-20261013-IvL1Lp9U/`; task delivery records retain
actual native joins and hosted monitoring separately from implementation status.

## Risks

- The dependency's five-year search is bounded. Zero means no supported next
  occurrence from this anchor, not a new proof about every future year. Valid
  leap-day controls within that horizon must pass; do not invent a longer
  calendar search to handle century gaps.
- Existing impossible rows remain visible and editable, but do not run
  automatically. Manual Run now and prior erroneous history retain their
  existing behavior.
- The shared design is close to its 32 KiB ceiling. Amend its existing cron
  bullet concisely; preserve unrelated migrated and draft material.
