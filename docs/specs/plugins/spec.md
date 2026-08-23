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
  declared webhooks and authenticated actions, repository-provider and reference-source
  ownership, config schema, and optional UI bundle.
- Plugins SHALL receive events, expose proxied external webhook
  endpoints, and read/write a plugin-scoped KV state — all over gRPC.
- Plugins are distributed as a signed-or-unsigned release **tarball** and installed
  either by **URL** (kandev downloads it) or by **manual upload** (multipart file).
  There is no manifest-paste registration step.
- The Settings > Plugins install dialog SHALL show the primary Install action as
  busy while an install is in flight, including an animated loading indicator and
  an installing label, while keeping the action disabled until the pipeline settles.
- Capability-based access control: a plugin can only call Host RPCs it declared in its
  manifest; undeclared capabilities are rejected with a gRPC `PermissionDenied` status.
- **Kandev owns the plugin process lifecycle**: it extracts the package, spawns the
  binary, performs the go-plugin handshake, health-checks it (`Ping`), and restarts it
  on crash or health-check failure. Operators no longer run or manage plugin processes
  themselves. The remote/self-hosted tier (`base_url` registration of an
  operator-run process kandev never spawns) is removed; see "Out of scope".
- Plugin repository providers, task actions, and review panels register through
  revocable generic frontend contracts. A provider plugin can participate in native
  task creation, Link menus, desktop/mobile review surfaces, and composer `#` search
  without a host-specific provider branch.
- The independently consumable, runtime-free `@kandev/plugin-sdk` package is the
  frontend author contract. Official plugins use its typed `host.context` reads and
  named UI/provider contracts instead of importing or copying private Kandev store,
  React, Zustand, or `@/` application types.
- First-party-parity code-host dashboards use host-owned provider-neutral list,
  toolbar, scope, task-menu, and linked-task primitives exposed through `host.ui`.
  Plugins provide normalized data and callbacks; task presets open the native
  `TaskCreateDialog`, while review remains owned by the task review-provider surface.
- Native UI plugins register bounded, plugin-owned translation catalogs and consume them through a
  host-scoped imperative/reactive localization API. Locale changes follow Kandev without bundling a
  second i18n runtime, and disable/reload/uninstall removes only that plugin's resources.

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

`manifest.yaml` declares identity, capabilities, webhooks, config schema, and
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
  api_write: ["tasks", "messages"]            # Host data API writes (CreateTask/UpdateTask/SendMessage); live now
  state: true
  secrets: true
  auth: true                                  # establish a login session for an
                                              # external (OIDC/SAML) identity — see ADR 0050
  user_state: true                            # authenticated, per-user host.storage — see
                                              # "Frontend plugin runtime" and ADR
                                              # 2026-08-01-per-user-plugin-storage

actions:
  - key: "connection.save"
    scope: workspace                          # workspace | task | repository
    max_body_bytes: 65536

repository_providers: ["example-repository-provider"]

reference_sources:
  - source: "example_change_requests"
    provider: "example-repository-provider"
    kind: "change_request"
    display_name: "Example change requests"
    kind_label: "Change request"
    order: 100

webhooks:
  - key: "slack-events"
    description: "Slack Events API webhook"
    method: "POST"
    access: public

config_schema:
  type: object
  properties:
    bot_token_secret: { type: string, description: "Secret reference for Slack bot token" }
    default_channel:  { type: string, description: "Default channel for notifications" }
    notify_on_task_created: { type: boolean, default: true }
  required: ["bot_token_secret", "default_channel"]

ui:                                            # Native frontend plugin (see "Frontend plugin runtime")
  bundle: "/ui/bundle.js"                      # root-relative ES module path
  styles: ["/ui/plugin.css"]                   # optional root-relative stylesheets

# Runtime fields managed by kandev (not authored):
status: "active"
version: "1.0.0"
install_path: "~/.kandev/plugins/kandev-plugin-slack/1.0.0"
signed: false
installed_at: "2026-04-26T10:00:00Z"
restart_count: 0
last_error: null
last_error_at: null
```

`last_error` and `last_error_at` are host-managed runtime diagnostics. When a
spawn, handshake, health check, restart, or install-path check moves a plugin to
`error`, kandev stores a single-line diagnostic (bounded to 2048 bytes) and the
UTC timestamp of the failure. Before persistence, kandev redacts credential-like
values (including PATs, bearer tokens, and labeled passwords, tokens, secrets,
or API keys) and replaces the host home path with `~`; arbitrary plugin stdout
and untrusted subprocess details are never persisted verbatim. The diagnostic
is bounded after redaction. A successful handshake and health check clears both
fields. Existing records without these fields load as null.

`capabilities.api_read` / `capabilities.api_write` gate the **Host data API** Host
RPCs (both reads and writes live now) — the vocabulary is a list of
resource names: `tasks`, `sessions`, `messages`, `workspaces`, `workflows`,
`agent_profiles`, `executor_profiles`, `repositories` for `api_read`, plus `tasks` (CreateTask/
UpdateTask) and `messages` (SendMessage) for `api_write`. See "Host data API".

**Declaring data access.** Listing a resource under `api_read` grants the
corresponding Host data reads for that resource only — e.g. `api_read:
["sessions"]` unlocks `Host.Sessions().List(...)` and
`Host.Sessions().CodeStats(...)` (backed by `ListSessions` /
`ListSessionCodeStats`) but not `Host.Tasks()`. A resource left off the list
still resolves to a reader/accessor (no nil pointer), but every method on it
returns gRPC `PermissionDenied` with message `capability 'api_read:<resource>'
not declared`. Writes work the same way under `api_write` (message
`capability 'api_write:<resource>' not declared`), and reads and writes gate
independently on the same resource — a plugin may declare `api_read:tasks`
without `api_write:tasks`, or vice versa (see "Host data API writes").

`actions` declare the only browser-invocable plugin operations. Each key is unique per
plugin, has one resource scope (`workspace`, `task`, or `repository`), and supplies a
bounded request-body maximum. `repository_providers` declares stable IDs the plugin may
register in the frontend repository-provider registry; one enabled plugin owns one
provider ID. A provider that supports the native task branch picker declares the
workspace-scoped `repositories.branches` action. Kandev invokes it server-to-server with
the persisted repository identity and accepts only a bounded normalized branch list;
browser input cannot select the provider or repository for that invocation.
`reference_sources` declares plugin-owned composer sources. Source names
are unique, a descriptor cannot claim another provider, and all entries are removed when
the plugin disables. See ADRs
[2026-07-31-authenticated-plugin-actions](../../decisions/2026-07-31-authenticated-plugin-actions.md)
and
[2026-07-31-plugin-repository-provider-extensions](../../decisions/2026-07-31-plugin-repository-provider-extensions.md).

**External login (`auth`).** An `auth`-capable plugin can log a visitor in
against an external IdP (OIDC/SAML): its webhook (the callback / SAML ACS)
validates the token, then asserts the identity to kandev via the reserved
`X-Kandev-Auth-Login` response header (`{provider, subject, email,
display_name}`). The host maps it to a user (link-by-email or just-in-time
member provisioning), mints the session, and sets the session cookie (name
derived from the request host) itself — the plugin never receives the raw
token. Requires auth enabled; new
users are members, and the host never creates an admin nor auto-links to an
existing admin account. The plugin **must** only assert IdP-verified emails (an
unverified email claim is an account-takeover vector the host cannot detect).
This is the highest-privilege capability; grant it only to trusted plugins.
See [ADR 0050](../../decisions/0050-plugin-external-auth-capability.md).

### Install pipeline

There is no manifest-paste registration step. A plugin is installed from a URL or an
uploaded tarball:

1. An administrator calls `POST /api/plugins/install` with JSON `{"url": "..."}`
   (kandev downloads the tarball) or a multipart `package` field (direct upload).
   Auth-disabled single-user instances retain their synthetic administrator.
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
   (restart retried with backoff; see "State machine"). The failure diagnostic is
   persisted on the record so the operator can see why activation failed and
   retry with **Enable**.

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

Install is administrator-only when authentication is enabled. Auth-disabled
single-user instances retain the synthetic administrator used by other system
settings.

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
GET    /api/plugins/{id}/bundle       # Frontend bundle, served from the extracted package dir
GET    /api/plugins/{id}/ui/*         # Frontend bundle assets, served from the extracted package dir
POST   /api/plugins/{id}/actions/{key} # Authenticated declared browser action
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

### Authenticated plugin actions (browser -> kandev -> plugin)

`POST /api/plugins/{id}/actions/{key}` is the only browser action route. Kandev first
applies ordinary HTTP authentication, then rejects inactive plugins or undeclared keys
and authorizes every referenced workspace, task, or repository. It derives a task's
workspace relation server-side, passes verified actor/resource context separately from
bounded untrusted JSON, invokes `Plugin.HandleAction` with a hard timeout and
cancellation, and relays only an allowlisted response-header set. Provider callback
routes under `/webhooks/` stay public and must not serve authenticated browser actions.
Task-scoped actions require a verified task and may optionally select one persisted
repository attached to that task; Kandev rejects unattached repository IDs and passes
the accepted repository separately in `VerifiedActionContext`.
The manifest's canonical action field is `scope`; `resource_scope` remains a read-only
compatibility alias for packages produced during the prerelease contract rollout.

### Dynamic composer reference sources (plugin -> kandev)

An active manifest `reference_sources` descriptor registers a plugin bridge in
`internal/mentions`. Kandev calls `Plugin.SearchEntityReferences` with verified
workspace, plain query, and bounded limit; plugin candidates are untrusted. The host
injects descriptor identity and constructs canonical references. Kandev calls
`Plugin.AuthorizeEntityReference` at search and again at message submission with
purpose `search` or `submission`. A disabled plugin, invalid result, stale selection,
or denial fails closed; no stale reference metadata enters the message queue.

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

| Category                       | Events                                                                                                                                                                                                            |
| ------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Tasks                          | `task.created`, `task.updated`, `task.state_changed`, `task.deleted`, `task.moved`                                                                                                                                |
| Sessions                       | `task_session.state_changed`, `turn.started`, `turn.completed`                                                                                                                                                    |
| Agents                         | `agent.started`, `agent.completed`, `agent.failed`, `agent.stopped`                                                                                                                                               |
| Other feature areas (examples) | Any additional subjects emitted by feature areas such as office/agents (e.g. `office.comment.created`, `office.approval.created`) — plugins may subscribe to these, but the plugin system does not depend on them |
| GitHub                         | `github.pr_state_changed`, `github.pr_feedback`, `github.new_issue`                                                                                                                                               |
| Plugin                         | `plugin.<plugin_id>.<name>` (cross-plugin events)                                                                                                                                                                 |

Wildcard subscriptions: `task.*`, `agent.*`, `<feature>.*` (any subject prefix).

### External webhook proxy (external -> kandev -> plugin)

```
POST /api/plugins/{plugin_id}/webhooks/{webhook_key}
```

This is kandev's only plugin-facing **HTTP** endpoint. Anonymous callers need
`access: public`; otherwise kandev returns 401 without revealing plugin/key. Kandev
validates plugin/key, strips session/PAT, converts to `WebhookRequest`, and calls
`Plugin.HandleWebhook`:

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

### Authenticated per-user storage API (browser -> kandev, `host.storage`)

```text
GET    /api/plugins/{id}/user-state/{scope}/{scopeId}          # list, ordered by key
GET    /api/plugins/{id}/user-state/{scope}/{scopeId}/{key}
PUT    /api/plugins/{id}/user-state/{scope}/{scopeId}/{key}     # body: {value, writerId?, ifUnmodifiedSince?}
DELETE /api/plugins/{id}/user-state/{scope}/{scopeId}/{key}
```

Unlike every other plugin HTTP surface, this one is reachable directly from an
authenticated browser session (session cookie or PAT) — it is **not** in
`httpmw`'s public-path allowlist, and it never touches the plugin's gRPC
subprocess. Identity comes from `authn.FromGin`; every read/write is scoped to
that user via a `plugin_user_state` row keyed
`(plugin_id, user_id, scope, scope_id, state_key)`, which is a separate table
from the plugin-owned `plugin_state` (no proto change, no migration between the
two). Requires the plugin manifest to declare `capabilities.user_state: true`
(`403` otherwise); an unknown/disabled plugin returns `404`; a cross-user `GET`
returns `404` (never leaks that the key exists for someone else). `scope` must
be one of `instance|workspace|task|session|repository`; `scopeId`/`key` must
match `[A-Za-z0-9][A-Za-z0-9._:-]{0,127}`; the body is capped (`413` over the
limit). `PUT` accepts an optional `ifUnmodifiedSince`, compared against the
stored row's `updated_at` — a conflicting write returns `409` and leaves the
stored value unchanged (optimistic concurrency; see ADR
2026-08-01-per-user-plugin-storage). Uninstalling a plugin deletes its
`plugin_user_state` rows for every user, not just one.

A successful `PUT`/`DELETE` publishes the `plugin.user-state.updated` bus event,
routed by the existing `UserEventBroadcaster` (the same user-scoped fan-out
`user.settings.updated` uses) to only the writing user's own WS connections. The
payload carries `{ pluginId, scope, scopeId, key, updatedAt, writerId, deleted }`
— keys only, never the value.

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
  rpc InvokeUtilityAgent(InvokeUtilityAgentRequest) returns (InvokeUtilityAgentResponse);
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
- `InvokeUtilityAgent(prompt)` runs a one-shot, non-interactive completion using
  the utility agent selected in the plugin's `utility_agent` config field and
  returns its text. Plugins declaring `capabilities.agent_invoke: true` must
  declare that field in `config_schema` with `type: string` and
  `format: utility-agent`; Settings > Plugins then renders a picker containing
  the configured built-in and custom utility agents. The picker displays the
  agent name and persists its stable ID. It reuses kandev's
  sessionless host-utility inference tier (ADR 0002) — no task, session, or
  workspace — so a plugin can delegate a lightweight LLM step without holding a
  provider API key. Returns gRPC `FailedPrecondition` when no utility agent is
  configured, selected agent was deleted, or it is disabled. See
  [ADR 0048](../../decisions/0048-plugin-host-utility-agent-invoke.md). The plugin does not select
  an execution profile directly; the selected utility agent resolves its effective profile and
  complete launch/permission policy according to
  [Profile-backed Utility Agents](../agents/utility-agent-profiles.md).

Every Host RPC is capability-gated: `GetState`/`SetState`/`DeleteState`/`ListState`
check `capabilities.state`, `RevealSecret` checks `capabilities.secrets`,
`InvokeUtilityAgent` checks `capabilities.agent_invoke`, and each Host data API
read RPC checks `capabilities.api_read` for its resource, and each Host data API
write RPC checks `capabilities.api_write` for its resource (see "Host data API"
below) — all before the handler runs, returning gRPC status `PermissionDenied`
with message `capability '<name>' not declared` on a miss. `EmitEvent` is
ungated (no boolean capability applies).

### Host data API (plugin -> kandev, gRPC)

Plugins read and write kandev's own domain data — tasks,
sessions, workspaces, workflows, agent profiles, repositories, messages — over the
same capability-gated Host gRPC channel they use for state and secrets, instead of
opening the kandev database file. The wire contract is the `kandev.plugin.v1`
Host data RPCs; DTOs are hand-mapped, versioned proto messages, never internal
domain structs. See [ADR 0043](../../decisions/0043-plugin-host-data-api.md) and
`docs/plans/plugins/HOST-DATA-API.proto`.

**Readable resources (v1).** Each is gated by an `api_read:<resource>` capability:

| RPC                     | Capability                   | Returns                                                                                                                                                                                                                                           |
| ----------------------- | ---------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ListTasks` / `GetTask` | `api_read:tasks`             | Tasks (id, workspace, workflow, title, description, state, priority, timestamps, parent, identifier, repositories, metadata)                                                                                                                      |
| `ListWorkspaces`        | `api_read:workspaces`        | Workspaces (id, name, owner, defaults, timestamps)                                                                                                                                                                                                |
| `ListWorkflows`         | `api_read:workflows`         | Workflows for a workspace                                                                                                                                                                                                                         |
| `ListWorkflowSteps`     | `api_read:workflows`         | Steps for a workflow (id, name, position, stage type)                                                                                                                                                                                             |
| `ListAgentProfiles`     | `api_read:agent_profiles`    | Agent profiles (id, agent id, display name, model, mode)                                                                                                                                                                                          |
| `ListExecutorProfiles`  | `api_read:executor_profiles` | Executor profiles (id, display name, executor type)                                                                                                                                                                                               |
| `ListRepositories`      | `api_read:repositories`      | Repositories for a workspace (id, name, default branch)                                                                                                                                                                                           |
| `ListSessions`          | `api_read:sessions`          | Session identity + agent context (id, task, agent profile, resolved display name + model, `acp_session_id`, state, timestamps)                                                                                                                    |
| `ListSessionCodeStats`  | `api_read:sessions`          | **Computed** per-session code metrics: committed lines added/deleted, peak pending-diff lines added/deleted                                                                                                                                       |
| `ListMessages`          | `api_read:messages`          | Historical conversation content (id, session, task, turn, `author_type` (user/agent), `content`, `type`, `created_at`), filterable by session ids, task ids, a `created_at` range (`since`/`until`), and types. See "Conversation content" below. |

`acp_session_id` on a session is the external usage-attribution join key (e.g.
`tokscale`): kandev exposes the session identity and code stats but stays out of
the token business. `SessionCodeStats` is a deliberately computed shape — the
aggregate the agent-stats plugin previously re-derived by hand from
`task_session_commits` and `task_session_git_snapshots` — so plugins never touch
those raw rows.

**Conversation content (`api_read:messages`, ADR 0047).** `ListMessages` reads
historical user/agent message content — the data a "summarize yesterday"
plugin needs, which the `message.added` bus event alone (live-only,
post-install-only) cannot provide. `MessageFilter` narrows by `session_ids`,
`task_ids`, a `created_at` window (`since` inclusive / `until` exclusive,
RFC3339), and message `types`; results are ordered oldest-first with opaque
cursor pagination. `content` is sanitized the same way the event path is —
kandev-injected `<kandev-system>` blocks are stripped via
`sysprompt.StripSystemContent`, and raw system content is never exposed.
`author_type` is only `user` or `agent`: kandev has no `system` author, since
system context is inline markup removed at read time. Reads route through the
task service's `ListMessagesForPlugin` (a single filtered
session/task/time/type query), never a repository or the DB file directly.

**Host data API writes.** `CreateTask`, `UpdateTask`, and `SendMessage` are
implemented, each gated by `api_write:<resource>` and routed through the
first-party service layer (never a repository) so the corresponding events fire
and WS clients update. The server stamps `source = "plugin:<id>"` — a plugin
cannot set provenance itself.

`Host.Tasks().Create`/`Update` (capability `api_write:tasks`) go through
`internal/task/service` so `task.*` events fire. `CreateTask` fills sane
placement defaults when the plugin omits them (single workspace; that
workspace's first workflow — ambiguous → `InvalidArgument`), accepts an optional
`start_agent` that best-effort auto-launches an agent, and requires a title.
`UpdateTask` accepts a conservative field mask — `title` / `description` /
`state` / `workflow_step_id`, each optional (a nil field is left unchanged); a
missing task returns gRPC `NotFound`.

`Host.Messages().Send` (capability `api_write:messages`) delivers a prompt to a
task session through the orchestrator's real delivery path — the same one
`message_task` uses — so the message reaches the agent and drives a turn (it is
_not_ an office comment). It resolves the target session (an explicit
`session_id`, verified to belong to the task, or the task's primary session),
records a user message stamped with the plugin source, and dispatches by session
state: a running session queues the prompt (`status: queued`); an idle/completed
one is prompted, resuming the agent process if it has gone (`sent`); a
never-started one is launched with the prompt as its first turn (`started`). If
dispatch fails after the user message was recorded, the recorded message is
deleted so a failed delivery leaves no durable user message (and a retry can't
stack a duplicate prompt). `task_id` and `text` are required (`InvalidArgument`
otherwise); a session that doesn't belong to the task, or a task with no active
session, returns `NotFound`; a terminal (failed/cancelled) session is rejected.

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
  a sibling feature specified in [marketplace.md](marketplace.md) and built on top of
  this spec's install pipeline.
- **Mandatory package signing.** `checksums.txt.sig` verification is supported when
  present, but signing is optional in v1 — an unsigned package installs with a warning
  rather than being rejected. Requiring signatures is future work.
- **Agent tools in the base v1 runtime.** The original parallel `tools[]` plus
  `InvokeTool` path remains removed. Plugin-contributed agent tools are a
  separate extension specified in [Plugin-Contributed Agent Tools](agent-tools.md)
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
