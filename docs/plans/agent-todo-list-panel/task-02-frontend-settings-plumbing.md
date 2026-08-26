---
id: "02-frontend-settings-plumbing"
title: "Frontend settings plumbing and UI"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/agent-todo-list-panel.md"
---

# Task 02: Frontend settings plumbing and UI

Add `showTodoListPanel` to the frontend `UserSettingsState`, its SSR
default/mapping, its WS sync mapping, and a new Settings-page toggle
component mounted on the General settings page, mirroring
`showTranscriptAutoScrollControl`/`UnreadDividerSettings` exactly. This task
does not depend on Task 01 landing first for local frontend unit tests (it can
stub the API response), but the toggle will not actually persist against a
real backend until Task 01's endpoint exists.

- **Acceptance:**
  1. `apps/web/lib/state/slices/settings/types.ts`'s `UserSettingsState` has
     `showTodoListPanel: boolean`; `createDefaultUserSettings()` defaults it
     to `false`.
  2. A new `TodoListPanelSettings` component renders a labeled switch bound to
     `userSettings.showTodoListPanel`, stages a draft, and on save calls
     `updateUserSettings({ show_todo_list_panel: <value> })` then
     `setUserSettings` with the merged value — verified by a component test.
  3. `TodoListPanelSettings` is mounted in `GeneralSettings`'s
     `TaskActionsSettings` section beside `UnreadDividerSettings`.
  4. New i18n keys exist in `apps/web/src/locales/en/settings.json` and are
     mirrored in `apps/web/src/locales/pseudo/settings.json`.
- **Verification:** `cd apps && pnpm install --frozen-lockfile && cd web && pnpm run typecheck && pnpm exec vitest run components/settings/todo-list-panel-settings.test.tsx lib/ssr/user-settings.test.ts`
- **Files touched (reconciled with actual diff):**
  - `apps/web/lib/state/slices/settings/types.ts`
  - `apps/web/lib/ssr/user-settings.ts`
  - `apps/web/lib/ssr/user-settings.test.ts`
  - `apps/web/lib/types/http-user-settings.ts` (`UserSettings`/`UserSettingsUpdatePayload` —
    `apps/web/lib/ws/handlers/users.ts` needed no code change: it maps through
    `mapUserSettingsData`, which picks up the new field automatically once the
    type and `buildBehaviorFields` mapping exist)
  - `apps/web/lib/ws/handlers/users.test.ts`
  - `apps/web/components/settings/todo-list-panel-settings.tsx` (new)
  - `apps/web/components/settings/todo-list-panel-settings.test.tsx` (new)
  - `apps/web/components/settings/general-settings.tsx`
  - `apps/web/src/locales/en/settings.json`
  - `apps/web/src/locales/pseudo/settings.json` (regenerated via
    `node scripts/generate-pseudo-locale.mjs`, not hand-edited)
  - `apps/web/hooks/use-ensure-user-settings.test.ts` (fixture completeness,
    surfaced by `tsc --noEmit`)
- **Dependencies:** None.
- **Parallelism:** `parallel-safe` (disjoint from Task 01's backend-only files).
- **Inputs:** Spec's Data model / API surface / Failure modes sections;
  plan's Frontend → "Settings state and persistence plumbing" and "Settings
  UI" subsections; `apps/web/components/settings/unread-divider-settings.tsx`
  and its test as the exact structural template; existing
  `showTranscriptAutoScrollControl` call sites in `types.ts` (~191),
  `user-settings.ts` (~45, ~243-244).

## Results

Added `showTodoListPanel` end-to-end on the frontend: `UserSettingsState`
type, `createDefaultUserSettings()` default (`false`), `buildBehaviorFields`
snake↔camel mapping, `UserSettings`/`UserSettingsUpdatePayload` wire types.
New `TodoListPanelSettings` component (copied from `UnreadDividerSettings`),
mounted in `TaskActionsSettings` under a new "Todo List Panel"
`SettingsSection`. Added i18n keys `showAgentTodoListPanel`,
`pinTheAgentsLiveTodoChecklistAs`, `todoListPanel` to `en/settings.json` at
their alphabetically-sorted positions; regenerated `pseudo/settings.json` via
the existing generator script rather than hand-transliterating.

Added `describe("todo list panel setting")` (user-settings.test.ts) and a
WS-sync case (users.test.ts) before implementing; confirmed both red
(assertion failures against `undefined`) before the implementation, green
after.

Commands and results:
- `pnpm exec vitest run lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts components/settings/todo-list-panel-settings.test.tsx` → 3 files, 53 tests passed.
- `pnpm run typecheck` → clean (after adding the missing field to
  `use-ensure-user-settings.test.ts`'s fixture, caught by `tsc`).
- `pnpm run i18n:check` → `2121 key(s) referenced, 2405 en entries, 0 orphans, pseudo in sync`.
