---
created: 2026-09-14
status: implemented
requirements:
  - REQ-TASKS-KEYBOARD-ACTIONS-001
  - REQ-TASKS-KEYBOARD-ACTIONS-002
system_design:
  - ../../specs/tasks/system-design/task-keyboard-actions.md
legacy_specs: []
---

# Implementation Plan: Task keyboard actions

## Overview

Deliver move submission first, then expose existing sidebar actions in the
palette. Implemented sequentially after the explicit implementation request.
Both work orders and their targeted validation are complete.

## Scope

Include active-task commands, nested searchable choices, existing plugin action
parity, local move shortcuts, desktop/mobile behavior, localization, and public
shortcut documentation. Exclude bulk commands, new task capabilities, backend
changes, and plugin SDK changes. The current-task scope follows existing
`SessionCommands`; it does not treat search highlight as task selection.

## Technical approach

Follow the [system design](../../specs/tasks/system-design/task-keyboard-actions.md).
Task 01 connects all full-form and inline field consumers to scoped guarded
submission. Task 02 extracts a task command host, shares existing action policy,
and adds internal command children/disabled handling while preserving established
confirmation flows. Existing move-overrides and task-menu-grouping plans remain
historical implementation evidence; this package owns only this extension.

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

## Tests

Task 01: `workflow-move-shortcut.test.tsx` covers AC-001.1 through AC-001.4
(abbreviated from AC-TASKS-KEYBOARD-ACTIONS); exercise full form and inline
consumers, both platform chords, text editing, composition, repeat, busy, click
races, and failed retries. Existing form/payload/proceed/stepper suites regress
normal operation. Task 02: `task-commands.test.tsx` covers AC-002.1/2/3 with a
per-action matrix of live, archived, unresolved, session-less, pending and plugin
states; `command-panel-task-actions.test.tsx` covers AC-002.4/5 with real keyboard
navigation, disabled choices, focus transitions, stale target cleanup, and locale
changes. Existing confirmation and search suites guard integration behavior.

## E2E tests

Task 01 extends existing workflow move-overrides desktop and mobile specs for
Mod+Enter payload delivery, retained failure drafts, and touch-button operation.
Task 02 adds `tests/task/command-palette-task-actions.spec.ts` and
`tests/task/mobile-command-palette-task-actions.spec.ts`. Prove keyboard-only
Move to -> step -> instructions -> submit; assert task placement and instructions
delivered once using existing move fixture helpers. Exercise rename, pin, a
nested metadata choice, link/relationship form entry, archive/delete confirmation
cancel and confirm, and stale task context. Unit inventory parity covers every
conditional action; E2E covers each distinct execution mechanism. Phone proves
attached-keyboard command entry, Back, task action completion, 44px targets, scrolling,
viewport containment, and no horizontal overflow. Use causal waits and rebuilt
managed runs, not fixed sleeps. Capture desktop/phone rendered evidence.

## Work orders

- [x] [Task 01: Move shortcut](task-01-move-shortcut.md)
- [x] [Task 02: Palette task actions](task-02-palette-task-actions.md), depends on 01

## Verification results

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

## Risks

Nested command selection can race delayed task search; child mode must isolate
its results. Inline options stop event propagation. Duplicating sidebar policy
would drift on archived/plugin states. Focus transfer must avoid stacked traps.

## UI refinement (2026-09-17)

User feedback extends the implemented flow:

- Center the Move button shortcut hint using the button's font and line height.
- Render task color swatches and workflow-step color dots.
- Destination rows show a right arrow and immediate-move shortcut. Enter opens
  options; Cmd/Ctrl+Enter dispatches with defaults. Cross-workflow destinations
  use the same options surface with the captured target workflow ID.
- Close the palette before opening Archive's existing standalone confirmation.
  Phone retains the responsive confirmation, 44px controls, and a single overlay.

Preview: `[color] Verify                  Ctrl+Enter →` opens the existing move
options dialog (desktop) or drawer (phone). Archive opens `[Cancel] [Archive]` in
its own confirmation with task identity and cleanup consequences.

Regression evidence: new modified-Enter and standalone-archive tests failed on
old behavior and pass after implementation. 36 unique focused tests pass across
command palette, session commands, move options, context menus, and destination
workflow payloads. Typecheck, targeted lint, i18n, specification validation and
public-docs validation pass. Managed browser verification: all 3 palette tests passed, both archive tests
passed on rerun after initial backend readiness timeouts, and the updated phone
move/archive test passed. The frontend and backend were rebuilt; reruns used
`--no-build` against that same build. Commands:

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/task/command-palette-task-actions.spec.ts tests/task/command-palette-archive.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project chromium tests/task/command-palette-archive.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-command-palette-task-actions.spec.ts)
```

Additional isolated browser checks confirmed cross-workflow options submission,
colored rows, immediate movement, and standalone desktop/mobile archive surfaces.
Rendered screenshots were inspected. The temporary verification instance on
48431 was stopped; the user's playground and main instance were left untouched.
