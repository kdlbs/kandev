---
created: 2026-09-30
status: implemented
requirements:
  - REQ-TASKS-CHANGE-WORKFLOW-001
system_design:
  - ../../specs/tasks/system-design/change-workflow.md
legacy_specs: []
---

# Change workflow step colors

## Overview

Restore visible destination-step colors in the existing Change workflow form.
One sequential work order updates rendering and proves the result on desktop
and phone. The task system owns this addition because it extends the existing
workflow-change form's destination contract.

## Scope

In scope: color dots in destination options and the selected trigger, supported
class tokens and hex colors, neutral fallback, and focused regression coverage.
Out of scope: new colors, workflow mutations, dialog redesign, agent routing,
new translations, and changes to the shared combobox contract.

## Intent and evidence

The user requested step colors while choosing a destination. Source confirms
`stepOptions` already builds dots, but writes class tokens into CSS
`backgroundColor`. Workflow settings persist nine `bg-*-500` tokens. Existing
dialog fixtures use hex colors and do not catch that mismatch.

Routine implementation choices: keep dots beside step names, retain them in the
selected trigger, and use the existing color resolver's neutral fallback. No
material question blocks planning.

## Technical approach

Use `parseWorkflowStepColor` from `lib/task-color-presentation.ts` inside
`stepOptions` in `components/task/change-workflow-form-sections.tsx`. Apply its
class or style to the decorative dot. Keep plain labels and the existing
`renderLabel` so search and selected-value rendering share one representation.

The phone exemplar is `components/kanban/mobile-column-tabs.tsx`: a colored dot
beside a step label. Preserve the shipped Change workflow drawer, its scrolling,
focus handling, touch targets, and shared selection state. This is a color-only
change inside that surface.

## Companion packages

[Change workflow](../change-workflow/plan.md) is completed. Preserve its work
orders and historical validation results; this package owns the additional
AC-TASKS-CHANGE-WORKFLOW-001.9 scenarios and their new results. No companion
task scope or existing scenario requires removal.

## ASCII UI preview

### UI-01: Destination step picker, desktop and phone

Entry: task actions -> Change workflow -> choose a destination workflow.
Both surfaces use the same picker content.

```text
Current:  Destination step [Analysis          v]
          Options: Analysis / Implement / Review

Proposed: Destination step [(blue dot) Analysis v]
          +-----------------------------------+
          | Search steps...                   |
          | (blue dot)   Analysis             |
          | (green dot)  Implement            |
          | (purple dot) Review               |
          +-----------------------------------+

No selection: [Select a step                 v]
Unknown color: [(gray dot) Custom step       v]
```

Dot-plus-name is structural; illustrative names and colors come from workflow
data. Desktop retains its dialog and 28px triggers. Phone retains its drawer,
at least 44px touch targets, and its existing scroll owner. The dot does not
shrink; long names truncate within the control. Labels remain localized or
workflow-owned data. Covers AC-TASKS-CHANGE-WORKFLOW-001.9 and preserves .2/.5/.7.

## Tests

Extend `apps/web/components/task/change-workflow-dialog.test.tsx` with
parameterized desktop/phone checks for a saved class token, custom hex color,
missing/unsupported color, and selected-value color. Cover the real renderer
rather than mocking the combobox. Browser tests own name-based search and
workflow-switch reset through the real domain state.
Existing `lib/task-color-presentation.test.ts` verifies the reused resolver.

## E2E tests

Add focused tests with `step colors` in their names to the existing
`tests/task/change-workflow.spec.ts` (chromium) and
`tests/task/mobile-change-workflow.spec.ts` (mobile-chrome). Seed two distinct
saved class colors. Inspect nontransparent computed backgrounds while choosing
each option and after selection, then switch workflows and prove the old color
and selection disappear. Complete the move and verify the destination step.
Phone coverage uses tap and checks containment, 44px step targets, and document
overflow. Capture each picker once for rendered visual verification.

## Work orders

- [x] [Task 01: Render destination step colors](task-01-step-colors.md) (done)

Execution is sequential. No subagents are authorized.

## Verification results

Design validation on 2026-09-30:

- `python3 scripts/list-docs.py validate`: passed, 336 decisions and 1269 specs.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs/tasks/requirements/change-workflow.md docs/specs/tasks/system-design/change-workflow.md docs/plans/change-workflow-step-colors`:
  passed.
- Local `.github/scripts/pr-docs.cjs` `validateCoverage` preflight with the
  planned renderer path included as projected implementation scope: passed,
  `covered`, one work order, no reference errors. The actual documentation-only
  diff is exempt; the projected check validates the delivery chain explicitly.
- `git status --short`: two changed specs and the new plan/work-order directory;
  all files remain unstaged and uncommitted.

The design turn changed no production code or permanent tests. The later
implementation turn was explicitly authorized by the user. Its final results
are recorded in [Task 01](task-01-step-colors.md#results).

Final implementation documentation gates on 2026-09-30 passed: catalog
validation (336 decisions, 1269 specifications), all-spec lint, `git diff --check`,
and the local PR-documentation coverage validator against all nine changed
files (`covered`, one work order, no reference errors). `git status --short`
confirms five frontend/test files, the paired specs, and the new two-file plan
package. The user subsequently authorized PR publication. The requirement is now `active`,
the design `current`, the work order `done`, and this plan `implemented`.

## Risks

- Class-only assertions miss missing generated CSS; browser checks must inspect
  computed backgrounds.
- Hex-only fixtures reproduce the existing coverage gap; use persisted class
  tokens in the new regressions.

## Documentation impact

Public task guidance already describes selecting a destination step. This
visual correction changes no labels or instructions, so no public page change
is planned. The owning requirement/design document the new visible contract.
