---
created: 2026-10-27
status: implemented
requirements:
  - REQ-TASKS-BOARD-BULK-MOVE-SCOPE-001
  - REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-002
  - REQ-TASKS-KANBAN-TASK-REORDERING-001
system_design:
  - ../../specs/tasks/system-design/board-priority-sort-filter.md
  - ../../specs/tasks/system-design/kanban-task-reordering.md
legacy_specs: []
---

# Implementation Plan: Kanban bulk-move workflow scope

## Overview

Correct the workflow sent with a selected destination step by deriving and
validating scope from the selected tasks and snapshots. Complete the single
work order after independent causal failing tests establish the regression.

## Scope

### In scope

- Desktop All Workflows, single-workflow control, and the existing phone
  selection and bulk-actions sheet.
- Fail closed on mixed, missing, ambiguous, or conflicting eligible task and
  destination evidence. Preserve one batch, eligibility, ordering, success,
  selection, and backend error semantics.
- Prevent a phone touch on the destination trigger from dispatching a move.

### Out of scope

- Cross-workflow moves, backend contract changes, per-task fanout, board-context
  redesign, TanStack migration, or new user-facing copy.

## Technical approach

`useEffectiveWorkflowContext` passes the hydrated workflow to
`useTaskMultiSelect` on desktop, while `useMultiSelectDerived` discovers the
selected workflow from `kanbanMulti.snapshots`. Thus a workflow B selection
can offer a B step but submit workflow A, or no request when A is null.
`validateTaskMove` rejects a step belonging to another workflow. Scope the
batch at invocation to the unique workflow of every eligible task and verify
the target step belongs to that same workflow using current snapshots and the
matching hydrated single-workflow data where necessary. Detect contradictory or
ambiguous evidence rather than choosing the first match. Align offered steps
and the move guard while preserving the existing board context for selection
reset and other actions. The backend remains authoritative for validation and
placement; retain its batch request and existing local update on success.

A retained phone trace shows a batch request for the current step during the
trigger tap, before the intended destination tap. It does not capture the raw
DOM event target. A later review found task-bearing placeholders can have no
steps; matching hydrated-board steps now remain available. Its RED test failed
before the correction, and six board tests, thirteen hook cases, scoped lint,
and a bounded 6 GiB typecheck passed after it. In `TaskMultiSelectToolbar`, follow the controlled touch
trigger used by `RepositorySourceMenu`: prevent default on pending touch
pointerdown, clear pending on cancellation, and toggle on the synthetic click.
Keep the phone sheet as modal owner, the nested phone menu non-modal, and the
desktop menu modal with its existing mouse and keyboard behavior.

## Tests

Create `apps/web/hooks/use-task-multi-select-workflow.test.tsx` independently:
mount a real state provider and the real board derivation and move hooks, mocking
only the batch API boundary and unrelated side effects. Capture requests for B
selected with hydrated A and with null, plus the matching A control. Both B cases
must fail for the observed wrong/missing request before production changes.
Add assertions for mixed, missing, duplicate/conflicting task and target
scope, wrong-workflow destination, priority-filter eligibility, display order,
one request, selection retention and backend rejection. Keep existing focused
hook and board tests, adapting their fixtures to provide real scope evidence.
Add focused toolbar coverage for touch pointerdown/click and cancellation, and
for desktop mouse and keyboard opening and selection.

## E2E tests

- `apps/web/e2e/tests/kanban/task-multi-select.spec.ts`, Chromium: select B
  on All Workflows with A also present, move to B's step, assert actual task
  workflow and step; retain the single-workflow and order controls.
- `apps/web/e2e/tests/kanban/mobile-task-multi-select-workflow.spec.ts`,
  mobile-chrome: focus a workflow, select through the phone control, open the
  existing bulk-actions sheet, and tap the move trigger. Assert zero batch
  requests at that point and visible choices, then tap the intended step and
  assert exactly one batch with the intended workflow and step plus the
  persisted task workflow and step.
  These cover AC-TASKS-BOARD-BULK-MOVE-SCOPE-001.1 and 001.3. If infrastructure
  prevents a run, record the exact blocker and do not claim browser proof.

## Work orders

- [x] [Task 01: Correct bulk-move workflow scope](task-01-bulk-move-scope.md)

## Verification results

The independent routing regression failed for hydrated A and null while the
matching A control passed. The desktop regression also failed before the fix.
The retained phone trace showed a current-step request on opening; three phone
attempts failed before the controlled touch correction. After the correction,
the touch and cancellation regressions failed before production changes and
passed afterward. Final focused tests passed (15 scope/hook and 7 toolbar);
three desktop browser controls and the phone browser outcome passed after the
last code change. Scoped lint, localization ratchet, and 6 GiB typecheck passed.
The initial 4 GiB typecheck exhausted its heap. The trace does not prove the raw
DOM event target. A later review found task-bearing placeholders can have no
steps; matching hydrated-board steps now remain available. Its RED test failed
before the correction, and six board tests, thirteen hook cases, scoped lint,
and a bounded 6 GiB typecheck passed after it.

## Risks

- Snapshot hydration may be incomplete or temporarily contradictory; silently
  choosing a stale workflow could move a task across workflows. Fail closed.
- The server owns final placement and may reject a move after client validation.
  Keep its error and selection behavior intact.
- Public `tasks-and-workflows.md` mentions hidden-step destination behavior but
  has no bulk workflow routing instructions. This repair restores that flow;
  recheck public docs at implementation and update only if user guidance needs it.
