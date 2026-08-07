# Kandev Plugin API contract (native JS UI plugins — "option C")

This is the frozen interface every frontend + example task builds against. Do not
diverge without updating this file.

## Loading model

1. Backend boot payload gains `plugins: ActivePlugin[]` where
   `ActivePlugin = { id: string; name: string; bundleUrl: string; styleUrls?: string[] }`.
   `bundleUrl` = `/api/plugins/{id}/bundle` — kandev serves this **directly from the
   extracted package directory** on local disk
   (`~/.kandev/plugins/<id>/<version>/ui/...`, per manifest `ui.bundle`). There is no
   reverse proxy and no live upstream request: the plugin subprocess does not need to
   be running to serve the UI bundle, since installation already extracted the file.
2. On SPA boot, the **plugin host** (`apps/web/lib/plugins/host.ts`) iterates
   `bootPayload.plugins`, injects any `styleUrls` as `<link>`, and dynamically
   `import(/* @vite-ignore */ bundleUrl)` each bundle as a native ES module.
3. Each bundle, when evaluated, calls the global:
   ```ts
   window.registerKandevPlugin(pluginId, {
     initialize(registry, host): void | Promise<void>,
     destroy?(): void,
   })
   ```
4. After the module resolves, the host calls `initialize(registry, host)`. A
   reload/update may unregister the previous generation before starting the next
   one; the host keeps that transition unresolved until the current generation's
   initialization finishes. Slow or failed reloads do not by themselves revoke
   open or saved task panels. On explicit plugin disable/uninstall the host calls
   `destroy?.()`, removes the plugin's registrations, and closes its panels.

## Global entry point

`window.registerKandevPlugin(id: string, plugin: KandevPlugin)` — defined by the
host before any bundle loads. Bundles are authored with React as an **external**;
they must use `host.React` (NOT bundle their own React) to share the host instance.

## `host: PluginHostApi`

```ts
interface PluginHostApi {
  pluginId: string;
  React: typeof import("react");            // host React instance (shared)
  jsx: typeof React.createElement;          // convenience alias (h)
  store: {                                   // kandev app store (zustand StoreApi)
    getState(): AppState;
    setState(partial): void;
    subscribe(listener): () => void;
  };
  api: {
    // fetch scoped to this plugin's backend via kandev proxy:
    // GET/POST {method} /api/plugins/{id}/... ; returns parsed JSON
    fetch(path: string, init?: RequestInit): Promise<Response>;
    // Backend API origin ("" when SPA and API share an origin) — for reaching
    // first-party kandev REST endpoints without re-deriving the split-origin
    // dev/desktop base URL from window internals.
    baseUrl: string;
  };
  ui: Record<string, unknown>;              // curated @kandev/ui components + app UI (see below)
  theme: "light" | "dark";
  // Soft SPA navigation (history push/replace + SPA re-render) — same code
  // path as the app router, so plugin pages can link into native routes
  // (e.g. /t/{taskId}) without a full reload.
  navigate(href: string, options?: { replace?: boolean }): void;
  // Imperatively opens a modal window rendered by the host's <PluginModalHost/>
  // (mounted once at the app root, isolated behind its own error boundary).
  // Independent of keybindings — any plugin code path may call it.
  openModal(options: PluginModalOptions): PluginModalHandle;
  // Authenticated, per-user key/value storage backed by
  // /api/plugins/{id}/user-state/... — see the "host.storage" section below.
  // Requires the plugin manifest to declare capabilities.user_state: true.
  storage: PluginStorageApi;
}

interface PluginStorageEntry { key: string; value: unknown; updatedAt: string; }

interface PluginStorageSetOptions {
  // Optimistic-concurrency guard: the updatedAt the caller last read. The
  // write is rejected (the returned promise rejects with a
  // PluginStorageConflictError) if the stored row was modified after this
  // time, leaving the stored value unchanged. Omit for unconditional
  // last-write-wins (the default).
  ifUnmodifiedSince?: string;
  // Identifies which logical surface made this write, for echo suppression
  // (see subscribe's writerId filter below). Appended to the host's own
  // per-tab id — not a replacement for it — so a static surface id like a
  // dockview panelId (shared by every tab that has that panel open) can't
  // make two different tabs look like the same writer to each other. Omit
  // to use the shared per-tab default alone — fine for a one-shot write with
  // no ongoing subscription (e.g. a kanban menu action). A surface that also
  // subscribes to its own writes (e.g. a task panel) should pass something
  // stable and unique to that surface, such as its own panelId from
  // PluginTaskPanelProps — otherwise its writes are indistinguishable from
  // any other surface of the same plugin in this tab, and one surface's
  // legitimate write can be silently swallowed by another surface's
  // subscription as if it were that other surface's own echo.
  writerId?: string;
}

// Mirrors the backend's user-state route scopes.
type PluginStorageScope = "instance" | "workspace" | "task" | "session" | "repository";

interface PluginStorageApi {
  get(scope: PluginStorageScope, scopeId: string, key: string): Promise<PluginStorageEntry | undefined>;
  set(
    scope: PluginStorageScope, scopeId: string, key: string, value: unknown,
    options?: PluginStorageSetOptions,
  ): Promise<{ updatedAt: string }>;
  delete(
    scope: PluginStorageScope, scopeId: string, key: string,
    options?: Pick<PluginStorageSetOptions, "writerId">,
  ): Promise<void>;
  // Every entry under (scope, scopeId), ordered by key. Not paginated.
  list(scope: PluginStorageScope, scopeId: string): Promise<PluginStorageEntry[]>;
  // Subscribes to live updates for this plugin's own storage made from
  // another tab, device, or surface — e.g. the kanban Edit modal and the
  // task panel both editing the same document. filter.scope/scopeId/key
  // narrow to a specific tuple; omit a field to match any value.
  //
  // filter.writerId, if given, must be the same value this surface passes to
  // set/delete's own writerId option — the host combines it with its own
  // per-tab id the same way on both sides, so a notification carrying that
  // resulting combined id is this surface's own echo and is skipped, and its
  // editor never clobbers its own caret/selection reacting to its own write.
  // Omit to fall back to the shared per-tab default alone: correct for a
  // plugin with only one surface, but two independent surfaces of the same
  // plugin (e.g. an open task panel and a kanban quick-action) both omitting
  // it would incorrectly suppress each other's legitimate writes as if they
  // were one surface's own echo.
  subscribe(
    filter: { scope?: PluginStorageScope; scopeId?: string; key?: string; writerId?: string },
    handler: (change: PluginUserStateChange) => void,
  ): () => void;
}

interface PluginUserStateChange {
  scope: PluginStorageScope;
  scopeId: string;
  key: string;
  updatedAt: string;
  deleted?: boolean; // true when the change was a delete rather than a set
}

interface PluginModalOptions {
  title?: string;                          // rendered in a DialogHeader/DialogTitle; omit for no header title
  content: React.ComponentType<{ slotProps?: unknown }>; // reuses the slot-component contract
  size?: "sm" | "md" | "lg" | "xl";         // maps to the host's Dialog width classes; default "md"
  dismissible?: boolean;                    // overlay click / Escape close the modal; default true
}

interface PluginModalHandle {
  close(): void; // closes this modal instance; no-op if already closed
}
```

`host.ui` contents: shadcn primitives (Alert*, Badge, Button, Card*, Checkbox,
Dialog*, DropdownMenu*, Input, Label, Pagination*, ScrollArea, Select*,
Separator, Sheet*, Skeleton, Spinner, Switch, Table*, Tabs*, Textarea,
Tooltip*) plus first-party app UI: `PageTopbar` (the kandev title bar, for
routes that opt out of the default chrome and own their layout),
`TaskCreateDialog` (kandev's real create-task modal, prefilled via
`initialValues`), `Combobox` (the app's Command+Popover picker), and
`RichTextEditor`/`RichTextReadOnly` (narrow wrappers over the Plan panel's
tiptap markdown editor — see below). The authoritative list is
`apps/web/lib/plugins/host-api.ts` (`PLUGIN_UI`).

Plugins must use these host instances — bundling copies of anything
Radix/portal/context-based would split React context across instances and
break refs/`asChild`. Pure-React libs (e.g. `@tabler/icons-react`) bundle
fine.

### `host.ui.RichTextEditor` / `host.ui.RichTextReadOnly`

Pixel-identical to the Plan panel's markdown editor (paste handling, slash
commands, drag handles, mermaid), so a plugin doesn't ship its own tiptap:

```ts
// RichTextEditor: editable, value/onChange round-trip markdown.
interface RichTextEditorProps {
  taskId: string; // required — scopes mermaid/image asset resolution
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  className?: string;
  testId?: string;
}
// RichTextReadOnly: renders markdown read-only, no taskId dependency.
interface RichTextReadOnlyProps { value: string; className?: string; testId?: string; }
```

Deliberately narrow — not the plan editor's `comments`, `onSelectionChange`,
`onCommentClick`, `onCommentDeleted`, or `onEditorReady` props, so the plan
editor's internals can keep evolving without breaking this contract.

### `host.storage` — authenticated per-user key/value storage

Backed by `PUT/GET/DELETE /api/plugins/{id}/user-state/{scope}/{scopeId}/{key}`
and `GET /api/plugins/{id}/user-state/{scope}/{scopeId}` (list). Every
read/write is scoped to the calling user via the session/PAT identity — two
users writing the same `(scope, scopeId, key)` never see each other's value;
a `GET` for another user's key returns `404`. Requires the plugin manifest to
declare `capabilities.user_state: true` (`403` otherwise); an unknown or
disabled plugin returns `404`. `scopeId`/`key` must match
`[A-Za-z0-9][A-Za-z0-9._:-]{0,127}`; the request body is capped (`413` over
the limit — see `apps/backend/internal/plugins/user_state_handlers.go`'s
`maxUserStateBodyBytes`). `PUT` accepts an optional `ifUnmodifiedSince`
(compared against the stored row's `updatedAt`) — a conflicting write returns
`409` and leaves the stored value unchanged.

This is entirely separate from the plugin-owned `plugin_state` table (written
only by a plugin's own gRPC-connected backend via the Host `SetState` RPC) —
`host.storage` needs no plugin backend at all, so a UI-only plugin bundle can
persist data with zero Go code.

`host.storage.set` stamps a per-browser-tab `writerId` on every write (one id
per page load, shared across every plugin in that tab). A successful
`PUT`/`DELETE` publishes the `plugin.user-state.updated` WS action to the
writing user's own connections only:

```ts
// WS message: { action: "plugin.user-state.updated", payload: PluginUserStateUpdatedPayload }
interface PluginUserStateUpdatedPayload {
  pluginId: string;
  scope: PluginStorageScope;
  scopeId: string;
  key: string;
  updatedAt: string;
  writerId?: string;
  deleted?: boolean;
}
```

The payload carries keys only, never the stored value — a subscriber refetches
via `host.storage.get`. `host.storage.subscribe(...)` is a typed convenience
wrapper over `registry.registerWsHandler("plugin.user-state.updated", ...)`
that already filters to this plugin's own events, applies your `scope`/
`scopeId`/`key` filter, and skips notifications whose `writerId` matches this
tab's own writes (so an editor never clobbers its own caret/selection from its
own write).

## `registry: PluginRegistry`

```ts
// icon: curated icon name (apps/web/lib/plugins/icons.ts — "ticket", "chart",
// "robot", "database", ...); unknown/missing names render a puzzle glyph in
// the sidebar.
// section: "main" (default) renders as a top-level sidebar entry;
// "integrations" renders inside the sidebar's Integrations section alongside
// the first-party integration links (GitHub, Jira, ...). Hosts predating a
// section value simply don't render items targeting it (additive change).
interface NavItem { id: string; label: string; path: string; icon?: string; section?: "main" | "settings" | "integrations"; }

// Configuration for the kandev-style title bar the host renders above a plugin
// route. All fields optional; defaults are derived (see registerRoute below).
interface PluginPageChrome {
  title?: string;      // default: nav-item label for the same path, else plugin name
  subtitle?: string;   // muted text next to the title
  icon?: string;       // curated icon name; default: matching nav item's icon
  backHref?: string;   // back-link target (host default "/")
  backLabel?: string;  // back-link label (host default "Kandev")
  actions?: React.ComponentType; // rendered on the right side of the topbar
}

interface PluginRouteOptions {
  // Default: enabled with derived title. Object → configure; false → render the
  // route full-bleed and own the chrome (e.g. with host.ui.PageTopbar).
  topbar?: boolean | PluginPageChrome;
}

interface PluginRegistry {
  // Top-level SPA route, e.g. "/jira". Component rendered by the SPA route resolver
  // when window.location path === path (exact match; trailing segments via ":param" not
  // required for v1 — exact + startsWith("/plugins/{id}") allowed). The host wraps the
  // page in kandev chrome (PageTopbar + scrollable content area) by default —
  // configure or opt out via options.topbar.
  registerRoute(path: string, Component: React.ComponentType, options?: PluginRouteOptions): void;

  // Sidebar/main nav entry. Rendered by <PluginNavItems/> in the app sidebar,
  // and by <MobilePluginNavSection/> in the phone menu sheet (the sidebar is
  // hidden below md), with item.icon resolved against the curated icon map
  // (fallback: puzzle).
  registerNavItem(item: NavItem): void;

  // Route under /settings/plugins/{id}/... rendered inside settings shell.
  // The settings shell already provides its own topbar chrome — no options here.
  registerSettingsRoute(path: string, Component: React.ComponentType): void;

  // Named slot injection. Host renders all components registered for a slot via
  // <PluginSlot name="..." slotProps={...}/>. Initial slots: "task-sidebar",
  // "settings-nav", "chat-input-actions", "chat-top-bar",
  // "main-top-bar", "app-status-bar-left", "app-status-bar-right",
  // "plugin-settings", "task-card-indicators", and "task-card-tags".
  // "task-card-indicators" renders a small icon/badge beside the PR status
  // icon on every kanban card and forwards
  // `{ taskId, workspaceId, workflowStepId }` as `slotProps`. Not a closed
  // union — hosts may register additional slot names.
  // "task-card-tags" renders in its own row on every kanban card, below the
  // badges row — for contributions too wide for the cramped title-row
  // "task-card-indicators" spot (e.g. a row of tag chips) — and forwards the
  // same `{ taskId: string, workspaceId: string | null, workflowStepId: string | null }`
  // shape as `slotProps` (`workspaceId` is null with no active workspace, and
  // `workflowStepId` is null when the task has no workflow step assigned).
  // "chat-input-actions" renders icon buttons in the chat composer toolbar
  // (beside the model picker, mic, and send) and forwards
  // `{ taskId, taskTitle, activeSessionId, sessionIds }` as `slotProps`.
  // "chat-top-bar" renders status in the session top bar (beside the
  // document/editor/debug controls) and forwards
  // `{ taskId, taskTitle, workspaceId, activeSessionId, sessionIds }`. Both
  // carry the active session plus every kandev session id on the task.
  // "main-top-bar" renders status/actions in the default app top bar on the
  // Home / Kanban / Tasks views (beside the CPU/DB metrics and the view/display
  // controls) and forwards `{ workspaceId, workspaceLabel, currentPage }`. It is
  // the app-wide, task-agnostic counterpart to "chat-top-bar", so it carries no
  // task/session ids.
  // Resolving a session id to an agent/ACP transcript id (e.g. to key
  // tokscale cost data on a session) is the plugin's job, done server-side in
  // the plugin backend via the Host data API; the host only propagates ids.
  // "plugin-settings" renders inline on a plugin's own settings page
  // (Settings > Plugins > <plugin>, at the top above the schema-driven settings
  // form) and forwards `{ pluginId: string, status: PluginStatus }`
  // as `slotProps`. It is owner-scoped: the host renders only the component
  // registered by the plugin currently being viewed, so your card appears on
  // your own settings page and never on another plugin's — no per-id gating
  // needed in your component.
  registerComponent(slot: string, Component: React.ComponentType<{ slotProps?: unknown }>): void;

  // WS action handler. Bridged into the existing lib/ws dispatch; called with the
  // decoded message payload for that action string.
  registerWsHandler(action: string, handler: (payload: unknown) => void): void;

  // Binds a handler to a keybinding declared in this plugin's manifest
  // (ui.keybindings[].id — { id, default, description }, see manifest schema).
  // The host resolves the effective combo (user override if the user
  // rebound it, else the manifest default) and dispatches globally, skipping
  // editable targets the same way core app shortcuts do. Combos are
  // user-overridable in Settings > Keyboard Shortcuts, namespaced
  // `plugin:{pluginId}:{id}`. Binding an id the manifest didn't declare still
  // stores the handler (a console warning is logged) since the dispatcher's
  // effective-shortcut resolution keys off the manifest list.
  //
  // Combo grammar (manifest `default` and any user override): `+`-separated
  // tokens, one of the modifiers `mod|ctrl|cmd|meta|alt|option|shift`
  // (repeatable) plus exactly one key token. `mod` resolves to Cmd on macOS
  // and Ctrl elsewhere (⌘/Ctrl). `shift` may not be combined with a digit or
  // symbol key (e.g. `shift+1`, `shift+slash`) — Shift changes the character
  // a browser reports for those keys, so the combo could never dispatch; both
  // the manifest validator and the frontend parser reject it.
  registerKeybinding(id: string, handler: (event: KeyboardEvent) => void): void;

  // Contributes a panel to the task workspace "+" (add panel) menu (dockview
  // desktop) and, when mobileEnabled, the phone bottom nav. Panel identity
  // lives in the dockview params (pluginId/panelKey); the panel id is
  // `plugin:{pluginId}:{panelKey}`. See "Task panels" below.
  registerTaskPanel(registration: TaskPanelRegistration): void;

  // Contributes an item to the kanban card's Edit submenu (group "edit") or
  // a flat, top-level card menu item between "Move to"/"Send to workflow"
  // and "Link" (group "primary"). See "Kanban card contributions" below.
  registerTaskMenuAction(registration: TaskMenuActionRegistration): void;

  // Contributes a client-side filter section to the kanban board's display
  // dropdown, alongside the built-in Workflow/Repository sections. See
  // "Task filters" below.
  registerTaskFilter(registration: TaskFilterRegistration): void;
}

type PluginPresentation = "desktop" | "mobile";

interface PluginTaskPanelProps {
  panelId: string; // this registration's panel id, so one Component can back multiple panels
  taskId: string;
  sessionId: string | null;
  presentation: PluginPresentation;
}

interface TaskPanelRegistration {
  id: string;          // plugin-local panel id (unique within the plugin, not globally)
  title: string;        // add-panel-menu row label and dockview tab title
  icon?: string;         // curated icon name (apps/web/lib/plugins/icons.ts)
  Component: React.ComponentType<PluginTaskPanelProps>; // wrapped in a PluginErrorBoundary
  mobileEnabled?: boolean; // include in the phone's grouped Panels picker. Default: false.
}

interface PluginTaskMenuContext {
  workspaceId: string;
  taskId: string;
  taskTitle: string;
  workflowStepId: string | null;
  presentation: PluginPresentation; // the actual kanban layout: desktop or mobile
}

interface TaskMenuActionRegistration {
  id: string;
  label: string;
  icon?: React.ReactNode;
  // "edit" nests the item in the card's Edit submenu; "primary" renders it
  // as a flat, top-level item between the "Move to"/"Send to workflow"
  // submenus and the "Link" submenu.
  group: "edit" | "primary";
  visible?(context: PluginTaskMenuContext): boolean; // default: always visible
  run(context: PluginTaskMenuContext): void | Promise<void>; // a rejection is caught and logged
}

interface PluginTaskFilterContext {
  taskId: string;
}

interface PluginTaskFilterOption {
  value: string;
  label: string;
  color?: string; // optional swatch color rendered beside the option label
}

interface TaskFilterRegistration {
  id: string;   // plugin-local filter id (unique within the plugin, not globally)
  label: string; // filter section label shown in the dropdown
  getOptions(): PluginTaskFilterOption[];
  // Called only when `selected` is non-empty — an empty selection is
  // implicit "All" and always matches without invoking this method.
  matches(context: PluginTaskFilterContext, selected: string[]): boolean;
}
```

### App-status-bar slots

`app-status-bar-left` and `app-status-bar-right` are live named component slots.
Each registration is one opaque status item; the slot chooses its default side,
not a permanent side after user customization. Components receive
`slotProps` with this exact shape:

```ts
interface AppStatusBarSlotProps {
  placement: "left" | "right";
  presentation: "bar" | "mobile-drawer";
  density: "full" | "compact";
  pathname: string;
  activeWorkspaceId: string | null;
  activeTaskId: string | null;
  activeSessionId: string | null;
}
```

`placement` matches registration slot. `presentation` identifies the mounted host;
the host mounts only one presentation at once. `density` is `full` on desktop and
phone drawer, `compact` on tablet. `pathname` and active IDs are current-context
hints, not entity payloads; read complete records from `host.store`.

Before customization, registration order is render order within each default side.
Users can Cmd-drag on macOS or Ctrl-drag elsewhere with a mouse to move any item
across the whole desktop/tablet bar. Kandev stores that order in backend user
settings; disabled contributions keep their place and return there when enabled.
Phone renders the saved left sequence followed by the saved right sequence, without
dragging. There is no cross-plugin priority API, keyboard-arrow ordering, or touch
ordering. Enable, disable, and uninstall update slots without reload. Each component
is isolated by an owner-aware error boundary, so plugins must tolerate remounting and
render a compact bar control or touch-usable drawer row for the supplied presentation.
The host neither inspects nor separately reorders children inside a registration, and
does not add a nested interactive wrapper.

A full-bleed plugin route (`topbar: false`) opts out of host chrome. It may mount
the host-provided Status drawer trigger when its own chrome should expose status;
otherwise status access is intentionally its responsibility.

### Task panels

`registerTaskPanel` adds one row to the task workspace's "+" (add panel)
menu, after "Plan". Selecting it opens a dockview panel using a single
generic `"plugin-panel"` dockview component shared by every plugin panel —
panel identity lives in `params: { pluginId, panelKey }`, and the panel id is
`plugin:{pluginId}:{panelKey}`. This keeps the host's panel-rendering dispatch
to one branch per host release rather than one per plugin, and lets a saved
layout round-trip a plugin panel reference even when that plugin is no
longer installed: the layout manager drops (not throws on) an unresolvable
plugin panel, and `Settings > Layouts` renders a generic placeholder box for
one it can't render live.

Disabling or uninstalling a plugin closes any of its open panels in the
current session and removes its add-panel-menu row; a panel your Component
was actively rendering unmounts in place (no console error, no dockview
exception). A plugin `Component` that throws during render shows a small
"failed to load" fallback inside just that panel — the panel error boundary
is scoped to your panel only, not the surrounding dockview layout.

On a phone viewport, `mobileEnabled: true` adds the panel to one grouped
**Panels** bottom-nav action (after Terminal); it does not add one navigation item
per panel. The touch-sized `MobilePickerSheet` presents every available panel in
an internally scrolling list. Selecting a row dismisses the picker and renders
your `Component` as the single full-height mobile surface with
`presentation: "mobile"` — the same `Component`, no separate mobile
registration. During a slow or failed reload, the host preserves a selected panel;
after a ready generation, a panel omitted by the new registration is closed. An
explicit disable or uninstall closes every panel owned by the plugin.

### Kanban card contributions

With no plugin registered for group `"edit"`, the kanban card's context/
dropdown menu shows the same flat `Edit` item as today. Once any plugin
registers a `registerTaskMenuAction({ group: "edit", ... })`, that item
becomes an `Edit` submenu: `Edit task` (the original action) first, then each
visible plugin action in registration order. An action whose `visible(context)`
returns `false` is filtered out entirely (not shown disabled). Selecting an
action calls `run(context)`; a rejected promise is caught and logged to the
console, and the menu still closes either way (Radix's own close-on-select,
independent of the async result).

Group `"primary"` renders each visible action as its own flat, top-level menu
item instead of nesting it under `Edit` — positioned between the "Move
to"/"Send to workflow" submenus and the "Link" submenu. Visibility filtering,
registration order, and `run()`/error handling are identical to the `"edit"`
group; the two groups are independent lists (an action only ever belongs to
one).

`"task-card-indicators"` (documented above with the other slots) is the
matching read-only surface: a small icon/badge rendered beside the PR status
icon on every card, receiving `{ taskId, workspaceId, workflowStepId }`.

`"task-card-tags"` is a second, sibling read-only surface for the same
context — same `{ taskId, workspaceId, workflowStepId }` shape — but mounted
in its own row on the card instead of the cramped title row
`"task-card-indicators"` shares with `PRTaskIcon`. Use it for a contribution
that needs its own width, e.g. a row of tag chips.

### Task filters

`registerTaskFilter` adds one section to the kanban board's display dropdown
(the same menu that holds the built-in Workflow and Repository filters),
rendered below Repository and above the Preview Panel section. Each
registration's `getOptions()` supplies the section's checkbox list; the
plugin owns option identity, ordering, and labels — including any
"untagged"-style sentinel, which is just a normal option value the host does
not special-case.

Selections are multi-select and purely client-side against tasks already
loaded in the board's in-memory state: there is no backend query, pagination,
or persistence for this filter, and selections reset on reload (unlike
Workflow/Repository, which persist to backend user settings). An empty
selection is implicit "All" for that section — `matches()` is only invoked
once at least one option is selected, and multiple plugin filter sections
combine with AND (a task must match every section with an active selection),
mirroring how Workflow/Repository combine with the search query today. If
`matches()` throws, the task is treated as non-matching and the error is
logged (mirroring `TaskMenuActionRegistration.visible`'s error handling).

## Registry internals (host side)

`apps/web/lib/plugins/registry.ts` holds a singleton `PluginRegistry` whose data
is reactive (a small zustand store or event emitter) so host React components
re-render when registrations change. Every registration records the owning
`pluginId` so the host can bulk-unregister on disable. Exposes read selectors:
`getRoutes()` (each entry carries `pluginId` + `options`), `getNavItems()`,
`getSettingsRoutes()`, `getSlotComponents(slot)`, `getWsHandlers(action)`,
`getPluginName(pluginId)` (display name recorded by `forPlugin(id, name)`, used
for derived page-chrome titles), `getTaskPanels()` / `getTaskPanel(pluginId, id)`,
and `getTaskMenuActions(group?)`. `unregisterWsHandler(pluginId, action, handler)`
removes exactly one WS handler (used by `host.storage.subscribe`'s returned
unsubscribe) without disturbing the plugin's other registrations.

Plugin top-level routes render inside `PluginPageFrame`
(`apps/web/components/plugins/plugin-page.tsx`): a `PageTopbar` title bar above
a scrollable content area, resolved from `options.topbar` with derived
defaults, or the bare component when the route opted out (`topbar: false`).

## Integration points the app must add (task-19)

- `src/spa-routes.tsx`: after the static route switch, before the not-found
  fallback, consult `registry.getRoutes()` for a matching path and render it inside
  the normal app shell.
- `src/settings-routes.tsx`: consult `registry.getSettingsRoutes()` for
  `/settings/plugins/{id}/*` paths.
- App sidebar (grep the nav list component): render `<PluginNavItems/>` reading
  `registry.getNavItems()`.
- `lib/ws/router.ts` / `lib/ws/client.ts`: after built-in dispatch, forward the
  message to any `registry.getWsHandlers(action)`.
- `components/plugins/plugin-slot.tsx`: `<PluginSlot name props/>` renders all
  slot components; drop into task detail sidebar + settings nav as initial hosts.
  The chat composer toolbar
  (`components/task/chat/chat-input-toolbar-desktop.tsx` and
  `-mobile.tsx`, via `chat-input-plugin-actions.tsx`) hosts the
  `chat-input-actions` slot, passing
  `{ taskId, taskTitle, activeSessionId, sessionIds }`. `kanban-card-plugin-slots.tsx`
  hosts `task-card-indicators` beside `PRTaskIcon` and `task-card-tags` as its
  own row, both mounted from `kanban-card-content.tsx`'s `KanbanCardBody`.
- `components/task/dockview-shared.tsx` / `dockview-panel-content.tsx` /
  `dockview-desktop-layout.tsx`: the generic `"plugin-panel"` dockview
  component + `pluginPanelTab`, and `renderPanel`'s `"plugin-panel"` case
  resolving `{ pluginId, panelKey }` via `registry.getTaskPanel(...)`.
  `dockview-add-panel-items.tsx` renders one "+" menu row per
  `registry.getTaskPanels()`. `use-close-revoked-plugin-panels.ts` closes an
  open panel whose registration disappeared.
- `components/task/mobile/session-mobile-bottom-nav.tsx` /
  `plugin-panel-picker.tsx` / `session-mobile-layout.tsx`: expose all
  `mobileEnabled` registrations through one grouped Panels picker, and reconcile
  the focused panel against host lifecycle state; `MobilePanelArea` renders the
  selected `plugin:{pluginId}:{panelKey}` panel.
- `components/kanban-card-edit-submenu.tsx`: builds the card's `Edit` entry
  from `registry.getTaskMenuActions("edit")`.
- `lib/state/layout-manager/plugin-panels.ts`: `pluginPanelId`/
  `parsePluginPanelId` identity helpers plus registry-aware
  `isKnownPanelId`/`isStructuralComponent`/`resolvePluginPanelDefinition`,
  consulted by the layout manager's persistence/merge logic.
- `lib/plugins/host-api.ts` / `user-state-sync.ts`: `host.storage`
  implementation (fetch against the user-state routes, per-tab `writerId`)
  and `host.storage.subscribe` (over `registerWsHandler`).
- `internal/gateway/websocket/user_notifications.go` (backend): subscribes
  the `plugin.user-state.updated` bus event into the existing
  `UserEventBroadcaster` (user-scoped fan-out, same path as
  `user.settings.updated`).

## Security posture (documented, enforced where cheap)

Plugin JS runs in the kandev origin with store access — this is the accepted
tradeoff of option C. v1 mitigations: only **active, operator-installed** plugins
load; bundles are served by kandev from the extracted package directory (same-origin,
no third-party CDN, no upstream network hop); host wraps `initialize` in try/catch so
a broken plugin can't crash boot; registrations namespaced + bulk-revocable per
plugin. No credentials are ever displayed to the operator — installing a plugin (via
URL or upload) has nothing to copy or reveal, unlike the old register flow's one-time
API key/webhook secret. Sandboxing plugin JS (worker/realms) is explicit future work.

## Example plugin must (task-21)

Ship a bundle that on `initialize` registers: a nav item "Hello" → route
`/plugins/hello` rendering a native page (uses `host.jsx` + `host.ui`), a
`task-sidebar` slot component, and a WS handler for `task.created` that updates a
counter in its page via the plugin's own module state. No bundled React.
