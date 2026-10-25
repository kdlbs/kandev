---
id: "01-responsive-browser-contract"
title: "Responsive browser contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/adaptive-kanban.md"
---

# Task 01: Responsive browser contract

## Acceptance

- Desktop E2E coverage expresses the wide-versus-windowed board contract, direct lane scrolling,
  internal horizontal overflow, preview-driven adaptation, and long-parent subtask containment.
- The responsive E2E coverage proves the 700px tablet composition remains separate from the new
  desktop lane window.
- The focused RED run fails for the expected missing adaptive lane/relationship behavior, not
  from fixture setup, compilation, or an unrelated assertion.

## Files likely touched

- `apps/web/e2e/tests/layout/compact-desktop-responsive.spec.ts`
- `apps/web/e2e/tests/kanban/kanban-board.spec.ts`

## Inputs

- Spec `Scenarios`: wide desktop, constrained desktop, direct lane navigation, preview-driven width,
  long parent title, and tablet parity.
- `apps/web/e2e/pages/kanban-page.ts` for existing card and column locators.
- Existing `compact-desktop-responsive.spec.ts` 900px composition and `kanban-board.spec.ts` preview
  width scenario.

## Dependencies

None.

## Parallelism

Sequential. This establishes the RED contract consumed by Task 02.

## Verification

Run the managed production-build test command and confirm the named new assertions fail because the
desktop lane window and relationship line are not implemented yet:

```bash
cd apps/web && pnpm e2e:run tests/layout/compact-desktop-responsive.spec.ts tests/kanban/kanban-board.spec.ts
```

## Risks

- Use bounding-box relationships instead of hard-coded card widths.
- Preserve existing test names unless a behavior description genuinely changes; do not duplicate the
  existing preview setup merely to obtain a RED failure.

## Output contract

Report the E2E scenarios added, the exact expected RED failures, files changed, blockers, and risks;
mark this task and its plan item complete while leaving the product implementation untouched.
