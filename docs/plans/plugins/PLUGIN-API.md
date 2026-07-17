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
  };
  ui: Record<string, unknown>;              // curated @kandev/ui components (Button, Card, Badge...)
  theme: "light" | "dark";
}
```

## `registry: PluginRegistry`

```ts
interface NavItem { id: string; label: string; path: string; icon?: string; section?: "main" | "settings"; }

interface PluginRegistry {
  // Top-level SPA route, e.g. "/jira". Component rendered by the SPA route resolver
  // when window.location path === path (exact match; trailing segments via ":param" not
  // required for v1 — exact + startsWith("/plugins/{id}") allowed).
  registerRoute(path: string, Component: React.ComponentType): void;

  // Sidebar/main nav entry. Rendered by <PluginNavItems/> in the app sidebar.
  registerNavItem(item: NavItem): void;

  // Route under /settings/plugins/{id}/... rendered inside settings shell.
  registerSettingsRoute(path: string, Component: React.ComponentType): void;

  // Named slot injection. Host renders all components registered for a slot via
  // <PluginSlot name="..." props={...}/>. Initial slots: "task-sidebar",
  // "settings-nav", "main-nav-footer".
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
`getRoutes()`, `getNavItems()`, `getSettingsRoutes()`, `getSlotComponents(slot)`,
`getWsHandlers(action)`.

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
