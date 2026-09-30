---
id: "01-step-colors"
title: "Restore step color dots"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CHANGE-WORKFLOW-001
acceptance_criteria:
  - AC-TASKS-CHANGE-WORKFLOW-001.7
  - AC-TASKS-CHANGE-WORKFLOW-001.9
system_design:
  - ../../specs/tasks/system-design/change-workflow.md
---

# Task 01: Restore step color dots

## Summary

Render configured step background classes in Change workflow options and selected
values. Establish the regression with realistic utility-class fixtures first.

## In scope

The destination selector renderer and focused desktop/mobile tests.

## Out of scope

New color formats, palette/API changes, copy, layout, and workflow mutations.

## Acceptance

- Options and selected trigger show each configured color beside an accessible
  name, with decorative dots hidden from assistive technology.
- Selecting another step updates its dot; changing workflow clears the prior
  selection indicator, retaining search, order and selection behavior.
- Desktop and phone browser checks prove visible colors; existing mobile
  containment and touch checks continue to pass.

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

See the [full plan](plan.md#ascii-ui-preview).

## Verification

Write the named component regression in the plan first and record its expected
failure before changing production code. Then run:

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
## Files likely touched

- `apps/web/components/task/change-workflow-form-sections.tsx`
- `apps/web/components/task/change-workflow-dialog.test.tsx`
- `apps/web/e2e/pages/change-workflow-page.ts`
- `apps/web/e2e/tests/task/change-workflow.spec.ts`
- `apps/web/e2e/tests/task/mobile-change-workflow.spec.ts`

## Dependencies

None. Use the existing form, fixtures and Combobox.

## Risks

Verify actual computed colors after rebuilding; unit tests do not load Tailwind.

## Parallelism

Sequential.

## Inputs

- [Requirements](../../specs/tasks/requirements/change-workflow.md), .7 and .9.
- [Design](../../specs/tasks/system-design/change-workflow.md), Destination step colors.
- Existing class-based dot in `components/task/task-move-context-menu.tsx`.
- Existing `ChangeWorkflowPage` and workflow fixture helpers for E2E.

## Results

- `pnpm install --frozen-lockfile` from `apps/`: passed.
- Dialog regression: failed before the fix because the configured blue class was
  absent; after the fix, `pnpm exec vitest run
  components/task/change-workflow-dialog.test.tsx` passed all 5 tests.
- The exact focused ESLint command above passed without warnings.
- `make build-web`: passed. Managed E2E also rebuilt the backend, web with the
  pseudo locale, and plugin fixture. Existing Vite chunk/deprecation warnings remain.
- Desktop managed E2E: all 4 tests passed, including colors, selection/reset,
  task preservation, right-click entry and coarse-pointer tablet behavior.
- Phone managed E2E with `--no-build`: 1 test passed, including the real touch
  flow, distinct computed colors, selected color, reset, containment and targets.
  Reused the fresh desktop build without intervening production edits.
- Added a causal picker-close wait after screenshot inspection caught a closing
  animation. Phone rerun passed (1 test); the settled phone screenshot matches
  UI-01 and shows the configured green marker beside Implement.
- `python3 scripts/list-docs.py validate`: passed (321 decisions, 1220 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- Local PR documentation coverage preflight passed (`covered`).
  Public docs were checked with `/docs-maintainer`; existing
  workflow instructions remain accurate because this restores the configured cue.

- Final desktop color-scenario rerun after the picker-close wait: 1 passed
  (`pnpm e2e:run --no-build --project chromium tests/task/change-workflow.spec.ts
  -- --grep "updates the open task stepper"`).

## PR review remediation

- CSS-color regressions failed before the compatibility repair (3 failures).
- Preserve background utility classes and the prior inline CSS-color behavior.
- Add reset-clears-dot and aria-hidden assertions requested in review.
- Add a hex destination to desktop and phone checks and capture the open picker.
- Final verification: dialog Vitest 9 passed; focused ESLint with zero warnings
  passed; `make build-web` passed; managed desktop E2E 4 passed and phone E2E
  1 passed (`--no-build` against that fresh web build). Commands are listed above.
- Specification lint, documentation catalog validation and `git diff --check`
  passed. Desktop and phone populated-picker screenshots recaptured.
- Remote CI/review completion remains pending for the remediation commit.
