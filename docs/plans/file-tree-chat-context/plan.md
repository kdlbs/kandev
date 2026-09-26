---
spec: docs/specs/ui/requirements/file-tree-chat-context.md
created: 2026-08-04
status: complete
---

# Implementation Plan: File Tree Chat Context

## Overview

Extend the existing session-scoped context-file model so it can represent directories without breaking stored file entries, then wire a single-node add action into the Files tree's context menu and a visible touch/coarse-pointer row menu. The work reuses the current chat composer, message formatting, `context_files` metadata, Radix menu primitives, and responsive bottom-sheet CSS. Direct and queued sends carry `{ path, name }` plus optional `is_directory` metadata through the existing user-message path, preserving legacy file entries. Focused component tests land with each frontend step, followed by desktop and mobile Playwright coverage of the complete Files-to-Chat flow.

## Backend

No database or filesystem changes are required. `message.add.context_files` accepts task-root-relative `{ path, name }` references with optional `is_directory` identity and persists them in user-message metadata. The queue WebSocket request accepts the same optional `context_files` array, and queue draining preserves it through the existing metadata path; no schema or new message contract is needed. The hidden prompt block still tells the agent whether each pending path is a file or directory before send.

## Frontend

### Directory-aware context items

- Extend `ContextFile` in `apps/web/lib/state/context-files-store.ts` with optional directory identity while preserving hydration of older session-storage entries. Persist that property with the existing `path`, `name`, and `pinned` fields; retain path-based deduplication and ephemeral clearing.
- Extend `FileContextItem` in `apps/web/lib/types/context.ts` and `buildFileContextItem` in `apps/web/components/task/chat-context-items.ts` so directory items have no file-open handler or lazy file preview.
- Update `apps/web/components/task/chat/context-items/file-item.tsx` to render the existing `ContextChip` with `IconFolder` for directories and `IconFile` plus `LazyFilePreview` for files. Give path chips stable `data-testid`/path metadata through `ContextChip` for component and E2E assertions.
- Update `buildContextFilesContext` in `apps/web/hooks/use-message-handler.ts` to describe attached context as file and directory paths while preserving optional directory identity in the filtered `context_files` metadata payload. Forward that payload through direct and queued sends. Add tests for file-only, directory-only, mixed, duplicate, legacy-hydration, queued metadata, and successful/failed-send retention behavior at the narrowest owning layers.

### File-tree actions

- In `useFileBrowserHandlers` (`apps/web/components/task/file-browser.tsx`), add one session-bound handler that writes `{ path, name, isDirectory }` to `useContextFilesStore`, leaves it unpinned, and emits localized confirmation feedback. Pass this handler through `FileBrowserContentArea` and `TreeNodeItem` in `apps/web/components/task/file-browser-parts.tsx`.
- Add a localized **Add to chat context** item to `FileContextMenu` in `apps/web/components/task/file-context-menu.tsx` for a single file or directory. Keep it hidden while `selectedCount > 1`, and preserve all existing editor, delete, rename, and download separators and actions.
- Read `isMobile` and `isFinePointer` once through `useResponsiveBreakpoint` in the file-browser composition. On phone or coarse-pointer layouts, render a visible per-row 44px overflow trigger that stops row open/expand/selection events and exposes the same add handler through `@kandev/ui/dropdown-menu`. Fine-pointer desktop keeps the existing right-click interaction and row density.
- Add the menu label, accessible trigger label, and success feedback to `apps/web/src/locales/en/chat.json`, regenerate `apps/web/src/locales/pseudo/chat.json`, and use `useTranslation("chat")` at render/callback time.

### Mobile design contract

- **Desktop outcome / mobile entry:** fine-pointer desktop users right-click a file-tree row; phone and coarse-pointer users open the visible row overflow action from the existing Files bottom-navigation destination.
- **Nearest shipped exemplars:** `apps/web/components/task/file-browser-toolbar.tsx` contributes the visible 44px responsive overflow trigger and `DropdownMenu`; `apps/web/components/task/mobile/mobile-sessions-section.tsx` contributes a row-level touch overflow pattern; `apps/web/app/globals.css` contributes the inset, safe-area-aware mobile menu treatment.
- **Hierarchy and primary action:** opening a file or expanding a folder remains the row's primary action. Add to chat context is a secondary action inside the explicit overflow/right-click menu.
- **Presentation and rationale:** use the existing compact dropdown/context menu, which becomes an inset bottom-sheet menu below 640px. This is a short, temporary, one-step choice and does not justify a new full-height surface or navigation route.
- **Geometry and scrolling:** each touch trigger and menu row is at least 44px in its active dimension. The existing menu content owns overflow, stays within `70dvh`, clears the bottom safe area, and does not create document-level horizontal scrolling.
- **Shared versus responsive logic:** both entry points call the same session-bound add handler and context store. Only the trigger presentation differs by pointer/viewport.
- **Mobile proof:** a Pixel 5 Playwright test taps the visible row action, selects Add to chat context from the bottom menu, navigates to Chat, and observes the same folder chip and sent-message outcome as desktop.

## Tests

- **What:** directory identity round-trips through session storage; old entries remain valid; path deduplication and ephemeral clearing apply equally to files and directories.
  **Files:** `apps/web/lib/state/context-files-store.test.ts`.
  **How:** focused Zustand store tests with mocked session storage.
- **What:** file context items retain previews/open behavior, directory items use a folder icon and do not call the file opener or mount `LazyFilePreview`.
  **Files:** `apps/web/components/task/chat-context-items.test.ts`, `apps/web/components/task/chat/context-items/file-item.test.tsx`.
  **How:** pure builder assertions plus React Testing Library interaction/render tests.
- **What:** hidden context text names file and directory paths while outbound `context_files` metadata preserves optional directory identity and legacy `{ path, name }` entries; failed sends do not clear pending context and successful sends do.
  **Files:** `apps/web/hooks/use-message-handler.test.ts`, `apps/web/hooks/domains/session/use-queue.test.ts`, `apps/web/lib/api/domains/queue-api.test.ts`, `apps/backend/internal/orchestrator/handlers/queue_handlers_test.go`, and existing chat input area tests where clearing is already owned.
  **How:** focused formatter and submission-state tests with the existing WebSocket/store mocks.
- **What:** queued context metadata survives the queue API and WebSocket handler into the stored user message.
  **Files:** `apps/web/hooks/domains/session/use-queue.test.ts`, `apps/web/lib/api/domains/queue-api.test.ts`, and `apps/backend/internal/orchestrator/handlers/queue_handlers_test.go`.
  **How:** focused frontend payload assertions plus a backend queue-handler metadata assertion.
- **What:** right-click menus expose the action for a file and a directory, invoke the supplied node callback once, deduplicate through the owning store handler, and omit the action for bulk selection; the touch trigger stops the row's primary action.
  **Files:** `apps/web/components/task/file-context-menu.test.tsx`, `apps/web/components/task/file-browser-context-action.test.tsx`, and `apps/web/components/task/file-browser-search-context-action.test.tsx`.
  **How:** React Testing Library context-menu/dropdown events with mocked breakpoint and store state.
- **Targeted command:** `cd apps && pnpm --filter @kandev/web test -- --run lib/state/context-files-store.test.ts components/task/chat-context-items.test.ts components/task/chat/context-items/file-item.test.tsx hooks/use-message-handler.test.ts hooks/domains/session/use-queue.test.ts lib/api/domains/queue-api.test.ts components/task/chat/chat-input-area.test.tsx components/task/file-context-menu.test.tsx components/task/file-browser-context-action.test.tsx components/task/file-browser-search-context-action.test.tsx`.

## E2E Tests

- **Scenario:** given a desktop task with a file and directory, right-clicking each node and choosing Add to chat context creates one correctly typed chip per path; adding a duplicate remains idempotent; sending records both paths and clears the ephemeral composer chips.
  **File:** `apps/web/e2e/tests/task/file-tree-chat-context.spec.ts`.
  **What to verify:** stable node/search-result/menu/chip selectors, exact session ownership, deduplication, sent user-message badges, and no file-open side effect.
- **Scenario:** given the same task on Pixel 5, visible file-tree and search-result row actions add a directory without right-click or long press.
  **File:** `apps/web/e2e/tests/task/mobile-file-tree-chat-context.spec.ts`.
  **What to verify:** `.tap()` opens the responsive menu, trigger and action rows meet the 44px contract, menu bounds stay inside the viewport, document horizontal overflow is absent, the Chat tab shows the folder chip, and send clears it after recording the path.
- **Page object:** add focused file-tree context-menu, touch-trigger, and context-chip helpers through `apps/web/e2e/pages/file-tree-page.ts`, composed by `apps/web/e2e/pages/session-page.ts`.
- **Targeted commands:**
  - `cd apps/web && pnpm e2e:run tests/task/file-tree-chat-context.spec.ts`
  - `cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-file-tree-chat-context.spec.ts`

## Verification Results

- RED/GREEN frontend coverage: the focused suite first failed on the new directory model and
  action cases, then passed with 7 files and 52 tests after the PR fixup added prompt-path
  sanitization and interaction coverage. The final command was
  `cd apps && pnpm --filter @kandev/web test -- --run lib/state/context-files-store.test.ts
  components/task/chat-context-items.test.ts components/task/chat/context-items/file-item.test.tsx
  hooks/use-message-handler.test.ts components/task/chat/chat-input-area.test.tsx
  components/task/file-context-menu.test.tsx components/task/file-browser-context-action.test.tsx`.
- `cd apps/web && pnpm run typecheck` passed. The i18n pseudo-catalog, key check, ratchet, and
  Simplified Chinese catalog parity checks passed. Targeted ESLint passed with no errors, and
  the final commit-hook web lint passed with no warnings.
- Desktop managed E2E: `file-tree-chat-context.spec.ts` — 1 test passed on the initial run and
  again after the PR fixup, including the explicit collapsed-directory assertion.
- Mobile managed E2E: `mobile-file-tree-chat-context.spec.ts` with `mobile-chrome` — 1 test
  passed on the initial run and again after the PR fixup. The test observed a 44px row trigger,
  a settled 44px menu item, viewport-bounded menu geometry, and no document horizontal overflow.
- Each managed runner started from a fresh production build and logged cleanup of E2E results,
  blob reports, PR assets, and shard logs. No database schema or filesystem changes were required;
  the queue metadata extension reused the existing user-message metadata path.
- PR review remediation: queued sends now preserve `context_files` through the frontend queue
  hook/API and backend queue handler; Files search results now share the desktop context menu and
  coarse-pointer overflow action with tree rows. The initial RED regressions were 2 failed
  assertions in the queue tests, 2 failed search-action assertions, and a backend compile failure
  for the missing queue metadata constant. GREEN coverage was 6 frontend files/69 tests, the
  focused queue-handler test passed 9 tests, and the adjacent orchestrator packages passed 1,640
  tests. The first mobile rerun also confirmed the search API returns files rather than directory
  rows; the final fixture searches a file while retaining the directory tree-row flow. Final
  managed E2E runs passed 1 desktop test and 1 Pixel 5 mobile test from fresh production builds,
  with the expected menu hitbox, viewport, overflow, metadata, and cleanup checks.
- Rebase PR-fixup remediation: partial bulk deletion now removes only paths whose backend
  deletion fulfilled successfully, and sent context metadata carries optional `is_directory`
  identity through direct and queued messages into folder/file history badges. RED covered the
  partial-delete, metadata, and badge regressions; GREEN passed 6 web files/95 tests and 2 backend
  packages/10 focused tests, plus typecheck and changed-file ESLint.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-model-directory-context](task-01-model-directory-context.md) — done

Wave 2:

- [x] [task-02-add-file-tree-action](task-02-add-file-tree-action.md) — done

Wave 3:

- [x] [task-03-cover-responsive-flow](task-03-cover-responsive-flow.md) — done

All tasks are sequential. Task 02 consumes the context model from Task 01, and Task 03 proves the integrated behavior after both frontend layers land.

## Risks

- The context store is shared by ACP and passthrough composers; directory metadata must remain optional so existing stored entries and file-only callers keep working.
- A touch overflow button lives inside a clickable/expandable tree row. It must stop propagation without changing keyboard tree navigation, drag/drop, selection, or the row's primary tap target.
- The file tree can be mounted beside a non-active chat panel. The handler must key writes by the file browser's `sessionId`, not global active-session state.
- Directory contents can change between attachment and send. The feature intentionally sends a path reference and does not snapshot or recursively enumerate the directory.

## Open Questions

None.
