---
id: "03-subscriber-integration"
title: "Prove subscribers and blocker compatibility"
status: done
wave: 3
depends_on:
  - "01-dashboard-state-events"
  - "02-tree-state-events"
plan: "plan.md"
requirements:
  - REQ-TASKS-STATE-CHANGE-EVENTS-001
acceptance_criteria:
  - AC-TASKS-STATE-CHANGE-EVENTS-001.1
  - AC-TASKS-STATE-CHANGE-EVENTS-001.4
  - AC-TASKS-STATE-CHANGE-EVENTS-001.5
  - AC-TASKS-STATE-CHANGE-EVENTS-001.7
  - AC-TASKS-STATE-CHANGE-EVENTS-001.8
  - AC-TASKS-STATE-CHANGE-EVENTS-001.9
system_design:
  - ../../specs/tasks/system-design/state-change-events.md
---

# Task 03: Prove Subscribers and Blocker Compatibility

## Summary

Prove that real subscriber wiring receives Office state changes and preserves
its existing occurrence semantics. Add the reported Office-to-Kanban chain as
  an isolated E2E regression and reconcile #5277's compensating triggers.

## In scope

- Refresh `main`, #5277's state/head, its requirement/design, and companion
  package before choosing the integration base. The recorded head is a
  snapshot. Combined verification requires its merged code or a coordinated
  combined checkout.
- Add `TestOfficeStateEventIntegration` through production-style service wiring
  for dependency launches, parent handling, handoffs, coordinator completion,
  and plugin forwarding. Do not synthesize a replacement canonical event after
  calling the dashboard in these tests.
- Cover the universal condition: all blockers must succeed. Include mixed
  completed/pending and completed/failed predecessors and the existing WIP and
  deferred-launch claim behavior.
- Keep successful blocker and parent-wave occurrence keys. Exercise canonical
  delivery before and after dashboard reactivity and repeat canonical delivery.
- Once #5277 coexists with Tasks 01/02, retain its state subscriber and remove
  its direct status-reactivity and tree-hold fallback failure triggers. Keep its
  classifier, recipient, payload, and occurrence-key policy.
- Exercise repeated cancellation, `FAILED -> CANCELLED`, retry then re-failure,
  separate blockers, and in-tree versus outside dependents. Assert run counts
  and recipients, not just event counts.
- Reconcile Office scheduler/task design wording about publication and
  reactivity. Update the related #5277 package if its owned scope changes.
- Add the E2E cases named in the plan. Drive the existing Office status picker
  and assert the dependent's visible mock-agent response after its automatic
  session exists. Keep API evidence as support for the session count.
- Review the public event reference for a small restored-coverage note when the
  behavior ships.

## Out of scope

- Changing #4548's policy, new subscriber features, or a generic local audit.
- Production frontend and mobile layout changes.

## Acceptance

1. Office completion releases an eligible Kanban dependent once without
   restart. It does not launch while any blocker remains unresolved or after
   an approvals-pending redirect. Existing subscriber effects occur once per
   existing occurrence.
2. Combined #5277 tests show one failure wake per occurrence with the same
   recipient and payload. Internal tree dependents stay excluded and a later
   re-failure creates its own occurrence.
3. Targeted integration/E2E checks pass from fresh builds. Every referenced
   package records current results, with no skipped compatibility claim.

## Verification

Run backend commands from the repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/backendapp -run '^TestOfficeStateEventIntegration' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/office/service ./internal/office/scheduler ./internal/orchestrator ./internal/coordinator ./internal/plugins -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Before the package-wide Office check, verify that
`apps/backend/internal/office/service/blocker_failure_escalation_test.go` exists
from #5277 and that the run discovers `TestOfficeBlockerFailure*` tests. If it
is unavailable, retain the compatibility work as pending. Do not replace it
with a skipped green result.

For the web E2E in a fresh worktree, bootstrap dependencies once:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --project chromium tests/office/task-state-events.spec.ts tests/office/task-status.spec.ts tests/office/subtree-controls.spec.ts tests/office/property-pickers.spec.ts tests/task/task-dependencies.spec.ts)
```

The managed runner builds current backend/web assets and owns teardown.
Keep its one-worker-per-shard resource policy. Do not overlap full suites.
Record discovered test counts and actual results.

Run changed-Go lint, the planned-path documentation coverage preflight from
the plan, and package status/diff checks. If public docs change, also run:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/backend/internal/backendapp/office_task_state_events_test.go` (new).
- `apps/backend/internal/office/service/assignment_blocker_resolution_test.go`
  and focused handoff tests where fixture publication currently compensates.
- `apps/backend/internal/office/scheduler/reactivity.go`, `task_completion.go`,
  and focused blocker/children-wave tests.
- After #5277 is available: `office/scheduler/blocker_failure.go`, its
  `office/service/blocker_failure_escalation_test.go`, and its wiring test.
- `apps/backend/internal/orchestrator/office_task_status_test.go` as required
  for real publisher expectations.
- `apps/backend/internal/coordinator/wake_recorder_test.go` and focused plugin
  event tests if consumer-specific coverage needs their local fixtures.
- `apps/web/e2e/tests/office/task-state-events.spec.ts` (new).
- `docs/specs/office/system-design/scheduler-01.md` and `tasks-01.md`.
- `docs/public/websocket-api.md` if restored coverage needs a reference note.
- Current package results and affected #5277 implementation records.

## Dependencies

Tasks 01 and 02. Combined blocker-failure validation also depends on #5277 or
its successor. Confirm that dependency from live GitHub evidence.

## Risks

- Existing test helpers manually publish the event after dashboard completion
  and can conceal missing production wiring.
- Automatic completion during E2E setup can release the dependent before the
  status action. Require an unfinished predecessor immediately before it.
- A live dependent inside a partly cancelled tree changes escalation policy.
- Different keys across overlapping producers can create two runs despite
  one canonical event. Assert durable run rows and their keys.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/state-change-events.md).
- [Design](../../specs/tasks/system-design/state-change-events.md), subscriber
  compatibility and verification sections.
- [Plan E2E scenarios](plan.md#e2e-tests).
- Current #5277 source and its `blocker-terminal-policy` requirement/design.
- Existing Office fixture, task dependency E2E, and subscriber test patterns.

## Results

Complete. The user-visible dependency chain, combined blocker-failure triggers,
production-wired cross-consumer event integration, and affected #5277-owned
design/package records are verified.

- `TestOfficeAssignmentBlockerCommittedCompletion` passes with the real
  task-service publisher wired into the dashboard. Duplicate deliveries still
  leave the existing blocker-resolution run count unchanged.
- Added `apps/web/e2e/tests/office/task-state-events.spec.ts` with three
  scenarios: Office completion starts a Kanban dependent once and preserves
  its visible response, another pending blocker prevents launch, and an
  approval redirect to Review keeps the dependent blocked.
- The managed Chromium command in this work order passed all 34 selected tests,
  including all three new scenarios.
- The E2E cases were added after the Task 01/02 backend regressions had already
  served as the red gate; they now verify the behavior through the production
  build and browser path.
- Latest GitHub check on 2026-11-12 02:00 UTC: PR #5277 is open and mergeable,
  with `mergeStateStatus=BLOCKED`, at head
  `aef39e3904f5bb00a62fca518aec3ac71a464938` (updated 00:51 UTC). Its subscriber
  and tests are absent from current `main`.
- A coordinated combined checkout at
  `/tmp/kandev-office-task-state-pr5277-latest` applies this work to the exact
  PR head. It retains the canonical `task.state_changed` subscriber as the
  only failure-alert trigger and removes the overlapping direct status and
  tree-hold triggers. Focused blocker-failure race tests pass there.
- The combined five-package race command has passed the Office service and
  scheduler, orchestrator, coordinator, and plugins packages.
- The combined checkout's #5277 system design, plan, and Task 02 work order now
  describe the single canonical failure trigger and receipt-based Office state
  publication.
- `TestOfficeStateEventIntegration` exercises the real dashboard task-state
  writer and publisher, shared memory event bus, orchestrator watcher, Office
  subscribers, coordinator wake recorder, and scoped plugin event bridge. It
  covers parent advancement, one handoff report, one coordinator completion
  episode, plugin forwarding, and duplicate delivery. The race-enabled test
  passes in both the root checkout and the coordinated #5277 checkout.
- Partial-restore and hold-release retries, post-commit caller-cancellation
  activity, and task-state commit guard regressions pass with `-race` in both
  checkouts. The existing `TestOfficeAssignmentBlockerCommittedCompletion`
  also passes with `-race` in both checkouts.
- The combined test writer now models the already-at-target guarded no-op used
  by production restoration retries; no unrelated #5277 production changes
  were overwritten.
- `make -C apps/backend build` passes in the root checkout and the combined
  checkout at the verified PR head.
- Changed-Go lint for the new backendapp integration test reports zero issues.
- The full non-race suite for `internal/backendapp`, `internal/office/service`,
  `internal/office/dashboard`, `internal/task/repository/sqlite`, and
  `internal/task/service` passes.
- The broader race run passed `internal/backendapp`, Office service/dashboard,
  and task service. The full `internal/task/repository/sqlite` race suite hit
  Go's 10-minute package timeout; the focused task-state commit guard tests
  pass with `-race` in both checkouts.
- #5277 remains unmerged; its combined source and package edits remain in the
  coordinated checkout.
