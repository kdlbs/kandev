---
created: 2026-09-26
status: complete
requirements:
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-002
system_design:
  - ../../specs/tasks/system-design/workflow-signal-gated-manual-move-visibility.md
legacy_specs: []
---

# Implementation Plan: Keep the next-step control in the task's workflow

## Overview

An idle task in the Feature workflow can show `Analysis` in the top stepper but
lose the `Implement` action above chat when the selected board uses another
workflow. The header resolves the task's workflow snapshot; the composer hook
reads only the selected board's `kanban.workflowId`, `kanban.steps`, and
`kanban.tasks`. Correct the shared composer projection so its label, preview,
and move payload use the task's own workflow. One sequential work order owns
the projection and desktop/phone proof.

The affected live task was in Feature/Analysis with Implement next and a
`WAITING_FOR_INPUT` primary session. A temporary focused test placed that task
only in its Feature workflow snapshot while Kanban was selected. The command
below failed before any production change: expected `Implement`, received
`null`. The temporary test was removed after the reproduction.

```bash
(cd apps && pnpm --filter @kandev/web exec vitest run hooks/domains/kanban/use-plan-actions.test.ts -t 'uses the task workflow when a different board is selected')
```

## Scope

### In scope

- Derive the next-step label, preview target, work-step classification, and
  manual move payload from the task's current workflow and adjacent step.
- Recompute after the task's workflow snapshot hydrates or changes, including
  navigation between tasks in different workflows without a full page reload.
- Keep the standard chat and passthrough composer controls consistent on
  desktop and phone.
- Add a permanent focused regression that fails before the correction and
  browser flows that prove the button can move a task while another board
  workflow is selected.

### Out of scope

- Backend workflow transitions, `step_complete_kandev`, task-move API shape,
  persistence, and manual completion-signal fallback.
- New copy, buttons, layouts, gestures, or mobile navigation.
- Relaxing the existing busy, clarification, auto-transition, or backend move
  eligibility gates.

## Technical approach

- In `apps/web/hooks/domains/kanban/use-plan-actions.ts`, make
  `useNextWorkflowStep` read the task's latest projection across the active
  Kanban list and `kanbanMulti.snapshots` using the existing
  `resolveLatestTaskProjection` convention from the task page. Select
  `kanban.steps` only when the task's workflow matches `kanban.workflowId`;
  otherwise select the task workflow's snapshot steps. Never combine a step ID
  from one workflow with the step list or move target from another.
- Keep the existing `currentStepAutoTransitions` policy and `proceed()` error
  path. When task identity or steps are unavailable, return no action and let
  store hydration trigger a normal recomputation. Do not request or move a
  task from the hook merely to fill the gap.
- Add a permanent test to `use-plan-actions.test.ts` with the proven state:
  selected Kanban board, Feature task and steps only in the Feature snapshot.
  Assert the label, preview target, and `moveTask` payload. Cover a workflow
  switch and an absent/placeholder snapshot so a stale board step cannot leak.
- Extend the browser workflow proceed coverage with a client-side task switch
  from one workflow to a task in another, then assert and use the existing
  control. Add a matching phone flow or extend an existing mobile workflow
  proceed spec. Reuse the existing `proceed-next-step` test ID and task API
  polling for the persisted move.

## ASCII UI preview

### UI-01: Task composer, Feature/Analysis task while Kanban is selected

Current desktop behavior, confirmed by the screenshot and focused reproduction:

```text
Task header:  Backlog  [Analysis]  Implement  Review
Chat:         (idle conversation)
              [ Continue working on the task...          ]
```

Required desktop behavior:

```text
Task header:  Backlog  [Analysis]  Implement  Review
Chat:         (idle conversation)
                                      [ Implement -> ]
              [ Continue working on the task...          ]
```

Required phone behavior uses the existing chat composer and touch-sized action:

```text
Task: Feature / Analysis
Chat: (idle conversation)
                          [ Implement -> ]
      [ Continue working on the task... ]
```

The button remains in its current status row. Desktop keeps its hover options;
phone keeps the current tap and long-press Drawer behavior. The drawing's
spacing is illustrative. The task-owned label and target are required by
`AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-002.6`; busy and clarification
visibility remain governed by `.3` and `.5`. The plan changes no scroll owner,
safe-area treatment, or control size.

## Tests

- `AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-002.6`:
  `apps/web/hooks/domains/kanban/use-plan-actions.test.ts`, `uses the task
  workflow when a different board is selected`, must be the red test. Add
  assertions for the preview and move payload in the same scenario.
- `.6` and `.4`: the same test file covers snapshot arrival, task switch, and
  missing snapshot without borrowing the selected board's steps.
- `.1` through `.5`: keep the existing hook, composer, and passthrough tests
  passing so signal, automatic, busy, and clarification gates retain their
  behavior.

## E2E tests

- `AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-002.6`: extend
  `apps/web/e2e/tests/workflow/workflow-step-proceed.spec.ts` with a Chromium
  case that selects workflow A, navigates within the SPA to an idle task in
  workflow B, sees B's next-step label, clicks it, and polls the task API for
  B's target step. The top stepper and composer must agree before the click.
- Add `apps/web/e2e/tests/workflow/mobile-workflow-step-proceed.spec.ts` under
  `mobile-chrome` for the same cross-workflow task switch and touch action.
  Reuse the current mobile composer surface; no new phone control is planned.

## Work orders

- [x] [Task 01: Resolve composer next step from the task workflow](task-01-resolve-task-workflow-next-step.md) (done)

The prior [signal-gated proceed package](../signal-gated-workflow-proceed-control/plan.md)
introduced the visibility policy. This repair preserves its completed scope
and adds task-workflow identity to the shared projection.

## Verification results

Implementation is complete. The composer now resolves the freshest task
projection and uses only the step list from that task's workflow. Missing or
placeholder snapshots suppress the action until the task workflow hydrates.

- Focused hook, chat-input, and passthrough tests: 4 files passed, 96 tests.
- Frontend typecheck and scoped ESLint passed.
- Chromium and `mobile-chrome` cross-workflow switch-and-move browser tests
  passed against the managed production build.
- Documentation catalog, spec-linter tests, full spec lint, and scoped diff
  checks passed.
- Public workflow docs already describe manual moves and signal recovery; no
  public doc change was needed for this bug fix.

## Risks

- A task can move between workflows while an older snapshot remains cached.
  Pair the freshest task projection with steps from that projection's workflow
  and never use the selected board as a fallback.
- A placeholder or failed snapshot can temporarily have no usable steps. Hide
  the action until the task's workflow steps arrive; do not offer a wrong move.
- Recomputing the label without changing the move payload would show the
  correct button but submit the selected board's workflow. Test both values.
