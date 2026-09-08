---
id: "09-e2e-coverage"
title: "E2E coverage"
status: done
wave: 5
depends_on: ["08-conditional-pin-sync"]
plan: "plan.md"
spec: "../../specs/ui/requirements/agent-todo-list-panel.md"
---

# Task 09: E2E coverage

Extend `apps/web/e2e/tests/settings/todo-list-panel.spec.ts` with the
Iteration 2 user-facing scenarios: sub-option visibility/inhibition with
state preservation, and the only-pin-when-not-empty runtime behavior against
both an empty-todo session and a session whose todos are already persisted in
message history.

- **Acceptance:**
  1. New E2E tests cover: sub-option switch absent with the main preference
     off; visible after turning the main preference on; hidden again after
     turning it off; previously saved `true` value survives a page reload and
     shows checked when the main preference is re-enabled.
  2. Runtime tests cover: main on + sub-option on + task with no todos →
     no Todos tab; main on + sub-option on + task with persisted todos
     (`e2e:plan` script) → Todos tab auto-pinned.
  3. The full spec file passes: `pnpm exec playwright test
     e2e/tests/settings/todo-list-panel.spec.ts` (expect 6/6 with the 4
     existing + 2 new tests, or the exact count reported).
- **Verification:** `cd apps/web && pnpm exec playwright test e2e/tests/settings/todo-list-panel.spec.ts`
  (requires the e2e backend harness; run via the repo's documented e2e flow
  from `apps/web/e2e/README.md`).
- **Files likely touched:**
  - `apps/web/e2e/tests/settings/todo-list-panel.spec.ts`
  - `apps/web/e2e/helpers/api-client.ts` (only if Task 07 did not already add
    the `show_todo_list_panel_only_when_not_empty` response type)
- **Dependencies:** Task 08 (runtime behavior exists), Task 07 (settings UI
  exists).
- **Parallelism:** `sequential` (E2E follows the frontend changes it covers).
- **Inputs:** Spec Scenarios section
  (`docs/specs/ui/requirements/agent-todo-list-panel.md`); the existing
  `todo-list-panel.spec.ts` helpers (`createTaskWithSession`,
  `createTaskWithTodos`, `setTodoListPanelPreference`, `readTodosLayout`,
  `todosTabWrapper`) as the base; the settings-page switch locator pattern
  `getByRole("switch", { name: ... })`.

Implementation notes:

- Settings-page test flow (reuse the existing second-tab pattern from the
  "adds then removes the tab live" test, or a fresh page to
  `/settings/general/task-actions`):
  1. Assert the sub-option switch is absent while the main toggle is off.
  2. Click the main toggle ("Show agent todo list panel") → sub-option
     appears; toggle it on; save; assert
     `apiClient.getUserSettings()` reports
     `show_todo_list_panel_only_when_not_empty: true`.
  3. Toggle the main preference off; save; assert the sub-option switch is
     absent on the live page AND the persisted settings still report the
     sub-option `true`.
  4. Reload the settings page; toggle the main preference on; assert the
     sub-option switch appears already checked (state preserved).
- Runtime tests:
  - "does not pin for an empty todo list": set both preferences true via
    `rawRequest("PATCH", "/api/v1/user/settings", { show_todo_list_panel:
    true, show_todo_list_panel_only_when_not_empty: true })`, open a task
    from `createTaskWithSession` (no todos), assert `todosTabWrapper` count 0
    and `readTodosLayout` reports `todosExists: false` (poll 15s).
  - "pins when the todo list is not empty": same preferences, open a task
    from `createTaskWithTodos` (persisted `e2e:plan` entries), assert
    `todosTabWrapper` visible and `todosExists: true` beside Files/Changes.
- Keep `test.beforeEach`/`afterEach` baseline restore behavior correct for
  the new field: capture and restore
  `show_todo_list_panel_only_when_not_empty` alongside
  `show_todo_list_panel`.

## Results

Added three tests to `apps/web/e2e/tests/settings/todo-list-panel.spec.ts`:

1. "inhibits the only-pin-when-not-empty sub-option while the main preference
   is off, preserving its value" — settings page: sub-option absent with main
   off; appears when main on; persists `show_todo_list_panel_only_when_not_empty:
   true` via API; hidden (inhibited) when main turned off while the saved
   value survives; survives a page reload; reappears checked when main is
   re-enabled.
2. "does not auto-pin the Todos tab for an empty todo list when
   only-pin-when-not-empty is on" — both prefs true + task with no todos →
   `todosExists: false` beside Files/Changes.
3. "auto-pins the Todos tab when the todo list is not empty and
   only-pin-when-not-empty is on" — both prefs true + task with persisted
   `e2e:plan` todos → tab pinned beside Files/Changes.

Also extended `setTodoListPanelPreference` helpers (new
`setOnlyPinWhenNotEmptyPreference`) and `beforeEach`/`afterEach` baseline
capture/restore for the new field.

Command:
`cd apps/web && pnpm e2e:raw --project=chromium tests/settings/todo-list-panel.spec.ts`

First run: **1 passed, 7 failed** — every task-workbench test (including the
pre-existing v1 ones) hit "This page couldn't load." Trace inspection
(`0-trace.trace`) showed `Minified React error #185` (maximum update depth
exceeded) in the desktop layout render. Root cause: my
`useSyncTodoPanel` selector `state.sessionTodos.bySessionId[sessionId] ?? []`
returned a fresh `[]` on every read, so zustand re-rendered infinitely.
Fixed by returning the stable stored array or `undefined`
(`(liveTodos?.length ?? 0) > 0`), reran sync unit tests (23 passed) +
typecheck, rebuilt web (`make build-web`), and re-ran the spec.

Second run: **8 passed (1.3m)** — all 5 v1 tests + 3 new tests green.

Prereqs built for the run: `make -C apps/backend build`, `make -C apps/backend
e2e-plugin-package`, `make build-web`.

Blockers/risks: none.
