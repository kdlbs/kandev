---
created: 2026-08-29
status: complete
requirements:
  - REQ-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001
system_design:
  - ../../specs/tasks/system-design/runtime-state-publication-order.md
legacy_specs: []
---

# Implementation Plan: Runtime Task-State Client Reconciliation

## Overview

Preserve the newest task state across delayed workflow snapshots and active
task aggregation. The snapshot merge changes first because it is the stale
write source. The aggregator then provides a second consistency boundary.

## Scope

### In scope

- Preserve a newer live `state` and `updatedAt` during workflow snapshot merge.
- Use task update time to select task-level data during sidebar aggregation.
- Select status-summary data through its independent revision contract.
- Add deterministic regression tests for both race boundaries.

### Out of scope

- Backend task-state publication changes.
- New state values, event payloads, or database fields.
- Changes to State grouping, labels, icons, or task-row layout.
- New desktop or mobile interaction behavior.

## Technical approach

### Workflow snapshot merge

Update `mergeFetchedTask` in
`apps/web/hooks/domains/kanban/use-all-workflow-snapshots.ts`.

Compare the mapped snapshot task with the current cached task by `updatedAt`.
If the cached task is newer, preserve its `state` and `updatedAt`. Keep the
existing independent merge rules for placement, status summary, executor
binding, autopilot, and auto-start errors.

### Sidebar aggregation

Update the task precedence logic in
`apps/web/components/task/task-session-sidebar-aggregate.ts`.

Use `updatedAt` to choose task-level data. Use status-summary revision only to
choose the status summary. On equal task update times, keep the current active
fallback behavior.

### Mobile parity

Desktop and mobile already share `aggregateSidebarTasks` and `applyView`.
`session-task-switcher-sheet.tsx` is the nearest mobile surface.

This change does not alter composition, navigation, scrolling, safe areas,
pointer behavior, or touch targets. The shared unit tests satisfy the mobile
parity rule for state-only normalization.

## Tests

- **AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.3:** Add a deferred snapshot
  test in
  `apps/web/hooks/domains/kanban/use-all-workflow-snapshots-inflight.test.ts`.
  The response has `REVIEW` with an older update time. The live projection has
  `IN_PROGRESS` with a newer update time.
- **AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.4:** Add aggregation tests in
  `apps/web/components/task/task-session-sidebar-aggregate.test.ts`. Equal
  status-summary revisions must not make an older `REVIEW` snapshot win, and
  equal task timestamps must retain the active projection.
- **AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.6:** The same aggregation tests
  cover the data path that the desktop sidebar and mobile task switcher share.
- Add the inverse freshness case so a newer snapshot remains authoritative.
- Add failed-refresh coverage for both placeholder and resolved snapshots.

## E2E tests

The existing scenario in `apps/web/e2e/tests/chat/clarification.spec.ts` covers
`AC-TASKS-RUNTIME-STATE-PUBLICATION-ORDER-001.5`. It proves that State grouping
moves a running task without a reload.

No new Playwright test will simulate the HTTP and WebSocket race. The deferred
unit test controls that order without scheduler timing. This change qualifies
for the mobile-parity state-normalization exception.

## Work orders

- [x] [Task 01: Reconcile task-state projections](task-01-reconcile-task-state-projections.md)

## Verification results

- Focused Vitest suite: 29 tests passed.
- Focused ESLint: passed with no warnings.
- Web TypeScript typecheck: passed.
- Existing clarification Playwright scenario: 1 Chromium test passed.
- Specification lint suite and diff checks: passed.

## Risks

- Missing or invalid update times can make freshness ambiguous. The existing
  fallback behavior must remain stable.
- A whole-task precedence change can erase independently newer summary data.
  The implementation must merge status summary separately.
- Over-preserving task state can block a legitimate newer snapshot. The
  regression tests must cover both timestamp directions.
