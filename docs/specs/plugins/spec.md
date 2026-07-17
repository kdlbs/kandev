---
status: draft
created: 2026-04-26
owner: cfl
---

# Plugin System

The plugin system is a **platform-level capability of kandev**, not tied to any single
feature area. It lets third parties (and kandev itself) extend the product — backend
behavior and native frontend UI — through a stable contract, without forking or
modifying core.

The gRPC/go-plugin transport, package format, and RPC surface described below are
frozen in `docs/plans/plugins/GRPC-CONTRACT.md`; that file is the authoritative
wire-level reference. The native frontend contract is frozen in
`docs/plans/plugins/PLUGIN-API.md`. This spec describes the resulting product
behavior.

## Why

Kandev keeps growing external integrations and surface-specific behavior directly in
the core codebase: source-control sync, issue-tracker browsing, notification providers,
and planned channel types (Slack, Discord, Telegram, email). Each one adds
platform-specific logic — API clients, webhook handlers, payload formatting, OAuth
flows, secret management, and bespoke UI — to the Go backend and the SPA. This creates
three problems:

1. **Core bloat.** Every new integration increases the surface area of core. Adding more
   at similar scale makes the codebase unmaintainable.
2. **Release coupling.** Fixing a bug in one integration requires a full kandev release,
   and users who don't use it still receive the code. Integration authors cannot ship
   independently of the core release cycle.
3. **Extensibility ceiling.** Users and third parties cannot add their own integrations
   or UI without forking kandev.

A plugin system decouples extensions from core. Plugin **backends** are Go binaries
that kandev spawns and supervises as subprocesses, communicating over a strict typed
gRPC protocol; plugins may additionally ship a **native frontend bundle** that kandev
loads into the SPA. Both extend kandev through well-defined, capability-gated
surfaces. The core stays small; the ecosystem grows independently.

## What

- Plugin **backends** are **Go binaries** distributed inside a release tarball
  (per-platform executables) that kandev **spawns and supervises as subprocesses**
  via `hashicorp/go-plugin`, speaking a strict typed **gRPC protocol**
  (`kandev.plugin.v1`) over a unix domain socket (macOS/Linux) or loopback TCP with
  AutoMTLS (Windows). No in-process backend loading, no separately-managed operator
  process, no HTTP transport for the backend contract.
- A plugin MAY additionally ship a **native frontend bundle** (`ui.bundle`) that kandev
  loads into the SPA to register native routes/nav/components (see "Frontend plugin
  runtime"). This is the one in-process surface; the backend stays out-of-process
  (but is now kandev-managed, not operator-managed).
- A plugin manifest declares identity, runtime executables (per OS/arch), capabilities,
  declared tools, declared webhooks, config schema, and optional UI bundle.
- Plugins SHALL receive events, register agent tools, expose proxied external webhook
  endpoints, and read/write a plugin-scoped KV state — all over gRPC.
- Plugins are distributed as a signed-or-unsigned release **tarball** and installed
  either by **URL** (kandev downloads it) or by **manual upload** (multipart file).
  There is no manifest-paste registration step.
- Capability-based access control: a plugin can only call Host RPCs it declared in its
  manifest; undeclared capabilities are rejected with a gRPC `PermissionDenied` status.
- **Kandev owns the plugin process lifecycle**: it extracts the package, spawns the
  binary, performs the go-plugin handshake, health-checks it (`Ping`), and restarts it
  on crash or health-check failure. Operators no longer run or manage plugin processes
  themselves. The remote/self-hosted tier (`base_url` registration of an
  operator-run process kandev never spawns) is removed; see "Out of scope".

## Data model

### Plugin package format

A plugin ships as a release tarball, `<id>-<version>.tar.gz`:

```
manifest.yaml                        # authoritative; read BEFORE any code runs
server/plugin-<goos>-<goarch>[.exe]  # any subset of platforms; host key required at install
ui/bundle.js                         # optional (frontend half)
ui/*.css / assets/icon.svg           # optional
checksums.txt                        # "sha256  path" for every other file
checksums.txt.sig                    # OPTIONAL ed25519 signature (unsigned → warn, not blocked)
```

`manifest.yaml` declares identity, capabilities, tools, webhooks, config schema, and
an optional UI bundle, plus a `runtime` block naming the per-platform executables
(replaces the old `base_url`/`endpoints` block, which is removed entirely):

```yaml
id: "kandev-plugin-slack"                    # Unique, pattern: ^[a-z0-9][a-z0-9._-]*$
api_version: 1
version: "1.0.0"
display_name: "Slack Notifications"
description: "Post to Slack on task events, relay messages to agents"
author: "kandev"
categories: ["connector"]                    # connector | automation | tools | analytics

runtime:
  type: binary
  executables:
    linux-amd64: server/plugin-linux-amd64
    darwin-arm64: server/plugin-darwin-arm64
    # ... any subset; kandev requires the running host's platform key at install time
min_kandev_version: "0.78.0"                 # optional

capabilities:
  events: ["task.created", "task.state_changed", "agent.completed"]
  api_read: ["tasks", "sessions"]             # Host data API reads (see below); live now
  api_write: ["tasks", "comments"]            # Host data API writes; deferred, no effect yet
  state: true
  secrets: true

tools:
  - name: "slack_send_message"
    display_name: "Send Slack Message"
    description: "Posts a message to a Slack channel"
    input_schema:
      type: object
      properties:
        channel: { type: string }
        text: { type: string }
      required: ["channel", "text"]

webhooks:
  - key: "slack-events"
    description: "Slack Events API webhook"
    method: "POST"

config_schema:
  type: object
  properties:
    bot_token_secret: { type: string, description: "Secret reference for Slack bot token" }
    default_channel:  { type: string, description: "Default channel for notifications" }
    notify_on_task_created: { type: boolean, default: true }
  required: ["bot_token_secret", "default_channel"]

ui:                                            # Native frontend plugin (see "Frontend plugin runtime")
  bundle: "ui/bundle.js"                       # ES module extracted alongside the package
  styles: ["ui/plugin.css"]                    # optional stylesheets

# Runtime fields managed by kandev (not authored):
status: "active"
version: "1.0.0"
install_path: "~/.kandev/plugins/kandev-plugin-slack/1.0.0"
installed_at: "2026-04-26T10:00:00Z"
restart_count: 0
```

`capabilities.api_read` / `capabilities.api_write` gate the **Host data API** Host
RPCs (read RPCs live now; write RPCs are deferred) — the vocabulary is a list of
resource names: `tasks`, `sessions`, `workspaces`, `workflows`, `agent_profiles`,
`repositories` for `api_read`, plus `comments` for `api_write` only (there is no
`ListComments` read RPC). They are unrelated to office. See "Host data API".

**Declaring data access.** Listing a resource under `api_read` grants the
corresponding Host data reads for that resource only — e.g. `api_read:
["sessions"]` unlocks `Host.Sessions().List(...)` and
`Host.Sessions().CodeStats(...)` (backed by `ListSessions` /
`ListSessionCodeStats`) but not `Host.Tasks()`. A resource left off the list
still resolves to a reader/accessor (no nil pointer), but every method on it
returns gRPC `PermissionDenied` with message `capability 'api_read:<resource>'
not declared`. `api_write` entries are accepted and stored but currently have no
effect (see "Write phase (deferred)").

### Install pipeline

There is no manifest-paste registration step. A plugin is installed from a URL or an
uploaded tarball:

1. Operator calls `POST /api/plugins/install` with JSON `{"url": "..."}` (kandev
   downloads the tarball) or a multipart `package` field (direct upload).
2. Kandev verifies `checksums.txt` covers every other file in the tarball and every
   hash matches (integrity gate, always enforced).
3. If `checksums.txt.sig` is present, kandev verifies the ed25519 signature; if
   absent, install proceeds with a surfaced "unsigned plugin" warning (signing is not
   required in v1 — see "Out of scope").
4. Kandev parses and validates `manifest.yaml` **before any code runs**: schema, `id`
   pattern, capability vocabulary, and that `runtime.executables` contains an entry
   for the host's OS/arch.
5. Kandev extracts the package to `~/.kandev/plugins/<id>/<version>/` and records the
   installation (`id`, `version`, `install_path`, capabilities, status).
6. Kandev spawns the platform-matched binary via `hashicorp/go-plugin`, completes the
   handshake (§2 of GRPC-CONTRACT.md) — status `registered` while this is pending.
7. Handshake succeeds → status `active`. Handshake/spawn failure → status `error`
   (restart retried with backoff; see "State machine").

Uninstall stops the subprocess and removes the record, all installed versions, and
plugin state (no 24-hour grace period in v1). `POST /api/plugins/register` is removed;
there is no operator-supplied manifest, no generated credentials, and no cleartext
secret returned at install time.

### `plugin_state` (SQLite)

```sql
CREATE TABLE plugin_state (
    id TEXT PRIMARY KEY,
    plugin_id TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT 'instance',    -- instance | workspace | task | agent
    scope_id TEXT,                              -- NULL for instance scope
    state_key TEXT NOT NULL,
    value_json TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (plugin_id, scope, scope_id, state_key)
);
```

## API surface

### Plugin management API (operator -> kandev, HTTP)

```
POST   /api/plugins/install           # Install a plugin: JSON {"url": "..."} or multipart `package`
POST   /api/plugins/sync              # Reconcile the registry with the plugins directory on disk
GET    /api/plugins                    # List installed plugins
GET    /api/plugins/{id}              # Get plugin detail
GET    /api/plugins/{id}/config       # Stored operator config; secret values masked
                                      # (secret fields live in the encrypted vault; the
                                      # config file persists only a vault reference)
PATCH  /api/plugins/{id}              # Update plugin config (masked secrets keep stored values; restarts a running plugin)
DELETE /api/plugins/{id}              # Uninstall plugin (stops subprocess, removes package + state)
POST   /api/plugins/{id}/enable
POST   /api/plugins/{id}/disable
GET    /api/plugins/tools             # List all declared tools across active plugins
GET    /api/plugins/{id}/bundle       # Frontend bundle, served from the extracted package dir
GET    /api/plugins/{id}/ui/*         # Frontend bundle assets, served from the extracted package dir
```

`POST /api/plugins/sync` is documented in full under "Filesystem sideloading & sync"
below.

`GET /api/plugins/{id}/bundle` and `/ui/*` are served directly by kandev **from the
extracted package directory** (`~/.kandev/plugins/<id>/<version>/ui/...`) — there is
no reverse proxy and no upstream plugin process involved in serving frontend assets,
since the files are already on local disk after install.

Enable/disable/uninstall act on the supervised subprocess: disable stops it (state and
config preserved); enable respawns it; uninstall stops it and deletes its package,
record, and state.

### Event delivery (kandev -> plugin, gRPC `DeliverEvent`)

Kandev calls the plugin's `Plugin.DeliverEvent` RPC (unary) with an `Event` message:

```proto
message Event {
  string event_id = 1;                     // fresh uuid per delivery
  string event_type = 2;                   // bus subject, e.g. "task.created"
  string occurred_at = 3;                  // RFC3339 UTC
  string workspace_id = 4;                 // empty if not derivable
  google.protobuf.Struct payload = 5;      // marshaled bus event.Data
}
```

Expected response: `EventAck{}`. A non-nil gRPC error or a timeout counts as failure.

Delivery semantics (unchanged from earlier design, now carried over gRPC):
- **At-least-once.** Plugins must be idempotent (dedup by `event_id`).
- **Timeout:** 10 seconds. Up to 3 retries with exponential backoff (5s, 15s, 45s).
- **Sequential per plugin** — no concurrent delivery to the same plugin. Plugins
  needing parallel processing queue internally.
- **Buffering while unhealthy:** events are held in a ring buffer (100 events, 5-minute
  TTL) and flushed in order once the plugin recovers (health/Ping succeeds again).

Event types: any subject kandev publishes on its internal event bus
(`internal/events/types.go`). Plugins subscribe to whatever they need; the catalog
below is a non-exhaustive sample across feature areas — the plugin system is not tied to
any one of them.

| Category | Events |
|----------|--------|
| Tasks | `task.created`, `task.updated`, `task.state_changed`, `task.deleted`, `task.moved` |
| Sessions | `task_session.state_changed`, `turn.started`, `turn.completed` |
| Agents | `agent.started`, `agent.completed`, `agent.failed`, `agent.stopped` |
| Other feature areas (examples) | Any additional subjects emitted by feature areas such as office/agents (e.g. `office.comment.created`, `office.approval.created`) — plugins may subscribe to these, but the plugin system does not depend on them |
| GitHub | `github.pr_state_changed`, `github.pr_feedback`, `github.new_issue` |
| Plugin | `plugin.<plugin_id>.<name>` (cross-plugin events) |

Wildcard subscriptions: `task.*`, `agent.*`, `<feature>.*` (any subject prefix).

### Agent tool invocation (kandev -> plugin, gRPC `InvokeTool`)

When an agent calls a plugin tool, kandev calls the plugin's `Plugin.InvokeTool` RPC:

```proto
message ToolRequest {
  string tool_call_id = 1;
  string tool_name = 2;
  google.protobuf.Struct input = 3;
  ToolContext context = 4;
}
message ToolContext { string task_id = 1; string agent_instance_id = 2; string session_id = 3; }
message ToolResponse { google.protobuf.Struct output = 1; string error = 2; }
```

Timeout: 30 seconds. Tools available to all agents by default (`tool_scope` for
role/instance restrictions is future work).

### External webhook proxy (external -> kandev -> plugin)

```
POST /api/plugins/{plugin_id}/webhooks/{webhook_key}
```

This remains kandev's one plugin-facing **HTTP** endpoint (external systems like Slack
or Jira cannot speak gRPC). Kandev validates the plugin is active and the webhook key
is declared, converts the HTTP request into a `WebhookRequest` gRPC message, and calls
the plugin's `Plugin.HandleWebhook` RPC:

```proto
message WebhookRequest {
  string webhook_key = 1;
  string method = 2;
  string path = 3;                         // remainder after the key
  string query = 4;
  map<string, string> headers = 5;         // single-valued; multi joined by ", "
  bytes body = 6;
}
message WebhookResponse { int32 status = 1; map<string, string> headers = 2; bytes body = 3; }
```

The `WebhookResponse` is relayed back as the HTTP response. The plugin verifies the
external system's signature (Slack signing secret, GitHub webhook secret, etc.) itself.

### Host gRPC service (plugin -> kandev)

There is no plugin-facing HTTP API for state, secrets, or cross-plugin events. Instead,
kandev implements a `Host` gRPC service and serves it back to the plugin over the
go-plugin broker (the plugin is the gRPC client for this service):

```proto
service Host {
  rpc GetState(GetStateRequest) returns (GetStateResponse);
  rpc SetState(SetStateRequest) returns (SetStateResponse);
  rpc DeleteState(DeleteStateRequest) returns (DeleteStateResponse);
  rpc ListState(ListStateRequest) returns (ListStateResponse);
  rpc RevealSecret(RevealSecretRequest) returns (RevealSecretResponse);
  rpc EmitEvent(EmitEventRequest) returns (EmitEventResponse);
}
```

- `GetState`/`SetState`/`DeleteState`/`ListState` operate on `plugin_state`, scoped by
  `scope` (`instance`, `workspace`, `task`, `agent`) and `scope_id`. Plugins cannot
  read other plugins' state — the Host service instance handed to a plugin's
  subprocess is bound to that plugin's own ID at spawn time, so there is no
  plugin-supplied ID to spoof.
- `RevealSecret(ref)` resolves a secret reference through kandev's `internal/secrets/`
  package. Requires `capabilities.secrets: true`.
- `EmitEvent(event_name, payload)` publishes `plugin.<plugin_id>.<event_name>` on the
  internal event bus for delivery to subscribers (replaces the old
  `POST /api/plugins/{plugin_id}/events/emit` HTTP endpoint).

Every Host RPC is capability-gated: `GetState`/`SetState`/`DeleteState`/`ListState`
check `capabilities.state`, `RevealSecret` checks `capabilities.secrets`, and each
Host data API read RPC checks `capabilities.api_read` for its resource (see "Host
data API" below) — all before the handler runs, returning gRPC status
`PermissionDenied` with message `capability '<name>' not declared` on a miss.
`EmitEvent` is ungated (no boolean capability applies). The Host data API write
RPCs are not implemented yet, so they return gRPC `Unimplemented` unconditionally
and never reach an `api_write` capability check (see "Write phase (deferred)").

### Host data API (plugin -> kandev, gRPC)

Plugins read (and, in a later phase, write) kandev's own domain data — tasks,
sessions, workspaces, workflows, agent profiles, repositories, comments — over the
same capability-gated Host gRPC channel they use for state and secrets, instead of
opening the kandev database file. The wire contract is the `kandev.plugin.v1`
Host data RPCs; DTOs are hand-mapped, versioned proto messages, never internal
domain structs. See [ADR 0043](../../decisions/0043-plugin-host-data-api.md) and
`docs/plans/plugins/HOST-DATA-API.proto`.

**Readable resources (v1).** Each is gated by an `api_read:<resource>` capability:

| RPC | Capability | Returns |
|---|---|---|
| `ListTasks` / `GetTask` | `api_read:tasks` | Tasks (id, workspace, workflow, title, description, state, priority, timestamps, parent, identifier, repositories, metadata) |
| `ListWorkspaces` | `api_read:workspaces` | Workspaces (id, name, owner, defaults, timestamps) |
| `ListWorkflows` | `api_read:workflows` | Workflows for a workspace |
| `ListWorkflowSteps` | `api_read:workflows` | Steps for a workflow (id, name, position, stage type) |
| `ListAgentProfiles` | `api_read:agent_profiles` | Agent profiles (id, agent id, display name, model, mode) |
| `ListRepositories` | `api_read:repositories` | Repositories for a workspace (id, name, default branch) |
| `ListSessions` | `api_read:sessions` | Session identity + agent context (id, task, agent profile, resolved display name + model, `acp_session_id`, state, timestamps) |
| `ListSessionCodeStats` | `api_read:sessions` | **Computed** per-session code metrics: committed lines added/deleted, peak pending-diff lines added/deleted |

`acp_session_id` on a session is the external usage-attribution join key (e.g.
`tokscale`): kandev exposes the session identity and code stats but stays out of
the token business. `SessionCodeStats` is a deliberately computed shape — the
aggregate the agent-stats plugin previously re-derived by hand from
`task_session_commits` and `task_session_git_snapshots` — so plugins never touch
those raw rows.

**Write phase (deferred).** `CreateTask`, `UpdateTask`, and `CreateComment` are
specified in the proto but not implemented in this phase. When added, each is
gated by `api_write:<resource>` (`api_write:tasks`, `api_write:comments`), routes
through the task service methods that publish `task.*` events (never a
repository), and the server stamps `source = "plugin:<id>"` on the created
row/comment — a plugin cannot set provenance itself. Declaring an `api_write`
capability has no effect until the write RPCs ship.

**Conventions.**

- **Pagination** is opaque-cursor based: a request carries `Page{limit, cursor}`
  and a list response carries `PageInfo{next_cursor, has_more}`. `limit: 0` means
  the server default; the server caps the maximum. An empty cursor is the first
  page; echoing `next_cursor` continues. Plugins MUST NOT interpret cursor
  contents.
- **Timestamps** are RFC3339 strings.
- **Nullable** string fields use proto `optional`, so absent (NULL) is
  distinguishable from empty.
- **Scoping (v1):** reads are global to the kandev instance (plugins are
  instance-global; see "Permissions"). Filters (`workspace_ids`, `task_ids`,
  `states`) narrow results but do not confer or restrict visibility. A
  server-side scoping hook is reserved for a future per-plugin/per-user
  restriction without a contract change.
- **Ephemeral tasks** (quick-chat) are excluded from `ListTasks` unless the
  request sets `include_ephemeral`.

Every Host data RPC is capability-gated the same way as state/secrets: an
undeclared capability returns gRPC `PermissionDenied` with message
`capability '<name>' not declared` before the handler runs. Because the Host
service instance is bound to the plugin's own ID at spawn time, the check
evaluates directly against that plugin's installed manifest.

## Filesystem sideloading & sync

Besides the URL/upload install pipeline, an operator with shell access to the host
can place plugin content directly under the plugins directory
(`~/.kandev/plugins/`) without going through `POST /api/plugins/install`. `POST
/api/plugins/sync` (and the Settings > Plugins **Sync** button, which calls it and
refreshes the list) reconciles the registry with whatever is actually on disk:

1. **Directory sideloads.** For every `<pluginsDir>/<id>/<version>/manifest.yaml`
   found with no existing `{id}.yml` record, kandev parses and validates the
   manifest, requires it to be runtime-managed (`runtime.type: binary`) and its `id`
   field to match the `<id>` directory name, and registers it — always with status
   **`disabled`**, never `active`. Sideloads are unverified (no checksum, no
   integrity gate the URL/upload pipeline runs) and are never auto-spawned; an
   operator must explicitly enable one after inspecting it. If more than one version
   directory exists for the same unregistered id, the lexically greatest version is
   registered and the others are reported as skipped, not registered.
2. **Dropped tarballs.** Every `*.tar.gz` file sitting directly in the plugins
   directory is run through the same verified install pipeline `POST
   /api/plugins/install` uses (checksum verification, manifest validation, platform
   executable check, extraction, spawn, activate). On success the tarball file is
   deleted. On a validation failure the file is left in place (not retried
   automatically) and the failure is reported.
3. **Missing installs.** Every registered record whose `install_path` no longer
   exists on disk (deleted out from under kandev) is stopped, if its process is
   running, and transitioned to status `error`.

`POST /api/plugins/sync` returns a `SyncResult`: which plugin ids were newly
sideloaded (`added`), which were installed from a dropped tarball (`installed`),
which were marked missing (`missing`), and a list of per-item `errors` (path +
reason) for anything rejected or skipped along the way. A single item's failure
never aborts the rest of the scan. Concurrent sync calls are serialized so an
operator double-clicking Sync (or a sync racing the boot scan) cannot double-install
the same dropped tarball.

At **boot**, kandev runs only the directory-sideload and missing-install steps
(never the tarball-install step) as part of resuming previously-active plugins —
conservative by design: starting up should never itself spawn a binary an operator
has not explicitly approved via install or Sync. What the boot scan found is logged;
an operator triggers the full sync (including tarball installs) explicitly via the
Sync button or the API.

## Frontend plugin runtime (native JS UI plugins)

Plugins may extend the SPA with **native** React UI — routes, nav items, slot
components, and WebSocket handlers that run inside the kandev frontend (the
Mattermost-webapp model), not iframes. The full contract lives in
`docs/plans/plugins/PLUGIN-API.md`; summary:

- **Manifest:** a plugin declares `ui.bundle` (a path inside the extracted package,
  e.g. `ui/bundle.js`) and optional `ui.styles`.
- **Bundle delivery:** kandev serves the bundle at `GET /api/plugins/{id}/bundle`
  (and any assets under `GET /api/plugins/{id}/ui/*`) directly from the extracted
  package directory on local disk, forcing `Content-Type: text/javascript` and
  stripping frame-blocking headers. No reverse proxy and no live upstream process are
  involved in serving these assets — the plugin subprocess only needs to be running
  to serve gRPC calls, not the UI bundle.
- **Boot payload:** the SPA boot payload carries
  `plugins: [{ id, name, bundleUrl, styleUrls }]` for every **active** plugin that
  declares a bundle (gated on the `plugins` feature flag).
- **Loading:** on boot (and on runtime enable), the frontend host dynamically
  `import()`s each `bundleUrl`. The bundle calls
  `window.registerKandevPlugin(id, { initialize(registry, host), destroy? })`.
  `host` shares the kandev React instance, the app store, a plugin-scoped
  `api.fetch`, a curated `@kandev/ui` subset, and the theme — so a plugin can build
  a page indistinguishable from first-party UI (e.g. a native `/jira` page).
- **Registry surface:** `registerRoute(path, C)`, `registerNavItem(item)`,
  `registerSettingsRoute(path, C)`, `registerComponent(slot, C)` (slots:
  `task-sidebar`, `settings-nav`, ...), `registerWsHandler(action, fn)`.
  Registrations are namespaced per plugin and bulk-revoked on disable/uninstall.
- **Isolation (v1):** only active, operator-installed plugins load; a failing
  bundle/`initialize` is caught and never breaks boot; slot components render behind
  error boundaries. Plugin JS otherwise runs with full in-origin store access —
  hard sandboxing (workers/realms) is future work (see Out of scope).

## State machine

```
registered -> active -> disabled -> uninstalled
                 |          |
                 +-> error -+
```

| State | Meaning |
|---|---|
| `registered` | Package extracted and record written; go-plugin spawn/handshake pending or in flight |
| `active` | Handshake succeeded and health (`Ping`) passes; events delivered, tools available, webhooks proxied |
| `error` | 3 consecutive `Ping` failures (30s interval, injectable), or the subprocess crashed and restart attempts (backoff, max 5) are exhausted. Events buffered (ring buffer, 100 events, 5-minute TTL). Tools removed from agent tool set. Webhooks return 503 |
| `disabled` | Operator explicitly disabled. Subprocess stopped. No events, no tools, no webhooks. State and config preserved |
| `uninstalled` | Subprocess stopped, package/record/state deleted (no grace period in v1) |

Health monitoring: kandev's go-plugin client calls `Ping()` on the plugin every 30
seconds (injectable). 3 consecutive failures -> `error` + inbox item + restart attempt
with backoff. A subprocess crash (unexpected process exit) triggers an immediate
restart with backoff (max 5 attempts, then `error`). Next successful handshake/`Ping`
-> `active`, queued events delivered in order.

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

## Failure modes

- **3 consecutive `Ping` failures (90s), or crash with restart attempts exhausted
  (max 5, backoff)**: status -> `error`. Events buffered (100, 5min TTL). Tools hidden
  from agent tool set. Webhooks return 503. Inbox item created.
- **Buffer overflows (>100 events or >5min)**: oldest events dropped and logged.
- **Plugin returns a gRPC error (or times out) on `DeliverEvent`**: retry up to 3 times
  with exponential backoff (5s, 15s, 45s). After exhaustion, event is logged as failed
  and dropped.
- **Tool call timeout (>30s)**: agent receives a timeout error.
- **External webhook hits a disabled/error plugin**: kandev returns 503.
- **Undeclared capability access attempt on a Host RPC**: gRPC `PermissionDenied` with
  a message naming the missing capability.
- **Checksum mismatch or unresolvable host-platform executable at install time**:
  install is rejected before any code runs.

## Persistence guarantees

- Plugin installation records (`id`, `version`, `install_path`, capabilities, status)
  persist to disk under `~/.kandev/plugins/<id>/` and survive backend restarts.
- Extracted plugin packages persist at `~/.kandev/plugins/<id>/<version>/` until
  uninstall.
- Plugin state in SQLite survives restarts.
- Event delivery buffer is in-memory; events in the buffer do not survive a backend
  restart.
- There are no plugin credentials to persist or lose — auth is re-derived from the
  spawn relationship on every process launch.

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

- **GIVEN** a Jira sync plugin that registered the `jira_search` agent tool, **WHEN**
  an agent calls `jira_search` with query "auth migration", **THEN** kandev calls the
  plugin's `InvokeTool` gRPC method. The plugin queries Jira, returns results as the
  `ToolResponse`. The agent receives the search results as the tool output.

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
  and buffered events are delivered in order.

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

- **GIVEN** a plugin whose manifest declares `api_write: ["tasks"]` in this phase,
  **WHEN** it calls `CreateTask`, **THEN** kandev returns gRPC status `Unimplemented`
  (write RPCs are deferred), and declaring the capability has no other effect.

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
- **In-process backend plugins.** Plugin *backends* remain out-of-process — no Go
  plugin loading via `plugin.Open`, no WASM, no shared-memory communication. (This is
  distinct from the frontend bundle, which does load into the SPA.)
- **Plugin marketplace or registry.** No central catalog, no one-click install, no
  automatic discovery. Install-by-URL/upload is a manual, single-plugin action; a
  searchable catalog is a later phase.
- **Mandatory package signing.** `checksums.txt.sig` verification is supported when
  present, but signing is optional in v1 — an unsigned package installs with a warning
  rather than being rejected. Requiring signatures is future work.
- **Agent-session tool wiring.** Declared plugin tools are listed
  (`GET /api/plugins/tools`) and invocable via the tool client, but are not yet
  injected into live agent tool sets.
- **Hot reload.** Upgrading a plugin requires a new install (new version directory);
  there is no in-place manifest or binary swap on a running process.
- **Multi-instance plugins.** Each plugin ID maps to exactly one supervised subprocess.
- **Rate limiting.** No per-plugin rate limits in v1. Misbehaving plugins can be disabled manually.
- **Plugin database namespaces.** Plugins do not get their own SQLite schemas. KV state is sufficient for v1.
- **Host data API write RPCs.** `CreateTask`, `UpdateTask`, and `CreateComment` are
  specified but deferred to a later phase; only the read RPCs ship in v1. See "Host
  data API".
- **Per-session code-stats precomputation.** `SessionCodeStats` is computed on
  demand per request in v1; a materialized or cached aggregation is future work.
- **Workspace-scoped plugin data access.** v1 reads are global to the instance with
  a reserved scoping hook; per-plugin or per-user workspace restriction is future
  work (see ADR 0043 open decisions).
