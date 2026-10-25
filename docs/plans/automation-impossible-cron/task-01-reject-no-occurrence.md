---
id: "01-reject-no-occurrence"
title: "Reject schedules with no occurrence and exclude persisted rows"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATIONS-SETTINGS-001
acceptance_criteria:
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.16
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.17
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.18
system_design:
  - ../../specs/office/system-design/automations-settings-01.md
---

# Task 01: Reject schedules with no occurrence and exclude persisted rows

## Summary

Make the shared next-occurrence helper reject a non-empty schedule with no
supported future occurrence. Prove save rejection leaves the actual store
unchanged, and legacy persisted impossible schedules cannot reach automatic
firing or create skipped history. Preserve accepted cron and empty-config behavior.

## In scope

- One correction in the shared `nextCronFire` contract and relevant comments;
  reuse `validateScheduledConfig`, `validateTriggerConfig`, and `shouldFire`.
- All existing initial-create, add, and config-update save-validation paths,
  plus the managed-schedule adapter's existing wrapper.
- The independently authored permanent regressions named in the
  [plan's Tests table](plan.md#tests), including actual service/store and
  persisted scheduler/run-history assertions and positive controls.
- A bounded public Schedule clarification, final manifest/work-order results,
  and normal delivery once the reviewed implementation is authorized.

## Out of scope

No retries, lifecycle, admission/dedup redesign, schema/migration, store-level
validation, UI/calendar/regex changes, historical-run purge, companion race
fix, new flag, Office routine changes, or manual-fire behavior change. No
delegation, new task/session/tab, protected-resource cleanup, or model switch.

## Acceptance

1. `nextCronFire` returns zero plus an error for a parsed calendar schedule
   whose final candidate is zero; errors reach save validation before writes.
   Empty raw/null/object/expression and whitespace cases keep their existing
   unscheduled behavior. Create with mixed valid/invalid initial triggers
   persists nothing; rejected add/update preserves the reloaded original state.
2. Three scheduler minutes over an impossible persisted row create no runs or
   events and do not advance its cursor, whether anchored to creation or a
   stored evaluation. A due valid sibling remains eligible and fires normally.
3. Valid leap day and DOM/DOW OR are accepted, valid occurrences are due at
   their instant but not before, and the whole automation package retains its
   descriptor/interval/timezone/DST controls. Public docs explain rejection and
   correction of old rows. All required checks have actual joined results.

## Implementation sequence

Only after ROOT's later explicit instruction, mark this order `in_progress`.
Author tests without ROOT's private candidate source. First run the focused
regressions against unchanged production code and retain the real Red result:
save wrongly succeeds and the persisted scheduler path creates skipped rows.
Correct the shared helper, update the previous zero/nil assertion and relevant
contract comments, then run Green and the complete owning package.

Use fixed calendar instants for helper/due tests. For persisted scheduler
tests, seed deterministic `CreatedAt`/`LastEvaluatedAt`, reload before each
check, and conditionally call actual `fire` only when `shouldFire` says due.
Use the actual event bus and store; no stubbed next-occurrence implementation.
Assert both the predicate and the persisted consequence, not only no error.
Keep resources test-owned and register cleanup immediately. Use synchronized
completion for any async bus callbacks; no timer sleep to infer no events.

Verify existing config validation remains shared on `createAutomation`,
`AddTrigger`, `UpdateTrigger` with non-nil Config, and
`managedScheduleTriggerSpecs`. No validation of omitted config is added to
metadata-only updates. Never auto-delete/disable invalid existing rows.

Add this concise public guidance under the existing Schedule section, adapted
only if implementation evidence requires it: a non-empty expression must have
a next occurrence within the supported five-year search horizon; schedules
such as February 31 are rejected on save. Already-saved impossible schedules
do not create automatic runs; edit them to a valid schedule. Empty schedules
remain unscheduled, and valid leap days and cron day-of-month/day-of-week OR
semantics remain supported. Do not claim a preview/UI change or erase history.

## Verification

Run from the repository root. No commands below run during the design turn.
ROOT grants the serial heavy-work lease before dependency install, Go tests,
lint, or commit hooks. In this fresh worktree run frozen pnpm install once
before package commands or commits; preserve shared dependencies and caches.
Use the existing local Go 1.26 toolchain, never the `/usr/bin/go` default.

```bash
export PATH="/home/jcfs/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.0.linux-amd64/bin:/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH"
(cd apps && pnpm install --frozen-lockfile)
env PATH="/home/jcfs/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.0.linux-amd64/bin:$PATH" GOTOOLCHAIN=local go version
```

Use this focused command for Red, then Green after the minimal correction:

```bash
(cd apps/backend && timeout --kill-after=10s 280s env PATH="/home/jcfs/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.0.linux-amd64/bin:$PATH" GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -race -trimpath -tags=fts5 -p 1 ./internal/automation -run 'TestScheduledConfig_|TestManagedScheduleTriggerSpecs_RejectsNoOccurrence|TestCronScheduler_PersistedImpossible|TestCronScheduler_ImpossibleTrigger|TestCronScheduler_ScheduleOccurrenceControls|TestNextCronFire_Unsatisfiable' -count=1 -timeout=240s)
```

Then run the whole owning automation package once:

```bash
(cd apps/backend && timeout --kill-after=10s 280s env PATH="/home/jcfs/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.0.linux-amd64/bin:$PATH" GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -race -trimpath -tags=fts5 -p 1 ./internal/automation -count=1 -timeout=240s)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
/home/jcfs/.nvm/versions/node/v24.18.0/bin/node scripts/validate-public-docs.mjs
git diff --check
git diff --check -- docs/plans/automation-impossible-cron
git status --short -- docs/plans/automation-impossible-cron
```

Stage the actual owned new/changed Go files before the canonical initial local
lint. The command consumes the staged filenames and lints their complete
packages for type information; CI owns the full backend scan. Do not add local
`golangci-lint ./...` unless ROOT authorizes it for a concrete full-backend CI
finding. Run normal active hooks on commit; no bypass. If formatting changes
staged files, restage and create a new commit, without amend.

```bash
git diff --cached --name-only -z -- '*.go' | env PATH="/home/jcfs/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.0.linux-amd64/bin:$PATH" GOTOOLCHAIN=local xargs -0 -r gofmt -l
git diff --cached --name-only -z -- '*.go' | timeout --kill-after=10s 360s env PATH="/home/jcfs/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.0.linux-amd64/bin:$PATH" GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB xargs -0 -r bash scripts/lint-go-changed
```

Check repository PR-documentation coverage with the four linked artifacts:

```bash
/home/jcfs/.nvm/versions/node/v24.18.0/bin/node <<'JS'
const fs = require('node:fs');
const validator = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/office/requirements/automations-settings.md',
  'docs/specs/office/system-design/automations-settings-01.md',
  'docs/plans/automation-impossible-cron/plan.md',
  'docs/plans/automation-impossible-cron/task-01-reject-no-occurrence.md',
];
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = paths.map(filename => ({filename, status: 'modified'}));
changedFiles.push({filename: 'apps/backend/internal/automation/scheduler.go', status: 'modified'});
const result = validator.validateCoverage({changedFiles, fileContents});
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.status !== 'covered' || result.errors.length) process.exit(1);
JS
```

Retain every native session handle and join its actual exit; never infer
completion from duplicate runs. Stop and report unknown execution/resource
state to ROOT. Only clean up current known-owned resources, then return the
heavy lease after actual joins. Do not overlap heavy commands or run broad
frontend/backend verification beyond the named checks.

## Files likely touched

- `apps/backend/internal/automation/scheduler.go`
- `apps/backend/internal/automation/service.go` (contract comments only if needed)
- `apps/backend/internal/automation/scheduler_cron_test.go`
- `apps/backend/internal/automation/impossible_schedule_validation_test.go` (new)
- `apps/backend/internal/automation/scheduler_impossible_cron_test.go` (new)
- `docs/public/automation-and-mcp.md` (Schedule paragraph only)
- The owning requirement/design pair and this plan/work order.

## Dependencies

None. ROOT design review, later explicit implementation instruction, and serial
heavy-work admission precede execution. The existing reviewed requirements and
shared parser settle the repair's behavior; no material open choice remains.

## Risks

Preserve the five-year horizon rather than claiming an unbounded calendar
proof. Preserve DOM/DOW OR, leap-day and DST controls. Existing rows stay
editable and visible; old run history is not removed. Keep the shared design
within its size ceiling without rewriting unrelated legacy sections.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/office/requirements/automations-settings.md), criteria .16-.18.
- [System design](../../specs/office/system-design/automations-settings-01.md#what).
- `apps/backend/internal/office/AGENTS.md`, backend guidance, and
  `.agents/skills/tdd/references/backend-tests.md`.
- Existing scheduler, cron, service-validation, and managed-schedule tests.
- ROOT's qualified proof metadata/log referenced by the plan; never private candidate code.

## Results

Completed 2026-10-13 after ROOT's full design review and explicit implementation
release. `nextCronFire` now rejects its zero final candidate; all existing save
and scheduler callers reuse that error without new calendar, lifecycle, or
storage behavior. Independent regressions cover real create/add/config-update
no-write results, persisted run history over three minutes under both anchor
states, a valid sibling reaching actual publication, empty configurations,
disabling/repairing an old impossible row, leap-day and DOM/DOW OR controls.

Actual joined results:

- Frozen pnpm install once: session 63179, exit 0. Toolchain confirmed Go 1.26.0.
- The focused Verification race command: session 13894 Red exit 1, package
  0.448s, on impossible acceptance and three skipped rows per persisted case;
  session 71517 Green exit 0, package 1.451s.
- The whole owning-package Verification race command: session 90238, exit 0,
  package 8.388s. It includes existing descriptors, intervals, timezone-prefix,
  spring-forward and fall-back controls.
- Initial staged helper launch exited 126 before the linter ran: no lint
  verdict. The helper is tracked non-executable; its mode was preserved. The
  corrected documented `xargs -0 -r bash scripts/lint-go-changed` command
  analyzed the complete staged automation package, session 54268 joined exit 0.
- Staged Go formatting list: empty. Catalog: passed (380 decisions/1580
  specifications). All-spec lint: passed. Public docs: 48 pages validated.
- PR-documentation coverage: `covered`, `ok: true`, `errors: []`. Diff and
  plan whitespace checks passed; both plan files are accounted for by
  plan-status checks (this package has one manifest and one work order).

Normal active commit hooks and the exact delivery receipt are recorded in the
durable task Plan before publication. Implementation checks are complete; PR
observation and ROOT's later expected-head merge authorization remain separate
delivery gates. No protected resources or shared caches were cleaned up.
