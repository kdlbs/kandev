---
id: "02-add-file-tree-action"
title: "Add file-tree context action"
status: done
wave: 2
depends_on: ["01-model-directory-context"]
plan: "plan.md"
spec: "../../specs/ui/requirements/file-tree-chat-context.md"
---

# Task 02: Add File-Tree Context Action

## Intent

Connect file-tree nodes to the session-scoped context store through the desktop context menu and a touch-reachable row action without disturbing existing tree interactions.

## Acceptance

- A single file or directory context menu invokes one session-bound add handler, confirms success, and omits the action during bulk selection.
- Phone and coarse-pointer rows expose a visible 44px overflow action that calls the same handler without opening/expanding/selecting the row.
- New menu, trigger, and feedback copy is localized in English and pseudo catalogs; existing editor, rename, download, delete, keyboard, drag/drop, and selection behavior remains intact.

## TDD sequence

1. Add failing file/directory/bulk context-menu cases and a row-action propagation/breakpoint case.
2. Implement the session-bound handler, prop wiring, desktop action, responsive touch menu, stable selectors, and localized copy.
3. Rerun the focused tests, i18n checks, targeted lint, and typecheck; inspect the rendered mobile row/menu before handing off to E2E.

## Files likely touched

- `apps/web/components/task/file-browser.tsx`
- `apps/web/components/task/file-browser-parts.tsx`
- `apps/web/components/task/file-context-menu.tsx`
- `apps/web/components/task/file-context-menu.test.tsx`
- `apps/web/components/task/file-browser-context-action.test.tsx`
- `apps/web/src/locales/en/chat.json`
- `apps/web/src/locales/pseudo/chat.json`

## Dependencies

Task 01.

## Parallelism

`sequential` — the action writes the directory-aware model established by Task 01, and its desktop/touch entry points share the same file-tree components.

## Verification

- `cd apps && pnpm install --frozen-lockfile`
- `cd apps && pnpm --filter @kandev/web test -- --run components/task/file-context-menu.test.tsx components/task/file-browser-context-action.test.tsx`
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm run i18n:pseudo && pnpm run i18n:check && pnpm run i18n:ratchet`
- `cd apps/web && pnpm exec eslint components/task/file-browser.tsx components/task/file-browser-parts.tsx components/task/file-context-menu.tsx components/task/file-context-menu.test.tsx components/task/file-browser-context-action.test.tsx`

## Inputs

- Spec `What` plus phone/coarse-pointer and bulk-selection scenarios.
- Plan `File-tree actions` and `Mobile design contract`.
- `apps/web/components/task/file-browser-toolbar.tsx`, `apps/web/components/task/mobile/mobile-sessions-section.tsx`, and `apps/web/app/globals.css` as responsive menu precedents.
- `apps/web/AGENTS.md` i18n, menu, touch, and component rules.

## Output contract

Report RED/GREEN evidence, actual files changed, exact commands/test counts, translated keys, rendered desktop/mobile inspection status, blockers/risks, and synchronized task/plan status in this conversation.

## Results

- RED: the new menu/row tests initially failed on the missing responsive action;
  after the implementation, the focused suite is green.
- GREEN: `cd apps && pnpm --filter @kandev/web test -- --run
  components/task/file-context-menu.test.tsx components/task/file-browser-context-action.test.tsx`
  — 2 files, 8 tests passed.
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps/web && pnpm run i18n:pseudo` — regenerated the pseudo chat catalog;
  `pnpm run i18n:check` and `pnpm run i18n:ratchet` passed. The Simplified Chinese chat
  catalog also contains the complete key set required by the current `main` catalog.
- Targeted ESLint exited successfully with no errors or warnings after extracting the file
  browser header and the focused E2E page object.
- Localized keys added: `chat:addToChatContext`, `chat:fileTreeActions`, and
  `chat:addedToChatContext` in English, pseudo, and Simplified Chinese catalogs.
- PR review remediation: Files-tab search results now wrap the same `FileContextMenu` and
  `FileTreeNodeTouchActions` used by tree rows, with a synthetic file node carrying the searched
  path/name. The new focused component test was RED before the wiring and GREEN at 2 tests; the
  shared handler/store path remains unchanged and search-result additions stay idempotent.
