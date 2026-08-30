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
   outside that declared set. An omitted field preserves older-host compatibility;
   it does not invent provider ownership. Provider IDs are canonical lowercase
   identifiers: the manifest validator and frontend registry reject uppercase or
   otherwise non-canonical variants instead of creating case-dependent ownership.
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
   Each initialization attempt is transactional for plugin-owned runtime state:
   failure or timeout aborts plugin-owned work and fences callbacks from the
   expired generation. The same generation owns host-created subscriptions,
   modal and task-link handles, toasts, and review surfaces; the loader closes or
   unsubscribes them before calling `destroy` exactly once. Requests and callbacks
   from an expired generation cannot mutate the replacement generation. Failure or
   timeout does **not** unregister `registry` contributions (nav items, routes,
   etc.) already made before the failure — those persist, and only the plugin's
   lifecycle status becomes failed, until the plugin's _next_ load revokes them.

## Global entry point

`window.registerKandevPlugin(id: string, plugin: KandevPlugin)` — defined by the
host before any bundle loads. Bundles are authored with React as an **external**;
they must use `host.React` (NOT bundle their own React) to share the host instance.

The independently consumable frontend type contract is the runtime-free
`@kandev/plugin-sdk` package in `apps/packages/plugin-sdk`. Official plugins import
these types instead of re-declaring this document or importing `apps/web` internals.
The host has a compile-time assignability test, and the real Bitbucket package is a
required exact-head compatibility consumer.

## `host: PluginHostApi`

```ts
interface PluginHostApi {
  pluginId: string;
  React: typeof import("react"); // host React instance (shared)
  jsx: typeof React.createElement; // convenience alias (h)
  context: {
    // Versioned provider-neutral reads; never exposes private AppState slices.
    getActiveWorkspaceId(): string | undefined;
    subscribeActiveWorkspace(listener): () => void;
    getWorkspaceIds(): readonly string[];
    subscribeWorkspaces(listener): () => void;
    getTaskCreationContext(workspaceId: string): TaskCreationContext | null;
    subscribeTaskCreationContext(workspaceId: string, listener): () => void;
    resolveRepositoryId(identity: RepositoryIdentityInput): string | undefined;
  };
  // Compatibility only for older bundles. Deliberately absent from
  // @kandev/plugin-sdk; new/official plugins must use host.context.
  store: Pick<StoreApi<AppState>, "getState" | "setState" | "subscribe">;
  api: {
    // Low-level request scoped to this plugin's host path. It MUST NOT target a
    // public webhook path or be used for authenticated provider commands.
    fetch(path: string, init?: RequestInit): Promise<Response>;
    // Authenticated action declared in this plugin's manifest. The host verifies the
    // indicated resource and passes it to the plugin separately from body JSON. This
    // is the only browser-to-plugin command path; never call a public webhook route.
    invokeAction<TResponse>(
      key: string,
      input?: {
        workspaceId?: string;
        taskId?: string;
        sessionId?: string;
        repositoryId?: string;
        body?: unknown;
      },
      options?: { signal?: AbortSignal },
    ): Promise<TResponse>;
    // Backend API origin ("" when SPA and API share an origin) — for reaching
    // first-party kandev REST endpoints without re-deriving the split-origin
    // dev/desktop base URL from window internals.
    baseUrl: string;
  };
  ui: PluginUIApi; // named curated host components; no open Record index
  // Plugin-scoped locale and translation API. Components use the reactive
  // hook; registry getters may use the imperative translator.
  i18n: {
    readonly locale: string;
    t(
      key: string,
      options?: {
        defaultValue?: string;
        count?: number;
        values?: Readonly<Record<string, string | number>>;
      },
    ): string;
    useTranslation(): {
      readonly locale: string;
      t: PluginHostApi["i18n"]["t"];
    };
  };
  // The resolved light/dark theme, read live on every access. `host` is built
  // once per plugin load, so copying this into a variable that outlives a
  // render freezes it; read it during render, and pair it with onThemeChange
  // for anything that paints imperatively (canvas, inline SVG colors).
  readonly theme: "light" | "dark";
  // Fires on every light/dark change — the settings picker, its live preview,
  // and an OS prefers-color-scheme flip while the app is set to "system".
  // Returns an unsubscribe function; call it on teardown (component unmount,
  // KandevPlugin.destroy) or the listener outlives the surface that owns it.
  onThemeChange(listener: (theme: "light" | "dark") => void): () => void;
  // Soft SPA navigation (history push/replace + SPA re-render) — same code
  // path as the app router, so plugin pages can link into native routes
  // (e.g. /t/{taskId}) without a full reload.
  navigate(href: string, options?: { replace?: boolean }): void;
  // Imperatively opens a modal window rendered by the host's <PluginModalHost/>
  // (mounted once at the app root with its own tooltip provider and isolated
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
  // Sonner's imperative toast. The host mounts the single <Toaster/>, so
  // there is nothing to render and nothing to wire — and because it is
  // imperative rather than a component, it works from inside a plugin modal
  // regardless of which providers that modal sits under.
  toast: PluginToastApi;
  // Shared helpers — plain functions, so they live here rather than in `ui`
  // (a component map). See the "host.utils" section below.
  utils: PluginUtilsApi;
  // Registers one plugin-owned contributor with the native settings save bar.
  // The host prefixes the contributor id with `plugin:<pluginId>:` before it
  // reaches the shared coordinator, so ids only need to be unique inside the
  // plugin. The contributor owns persistence and draft state.
  useSettingsSaveContributor(contributor: SettingsSaveContributor): void;
  // Publishes one integration registration's live enabled state for one
  // workspace. The value is memory-only; persist it with host.storage and
  // republish it for every workspace after plugin load.
  setIntegrationEnabled(
    integrationId: string,
    workspaceId: string,
    enabled: boolean,
  ): void;
  // Authenticated, per-user key/value storage backed by
  // /api/plugins/{id}/user-state/... — see the "host.storage" section below.
  // Requires the plugin manifest to declare capabilities.user_state: true.
  storage: PluginStorageApi;
}

interface PluginStorageEntry {
  key: string;
  value: unknown;
  updatedAt: string;
}

interface PluginStorageSetOptions {
  // Abort the request when the owning surface or plugin generation ends.
  signal?: AbortSignal;
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
type PluginStorageScope =
  | "instance"
  | "workspace"
  | "task"
  | "session"
  | "repository";

interface PluginStorageApi {
  get(
    scope: PluginStorageScope,
    scopeId: string,
    key: string,
    options?: { signal?: AbortSignal },
  ): Promise<PluginStorageEntry | undefined>;
  set(
    scope: PluginStorageScope,
    scopeId: string,
    key: string,
    value: unknown,
    options?: PluginStorageSetOptions,
  ): Promise<{ updatedAt: string }>;
  delete(
    scope: PluginStorageScope,
    scopeId: string,
    key: string,
    options?: Pick<PluginStorageSetOptions, "writerId" | "signal">,
  ): Promise<void>;
  // Every entry under (scope, scopeId), ordered by key. Not paginated.
  list(
    scope: PluginStorageScope,
    scopeId: string,
    options?: { signal?: AbortSignal },
  ): Promise<PluginStorageEntry[]>;
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
    filter: {
      scope?: PluginStorageScope;
      scopeId?: string;
      key?: string;
      writerId?: string;
    },
    handler: (change: PluginUserStateChange) => void,
  ): () => void;
}

type SettingsSaveRevision = string | number;

interface SettingsSaveContributor {
  id: string;
  order?: number;
  revision: SettingsSaveRevision;
  isDirty: boolean;
  canSave?: boolean;
  invalidReason?: string;
  save(revision: SettingsSaveRevision): Promise<void> | void;
  discard(revision?: SettingsSaveRevision): Promise<void> | void;
}

interface PluginUserStateChange {
  scope: PluginStorageScope;
  scopeId: string;
  key: string;
  updatedAt: string;
  deleted?: boolean; // true when the change was a delete rather than a set
}

interface PluginModalOptions {
  title?: string; // rendered in a DialogHeader/DialogTitle; omit for no header title
  description?: string; // rendered below the title in the host-owned header
  content: React.ComponentType<{ slotProps?: unknown }>; // reuses the slot-component contract
  size?: "sm" | "md" | "lg" | "xl"; // maps to the host's Dialog width classes; default "md"
  dismissible?: boolean; // overlay click / Escape close the modal; default true
  presentation?: "dialog" | "drawer"; // default dialog; use drawer for native phone actions
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
  // The host aborts this signal when the user cancels or closes the dialog.
  onSubmit(reference: string, signal: AbortSignal): Promise<void>;
}

interface PluginTaskReviewOptions {
  providerId: string;
  reviewKey: string;
  title?: string;
  presentation: "desktop" | "mobile";
  sessionId?: string;
}
```

For `host.api.invokeAction`, workspace-scoped actions accept only `workspaceId`;
repository-scoped actions accept `workspaceId` plus `repositoryId`; task-scoped
actions require `taskId`, may include a matching `workspaceId`, and may include
`repositoryId` only when that persisted repository is attached to the verified task.
A task action may also include `sessionId`; the host verifies it belongs to that task.
When both session and repository selectors are present, the host resolves the exact
non-empty worktree branch and exposes it only in the backend verified action context.
Resource selectors never become part of the untrusted action body.

An action handler may return an HTTP status from 200 through 599; zero preserves the
legacy 200 response. The host forwards only safe response headers (`Content-Type`,
`Cache-Control`, `ETag`, and `Retry-After`). Invalid statuses become 502, while
transport/runtime errors become 503 without exposing internal error text. Plugins
should use explicit statuses only for safe domain outcomes that callers can act on.

`host.ui` contents: shadcn primitives (Accordion*, Alert*, Badge, Button,
Card*, Checkbox, Collapsible*, Dialog*, DropdownMenu*, Empty*, Input, Kbd,
KbdGroup, Label, Pagination*, Popover*, Progress, ScrollArea, Select*,
Separator, Sheet*, Skeleton, Spinner, Switch, Table*, Tabs*, Textarea,
Tooltip*, including `TooltipProvider`), the recharts wrappers (`ChartContainer`,
`ChartTooltip`, `ChartTooltipContent`, `ChartLegend`, `ChartLegendContent`,
`ChartStyle`), plus first-party app UI: `PageTopbar` (the kandev title bar, for
routes that opt out of the default chrome and own their layout),
`TaskCreateDialog` (kandev's real create-task modal, prefilled via
`initialValues`), `Combobox` (the app's Command+Popover picker), and the
provider-neutral code-host dashboard set: `ChangeRequestList`,
`ChangeRequestRow`, `ChangeRequestDetail`, `IntegrationListToolbar`, `IntegrationScopeBar`,
`IntegrationSaveQueryDialog`, `IntegrationRepositoryFilter`, `IntegrationCursorPagination`,
`IntegrationStartTaskMenu`, `IntegrationIcon`, `IntegrationChangeRequestStatus`, and
`TaskRowIndicator`, plus native integration settings surfaces:
`IntegrationAuthStatusBanner`, `IntegrationEnabledControl`, `SettingsSection`,
`SettingsCard`, and `WorkspaceScopedSection`. The authoritative list is
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
topbar button, composer CI chip, desktop hover popover, and mobile drawer. Each rendered
linked-task row acquires one deduplicated provider refresh so its indicator color is live
before hover; hover/focus can refresh again, while active topbar/composer status refreshes
every 90 seconds. Plugins must not register
a visual slot or a second poller for these surfaces. Linked-task indicators also derive
the same semantic color hierarchy from normalized state, review, and pipeline fields;
providers do not send CSS classes or provider-specific color tokens. While the initial
task detail request is pending, the indicator remains muted; publishing the snapshot
updates both its summary and color. `IntegrationChangeRequestStatus` remains exposed for non-review-
provider composition only.

`ChangeRequestDetail` is the exact provider-neutral detail component consumed by
Kandev's GitHub panel. A review provider supplies its normalized model and advertised
callbacks; Kandev owns header, branches, state/review badges, description, review/check/
comment sections, add-to-context controls, scrolling, loading/error states, and native
mobile sizing. Code-host plugins must use it instead of recreating the review page.
A future SDK package may contain types or pure helpers, but not duplicate React/Radix
runtime components.
The curated surface also includes `RichTextEditor`/`RichTextReadOnly`, narrow
wrappers over the Plan panel's tiptap markdown editor (see below).

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
    "provider_scope": "https://code.example.com/context-a",
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

The host wraps plugin routes, slots, and `openModal` content in a
`TooltipProvider`, so a plain `Tooltip` works anywhere without one.
`TooltipProvider` is exported for plugins that want their own
`delayDuration`/`skipDelayDuration` over a dense cluster of tooltips; Radix
supports nesting it.

**Charts use the host's recharts.** `recharts` is already a dependency of both
`apps/web` and `@kandev/ui`, so the `Chart*` exports add no bundle weight — and
a plugin must never bundle its own copy. recharts drives layout through its own
React context and portals its tooltips, so a second copy splits that context
exactly the way a second Radix copy does: charts render at zero size, tooltips
attach to the wrong tree, and responsive containers stop resizing. Compose the
`Chart*` wrappers with the chart primitives they hand you rather than importing
`recharts` in plugin code.

### `host.toast` and `host.utils`

```ts
// Sonner's imperative toast — the host owns the single <Toaster/>, so there
// is nothing to render and it works from any plugin code path, modals
// included.
host.toast.success("Synced 12 issues");
host.toast.error("Sync failed");

interface PluginToastApi {
  (message: string, options?: Record<string, unknown>): string | number;
  success(message: string, options?: Record<string, unknown>): string | number;
  // Renders the same toast as any other variant, and additionally logs
  // `[plugins] toast.error from "<pluginId>":` to the browser console.
  // It does NOT file a report into kandev's frontend error log — that log is
  // for kandev's own application errors, and a plugin toasting an expected
  // condition (a failed poll, say) would otherwise record an Error-level
  // entry every cycle. Console is where every plugin failure surfaces.
  error(message: string, options?: Record<string, unknown>): string | number;
  warning(message: string, options?: Record<string, unknown>): string | number;
  info(message: string, options?: Record<string, unknown>): string | number;
  // Dismisses one toast by the id a variant returned, or all of them when
  // called with no argument.
  dismiss(id?: string | number): unknown;
}

interface PluginUtilsApi {
  // The host's clsx + tailwind-merge combiner, so class merging matches the
  // components it styles.
  cn(...inputs: unknown[]): string;
  // Non-security UUID with a fallback for supported insecure HTTP origins.
  generateUUID(): string;
  // Locale-aware relative time ("3 hours ago", "in 2 days", "yesterday") via
  // Intl.RelativeTimeFormat in the user's active locale; "" for unparseable
  // input. Prefer it over a hand-rolled ladder, which is English-only by
  // construction and goes untranslated for every non-English user.
  formatRelativeTime(value: string | number | Date): string;
  // Polling interval used by native integration health controls.
  integrationStatusRefreshMs: number;
}
```

These are functions, so they sit beside `navigate`/`openModal` rather than in
`ui`, which is a component map.

### Native integration settings state

`registerIntegrationSettings` may provide an `action` component. The host mounts
the action in the detail `SettingsSection` header and in the native integrations
index card. The host passes the routed `workspaceId` and a `surface` value of
`"detail"` or `"index"`. Use the workspace value for workspace-scoped reads and
writes. Do not use `getActiveWorkspaceId()` for a routed settings page.

`host.setIntegrationEnabled(integrationId, workspaceId, enabled)` publishes a
live value for one registration and workspace. The host checks that the
registration belongs to the current plugin. The value is not durable and is
cleared when the plugin unloads. Persist the source of truth with
`host.storage`, then republish it after load:

```ts
function publishAll(
  host: PluginHostApi,
  integrationId: string,
  enabledByWorkspace: Map<string, boolean>,
) {
  for (const workspaceId of host.context.getWorkspaceIds()) {
    host.setIntegrationEnabled(
      integrationId,
      workspaceId,
      enabledByWorkspace.get(workspaceId) === true,
    );
  }
}

const unsubscribe = host.context.subscribeWorkspaces((workspaceIds) => {
  // Load or refresh the durable value for the changed workspace ids.
});
```

`host.useSettingsSaveContributor` connects plugin drafts to the native save
bar. Contributor ids are local to the plugin because the host adds the
`plugin:<pluginId>:` namespace before registration. Save and discard remain
plugin-owned.

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
interface RichTextReadOnlyProps {
  value: string;
  className?: string;
  testId?: string;
}
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
// "robot", "database", ...) or a plugin-owned component rendered with host
// React. Unknown/missing names render a puzzle glyph in the sidebar.
// section: "main" (default) renders as a top-level sidebar entry;
// "integrations" renders inside the sidebar's Integrations section alongside
// the first-party integration links (GitHub, Jira, ...); "sidebar-footer"
// renders as an icon button in the sidebar footer's icon row and as a
// labelled row in the phone menu's Utilities group, subject to the footer's
// inline budget — an over-budget item is reached through the footer's
// overflow menu instead of an inline button; "settings" is accepted but
// renders on no surface. Hosts predating a section value, or seeing an
// unrecognised one, simply degrade to "main"'s placement — nothing is ever
// silently dropped.
type PluginIcon = string | React.ComponentType<{ className?: string }>;
export type PluginNavSection =
  | "main"
  | "settings"
  | "integrations"
  | "sidebar-footer";

interface NavItem {
  id: string;
  label: string;
  path: string;
  icon?: PluginIcon;
  section?: PluginNavSection;
}

// Configuration for the kandev-style title bar the host renders above a plugin
// route. All fields optional; defaults are derived (see registerRoute below).
interface PluginPageChrome {
  title?: string; // default: nav-item label for the same path, else plugin name
  subtitle?: string; // muted text next to the title
  icon?: PluginIcon; // default: matching nav item's icon
  backHref?: string; // back-link target (host default "/")
  backLabel?: string; // back-link label (host default "Kandev")
  actions?: React.ComponentType; // rendered on the right side of the topbar
}

interface PluginRouteOptions {
  // Default: enabled with derived title. Object → configure; false → render the
  // route full-bleed and own the chrome (e.g. with host.ui.PageTopbar).
  topbar?: boolean | PluginPageChrome;
}

interface PluginRegistry {
  // Installs flat locale catalogs under this plugin's isolated namespace.
  // English is required as the fallback. Registration is atomic; unload
  // removes all catalog bundles and locale changes invalidate host consumers.
  registerTranslations(
    catalogs: Readonly<Record<string, Readonly<Record<string, string>>>>,
  ): void;

  // Top-level SPA route, e.g. "/jira". Component rendered by the SPA route resolver
  // when window.location path === path (exact match; trailing segments via ":param" not
  // required for v1 — exact + startsWith("/plugins/{id}") allowed). The host wraps the
  // page in kandev chrome (PageTopbar + scrollable content area) by default —
  // configure or opt out via options.topbar.
  registerRoute(
    path: string,
    Component: React.ComponentType,
    options?: PluginRouteOptions,
  ): void;

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
  registerIntegrationSettings(
    registration: IntegrationSettingsRegistration,
  ): void;

  // Named slot injection. Host renders all components registered for a slot via
  // <PluginSlot name="..." slotProps={...}/>. Initial slots: "task-sidebar",
  // "settings-nav", "chat-input-actions", "task-create-input-actions",
  // "new-session-input-actions", "chat-top-bar",
  // "main-top-bar", "app-status-bar-left", "app-status-bar-right",
  // "plugin-settings", "task-card-indicators", "task-card-tags",
  // "task-row-metadata", and
  // "sidebar-workspace-actions".
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
  // "task-row-metadata" renders compact, read-only metadata on the sidebar
  // task tree and `/tasks` list. It forwards
  // `{ taskId, workspaceId, workflowStepId, surface }`, where `surface` is
  // "sidebar" or "task-list". The slot is plugin-agnostic.
  // "chat-input-actions", "task-create-input-actions", and
  // "new-session-input-actions" render composer actions for task/Quick Chat,
  // task creation, and new-session creation. Each forwards the typed
  // `PluginComposerSlotProps`, including native insert/focus/submit capabilities.
  // "chat-top-bar" renders status in the session top bar (beside the
  // document/editor/debug controls) and forwards
  // `{ taskId, taskTitle, workspaceId, activeSessionId, sessionIds }`. Both
  // carry the active session plus every kandev session id on the task.
  // "main-top-bar" renders status/actions in the default app top bar on the
  // Home / Kanban / Tasks views (beside the CPU/DB metrics and the view/display
  // controls) and forwards `{ workspaceId, workspaceLabel, currentPage,
  // presentation }`, where presentation is "desktop" or "mobile". On a phone,
  // contributions join the horizontally scrollable middle action strip between
  // the fixed Kandev link and menu button. Documented host ui.Button icon
  // contributions are normalized to a 32px box with a 16px SVG icon on phones;
  // desktop contribution sizing is unchanged. It is the app-wide,
  // task-agnostic counterpart to "chat-top-bar", so it carries no task/session
  // ids.
  // "sidebar-workspace-actions" renders icon buttons after the built-in Quick
  // Terminal and Quick Chat actions in the desktop sidebar's New Task row and
  // in the shared phone navigation sheet. It forwards
  // `SidebarWorkspaceActionsSlotProps` as `slotProps`, with `presentation` set
  // to "desktop" or "mobile". The mobile presentation must use a touch target
  // of at least 44px in its active dimension.
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
  registerComponent(
    slot: string,
    Component: React.ComponentType<{ slotProps?: unknown }>,
  ): void;

  // WS action handler. Bridged into the existing lib/ws dispatch; called with the
  // decoded message payload for that action string.
  registerWsHandler(action: string, handler: (payload: unknown) => void): void;

  // Binds a handler to a keybinding declared in this plugin's manifest
  // (ui.keybindings[].id — { id, default, description, allow_in_editor? },
  // see manifest schema).
  // The host resolves the effective combo (user override if the user
  // rebound it, else the manifest default) and dispatches globally, skipping
  // editable targets the same way core app shortcuts do — unless that entry
  // declared `allow_in_editor: true`, which lets it fire while an input,
  // textarea or contenteditable has focus. That opt-in exists for bindings
  // whose whole job is to act on the focused composer (dictation); the
  // manifest validator only accepts it on a combo carrying a
  // ctrl/cmd/mod/alt modifier, and the dispatcher re-checks the *resolved*
  // combo, so a user override that drops the modifier silently falls back to
  // skipping editables rather than swallowing ordinary keystrokes. Combos are
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

  // Contributes a panel to the task workspace "+" menu and, when enabled,
  // the phone Panels picker.
  registerTaskPanel(registration: TaskPanelRegistration): void;

  // Contributes an item to the kanban card's Edit submenu (group "edit") or
  // a flat, top-level card menu item between "Move to"/"Send to workflow"
  // and "Link" (group "primary"). See "Kanban card contributions" below.
  registerTaskMenuAction(registration: TaskMenuActionRegistration): void;

  // Contributes a client-side filter section to the kanban board's display
  // dropdown, alongside the built-in Workflow/Repository sections. See
  // "Task filters" below.
  registerTaskFilter(registration: TaskFilterRegistration): void;
  registerTaskListFacet(registration: TaskListFacetRegistration): void;
}

interface IntegrationSettingsRegistration {
  id: string;
  label: string;
  description: string;
  icon?: PluginIcon;
  Component: React.ComponentType<{ workspaceId?: string }>;
  // Optional action for the detail section header and integrations index card.
  // The surface identifies the host location and the workspace id identifies
  // the settings target.
  action?: React.ComponentType<IntegrationSettingsActionProps>;
}

type IntegrationSettingsActionSurface = "detail" | "index";

interface IntegrationSettingsActionProps {
  workspaceId?: string;
  surface: IntegrationSettingsActionSurface;
}

// Integration settings render at /settings/integrations/{id} and
// /settings/workspaces/{workspaceId}/integrations/{id}. IDs are URL-safe, cannot
// shadow first-party integrations, and have one active owner; unload revokes them.

interface RepositoryProviderRegistration {
  id: string;
  label: string;
  icon?: PluginIcon;
  listRepositories(context: {
    workspaceId: string;
    /** Optional server-side search text. */
    query?: string;
    /** Opaque cursor returned by this provider for the same query. */
    cursor?: string;
    /** Requested page size; a provider may return fewer items. */
    limit?: number;
    signal: AbortSignal;
  }): Promise<RepositoryInspection[] | RepositoryProviderPage>;
  // Optional synchronous performance hint. It never establishes ownership.
  matchesURL?(url: string): boolean;
  listBranches(context: {
    workspaceId: string;
    repository: RepositoryInspection;
    signal: AbortSignal;
  }): Promise<RepositoryProviderBranch[]>;
  inspectURL(context: {
    workspaceId: string;
    url: string;
    signal: AbortSignal;
  }): Promise<RepositoryInspection | null>;
  supportsDraft?: boolean; // default true; false hides the native draft option
  // Kandev pushes the verified checkout branch before invoking this callback.
  createChangeRequest?(context: {
    workspaceId: string;
    taskId: string;
    sessionId: string; // session whose checkout Kandev pushed
    repositoryId: string;
    repository: PluginHostRepository; // persisted host repository, not browser authority
    title: string;
    body: string;
    baseBranch?: string;
    draft: boolean;
    signal: AbortSignal;
  }): Promise<RepositoryChangeRequestCreateResult>;
}

interface RepositoryProviderPage {
  repositories: RepositoryInspection[];
  nextCursor?: string;
}

// The host binds every returned RepositoryInspection.providerId to the
// registration's manifest-owned id; result data cannot spoof another namespace.
// Legacy arrays remain valid. The host follows page cursors and rejects a cursor
// repeated by a broken provider instead of looping forever.
interface PluginHostRepository {
  id: string;
  workspace_id: string;
  name: string;
  provider: string;
  source_type?: string;
  provider_repo_id?: string;
  provider_host?: string;
  provider_scope?: string;
  provider_owner?: string;
  provider_name?: string;
  remote_url?: string;
  default_branch?: string;
}
interface RepositoryProviderBranch {
  name: string;
}
interface RepositoryChangeRequestCreateResult {
  url: string;
  provider?: string;
  output?: string;
  // False means remote creation succeeded but task association did not.
  linked?: boolean;
  // Safe detail shown with recovery guidance. Never retry remote creation.
  associationError?: string;
}
interface RepositoryInspection {
  providerId: string;
  providerHost: string;
  // Opaque, credential-free connection identity. Required when one authority
  // can host multiple independent provider roots.
  providerScope?: string;
  ownerOrProject: string;
  repositoryId: string;
  repositoryName: string;
  cloneUrl: string;
  defaultBranch?: string;
  baseBranch?: string;
  headBranch?: string;
  pullRequest?: { number: number; title: string };
}
interface TaskActionRegistration {
  id: string;
  label: string;
  icon?: PluginIcon;
  placement: "link";
  group?: string;
  visible?(context: PluginTaskActionContext): boolean;
  singleTaskOnly?: boolean;
  run(context: PluginTaskActionContext): Promise<void>;
}
interface PluginTaskActionContext {
  workspaceId: string;
  taskId: string;
  repositories: readonly PluginHostRepository[];
  pathname: string;
  presentation: "desktop" | "mobile";
}
interface ReviewProviderRegistration {
  id: string;
  label: string;
  icon?: PluginIcon;
  changeRequestNoun: string;
  order: number;
  getSnapshot(taskId: string): readonly ReviewItemSummary[];
  subscribe(taskId: string, listener: () => void): () => void;
  refresh(taskId: string, signal: AbortSignal): Promise<void>;
  // Implement all three association callbacks together. The host performs one
  // workspace-bounded association refresh and renders native task-list/card
  // indicators. Each rendered linked task leases refresh(taskId), deduplicated
  // with topbar/panel consumers; hover/focus may refresh it again.
  getAssociationSnapshot?(
    workspaceId: string,
  ): readonly ReviewTaskAssociation[];
  subscribeAssociations?(workspaceId: string, listener: () => void): () => void;
  refreshAssociations?(workspaceId: string, signal: AbortSignal): Promise<void>;
  // Removes only this task link; it never deletes the remote change request.
  unlink?(context: {
    workspaceId: string;
    taskId: string;
    reviewKey: string;
    signal: AbortSignal;
  }): Promise<void>;
  ReviewPanel: React.ComponentType<PluginReviewPanelProps>;
  Selector?: React.ComponentType;
  EmptyState?: React.ComponentType;
}
interface ReviewTaskAssociation {
  providerId: string;
  taskId: string;
  reviewKey: string;
  connectionScope: string;
  repositoryId: string;
  changeRequestNumber: string | number;
}
interface ReviewItemSummary {
  providerId: string;
  reviewKey: string;
  title: string;
  url: string;
  connectionScope: string;
  repositoryId: string;
  changeRequestNumber: string | number;
  // open/opened/draft participates in native Create PR eligibility.
  state: string;
  statusBadge?: { label: string; tone?: string };
  taskStatus?: ReviewTaskStatus;
}
type ReviewTaskPipelineState = "success" | "failure" | "pending" | "neutral";
interface ReviewTaskStatus {
  number: number | string;
  state: "open" | "merged" | "closed" | "draft";
  pipelineState: ReviewTaskPipelineState;
  checks: readonly {
    id: string;
    label: string;
    state: ReviewTaskPipelineState;
    detail?: string;
    url?: string;
  }[];
  review?: {
    state: "approved" | "changes_requested" | "pending";
    approved: number;
    required?: number;
    requested?: number;
  };
  unresolvedComments?: number;
  loading?: boolean;
  error?: string;
  updatedAt?: number;
}
interface PluginReviewPanelProps {
  panelId: string;
  presentation: "desktop" | "mobile";
  workspaceId: string;
  taskId: string;
  sessionId?: string;
  reviewKey: string;
  connectionScope: string;
  repositoryId: string;
  changeRequestNumber: string | number;
}

type PluginPresentation = "desktop" | "mobile";

type PluginComposerSurface =
  | "task-chat"
  | "quick-chat"
  | "task-create"
  | "new-session";
interface PluginComposerCapability {
  insertText(text: string): { status: "inserted" | "ignored" | "unavailable" };
  focus(): { status: "focused" | "unavailable" };
  submit(): Promise<{
    status: "submitted" | "blocked" | "unavailable";
    reason?: string;
  }>;
}
interface PluginComposerSlotProps {
  surface: PluginComposerSurface;
  presentation: PluginPresentation;
  taskId: string | null;
  taskTitle?: string;
  activeSessionId: string | null;
  sessionIds: string[];
  disabled: boolean;
  submittable: boolean;
  disabledReason?: string;
  composer: PluginComposerCapability;
}

interface PluginTaskPanelProps {
  panelId: string; // this registration's panel id, so one Component can back multiple panels
  taskId: string;
  sessionId: string | null;
  presentation: PluginPresentation;
}

interface TaskPanelRegistration {
  id: string; // plugin-local panel id (unique within the plugin, not globally)
  title: string; // add-panel-menu row label and dockview tab title
  icon?: PluginIcon;
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
  id: string; // plugin-local filter id (unique within the plugin, not globally)
  label: string; // filter section label shown in the dropdown
  getOptions(): PluginTaskFilterOption[];
  // Called only when `selected` is non-empty — an empty selection is
  // implicit "All" and always matches without invoking this method.
  matches(context: PluginTaskFilterContext, selected: string[]): boolean;
}
```

The host treats `matchesURL` only as a coarse candidate filter. It runs every
remaining provider's cancellable, workspace-scoped `inspectURL`; `null` means the
configured provider does not own the URL. One structured result wins, more than one
is an ambiguity error, and registration order never decides ownership.

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
hints, not entity payloads. Use a typed `host.context` read or host-verified action;
do not inspect private store slices from a released plugin.

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
item instead of nesting it under `Edit`. It appears on cards and on the shared
desktop/mobile task-row menu. Group `"edit"` remains card-only. Visibility
filtering, registration order, and `run()`/error handling are identical; the
two groups are independent lists (an action only ever belongs to one).

`"task-card-indicators"` (documented above with the other slots) is the
matching read-only surface: a small icon/badge rendered beside the PR status
icon on every card, receiving `{ taskId, workspaceId, workflowStepId }`.

`"task-card-tags"` is a second, sibling read-only surface for the same
context — same `{ taskId, workspaceId, workflowStepId }` shape — but mounted
in its own row on the card instead of the cramped title row
`"task-card-indicators"` shares with `PRTaskIcon`. Use it for a contribution
that needs its own width, e.g. a row of tag chips.

`"task-row-metadata"` is a separate, plugin-agnostic slot for compact,
read-only metadata in the sidebar task tree and `/tasks` list. Its shape is
`{ taskId, workspaceId, workflowStepId, surface }`, with `surface` equal to
`"sidebar"` or `"task-list"`. The host emits no wrapper when the slot is empty.

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

### Task-list facets

`registerTaskListFacet({ id, label, getValues, subscribe? })` adds a choice to `/tasks` Sort and
Group controls. `getValues({ taskId, workspaceId })` synchronously returns `{ value, label,
color? }[]`; `subscribe` invalidates the loaded page. Return each value at most once for a task,
and keep one label and color for a value across all tasks. Facet sorting uses the first value label
after a case-insensitive alphabetical comparison. Facets are page-local: no facet selection is
persisted or sent to the backend. The host catches callback errors and revokes registrations and
active subscriptions when the owning plugin unloads. A task with multiple values appears in each
matching group; a task without a value appears in the host's `Ungrouped` section. Parent/child
indentation is preserved only within a group both tasks match, so a matching child without its
parent is rendered at that group's root.

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
  `chat-input-actions` slot for task and Quick Chat, passing the typed
  `PluginComposerSlotProps`. Task creation and new-session creation mount
  `task-create-input-actions` and `new-session-input-actions` with the same
  capability contract. `kanban-card-plugin-slots.tsx`
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
