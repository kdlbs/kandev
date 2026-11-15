---
id: "01-selector-fields"
title: "Style and verify Change workflow selector fields"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CHANGE-WORKFLOW-001
acceptance_criteria:
  - AC-TASKS-CHANGE-WORKFLOW-001.3
  - AC-TASKS-CHANGE-WORKFLOW-001.7
  - AC-TASKS-CHANGE-WORKFLOW-001.9
  - AC-TASKS-CHANGE-WORKFLOW-001.10
system_design:
  - ../../specs/tasks/system-design/change-workflow.md
---

# Task 01: Style and verify selector fields

## Summary

Give the destination workflow, step, and replacement-profile triggers the same
input appearance. Prove their visible boundaries and existing interactions on
desktop and phone before completing the work order.

## In scope

- Apply local field styling through all three existing `triggerClassName` props.
- Reuse the Input and SelectTrigger border, surface, hover, and focus tokens.
- Constrain long selected names so nested form grids do not widen the phone
  drawer or hide the chevron and Reset action.
- Add shared browser assertions to `ChangeWorkflowPage` and extend both existing specs.
- Inspect desktop and phone captures against UI-01 and UI-02 in both themes.

## Out of scope

- Shared Combobox/Button changes, new APIs, dependencies, copy, or locale keys.
- New selection, validation, search, submission, persistence, or routing behavior.
- Drawer, popover, footer, or entry-point redesigns.

## Acceptance

1. All three triggers satisfy .10 in normal, hover, focused, expanded, and disabled states in both themes.
2. Desktop controls remain 28px high. Phone targets remain at least 44px, with contained names, visible chevrons, and existing form scrolling.
3. Existing mapping, reset, step-color, search, cancel, and move scenarios pass with unchanged shared defaults.

## ASCII UI preview

Excerpts from [UI-01 and UI-02](plan.md#ascii-ui-preview).
Field boundaries are required. Labels and values use existing localized copy
and domain data. The drawings do not specify exact pixel spacing.

UI-01: Desktop field region, open from a task action:

```text
Destination workflow
+----------------------------------------------+
| Contributor PR Review                      v |
+----------------------------------------------+
Destination step
+----------------------------------------------+
| Select a step                              v |
+----------------------------------------------+
Workflow agents: Reviewer
+----------------------------------+  +-------+
| Reviewer profile               v |  | Reset |
+----------------------------------+  +-------+
```

UI-02: Phone field region, inside the existing full-height drawer:

```text
Destination workflow
+-----------------------------------------+
| Contributor PR Review                 v |
+-----------------------------------------+
Destination step
+-----------------------------------------+
| Select a step                         v |
+-----------------------------------------+
Workflow agents: Reviewer
+-----------------------------+  +-------+
| Reviewer profile          v |  | Reset |
+-----------------------------+  +-------+
```

The phone header/footer stay fixed around the scrolling body. Touch targets
remain at least 44px. Focus, expanded, and disabled states retain the border.
Map: AC-TASKS-CHANGE-WORKFLOW-001.7 and .10, preserving .3 and .9.

## Verification

Run every command from the repository root. In a fresh worktree, install the
workspace dependencies before the first package command:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Add the browser assertions before changing production styling. Prove the
transparent-border regression with these commands:

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/task/change-workflow.spec.ts -- --grep 'selector fields')
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-change-workflow.spec.ts -- --grep 'selector fields')
```

After the styling change, run these commands sequentially. The managed runners
build the production assets before each browser run:

```bash
(cd apps/web && pnpm exec vitest run components/task/change-workflow-dialog.test.tsx)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/task/change-workflow-form-sections.tsx e2e/pages/change-workflow-page.ts e2e/tests/task/change-workflow.spec.ts e2e/tests/task/mobile-change-workflow.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/task/change-workflow.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-change-workflow.spec.ts)
(cd apps/web && pnpm exec prettier --check components/task/change-workflow-form-sections.tsx e2e/pages/change-workflow-page.ts e2e/tests/task/change-workflow.spec.ts e2e/tests/task/mobile-change-workflow.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

For each theme, inspect a settled desktop and phone capture. Compare the
rendered field boundaries, chevrons, text, and spacing against the previews.
Record command results and capture paths here. Do not reuse companion test
counts as evidence for this package.

## Files likely touched

- `apps/web/components/task/change-workflow-form-sections.tsx`
- `apps/web/components/task/change-workflow-dialog.tsx`
- `apps/web/e2e/pages/change-workflow-page.ts`
- `apps/web/e2e/tests/task/change-workflow.spec.ts`
- `apps/web/e2e/tests/task/mobile-change-workflow.spec.ts`

The primary session updates this work order and `plan.md` after verification.

## Dependencies

None. The form and color behavior are already implemented.

## Risks

- Dark-theme ghost hover rules can override a local surface class.
- A border-width assertion alone permits the current transparent border.
- Touch `min-height` rules can override explicit heights. Measure the actual targets.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/change-workflow.md), especially .3, .7, .9, and .10.
- [Selector field design](../../specs/tasks/system-design/change-workflow.md#selector-field-appearance).
- The existing form, Combobox, shared Input/SelectTrigger, component tests, and desktop/phone specs.
- [Control sizing](../../specs/ui/requirements/control-sizing.md), as an external shared contract.

## Results

Implemented and verified on 2026-11-10.

RED evidence: before the field styling, both desktop theme cases failed with
`focused destination workflow border should be visible` because the computed
border alpha was 0. Both phone theme cases failed with `destination workflow
border should be visible` for the same transparent boundary. The first phone
green run then exposed a pre-existing long-name grid overflow: the form body
scrolled horizontally and the fields crossed the drawer padding. The fix
constrains nested tracks with `grid-cols-1`/`min-w-0` and clips the
replacement-profile custom trigger label inside its field; shared Combobox,
Button, and profile option definitions are unchanged.

| Command from repository root | Result |
| --- | --- |
| `(cd apps/web && pnpm exec vitest run components/task/change-workflow-dialog.test.tsx)` | Passed, 1 file and 14 tests. |
| `(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/task/change-workflow-form-sections.tsx components/task/change-workflow-dialog.tsx e2e/pages/change-workflow-page.ts e2e/tests/task/change-workflow.spec.ts e2e/tests/task/mobile-change-workflow.spec.ts)` | Passed with zero warnings. |
| `(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run typecheck)` | Passed. |
| `(cd apps/web && pnpm run i18n:ratchet)` | Passed, no new copy, guard allowlist intact. |
| `(cd apps/web && pnpm e2e:run --project chromium tests/task/change-workflow.spec.ts)` | Passed, 7 tests. |
| `(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-change-workflow.spec.ts)` | Passed, 4 tests. |
| `(cd apps/web && pnpm exec prettier --check components/task/change-workflow-form-sections.tsx components/task/change-workflow-dialog.tsx e2e/pages/change-workflow-page.ts e2e/tests/task/change-workflow.spec.ts e2e/tests/task/mobile-change-workflow.spec.ts)` | Passed. |
| `python3 scripts/list-docs.py validate` | Passed. |
| `python3 scripts/lint-spec-files.test.py` | Passed. |
| `python3 scripts/lint-spec-files.py --all` | Passed. |
| `git diff --check` | Passed. |

The local `.github/scripts/pr-docs.cjs` `validateCoverage` run over the actual
changed paths returned `covered` with one changed work order and no reference
errors.

Captures were generated through host-mode Playwright runs and inspected in both
themes: `desktop-selector-fields-light.png`, `desktop-selector-fields-dark.png`,
`phone-selector-fields-light.png`, and `phone-selector-fields-dark.png` under
`apps/web/e2e/test-results/`. All three fields show a visible border and opaque
surface at rest, keyboard focus keeps the ring, disabled fields stay bordered and
dimmed, and phone targets measure at least 44px. The long workflow name
truncates, the analysis dot remains visible, the chevron stays inside the field,
the form has one vertical scroller, and the document has no horizontal overflow.
The replacement-profile trigger contains its long rich label before the chevron
and Reset action.

The user controls any later commit or publication. No subagents or commits were
used for this implementation.
