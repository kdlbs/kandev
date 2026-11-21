---
created: 2026-11-10
status: implemented
requirements:
  - REQ-TASKS-CHANGE-WORKFLOW-001
system_design:
  - ../../specs/tasks/system-design/change-workflow.md
legacy_specs: []
---

# Change workflow selector appearance

## Overview

Make the three Change workflow selectors recognizable as fields before interaction.
One work order applies local input styling and proves the desktop and phone result.
This package extends the existing [requirement](../../specs/tasks/requirements/change-workflow.md)
and [design](../../specs/tasks/system-design/change-workflow.md#selector-field-appearance).

## Intent and evidence

The user reports that the dropdowns resemble static text and suggests borders.
Source inspection confirms that all three use a `Combobox` ghost Button with a
transparent border. The agent card border surrounds a group, not its selector.
The task system owns this form as part of the workflow-change outcome.

Use the existing Input and SelectTrigger styling as the visual reference.
This is a routine styling choice within the requested scope. No material
question blocks the package.

## Scope

In scope: the destination workflow, destination step, and replacement-profile
triggers. Cover their normal, hover, keyboard-focus, expanded, and disabled
appearance in light and dark themes. Retain the current responsive composition.

Out of scope: changes to other combobox callers, shared defaults, copy,
selection rules, search, validation, routing, persistence, and submission.
No new component API, dependency, feature flag, or ADR is required.

## Technical approach

Apply the same local field classes through the three `triggerClassName` props
in `apps/web/components/task/change-workflow-form-sections.tsx`.
Reuse `border-input`, `bg-background`, and a neutral hover fill from the shared
Input and SelectTrigger. Preserve the Button focus ring and disabled feedback.
Review dark-theme state variants so they retain a visible boundary.

Keep the existing `h-7` desktop and conditional touch sizing. The shared
Combobox also supplies touch minimum heights, so inspect both `height` and
`min-height`. Preserve truncation, step dots, chevrons, and accessible labels.
Do not change `apps/web/components/combobox.tsx` or shared UI primitives.

Use `ChangeWorkflowPage` for shared browser assertions. Extend the existing
desktop and phone spec files rather than creating another selector test suite.
Add browser regressions before the styling change and record their initial
failure on the transparent boundary. Existing component tests cover form
semantics. Class-string unit assertions add no useful evidence for this change.

## Companion packages

- [Change workflow](../change-workflow/plan.md): completed form and move contract.
- [Step colors](../change-workflow-step-colors/plan.md): implemented dot styling and browser coverage.

Their work orders remain complete. Preserve their scenarios and historical
results. This package owns the new appearance checks and their results.

## ASCII UI preview

### UI-01: Desktop field region

Entry: an existing task's `Change workflow...` action. State: a destination
workflow is selected, a destination step is required, and one fixed agent exists.
The source summary, preview, warning, and footer retain their current positions.

Current source uses unframed triggers:

```text
Destination workflow
  Contributor PR Review                          v
Destination step
  Select a step                                  v
```

Proposed region:

```text
Destination workflow
+------------------------------------------------+
| Contributor PR Review                        v |
+------------------------------------------------+
Destination step
+------------------------------------------------+
| Select a step                                v |
+------------------------------------------------+
Workflow agents
  Reviewer
  Used by: Review
  +----------------------------------+  +-------+
  | Reviewer profile               v |  | Reset |
  +----------------------------------+  +-------+
```

Persistent field boundaries and aligned values/chevrons are required.
Border shade and ASCII spacing are illustrative. Use the existing theme tokens.
Focused fields retain the shared focus ring. Expanded fields retain the border
and chevron. Disabled fields retain the boundary and current dimmed feedback.
Map: AC-TASKS-CHANGE-WORKFLOW-001.10, with existing .3 and .9 preserved.

### UI-02: Phone field region in the existing drawer

Entry: the visible task overflow action. State: the same draft as UI-01.
The current full-height drawer remains the closest shipped form exemplar.
Its structure follows `components/kanban/mobile-menu-sheet.tsx`.

```text
Change workflow...                     [Close]
---------------------------------------------
Current: Feature / Analysis

Destination workflow
+-------------------------------------------+
| Contributor PR Review                   v |
+-------------------------------------------+
Destination step
+-------------------------------------------+
| Select a step                           v |
+-------------------------------------------+
Workflow agents
  Reviewer
  +-------------------------------+  +-----+
  | Reviewer profile            v |  |Reset|
  +-------------------------------+  +-----+
---------------------------------------------
[Cancel]                    [Change workflow]
```

The header and footer remain fixed. The body owns form scrolling, with safe-area
clearance below the actions. Each trigger keeps a touch target of at least 44px.
The contained search popovers remain temporary choices inside this flow.
Long names truncate without clipping the chevron or expanding the drawer.
This occasional multi-field action needs the existing full-height form.
Borders require no additional sheet, route, or navigation step.
Map: AC-TASKS-CHANGE-WORKFLOW-001.7 and .10.

## Tests

Run the existing `components/task/change-workflow-dialog.test.tsx` suite.
It covers rendering, reset, cancellation, phone composition, and step colors.
There is no changed selection logic that needs a new unit test.

## E2E tests

Add a `selector fields` scenario to each existing spec:

| Project and file | Evidence |
| --- | --- |
| `chromium`, `tests/task/change-workflow.spec.ts` | All three field boundaries and surfaces in light/dark themes. Real Tab focus, hover, opening, disabled submission, stable bounds, and 28px desktop height. |
| `mobile-chrome`, `tests/task/mobile-change-workflow.spec.ts` | The same fields through taps, light/dark themes, at least 44px targets, contained long names, and no document horizontal overflow. |

Map these checks to AC-TASKS-CHANGE-WORKFLOW-001.10 and existing .7.
Use computed CSS and rendered bounds, not class presence alone. Exercise a step
placeholder and selected value. Keep the colored dot visible after selection.
For the disabled state, hold the existing submit request until assertions finish.
Do not add a sleep or change product behavior to create this state.
Preserve existing workflow-change outcomes, search, reset, cancellation, and
move checks. Capture and inspect one desktop and one phone view in each theme.

## Work orders

- [x] [Task 01: Style and verify selector fields](task-01-selector-fields.md) (done)

Execution is sequential. No delegation is authorized.

## Verification results

Implemented and verified on 2026-11-10.

- Targeted component suite: 1 file and 14 tests passed.
- OXlint: passed with zero warnings across both production files and all three
  test and page files.
- Typecheck with a 4 GB Node heap: passed.
- i18n ratchet: passed with no new copy and an intact guard allowlist.
- Full desktop browser suite: 7 tests passed, including both new theme cases,
  existing color/search/reset/move coverage, and coarse-pointer tablet sizing.
- Full phone browser suite: 4 tests passed, including both new theme cases and
  the existing color and task-action flows.
- Prettier, specification catalog, specification lint, and diff checks passed.
- Local `.github/scripts/pr-docs.cjs` `validateCoverage` over the actual changed
  paths: `covered`, one changed work order, no reference errors.

RED evidence: before the styling change, both desktop theme cases failed because
the focused destination trigger's computed border alpha was 0. Both phone theme
cases failed for the same transparent boundary. The first phone green run then
exposed a pre-existing grid-track overflow with a long workflow name: the form
body scrolled horizontally and the fields crossed the drawer padding. The form
now constrains nested tracks with `grid-cols-1`/`min-w-0`, and the
replacement-profile custom label is clipped inside its trigger. The same checks
then passed with one vertical scroller and no document horizontal overflow.

Captured and inspected desktop and phone views in light and dark themes:
`desktop-selector-fields-{light,dark}.png` and
`phone-selector-fields-{light,dark}.png` under Playwright's `e2e/test-results/`
output. All three selectors show a visible boundary and surface, a trailing
chevron, a readable value or placeholder, and unchanged bounds in normal, hover,
focused, expanded, and disabled states. Step colors and long names remain
visible or contained.

Design validation on 2026-11-10:

- `python3 scripts/list-docs.py validate`: passed, 447 decisions and 1970 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- Local `.github/scripts/pr-docs.cjs` `validateCoverage`: actual documentation-only scope is exempt.
- The same coverage preflight with all four planned frontend paths: `covered`, one work order, no reference errors.
- `git status --short --untracked-files=all`: two changed specifications and two new delivery files, all unstaged.

No subagents, commits, or publication occurred. The user controls any later
commit or publication.

## Risks

- Ghost Button variants include dark-theme hover rules. Local classes must retain the intended neutral surface.
- The profile card boundary can hide a missing trigger boundary in screenshots. Assert each trigger separately.
- A transparent border occupies space but remains invisible. Inspect computed color as well as border width.

## Documentation impact

This design package updates internal specifications and delivery records.
The proposed styling does not change labels or the workflow procedure.
Public documentation and translation catalogs need no change.
