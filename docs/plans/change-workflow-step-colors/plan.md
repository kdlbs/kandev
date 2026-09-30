---
created: 2026-09-28
status: implemented
requirements:
  - REQ-TASKS-CHANGE-WORKFLOW-001
system_design:
  - ../../specs/tasks/system-design/change-workflow.md
legacy_specs: []
---

# Implementation plan: Change workflow step colors

## Overview

Repair destination step color rendering in one sequential work order. Tasks owns
this extension of its existing Change workflow contract. Intent is settled by
the user's request: show configured colors while choosing a destination step.

## Scope

In scope: option and selected-value color dots, realistic regression fixtures,
and desktop/phone rendered checks. Out of scope: palette changes, new color
formats, workflow transitions, API changes, modal redesign, and new copy.

## Technical approach and evidence

`stepOptions` in `apps/web/components/task/change-workflow-form-sections.tsx`
already creates a dot but uses `style={{ backgroundColor: step.color }}`.
The `STEP_COLORS` configuration in `workflow-pipeline-editor-helpers.tsx`
persists utility classes, as consumed by `task-move-context-menu.tsx`.
`Combobox` reuses `renderLabel` for both option rows and the selected trigger.
The dialog test uses `#abcdef`, masking the class/value mismatch.

Smallest reproduction: open Change workflow, choose a workflow whose steps use
`bg-blue-500`, and inspect its options and selected step. Source tracing confirms
the class is passed into a CSS color property. The failing component regression
confirms this mismatch; post-fix browser checks verify computed colors.
Apply the existing class-based dot pattern with `cn` and aria-hidden decoration.
Preserve inline rendering for saved CSS color values, including imported hex
values; only `bg-` tokens are treated as background utility classes.

## Companion package

The original `../change-workflow/plan.md` and its Task 02 own the shipped form.
Their recorded evidence remains historical. This package adds only the color
regression and does not reopen unrelated work orders or reuse old test counts.

## ASCII UI preview

### UI-01: Destination step picker, desktop and phone

Entry: task actions > Change workflow > select destination workflow.

```text
Before: [  Analysis        v]    Options:   Analysis
After:  [o Analysis        v]    Options: o Analysis
                                        o Implement
                                        o Review
```

`o` is a decorative dot in each step's configured color, never replacement text.
The dot/name order and presence in options and selected value are required;
spacing and sample names are illustrative. Phone uses the existing full-height
Change workflow drawer and touch-sized combobox; desktop keeps its dialog.
The nearest mobile exemplar is `components/kanban/mobile-menu-sheet.tsx`;
no surface geometry changes. Labels, search, focus and scroll behavior remain.
Maps to AC-TASKS-CHANGE-WORKFLOW-001.9 and preserves .7.

## Tests

AC-TASKS-CHANGE-WORKFLOW-001.9: add `renders configured step colors in options
and selected value` to `components/task/change-workflow-dialog.test.tsx`, using
at least two distinct real utility-class colors and the real Combobox. Assert
names and decorative dots, select another step, and verify the selected color.
Verify the placeholder after workflow reset contains no stale selected dot.
Run the regression red before the correction and green afterward.

## E2E tests

Extend `e2e/tests/task/change-workflow.spec.ts` (chromium) and
`e2e/tests/task/mobile-change-workflow.spec.ts` (mobile-chrome) with computed
background-color assertions for options and selected trigger using explicitly
configured, distinct palette colors. Capture the populated picker on both
viewports and inspect it. Retain existing mobile touch/containment checks (.7).
No new unit-only implementation-mirroring test is needed.

## Work orders

- [x] [Task 01: Restore step color dots](task-01-step-colors.md)

## Verification

```sh
(cd apps/web && pnpm exec vitest run components/task/change-workflow-dialog.test.tsx)
(cd apps/web && pnpm exec eslint components/task/change-workflow-form-sections.tsx components/task/change-workflow-dialog.test.tsx e2e/pages/change-workflow-page.ts e2e/tests/task/change-workflow.spec.ts e2e/tests/task/mobile-change-workflow.spec.ts)
make build-web
(cd apps/web && pnpm e2e:run --project chromium tests/task/change-workflow.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-change-workflow.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```
## Verification results

Design validation: catalog validation passed (321 decisions, 1220 specifications);
specification lint passed; all 36 specification-linter tests passed;
`git diff --check` passed. Requirement/design/work-order references checked.
Local PR-documentation coverage preflight (`validateCoverage` with changed
paths and referenced documents) passed with status `covered`. Implementation complete: 5 dialog tests, 4 desktop E2E tests, 1 phone E2E test,
focused ESLint and production build passed. Final color-scenario reruns after
the picker-close wait passed on desktop and phone. Detailed commands and
results are recorded in Task 01.

## Risks

A class assertion alone cannot prove the CSS is present in a production build;
computed-style browser checks must confirm visible, distinct backgrounds.

## PR review remediation

Preserve supported CSS colors alongside palette classes. Extend regression
coverage with hex, RGB and named colors, reset-clears-dot and aria-hidden checks.
Desktop and phone scenarios also verify a hex destination and capture the open
populated picker. Verification results are recorded in Task 01.
