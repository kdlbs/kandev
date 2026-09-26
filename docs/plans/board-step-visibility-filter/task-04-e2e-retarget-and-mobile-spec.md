---
id: "04-e2e-retarget-and-mobile-spec"
title: "Retarget the E2E spec, add lane retention, add the mobile spec"
status: done
wave: 3
depends_on: ["01-columns-menu-on-swimlane-header", "02-retain-lanes-with-hidden-steps", "03-phone-columns-home"]
plan: "plan.md"
spec: "../../specs/ui/requirements/board-step-visibility-filter.md"
---

# Task 04: Retarget the E2E spec, add lane retention, add the mobile spec

## Acceptance

- `apps/web/e2e/tests/kanban/step-visibility-filter.spec.ts` drives
  `columns-menu-<workflowId>` / `columns-menu-step-<stepId>` instead of
  `display-button` / `steps-filter-step-<stepId>`, and its per-workflow-isolation
  and persistence-across-reload scenarios keep their assertions.
- That spec gains a lane-retention scenario: in All-Workflows view with another
  workflow still populated, hiding every step of a workflow leaves its lane and
  `columns-menu-<workflowId>` trigger present, and a re-tick restores the column
  and its tasks. Its two comments describing the old drop behaviour (~line 45,
  ~line 266) are corrected.
- `apps/web/e2e/tests/kanban/mobile-step-visibility-filter.spec.ts` exists in the
  mobile-chrome project and asserts the drawer toggle, the phone board result,
  a ≥ 44px row height on `columns-menu-step-<stepId>`, and
  `document.documentElement.scrollWidth <= clientWidth`.

## Verification

```
cd apps/web && pnpm e2e:raw --project=chromium --grep "step visibility"
cd apps/web && pnpm e2e:raw --project=mobile-chrome --grep "step visibility"
```

Both must pass. Narrow further with `--grep` if a run does not finish inside a
turn; do not background the run and end the turn on it.

## Files likely touched

- `apps/web/e2e/tests/kanban/step-visibility-filter.spec.ts`
- `apps/web/e2e/tests/kanban/mobile-step-visibility-filter.spec.ts` (new)
- `apps/web/e2e/pages/kanban-page.ts` (helper for the new trigger, if one fits)

## Dependencies

Tasks 01–03. The retention scenario cannot pass before Task 02, and the mobile
spec cannot pass before Task 03.

## Risks

- Seeding must use the existing `apiClient` helpers (`createWorkflow`,
  `listWorkflowSteps`, `createWorkflowStep`, `createTask`, `saveUserSettings`)
  the way `workflow-filter.spec.ts` does; hand-rolled seeding drifts from the
  rest of the suite.
- On the phone, columns are all mounted inside `SwipeableColumns` and revealed
  one at a time — assert `kanban-column-<stepId>` **count**, not visibility, and
  address tabs by step title, never by index.

## Inputs

- Spec section `E2E requirement` (all three chromium scenarios and the mobile
  spec), plus the `(R2)` scenarios it verifies
- Plan section `Testing → E2E`
- `apps/web/e2e/README.md` for the `mobile-*.spec.ts` convention and project
  names; `/e2e` for the suite's conventions

## Output contract

Report both exact command results with their pass/fail lines, the corrected
comments, files changed, and any scenario that could not be covered and why.
Mark this task `done` and tick its plan checkbox.
