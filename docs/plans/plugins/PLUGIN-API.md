# Kandev Plugin API contract (native JS UI plugins — "option C")

This is the frozen interface every frontend + example task builds against. Do not
diverge without updating this file.

## Loading model

1. Backend boot payload gains `plugins: ActivePlugin[]` where
   `ActivePlugin = { id: string; name: string; bundleUrl: string; styleUrls?: string[];
   repositoryProviderIds?: string[] }`. `repositoryProviderIds` is JSON
   `repositoryProviderIds`, copied from manifest `repository_providers`. It is optional
   only for additive compatibility with older boot payloads; a present empty list means
   the plugin declared no repository providers.
   `bundleUrl` = `/api/plugins/{id}/bundle` — kandev serves this **directly from the
   extracted package directory** on local disk
   (`~/.kandev/plugins/<id>/<version>/ui/...`, per manifest `ui.bundle`). There is no
   reverse proxy and no live upstream request: the plugin subprocess does not need to
   be running to serve the UI bundle, since installation already extracted the file.
2. On SPA boot, the **plugin host** (`apps/web/lib/plugins/host.ts`) iterates
   `bootPayload.plugins`, injects any `styleUrls` as `<link>`, and dynamically
   `import(/* @vite-ignore */ bundleUrl)` each bundle as a native ES module. Before a
   bundle can initialize, the loader supplies its `repositoryProviderIds` to the
   registry (for example `setDeclaredRepositoryProviderIds(pluginId, ids)`). The scoped
   registry then rejects `registerRepositoryProvider` or `registerReviewProvider` IDs
   absent from that declared set. An absent field preserves older-host compatibility;
   it does not invent provider ownership.
3. Each bundle, when evaluated, calls the global:
   ```ts
   window.registerKandevPlugin(pluginId, {
     initialize(registry, host): void | Promise<void>,
     destroy?(): void,
   })
   ```
4. After the module resolves, the host calls `initialize(registry, host)`. Activation
   is transactional: failure or timeout unregisters partial contributions, aborts
   plugin-owned work, and fences late registrations from that expired attempt. On
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
    // Low-level request scoped to this plugin's host path. It MUST NOT target a
    // public webhook path or be used for authenticated provider commands.
    fetch(path: string, init?: RequestInit): Promise<Response>;
    // Authenticated action declared in this plugin's manifest. The host verifies the
    // indicated resource and passes it to the plugin separately from body JSON. This
    // is the only browser-to-plugin command path; never call a public webhook route.
    invokeAction<TResponse>(
      key: string,
      input?: { workspaceId?: string; taskId?: string; repositoryId?: string; body?: unknown },
      options?: { signal?: AbortSignal },
    ): Promise<TResponse>;
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
  // (mounted once inside AppShell's theme/tooltip/toast providers and isolated
  // behind its own error boundary).
  // Independent of keybindings — any plugin code path may call it.
  openModal(options: PluginModalOptions): PluginModalHandle;
  // Opens Kandev's native one-field task change-request linking workflow.
  // Provider code supplies copy, parsing, and mutation only; the host owns
  // validation placement, submitting state, footer, toast, and close behavior.
  openTaskLinkDialog(options: PluginTaskLinkDialogOptions): PluginModalHandle;
  // Opens a registered provider review in the native desktop dock panel or
  // current mobile session review. Plugins do not reach into host layout stores.
  openTaskReview(options: PluginTaskReviewOptions): void;
}

interface PluginModalOptions {
  title?: string;                          // rendered in a DialogHeader/DialogTitle; omit for no header title
  description?: string;                    // rendered below the title in the host-owned header
  content: React.ComponentType<{ slotProps?: unknown }>; // reuses the slot-component contract
  size?: "sm" | "md" | "lg" | "xl";         // maps to the host's Dialog width classes; default "md"
  dismissible?: boolean;                    // overlay click / Escape close the modal; default true
  presentation?: "dialog" | "drawer";       // default dialog; use drawer for native phone actions
}

interface PluginModalHandle {
  close(): void; // closes this modal instance; no-op if already closed
}

interface PluginTaskLinkDialogOptions {
  title: string;
  description: string;
  inputLabel: string;
  placeholder?: string;
  emptyError: string;
  failureMessage: string;
  successMessage: string;
  inputTestId?: string;
  errorTestId?: string;
  submitTestId?: string;
  onSubmit(reference: string): Promise<void>;
}

interface PluginTaskReviewOptions {
  providerId: string;
  reviewKey: string;
  title?: string;
  presentation: "desktop" | "mobile";
  sessionId?: string;
}
```

`host.ui` contents: shadcn primitives (Alert*, Badge, Button, Card*, Checkbox,
Dialog*, DropdownMenu*, Input, Label, Pagination*, ScrollArea, Select*,
Separator, Sheet*, Skeleton, Spinner, Switch, Table*, Tabs*, Textarea,
Tooltip*) plus first-party app UI: `PageTopbar` (the kandev title bar, for
routes that opt out of the default chrome and own their layout),
`TaskCreateDialog` (kandev's real create-task modal, prefilled via
`initialValues`), `Combobox` (the app's Command+Popover picker), and the
provider-neutral code-host dashboard set: `ChangeRequestList`,
`ChangeRequestRow`, `ChangeRequestDetail`, `IntegrationListToolbar`, `IntegrationScopeBar`,
`IntegrationStartTaskMenu`, `IntegrationIcon`, `IntegrationChangeRequestStatus`, and
`TaskRowIndicator`. The authoritative list is
`apps/web/lib/plugins/host-api.ts` (`PLUGIN_UI`).

In create mode, `TaskCreateDialog` accepts this optional transport seam:

```ts
createTask?: (
  payload: Parameters<typeof createTask>[0],
) => Promise<CreateTaskResponse>;
```

When omitted, the dialog uses the normal `/api/v1/tasks` REST client. A trusted
plugin wrapper may provide it to send the unchanged native payload through
`Host Tasks.Create`; the same callback handles both the initial submission and
the one allowed fresh-branch re-consent retry. Edit and session modes ignore it.
The browser callback must not manufacture a repository-provider descriptor. It sends
only native task choices plus an idempotency identifier to an authenticated plugin
action; the plugin resolves repository identity from its live provider connection.
Scope idempotency to one open dialog so retry does not create duplicates while a later
intentional launch for the same change request still can.

Code-host plugins use that dashboard set as one contract. The plugin supplies
normalized change-request data, filter state, task presets, and callbacks; the
host owns row density, external-title behavior, responsive task menus,
linked-task navigation, and loading/error/empty treatment. A row's sole workflow CTA
is the shared **Task** preset menu, whose selection opens `TaskCreateDialog`
directly. Review belongs to the registered task review-provider surface, not a
parallel dashboard button or plugin-specific launch modal. `IntegrationIcon` maps
semantic names (`pull-request`, `pull-request-closed`, `merged`, `filter`) to the same
host Tabler glyphs used by first-party code-host pages; plugins do not copy their SVG
paths. Runtime components stay in the Kandev host and are versioned with `host.ui`.
A task preset may provide a semantic `iconName`; the host maps `eye`, `message`, and
`tool` to the exact first-party **Review**, **Address feedback**, and **Fix CI** icons.
`ReviewItemSummary.taskStatus` is the normal code-host status integration. Once a
registered review provider publishes it, Kandev automatically mounts the exact shared
topbar button, composer CI chip, desktop hover popover, and mobile drawer; Kandev also
leases one provider refresh on mount/open and every 90 seconds. Plugins must not register
a visual slot or a second poller for these surfaces. `IntegrationChangeRequestStatus`
remains exposed for non-review-provider composition only.

`ChangeRequestDetail` is the exact provider-neutral detail component consumed by
Kandev's GitHub panel. A review provider supplies its normalized model and advertised
callbacks; Kandev owns header, branches, state/review badges, description, review/check/
comment sections, add-to-context controls, scrolling, loading/error states, and native
mobile sizing. Code-host plugins must use it instead of recreating the review page.
A future SDK package may contain types or pure helpers, but not duplicate React/Radix
runtime components.

Plugins must use these host instances — bundling copies of anything
Radix/portal/context-based would split React context across instances and
break refs/`asChild`. Pure-React libs (e.g. `@tabler/icons-react`) bundle
fine.

### Persisted repository branch action

A manifest-owned repository provider that participates in Kandev's native task branch
picker declares this standardized action:

```yaml
actions:
  - key: "repositories.branches"
    scope: "workspace"
    max_body_bytes: 16384
```

This action is invoked by the host backend, not the browser callback. Kandev resolves the
active plugin that owns the repository's persisted provider ID and supplies a verified
workspace context plus this snake-case body:

```json
{
  "repository": {
    "provider_id": "example-provider",
    "provider_host": "https://code.example.com",
    "provider_repository_id": "owner/repository",
    "owner_or_project": "owner",
    "name": "repository",
    "clone_url": "https://code.example.com/owner/repository.git",
    "default_branch": "main"
  }
}
```

Every field comes from the persisted workspace repository. The plugin returns
`{"branches":[{"name":"main","commit":"optional","is_default":true}]}`. Kandev
enforces the manifest request cap, a 15-second timeout, a 1 MiB response cap, at most
10,000 branches, non-empty names, and name deduplication. Missing ownership, an inactive
plugin, an undeclared/wrong-scope action, or malformed output fails closed. Providers
must not require browser-supplied repository authority on this path.

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

  // Native Settings > Integrations contribution. The host adds index/navigation
  // entries and wraps this component in the shared settings section.
  registerIntegrationSettings(registration: IntegrationSettingsRegistration): void;

  // Named slot injection. Host renders all components registered for a slot via
  // <PluginSlot name="..." slotProps={...}/>. Initial slots: "task-sidebar",
  // "settings-nav", "main-nav-footer", "chat-input-actions", "chat-top-bar",
  // "main-top-bar", "app-status-bar-left", "app-status-bar-right", and
  // "plugin-settings".
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

  // Requires manifest ownership of provider.id. One active plugin owns one provider;
  // unload revokes it and aborts in-flight provider work. inspectURL returns a complete
  // credential-free HTTPS descriptor—host does not parse plugin provider URLs.
  registerRepositoryProvider(provider: RepositoryProviderRegistration): void;

  // Native task-menu contribution. placement "link" renders in Link menus on desktop
  // and visible mobile action surfaces; host closes the menu before handler invocation.
  registerTaskAction(action: TaskActionRegistration): void;

  // Native desktop/mobile review integration. Use external-store callbacks, never a
  // plugin hook, so enable/disable does not alter host hook ordering.
  registerReviewProvider(provider: ReviewProviderRegistration): void;
}

interface IntegrationSettingsRegistration {
  id: string;
  label: string;
  description: string;
  icon?: string;
  Component: React.ComponentType<{ workspaceId?: string }>;
}

// Integration settings render at /settings/integrations/{id} and
// /settings/workspace/{workspaceId}/integrations/{id}. IDs are URL-safe, cannot
// shadow first-party integrations, and have one active owner; unload revokes them.

interface RepositoryProviderRegistration {
  id: string;
  label: string;
  icon?: string;
  listRepositories(context: { workspaceId: string; signal: AbortSignal }): Promise<unknown[]>;
  matchesURL(url: string): boolean;
  listBranches(context: { workspaceId: string; repository: unknown; signal: AbortSignal }): Promise<unknown[]>;
  inspectURL(context: { workspaceId: string; url: string; signal: AbortSignal }): Promise<RepositoryInspection | null>;
}
interface RepositoryInspection {
  providerId: string; providerHost: string; ownerOrProject: string;
  repositoryId: string; repositoryName: string; cloneUrl: string;
  defaultBranch?: string; baseBranch?: string; headBranch?: string;
  pullRequest?: { number: number; title: string };
}
interface TaskActionRegistration {
  id: string; label: string; icon?: string; placement: "link";
  group?: string; visible?(context: PluginTaskActionContext): boolean;
  singleTaskOnly?: boolean; run(context: PluginTaskActionContext): Promise<void>;
}
interface PluginTaskActionContext {
  workspaceId: string; taskId: string; repositories: readonly unknown[]; pathname: string;
  presentation: "desktop" | "mobile";
}
interface ReviewProviderRegistration {
  id: string; label: string; icon?: string; changeRequestNoun: string; order: number;
  getSnapshot(taskId: string): readonly ReviewItemSummary[];
  subscribe(taskId: string, listener: () => void): () => void;
  refresh(taskId: string, signal: AbortSignal): Promise<void>;
  ReviewPanel: React.ComponentType<PluginReviewPanelProps>;
  Selector?: React.ComponentType; EmptyState?: React.ComponentType;
}
interface ReviewItemSummary {
  providerId: string; reviewKey: string; title: string; url: string; repositoryId: string;
  state: string; statusBadge?: { label: string; tone?: string };
  taskStatus?: ReviewTaskStatus;
}
type ReviewTaskPipelineState = "success" | "failure" | "pending" | "neutral";
interface ReviewTaskStatus {
  number: number | string;
  state: "open" | "merged" | "closed" | "draft";
  pipelineState: ReviewTaskPipelineState;
  checks: readonly {
    id: string; label: string; state: ReviewTaskPipelineState;
    detail?: string; url?: string;
  }[];
  review?: {
    state: "approved" | "changes_requested" | "pending";
    approved: number; required?: number; requested?: number;
  };
  unresolvedComments?: number;
  loading?: boolean; error?: string; updatedAt?: number;
}
interface PluginReviewPanelProps {
  panelId: string; presentation: "desktop" | "mobile"; workspaceId: string;
  taskId: string; sessionId?: string; reviewKey: string;
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

## Registry internals (host side)

`apps/web/lib/plugins/registry.ts` holds a singleton `PluginRegistry` whose data
is reactive (a small zustand store or event emitter) so host React components
re-render when registrations change. Every registration records the owning
`pluginId` so the host can bulk-unregister on disable. Exposes read selectors:
`getRoutes()` (each entry carries `pluginId` + `options`), `getNavItems()`,
`getSettingsRoutes()`, `getSlotComponents(slot)`, `getWsHandlers(action)`, and
`getPluginName(pluginId)` (display name recorded by `forPlugin(id, name)`, used
for derived page-chrome titles).

Before `initialize`, the loader records `ActivePlugin.repositoryProviderIds` for the
plugin ID. A present declaration is an allowlist for provider/review registration; an
absent declaration is tolerated only for older payload compatibility. Disable/unload
removes this declaration with the plugin's registrations. Failed/timed-out activation
performs the same cleanup and an attempt token prevents late async registrations from
reappearing.

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
a broken plugin can't crash boot; failed/timed-out activation is rolled back; and
registrations are namespaced + bulk-revocable per plugin. No credentials are ever
displayed to the operator — installing a plugin (via
URL or upload) has nothing to copy or reveal, unlike the old register flow's one-time
API key/webhook secret. Sandboxing plugin JS (worker/realms) is explicit future work.

## Example plugin must (task-21)

Ship a bundle that on `initialize` registers: a nav item "Hello" → route
`/plugins/hello` rendering a native page (uses `host.jsx` + `host.ui`), a
`task-sidebar` slot component, and a WS handler for `task.created` that updates a
counter in its page via the plugin's own module state. No bundled React.
