---
spec: docs/specs/quick-chat-idle-dot/spec.md
created: 2026-08-23
status: implemented
---

# Implementation Plan: Quick Chat Activity Indicators

## Overview

Extend the existing completion-only Quick Chat dot into a derived running or finished activity state. First add pure selectors, then update tabs and entry icons, and finally cover the real lifecycle in Playwright.

No backend contract changes are necessary. The frontend already receives session state, foreground activity, preparation progress, and completion events.

## Frontend

### Activity selectors

- Add `apps/web/lib/state/slices/ui/quick-chat-activity-selectors.ts`.
- Export `QuickChatActivityState` as `"running" | "finished" | null`.
- Extract the shared live-work predicate to `apps/web/lib/session-working.ts`. Use it from `deriveSessionFlags()` and the Quick Chat selector.
- Add `selectQuickChatSessionIsWorking(state, sessionId)`. Include `prepareProgress.bySessionId[sessionId]?.status === "preparing"`.
- Add `selectQuickChatActivity(state, workspaceId)`. Return `null` while the dialog is open or the workspace is absent.
- For a closed dialog, return `"running"` when any workspace conversation is working. Otherwise, return `"finished"` when the workspace has an unseen marker.
- Keep `quick-chat-unseen-selectors.ts` for the existing marker-level tests and lifecycle helpers.

### Tab status

- Update `apps/web/components/quick-chat/quick-chat-modal.tsx` with a conversation-tab wrapper that reads `selectQuickChatSessionIsWorking` for one session.
- Add an `isWorking` prop to `QuickChatTabItem` in `quick-chat-tab-item.tsx`.
- Render `GridSpinner` before the tab name while `isWorking` is true.
- Do not add a spinner to setup tabs or `QuickTerminalTabItem`.

### Entry activity bubble

- Add `apps/web/components/quick-chat/quick-chat-activity-indicator.tsx` for the shared bubble.
- Use `bg-blue-500` for `running` and `bg-emerald-500` for `finished`.
- Add `data-testid="quick-chat-activity-indicator"`, `data-state`, and `data-legacy-testid="quick-chat-unseen-dot"` during the selector migration.
- Add `apps/web/components/quick-chat/use-quick-chat-activity.ts` to select the aggregate state and resolve the accessible button label.
- Use the shared hook and indicator in these entry points:
  - `apps/web/components/app-sidebar/app-sidebar-primary-nav.tsx`
  - `apps/web/components/app-sidebar/app-sidebar-new-task-item.tsx`
  - `apps/web/components/kanban/kanban-header.tsx`
  - `apps/web/components/kanban/kanban-header-mobile.tsx`
  - `apps/web/components/task/mobile/quick-chat-sheet-button.tsx`
- Replace the boolean `dot` props in `AppSidebarNavItem` and `RowActionButton` with the typed activity state.
- Add `sidebar:quickChatRunning` in English, Portuguese, Simplified Chinese, Traditional Chinese, and the generated pseudo-locale. Keep `sidebar:quickChatUnseen` for the finished label.

### Mobile parity

This change modifies status content inside existing controls. It does not change composition, navigation, scrolling, or touch behavior.

The nearest mobile exemplars are the current mobile Quick Chat idle dot and the status dot in `session-mobile-bottom-nav.tsx`. Keep the existing mobile header and task-switcher entry points, touch targets, and dismissal behavior.

## Tests

- **What:** The shared working predicate includes `STARTING`, `RUNNING`, and background work.
  **File:** `apps/web/lib/session-working.test.ts`.
  **How:** Use a table-driven unit test for every session state and foreground activity value.
- **What:** Quick Chat working detection also includes environment preparation.
  **File:** `apps/web/lib/state/slices/ui/quick-chat-activity-selectors.test.ts`.
  **How:** Use selector tests with two workspaces and two sessions.
- **What:** Running has priority over finished, the dialog suppresses the bubble, and seen results remain clear after close.
  **File:** `apps/web/lib/state/slices/ui/quick-chat-activity-selectors.test.ts`.
  **How:** Use pure aggregate-state tests.
- **What:** A conversation tab shows the grid spinner only while its session works.
  **Files:** `apps/web/components/quick-chat/quick-chat-tab-item.test.tsx` and `quick-chat-modal.test.tsx`.
  **How:** Test the tab prop and the modal mapping for server-backed, setup, and terminal tabs.
- **What:** Blue and emerald map to the correct semantic state.
  **File:** `apps/web/components/quick-chat/quick-chat-activity-indicator.test.tsx`.
  **How:** Use rendered class and `data-state` assertions.
- **What:** Every desktop, tablet, and mobile Quick Chat entry uses the aggregate state and accessible label.
  **Files:** Existing sidebar, kanban-header, and mobile task-switcher component tests.
  **How:** Use store-backed rendered tests for no activity, running, and finished.

## E2E Tests

- **Scenario:** A real quick-chat turn starts, the tab shows a grid spinner, the dialog closes, and the entry shows the blue state.
  **File:** `apps/web/e2e/tests/chat/quick-chat-idle-dot.spec.ts`.
  **What to verify:** Tab spinner visibility and the desktop sidebar `data-state="running"` value.
- **Scenario:** The last agent settles while the dialog is closed.
  **File:** `apps/web/e2e/tests/chat/quick-chat-idle-dot.spec.ts`.
  **What to verify:** The same bubble changes to `data-state="finished"`, then disappears after the dialog opens.
- **Scenario:** The same running-to-finished lifecycle occurs at tablet width.
  **File:** `apps/web/e2e/tests/chat/quick-chat-idle-dot.spec.ts`.
  **What to verify:** The tablet header uses the same state sequence.
- **Scenario:** The mobile header and task-switcher entry expose the same activity state.
  **File:** `apps/web/e2e/tests/chat/mobile-quick-chat-idle-dot.spec.ts`.
  **What to verify:** Touch entry, close behavior, blue running state, emerald finished state, and clear-on-open behavior.

The mobile Playwright run is also the focused rendered mobile check. It keeps the existing full-height dialog and touch paths unchanged.

## Verification Results

- Focused frontend tests passed: 10 files, 107 tests.
- `pnpm --filter @kandev/web lint` passed with zero warnings.
- `pnpm --filter @kandev/web run typecheck` passed.
- `pnpm --filter @kandev/web run i18n:check` passed; all supported catalogs are complete and pseudo is in sync.
- `pnpm --filter @kandev/web run i18n:ratchet` passed with zero new violations.
- `pnpm --filter @kandev/web run lint:e2e-sleeps` passed.
- Desktop and tablet Playwright coverage passed: 2 tests.
- Mobile Playwright coverage passed: 2 tests.
- The full `i18n:zh-hant` generator remains blocked by pre-existing residual simplified-Chinese keys in `agents:dynamicProfileSettings`; the targeted `sidebar` conversion completed successfully for this change.

## Implementation Waves And Parallel Candidates

The tasks run sequentially because the UI and E2E work depend on the activity selectors.

- [x] [task-01-activity-selectors](task-01-activity-selectors.md)
- [x] [task-02-activity-ui](task-02-activity-ui.md)
- [x] [task-03-activity-e2e](task-03-activity-e2e.md)

## Open Questions

None. The plan uses blue for running and emerald for finished to match existing Kandev status-dot semantics.
