---
status: draft
system: plugins
requirements:
  - REQ-PLUGINS-PLUGINS-001
created: 2026-04-26
owners:
  - cfl
---
# Plugin System System Design Part 4

## Purpose and boundaries

This design preserves the technical source detail for `REQ-PLUGINS-PLUGINS-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLUGINS-PLUGINS-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Scenarios

- **GIVEN** an operator with a release tarball URL for a Slack notification plugin,
  **WHEN** the operator calls `POST /api/plugins/install` with `{"url": "..."}`,
  **THEN** kandev downloads the tarball, verifies `checksums.txt`, validates the
  manifest, extracts it to `~/.kandev/plugins/kandev-plugin-slack/1.0.0/`, spawns the
  platform-matched binary, completes the go-plugin handshake, and the plugin appears
  in `GET /api/plugins` with status `active`.

- **GIVEN** an operator with a plugin tarball on their local machine, **WHEN** the
  operator uploads it via `POST /api/plugins/install` (multipart `package`), **THEN**
  kandev runs the same verify → validate → extract → spawn pipeline and the plugin
  reaches `active` without any URL ever being contacted.

- **GIVEN** the operator has entered a valid URL or selected a package in the
  Settings > Plugins install dialog, **WHEN** the operator submits the install,
  **THEN** the primary Install action is disabled, marked busy, and shows an
  animated loading indicator with the installing label until the install and
  post-install loading pipeline settles. **WHEN** the pipeline succeeds, **THEN**
  the dialog closes as usual. **WHEN** it fails, **THEN** the indicator stops, the
  action becomes available for retry, and the existing inline error remains visible.

- **GIVEN** an operator who extracted a plugin package directly into
  `~/.kandev/plugins/<id>/<version>/` on the host filesystem (no install call), **WHEN**
  the operator clicks **Sync** in Settings > Plugins (`POST /api/plugins/sync`),
  **THEN** kandev finds the unrecorded `manifest.yaml`, validates it, and registers the
  plugin with status **`disabled`** — never spawning it automatically. The plugin then
  appears in the list, and the operator can enable it explicitly like any other plugin.

- **GIVEN** an active Slack plugin subscribed to `task.state_changed`, **WHEN** a task
  moves to `done`, **THEN** kandev calls the plugin's `DeliverEvent` gRPC method with
  the event over the go-plugin unix socket. The plugin formats a Slack message and
  calls the Slack API, then returns `EventAck{}`.

- **GIVEN** a Jira sync plugin with a registered `jira-webhooks` webhook, **WHEN**
  Jira POSTs a webhook to
  `https://kandev.example.com/api/plugins/kandev-plugin-jira/webhooks/jira-webhooks`,
  **THEN** kandev converts the HTTP request into a `WebhookRequest` and calls the
  plugin's `HandleWebhook` gRPC method. The plugin parses the Jira event, calls
  `Host.SetState` to record the linked task, and returns a `WebhookResponse` that
  kandev relays back as the HTTP response.

- **GIVEN** an active plugin subprocess that crashes, **WHEN** kandev detects the
  process exit, **THEN** kandev immediately attempts a restart with backoff, marks the
  plugin `error` while buffering events (up to 100 or 5 minutes), and creates an inbox
  item "Plugin kandev-plugin-slack is unreachable". **WHEN** a subsequent restart
  attempt succeeds and the handshake completes, **THEN** status returns to `active`
  and buffered events are delivered in order, with the persisted failure diagnostic
  cleared.

- **GIVEN** a plugin in `error` with a persisted `last_error`, **WHEN** the operator
  opens Settings > Plugins, **THEN** the row shows the diagnostic and an **Enable**
  action. **WHEN** the operator clicks **Enable** and the plugin starts successfully,
  **THEN** status returns to `active`, the diagnostic fields are cleared, and normal
  delivery resumes. **WHEN** the retry fails, **THEN** status remains `error`, the
  client refetches the authoritative plugin record, and the new diagnostic replaces
  the previous one in the row and detail views.

- **GIVEN** restart exhaustion is entering its final unhealthy callback, **WHEN** an
  operator concurrently clicks **Enable**, **THEN** the callback and Enable both
  complete and the replacement process is either started or reports its own result;
  neither operation waits forever on the other.

- **GIVEN** a persisted diagnostic containing an unbroken long token, **WHEN** the
  operator opens the plugin row or detail view on a phone, **THEN** the diagnostic
  wraps inside its container and the page has no horizontal overflow. Recovery
  actions have a minimum 44 CSS-pixel phone target.

- **GIVEN** a plugin whose event ring buffer is full, **WHEN** additional events are
  dropped, **THEN** kandev logs one warning for the first drop and suppresses further
  warnings for that plugin for one minute while counting them. **WHEN** the next
  warning is emitted, **THEN** it includes the aggregate number of drops since the
  previous warning.

- **GIVEN** a plugin whose manifest declares `secrets: false`, **WHEN** the plugin
  calls `Host.RevealSecret`, **THEN** kandev's server interceptor returns gRPC status
  `PermissionDenied` with message `capability 'secrets' not declared`, before the
  handler runs.

- **GIVEN** two plugins (Slack and Jira), **WHEN** the Jira plugin calls
  `Host.EmitEvent` with `event_name: "sync-completed"`, **THEN** it is published as
  `plugin.kandev-plugin-jira.sync-completed` and the Slack plugin (subscribed to
  `plugin.kandev-plugin-jira.*`) receives it via `DeliverEvent` and posts a sync
  summary to Slack.

- **GIVEN** a plugin with state, **WHEN** the plugin calls `Host.SetState` with
  `scope: "task", scope_id: "task_xyz", key: "jira_issue_id", value: "PROJ-123"`,
  **THEN** the state is persisted in SQLite. A subsequent `Host.GetState` call with the
  same scope/key returns `"PROJ-123"`.

- **GIVEN** a plugin whose manifest declares `api_read: ["sessions"]`, **WHEN** the
  plugin calls `ListSessionCodeStats`, **THEN** kandev returns per-session committed
  and peak-pending line counts (plus, via `ListSessions`, each session's
  `acp_session_id`) computed from the service layer, without the plugin opening the
  kandev database file.

- **GIVEN** a plugin whose manifest does **not** declare `tasks` in `api_read`,
  **WHEN** the plugin calls `ListTasks`, **THEN** kandev returns gRPC status
  `PermissionDenied` with message `capability 'api_read:tasks' not declared`, before
  the handler runs.

- **GIVEN** a plugin with `api_read: ["tasks"]` and more tasks than one page,
  **WHEN** it calls `ListTasks` with `Page{limit: 50}` and then again with the
  returned `PageInfo.next_cursor`, **THEN** the second call returns the next page and
  `has_more` is false once the last page is reached.

- **GIVEN** a plugin with `api_read: ["tasks"]`, **WHEN** it calls `ListTasks`
  without `include_ephemeral`, **THEN** quick-chat ephemeral tasks are excluded from
  the results.

- **GIVEN** an active plugin registers `app-status-bar-left` or
  `app-status-bar-right`, **WHEN** Kandev switches between desktop/tablet and phone,
  **THEN** the plugin receives the exact slot props for the active bar or Status
  drawer presentation, and only that presentation is mounted.

- **GIVEN** a user has moved a registered status contribution, **WHEN** the plugin
  disables and later enables or Kandev restarts, **THEN** its deterministic
  plugin/slot/ordinal identity restores the saved position; the original slot
  remains its default side rather than overriding user order.

- **GIVEN** a plugin whose manifest declares `api_write: ["tasks"]`, **WHEN** it
  calls `CreateTask`, **THEN** kandev creates the task through the task service
  (firing `task.created`), stamps `source = "plugin:<id>"`, and returns the task;
  **WHEN** a plugin without `api_write:tasks` calls `CreateTask`, **THEN** kandev
  returns gRPC `PermissionDenied` with `capability 'api_write:tasks' not declared`.

- **GIVEN** a plugin whose manifest declares `api_write: ["messages"]`, **WHEN**
  it calls `SendMessage` on a task, **THEN** kandev delivers the prompt to the
  task's session through the orchestrator (queueing if running, resuming/starting
  otherwise), records a user message with `source = "plugin:<id>"`, and returns
  the target session and a `queued`/`sent`/`started` status; a plugin lacking
  `api_write:messages` is denied.

- **GIVEN** a `SendMessage` whose dispatch fails after the user message was
  recorded, **THEN** kandev deletes the recorded message and returns an error, so
  a failed delivery leaves no durable user message and a retry can't duplicate
  the prompt.

- **GIVEN** an active plugin declares `connection.save` with workspace scope,
  **WHEN** a signed-in user invokes `host.api.invokeAction("connection.save", ...)`,
  **THEN** Kandev authenticates the caller, authorizes the workspace, bounds the
  payload, and passes verified context separately to `Plugin.HandleAction`; the public
  webhook endpoint is not invoked.

- **GIVEN** a provider operation owns an `AbortSignal`, **WHEN** its registration is
  replaced, unloaded, or its request is superseded, **THEN** `invokeAction` propagates
  cancellation to the authenticated HTTP request and backend action context.

- **GIVEN** a manifest-owned repository and review provider, **WHEN** it registers
  native task-link actions and a review panel, **THEN** Kandev renders those contributions
  in applicable desktop and mobile task/review surfaces and removes them, including
  in-flight work, when the plugin disables.

- **GIVEN** a registered review provider publishes normalized task status, **WHEN** a
  linked task renders on desktop or mobile, **THEN** Kandev automatically mounts the
  same topbar control, composer CI chip, hover popover/mobile drawer, review summary,
  comment summary, and checks anatomy used by GitHub, refreshes through the provider
  lifecycle, and requires no provider-owned visual slot or second poller.

- **GIVEN** a compatible code-host review provider, **WHEN** its panel renders normalized
  detail through `host.ui.ChangeRequestDetail`, **THEN** GitHub and the provider share
  the same header, collapsible sections, add-to-context controls, scroll ownership,
  loading/error treatment, and desktop/mobile geometry; only data, capabilities, and
  callbacks remain provider-owned.

- **GIVEN** a plugin-owned `#` reference source returns a pull-request candidate,
  **WHEN** the user submits the selected reference, **THEN** Kandev authorizes it
  again against the live plugin and rejects a disabled, stale, tampered, or
  cross-workspace candidate before queueing message metadata.
- **GIVEN** a saved or open plugin task panel and a plugin reload whose import or
  `initialize()` takes longer than 500 milliseconds, **WHEN** registration eventually
  succeeds with the same panel identity, **THEN** the panel remains in the layout and
  renders the reloaded component without being closed or deleted.

- **GIVEN** an open plugin task panel and a successfully initialized replacement
  version that no longer registers that panel identity, **WHEN** initialization
  completes, **THEN** the obsolete panel closes exactly once.

- **GIVEN** a plugin task panel focused on a phone, **WHEN** the plugin is disabled or
  uninstalled, **THEN** its picker row disappears, its component unmounts, and the
  session focuses Chat instead of leaving an unavailable panel selected.

- **GIVEN** several plugins register mobile-enabled task panels, **WHEN** a user opens
  the task workspace on a phone, **THEN** the fixed bottom navigation exposes one
  touch-sized Panels entry whose picker lists every panel without shrinking the other
  navigation targets or causing document-level horizontal overflow.

- **GIVEN** a plugin task-menu action is invoked from the phone kanban, **WHEN** its
  `visible(context)` or `run(context)` callback executes, **THEN**
  `context.presentation` is `"mobile"`; the same action invoked from desktop receives
  `"desktop"`.

- **GIVEN** a plugin registers a `"primary"` task-menu action, **WHEN** a user
  opens a desktop sidebar or phone task-sheet menu, **THEN** the same action is
  present with the correct `presentation`. **GIVEN** the plugin unregisters
  while the menu is open, **THEN** its action disappears without a reload.

- **GIVEN** any plugin registers `"task-row-metadata"`, **WHEN** sidebar or
  `/tasks` rows render, **THEN** it receives `{ taskId, workspaceId,
  workflowStepId, surface }`. **GIVEN** the slot is empty, **THEN** the host
  renders no metadata wrapper or extra spacing.

- **GIVEN** a plugin has per-user state and deleting those rows returns an error,
  **WHEN** an operator uninstalls it, **THEN** the request fails, the package and plugin
  record remain installed for retry, and no successful-uninstall response is emitted.
  **WHEN** cleanup later succeeds and uninstall is retried, **THEN** all users' rows,
  the package, and the record are removed.

- **GIVEN** a plugin registers a component for `"task-card-tags"`, **WHEN** any
  kanban card renders, **THEN** that component mounts in its own row (distinct
  from the title row hosting `"task-card-indicators"`), receiving exactly
  `{ taskId, workspaceId, workflowStepId }` for that card. **GIVEN** no plugin
  is registered for the slot, **WHEN** a card renders, **THEN** no extra DOM
  node or empty-row spacing appears. **GIVEN** two plugins register for
  `"task-card-tags"`, **WHEN** a card renders, **THEN** both render, in
  registration order. **GIVEN** a `"task-card-tags"` component throws during
  render, **WHEN** that card renders, **THEN** the card's title and its other
  slot components (e.g. `"task-card-indicators"`) still render, isolated by
  the existing per-registration error boundary.

- **GIVEN** a plugin registers a component for `"sidebar-workspace-actions"`,
  **WHEN** the sidebar's New Task row renders with the rail expanded and a
  workspace active, **THEN** that component mounts in the row's action
  cluster after the built-in Quick Terminal and Quick Chat icons, receiving
  `{ workspaceId, workspaceLabel, presentation: "desktop" }` for the active
  workspace, and the flex row keeps the label and every registered action
  reachable without overlap.
  **GIVEN** no plugin is registered for the slot, **WHEN** the row renders,
  **THEN** no plugin action markup or extra row spacing appears. **GIVEN** the
  desktop rail is collapsed or no workspace is active, **WHEN** the desktop
  row renders, **THEN** no `"sidebar-workspace-actions"` markup renders,
  matching the built-in icons' own visibility. **GIVEN** a
  `"sidebar-workspace-actions"`
  component throws during render, **WHEN** the row renders, **THEN** Quick
  Terminal and Quick Chat still render and function, isolated by the existing
  per-registration error boundary.
  **GIVEN** the same plugin is active on a phone, **WHEN** the shared navigation
  sheet opens, **THEN** the component mounts with
  `presentation: "mobile"` and its control remains reachable with a 44px
  touch target.

## Out of scope

- **Remote / operator-hosted plugin tier.** The earlier `base_url` registration model,
  where an operator ran and managed a plugin process themselves and kandev only knew
  its address, is removed. Every plugin backend in v1 is a binary kandev spawns and
  supervises locally. A remote tier (kandev talking gRPC to a plugin process it does
  not spawn) may return as future work if a real need emerges.
- **Plugin JS sandboxing.** Native UI plugins (see "Frontend plugin runtime") run
  in the kandev origin with full app-store access. Isolating plugin JS in a worker,
  realm, or comparable sandbox is future work; v1 relies on only loading active,
  operator-installed plugins served same-origin.
- **In-process backend plugins.** Plugin _backends_ remain out-of-process — no Go
  plugin loading via `plugin.Open`, no WASM, no shared-memory communication. (This is
  distinct from the frontend bundle, which does load into the SPA.)
- **Plugin marketplace or registry.** Out of scope _for this spec_: this spec covers
  install-by-URL/upload as a manual, single-plugin action. The discoverable, curated
  catalog (central registry, one-click install, star ranking, third-party sources) is
  a sibling feature specified in [marketplace.md](../requirements/marketplace.md) and built on top of
  this spec's install pipeline.
- **Mandatory package signing.** `checksums.txt.sig` verification is supported when
  present, but signing is optional in v1 — an unsigned package installs with a warning
  rather than being rejected. Requiring signatures is future work.
- **Agent tools in the base v1 runtime.** The original parallel `tools[]` plus
  `InvokeTool` path remains removed. Plugin-contributed agent tools are a
  separate extension specified in [Plugin-Contributed Agent Tools](../requirements/agent-tools.md)
  and feed through Kandev's existing MCP surface rather than a second discovery
  or invocation protocol.
- **Hot reload.** Upgrading a plugin requires a new install (new version directory);
  there is no in-place manifest or binary swap on a running process.
- **Multi-instance plugins.** Each plugin ID maps to exactly one supervised subprocess.
- **Event admission rate limiting.** No per-plugin rate limits in v1. Misbehaving
  plugins can be disabled manually. Diagnostic log aggregation is still required
  for bounded event-buffer overflow warnings.
- **Plugin database namespaces.** Plugins do not get their own SQLite schemas. KV state is sufficient for v1.
- **Broader write surface.** The Host data API writes cover task create/update
  (conservative field mask) and sending a message to a task session. Deleting or
  archiving tasks, writing sessions/workspaces/workflows/repositories, a wider
  task-update mask, and delivery-mode/interrupt control on SendMessage are out of
  scope for now. See "Host data API".
- **Per-session code-stats precomputation.** `SessionCodeStats` is computed on
  demand per request in v1; a materialized or cached aggregation is future work.
- **New plugin contribution hooks or storage contracts.** This hardening does not add
  registration methods, change the `plugin_user_state` schema, alter user-state HTTP
  routes or WS payloads, or broaden the rich-text wrappers.
- **A general mobile navigation redesign.** The bounded Panels picker applies only to
  plugin-contributed task panels and reuses the existing task-mobile picker pattern.
- **Workspace-scoped plugin data access.** v1 reads are global to the instance with
  a reserved scoping hook; per-plugin or per-user workspace restriction is future
  work (see ADR 0043 open decisions).
