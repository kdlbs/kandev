---
id: "01-task-title-continuity"
title: "Preserve task title editing across external focus changes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-EDIT-FOCUS-001
acceptance_criteria:
  - AC-UI-EDIT-FOCUS-001.1
  - AC-UI-EDIT-FOCUS-001.2
  - AC-UI-EDIT-FOCUS-001.3
  - AC-UI-EDIT-FOCUS-001.4
  - AC-UI-EDIT-FOCUS-001.5
  - AC-UI-EDIT-FOCUS-001.6
system_design:
  - ../../specs/ui/system-design/inline-edit-focus-continuity.md
---

# Task 01: Preserve task title editing

## Summary

Reproduce the task-title failure with native browser focus transitions.
Implement focus classification for task titles and entry-only selection.

## In scope

- Add `useInlineEditFocus` with active-edit cleanup and identity fencing.
- Adopt it in top-bar and Office title inputs. Preserve the Office guard.
- Correct repeated select-all in both inline titles and the Rename dialog.
- Reset the top-bar editing state on task identity replacement.
- Add independent regressions before production changes.

## Out of scope

Other inline editors belong to task 02. No storage, backend, copy, or layout
changes. Keep existing save failures and task permissions unchanged.

## Acceptance

1. Native external focus changes retain exact drafts and selection without a
   mutation. A subsequent keystroke edits at the retained selection.
2. Internal focus changes, keyboard completion, IME, title clamping, and Office
   guard release retain their intended behavior. No edit completes twice.
3. Same-target refresh preserves the draft. Target replacement, unmount, and
   stale queued callbacks cannot apply it to a different task or edit.

## Interaction preview

Use UI-01 and UI-02 from [the plan](plan.md#interaction-preview). Desktop keeps
the inline input. Phone keeps the existing Rename dialog, Cancel, and Save.
Both retain the draft on external focus. Covers AC-001.1, .2, and .6.

## Regression strategy

First add `keeps the draft when the document loses focus` to
`task-top-bar-title.test.tsx`. Assert the editor remains open and no rename
occurs. It must fail on the current unconditional blur handler.
Add a separate refocus-selection failure so cancellation does not mask it.

Render real title components. Mock only mutation/transport and deterministic
document focus observations. Cover visible-but-unfocused and hidden cases,
null related targets, internal Tab, nonfocusable clicks, iframe focus,
external return followed by outside click, repeated event delivery, and cleanup.
Preserve existing caret tests and test initial selection versus returning focus.
Office tests must assert that external focus retains the editor guard.

The full Chromium channel emits native transitions with CDP focus emulation
disabled. The shared helper verifies both window events and document focus.
Browser tests must open another page in the same context, move foreground,
verify native events, return, type, and save. If headless Chromium does not
produce the transition, use a headed run and record that constraint.
Use the existing mobile task-switcher action menu, not a desktop double-click.

## Verification

Run from repository root. Install workspace dependencies once if absent.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/use-inline-edit-focus.test.tsx hooks/use-external-focus-selection.test.tsx hooks/use-task-title-selection-restore.test.tsx components/task/task-top-bar-title.test.tsx components/task/simple/components/task-title-editable.test.tsx components/task/task-rename-dialog.test.tsx)
(cd apps/web && KANDEV_SERVER_HOST=127.0.0.1 pnpm e2e:run --host --project chromium tests/task/inline-edit-focus.spec.ts tests/task/topbar-rename.spec.ts tests/task/task-title-caret.spec.ts)
(cd apps/web && KANDEV_SERVER_HOST=127.0.0.1 pnpm e2e:run --host --project mobile-chrome tests/task/mobile-inline-edit-focus.spec.ts tests/task/mobile-task-title-caret.spec.ts)
(cd apps/web && pnpm exec eslint hooks/use-inline-edit-focus.ts hooks/use-inline-edit-focus.test.tsx hooks/use-external-focus-selection.ts hooks/use-external-focus-selection.test.tsx e2e/helpers/external-focus.ts components/task/task-top-bar-title.tsx components/task/task-top-bar-title.test.tsx components/task/simple/components/task-title-editable.tsx components/task/simple/components/task-title-editable.test.tsx components/task/task-rename-dialog.tsx components/task/task-rename-dialog.test.tsx e2e/tests/task/inline-edit-focus.spec.ts e2e/tests/task/mobile-inline-edit-focus.spec.ts --max-warnings 0)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
git diff --check
```

## Files likely touched

- `apps/web/hooks/use-inline-edit-focus.ts` and `.test.tsx` (new).
- `apps/web/hooks/use-external-focus-selection.ts` and `.test.tsx` (new).
- `apps/web/e2e/helpers/external-focus.ts` (new).
- `apps/web/components/task/task-top-bar-title.tsx` and `.test.tsx`.
- `apps/web/components/task/simple/components/task-title-editable.tsx` and `.test.tsx`.
- `apps/web/components/task/task-rename-dialog.tsx` and `.test.tsx` (test new).
- `apps/web/e2e/tests/task/inline-edit-focus.spec.ts` (new).
- `apps/web/e2e/tests/task/mobile-inline-edit-focus.spec.ts` (new).

## Dependencies

None. Read the linked design, the Office content contract, `/tdd`, and `/e2e`.

## Risks

Native event ordering and title selection can interact. The browser regression
must test the next typed character, not only the retained value.

## Parallelism

`sequential`

## Inputs

Use existing top-bar, Office title, title-caret, and mobile rename tests as
patterns. Keep their real component and entry-point boundaries.

## Results

Completed on 2026-10-19. Component regressions first reproduced title draft
loss, repeated select-all, and cross-task draft reuse. Native browser testing
also found repeated Radix focus restoration in the Rename dialog. A scoped
selection hook now retains the range through that sequence and discards it
on user interaction or unmount.

The final combined unit run passed 116 tests in 10 files. All task 01 browser
cases passed within the 18-test desktop and six-test phone runs. The dialog
native-focus case also passed three consecutive repetitions. Changed-file
ESLint, full web typecheck, and the i18n changed-line ratchet passed. Exact
commands are above; the browser runs used `--host`, with `--no-build` after
the final managed build. See [the plan](plan.md#verification-results) for the
shared environment notes and evidence boundary.

PR review remediation: the interactive title omits the default
`aria-disabled="false"` attribute. The existing accessibility test was changed
first and failed before the fix. The focus observer comment also clarifies why
an explicit body focus event differs from an ambiguous deferred active element.
See the plan's PR review remediation section for commands and results.
