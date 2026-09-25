---
id: "01-open-task-workflow-placement"
title: "Reconcile open task workflow placement"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CHANGE-WORKFLOW-001
acceptance_criteria:
  - AC-TASKS-CHANGE-WORKFLOW-001.8
system_design:
  - ../../specs/tasks/system-design/change-workflow.md
---

# Task 01: Reconcile open task workflow placement

## Summary

Make an open task page use the destination task projection and step definitions
after a cross-workflow move. Keep the task page open and update its desktop
stepper as soon as the live move is observed.

## In scope

- Add a failing regression for workflow ID and step ID reconciliation after the
  task leaves `kanban.tasks` and enters a destination snapshot.
- Resolve the task's live placement and ordered workflow steps without allowing
  stale HTTP data or the source-board step fallback to restore source labels.
- Fetch only the resolved task workflow snapshot by ID, even when its workflow
  is absent from the global workflow catalog.
- Extend desktop and phone change-workflow E2E to assert visible destination
  placement before reload, including a move received from another client.

## Out of scope

- Backend changes, form redesign, new mobile stepper, and unrelated cache
  refactoring.

## Acceptance

- After a committed move, an open desktop task shows destination workflow
  steps with the destination step current; the URL and board selection stay put.
- An uncached destination loads its steps, and a stale snapshot or source-board step
  does not restore source placement.
- The phone flow remains usable and shows the task in its destination before
  reload.

## ASCII UI preview

### UI-01: Desktop open task after migration

```text
Before: Task title   [Backlog] - [In Progress] - [Review*] - [Done]
After:  Task title   [Backlog] - [Analysis*] - [Implement] - [Review] - [PR]
```

`*` is the current step. Actual names come from the selected workflow. See
[the full preview](plan.md#ascii-ui-preview). AC-TASKS-CHANGE-WORKFLOW-001.8.

### UI-02: Phone task action and destination placement

```text
Task actions > Change workflow... > Feature / Analysis > Change workflow
Task list and actions reflect Feature / Analysis without reload.
```

The existing phone composition remains. See
[the full preview](plan.md#ascii-ui-preview). AC-TASKS-CHANGE-WORKFLOW-001.8.

## Verification

Run from the repository root, without overlapping E2E suites:

```bash
(cd apps/web && pnpm test -- components/task/task-page-content-helpers.test.ts components/task/task-page-content-workflow.test.ts components/task/task-page-content.test.tsx hooks/domains/kanban/use-workflow-steps-by-id.test.ts hooks/domains/kanban/use-all-workflow-snapshots.test.ts hooks/domains/kanban/use-all-workflow-snapshots-inflight.test.ts hooks/domains/kanban/use-all-workflow-snapshots.signal-gated.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run build:vite)
(cd apps/web && pnpm exec eslint components/task/task-page-content-helpers.ts components/task/task-page-content-workflow.test.ts components/task/task-page-content.tsx components/task/task-page-inner.tsx hooks/domains/kanban/use-workflow-steps-by-id.ts hooks/domains/kanban/use-workflow-steps-by-id.test.ts hooks/domains/kanban/use-all-workflow-snapshots.ts hooks/domains/kanban/use-all-workflow-snapshots.test.ts e2e/tests/task/change-workflow.spec.ts e2e/tests/task/mobile-change-workflow.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/task/change-workflow.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-change-workflow.spec.ts)
```

Run scoped lint on changed frontend files if the implementation modifies linted
source outside the already covered test paths.

## Files changed

- `apps/web/components/task/task-page-content-helpers.ts`
- `apps/web/components/task/task-page-content-workflow.test.ts`
- `apps/web/components/task/task-page-content.tsx`
- `apps/web/components/task/task-page-inner.tsx`
- `apps/web/hooks/domains/kanban/use-all-workflow-snapshots.ts`
- `apps/web/hooks/domains/kanban/use-all-workflow-snapshots.test.ts`
- `apps/web/hooks/domains/kanban/use-workflow-steps-by-id.ts`
- `apps/web/hooks/domains/kanban/use-workflow-steps-by-id.test.ts`
- `apps/web/e2e/tests/task/change-workflow.spec.ts`
- `apps/web/e2e/tests/task/mobile-change-workflow.spec.ts`
- `docs/specs/tasks/system-design/change-workflow.md`

## Dependencies

None.

## Risks

- Snapshot freshness and placeholder step loading need separate assertions so
  a fast local E2E does not mask either path.

## Parallelism

`sequential`

## Inputs

- [Change workflow requirement](../../specs/tasks/requirements/change-workflow.md),
  especially AC-TASKS-CHANGE-WORKFLOW-001.8.
- [Open task workflow projection](../../specs/tasks/system-design/change-workflow.md#open-task-workflow-projection).
- Existing task page helpers, task WebSocket handler, and change-workflow E2E.

## Results

- Focused unit tests: 7 files, 108 tests passed.
- PR fixup regression tests: 4 files, 97 tests passed, covering strict
  projection freshness, task-detail refresh on reconnect, retry after a failed
  placeholder fetch, and valid timestamps on live projection fixtures.
- `pnpm run typecheck` passed.
- `pnpm run build:vite` passed. Vite reported existing chunk-size,
  deprecated option, and ineffective dynamic import warnings.
- Scoped ESLint passed with no warnings.
- Desktop Chromium E2E: 4 tests passed, including an external move event
  received by an open task page.
- Phone mobile-chrome E2E: 1 test passed.
- PR screenshot recapture passed: desktop Chromium 4 tests and phone mobile-
  chrome 1 test. The phone test's first capture attempt hit a tap-stability
  timeout and passed on retry.
- `CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --no-build --project chromium e2e/tests/task/change-workflow.spec.ts`: 4 passed; captured the destination stepper.
- `CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/task/mobile-change-workflow.spec.ts`: 1 passed; captured the destination step action.
- Specification catalog and specification lint passed.
- `git diff --check` passed.
