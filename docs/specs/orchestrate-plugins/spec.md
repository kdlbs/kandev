---
status: draft
created: 2026-04-26
owner: cfl
---

# Orchestrate: Plugin System

## Why

Kandev's orchestrate feature is building a growing list of external integrations directly into the core codebase: GitHub PR sync, Jira ticket browsing, notification providers (Apprise, system notifications), and planned channel types (Telegram, Slack, Discord, email). Each integration adds platform-specific logic -- API clients, webhook handlers, payload formatting, OAuth flows, secret management -- to the Go backend. This creates three problems:

1. **Core bloat.** Every new integration increases the surface area of the backend. The existing GitHub integration is ~40 files; the Jira integration is ~14 files. Adding Slack, Discord, Telegram, Linear, and others at similar scale makes the backend unmaintainable.

2. **Release coupling.** Fixing a bug in the Telegram webhook parser requires a kandev release. Users who don't use Telegram still receive the code. Integration authors (including the kandev team) cannot ship independently of the core release cycle.

3. **Extensibility ceiling.** Users cannot add their own integrations without forking kandev. There is no way to connect kandev to an internal ticketing system, a custom notification target, a proprietary CI/CD pipeline, or a niche chat platform without modifying core code.

A plugin system decouples integrations from core. Plugins run as separate processes, communicate with kandev over HTTP, and extend orchestrate through well-defined capabilities: receiving events, registering agent tools, exposing webhook endpoints, and reading/writing kandev data. The core stays small; the ecosystem grows independently.

## What

### Plugin model

Plugins are **HTTP-based external services**. A plugin is a separate process -- a Go binary, a Docker container, a Node.js service, a Python script, or anything that speaks HTTP. Kandev communicates with plugins via:

- **Webhook delivery**: kandev POSTs event payloads to the plugin's registered webhook URLs.
- **REST API**: the plugin calls kandev's REST API to read/write data.
- **Tool invocation**: when an agent calls a plugin-registered tool, kandev POSTs the tool call to the plugin and returns the response to the agent.

This model fits kandev's architecture: the backend is Go, agents run in containers or as standalone processes, and the system already communicates with agentctl over HTTP. No in-process plugin loading, no child process management, no language-specific SDK required for v1.

### Plugin manifest

Every plugin declares a manifest -- a YAML document describing what the plugin is, what it can do, and where to reach it.

```yaml
# Plugin manifest
id: "kandev-plugin-slack"                    # Unique ID, pattern: ^[a-z0-9][a-z0-9._-]*$
api_version: 1                               # Manifest schema version
version: "1.0.0"                             # Semver
display_name: "Slack Notifications"
description: "Post to Slack on task events, relay messages to agents"
author: "kandev"
categories: ["connector"]                    # connector | automation | tools | analytics

# Where kandev sends requests
base_url: "http://localhost:9100"            # Plugin's HTTP base URL

# Endpoints (all relative to base_url)
endpoints:
  health: "/health"                          # GET, returns 200 if healthy
  events: "/events"                          # POST, receives event webhooks
  tools: "/tools/{tool_name}"               # POST, receives tool invocations
  webhooks: "/webhooks/{webhook_key}"       # POST, receives proxied external webhooks

# What the plugin needs
capabilities:
  events:                                    # Events the plugin wants to receive
    - "task.created"
    - "task.state_changed"
    - "orchestrate.comment.created"
    - "orchestrate.approval.created"
  api_read:                                  # Kandev REST endpoints the plugin reads
    - "tasks"
    - "agents"
    - "projects"
  api_write:                                 # Kandev REST endpoints the plugin writes
    - "tasks"
    - "comments"
  state: true                                # Plugin needs KV state storage
  secrets: true                              # Plugin needs to resolve secret refs

# Agent tools this plugin provides
tools:
  - name: "slack_send_message"
    display_name: "Send Slack Message"
    description: "Posts a message to a Slack channel"
    input_schema:
      type: object
      properties:
        channel: { type: string, description: "Slack channel name or ID" }
        text: { type: string, description: "Message text" }
      required: ["channel", "text"]

# External webhooks this plugin receives (proxied through kandev)
webhooks:
  - key: "slack-events"
    description: "Slack Events API webhook"
    method: "POST"

# Plugin configuration schema (operator-editable)
config_schema:
  type: object
  properties:
    bot_token_secret:
      type: string
      description: "Secret reference for Slack bot token"
    default_channel:
      type: string
      description: "Default channel for notifications"
    notify_on_task_created:
      type: boolean
      default: true
  required: ["bot_token_secret", "default_channel"]
```

### Plugin lifecycle

```
registered -> active -> disabled -> uninstalled
                 |          |
                 +-> error -+
```

**Registered**: the plugin manifest is stored on disk and in memory. The plugin process is expected to be running (operator-managed).

**Active**: kandev has verified the plugin's health endpoint responds. Events are delivered, tools are available, webhooks are proxied.

**Error**: the plugin's health check failed 3 consecutive times (30-second interval). Events are queued with a short buffer (100 events, 5 minutes). If the plugin recovers, queued events are delivered in order. If the buffer fills or times out, events are dropped and logged.

**Disabled**: the operator explicitly disabled the plugin via API or UI. No events, no tools, no webhooks. State and config are preserved.

**Uninstalled**: the plugin is removed. State is deleted after a 24-hour grace period (operator can force-delete immediately).

Kandev does not manage plugin processes. Operators start, stop, restart, and upgrade plugin processes using their own tooling (systemd, Docker Compose, Kubernetes, manual). Kandev only knows the plugin's `base_url` and health endpoint.

### Plugin capabilities

#### 1. Receive event webhooks

Plugins subscribe to kandev events by declaring event patterns in the manifest's `capabilities.events` list. When a matching event occurs, kandev POSTs the event to the plugin's `endpoints.events` URL.

**Event envelope:**

```json
{
  "event_id": "evt_abc123",
  "event_type": "task.state_changed",
  "occurred_at": "2026-04-26T10:30:00Z",
  "workspace_id": "ws_default",
  "payload": {
    "task_id": "task_xyz",
    "old_status": "in_progress",
    "new_status": "done",
    "agent_instance_id": "agent_frontend"
  }
}
```

**Delivery semantics:**
- At-least-once delivery. Plugins must be idempotent (use `event_id` for dedup).
- Events are signed with HMAC-SHA256 (see Security section).
- Timeout: 10 seconds. If the plugin doesn't respond, the event is retried up to 3 times with exponential backoff (5s, 15s, 45s).
- Events are delivered sequentially per plugin (no concurrent delivery to the same plugin). This simplifies plugin implementation at the cost of throughput. Plugins that need parallel processing can queue internally.

**Available event types:**

All events from `internal/events/types.go` are available, plus orchestrate-specific events:

| Category | Events |
|----------|--------|
| Tasks | `task.created`, `task.updated`, `task.state_changed`, `task.deleted`, `task.moved` |
| Sessions | `task_session.state_changed`, `turn.started`, `turn.completed` |
| Agents | `agent.started`, `agent.completed`, `agent.failed`, `agent.stopped` |
| Orchestrate | `orchestrate.agent.created`, `orchestrate.agent.updated`, `orchestrate.agent.status_changed`, `orchestrate.comment.created`, `orchestrate.approval.created`, `orchestrate.approval.resolved`, `orchestrate.cost.recorded`, `orchestrate.wakeup.queued`, `orchestrate.wakeup.processed`, `orchestrate.routine.triggered`, `orchestrate.inbox.item` |
| GitHub | `github.pr_state_changed`, `github.pr_feedback`, `github.new_issue` |
| Plugin | `plugin.<plugin_id>.<name>` (cross-plugin events) |

**Wildcard subscriptions:** Plugins can subscribe to patterns like `task.*`, `orchestrate.*`, or `agent.*`.

#### 2. Register custom agent tools

Plugins declare tools in the manifest. When an agent session starts, kandev includes plugin-provided tools in the agent's available tool set (alongside built-in tools and MCP tools).

**Tool invocation flow:**

```
Agent calls tool "slack_send_message"
  -> kandev identifies tool owner (plugin "kandev-plugin-slack")
  -> POST http://localhost:9100/tools/slack_send_message
     Body: { "tool_call_id": "tc_123", "input": { "channel": "#dev", "text": "Build passed" }, "context": { "task_id": "...", "agent_instance_id": "...", "session_id": "..." } }
  -> Plugin processes, returns: { "output": { "message_id": "M123", "channel": "#dev" } }
  -> kandev returns tool result to agent
```

**Tool scoping:** Tools are available to all agents by default. The manifest can declare `tool_scope` to restrict tools to specific agent roles or instances (future).

**Timeout:** 30 seconds per tool call. Agent receives a timeout error if the plugin doesn't respond.

#### 3. Expose webhook endpoints (inbound from external systems)

Plugins can receive webhooks from external systems (GitHub, Slack, Jira, etc.) through kandev's proxy:

```
External system POSTs to:
  https://kandev.example.com/api/plugins/kandev-plugin-slack/webhooks/slack-events

Kandev proxies to:
  POST http://localhost:9100/webhooks/slack-events
  Headers: X-Plugin-Id, X-Webhook-Key, original headers
  Body: original body (passthrough)
```

Kandev acts as a reverse proxy, adding plugin authentication headers. The plugin is responsible for verifying the external system's signature (Slack signing secret, GitHub webhook secret, etc.).

**Route:** `POST /api/plugins/{plugin_id}/webhooks/{webhook_key}`

#### 4. Read/write plugin-scoped state (KV store)

Each plugin gets an isolated key-value store for persistent state. State is stored in SQLite alongside other kandev runtime data.

**Plugin state table:**

```sql
CREATE TABLE plugin_state (
    id TEXT PRIMARY KEY,
    plugin_id TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT 'instance',    -- instance | workspace | task | agent
    scope_id TEXT,                              -- NULL for instance scope
    state_key TEXT NOT NULL,
    value_json TEXT NOT NULL,                   -- JSON-encoded value
    updated_at TEXT NOT NULL,
    UNIQUE (plugin_id, scope, scope_id, state_key)
);
```

**API (plugin calls kandev):**

```
GET    /api/plugins/{id}/state?scope={scope}&scope_id={id}&key={key}
POST   /api/plugins/{id}/state
       Body: { "scope": "task", "scope_id": "task_xyz", "key": "sync_status", "value": { "synced": true } }
DELETE /api/plugins/{id}/state?scope={scope}&scope_id={id}&key={key}
GET    /api/plugins/{id}/state/list?scope={scope}&scope_id={id}
```

**Scopes:**
- `instance`: global plugin state (OAuth tokens, config cache)
- `workspace`: per-workspace state
- `task`: per-task state (sync mappings, linked entity IDs)
- `agent`: per-agent state

#### 5. Read kandev data via REST API

Plugins authenticate with their API key and can call existing orchestrate REST endpoints:

- `GET /api/orchestrate/tasks` -- list/search tasks
- `GET /api/orchestrate/tasks/{id}` -- get task detail
- `GET /api/orchestrate/tasks/{id}/comments` -- list comments
- `GET /api/orchestrate/agents` -- list agent instances
- `GET /api/orchestrate/agents/{id}` -- get agent detail
- `GET /api/orchestrate/projects` -- list projects
- `GET /api/orchestrate/costs` -- list cost events
- `GET /api/orchestrate/activity` -- list activity log
- `GET /api/orchestrate/approvals` -- list approvals

Access is gated by the plugin's declared `capabilities.api_read` list.

#### 6. Write kandev data via REST API

Plugins can create and modify orchestrate data:

- `POST /api/orchestrate/tasks` -- create a task
- `PATCH /api/orchestrate/tasks/{id}` -- update task fields (status, title, labels, assignee, etc.)
- `POST /api/orchestrate/tasks/{id}/comments` -- post a comment (with `source: "plugin:<plugin_id>"`)
- `POST /api/orchestrate/wakeups` -- queue a wakeup for an agent
- `POST /api/orchestrate/activity` -- log an activity entry

Access is gated by `capabilities.api_write`. Comments posted by plugins include `source` metadata so the system can identify their origin (important for feedback loop prevention in bidirectional sync plugins).

### Plugin APIs (what kandev exposes)

#### Event webhook delivery

Kandev delivers events via HTTP POST to the plugin's events endpoint.

**Request:**

```
POST {plugin.base_url}{plugin.endpoints.events}
Content-Type: application/json
X-Kandev-Plugin-Id: kandev-plugin-slack
X-Kandev-Signature: sha256=<hmac>
X-Kandev-Delivery-Id: del_abc123
X-Kandev-Event-Type: task.state_changed

{event envelope JSON}
```

**HMAC signature:** `HMAC-SHA256(webhook_secret, request_body)`. The `webhook_secret` is generated at plugin registration and shared with the plugin.

**Expected response:** `200 OK` (body ignored). Any non-2xx response triggers retry.

#### Plugin state API

```
GET    /api/plugins/{plugin_id}/state/{key}?scope={scope}&scope_id={id}
POST   /api/plugins/{plugin_id}/state
DELETE /api/plugins/{plugin_id}/state/{key}?scope={scope}&scope_id={id}
GET    /api/plugins/{plugin_id}/state?scope={scope}&scope_id={id}    # list keys
```

Authenticated with the plugin's API key. Scoped to the requesting plugin (a plugin cannot read another plugin's state).

#### Plugin secrets API

Plugins store secret references (not raw secrets) in their config. To resolve a reference:

```
GET /api/plugins/{plugin_id}/secrets/{ref}
```

Returns the resolved secret value. Refs point to kandev's secret store (`internal/secrets/`). The plugin's manifest must declare `capabilities.secrets: true`.

#### Plugin registration API

```
POST   /api/plugins/register          # Register a new plugin (manifest YAML in body)
GET    /api/plugins                    # List registered plugins
GET    /api/plugins/{id}              # Get plugin detail
PATCH  /api/plugins/{id}              # Update plugin config
DELETE /api/plugins/{id}              # Uninstall plugin
POST   /api/plugins/{id}/enable       # Enable a disabled plugin
POST   /api/plugins/{id}/disable      # Disable a plugin
```

#### Cross-plugin events

Plugins can emit custom events by POSTing to:

```
POST /api/plugins/{plugin_id}/events/emit
Body: { "event_name": "sync-completed", "payload": { "count": 5 } }
```

This is published as `plugin.<plugin_id>.sync-completed` and delivered to any plugin subscribed to that pattern.

### Plugin registration

#### Registration flow

1. Operator starts the plugin process (their responsibility).
2. Operator calls `POST /api/plugins/register` with the plugin manifest.
3. Kandev validates the manifest schema, checks for ID conflicts.
4. Kandev generates a `webhook_secret` (for HMAC signing) and an `api_key` (for plugin-to-kandev auth).
5. Kandev stores the plugin record on disk at `~/.kandev/plugins/{plugin_id}.yml` and in the in-memory plugin registry.
6. Kandev calls the plugin's health endpoint to verify reachability.
7. If healthy, status = `active`. If unreachable, status = `registered` (will retry on next health check cycle).
8. Kandev returns the `api_key` and `webhook_secret` to the operator. These are shown once and must be configured in the plugin.

#### Filesystem storage

Plugin registrations are stored at `~/.kandev/plugins/` (not inside a workspace -- plugins are global to the kandev instance):

```yaml
# ~/.kandev/plugins/kandev-plugin-slack.yml
id: "kandev-plugin-slack"
api_version: 1
version: "1.0.0"
display_name: "Slack Notifications"
description: "Post to Slack on task events"
author: "kandev"
categories: ["connector"]
base_url: "http://localhost:9100"
endpoints:
  health: "/health"
  events: "/events"
  tools: "/tools/{tool_name}"
  webhooks: "/webhooks/{webhook_key}"
capabilities:
  events: ["task.created", "task.state_changed", "orchestrate.comment.created"]
  api_read: ["tasks", "agents", "projects"]
  api_write: ["tasks", "comments"]
  state: true
  secrets: true
tools:
  - name: "slack_send_message"
    display_name: "Send Slack Message"
    description: "Posts a message to a Slack channel"
    input_schema: { ... }
webhooks:
  - key: "slack-events"
    description: "Slack Events API webhook"
    method: "POST"
config_schema: { ... }
# Runtime fields (managed by kandev, not user-editable)
status: "active"
api_key_hash: "<bcrypt hash>"
webhook_secret_hash: "<bcrypt hash>"
registered_at: "2026-04-26T10:00:00Z"
last_health_check: "2026-04-26T10:30:00Z"
```

The `api_key` and `webhook_secret` are stored as bcrypt hashes. The cleartext values are returned once at registration and never stored by kandev.

#### Plugin configuration

Operators configure plugins by calling `PATCH /api/plugins/{id}` with a config payload that conforms to the plugin's `config_schema`. Config is stored in `~/.kandev/plugins/{plugin_id}.config.yml`:

```yaml
# ~/.kandev/plugins/kandev-plugin-slack.config.yml
bot_token_secret: "secret:slack-bot-token"
default_channel: "#kandev-notifications"
notify_on_task_created: true
```

Config changes are delivered to the plugin via a POST to a `config-changed` endpoint (if declared) or the plugin polls its config on the next event.

#### Health monitoring

- Kandev polls each active plugin's health endpoint every 30 seconds.
- `GET {base_url}{endpoints.health}` must return `200 OK` within 5 seconds.
- 3 consecutive failures: status changes to `error`, inbox item created.
- Recovery: next successful health check restores `active` status, queued events are delivered.
- Health check results are logged in the activity log.

### Security

#### HMAC-SHA256 webhook signatures

Every HTTP request from kandev to a plugin includes a signature header:

```
X-Kandev-Signature: sha256=<hex(HMAC-SHA256(webhook_secret, raw_body))>
```

Plugins verify this signature to ensure the request came from their kandev instance. This is the same pattern used by GitHub, Slack, and Stripe webhooks.

#### Per-plugin API keys

Each plugin receives a unique API key at registration. The plugin includes this key in all requests to kandev:

```
Authorization: Bearer <api_key>
```

Kandev validates the key and maps it to the plugin ID. All capability checks are evaluated against the registered plugin's manifest.

#### Capability-based access control

Plugins can only access the kandev APIs they declared in their manifest:
- A plugin with `api_read: ["tasks"]` can call `GET /api/orchestrate/tasks` but not `GET /api/orchestrate/agents`.
- A plugin without `capabilities.state: true` gets 403 on all state API calls.
- A plugin without `capabilities.secrets: true` gets 403 on secret resolution.
- Tool invocations are only routed to plugins that declared the tool in their manifest.

Undeclared access attempts return `403 Forbidden` with a clear error message naming the missing capability.

#### Network considerations

Plugins run on the same machine or local network as kandev (localhost or LAN). There is no internet-facing plugin API in v1. For remote plugins (Docker on a different host), the operator is responsible for network security (VPN, SSH tunnel, etc.).

### Example plugins to build first

#### 1. Slack notifications (reference connector)

The simplest useful plugin. Receives `task.created`, `task.state_changed`, `orchestrate.approval.created` events. Posts formatted messages to Slack channels. Config: bot token (secret ref), default channel, per-event toggles.

This plugin serves as the reference implementation and template for other notification connectors.

#### 2. Jira sync (bidirectional connector)

Migrates the existing `internal/jira/` integration to a plugin. Bidirectional sync between Jira tickets and orchestrate tasks: status mapping, comment bridging, webhook ingestion from Jira, periodic reconciliation via polling. Uses plugin state for link tracking (Jira issue ID to kandev task ID mappings).

#### 3. Custom metrics exporter (analytics plugin)

Subscribes to `orchestrate.cost.recorded`, `orchestrate.wakeup.processed`, `agent.completed`, `agent.failed`. Aggregates metrics and exposes a Prometheus `/metrics` endpoint. No agent tools, no webhooks, no state -- pure event consumer.

## Scenarios

- **GIVEN** an operator with a Slack notification plugin process running on localhost:9100, **WHEN** the operator calls `POST /api/plugins/register` with the plugin manifest, **THEN** kandev validates the manifest, generates API key and webhook secret, stores the registration at `~/.kandev/plugins/kandev-plugin-slack.yml`, calls `GET http://localhost:9100/health`, and returns the credentials. The plugin appears in `GET /api/plugins` with status `active`.

- **GIVEN** an active Slack plugin subscribed to `task.state_changed`, **WHEN** a task moves to `done`, **THEN** kandev POSTs the event to `http://localhost:9100/events` with HMAC signature. The plugin verifies the signature, formats a Slack message, and calls the Slack API.

- **GIVEN** a Jira sync plugin that registered the `jira_search` agent tool, **WHEN** an agent calls `jira_search` with query "auth migration", **THEN** kandev POSTs the tool call to `http://localhost:9200/tools/jira_search`. The plugin queries Jira, returns results. The agent receives the search results as the tool output.

- **GIVEN** a Jira sync plugin with a registered `jira-webhooks` webhook, **WHEN** Jira POSTs a webhook to `https://kandev.example.com/api/plugins/kandev-plugin-jira/webhooks/jira-webhooks`, **THEN** kandev proxies the request to the plugin's webhook endpoint. The plugin parses the Jira event and calls `PATCH /api/orchestrate/tasks/{id}` to update the linked task's status.

- **GIVEN** an active plugin that becomes unreachable (process crash), **WHEN** 3 consecutive health checks fail (90 seconds), **THEN** kandev marks the plugin as `error`, creates an inbox item "Plugin kandev-plugin-slack is unreachable", and buffers events (up to 100 or 5 minutes). **WHEN** the operator restarts the plugin and the next health check succeeds, **THEN** status returns to `active` and buffered events are delivered in order.

- **GIVEN** a plugin with `api_read: ["tasks"]`, **WHEN** the plugin calls `GET /api/orchestrate/agents`, **THEN** kandev returns `403 Forbidden: capability 'api_read:agents' not declared`.

- **GIVEN** two plugins (Slack and Jira), **WHEN** the Jira plugin emits a cross-plugin event `plugin.kandev-plugin-jira.sync-completed`, **THEN** the Slack plugin (subscribed to `plugin.kandev-plugin-jira.*`) receives the event and posts a sync summary to Slack.

- **GIVEN** a plugin with state, **WHEN** the plugin calls `POST /api/plugins/{id}/state` with `{ scope: "task", scope_id: "task_xyz", key: "jira_issue_id", value: "PROJ-123" }`, **THEN** the state is persisted in SQLite. A subsequent `GET /api/plugins/{id}/state/jira_issue_id?scope=task&scope_id=task_xyz` returns `"PROJ-123"`.

## Out of scope

- **UI extensions.** Plugins cannot add tabs, widgets, pages, or other UI elements to the kandev frontend in v1. UI extension requires a frontend plugin loading mechanism (dynamic imports, iframes, or a micro-frontend framework) which is a significant addition. Plugins can only affect the system through data (creating tasks, posting comments, updating state) which the existing UI renders.
- **In-process plugins.** No Go plugin loading, no WASM, no shared-memory communication. Plugins are always separate processes communicating over HTTP.
- **Plugin marketplace or registry.** No central catalog of available plugins, no one-click install, no automatic discovery. Operators find, install, and configure plugins manually.
- **Plugin process management.** Kandev does not start, stop, restart, or upgrade plugin processes. Operators manage the plugin lifecycle with their own tools.
- **Hot reload.** Changing a plugin's manifest requires re-registration. There is no live manifest update mechanism.
- **Multi-instance plugins.** Each plugin ID maps to exactly one base URL. Running multiple instances of the same plugin (for different workspaces or for HA) is not supported in v1.
- **Rate limiting.** No per-plugin rate limits on API calls or event delivery in v1. Misbehaving plugins can be disabled manually.
- **Plugin database namespaces.** Unlike Paperclip, kandev plugins do not get their own SQLite schemas. The KV state store is sufficient for v1. Plugins that need relational data manage their own database.

## Implementation plan

### Phase 1: Plugin registration + event delivery + state API

**Goal:** A plugin can register, receive events, and persist state. This is the minimum viable plugin system -- enough to build a Slack notification plugin.

**Backend work:**
- Plugin manifest schema: Go struct matching the YAML manifest above, validation logic.
- Plugin registry: in-memory registry loaded from `~/.kandev/plugins/*.yml` on startup, with `fsnotify` for external changes (same pattern as orchestrate config).
- Registration API: `POST /api/plugins/register`, `GET /api/plugins`, `GET /api/plugins/{id}`, `PATCH /api/plugins/{id}`, `DELETE /api/plugins/{id}`, `POST /api/plugins/{id}/enable`, `POST /api/plugins/{id}/disable`.
- API key generation and HMAC secret generation at registration. Bcrypt hash storage.
- Plugin auth middleware: validate `Authorization: Bearer <api_key>` header, resolve to plugin ID, check capabilities on each request.
- Event delivery: subscribe to kandev event bus, for each event check all active plugins' subscriptions, POST to matching plugins' event endpoints. Sequential delivery per plugin. Retry with backoff (3 attempts). Event buffer for `error` state plugins (ring buffer, 100 events, 5-minute TTL).
- Plugin state API: CRUD on the `plugin_state` SQLite table, scoped by plugin ID.
- Health check loop: goroutine polling health endpoints every 30 seconds, state transitions on failure/recovery.
- Filesystem persistence: write/read `~/.kandev/plugins/*.yml` and `~/.kandev/plugins/*.config.yml`.

**Estimated scope:** ~15 files in `internal/plugins/`, ~1500 lines.

### Phase 2: Agent tools + webhook proxy + secrets

**Goal:** Plugins can register tools that agents use, receive external webhooks, and resolve secrets.

**Backend work:**
- Tool registry: on plugin registration, extract tool declarations and register them in a plugin tool registry. The lifecycle manager queries this registry when assembling tools for an agent session.
- Tool invocation: when the agent calls a plugin tool, the orchestrator routes the call to the plugin's tool endpoint (POST with input + context). Response is returned to the agent.
- Tool injection: modify the agent session setup to include plugin tools alongside MCP tools. Tools are described as standard tool definitions (name, description, input schema) so agent CLIs handle them natively via ACP.
- Webhook proxy: `POST /api/plugins/{plugin_id}/webhooks/{webhook_key}` route. Validates the plugin is active and the webhook key is declared. Forwards the request body and headers to the plugin's webhook endpoint. Returns the plugin's response.
- Secrets API: `GET /api/plugins/{plugin_id}/secrets/{ref}` resolves a secret reference through kandev's existing `internal/secrets/` package. Capability-gated.

**Estimated scope:** ~8 files, ~800 lines. Touches lifecycle manager and orchestrator for tool injection.

### Phase 3: SDK + reference plugins

**Goal:** Make it easy to build plugins. Ship reference implementations.

**SDK work (Go module):**
- `github.com/kandev/kandev-plugin-sdk-go` -- Go module with:
  - HTTP server scaffold (health endpoint, event handler, tool handler, webhook handler pre-wired).
  - Kandev API client (typed methods for tasks, comments, state, secrets, activity).
  - HMAC signature verification helper.
  - Config loading (from env vars or file).
  - Testing harness (mock kandev server for unit testing plugins).

**SDK work (npm package):**
- `@kandev/plugin-sdk` -- TypeScript/Node.js package with the same capabilities for Node-based plugins.

**Reference plugins:**
- `kandev-plugin-slack`: Slack notifications. Go binary. Events -> Slack message formatting -> Slack API.
- `kandev-plugin-jira`: Bidirectional Jira sync. Go binary. Migrated from `internal/jira/`.
- Each plugin ships as a single binary (Go) or Docker image, with a README, config example, and manifest.

**Estimated scope:** SDK ~1000 lines per language. Each reference plugin ~500-1000 lines.

### Phase 4: Health monitoring + observability

**Goal:** Production-grade plugin health management.

**Backend work:**
- Health dashboard: `GET /api/plugins/health` returns all plugins with status, last health check time, consecutive failure count, event delivery stats (delivered, failed, buffered).
- Inbox integration: `error` state transitions create inbox items. Recovery clears them.
- Activity log: plugin registration, state changes, health transitions, event delivery failures all logged.
- Metrics: event delivery latency histogram, tool call latency histogram, health check success rate per plugin. Exposed to the metrics exporter plugin (or built-in Prometheus endpoint).
- Graceful degradation: when a plugin enters `error` state, its tools are removed from the agent tool set (agents don't see broken tools), its webhooks return 503, and its events are buffered.

**Estimated scope:** ~5 files, ~500 lines. Mostly wiring existing infrastructure (inbox, activity log, metrics).

## Related specs

- [orchestrate-overview](../orchestrate-overview/spec.md) -- Orchestrate feature overview, page structure
- [orchestrate-scheduler](../orchestrate-scheduler/spec.md) -- Wakeup queue drives event generation for plugins
- [orchestrate-skills](../orchestrate-skills/spec.md) -- Skills are the preferred extension mechanism for agent behavior; plugins extend the system, skills extend agents
- [orchestrate-assistant](../orchestrate-assistant/spec.md) -- Channels (Telegram, Slack) are the first candidates for plugin extraction
- [orchestrate-config](../orchestrate-config/spec.md) -- Filesystem-first config pattern reused for plugin registration storage
