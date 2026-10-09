---
id: "01-remove-panel-surfaces"
title: "Remove the built-in panel surfaces"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-EXTRACTION-001
acceptance_criteria:
  - AC-PLUGINS-PROMPT-HISTORY-EXTRACTION-001.1
  - AC-PLUGINS-PROMPT-HISTORY-EXTRACTION-001.3
  - AC-PLUGINS-PROMPT-HISTORY-EXTRACTION-001.9
system_design:
  - ../../specs/plugins/system-design/prompt-history-extraction.md
---

# Task 01: Remove the built-in panel surfaces

## Summary

Delete the built-in Prompt history panel from every core surface: the desktop
and Office workbench "+" menu row, the native-mobile `Panels` entry, the
dockview component and layout identity, the store action that opened it, the
layout-profile placeholder, and the panel's locale copy. After this work order
the workbench offers no built-in Prompt history panel, no dockview component or
panel id for it, and no panel copy in the catalogs. The projection, hooks, and
duration helpers are Task 02's and are the only remaining references.

## In scope

- Delete `apps/web/components/task/prompt-history-panel-content.tsx`,
  `prompt-history-panel-row.tsx`, `prompt-history-panel-host.tsx`, and their
  tests (`prompt-history-panel-content.test.tsx`,
  `prompt-history-panel-content.reactivity.test.tsx`).
- Remove the desktop registrations that render it:
  `dockview-panel-content.tsx` (`"prompt-history": () => <PromptHistoryContent />`),
  `dockview-shared.tsx` (import, `PortalSlot` entry, portal-host map entry),
  and `dockview-desktop-layout.tsx` (`components` map entry, which also feeds
  `DESKTOP_VALID_COMPONENTS`).
- Remove the "+" menu row: `PromptHistoryPanelMenuItem` and its
  `!state.isPassthrough && <PromptHistoryPanelMenuItem … />` call site in
  `dockview-add-panel-items.tsx`, drop the now-unused `IconHistory` import that
  only that row used (the zero-match search cannot see it, and
  `unused-imports/no-unused-imports` at `warn` fails `pnpm run lint`), and update
  the component's JSDoc that lists prompt history among the rows.
- Remove the panel's layout identity and add action:
  `PROMPT_HISTORY_PANEL_ID`, its `REUSABLE_PANEL_IDS`, `KNOWN_PANEL_IDS`, and
  `PANEL_REGISTRY` entries, and its re-export in
  `lib/state/layout-manager/index.ts`; `addPromptHistoryPanel` in
  `lib/state/dockview-extra-panel-actions.ts` and its declaration in
  `lib/state/dockview-store.ts`; and the placeholder map entry in
  `components/settings/layouts/layout-editor.tsx`.
- Remove the native-mobile surface: `showPromptHistory` and its button in
  `components/task/mobile/plugin-panel-picker.tsx`, its props and `Panels`
  active-state term in `components/task/mobile/session-mobile-bottom-nav.tsx`,
  the `currentMobilePanel === "prompt-history"` branch and prop plumbing in
  `components/task/mobile/session-mobile-layout.tsx`, and the
  `"prompt-history"` member of `MobileSessionPanel` in
  `lib/state/slices/ui/types.ts`, and reduce the picker's
  `@tabler/icons-react` import to `IconLayoutGrid` because `IconHistory` was used
  only by that button. The `Panels` bottom-navigation entry then renders from
  `hasTaskCanvases || mobilePluginPanelsAvailable` alone: a task
  with no canvas and no enabled plugin panel shows no entry instead of one that
  opens an empty sheet.
- Delete the six prompt-history copy keys (`promptHistory`,
  `promptHistoryPromptLabel`, `promptHistoryPromptLabelGeneric`,
  `promptHistoryEmpty`, `expandPrompt`, `collapsePrompt`) from all seven
  catalogs: `en`, `ja`, `pt-pt`, `pseudo`, `zh-cn`, `zh-hk`, `zh-tw`.
  Also delete the two matching declarations (`task:promptHistoryPromptLabel`,
  `task:promptHistoryPromptLabelGeneric`) from
  `src/locales/pt-pt/_verbatim.json`; `scripts/lib/i18n-catalogs.mjs` treats a
  declaration without a catalog key as an error, so `pnpm run i18n:check` fails
  until both are gone.
- Delete the panel's core end-to-end files and seed helper:
  `e2e/tests/task/prompt-history-panel.spec.ts`,
  `e2e/tests/task/prompt-history-auto-load.spec.ts`,
  `e2e/tests/task/mobile-prompt-history-panel.spec.ts`,
  `e2e/helpers/prompt-history-long-seed.ts`.
- Trim `lib/state/dockview-panel-actions.prompt-history-panel.test.ts`: keep
  the `scrollTranscriptToMessage` coverage (that action stays and serves the
  plugin navigation capability), drop the `addPromptHistoryPanel` coverage, and
  rename the file to `dockview-panel-actions.scroll-target.test.ts`. Also drop
  what only the dropped describe used: the `PROMPT_HISTORY_TITLE` constant and
  the `PROMPT_HISTORY_PANEL_ID` import (`CENTER_GROUP` stays, the kept describes
  use it). Leftovers like these are warn-level `no-unused-vars` findings that
  `eslint --max-warnings 0` rejects.
- Rewrite the two browser specs whose locators die with these deletions, and
  run them in Task 03:
  - `e2e/tests/settings/mobile-layout-profiles.spec.ts` — the touch layout
    editor case `adds Prompt History through the touch layout editor` drives
    `layouts.addPanel("Prompt History")`, which is rendered from
    `REUSABLE_PANEL_IDS` plus the `PANEL_REGISTRY` title. Re-target it to a
    surviving reusable panel (`Todos`), mirroring the desktop rewrite Task 03
    owns.
  - `e2e/tests/plugins/mobile-plugin-task-panel.spec.ts` — the disabled-plugin
    case asserts `mobile-prompt-history-option` is visible and the `Panels`
    entry stays after its only plugin is disabled. Assert the option is absent,
    assert the `Panels` entry matches the new rule (absent with no canvas and no
    enabled plugin panel), and reword the surrounding comment that explains the
    shared grouped action.
- Drop the bindings only the removed prompt-history blocks used, in every test file
  this work order edits: `VALID_COMPONENTS`, `SESSION_ID`, `MESSAGE_ID`, and
  `CUSTOM_AGENT` in `components/task/dockview-shared.test.tsx`; `SESSION_ID` in
  `components/task/dockview-panel-content.todos.test.tsx` (the surviving Todos
  describe uses a literal); `fireEvent` in the `@testing-library/react` import and
  the `handleNavigateToPrompt` local in
  `components/task/mobile/session-mobile-layout.test.tsx` (keep the other
  testing-library imports the surviving cases use); `REUSABLE_PANEL_IDS` and
  `KNOWN_PANEL_IDS` in `lib/state/layout-manager/panel-titles.test.ts`; and
  `filterEphemeral`, `panel`, `ephemeralPanel`, `LayoutPanel`, and `LayoutState`
  in `lib/state/layout-manager/serializer.test.ts`; and
  `mockScrollTranscriptToMessage` with its store-mock entry and its `mockClear()`
  reset in `components/task/dockview-shared.test.tsx` and
  `components/task/dockview-panel-content.todos.test.tsx` (keep each file's store
  mock, which the surviving cases still need). None of these names carries a
  prompt or history token, so neither search gate sees them, and `pnpm run lint`
  (`eslint --max-warnings 0`) rejects each leftover binding — except the mock,
  whose remaining references keep it lint-clean while it stops asserting
  anything.
- Update the tests that asserted the removed surfaces:
  `components/task/dockview-add-panel-items.test.tsx`,
  `components/task/dockview-desktop-layout.test.ts`,
  `components/task/dockview-panel-content.todos.test.tsx`,
  `components/task/dockview-shared.test.tsx`,
  `components/task/mobile/session-mobile-layout.test.tsx`,
  `components/task/mobile/session-mobile-bottom-nav.test.tsx`,
  `lib/state/layout-manager/panel-titles.test.ts`,
  `lib/state/layout-manager/serializer.test.ts`.
- Fix comments that describe the removed panel as a live consumer:
  `components/task/chat/message-list-native-scroll.ts` (the prompt-history jump
  note), the `PendingMessageScrollTarget` doc in
  `components/task/task-chat-panel.tsx`, and
  `hooks/domains/session/older-message-pagination.ts` with its test, whose
  docblock and concurrent-caller example list the panel sentinel among the
  coordinator's consumers. Reword those lists to the surviving callers (native
  transcript, automatic backfill, last-prompt and drain preloads) and rename the
  test's `panel` local; test assertions do not change. Keep the scroll-target
  behavior and its docs otherwise intact. Neither search gate sees any of these
  comments.

## Out of scope

- The core prompt projection, hooks, and duration helpers (Task 02).
- Any change to `plugin-panel`, plugin task-panel registration, the navigation
  capability, the conversation façade, favorite state, or
  `host.ui.PromptMentionText`.
- Saved-layout and profile compatibility proof, and the new browser coverage
  (Task 03).
- The plugin fixture and plugin E2E specs, which stay untouched.

## Acceptance

1. No core surface offers, renders, or can open the built-in panel: the "+"
   menu has no Prompt history row, the `Panels` picker has no Prompt history
   option, and no dockview component, reusable panel id, known panel id, or
   panel-registry entry named `prompt-history` remains.
2. Every other workbench surface still works: Chat, Plan, Todos, Changes, Files,
   Browser, VS Code, terminals, canvases, and plugin panels register and render
   as before, and `scrollTranscriptToMessage` still drives transcript navigation.
3. All seven locale catalogs are complete after the six keys are removed, and
   no remaining key repeats panel-only copy.

## ASCII UI preview

Relevant excerpt of the plan's previews (see
[the plan](plan.md#ascii-ui-preview)):

```text
UI-01: Desktop task workbench "+" menu (required)

  Sessions >                    Terminals >                 Browser
  VS Code                       Plan                        <plugin panels>
  <task canvases>               Todos                       Changes (when closed)
  Files (when closed)           <review panels>             <repository scripts>
  (no "Prompt history" row)

UI-02: Phone "Panels" picker (required)

  <task canvases>
  <plugin panels>
  (no "Prompt history" option; 44 px rows, one scrolling sheet)
```

## Verification

Run from the repository root. In a fresh worktree run
`(cd apps && pnpm install --frozen-lockfile)` first.

```bash
(cd apps/web && pnpm exec vitest run \
  components/task/dockview-add-panel-items.test.tsx \
  components/task/dockview-desktop-layout.test.ts \
  components/task/dockview-panel-content.todos.test.tsx \
  components/task/dockview-shared.test.tsx \
  components/task/mobile/session-mobile-layout.test.tsx \
  components/task/mobile/session-mobile-bottom-nav.test.tsx \
  lib/state/layout-manager/panel-titles.test.ts \
  lib/state/layout-manager/serializer.test.ts \
  lib/state/dockview-panel-actions.scroll-target.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run lint)
rg -n -i -e "prompt[ _-]?history|PROMPT_HISTORY|addPromptHistoryPanel|PromptHistoryPanel" \
  apps/web/src/locales \
  apps/web/components/task/mobile \
  apps/web/components/task/dockview-add-panel-items.tsx \
  apps/web/components/task/dockview-panel-content.tsx \
  apps/web/components/task/dockview-shared.tsx \
  apps/web/components/task/dockview-desktop-layout.tsx \
  apps/web/components/settings/layouts/layout-editor.tsx \
  apps/web/lib/state/layout-manager \
  apps/web/lib/state/dockview-store.ts \
  apps/web/lib/state/dockview-extra-panel-actions.ts
rg -l -i -e "prompt[ _-]?history" apps/web/e2e apps/web/scripts
```

The first search covers exactly the surfaces this work order owns, and must
return no matches. It deliberately does not cover the files Task 02 owns
(`lib/prompt-history.ts`, `lib/i18n/formats.ts`, `hooks/**`,
`lib/state/slices/**`, `lib/state/app-state-types.ts`,
`lib/state/default-state.ts`, `lib/state/store-reexports.ts`, and
`components/task/chat/messages/message-actions.tsx`); those still match until
Task 02 lands, and Task 02's own gate closes them. The pattern is case-insensitive and includes the spaced title
`Prompt History` and the SCREAMING_CASE constant, so neither a renamed survivor
nor a leftover catalog title passes.

`apps/web/scripts` is excluded from the zero-match search on purpose: the
fixture build tooling (`build-e2e-plugin.mjs`, `write-e2e-plugin-identity.mjs`)
names the retained plugin fixture and stays.

The second search lists test-side and fixture-tooling mentions; it is a listing,
not a zero-match gate, because the kept plugin specs legitimately name the
fixture. Every listed file must be one of: the plugin fixture or its identity
setup
(`e2e/fixtures/plugins/prompt-history-plugin/**`, `e2e/global-setup.ts`,
`e2e/README.md`), a kept or rewritten plugin spec
(`e2e/tests/plugins/**`), or a rewritten layout spec
(`e2e/tests/settings/layout-profiles.spec.ts` until Task 03 rewrites that one,
`e2e/tests/settings/mobile-layout-profiles.spec.ts` rewritten here), or a Task 03
replacement spec that asserts the row and the option are gone
(`e2e/tests/task/prompt-history-removed.spec.ts`,
`e2e/tests/task/mobile-prompt-history-removed.spec.ts`), or the
fixture build tooling (`apps/web/scripts/build-e2e-plugin.mjs`,
`apps/web/scripts/write-e2e-plugin-identity.mjs`). None of the deleted core
panel files may appear: `e2e/tests/task/prompt-history-panel.spec.ts`,
`e2e/tests/task/prompt-history-auto-load.spec.ts`,
`e2e/tests/task/mobile-prompt-history-panel.spec.ts`,
`e2e/helpers/prompt-history-long-seed.ts`.

## Files likely touched

- `apps/web/components/task/prompt-history-panel-content.tsx` (delete)
- `apps/web/components/task/prompt-history-panel-row.tsx` (delete)
- `apps/web/components/task/prompt-history-panel-host.tsx` (delete)
- `apps/web/components/task/prompt-history-panel-content.test.tsx` (delete)
- `apps/web/components/task/prompt-history-panel-content.reactivity.test.tsx` (delete)
- `apps/web/components/task/dockview-panel-content.tsx`
- `apps/web/components/task/dockview-shared.tsx`
- `apps/web/components/task/dockview-desktop-layout.tsx`
- `apps/web/components/task/dockview-add-panel-items.tsx`
- `apps/web/lib/state/layout-manager/constants.ts`, `index.ts`
- `apps/web/lib/state/dockview-extra-panel-actions.ts`
- `apps/web/lib/state/dockview-store.ts`
- `apps/web/components/settings/layouts/layout-editor.tsx`
- `apps/web/components/task/mobile/plugin-panel-picker.tsx`
- `apps/web/components/task/mobile/session-mobile-bottom-nav.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.tsx`
- `apps/web/lib/state/slices/ui/types.ts`
- `apps/web/src/locales/{en,ja,pt-pt,pseudo,zh-cn,zh-hk,zh-tw}/task.json`
- `apps/web/src/locales/pt-pt/_verbatim.json`
- `apps/web/e2e/tests/task/{prompt-history-panel,prompt-history-auto-load,mobile-prompt-history-panel}.spec.ts` (delete)
- `apps/web/e2e/tests/settings/mobile-layout-profiles.spec.ts` (rewrite)
- `apps/web/e2e/tests/plugins/mobile-plugin-task-panel.spec.ts` (rewrite assertions)
- `apps/web/components/task/dockview-add-panel-items.test.tsx`
- `apps/web/components/task/dockview-desktop-layout.test.ts`
- `apps/web/components/task/dockview-panel-content.todos.test.tsx`
- `apps/web/components/task/dockview-shared.test.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.test.tsx`
- `apps/web/components/task/mobile/session-mobile-bottom-nav.test.tsx`
- `apps/web/lib/state/layout-manager/panel-titles.test.ts`
- `apps/web/lib/state/layout-manager/serializer.test.ts`
- `apps/web/lib/state/dockview-panel-actions.scroll-target.test.ts` (renamed)
- `apps/web/e2e/helpers/prompt-history-long-seed.ts` (delete)
- `apps/web/components/task/chat/message-list-native-scroll.ts` (comment)
- `apps/web/components/task/task-chat-panel.tsx` (comment)

## Risks

Removing `prompt-history` from `DESKTOP_VALID_COMPONENTS` and `KNOWN_PANEL_IDS`
is what makes saved layouts drop the panel; do not add a compatibility shim or a
renamed component. If a test asserts that the persisted panel restores, that
test is asserting the removed behavior and must be replaced by Task 03's
compatibility proof rather than re-pinned.

## Dependencies

None. Run before Task 02: the panel is the last importer of the projection that
Task 02 deletes.

## Parallelism

`sequential`

## Inputs

- [Extraction requirements](../../specs/plugins/requirements/prompt-history-extraction.md)
  and [design](../../specs/plugins/system-design/prompt-history-extraction.md).
- [Superseded panel design](../../specs/ui/system-design/prompt-history-panel.md)
  for the behavior being removed.
- [Host prerequisites](../../specs/plugins/requirements/prompt-history-extraction-host.md)
  for what must stay.

## Results

Done. Commands run from the repository root.

```bash
(cd apps/web && pnpm exec vitest run \
  components/task/dockview-add-panel-items.test.tsx \
  components/task/dockview-desktop-layout.test.ts \
  components/task/dockview-panel-content.todos.test.tsx \
  components/task/dockview-shared.test.tsx \
  components/task/mobile/session-mobile-layout.test.tsx \
  components/task/mobile/session-mobile-bottom-nav.test.tsx \
  lib/state/layout-manager/panel-titles.test.ts \
  lib/state/layout-manager/serializer.test.ts \
  lib/state/dockview-panel-actions.scroll-target.test.ts)
# 9 files passed, 97 tests passed
(cd apps/web && pnpm run typecheck)   # tsc --noEmit clean
(cd apps/web && pnpm run i18n:check)  # keys, <Trans> indices, plurals, module-scope t(), em dashes, non-JSX copy all OK; all six locale catalogs complete
(cd apps/web && pnpm run lint)        # eslint --max-warnings 0 clean
```

Both searches from the Verification block were re-run through the agent search
tool.

- Zero-match gate (locales, `components/task/mobile`, the dockview
  registrations, `layout-editor.tsx`, `layout-manager/`, `dockview-store.ts`,
  `dockview-extra-panel-actions.ts`): no matches.
- Listing gate (`apps/web/e2e`, `apps/web/scripts`): only the plugin fixture,
  its identity setup (`global-setup.ts`, `e2e/README.md`), the kept plugin specs,
  the Task 03 replacement specs added after this work order
  (`e2e/tests/task/prompt-history-removed.spec.ts`,
  `e2e/tests/task/mobile-prompt-history-removed.spec.ts`), and the fixture build
  tooling match. None of
  `e2e/tests/task/prompt-history-panel.spec.ts`,
  `prompt-history-auto-load.spec.ts`,
  `e2e/tests/task/mobile-prompt-history-panel.spec.ts`, or
  `e2e/helpers/prompt-history-long-seed.ts` appears; all four are deleted.

One gate-driven comment fix was made beyond the named edits: the
`buildSidePanelActions` docblock in `dockview-extra-panel-actions.ts` listed
`prompt-history` among the side panels it builds, and the zero-match gate covers
that file.

Deleted files: `prompt-history-panel-content.tsx`,
`prompt-history-panel-row.tsx`, `prompt-history-panel-host.tsx`,
`prompt-history-panel-content.test.tsx`,
`prompt-history-panel-content.reactivity.test.tsx`, the three core panel e2e
specs, and `e2e/helpers/prompt-history-long-seed.ts`.
`dockview-panel-actions.prompt-history-panel.test.ts` was renamed to
`dockview-panel-actions.scroll-target.test.ts` with the `addPromptHistoryPanel`
coverage dropped and the `scrollTranscriptToMessage` coverage kept.
