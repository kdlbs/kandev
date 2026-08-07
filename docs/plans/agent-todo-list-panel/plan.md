---
spec: docs/specs/ui/agent-todo-list-panel.md
created: 2026-08-03
status: draft
---

# Implementation Plan: Agent Todo List Panel

## Overview

Add one new per-user boolean setting (`showTodoListPanel`) through the full
existing settings round trip (backend SQLite-backed model → DTO → service →
frontend SSR/WS mapping → Settings UI), then register a new reusable Dockview
panel (`todos`) that renders the agent's live todo checklist, gated at
runtime by that setting through a small conditional-sync hook modeled on the
existing PR Details pattern. Order: backend settings plumbing and frontend
settings plumbing first (independent of the panel and easiest to verify in
isolation), then the panel registration + rendering, then the visibility-sync
hook that wires the setting to the panel, then E2E coverage of the full flow.

---

## Backend

### Settings model, DTO, service, store

- `apps/backend/internal/user/models/models.go`: add `ShowTodoListPanel bool
  \`json:"show_todo_list_panel"\`` to the `UserSettings` struct beside
  `ShowTranscriptAutoScrollControl`.
- `apps/backend/internal/user/dto/dto.go`: add `ShowTodoListPanel bool
  \`json:"show_todo_list_panel"\`` to the response DTO, and `ShowTodoListPanel
  *bool \`json:"show_todo_list_panel,omitempty"\`` to the update-request DTO,
  mirroring the existing `ShowTranscriptAutoScrollControl` fields.
- `apps/backend/internal/user/service/service.go`: add the field to the
  update-request struct, the `if req.ShowTodoListPanel != nil { settings.ShowTodoListPanel = *req.ShowTodoListPanel }`
  merge-patch block, and the response map entry
  (`"show_todo_list_panel": settings.ShowTodoListPanel`), mirroring the three
  `ShowTranscriptAutoScrollControl` call sites (~lines 52-56, 286-292,
  689-693).
- `apps/backend/internal/user/store/sqlite.go`: add the field to the JSON
  column read map (~461-465), the fresh-user defaults (~582-586, `false`),
  the partial-update payload struct (~630-634), and the apply-if-present block
  (~692-696), mirroring `ShowTranscriptAutoScrollControl`.
- `apps/backend/internal/backendapp/boot_state_routes.go`: add
  `showTodoListPanel` to the boot/hydration payload beside
  `showTranscriptAutoScrollControl`/`showAnchoredPromptBar` (~459-463).

---

## Frontend

### Settings state and persistence plumbing

- `apps/web/lib/state/slices/settings/types.ts`: add `showTodoListPanel:
  boolean` to `UserSettingsState` beside `showTranscriptAutoScrollControl`
  (~line 191).
- `apps/web/lib/ssr/user-settings.ts`: add the default (`false`) to
  `createDefaultUserSettings()` (~line 45) and the snake_case↔camelCase
  mapping line `showTodoListPanel: s.show_todo_list_panel ?? current.showTodoListPanel`
  in the same mapping block as `showTranscriptAutoScrollControl` (~243-244).
- `apps/web/lib/ws/handlers/users.ts`: add the same field to the incoming
  user-settings WS-sync mapping, beside the existing boolean display-preference
  fields.
- `apps/web/lib/api/domains/settings-api.ts`: no new function — `updateUserSettings`
  already forwards an arbitrary snake_case payload object; the new settings
  component passes `{ show_todo_list_panel: value }` through the existing
  call.

### Settings UI

- New file `apps/web/components/settings/todo-list-panel-settings.tsx`:
  `TodoListPanelSettings` component, copied from the single-toggle
  `UnreadDividerSettings` template (`apps/web/components/settings/unread-divider-settings.tsx`):
  reads `useAppStore((s) => s.userSettings.showTodoListPanel)`, keeps a local
  draft/saved boolean, registers with `useSettingsSaveContributor` (id
  `general-todo-list-panel`), renders a `SettingsCard` with a `Label` +
  `Switch` (`@kandev/ui/switch`), and on save calls `updateUserSettings({
  show_todo_list_panel: submitted })` then `setUserSettings({ ...state,
  showTodoListPanel: submitted })`.
- `apps/web/components/settings/general-settings.tsx`: import
  `TodoListPanelSettings` and render it inside the existing
  `TaskActionsSettings` section's toggle list, beside `UnreadDividerSettings`
  (~line 255), since it is the same class of single-boolean display
  preference.
- i18n: add `todoListPanel` (label, e.g. "Show agent todo list panel") and a
  description key (e.g. `showAPersistentTodosTabInThe`) to
  `apps/web/src/locales/en/settings.json` beside the existing
  `showTranscriptAutoScrollControl` entries, and mirror both keys into
  `apps/web/src/locales/pseudo/settings.json` per the repo's pseudo-localization
  convention.

### Reusable Todos panel registration

- `apps/web/lib/state/layout-manager/constants.ts`: add `"todos"` to
  `REUSABLE_PANEL_IDS` and `KNOWN_PANEL_IDS`, and add a `PANEL_REGISTRY` entry
  `todos: { component: "todos", title: "Todos" }` (default tab component —
  `DockviewDefaultTab` via no explicit `tabComponent`, giving it a normal
  close control like Files/Changes).
- `apps/web/components/task/dockview-shared.tsx`:
  - Add `todos: PortalSlot` to `dockviewComponents`.
  - Add a new `TodosContent()` portal-content component (reads the active
    session's todo items and renders them) placed beside `PlanContent()`
    (~line 358), and a `case "todos": return <TodosContent panelId={panelId} />;`
    branch in `renderPanel` beside the `"plan"` case (~line 402-403).
  - `TodosContent` resolves the owning session id the same way
    `PlanContent`/`ChangesContent` resolve their task/session context (via
    `useEnvironmentSessionId`/store lookup used elsewhere in this file), then
    calls the shared checklist renderer described below.
- Extract the checklist body currently inlined in
  `apps/web/components/task/chat/todo-indicator.tsx`'s `TodoList` component
  (and its `resolveStatus`/`StatusIcon` helpers, already separately exported)
  into a shared, presentation-only component — e.g. rename/move `TodoList` to
  export it directly, or add a thin wrapper — so both `TodoIndicator`'s
  popover and the new `TodosContent` panel render identical rows without
  duplicating the status-icon/progress logic. `TodosContent` sources its
  `TodoDisplayItem[]` from the same hook `TodoIndicator`'s caller already
  uses (`useSessionTodoItems` in
  `apps/web/components/task/chat/use-chat-panel-state.ts:469-486`), called
  with the panel's owning session id instead of the chat panel's resolved
  session id, and an empty `messageTodos` fallback array when the panel has
  no local access to the chat's persisted-message data (matching the
  Files/Changes/Plan "show an empty state when there's no applicable content"
  convention from `docs/specs/ui/task-layout-profiles.md`).

### Visibility-sync hook

- New file `apps/web/components/task/dockview-todo-panel-sync.ts`, modeled
  directly on `apps/web/components/task/dockview-review-panel-sync.ts`, but
  substantially simpler: no review identity, no `reviewsLoaded` gating, no
  `wasOffered`/session-storage suppression.
  - `resolveConditionalTodoPanelAction({ showTodoListPanel, hasPanel }):
    "add" | "remove" | "none"` — pure decision function: `"add"` when the
    setting is on and the live layout lacks `todos`; `"remove"` when the
    setting is off and the live layout has it; `"none"` otherwise.
  - `resolveConfiguredTodoPanelPlacement(layout: LayoutState | null):
    { groupId: string; tabIndex: number } | null` — mirrors
    `resolveConfiguredReviewPanelPlacement`, scanning the custom Default
    layout for an existing `todos` entry's group/index.
  - `syncConditionalTodoPanel(api, options): void` — adds an inactive `todos`
    panel at the configured or Files/Changes-fallback group when action is
    `"add"`; removes the existing `todos` panel when action is `"remove"`.
  - `useSyncTodoPanel()` — reads `showTodoListPanel` from `useAppStore`, the
    active `taskId`/`sessionId`/`workspaceId`, and the same
    `hasApi`/`isRestoringLayout`/`centerGroupId`/`userDefaultLayout` Dockview
    store fields `useSyncReviewPanel` already reads, then calls
    `syncConditionalTodoPanel` inside the same double-`requestAnimationFrame`
    deferred-effect pattern `useSyncReviewPanel` uses, with `showTodoListPanel`
    added to the effect's dependency array in place of review `identity`.
- `apps/web/components/task/dockview-desktop-layout.tsx`: import and call
  `useSyncTodoPanel()` beside the existing `useSyncReviewPanel()` call
  (~line 411).

---

## Tests

- **What:** merge-patch persists and returns `show_todo_list_panel`.
  **File:** `apps/backend/internal/user/service/service_test.go`.
  **How:** table-driven case alongside the existing
  `ShowTranscriptAutoScrollControl` case, asserting a `PATCH`-equivalent
  service call with `ShowTodoListPanel: ptr(true)` updates and is reflected in
  the returned settings.
- **What:** SQLite store round-trips the new field for a fresh user and after
  a partial update. **File:** `apps/backend/internal/user/store/sqlite_test.go`.
  **How:** extend the existing default-settings assertion and the existing
  partial-update JSON-payload test (the one already covering
  `show_transcript_auto_scroll_control`) with `show_todo_list_panel`.
- **What:** SSR mapping default and snake_case→camelCase mapping.
  **File:** `apps/web/lib/ssr/user-settings.test.ts` (or the existing test
  file covering `mapUserSettingsData`/`buildBehaviorFields` if named
  differently — locate via the existing `showTranscriptAutoScrollControl`
  assertion and add a sibling case).
  **How:** unit test asserting `showTodoListPanel` defaults to `false` and
  maps from `show_todo_list_panel: true`.
- **What:** `TodoListPanelSettings` renders the current value, stages a draft,
  and calls `updateUserSettings({ show_todo_list_panel })` + `setUserSettings`
  on save. **File:** `apps/web/components/settings/todo-list-panel-settings.test.tsx`.
  **How:** component test mirroring `unread-divider-settings.test.tsx`'s
  structure (mock `updateUserSettings`, toggle the switch, trigger save via
  the settings save-provider, assert the call and store update).
- **What:** `resolveConditionalTodoPanelAction` returns `"add"`/`"remove"`/`"none"`
  for every combination of setting value and panel presence; the deferred
  effect never fires renders/looks up state before `hasApi`. **File:**
  `apps/web/components/task/dockview-todo-panel-sync.test.ts`.
  **How:** unit tests mirroring `dockview-review-panel-sync.test.ts`'s
  structure for the decision function and `syncConditionalTodoPanel`'s add/remove
  calls against a mocked `DockviewApi`.
- **What:** `todos` panel renders `TodoIndicator`-equivalent rows for a
  session with entries, and an empty state for a session without any.
  **File:** `apps/web/components/task/dockview-shared.test.tsx` (new, or added
  to an existing panel-content test file if one already covers
  `PlanContent`/`ChangesContent`).
  **How:** render `renderPanel("todos-panel-id", "todos", {})` inside the
  relevant store/portal test harness with seeded `sessionTodos` state, assert
  rendered rows/empty state.

---

## E2E Tests

- **Scenario:** GIVEN the preference is off, WHEN a task opens, THEN no Todos
  tab exists in the right panel. **File:**
  `apps/web/e2e/tests/settings/todo-list-panel.spec.ts`. **What to verify:**
  right-panel tab strip has no "Todos" tab.
- **Scenario:** GIVEN the preference is off, WHEN the user turns it on in
  `Settings > General` and saves, THEN the active task's right panel gains an
  inactive "Todos" tab beside Files and Changes without changing the
  currently selected tab. **File:** same spec file. **What to verify:** tab
  strip gains "Todos"; previously active tab (e.g. "Files") remains active;
  selecting "Todos" shows the checklist or its empty state.
- **Scenario:** GIVEN the preference is on, WHEN the user turns it off and
  saves, THEN the Todos tab disappears from the currently open task
  immediately, with no reload. **File:** same spec file. **What to verify:**
  tab strip no longer has "Todos" after save, without a page navigation.
- **Scenario:** GIVEN a custom Default layout with `todos` placed in a
  specific group via the Layout Editor, WHEN the preference is on and a fresh
  task opens, THEN the Todos tab appears in that configured group. **File:**
  `apps/web/e2e/tests/settings/layout-profiles.spec.ts` (extend with one case)
  or the new `todo-list-panel.spec.ts` file. **What to verify:** the Todos tab
  is in the user-configured group/position, not the Files/Changes fallback.

---

## Verification Results

All 5 tasks done. Full E2E suite for this feature
(`cd apps/web && pnpm exec playwright test e2e/tests/settings/todo-list-panel.spec.ts`)
passes (3/3), plus a joint run with `layout-profiles.spec.ts` to confirm no
regression in the two Dockview component registries this feature's fixes
touched (7/7). Re-ran backend
(`go test ./internal/user/... ./internal/backendapp/...`): every
`internal/user/*` package passes; `internal/backendapp` has the same 2
pre-existing failures Task 01 already documented
(`TestDetectBranchRemote_ReturnsConfiguredUpstream`,
`TestDetectBranchRemote_NoUpstreamFallsBackToOrigin`), a local GPG
commit-signing environment issue unrelated to any file this feature
touches. Frontend unit (`pnpm exec vitest run`, full `components/task/` +
`components/settings/` + settings/WS mapping tests), `pnpm run typecheck`,
and `pnpm run i18n:check` all clean. Full findings, including two product
bugs the E2E pass caught that unit tests could not (missing `todos` entries
in the desktop workbench's and Layout Editor's — as opposed to the Office
workbench's — separate Dockview component registries), are in Task 05's
Results section.

---

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01 - Backend settings field](task-01-backend-settings-field.md)
- [x] [Task 02 - Frontend settings plumbing and UI](task-02-frontend-settings-plumbing.md)

Wave 2 (depends on Wave 1):

- [x] [Task 03 - Todos panel registration and rendering](task-03-todos-panel-registration.md)

Wave 3 (depends on Wave 2):

- [x] [Task 04 - Visibility-sync hook](task-04-visibility-sync-hook.md)

Wave 4 (depends on Wave 3):

- [x] [Task 05 - E2E coverage](task-05-e2e-coverage.md)

Execution is sequential in the primary conversation by default. Tasks 01 and
02 touch disjoint backend/frontend files and are the only parallel-safe
candidates; a user may explicitly authorize running them together.

## Risks

- `useSyncReviewPanel`'s deferred double-`requestAnimationFrame` pattern
  exists to avoid racing Dockview's own layout-restoration timing; the new
  `useSyncTodoPanel` must reuse that exact pattern rather than approximating
  it, or the panel can flicker or be added before the restored layout settles.
- Extracting `TodoList`/`StatusIcon` from `todo-indicator.tsx` for reuse in
  the new panel must not change `TodoIndicator`'s existing rendered output —
  the chat status-bar chip has its own snapshot/visual expectations.
- The custom-Default placement lookup (`resolveConfiguredTodoPanelPlacement`)
  must scan the same `userDefaultLayout` shape `resolveConfiguredReviewPanelPlacement`
  already scans; a divergent lookup would silently place `todos` in the wrong
  group for users who already configured a placement via the Layout Editor
  before turning the preference on.

## Out of scope

- Everything listed in `docs/specs/ui/agent-todo-list-panel.md`'s Out of
  scope section (no backend todo-data changes, no removal of `TodoIndicator`/
  `TodoMessage`, no content-driven auto-show, no closed-for-session
  suppression, no unseen-update badge, no mobile/tablet layout changes, no
  per-task override of the preference).
