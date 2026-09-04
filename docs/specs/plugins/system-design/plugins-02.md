---
status: draft
system: plugins
requirements:
  - REQ-PLUGINS-PLUGINS-001
created: 2026-04-26
owners:
  - cfl
---
# Plugin System System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-PLUGINS-PLUGINS-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLUGINS-PLUGINS-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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

`POST /api/plugins/{id}/actions/{key}` is the only browser action route. Kandev
authenticates the caller, rejects inactive plugins and undeclared keys, and authorizes
each referenced workspace, task, or repository. It derives a task's workspace
server-side. `access` defaults to `authenticated`. `admin` requires
`min_kandev_version: "0.91.1"` or later; validation rejects older/missing
minimums. Non-admins are rejected before envelope parsing or plugin invocation.
Verified context is separate from untrusted JSON; calls are bounded/cancellable and
response headers allowlisted. Public `/webhooks/` callbacks cannot serve browser
actions. A task action may select one attached persisted repository; other IDs are
rejected, and the accepted repository is separate in `VerifiedActionContext`.
Canonical field: `scope`; `resource_scope` is a read-only prerelease alias.

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
  [ADR 0048](../../../decisions/0048-plugin-host-utility-agent-invoke.md). The plugin does not select
  an execution profile directly; the selected utility agent resolves its effective profile and
  complete launch/permission policy according to
  [Profile-backed Utility Agents](../../agents/requirements/utility-agent-profiles.md).

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
domain structs. See [ADR 0043](../../../decisions/0043-plugin-host-data-api.md) and
`docs/plans/plugins/HOST-DATA-API.proto`.

**Readable resources (v1).** Each is gated by an `api_read:<resource>` capability:

| RPC                     | Capability                   | Returns                                                                                                                                                                                                                                           |
| ----------------------- | ---------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ListTasks` / `GetTask` | `api_read:tasks`             | Tasks (id, workspace, workflow, title, description, state, priority, timestamps, parent, identifier, repositories, metadata)                                                                                                                      |
| `ListWorkspaces`        | `api_read:workspaces`        | Workspaces (id, name, owner, defaults, timestamps)                                                                                                                                                                                                |
| `ListWorkflows`         | `api_read:workflows`         | Workflows for a workspace                                                                                                                                                                                                                         |
| `ListWorkflowSteps`     | `api_read:workflows`         | Steps for a workflow (`id`, `workflow_id`, `name`, `position`, `stage_type`, `color`, `is_start_step`, `wip_limit`, `agent_profile_id`, `on_enter_action_types`)                                                                                   |
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

`on_enter_action_types` on a step lists the on-enter action *types* configured
for that step (e.g. `auto_start_agent`, `queue_run`), never the action
*config* — target task ids, payloads, and profile ids stay server-side. A type
appearing in this list means it is configured, not a guarantee of when or
whether it fires. Plugins must ignore action types they do not recognize.

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

### Generic Host boundary

Manifest declaration remains the package-authored maximum capability set. A
capability that needs workspace approval also uses the current
`(installation, workspace)` approval and immutable Human-reserved policy. The Host
enforces this intersection. The plugin cannot supply an installation identity or
grant itself authority.

The generic service and capability-family boundaries, domain ownership, versioning,
and migration rules are recorded in
[ADR-2026-08-31-generic-plugin-host-boundary](../../../decisions/2026-08-31-generic-plugin-host-boundary.md).
Host RPC and global MCP adapters can use different request DTOs, but both call the
same named domain command or query. Neither adapter duplicates authorization,
ownership, idempotency, transition, lifecycle, relation, or audit invariants.

The Host can provide a connection-bound capability context for discovery. The
context reports supported capability families and approved workspace scope. It does
not grant authority. The Host checks current approval for every request.

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
