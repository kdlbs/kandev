---
id: "07-frontend-settings-plumbing"
title: "Frontend settings plumbing and UI"
status: done
wave: 5
depends_on: ["06-backend-sub-setting-field"]
plan: "plan.md"
spec: "../../specs/ui/requirements/agent-todo-list-panel.md"
---

# Task 07: Frontend settings plumbing and UI

Plumb `showTodoListPanelOnlyWhenNotEmpty` through the frontend settings
state, extend `TodoListPanelSettings` with the inhibited sub-option, and
update the main toggle's copy to state explicitly that it auto-pins the
agent's live todo checklist and that the Todos tab can always be added
manually.

- **Acceptance:**
  1. The sub-option switch labeled "Only pin when todo list is not empty" is
     rendered only while the main "Show agent todo list panel" toggle is on;
     with the main toggle off it is absent (inhibited), and its saved value
     is preserved: toggling the main toggle off and on again restores the
     sub-option in its previous state.
  2. Saving persists both fields: `updateUserSettings` is called with
     `{ show_todo_list_panel, show_todo_list_panel_only_when_not_empty }`.
  3. The main toggle's description copy says the preference automatically
     pins the checklist and that the Todos tab can always be added manually
     (no em dash; i18n keys updated in `en`, regenerated `pseudo`, and
     mirrored in `pt-pt`/`zh-cn`).
  4. `pnpm run typecheck`, `pnpm run i18n:check`, and the touched unit tests
     pass; the component test goes red before implementation and green after.
- **Verification:** `cd apps/web && pnpm exec vitest run components/settings/todo-list-panel-settings.test.tsx lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts && pnpm run typecheck && pnpm run i18n:pseudo && pnpm run i18n:check`
  (fresh worktree bootstrap first if needed: `cd apps && pnpm install --frozen-lockfile`).
- **Files likely touched:**
  - `apps/web/lib/state/slices/settings/types.ts`
  - `apps/web/lib/ssr/user-settings.ts`
  - `apps/web/lib/ssr/user-settings.test.ts`
  - `apps/web/lib/types/http-user-settings.ts`
  - `apps/web/lib/ws/handlers/users.test.ts`
  - `apps/web/components/settings/todo-list-panel-settings.tsx`
  - `apps/web/components/settings/todo-list-panel-settings.test.tsx`
  - `apps/web/e2e/helpers/api-client.ts`
  - `apps/web/src/locales/en/settings.json`
  - `apps/web/src/locales/pseudo/settings.json` (regenerated)
  - `apps/web/src/locales/pt-pt/settings.json`
  - `apps/web/src/locales/zh-cn/settings.json`
- **Dependencies:** Task 06 (wire field exists on the backend).
- **Parallelism:** `sequential`.
- **Inputs:** Spec What / Data model / API surface / Scenarios sections;
  plan's "Iteration 2 additions" Frontend sections; the existing
  `TodoListPanelSettings` component and its test as the base to extend;
  `docs/i18n.md` for the pseudo-locale regeneration and em-dash rule.

Implementation notes:

- Keep the main toggle's `Label` text ("Show agent todo list panel") and
  `id="show-todo-list-panel"` unchanged so existing E2E locators and the
  existing component test keep working.
- Convert the component's single draft boolean to a
  `{ show: boolean; onlyWhenNotEmpty: boolean }` pair; `isDirty` compares
  both; `useSettingsSaveContributor` revision = `JSON.stringify(draft)`;
  `save` sends both snake_case fields and updates both store fields;
  `discard` restores both. The `useEffect` hydration sync must compare the
  full pair.
- Sub-option row: `Label htmlFor="todo-list-panel-only-when-not-empty"` +
  `Switch id="todo-list-panel-only-when-not-empty"`, rendered only when
  `draft.show` is true (unmounted when off — inhibited, not disabled).
- New en copy for `pinTheAgentsLiveTodoChecklistAs`: "Automatically pin the
  agent's live todo checklist as a Todos tab in the right panel. Even when
  this is off, you can always add the Todos tab manually." New key
  `onlyPinWhenTodoListIsNotEmpty`: "Only pin when todo list is not empty."
  Run `pnpm run i18n:pseudo` to regenerate pseudo, then mirror both strings
  into `pt-pt` and `zh-cn` (keep "Todos" untranslated as a UI noun).
- Component test additions: sub-option absent with main off; appears when
  main toggled on; toggling main off hides it; main on again restores its
  checked state; save calls `updateUserSettings` with both fields.
- SSR mapping test: `showTodoListPanelOnlyWhenNotEmpty` defaults to `false`
  and maps from `show_todo_list_panel_only_when_not_empty: true`.

## Results

Red first: added the sub-option cases to
`components/settings/todo-list-panel-settings.test.tsx` (inhibited while
main off; appears checked when main on with preserved value; both fields in
the save payload; state survives a main off/on cycle without saving), the
SSR mapping case to `lib/ssr/user-settings.test.ts`, and the WS sync case to
`lib/ws/handlers/users.test.ts`. `pnpm exec vitest run <3 files>` → 5 failed
(expected assertions: `expected undefined to be false/true`, missing
sub-option switch, save payload without the new field).

Implemented:
- `lib/state/slices/settings/types.ts`, `lib/ssr/user-settings.ts` (default
  `false` + `buildBehaviorFields` mapping), `lib/types/http-user-settings.ts`
  (both types), `e2e/helpers/api-client.ts` (response type).
- `components/settings/todo-list-panel-settings.tsx`: two-field
  `{ show, onlyWhenNotEmpty }` draft, `JSON.stringify` revision, save sends
  both snake_case fields and updates both store fields; sub-option row
  rendered only when `draft.show` is true (inhibited, not disabled); main
  toggle `Label`/`id` unchanged.
- i18n: `en/settings.json` — updated `pinTheAgentsLiveTodoChecklistAs`
  ("Automatically pin the agent's live todo checklist as a Todos tab in the
  right panel. Even when this is off, you can always add the Todos tab
  manually.") and added `onlyPinWhenTodoListIsNotEmpty`; regenerated pseudo
  (`pnpm run i18n:pseudo`, 2 additions/1 update); mirrored both into `pt-pt`
  and `zh-cn`.
- `hooks/use-ensure-user-settings.test.ts`: added the new field to the
  `makeUnloadedSettings()` fixture (required by `UserSettingsState`).

Commands:
- `cd apps/web && pnpm exec vitest run components/settings/todo-list-panel-settings.test.tsx lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts`
  → 62 passed (3 files), red→green.
- `cd apps/web && pnpm run typecheck` → clean (first attempt crashed node
  with exit 134 on this VM; re-ran with `NODE_OPTIONS=--max-old-space-size=4096`
  → clean; one TS error `Property 'showTodoListPanelOnlyWhenNotEmpty' is
  missing` in the fixture fixed).
- `cd apps/web && pnpm exec vitest run hooks/use-ensure-user-settings.test.ts`
  → 8 passed.
- `cd apps/web && pnpm run i18n:check` → all 5 gates pass ("i18n keys OK —
  6631 key(s) referenced, 7890 en entries, 10 orphan(s), pseudo in sync. 35
  advisory pt-pt, zh-cn issue(s)" — advisory real-locale gaps only).

Blockers/risks: none.

Reviewer-feedback follow-up (round 2, finding F1 = round-1 N2): both
switches now carry per-field `data-settings-dirty`
(`draft.show !== saved.show` / `draft.onlyWhenNotEmpty !==
saved.onlyWhenNotEmpty`) instead of the shared card-level flag, matching the
established per-control pattern (e.g. `lsp-language-cards.tsx`). Added a
component test "marks each toggle dirty only for its own unsaved change"
(four render blocks deduplicated into `renderTodoListPanelSettings` to stay
under the 100-line lint cap).
