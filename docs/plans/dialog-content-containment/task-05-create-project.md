---
id: "05-create-project"
title: "Contain Create Project form"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/dialog-content-containment.md"
design: "../../specs/ui/system-design/dialog-content-containment.md"
---

# Task 05: Contain Create Project Form

## Outcome

The Office Create Project form scrolls when repository chips or form content
grow, while Cancel/Create Project remain visible on desktop and phone.

## In scope

- Add desktop and mobile-Chrome browser regressions that add enough repository
  chips through the existing picker to exceed the dialog viewport.
- Make `ProjectFormBody` the bounded scroll body and keep the existing footer in
  the final persistent row.
- Add stable dialog/body/footer selectors and touch-safe phone actions.
- Preserve repository picker wiring and create payload behavior.

## Exclusions

- Do not change Office project APIs, form fields, defaults, validation, or
  repository semantics.
- Do not change the project detail repository picker.
- Do not change the shared Dialog primitive.

## Applicable requirements

- `REQ-UI-DIALOG-CONTENT-CONTAINMENT-001`
- `AC-UI-DIALOG-CONTENT-CONTAINMENT-001.1` through
  `AC-UI-DIALOG-CONTENT-CONTAINMENT-001.8`

## Implementation acceptance

1. Many repository chips create one form-body scroll range while the dialog,
   title, and action footer remain inside both viewports.
2. The final chip and its remove action are reachable, and Cancel/Create Project
   stay visible and hit-testable throughout scrolling.
3. Creating a project still persists chosen repositories; short content stays
   compact and phone has no horizontal document overflow.

## TDD sequence

1. Add stable selectors and long-repository desktop/phone browser cases.
2. RED: record the current oversized dialog or displaced footer.
3. GREEN: apply the capped header/body/footer composition and phone action
   sizing.
4. Run focused E2E, type, lint, i18n, and diff checks.

## Verification

```bash
(cd apps/web && pnpm e2e:run --host -- tests/office/project-repository-picker.spec.ts --grep "create-project dialog")
(cd apps/web && pnpm e2e:run --host --project mobile-chrome -- tests/office/mobile-project-create-dialog.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint app/office/projects/create-project-dialog.tsx e2e/tests/office/project-repository-picker.spec.ts e2e/tests/office/mobile-project-create-dialog.spec.ts)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

## Files likely touched

- `apps/web/app/office/projects/create-project-dialog.tsx`
- `apps/web/e2e/tests/office/project-repository-picker.spec.ts`
- `apps/web/e2e/tests/office/mobile-project-create-dialog.spec.ts`
- `docs/plans/dialog-content-containment/plan.md`
- `docs/plans/dialog-content-containment/task-05-create-project.md`

## Dependencies

None.

## Parallelism

`sequential`. This work order owns one responsive RED-GREEN cycle.

## Output contract

Record RED/GREEN geometry, chip and footer reachability, create-flow evidence,
focused results, files changed, cleanup, blockers, and residual risks.

## Results

- RED: The new long-chip desktop and mobile cases initially failed because the
  Create Project dialog had no stable bounded shell/body seam.
- GREEN: `ProjectFormBody` now occupies one dynamic capped scroll body while
  Cancel and Create Project remain in a persistent footer. Phone actions are
  stacked at a 44-pixel minimum. The shell uses `overflow: clip` so the
  existing inline picker cannot horizontally scroll the dialog; the picker
  wiring and project payload remain unchanged.
- The desktop Chromium and mobile-Chrome long-chip cases passed. They prove
  body scroll range, final chip/remove reachability, persistent footer geometry,
  footer hit testing, cancel behavior, and phone document-overflow safety.
- A parallel Office rerun hit the E2E harness shared backend port; the affected
  mobile case passed on the required sequential rerun.
