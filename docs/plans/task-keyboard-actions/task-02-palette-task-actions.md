---
id: "02-palette-task-actions"
title: "Palette task actions"
status: done
wave: 2
depends_on: ["01-move-shortcut"]
plan: plan.md
requirements:
  - REQ-TASKS-KEYBOARD-ACTIONS-002
acceptance_criteria:
  - AC-TASKS-KEYBOARD-ACTIONS-002.1
  - AC-TASKS-KEYBOARD-ACTIONS-002.2
  - AC-TASKS-KEYBOARD-ACTIONS-002.3
  - AC-TASKS-KEYBOARD-ACTIONS-002.4
  - AC-TASKS-KEYBOARD-ACTIONS-002.5
system_design:
  - ../../specs/tasks/system-design/task-keyboard-actions.md
---

# Task 02: Palette task actions

## Summary and scope

Expose the complete single-task sidebar action inventory in the palette, preserving handlers, live eligibility, plugin changes, and confirmations. Add nested searchable command navigation and active-context cleanup.
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

Full context: [plan preview](plan.md#ascii-ui-preview). Task 2 owns the
palette, nested navigation, and transfer to the move form.

## Verification

Run from repository root. New test paths below are deliverables of this work
order. Install dependencies once before the first pnpm command; task 02 reuses
task 01's install. Generate Traditional Chinese using `pnpm run i18n:zh-hant`
and pseudo using the repository generator after adding source translations.

```bash
(cd apps/web && pnpm exec vitest run components/session-commands.test.tsx components/task-commands.test.tsx components/command-panel-task-actions.test.tsx components/task/task-switcher-context-menu.test.tsx components/command-panel-confirmation.test.tsx lib/commands/search.test.ts hooks/use-command-panel-shortcuts.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/task/command-palette-task-actions.spec.ts tests/task/command-palette-archive.spec.ts tests/command-panel.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-command-palette-task-actions.spec.ts tests/task/mobile-command-panel-task-navigation.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run targeted ESLint on every changed TS/TSX file from apps/web using
`pnpm exec eslint <changed paths>`; record the expanded command in Results.
Update the existing public shortcut/reference sections only after behavior works; use the docs-maintainer skill.

## Files likely touched

- `apps/web/components/session-commands.tsx`
- `apps/web/components/task-commands.tsx (new)`
- `apps/web/components/task/task-switcher-context-menu-items.tsx`
- `apps/web/components/task/task-session-sidebar.tsx and its action helpers`
- `apps/web/components/command-panel.tsx`
- `apps/web/components/command-panel-results.tsx`
- `apps/web/components/command-panel-footer.tsx`
- `apps/web/lib/commands/types.ts`
- `apps/web/lib/commands/search.ts`
- `apps/web/hooks/use-command-panel-shortcuts.ts`
- `apps/web/components/task-commands.test.tsx (new)`
- `apps/web/components/command-panel-task-actions.test.tsx (new)`
- `apps/web/e2e/tests/task/command-palette-task-actions.spec.ts (new)`
- `apps/web/e2e/tests/task/mobile-command-palette-task-actions.spec.ts (new)`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/`
- `docs/public/use-kandev.md`
- `docs/public/tasks-and-workflows.md`

## Dependencies

Task 01 complete, providing the shared move submission shortcut.

## Risks

Stale command callbacks, eligibility drift, disabled selection, and overlay focus races.

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

