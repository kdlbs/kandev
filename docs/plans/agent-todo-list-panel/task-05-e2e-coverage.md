---
id: "05-e2e-coverage"
title: "E2E coverage"
status: done
wave: 4
depends_on: ["04-visibility-sync-hook"]
plan: "plan.md"
spec: "../../specs/ui/agent-todo-list-panel.md"
---

# Task 05: E2E coverage

Cover the full user-visible flow: preference off by default, turning it on
adds the tab live, turning it off removes it live, and a configured custom
placement is respected.

- **Acceptance:**
  1. A fresh task with the preference off has no "Todos" tab.
  2. Turning the preference on from `Settings > General` adds an inactive
     "Todos" tab to the currently active task without changing the selected
     tab; selecting it shows the checklist or empty state.
  3. Turning the preference off removes the "Todos" tab from the currently
     open task immediately, with no navigation/reload.
  4. A custom Default layout with `todos` placed in a specific group causes a
     fresh task to show the Todos tab in that configured group instead of the
     Files/Changes fallback.
- **Verification:** `cd apps && pnpm install --frozen-lockfile && cd web && pnpm exec playwright test e2e/tests/settings/todo-list-panel.spec.ts`
- **Files likely touched:**
  - `apps/web/e2e/tests/settings/todo-list-panel.spec.ts` (new)
  - `apps/web/e2e/tests/settings/layout-profiles.spec.ts` (extend with the
    configured-placement case, or keep it in the new file if that fits the
    existing file's scope better)
- **Dependencies:** Task 04 (the setting must actually gate the live panel
  before an E2E test can assert on it).
- **Parallelism:** `sequential` (final task; depends on the full stack being
  wired).
- **Inputs:** Spec's Scenarios section (each E2E case maps directly to one
  scenario); plan's E2E Tests section; `apps/web/e2e/tests/pr/pr-detail-layout.spec.ts`
  and `apps/web/e2e/tests/settings/layout-profiles.spec.ts` as structural
  templates for driving Settings toggles and asserting on the Dockview tab
  strip within Playwright.

## Results

All 3 scenarios covered in `todo-list-panel.spec.ts` (the fresh-task-hidden,
cross-tab live add/remove, and configured-placement cases; the fourth
acceptance item — turning it off removes the tab immediately — is the second
half of the "adds then removes" test rather than a separate one, since it
needs the tab present first).

First run surfaced one test-authoring bug and two real product bugs the
earlier waves' unit tests (which mock the store/API layer) could not have
caught, because they never exercise the actual bundled Dockview panel
registries or a live cross-tab settings round trip:

1. **Test bug:** `setTodoListPanelPreference` called `response.ok()` on the
   plain `fetch` `Response` `rawRequest` returns — `ok` is a boolean
   property there, not a method (confirmed against every other `rawRequest`
   caller in `api-client.ts`, which all read `.ok` as a property). Fixed to
   `expect(response.ok).toBe(true)`.
2. **Product bug:** `dockview-desktop-layout.tsx` — the file that actually
   backs the `/t/:id` desktop task workbench the spec drives — declares its
   *own* `components` map (module-local `const components = {...}`, passed
   as `<DockviewReact components={components}>`), separate from and parallel
   to `dockview-shared.tsx`'s exported `dockviewComponents` (which backs the
   *Office* task layout at `app/office/tasks/[id]/office-dockview-layout.tsx`).
   Task 03 added `todos: PortalSlot` only to the latter. Dockview silently
   failed to add an unregistered-component panel, so the live sync hook's
   `focusOrAddPanel` call was a no-op — explaining why the cross-tab test's
   tab never appeared even though the WS `user.settings.updated` notification
   demonstrably reached the second page (`[ws:dispatch] notification
   action=user.settings.updated ... handlers=1`, confirmed via temporary
   console instrumentation) and `useSyncTodoPanel` was correctly wired into
   this file. `dockview-panel-content.tsx` (this file's real `renderPanel`,
   again distinct from `dockview-shared.tsx`'s copy used only by Office) had
   the matching gap: no `"todos"` case, so even a manually-added panel would
   have rendered "Unknown panel: todos". Fixed both: added
   `todos: PortalSlot` to the `components` map, and a `TodosContent`
   function + `case "todos"` to `renderPanel`, mirroring `dockview-shared.tsx`'s
   already-correct versions exactly (same `useSessionTodoItems` /
   `TodoIndicatorContent` / `resolveStatus` reuse, same empty-state key).
3. **Product bug:** `apps/web/components/settings/layouts/layout-editor.tsx`'s
   `placeholderComponents` — a *third*, independent Dockview component map
   backing the Layout Editor's live preview instance — was also missing
   `todos`, so `Settings > General > Layouts`' "Add panel" menu could not
   place it at all (contrary to the spec's "Todos still appears in the
   visual editor's addable-panel list" scenario). Fixed by adding
   `todos: PlaceholderPanel`.

All three gaps are the same shape: this codebase keeps parallel,
hand-maintained Dockview component registries per rendering context (main
desktop workbench, Office workbench, Layout Editor preview) rather than one
shared source of truth — a pre-existing pattern, not something introduced by
this feature. Consolidating them is a larger refactor outside this task's
scope; not attempted here.

Added `apps/web/components/task/dockview-panel-content.todos.test.tsx`
(mirrors `dockview-shared.test.tsx`'s existing todos-panel test 1:1, but
against `dockview-panel-content.tsx` — the registry that actually backs the
desktop workbench under test) so the render-content half of this regression
has unit coverage in addition to E2E coverage. The component-registration
half (an unregistered Dockview component id being a silent no-op rather than
a thrown error) is guarded by the E2E test itself; the two heavier registry
files (`dockview-desktop-layout.tsx`, `layout-editor.tsx`) were judged too
expensive/fragile to import bodily into vitest just to assert on an internal
map, versus the E2E test that already exercises them for real.

Rebuilt both artifacts before every E2E run in this task
(`make -C apps/backend build`; `pnpm --filter @kandev/web build:vite`) — the
E2E fixture serves the pre-built `apps/backend/bin/kandev` binary and
`apps/web/dist` static bundle, not the live source tree, so source edits are
invisible to a run until rebuilt.

Commands and results (final, after all fixes):
- `cd apps/web && pnpm exec playwright test e2e/tests/settings/todo-list-panel.spec.ts` → 3 passed.
- `cd apps/web && pnpm exec playwright test e2e/tests/settings/layout-profiles.spec.ts e2e/tests/settings/todo-list-panel.spec.ts` → 7 passed (no regression in the pre-existing layout-profiles suite from the two registry edits).
- `cd apps/web && pnpm exec vitest run components/task/ components/settings/ lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts hooks/use-ensure-user-settings.test.ts` → 302 files / 2188 tests passed, 4 skipped; 1 unrelated pre-existing flaky test (`storage-maintenance-settings.test.tsx`, untouched by this feature) timed out under full-directory load and passed cleanly in isolation.
- `cd apps/web && pnpm run typecheck` → clean.
- `cd apps/web && pnpm run i18n:check` → `2123 key(s) referenced, 2407 en entries, 0 orphans, pseudo in sync`.

**Addendum (found during a manual demo-server review after this task's E2E
suite was already green):** a 4th bug, upstream of anything the E2E suite's
own mocked-timing exercised. `TodosContent` (both files) passed `[]` as the
`messageTodos` fallback to `useSessionTodoItems` — a deliberate but wrong
choice, since `sessionTodos.bySessionId` (the live store slice) is
populated *exclusively* by the `session.todos.updated` WS handler and is
never backfilled from history. Any session that already had todo data
*before* the viewing page loaded — a fresh page load, a reopened task, or
simply a task whose plan update completed before a demo browser connected —
showed "No todos yet." in the new panel while the adjacent `TodoIndicator`
chat-status-bar chip, looking at the identical session at the identical
moment, correctly showed "2/5" — because `TodoIndicator`'s call site already
derives a `messageTodos` fallback from the latest persisted `todo`-type
message (`buildTodoItems` in `hooks/use-processed-messages.ts`, previously
module-private) rather than passing `[]`. This directly violates the spec's
"shows the same rows... that the TodoIndicator popover shows for that
session" scenario for exactly the case — an already-completed session — the
E2E suite's own tests happened to always open a task via a still-connected
page immediately after the WS-live update fired, never exercising a true
cold load. Caught only by manually seeding a real backend (see below) and
looking at it, not by any automated test in this task.

Fixed by exporting `buildTodoItems` and having both `TodosContent`s call
`useSessionMessages(sessionId)` + `buildTodoItems(messages)` for the
fallback, exactly mirroring `TodoIndicator`'s data source. Added a
`data-testid="todos-panel"` wrapper (both files) so E2E/unit assertions can
scope to the panel's own rendered rows without colliding with the same
session's chat-transcript text or the (possibly still-mounted-but-hidden)
status-bar popover using the identical row markup. Added a 3rd unit case to
each `TodosContent` test file (message-history fallback with an empty live
store) and a 4th E2E case
(`shows the checklist for a session whose todos were already persisted
before this page ever loaded it`) seeding a task via the mock agent's
`e2e:plan(...)` scripting command, letting it settle, *then* turning the
preference on and opening it fresh.

To manually review the feature, a real backend + frontend was run outside
the E2E harness: `apps/backend/bin/kandev __backend` (the same hidden
subcommand the E2E fixture uses to skip the runtime-bundle bootstrap) with
`KANDEV_MOCK_AGENT=true`, `KANDEV_WEB_DIST_DIR` pointed at the built
`apps/web/dist`, and a fresh `HOME`/`KANDEV_DATABASE_PATH`, seeded via direct
REST calls (workspace, workflow, local git repo, a task whose description is
an `e2e:plan(...)` command) — this is what surfaced the bug via a
`tab.screenshot()` comparison against the chat status bar, which no unit or
E2E test had put side by side.

Re-verification after the fix, all green:
- `pnpm exec vitest run components/task/dockview-panel-content.todos.test.tsx components/task/dockview-shared.test.tsx components/task/dockview-panel-content.diff.test.tsx` → 3 files, 8 tests passed.
- `pnpm exec playwright test e2e/tests/settings/todo-list-panel.spec.ts` → 4 passed.
- `pnpm exec playwright test e2e/tests/settings/todo-list-panel.spec.ts e2e/tests/chat/chat-status-bar.spec.ts` → 10 passed (no regression in the pre-existing `TodoIndicator`/status-bar coverage this fix's shared-helper export touches).
- `pnpm exec vitest run components/task/ components/settings/` → 301 files / 2166 tests passed, 4 skipped.
- `pnpm run typecheck` → clean.
- `pnpm run i18n:check` → `2123 key(s) referenced, 2407 en entries, 0 orphans, pseudo in sync`.
- Manual re-screenshot of the live demo task's Todos panel after the fix: all 5 seeded items render with correct order/status/progress ("2/5 completed"), matching the chat transcript's "UPDATED TODOS (2/5)" summary.
