---
created: 2026-11-11
status: complete
requirements:
  - REQ-TASKS-STATE-CHANGE-EVENTS-001
system_design:
  - ../../specs/tasks/system-design/state-change-events.md
legacy_specs: []
---

# Implementation Plan: Office Task-State Events

## Overview

Restore canonical task-state notifications for Office status and tree controls.
First repair dashboard publication and its orchestrator caller. Then repair
tree cancellation and restoration. Finally prove subscriber behavior and
compatibility with the blocker-failure work.

- Issue: [#5274](https://github.com/kdlbs/kandev/issues/5274).
- Assignee: `carlosflorencio`, assigned and read back on 2026-11-11.
- Investigated checkout and remote `main`:
  `fa0c48b88e774936fd28d81123e16dc96e9a5be3`.
- Initial remote `main` and PR #5277 heads above are the investigation snapshot.

The requirements and system design remain draft. The user explicitly
authorized implementation in this turn, so each work order records its
execution status and verification results below.

## Execution refresh (2026-11-12)

- The task checkout was fast-forwarded from `fa0c48b88e7` to current
  `origin/main` `dfccc77a63e7`. This includes newer task-dependency success
  admission and Office human-completion behavior. Implementation preserves
  those contracts.
- At the latest GitHub check on 2026-11-12 00:49 UTC, `main` is
  `dab0d02e4571c147adf8b9c193db595dbafdd190`.
  It adds unrelated session-recovery changes; the task/Office implementation
  remains based on `dfccc77a63e7` in this workspace.
- PR #5277 remains open and mergeable, with merge state `BLOCKED`, at head
  `aef39e3904f5bb00a62fca518aec3ac71a464938`; GitHub reported `updatedAt`
  `2026-11-12T00:44:32Z`. The blocker-failure subscriber, its tests, and package
  docs exist on that PR head but not on current `main`.
- A coordinated combined checkout at
  `/tmp/kandev-office-task-state-pr5277-latest` applies the task-state changes
  to that exact PR head. The trigger reconciliation keeps `task.state_changed`
  as the sole failure-alert trigger and removes the overlapping direct
  status-reactivity and tree-hold fallbacks. The latest focused and five-package
  race results are recorded in Task 03 below.

## Confirmed root cause

`DashboardService.UpdateTaskStatus` calls the raw task repository through
`shared.TaskStateWriter`. Its persistence path preserves completion guards but
does not publish events. The dashboard publishes `office.task.status_changed`
and canonical `task.updated`, so connected views refresh while state-event
subscribers remain unaware of the transition.

`CancelTaskTree` instead calls the Office repository's raw
`BulkUpdateTaskState`. `RestoreTaskTree` uses the raw task writer. These paths
also bypass canonical state publication.

The orchestrator compensates only for automatic terminal-step completion. It
publishes state events after the dashboard returns. Adding another dashboard
publication without changing that caller creates producer duplicates.
The canonical publisher also owns an activity hook, which overlaps the
dashboard's synchronous activity row.

## Reproduction evidence

Two temporary Go tests used the real task/Office repositories and services with
the shared synchronous memory bus. The dashboard had the production-style raw
task writer, canonical task lifecycle publisher, and Office event bus wired.

| Entry | Persisted transition | Canonical state events |
| --- | --- | --- |
| User status | `IN_PROGRESS -> COMPLETED` | 0, expected 1 |
| Agent runtime status | `IN_PROGRESS -> CANCELLED` | 0, expected 1 |
| Automatic guarded completion | `IN_PROGRESS -> COMPLETED` | 0, expected 1 |
| Approval redirect | `IN_PROGRESS -> REVIEW` | 0, expected 1 |
| Tree cancel | Root `IN_PROGRESS -> CANCELLED`, child `TODO -> CANCELLED` | 0, expected 2 |
| Tree restore | Root `CANCELLED -> IN_PROGRESS`, child `CANCELLED -> TODO` | 0, expected 2 |

Every dashboard case also observed one canonical `task.updated`. This confirms
the publisher fixture worked. The redirect returned `ApprovalsPendingError`.
Both tree cases included an already-cancelled sibling, which stayed cancelled.

Command from `apps/backend`:

```bash
go test -trimpath -tags fts5 ./internal/office/service -run '^TestRepro5274' -count=1 -v
```

The final reproduction failed on the expected missing-event assertions in all
six subtests. The temporary tests are removed after diagnosis. Recreate them
as permanent regressions during the work orders, before production changes.

Minimal reproduction:

1. Use `newAssignmentBlockerFixture` and its `completionService` helper.
2. Put an `IN_PROGRESS` task on the fixture's terminal step.
3. Wire the dashboard's task writer, lifecycle publisher, and event bus.
4. Subscribe to `task.state_changed` and `task.updated`.
5. Call dashboard status `done` and read the persisted task back.
6. Observe `COMPLETED`, one refresh, and zero canonical state events.

The reported live chain, handoff, coordinator, and plugin consequences are
supported by subscriber source traces. They were not reproduced end to end in
this planning turn. Task 03 owns that proof.

## Assumption check and ownership

- Confirmed request: investigate, assign the issue when the repair is clear,
  and create a fix package.
- Verified behavior: the six missing-event paths reproduce on current `main`.
- Intended outcome: apply the issue's existing canonical event contract across
  Office surfaces while preserving gates and downstream policies.
- Existing task dependency behavior already requires the chain to start after
  successful predecessor completion. Its universal publication requirement was
  implicit. The new task-owned requirement makes that contract explicit.
- The task system owns the receipt and event contract. Office exposes mutation
  surfaces and owns its approval, activity, and wake policy.
- No unresolved product choice blocks this package. PR #5277 is an external
  integration dependency, not evidence that its code already exists on `main`.

## Scope

### In scope

- Transaction-derived previous/new states and committed task snapshots.
- Dashboard user, agent, redirect, and automatic status paths.
- Removal of orchestrator compensation and obsolete activity suppression.
- Tree batch cancellation, restore publication, and post-commit error handling.
- Existing event payloads, activity attribution, and refresh notifications.
- Duplicate-suppression and reentry coverage for affected subscribers.
- Combined compatibility with #5277 after both changes are available.

### Out of scope

- New blocker semantics, wake recipients, or parent-completion policy.
- Unrelated task-writer migrations or a new event transport/outbox.
- Hold-graph transaction redesign, new runtime flags, or recovery sweeps.
- Production frontend, copy, navigation, or desktop/phone composition changes.
- Commit, push, PR creation, or persistent Kandev task/session creation.

## Technical approach

Task 01 adds task-owned commit receipts and uses the existing canonical FIFO
publisher from the dashboard. Its receipt distinguishes guard rejection from a
matched no-op. The activity-handled option prevents a second activity row.
The orchestrator then relies on canonical events and transactional admission.

Task 02 moves raw tree state mutation into the task writer. Cancel commits
every selected state and checkout release before notifications. Restore
publishes successful members even if another member later fails.

Task 03 proves the real consumers and reconciles the overlapping failure
triggers from #5277. Its canonical subscriber becomes the sole failure trigger
after status and tree writers publish events. Existing completion cascades
retain their blocker-digest and wave keys.

There is no schema migration. Existing SQLite/PostgreSQL transaction rules,
task serialization, authorization, and bounded publication context apply.

## Tests

| Criteria | Permanent regression file and test |
| --- | --- |
| .2, .3, .9 | `task/repository/sqlite/task_state_commit_test.go`: `TestTaskStateCommit` (unguarded/step/state guards, no-op, missing row, rollback, competing writer). |
| .1-.4, .7, .10 | `office/dashboard/task_status_state_events_test.go`: `TestOfficeStatusStateEvents` (user, agent, automatic, redirect, cancelled caller, activity order and attribution). |
| .9 | `orchestrator/office_task_status_test.go`: update `TestMarkOfficeTaskCompleted_*` to observe one canonical event through the real seam, including concurrent and reentrant delivery. |
| .5, .6, .10 | `office/service/tree_controls_state_events_test.go`: `TestOfficeTreeStateEvents` (mixed states, complete batch visibility, failed cleanup, partial restore, pause/resume). |
| .8, .9 | `backendapp/office_task_state_events_test.go`: `TestOfficeStateEventIntegration` (dependency, parent, handoff, coordinator, plugin, production wiring). |
| .8 | Existing assignment-blocker and children-wave suites, plus `TestOfficeBlockerFailure*` from #5277 after integration. |

Paths in this table are relative to `apps/backend/internal`.
Each work order runs its focused regression red before it changes production.
PostgreSQL uses the existing `KANDEV_TEST_POSTGRES_DSN` convention. Record an
unavailable PostgreSQL service explicitly rather than treating a skip as proof.

## E2E tests

Add `apps/web/e2e/tests/office/task-state-events.spec.ts` using the existing
Office fixture and the `chromium` project. The principal scenario is:

> Office status completion starts a Kanban dependent exactly once.

Create both workflows in the same workspace. Use `apiClient.seedTask` to place
the Office predecessor at its terminal step with state `TODO`. Reuse the
timeline scenario in `office/property-pickers.spec.ts` as the setup pattern.
Verify that setup leaves the predecessor unfinished. Create a Kanban dependent
with a durable start-when-unblocked intent and no session.

Open `/office/tasks/<predecessor-id>` and select Done through
`status-picker-trigger` and `status-picker-option-done`. Observe the completed
status and one dependent session without restart. Only after the automatic
session exists, open `/t/<dependent-id>` and assert a unique mock-agent response
in the visible chat. Opening that view earlier can prepare a session and spoil
the sessionless admission assertion. Repeat the status request through the API
and verify the session count stays one. Reload the dependent view and verify
the same response and session persist.

Include a second pending blocker first. Completing only the Office predecessor
must leave the dependent unstarted. Complete the remaining blocker, then assert
the one launch. Also cover a gated `done` request that persists `REVIEW` and
does not release the dependent. These scenarios cover .1, .4, and .8.

Do not use setup that moves the predecessor into an automatically completing
step: that can publish the missing event before the action under test.
There are no rendered UI changes, so ASCII previews and a new mobile layout
scenario do not apply. Existing clients share the repaired event contract.

## Work orders

- [x] [Task 01: Publish dashboard state changes once](task-01-dashboard-state-events.md)
- [x] [Task 02: Publish tree state transitions](task-02-tree-state-events.md)
- [x] [Task 03: Prove subscribers and blocker compatibility](task-03-subscriber-integration.md)

Execute 01, then 02, then 03 in the primary conversation. No delegation is
authorized. Before combined #5277 verification, resolve its current merge state
and head and read its current requirement/design/package. Use its merged base
or an explicitly coordinated combined checkout. Do not copy a stale PR diff or
claim compatibility from this investigation alone.

## Verification results

- Diagnosis: six expected missing-event failures, using the command recorded
  above. This is reproduction evidence, not completed implementation testing.
- GitHub assignment: read back as `carlosflorencio`.
- Tasks 01 and 02 are done with focused race coverage. PostgreSQL-specific
  assertions were invoked and skipped because `KANDEV_TEST_POSTGRES_DSN` is
  unset.
- Managed Chromium E2E from the plan passed 34 selected tests, including the
  three new Office task-state scenarios.
- A combined race run of `internal/office/service`, `internal/office/scheduler`,
  `internal/orchestrator`, `internal/coordinator`, and `internal/plugins` passed
  against the coordinated #5277 checkout.
- The post-refactor focused race tests passed across task SQLite/service,
  Office dashboard/service/tree repository, and orchestrator packages.
- `make -C apps/backend build`, web typecheck, sqlguard, store conformance, and
  changed-Go lint passed. The lint wrapper timed out at five minutes with zero
  issues; the same package set passed with a 15-minute timeout.
- Public-doc and specification validators passed after the implementation
  updates.
- Task 03 is complete. `TestOfficeStateEventIntegration` passed with `-race`
  in the root checkout and coordinated combined #5277 checkout. It covers
  parent completion, handoff completion, coordinator episodes, scoped plugin
  forwarding, and duplicate delivery through the shared event bus.
- Restore and hold-release retries, caller-cancellation activity, task-state
  commit guards, and the existing committed-dependency regression passed with
  `-race` in both checkouts. The combined test writer was updated to model the
  production guarded already-at-target no-op.
- `make -C apps/backend build` passed in both checkouts at the verified PR
  head. #5277 remains unmerged, so its implementation changes remain in the
  coordinated checkout.
- The full non-race test suite for `internal/backendapp`, Office service and
  dashboard, task SQLite repository, and task service passed. A broader race
  run passed all these packages except the full SQLite repository suite, which
  hit Go's 10-minute package timeout; focused task-state commit tests pass with
  `-race` in both checkouts.
- Design-package checks passed:
  - `python3 scripts/list-docs.py validate`: 450 decisions and 1993 specifications.
  - `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
  - `python3 scripts/lint-spec-files.py --all`: all specification files passed.
  - `node --test scripts/validate-public-docs.test.mjs`: 63 tests passed.
  - `node scripts/validate-public-docs.mjs`: 55 public pages validated.
  - Planned-path `validateCoverage` preflight: all three work orders covered,
    with zero reference errors.

## Documentation checks

Run from the repository root after implementation and after package edits:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/tasks docs/plans/office-task-state-events
git status --short -- docs/specs/tasks docs/plans/office-task-state-events
```

Also run the pure `validateCoverage` export in `.github/scripts/pr-docs.cjs`
against the package, its linked requirement/design, and the planned dashboard
runtime path. This planned-path preflight checks references even though this
design-only diff is exempt from runtime documentation coverage.

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const packageRoot = 'docs/plans/office-task-state-events';
const paths = fs.readdirSync(packageRoot)
  .filter(name => name.endsWith('.md'))
  .map(name => `${packageRoot}/${name}`);
paths.push('docs/specs/tasks/requirements/state-change-events.md');
paths.push('docs/specs/tasks/system-design/state-change-events.md');
const fileContents = Object.fromEntries(
  paths.map(path => [path, fs.readFileSync(path, 'utf8')]),
);
const changedFiles = [
  {
    filename: 'apps/backend/internal/office/dashboard/task_status_persistence.go',
    status: 'modified',
  },
  ...paths.map(filename => ({ filename, status: 'added' })),
];
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({
  ok: result.ok,
  status: result.status,
  workOrders: result.workOrders,
  errors: result.errors,
}, null, 2));
if (!result.ok || result.workOrders.length !== 3) process.exit(1);
NODE
```

Public-doc review found the existing event subject in `docs/public/websocket-api.md`
and plugin subscriptions in `docs/public/plugins-manifest.md`. This design turn
changes internal records only. Task 03 reviews whether a short restored-coverage
note belongs in the existing public event reference when behavior ships.

## Risks

- Publishing inside the current Office completion mutex can deadlock through
  synchronous parent callbacks. Move admission to the existing guarded write.
- A stale pre-write read can report the wrong previous state. Use the commit
  receipt, not `preStatus`, for canonical event identity.
- Activity can duplicate or appear after the first refresh. Keep one owner and
  assert the row exists at each publication boundary.
- Per-member cancellation publication can expose partly live trees and choose
  the wrong escalation recipient. Commit the complete batch first.
- #5277's current head compensates for this defect through three triggers.
  Once combined with repaired writers, retain its canonical state subscriber
  and remove the direct status-reactivity and tree-hold fallbacks. Keep the
  package pending until its integration tests and owned records agree.
