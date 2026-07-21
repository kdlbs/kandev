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
4. After the module resolves, the host calls `initialize(registry, host)`. On
   plugin disable/uninstall the host calls `destroy?.()` and unregisters everything
   that plugin added (registrations are tracked per pluginId).

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
}
```

`host.ui` contents: shadcn primitives (Alert*, Badge, Button, Card*, Checkbox,
Dialog*, DropdownMenu*, Input, Label, Pagination*, ScrollArea, Select*,
Separator, Sheet*, Skeleton, Spinner, Switch, Table*, Tabs*, Textarea,
Tooltip*) plus first-party app UI: `PageTopbar` (the kandev title bar, for
routes that opt out of the default chrome and own their layout),
`TaskCreateDialog` (kandev's real create-task modal, prefilled via
`initialValues`), and `Combobox` (the app's Command+Popover picker). The
authoritative list is `apps/web/lib/plugins/host-api.ts` (`PLUGIN_UI`).

Plugins must use these host instances — bundling copies of anything
Radix/portal/context-based would split React context across instances and
break refs/`asChild`. Pure-React libs (e.g. `@tabler/icons-react`) bundle
fine.

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
  // with item.icon resolved against the curated icon map (fallback: puzzle).
  registerNavItem(item: NavItem): void;

  // Route under /settings/plugins/{id}/... rendered inside settings shell.
  // The settings shell already provides its own topbar chrome — no options here.
  registerSettingsRoute(path: string, Component: React.ComponentType): void;

  // Named slot injection. Host renders all components registered for a slot via
  // <PluginSlot name="..." props={...}/>. Initial slots: "task-sidebar",
  // "settings-nav", "main-nav-footer", "chat-input-actions", "chat-top-bar",
  // "plugin-settings".
  // "chat-input-actions" renders icon buttons in the chat composer toolbar
  // (beside the model picker, mic, and send) and forwards
  // `{ taskId, taskTitle, activeSessionId, sessionIds }` as `slotProps`.
  // "chat-top-bar" renders status in the session top bar (beside the CPU/DB
  // metrics and the document/editor/debug controls) and forwards
  // `{ taskId, taskTitle, workspaceId, activeSessionId, sessionIds }`. Both
  // carry the active session plus every kandev session id on the task.
  // Resolving a session id to an agent/ACP transcript id (e.g. to key
  // tokscale cost data on a session) is the plugin's job, done server-side in
  // the plugin backend via the Host data API; the host only propagates ids.
  // "plugin-settings" renders inline on a plugin's own settings page
  // (Settings > Plugins > <plugin>, between the schema-driven settings form and
  // the manifest card) and forwards `{ pluginId: string, status: PluginStatus }`
  // as `slotProps`. This slot renders on *every* plugin's detail page, so a
  // registered component MUST early-return null unless
  // `slotProps.pluginId === "<its own plugin id>"` — otherwise plugin A's card
  // would show on plugin B's settings page.
  registerComponent(slot: string, Component: React.ComponentType<{ slotProps?: unknown }>): void;

  // WS action handler. Bridged into the existing lib/ws dispatch; called with the
  // decoded message payload for that action string.
  registerWsHandler(action: string, handler: (payload: unknown) => void): void;
}
```

## Registry internals (host side)

`apps/web/lib/plugins/registry.ts` holds a singleton `PluginRegistry` whose data
is reactive (a small zustand store or event emitter) so host React components
re-render when registrations change. Every registration records the owning
`pluginId` so the host can bulk-unregister on disable. Exposes read selectors:
`getRoutes()` (each entry carries `pluginId` + `options`), `getNavItems()`,
`getSettingsRoutes()`, `getSlotComponents(slot)`, `getWsHandlers(action)`, and
`getPluginName(pluginId)` (display name recorded by `forPlugin(id, name)`, used
for derived page-chrome titles).

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
  `{ taskId, taskTitle, activeSessionId, sessionIds }`.

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
