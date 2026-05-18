---
status: draft
created: 2026-04-26
owner: cfl
---

# Office: Plugin System

## Why

Kandev's office feature is building a growing list of external integrations directly into the core codebase: GitHub PR sync, Jira ticket browsing, notification providers (Apprise, system notifications), and planned channel types (Telegram, Slack, Discord, email). Each integration adds platform-specific logic - API clients, webhook handlers, payload formatting, OAuth flows, secret management - to the Go backend. This creates three problems:

1. **Core bloat.** Every new integration increases the surface area of the backend. Adding Slack, Discord, Telegram, Linear, and others at similar scale makes the backend unmaintainable.
2. **Release coupling.** Fixing a bug in the Telegram webhook parser requires a kandev release. Users who don't use Telegram still receive the code. Integration authors cannot ship independently of the core release cycle.
3. **Extensibility ceiling.** Users cannot add their own integrations without forking kandev.

A plugin system decouples integrations from core. Plugins run as separate processes, communicate with kandev over HTTP, and extend office through well-defined capabilities. The core stays small; the ecosystem grows independently.

## What

- Plugins SHALL run as **separate processes** and communicate with kandev over HTTP only - no in-process loading, no language-specific SDK requirement for v1.
- A plugin manifest declares identity, endpoints, capabilities, declared tools, declared webhooks, and config schema.
- Plugins SHALL receive event webhooks, register agent tools, expose proxied external webhook endpoints, and read/write a plugin-scoped KV state.
- All plugin <-> kandev requests SHALL be authenticated (API key) and signed (HMAC-SHA256).
- Capability-based access control: a plugin can only call kandev APIs it declared in its manifest.
- Operators manage plugin processes themselves; kandev only knows `base_url` and health endpoint.

## Data model

### Plugin registration (filesystem-backed)

Stored at `~/.kandev/plugins/{plugin_id}.yml` (global to the kandev instance, not workspace-scoped). Cleartext `api_key` and `webhook_secret` are returned once at registration and stored only as bcrypt hashes.

```yaml
id: "kandev-plugin-slack"                    # Unique, pattern: ^[a-z0-9][a-z0-9._-]*$
api_version: 1
version: "1.0.0"
display_name: "Slack Notifications"
description: "Post to Slack on task events, relay messages to agents"
author: "kandev"
categories: ["connector"]                    # connector | automation | tools | analytics

base_url: "http://localhost:9100"

endpoints:
  health: "/health"                          # GET, returns 200 if healthy
  events: "/events"                          # POST, receives event webhooks
  tools: "/tools/{tool_name}"               # POST, receives tool invocations
  webhooks: "/webhooks/{webhook_key}"       # POST, receives proxied external webhooks

capabilities:
  events: ["task.created", "task.state_changed", "office.comment.created", "office.approval.created"]
  api_read: ["tasks", "agents", "projects"]
  api_write: ["tasks", "comments"]
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

# Runtime fields managed by kandev:
status: "active"
api_key_hash: "<bcrypt hash>"
webhook_secret_hash: "<bcrypt hash>"
registered_at: "2026-04-26T10:00:00Z"
last_health_check: "2026-04-26T10:30:00Z"
```

Plugin config (operator-editable, conforms to `config_schema`) lives at `~/.kandev/plugins/{plugin_id}.config.yml`.

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

### Plugin registration & management (operator -> kandev)

```
POST   /api/plugins/register          # Register a new plugin (manifest YAML in body)
GET    /api/plugins                    # List registered plugins
GET    /api/plugins/{id}              # Get plugin detail
PATCH  /api/plugins/{id}              # Update plugin config
DELETE /api/plugins/{id}              # Uninstall plugin
POST   /api/plugins/{id}/enable
POST   /api/plugins/{id}/disable
```

Registration flow:
1. Operator starts plugin process.
2. Operator calls `POST /api/plugins/register` with manifest.
3. Kandev validates schema, checks ID conflicts.
4. Kandev generates `webhook_secret` (HMAC) and `api_key` (plugin->kandev auth).
5. Kandev stores record on disk + in-memory registry.
6. Kandev calls plugin's health endpoint.
7. Healthy -> status `active`; unreachable -> status `registered` (retries on next health cycle).
8. Kandev returns cleartext `api_key` and `webhook_secret` once.

### Event webhook delivery (kandev -> plugin)

```
POST {plugin.base_url}{plugin.endpoints.events}
Content-Type: application/json
X-Kandev-Plugin-Id: kandev-plugin-slack
X-Kandev-Signature: sha256=<hmac>
X-Kandev-Delivery-Id: del_abc123
X-Kandev-Event-Type: task.state_changed

{
  "event_id": "evt_abc123",
  "event_type": "task.state_changed",
  "occurred_at": "2026-04-26T10:30:00Z",
  "workspace_id": "ws_default",
  "payload": { "task_id": "task_xyz", "old_status": "in_progress", "new_status": "done", "agent_instance_id": "agent_frontend" }
}
```

HMAC: `HMAC-SHA256(webhook_secret, raw_body)`. Expected response: `200 OK`. Non-2xx triggers retry.

Delivery semantics:
- **At-least-once.** Plugins must be idempotent (dedup by `event_id`).
- **Timeout:** 10 seconds. Up to 3 retries with exponential backoff (5s, 15s, 45s).
- **Sequential per plugin** - no concurrent delivery to the same plugin. Plugins needing parallel processing queue internally.

Event types: all from `internal/events/types.go` plus office-specific events.

| Category | Events |
|----------|--------|
| Tasks | `task.created`, `task.updated`, `task.state_changed`, `task.deleted`, `task.moved` |
| Sessions | `task_session.state_changed`, `turn.started`, `turn.completed` |
| Agents | `agent.started`, `agent.completed`, `agent.failed`, `agent.stopped` |
| Office | `office.agent.created`, `office.agent.updated`, `office.agent.status_changed`, `office.comment.created`, `office.approval.created`, `office.approval.resolved`, `office.cost.recorded`, `office.wakeup.queued`, `office.wakeup.processed`, `office.routine.triggered`, `office.inbox.item` |
| GitHub | `github.pr_state_changed`, `github.pr_feedback`, `github.new_issue` |
| Plugin | `plugin.<plugin_id>.<name>` (cross-plugin events) |

Wildcard subscriptions: `task.*`, `office.*`, `agent.*`.

### Agent tool invocation (kandev -> plugin)

When an agent calls a plugin tool, kandev POSTs to the plugin's tool endpoint:

```
POST {plugin.base_url}/tools/slack_send_message
{ "tool_call_id": "tc_123", "input": { "channel": "#dev", "text": "Build passed" },
  "context": { "task_id": "...", "agent_instance_id": "...", "session_id": "..." } }
```

Response: `{ "output": { "message_id": "M123", "channel": "#dev" } }`. Timeout: 30 seconds. Tools available to all agents by default (`tool_scope` for role/instance restrictions is future work).

### External webhook proxy (external -> kandev -> plugin)

```
POST /api/plugins/{plugin_id}/webhooks/{webhook_key}
```

Kandev acts as reverse proxy: validates plugin is active and webhook key is declared, forwards body + headers to the plugin's webhook endpoint, adds `X-Plugin-Id` and `X-Webhook-Key`. The plugin verifies the external system's signature (Slack signing secret, GitHub webhook secret, etc.).

### Plugin state API (plugin -> kandev)

```
GET    /api/plugins/{id}/state?scope={scope}&scope_id={id}&key={key}
POST   /api/plugins/{id}/state
       Body: { "scope": "task", "scope_id": "task_xyz", "key": "sync_status", "value": { "synced": true } }
DELETE /api/plugins/{id}/state?scope={scope}&scope_id={id}&key={key}
GET    /api/plugins/{id}/state/list?scope={scope}&scope_id={id}
```

Scopes: `instance`, `workspace`, `task`, `agent`. Plugins cannot read other plugins' state.

### Plugin secrets API (plugin -> kandev)

```
GET /api/plugins/{plugin_id}/secrets/{ref}
```

Resolves a secret reference through kandev's `internal/secrets/` package. Requires `capabilities.secrets: true`.

### Read/write office data (plugin -> kandev)

Plugins authenticate with their API key:

```
Authorization: Bearer <api_key>
```

Read endpoints (gated by `capabilities.api_read`):
- `GET /api/office/tasks`, `/api/office/tasks/{id}`, `/api/office/tasks/{id}/comments`
- `GET /api/office/agents`, `/api/office/agents/{id}`
- `GET /api/office/projects`, `/api/office/costs`, `/api/office/activity`, `/api/office/approvals`

Write endpoints (gated by `capabilities.api_write`):
- `POST /api/office/tasks`, `PATCH /api/office/tasks/{id}`
- `POST /api/office/tasks/{id}/comments` (`source: "plugin:<plugin_id>"`)
- `POST /api/office/wakeups`, `POST /api/office/activity`

### Cross-plugin events

```
POST /api/plugins/{plugin_id}/events/emit
Body: { "event_name": "sync-completed", "payload": { "count": 5 } }
```

Published as `plugin.<plugin_id>.sync-completed` and delivered to subscribers.

## State machine

```
registered -> active -> disabled -> uninstalled
                 |          |
                 +-> error -+
```

| State | Meaning |
|---|---|
| `registered` | Manifest stored; plugin process expected running but health not yet verified |
| `active` | Health check passes; events delivered, tools available, webhooks proxied |
| `error` | Health check failed 3x consecutively (30s interval). Events buffered (ring buffer, 100 events, 5-minute TTL). Tools removed from agent tool set. Webhooks return 503 |
| `disabled` | Operator explicitly disabled. No events, no tools, no webhooks. State and config preserved |
| `uninstalled` | Removed. State deleted after 24-hour grace period (force-delete available) |

Health monitoring: every 30 seconds, `GET {base_url}{endpoints.health}` must return `200 OK` within 5 seconds. 3 consecutive failures -> `error` + inbox item. Next success -> `active`, queued events delivered in order.

## Permissions

- Plugins are global to the kandev instance, registered by the operator. There is no per-user plugin access in v1.
- Capability-based access control: undeclared capabilities return `403 Forbidden: capability '<name>' not declared`.
- A plugin's API key maps to its plugin ID; capability checks evaluate against the registered manifest on every request.

## Security

- **HMAC-SHA256 webhook signatures** on every kandev -> plugin request (`X-Kandev-Signature`).
- **Per-plugin API keys** for plugin -> kandev requests (`Authorization: Bearer <api_key>`). Stored only as bcrypt hashes.
- **Capability-based access control** evaluated per request.
- **Network**: plugins run on the same host or LAN as kandev. No internet-facing plugin API in v1. Remote plugins are the operator's responsibility (VPN, SSH tunnel).

## Failure modes

- **Plugin unreachable for 3 consecutive health checks (90s)**: status -> `error`. Events buffered (100, 5min TTL). Tools hidden from agent tool set. Webhooks return 503. Inbox item created.
- **Buffer overflows (>100 events or >5min)**: oldest events dropped and logged.
- **Plugin returns non-2xx on event delivery**: retry up to 3 times with exponential backoff (5s, 15s, 45s). After exhaustion, event is logged as failed and dropped.
- **Tool call timeout (>30s)**: agent receives a timeout error.
- **External webhook hits a disabled/error plugin**: kandev returns 503.
- **Undeclared capability access attempt**: 403 with clear error message naming the missing capability.

## Persistence guarantees

- Plugin registration manifests persist at `~/.kandev/plugins/*.yml` and survive backend restarts.
- Plugin state in SQLite survives restarts.
- Event delivery buffer is in-memory; events in the buffer do not survive a backend restart.
- Cleartext `api_key` and `webhook_secret` are returned exactly once at registration. Lost credentials require re-registration.

## Scenarios

- **GIVEN** an operator with a Slack notification plugin process running on localhost:9100, **WHEN** the operator calls `POST /api/plugins/register` with the plugin manifest, **THEN** kandev validates the manifest, generates API key and webhook secret, stores the registration at `~/.kandev/plugins/kandev-plugin-slack.yml`, calls `GET http://localhost:9100/health`, and returns the credentials. The plugin appears in `GET /api/plugins` with status `active`.

- **GIVEN** an active Slack plugin subscribed to `task.state_changed`, **WHEN** a task moves to `done`, **THEN** kandev POSTs the event to `http://localhost:9100/events` with HMAC signature. The plugin verifies the signature, formats a Slack message, and calls the Slack API.

- **GIVEN** a Jira sync plugin that registered the `jira_search` agent tool, **WHEN** an agent calls `jira_search` with query "auth migration", **THEN** kandev POSTs the tool call to `http://localhost:9200/tools/jira_search`. The plugin queries Jira, returns results. The agent receives the search results as the tool output.

- **GIVEN** a Jira sync plugin with a registered `jira-webhooks` webhook, **WHEN** Jira POSTs a webhook to `https://kandev.example.com/api/plugins/kandev-plugin-jira/webhooks/jira-webhooks`, **THEN** kandev proxies the request to the plugin's webhook endpoint. The plugin parses the Jira event and calls `PATCH /api/office/tasks/{id}` to update the linked task's status.

- **GIVEN** an active plugin that becomes unreachable (process crash), **WHEN** 3 consecutive health checks fail (90 seconds), **THEN** kandev marks the plugin as `error`, creates an inbox item "Plugin kandev-plugin-slack is unreachable", and buffers events (up to 100 or 5 minutes). **WHEN** the operator restarts the plugin and the next health check succeeds, **THEN** status returns to `active` and buffered events are delivered in order.

- **GIVEN** a plugin with `api_read: ["tasks"]`, **WHEN** the plugin calls `GET /api/office/agents`, **THEN** kandev returns `403 Forbidden: capability 'api_read:agents' not declared`.

- **GIVEN** two plugins (Slack and Jira), **WHEN** the Jira plugin emits a cross-plugin event `plugin.kandev-plugin-jira.sync-completed`, **THEN** the Slack plugin (subscribed to `plugin.kandev-plugin-jira.*`) receives the event and posts a sync summary to Slack.

- **GIVEN** a plugin with state, **WHEN** the plugin calls `POST /api/plugins/{id}/state` with `{ scope: "task", scope_id: "task_xyz", key: "jira_issue_id", value: "PROJ-123" }`, **THEN** the state is persisted in SQLite. A subsequent `GET /api/plugins/{id}/state/jira_issue_id?scope=task&scope_id=task_xyz` returns `"PROJ-123"`.

## Out of scope

- **UI extensions.** Plugins cannot add tabs, widgets, pages, or other UI elements to the kandev frontend in v1. Plugins affect the system through data which the existing UI renders.
- **In-process plugins.** No Go plugin loading, no WASM, no shared-memory communication.
- **Plugin marketplace or registry.** No central catalog, no one-click install, no automatic discovery.
- **Plugin process management.** Kandev does not start, stop, restart, or upgrade plugin processes.
- **Hot reload.** Changing a manifest requires re-registration.
- **Multi-instance plugins.** Each plugin ID maps to exactly one base URL.
- **Rate limiting.** No per-plugin rate limits in v1. Misbehaving plugins can be disabled manually.
- **Plugin database namespaces.** Plugins do not get their own SQLite schemas. KV state is sufficient for v1.
