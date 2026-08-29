---
id: "01-reconcile-task-state-projections"
title: "Reconcile task-state projections"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001
acceptance_criteria:
  - AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.3
  - AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.4
  - AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.5
  - AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.6
  - AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.7
system_design:
  - ../../specs/tasks/system-design/runtime-state-publication-order.md
---

# Task 01: Reconcile Task-State Projections

## Summary

Prevent delayed workflow snapshots from restoring an old task state. Make the
sidebar aggregator select task state and status summary with separate clocks.

## In scope

- Add the two regression tests before production changes.
- Preserve newer task state in the workflow snapshot merge.
- Correct task-level precedence in the sidebar aggregator.
- Preserve independently newer status-summary data.

## Out of scope

- Backend lifecycle or event-order changes.
- New browser interactions or responsive layout changes.
- Changes to unrelated task fields or sidebar grouping rules.

## Acceptance

- A delayed `REVIEW` snapshot cannot replace a newer live `IN_PROGRESS` state.
- A newer snapshot can replace an older active task state.
- Equal summary revisions do not override task update-time ordering, and equal
  task timestamps retain the active projection.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm test -- \
  hooks/domains/kanban/use-all-workflow-snapshots-inflight.test.ts \
  components/task/task-session-sidebar-aggregate.test.ts)
(cd apps/web && pnpm exec eslint \
  hooks/domains/kanban/use-all-workflow-snapshots.ts \
  hooks/domains/kanban/use-all-workflow-snapshots-inflight.test.ts \
  components/task/task-session-sidebar-aggregate.ts \
  components/task/task-session-sidebar-aggregate.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run tests/chat/clarification.spec.ts -- \
  --grep 'moves answered task from Review to In progress without reload')
```

## Files likely touched

- `apps/web/hooks/domains/kanban/use-all-workflow-snapshots.ts`
- `apps/web/hooks/domains/kanban/use-all-workflow-snapshots-inflight.test.ts`
- `apps/web/components/task/task-session-sidebar-aggregate.ts`
- `apps/web/components/task/task-session-sidebar-aggregate.test.ts`

## Dependencies

None.

## Risks

- Task and summary freshness use different clocks. A whole-object replacement
  can regress one clock while it repairs the other.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001` and its acceptance criteria.
- The runtime task-state publication system design.
- Existing in-flight snapshot, aggregator, and hydration merge tests.

## Results

- Added deferred-response coverage that reproduces and prevents a stale
  workflow snapshot from replacing newer live task state.
- Added inverse timestamp coverage so a newer workflow snapshot remains
  authoritative.
- Changed sidebar aggregation to select task-level fields by `updatedAt` and
  status summary by its independent revision.
- Verified the shared desktop/mobile data path with 29 focused unit tests,
  focused ESLint, web typecheck, and the existing clarification Playwright
  scenario.
