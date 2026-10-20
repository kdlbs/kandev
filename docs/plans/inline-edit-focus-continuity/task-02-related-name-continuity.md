---
id: "02-related-name-continuity"
title: "Preserve related inline name edits across external focus changes"
status: done
wave: 2
depends_on:
  - "01-task-title-continuity"
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

# Task 02: Preserve related name editing

## Summary

Apply the proven focus behavior to session/terminal tabs, Quick Chat tabs,
and file-tree rename/create. Preserve each consumer's existing internal
blur completion behavior.

## In scope

- Adopt `useInlineEditFocus` in the four source files below.
- Keep Quick Chat's inside-container focus exception and completion guard.
- Prevent duplicate completion after Enter, Escape, blur, and unmount.
- Cover exact drafts, selection, target replacement, and consumer callbacks.

## Out of scope

Settings autosave, saved-query dialogs, persistent drafts, and mutation retry
redesign. Do not change domain validation or phone composition.

## Acceptance

1. External focus leaves each listed editor open without rename/create
   callbacks. Returning retains the draft and selection.
2. Internal outside focus and explicit completion retain existing semantics
   and produce at most one terminal action. Empty file names remain validated.
3. Desktop covers all listed consumers. Phone covers Quick Chat's visible
   rename action and its input continuity using the configured mobile project.

## Interaction preview

Use UI-03 from [the plan](plan.md#interaction-preview). The same inline input
remains visible after returning. Phone uses the existing Quick Chat action
menu and touch input. Covers AC-001.1 through .4 and .6.

## Regression strategy

Add a failing `does not commit when the document loses focus` test for each
consumer before migrating it. Separate external focus from real internal blur.
Use existing Quick Chat and file-context-menu tests as patterns. New tests
render `TabRenameInput` and `InlineFileInput` directly with callback spies.
Cover Enter followed by blur, Escape followed by blur, nonfocusable outside
click, returning outside click, and unmount before deferred classification.

Add desktop `inline-name-focus.spec.ts` for Quick Chat. Extend the existing
session-tab, terminal-tab, file rename, and file creation tests. Assert no mutation during external
focus, then verify the explicit completion and persisted result. Add the
mobile companion for Quick Chat through its visible touch menu. Use the same
native-event proof boundary as work order 01.

## Verification

Run from repository root after work order 01 installs dependencies.

```bash
(cd apps/web && pnpm exec vitest run hooks/use-inline-edit-focus.test.tsx components/task/tab-rename-input.test.tsx components/quick-chat/quick-chat-tab-item.test.tsx components/task/file-context-menu.test.tsx components/task/inline-file-input.test.tsx)
(cd apps/web && KANDEV_SERVER_HOST=127.0.0.1 pnpm e2e:run --host --project chromium tests/task/inline-name-focus.spec.ts tests/session/session-tab-rename.spec.ts tests/task/file-tree-rename.spec.ts tests/task/file-tree-create.spec.ts)
(cd apps/web && KANDEV_SERVER_HOST=127.0.0.1 pnpm e2e:run --host --project chromium tests/terminal/terminal-dockview-ui.spec.ts --grep "Rename swaps the tab title")
(cd apps/web && KANDEV_SERVER_HOST=127.0.0.1 pnpm e2e:run --host --project mobile-chrome tests/task/mobile-inline-name-focus.spec.ts tests/chat/mobile-quick-chat-tabs.spec.ts)
(cd apps/web && pnpm exec eslint components/task/tab-rename-input.tsx components/task/tab-rename-input.test.tsx components/quick-chat/quick-chat-tab-item.tsx components/quick-chat/quick-chat-tab-item.test.tsx components/task/file-context-menu.tsx components/task/file-context-menu.test.tsx components/task/inline-file-input.tsx components/task/inline-file-input.test.tsx e2e/tests/task/inline-name-focus.spec.ts e2e/tests/task/mobile-inline-name-focus.spec.ts e2e/tests/session/session-tab-rename.spec.ts e2e/tests/task/file-tree-rename.spec.ts e2e/tests/task/file-tree-create.spec.ts e2e/tests/terminal/terminal-dockview-ui.spec.ts --max-warnings 0)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/task/tab-rename-input.tsx` and `.test.tsx` (test new).
- `apps/web/components/quick-chat/quick-chat-tab-item.tsx` and `.test.tsx`.
- `apps/web/components/task/file-context-menu.tsx` and `.test.tsx`.
- `apps/web/components/task/inline-file-input.tsx` and `.test.tsx` (test new).
- `apps/web/e2e/tests/task/inline-name-focus.spec.ts` (new).
- `apps/web/e2e/tests/task/mobile-inline-name-focus.spec.ts` (new).
- `apps/web/e2e/tests/session/session-tab-rename.spec.ts`.
- `apps/web/e2e/tests/task/file-tree-rename.spec.ts` and `file-tree-create.spec.ts`.
- `apps/web/e2e/tests/terminal/terminal-dockview-ui.spec.ts` (rename test only).

## Dependencies

Work order 01 supplies the focus hook and browser transition evidence.

## Risks

File operations and rename callbacks can complete asynchronously. Focus
classification must not bypass existing request handling or rollback behavior.

## Parallelism

`sequential`

## Inputs

Linked requirement/design, existing session-tab rename and file-tree rename
E2E suites, Quick Chat tests, `/tdd`, and `/e2e`.

## Results

Completed on 2026-10-19. Regressions first reproduced premature tab, Quick
Chat, and file completion on external blur. Separate regressions reproduced
Escape followed by file creation and repeated Enter causing two tab commits.
The focus hook and completion guards now prevent these failures.

The final combined unit run passed 116 tests in 10 files. The desktop suite
passed 18 cases and the phone suite passed six cases, including the related
editor flows named above. The focused terminal-tab rename test passed
separately. Changed-file ESLint, full web typecheck, and the i18n changed-line
ratchet passed. See [the plan](plan.md#verification-results) for shared command
and environment details. No temporary diagnostic code remains.

PR review remediation: the shared focus observer now ignores outside pointer
and focus events until initial editor focus. Both new regressions failed before
the guard and pass after it. The nine file-create and file-rename browser cases
passed. See the plan's PR review remediation section for commands and results.
