---
status: draft
system: plugins
requirements:
  - REQ-PLUGINS-PLUGINS-001
created: 2026-04-26
owners:
  - cfl
---
# Plugin System System Design Part 3

## Purpose and boundaries

This design preserves the technical source detail for `REQ-PLUGINS-PLUGINS-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLUGINS-PLUGINS-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Frontend plugin runtime (native JS UI plugins)

Plugins may extend the SPA with **native** React UI — routes, nav items, slot
components, and WebSocket handlers that run inside the kandev frontend (the
Mattermost-webapp model), not iframes. The full contract lives in
`docs/plans/plugins/PLUGIN-API.md`; summary:

- **Manifest:** a plugin declares `ui.bundle` (a root-relative path inside the
  extracted package, e.g. `/ui/bundle.js`) and optional root-relative `ui.styles`.
- **Bundle delivery:** kandev serves the bundle at `GET /api/plugins/{id}/bundle`
  (and any assets under `GET /api/plugins/{id}/ui/*`) directly from the extracted
  package directory on local disk, forcing `Content-Type: text/javascript` and
  stripping frame-blocking headers. No reverse proxy and no live upstream process are
  involved in serving these assets — the plugin subprocess only needs to be running
  to serve gRPC calls, not the UI bundle.
- **Boot payload:** the SPA boot payload carries
  `plugins: [{ id, name, bundleUrl, styleUrls, repositoryProviderIds? }]` for every
  **active** plugin that declares a bundle (gated on the `plugins` feature flag).
  `repositoryProviderIds` is the JSON projection of manifest
  `repository_providers`; before `initialize`, the loader supplies it to the registry
  as the plugin's provider/review ownership allowlist. Omission keeps older payloads
  compatible but grants no invented manifest declaration.
- **Loading:** on boot (and on runtime enable), the frontend host dynamically
  `import()`s each `bundleUrl`. The bundle calls
  `window.registerKandevPlugin(id, { initialize(registry, host), destroy? })`.
  `host` shares the kandev React instance, the app store, a plugin-scoped
  `api.fetch`, a curated `@kandev/ui` subset, and the theme — so a plugin can build
  a page indistinguishable from first-party UI (e.g. a native `/jira` page).
  Activation is transactional: failure or timeout revokes every partial registration,
  aborts owned work, and fences callbacks that arrive after the attempt ended.
- **Registry surface:** `registerRoute(path, C)`, `registerNavItem(item)`,
  `registerSettingsRoute(path, C)`,
  `registerIntegrationSettings({ id, label, description, icon?, Component })`,
  `registerComponent(slot, C)` (including `app-status-bar-left` and
  `app-status-bar-right`), and `registerWsHandler(action, fn)`. Integration-settings
  contributions appear in the native global integrations index, workspace settings
  navigation, and global/workspace settings routes. An optional integration action
  receives the routed workspace and a `surface` value for the detail header or index
  card. IDs are URL-safe, unique among active plugins, and cannot shadow host
  integrations; unload revokes the contribution.
  Status-slot components receive the exact `AppStatusBarSlotProps` contract in
  `PLUGIN-API.md`: current path/context plus placement and presentation. The host
  renders one responsive presentation at once — 24 px bar on tablet/desktop or
  phone Status drawer — so a plugin must tolerate remounting and adapt its own UI.
  A status slot chooses the contribution's default side; portable user order may
  move either contribution across the desktop spacer and determines drawer order.
  Registrations are namespaced per plugin and bulk-revoked on disable/uninstall.
- **Isolation (v1):** only active, operator-installed plugins load; a failing
  bundle/`initialize` is caught and never breaks boot; slot components render behind
  error boundaries. Plugin JS otherwise runs with full in-origin store access —
  hard sandboxing (workers/realms) is future work (see Out of scope).
- **Keybindings:** a plugin declares `ui.keybindings: [{ id, default, description }]`
  in its manifest (`default` is a combo string like `mod+shift+k`, validated at
  registration time). `registry.registerKeybinding(id, handler)` binds a JS handler
  to a declared id; the host resolves the effective combo (a user override from
  Settings > Keyboard Shortcuts if set, else the manifest `default`) and dispatches
  it globally, skipping editable targets the same way core app shortcuts do. User
  overrides are namespaced `plugin:{pluginId}:{id}` so they survive independently
  per plugin.
- **Modals and drawers:** `host.openModal({ title?, description?, content, size?, dismissible?, presentation? })` imperatively
  opens a modal rendered by the host's `<PluginModalHost/>` (mounted once inside the
  authenticated `AppShell` theme/tooltip/toast provider tree and isolated behind its
  own error boundary) and returns `{ close() }` to close
  that instance. `content` reuses the slot-component contract (rendered with the
  host React instance). `presentation: "drawer"` renders the contribution in a
  safe-area-aware host drawer for phone actions; omitted/`dialog` keeps desktop modal
  presentation. Independent of keybindings — any plugin code path may call it.
- **Task change-request links:** `host.openTaskLinkDialog(...)` renders the same
  one-field dialog anatomy, inline validation, Cancel/Save footer, submitting state,
  success toast, and close behavior as Kandev's GitHub task-link flow. Code-host
  plugins supply provider copy and an authenticated
  `onSubmit(reference, signal)` callback; closing, unmounting, or revoking the plugin
  aborts that signal and suppresses late feedback;
  they do not build this workflow inside `openModal`. Link submenu children name only
  the target because the parent already supplies the verb.
- **Provider registrations:** `registry.registerRepositoryProvider(...)` contributes
  repository listing, optional coarse URL matching, structured URL inspection, and
  branch listing. `matchesURL` is only a synchronous performance hint; workspace-scoped
  `inspectURL` is authoritative, returns `null` when the configured provider does not
  own the URL, and must produce exactly one winner across active providers. `inspectURL` returns
  a complete credential-free provider/repository/pull-request descriptor with an HTTPS
  clone URL. The host
  persists that descriptor rather than parsing plugin URLs. Once persisted, native task
  branch requests are routed to the active manifest owner through its declared
  workspace-scoped `repositories.branches` action using only that stored descriptor.
  `registerTaskAction(...)`
  contributes children of the native **Link** submenu with visibility and a read-only
  task/workspace/repository context. Unsupported top-level action placements are
  rejected at compile/registration time rather than accepted without a consumer.
  `registerReviewProvider(...)` supplies
  normalized review summaries, an external-store task-item source, and a plugin
  `ReviewPanel` rendered in native desktop/mobile review surfaces. All are
  manifest-owned, lifecycle-revocable registrations; duplicate active provider
  ownership is rejected.
- **Task panels:** `registerTaskPanel({ id, title, icon?, Component, mobileEnabled? })`
  adds a row to the task workspace's "+" (add panel) menu; selecting it opens a
  dockview panel rendering `Component` with `{ panelId, taskId, sessionId,
presentation }`. Every plugin panel shares one generic `"plugin-panel"` dockview
  component (identity in `params.pluginId`/`params.panelKey`), so a saved layout
  round-trips even when the owning plugin is later uninstalled — the layout manager
  drops an unresolvable reference instead of throwing, and `Settings > Layouts`
  renders a placeholder box for it. On phones, one bounded **Panels**
  bottom-navigation entry opens a touch-native picker containing every
  `mobileEnabled: true` registration; choosing one renders the same `Component`
  full-height with `presentation: "mobile"`. Plugin loading and reloading are
  authoritative lifecycle states, not elapsed-time guesses: a temporarily missing
  registration never deletes an open or saved panel while its plugin is loading.
  A successful initialization that omits a previously registered panel, or a
  definitive disable/uninstall, closes that panel. If the removed panel is focused
  on a phone, the session deterministically returns to Chat. A `Component` that
  throws renders a per-panel error-boundary fallback without affecting the rest of
  the layout. Decision:
  ADR-2026-08-04-plugin-contribution-lifecycle-authority.
- **Task-menu and row contributions:** `registerTaskMenuAction({ id, label, icon?,
group: "edit" | "primary", visible?, run })` adds `"edit"` actions to the kanban card's `Edit`
  submenu (the flat `Edit` item becomes `Edit > Edit task` once any plugin
  registers one); `run(context)` receives `{ workspaceId, taskId, taskTitle,
workflowStepId, presentation }`, and a rejected `run` is caught and logged
  without blocking the menu from closing. `"primary"` actions render on cards
  and desktop/mobile task-row menus. `registerComponent("task-card-indicators",
C)` renders `C` beside the PR status icon on every kanban card, receiving
  `{ taskId, workspaceId, workflowStepId }` as `slotProps`.
  `registerComponent("task-card-tags", C)` renders `C` in its own row on every
  kanban card (below the badges row), receiving the same
  `{ taskId, workspaceId, workflowStepId }` shape — for a contribution too wide
  for the cramped title-row `task-card-indicators` spot, e.g. a row of tag chips.
  `registerComponent("task-row-metadata", C)` renders plugin-agnostic,
  read-only metadata on sidebar and `/tasks` rows. It receives
  `{ taskId, workspaceId, workflowStepId, surface }`. An empty slot adds no
  wrapper or spacing. Decision: ADR-2026-08-18-plugin-task-row-metadata.
- **Sidebar workspace actions:** `registerComponent("sidebar-workspace-actions", C)`
  renders `C` after Quick Terminal/Quick Chat in the desktop sidebar's New Task
  row and in the shared phone navigation sheet, forwarding
  `{ workspaceId, workspaceLabel?, presentation }`. The host uses
  `presentation: "desktop"` or `"mobile"`; mobile actions must expose a
  touch-sized, accessible control.
- **`host.storage`:** authenticated, per-user key/value storage
  (`get`/`set`/`delete`/`list`/`subscribe`), backed by the `plugin_user_state`
  table (separate from the plugin-backend-only `plugin_state` table — no gRPC/proto
  change) and requiring `capabilities.user_state: true`. A successful
  `set`/`delete` publishes `plugin.user-state.updated` to the writing user's own
  WS connections only (payload carries keys, never the value); `host.storage.subscribe`
  filters to the plugin's own events and suppresses the writing tab's own echo via a
  per-tab `writerId`. See `docs/decisions/2026-08-01-per-user-plugin-storage.md` and
  the full contract in `PLUGIN-API.md`.
- **`host.ui.RichTextEditor` / `host.ui.RichTextReadOnly`:** narrow wrappers over the
  Plan panel's tiptap markdown editor, pixel-identical to the Plan panel, so a
  plugin needing rich text (e.g. a notes scratchpad) ships no tiptap of its own.

## State machine

```
registered -> active -> disabled -> uninstalled
registered|active|disabled --failure--> error
error --successful Enable/health recovery--> active
error --Disable--> disabled
```

| State         | Meaning                                                                                                                                                                                                               |
| ------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `registered`  | Package extracted and record written; go-plugin spawn/handshake pending or in flight                                                                                                                                  |
| `active`      | Handshake succeeded and health (`Ping`) passes; events delivered, webhooks proxied                                                                                                                                    |
| `error`       | 3 consecutive `Ping` failures (30s interval, injectable), or the subprocess crashed and restart attempts (backoff, max 5) are exhausted. Events buffered (ring buffer, 100 events, 5-minute TTL). Webhooks return 503 |
| `disabled`    | Operator explicitly disabled. Subprocess stopped. No events, no webhooks. State and config preserved                                                                                                                  |
| `uninstalled` | Subprocess stopped, package/record/state deleted (no grace period in v1)                                                                                                                                              |

Health monitoring: kandev's go-plugin client calls `Ping()` on the plugin every 30
seconds (injectable). 3 consecutive failures -> `error` + inbox item + restart attempt
with backoff. A subprocess crash (unexpected process exit) triggers an immediate
restart with backoff (max 5 attempts, then `error`). Next successful handshake/`Ping`
-> `active`, queued events delivered in order, and the persisted failure diagnostic
is cleared. An operator can manually enable a plugin in `error` to retry its spawn
and handshake; boot does not automatically retry a persisted `error` state. A
manual Enable racing the final restart-exhaustion callback must complete without
deadlock: the manager never waits for a stopping process while holding its
process registry lock, so the final callback and replacement start can both
complete.

## Permissions

- Plugins are global to the kandev instance, installed by the operator. There is no per-user plugin access in v1.
- Capability-based access control: undeclared capabilities on a Host RPC return gRPC
  status `PermissionDenied` with message `capability '<name>' not declared`.
- Each plugin's Host service instance is bound to its own plugin ID at spawn time —
  there is no plugin-supplied ID to check, so capability checks evaluate directly
  against that plugin's installed manifest on every RPC.

## Security

- **Auth is the spawn relationship.** Kandev spawns the plugin subprocess itself, so
  there is no separate credential to issue, store, or leak: the go-plugin handshake
  (magic cookie) plus AutoMTLS (mutual TLS negotiated per-launch, transparent to
  plugin authors) authenticate the channel. There is no `api_key`, no
  `webhook_secret`, and no HMAC signing anywhere in the contract.
- **Package integrity.** `checksums.txt` is verified for every file at install time.
  An optional ed25519 signature (`checksums.txt.sig`) is verified when present;
  unsigned packages install with a surfaced warning rather than being blocked (signing
  is not required in v1 — see "Out of scope").
- **Capability-based access control** evaluated per Host RPC via a server interceptor.
- **Network**: the plugin subprocess talks to kandev over a unix domain socket
  (macOS/Linux) or loopback TCP with AutoMTLS (Windows) — never a routable network
  address. There is no remote/operator-hosted plugin tier in v1; every plugin backend
  is a binary kandev spawns and supervises on the same host. See "Out of scope".
- **`host.storage`'s user-state routes are the first plugin HTTP surface reachable
  with the caller's own browser session** (every other plugin HTTP route is either
  operator-only management or a self-authenticating external webhook). It is
  guarded the same way any other authenticated API route is — `httpmw`'s allowlist
  explicitly does not cover it, so an unauthenticated request is rejected before the
  handler runs — plus a capability gate (`user_state`), scope/key validation, a body
  cap, and per-user row isolation. The stored value is opaque to the host: it is
  never interpreted, never included in the `plugin.user-state.updated` WS payload,
  and never delivered to the plugin's gRPC backend (a browser-only surface).

## Failure modes

- **3 consecutive `Ping` failures (90s), or crash with restart attempts exhausted
  (max 5, backoff)**: status -> `error`. Events buffered (100, 5min TTL). Webhooks
  return 503. Inbox item created and the bounded failure diagnostic is persisted
  with its timestamp.
- **Failed spawn/handshake, missing install path, or failed restart**: status ->
  `error` with the bounded diagnostic persisted; the Plugins UI exposes **Enable**
  as the manual retry action.
- **Diagnostic safety**: persisted failure text is normalized, credential/path
  redacted, and bounded before it is returned by plugin list/detail APIs. Plugin
  stdout is not a durable diagnostic channel.
- **Buffer overflows (>100 events or >5min)**: oldest events dropped. Kandev emits
  at most one overflow warning per plugin per minute, aggregating the number of
  dropped events since the previous warning.
- **Plugin returns a gRPC error (or times out) on `DeliverEvent`**: retry up to 3 times
  with exponential backoff (5s, 15s, 45s). After exhaustion, event is logged as failed
  and dropped.
- **External webhook hits a disabled/error plugin**: kandev returns 503.
- **Undeclared capability access attempt on a Host RPC**: gRPC `PermissionDenied` with
  a message naming the missing capability.
- **Undeclared, unauthorized, oversized, or timed-out browser action**: Kandev rejects
  it before/at bounded plugin dispatch; it never falls back to public webhook routing.
- **Disabled/unloaded repository or review provider**: registrations and in-flight work
  are revoked; provider credential leases are invalidated immediately, and host
  selectors/panels remove stale plugin items safely, including after same-ID reload.
- **Reference source denied during message submission**: Kandev rejects the selected
  reference rather than queuing stale or cross-workspace metadata.
- **Checksum mismatch or unresolvable host-platform executable at install time**:
  install is rejected before any code runs.
- **Frontend bundle import or initialization remains in flight**: open and saved
  plugin-panel identities are preserved regardless of duration. A failed or timed-out
  initialization leaves the panel recoverable for a later successful load; it is not
  interpreted as disable or uninstall.
- **Per-user state purge fails during uninstall**: uninstall fails closed before the
  package or plugin record is removed. The stopped-but-installed plugin remains
  retryable and a successful retry purges every user's rows before removal completes.

## Persistence guarantees

- Plugin installation records (`id`, `version`, `install_path`, capabilities, status,
  `signed`, `last_error`, `last_error_at`) persist to disk as
  `~/.kandev/plugins/<id>.yml` and survive backend restarts.
- Extracted plugin packages persist at `~/.kandev/plugins/<id>/<version>/` until
  uninstall.
- Plugin state in SQLite survives restarts.
- `plugin_user_state` survives restarts and ordinary disable/enable cycles, but no row
  survives a successful uninstall. If its purge fails, uninstall does not report
  success and leaves the plugin installed so the cleanup can be retried.
- Event delivery buffer is in-memory; events in the buffer do not survive a backend
  restart.
- There are no plugin credentials to persist or lose — auth is re-derived from the
  spawn relationship on every process launch.
