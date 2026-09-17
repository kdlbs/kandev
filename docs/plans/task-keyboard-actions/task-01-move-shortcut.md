---
id: "01-move-shortcut"
title: "Move shortcut"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-TASKS-KEYBOARD-ACTIONS-001
acceptance_criteria:
  - AC-TASKS-KEYBOARD-ACTIONS-001.1
  - AC-TASKS-KEYBOARD-ACTIONS-001.2
  - AC-TASKS-KEYBOARD-ACTIONS-001.3
  - AC-TASKS-KEYBOARD-ACTIONS-001.4
system_design:
  - ../../specs/tasks/system-design/task-keyboard-actions.md
---

# Task 01: Move shortcut

## Summary and scope

Wire scoped Mod+Enter submission and shortcut hints across all move-option consumers. Preserve normal editing, existing payload normalization, touch controls, and failure drafts.
Use TDD: observe the targeted regression tests fail before implementation.

## Out of scope

New backend contracts, persistence, bulk commands, implementing Duplicate, and
plugin SDK expansion. Follow the package exclusions.

## Acceptance

- Meet the referenced criteria with shared existing mutation paths.
- Pass the targeted unit and desktop/mobile E2E evidence in the plan.
- Preserve localized copy and current focus/confirmation behavior.

## ASCII UI preview

UI-01: Desktop, current task, expanded Move to.

```text
[ Search commands                      ]
Task: Fix login
[< Back] Move to
  Review                               >
  Done                                 >
              select Review
[ Move to Review                       ]
[ ] Reset context   [ ] Skip step prompt
Instructions
[ Check the timeout case               ]
[ Cancel ]            [ Move  Cmd+Enter ]
```

UI-02: Phone, attached-keyboard command entry, nested choice then move drawer.

```text
+--------------------------------+
| Task: Fix login                |
| [< Back] Move to               |
| [ Search steps               ] |
| Review                       > |
| Done                         > |
+--------------------------------+
          select Review
+--------------------------------+
| Move to Review                 |
| [ ] Reset context              |
| [ ] Skip step prompt           |
| Instructions                   |
| [ Check the timeout case     ] |
| [ Cancel ] [ Move            ] |
|          safe area             |
+--------------------------------+
```

Required: single active surface, visible task identity, searchable choices and
Back, editable instructions, and an explicit Move action. Phone search/header
remain fixed and the list/form owns vertical scrolling. Details and names are
illustrative; labels use localization. Pending: Move disabled with Moving label;
failure: existing error feedback with draft retained; empty list: localized no
available steps and Back; Duplicate: visible disabled. UI-01/02 map to all ACs
in this package and the targeted desktop/mobile E2E below.

Full context: [plan preview](plan.md#ascii-ui-preview). Task 1 owns the
move form and shortcut hint.

## Verification

Run from repository root. New test paths below are deliverables of this work
order. Install dependencies once before the first pnpm command; task 02 reuses
task 01's install. Generate Traditional Chinese using `pnpm run i18n:zh-hant`
and pseudo using the repository generator after adding source translations.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/workflow-move-options-form.test.tsx components/task/workflow-move-options.test.ts components/task/workflow-stepper-keyboard.test.tsx components/task/workflow-move-proceed-button.test.tsx components/task/workflow-move-shortcut.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/workflow/workflow-step-move-overrides.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/workflow/mobile-workflow-step-move-overrides.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run targeted ESLint on every changed TS/TSX file from apps/web using
`pnpm exec eslint <changed paths>`; record the expanded command in Results.
Public docs remain deferred to task 02.

## Files likely touched

- `apps/web/components/task/workflow-move-options.tsx`
- `apps/web/components/task/workflow-stepper.tsx`
- `apps/web/components/task/workflow-step-disclosure.tsx`
- `apps/web/components/task/workflow-move-proceed-button.tsx`
- `apps/web/components/task/workflow-move-shortcut.test.tsx (new)`
- `apps/web/e2e/tests/workflow/*workflow-step-move-overrides.spec.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/`

## Dependencies

None.

## Risks

Inline propagation guards and same-render duplicate submissions.

## Parallelism

Sequential. No delegated implementation is authorized.

## Inputs

- [Requirements](../../specs/tasks/requirements/task-keyboard-actions.md)
- [System design](../../specs/tasks/system-design/task-keyboard-actions.md)
- Existing sidebar menu, move form, command confirmation, and workflow E2E patterns.

## Results

Implemented and verified on 2026-09-15.

- Red/green regression evidence captured for move submission, full/compact inline
  forms, disabled/nested palette selection, and task action inventory.
- Combined targeted Vitest run: 12 files, 105 tests passed.
- Desktop workflow move overrides: 5 browser tests passed.
- Desktop task palette, archive, and existing command panel: 18 browser tests passed.
- Phone task palette, navigation, and workflow move overrides: 5 browser tests passed.
- Typecheck, targeted ESLint (zero warnings), and i18n checks passed.
- Public docs validation (46 pages), public docs validator test, specification
  catalog validation (269 decisions, 923 specifications), full spec lint, and
  `git diff --check` passed.
- Desktop dialog and phone drawer screenshots inspected: instructions visible,
  no stacked palette, and phone controls meet the tested 44px touch targets.

Browser commands used the work-order paths. The final phone run combined both
work orders with `--no-build --project mobile-chrome` against the same freshly
built runtime used by the passing desktop run. Initial sandbox Go-cache access
failed; the approved elevated managed runner completed successfully.

Browser coverage exercises move success/failure/retry, instruction delivery once,
metadata actions, delete cancellation, archive confirmation, and phone navigation.
Conditional action inventory and disappeared-parent cleanup are unit-tested;
link/nesting mutations and plugin execution were not separately browser-tested.

Expanded lint command:

```bash
(cd apps/web && pnpm exec eslint --max-warnings 0 components/command-panel-dialog.tsx components/command-panel-footer.tsx components/command-panel-results.tsx components/command-panel-task-actions.test.tsx components/command-panel.tsx components/task-command-choices.tsx components/task-command-items.tsx components/task-commands.test.tsx components/task-commands.tsx components/task/task-move-context-menu.tsx components/task/task-page-inner.tsx components/task/task-row-action-availability.ts components/task/task-session-sidebar.tsx components/task/task-switcher-context-menu-items.tsx components/task/use-workflow-move-submit.ts components/task/workflow-move-options.tsx components/task/workflow-move-shortcut.test.tsx components/task/workflow-step-disclosure-actions.tsx components/task/workflow-step-disclosure.tsx components/task/workflow-stepper-keyboard.test.tsx components/task/workflow-stepper.tsx e2e/tests/task/command-palette-task-actions.spec.ts e2e/tests/task/mobile-command-palette-task-actions.spec.ts e2e/tests/workflow/mobile-workflow-step-move-overrides.spec.ts e2e/tests/workflow/workflow-step-move-overrides.spec.ts hooks/use-command-children.ts lib/commands/search.ts lib/commands/types.ts)
```

