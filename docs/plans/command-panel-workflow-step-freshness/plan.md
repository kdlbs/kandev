---
created: 2026-08-31
status: done
requirements:
  - REQ-UI-COMMAND-PANEL-TASK-ACTIVITY-001
system_design:
  - ../../specs/ui/system-design/command-panel-task-activity-icons.md
legacy_specs: []
---

# Implementation Plan: Keep Command Panel Workflow Steps Fresh

## Overview

Keep the workflow-step badge in an open command-panel task result aligned with
the same accepted live task projection that already drives the sidebar-parity
activity icon. A search response may contain an older `workflow_step_id` after
a task move, while the Kanban snapshot has the current placement. The result
row must use the current placement without requiring a new search.

The existing activity resolver already applies the timestamp guard that
rejects stale live projections. The implementation will expose its effective
workflow-step ID to the result row and remove the duplicate, HTTP-only badge
lookup.

## Scope

### In scope

- Reuse the accepted live placement for the command-panel workflow-step badge.
- Preserve the existing step name, color, layout, selection, and navigation
  presentation.
- Keep the existing HTTP fallback when live data is absent or stale.
- Add component regression coverage for newer and stale live placements.
- Extend the existing desktop command-panel badge scenario and retain the
  existing phone row coverage for mobile parity.

### Out of scope

- Backend, HTTP, or WebSocket contract changes.
- Changes to task search ranking, result limits, or navigation.
- Changes to badge presence, styling, responsive geometry, or touch targets.
- A second live-data freshness policy or a subscription to session-detail
  streams.

## Technical approach

### Effective placement

- `apps/web/lib/commands/task-result-activity.ts`
  (`TaskResultActivity`, `resolveTaskResultActivity`): return the effective
  workflow-step ID selected from the accepted live projection, falling back to
  `task.workflow_step_id` when the live projection is absent or rejected by
  `currentLiveTask`.
- Keep the existing status-summary revision and task timestamp rules as the
  only freshness decisions. Do not infer placement from activity or session
  state.

### Result-row rendering

- `apps/web/components/command-panel-results.tsx` (`TaskResultItem`): resolve
  activity before reading the step map, then use the effective step ID for the
  existing badge name and color lookup.
- Keep `stepMap` ownership and the current behavior when a step definition is
  not loaded. No API or store shape changes are needed.

### Regression coverage

- `apps/web/components/command-panel-task-activity.test.tsx`: add a failing
  first test with an older HTTP step and a newer live step, then assert the
  live step badge is shown. Add the inverse stale-live case to pin the
  timestamp guard.
- `apps/web/e2e/tests/command-panel.spec.ts`: extend the existing inline task
  search badge scenario to move the task through the API after the initial
  result is available and assert the open result updates to the new step.
- `apps/web/e2e/tests/search/mobile-command-palette-scopes.spec.ts`: keep the
  existing active-task row icon and title-width assertions as the mobile
  composition/parity check; no new mobile layout is introduced.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.4` | Component coverage proves an accepted live placement update is reflected without a new search. |
| `AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.5` | Component and desktop E2E coverage preserve the existing badge presentation and row behavior while changing only stale placement data. |
| `AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.6` | Existing phone E2E coverage continues to assert icon state, title width, and no horizontal overflow. |
| `AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.8` | Component coverage asserts newer live placement wins and stale live placement does not. Desktop E2E covers the live move flow. |

## E2E tests

Use the managed web runner for the existing desktop command-panel spec and
the existing mobile command-palette scope spec. The desktop test can use
`apiClient.listWorkflowSteps` to select a non-start step and
`apiClient.moveTask` to generate the live placement update.

## Work orders

- [x] [Task 01: Refresh workflow-step badges](task-01-refresh-workflow-step-badges.md)

## Verification results

Focused Vitest (28 tests), web typecheck, targeted ESLint, i18n validation,
desktop Chromium E2E (1 test), mobile Chrome E2E (1 test), specification lint,
and diff checks passed.

## Risks

- A live task projection without `updatedAt` follows the existing legacy
  acceptance rule; changing that rule would regress live activity clears.
- The result row can only show a live step label when its step definition is
  already present in the existing `stepMap`; broadening step loading is a
  separate concern.
- The E2E move must wait for the command-panel result to receive the Kanban
  update, not just for the move request to return.
