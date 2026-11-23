## Plugin system

The public frontend contract is `apps/packages/plugin-sdk`; `docs/plans/plugins/PLUGIN-API.md` and `lib/plugins/types.ts` are its detailed host implementation — all three must change together.
`lib/plugins/registry.ts` is the reactive singleton `PluginRegistry`; every `register*` call needs matching cleanup in `unregisterPlugin` and `totalCount()`, or a disabled/uninstalled plugin leaks a stale registration.

- **Task panels** (`registerTaskPanel`): one generic dockview component, `"plugin-panel"`, shared by
  every plugin — identity lives in `params: { pluginId, panelKey }` (id helpers in
  `lib/state/layout-manager/plugin-panels.ts`). `renderPanel` in `dockview-shared.tsx` and
  `dockview-panel-content.tsx` each get exactly one `"plugin-panel"` case (lookup tables, not
  switches). `PluginTaskPanel` (`components/task/`) resolves the registration behind a
  `PluginErrorBoundary`; `mobileEnabled: true` also renders it via the phone bottom nav
  (`session-mobile-bottom-nav.tsx`) with `presentation: "mobile"`. Reusable layout profiles (Settings > Layouts): Settings > Preferences > Layouts lists `getTaskPanels()` in Add panel (`layout-editor-toolbar.tsx`), resolved at selection time by `addReusablePanel`. The editor renders placeholders only, retains canonical plugin titles in an editor-owned map (`layout-editor-titles.ts`), and never mounts `useCloseRevokedPluginPanels`. `validateReusableLayout` checks the generic `plugin:<pluginId>:<panelKey>` shape without consulting the registry, so unavailable plugins stay valid portable placements. Plugin-bearing environment switches skip the fast path in `dockview-env-switch.ts`, and root width policy (pinned targets, sash width, caps) applies only to a HORIZONTAL root.
- **Task contributions:** `registerTaskMenuAction({ group: "edit", ... })` adds card-only actions to
  the `Edit` submenu. Group `"primary"` adds top-level actions to card and desktop/mobile task-row
  menus; declaring `items(context)` renders an action of either group as a submenu of its children
  instead, with `run` kept as the flat fallback.
  Card indicator/tag slots stay card-specific; `task-row-metadata` is generic for sidebar and `/tasks` rows.
- **Sidebar workspace actions:** `registerComponent("sidebar-workspace-actions", ...)` renders after Quick Terminal/Quick Chat in the desktop sidebar's New Task row and in the shared phone navigation sheet, forwarding `SidebarWorkspaceActionsSlotProps` with `presentation: "desktop" | "mobile"`; mobile plugin controls own a 44px touch target and accessible name.
- **`host.storage`:** authenticated per-user key/value storage (`lib/plugins/host-api.ts`) backed by `/api/plugins/{id}/user-state/...` (`docs/decisions/2026-08-01-per-user-plugin-storage.md`); `subscribe` (`lib/plugins/user-state-sync.ts`) wraps `registerWsHandler` with own-plugin filtering and own-tab echo suppression via a per-tab `writerId`.
- **`host.ui.RichTextEditor`/`RichTextReadOnly`** (`components/editors/tiptap/rich-text-editor.tsx`): narrow Plan-panel-tiptap wrappers; update `PLUGIN-API.md` before widening props beyond `{ taskId, value, onChange, placeholder, className, testId }` / `{ value, className, testId }`.
