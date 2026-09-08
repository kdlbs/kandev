# Canvas browser API

The host exposes protocol version 1 below the capability URL. Resolve every
route from the application document with a relative URL such as
`./_kandev/v1/context`. Do not use a host URL, a hard-coded port, or a token
from application state. The browser sends the capability token as part of the
current URL.

Every request is checked against the current canvas release, scope, status,
and grants. A request can fail after a release is archived, a grant is
revoked, or the canvas is removed. Abort requests during iframe teardown and
show a retry state for transient reads.

## Context

`GET ./_kandev/v1/context` returns:

```json
{
  "protocol_version": 1,
  "instance_id": "instance-id",
  "plugin_id": "example-canvas",
  "release_id": "release-id",
  "web_app_key": "main",
  "placement": "task-canvas",
  "scope_kind": "task",
  "workspace_id": "workspace-id",
  "task_id": "task-id",
  "session_id": "session-id",
  "repository_id": "repository-id",
  "capabilities": ["api_read:tasks", "api_write:messages"]
}
```

Scope identifiers are omitted when they do not apply. `capabilities` contains
the effective, approved permission keys. It is not a replacement for handling
permission errors from later requests.

## Data routes

All data responses use JSON. Collection responses use this envelope:

```json
{
  "items": [],
  "page_info": { "next_cursor": "", "has_more": false }
}
```

`next_cursor` is omitted when there is no next page. `limit` must be from 1 to
the host page limit. A task-scoped canvas is restricted to its task. A
workspace-scoped canvas is restricted to its workspace.

| Method | Route | Permission | Use |
| --- | --- | --- | --- |
| GET | `./_kandev/v1/data/tasks` | `api_read:tasks` | List tasks |
| GET | `./_kandev/v1/data/tasks/{task_id}` | `api_read:tasks` | Read one task |
| PATCH | `./_kandev/v1/data/tasks/{task_id}` | `api_write:tasks` | Update a task |
| POST | `./_kandev/v1/data/tasks/{task_id}/messages` | `api_write:messages` | Send a task message |
| GET | `./_kandev/v1/data/workflows` | `api_read:workflows` | List workflows |
| GET | `./_kandev/v1/data/workflows/{workflow_id}/steps` | `api_read:workflows` | Read workflow steps |

The task-list query accepts `cursor`, `limit`, `include_archived`,
`workflow_id`, `state`, and `parent_id`. `workflow_id` and `state` can be
repeated or comma-separated. `include_archived` is a boolean.

A task object contains these fields: `id`, `workspace_id`, `workflow_id`,
`title`, `description`, `state`, `priority`, `created_by`, `created_at`,
`updated_at`, `started_at`, `completed_at`, `parent_id`, `identifier`,
`is_ephemeral`, `repositories`, `metadata`, `archived_at`, `pull_requests`,
`workflow_step_id`, `position`, `assignee_agent_profile_id`, `labels`,
`autopilot`, `wip_admitted`, `queued_for_step_id`, `queued_at`, `project_id`,
and `external_id`. Repository entries contain `id`, `repository_id`,
`base_branch`, `position`, and `checkout_branch`.

A workflow object contains `id`, `workspace_id`, `name`, `description`,
`sort_order`, `created_at`, and `updated_at`. A workflow-step object contains
`id`, `workflow_id`, `name`, `position`, `stage_type`, `color`,
`is_start_step`, `wip_limit`, `agent_profile_id`, and
`on_enter_action_types`.

## Writes and workflow movement

`PATCH ./_kandev/v1/data/tasks/{task_id}` accepts one or more of these JSON
fields: `title`, `description`, `state`, and `workflow_step_id`. A workflow
step move is therefore a task patch. There is no separate workflow-step write
route. The response is the updated task object.

For example, to continue work, send a message through the normal task
message path:

```http
POST ./_kandev/v1/data/tasks/task-id/messages
Content-Type: application/json

{"text":"continue"}
```

To move the task, use its known target step ID:

```http
PATCH ./_kandev/v1/data/tasks/task-id
Content-Type: application/json

{"workflow_step_id":"step-in-progress"}
```

`POST ./_kandev/v1/data/tasks/{task_id}/messages` requires a non-empty `text`
field and accepts an optional `session_id`. It returns HTTP 202 with
`session_id` and `status`, where status is `queued`, `sent`, or `started`.

## Instance state

`GET ./_kandev/v1/state` lists state entries. `GET
./_kandev/v1/state/{key}` reads one entry. `PUT` and `DELETE` use
`./_kandev/v1/state/{key}` and require `If-Match`.

| Method | Route | Permission | Use |
| --- | --- | --- | --- |
| GET | `./_kandev/v1/state` | `state` | List state |
| GET | `./_kandev/v1/state/{key}` | `state` | Read state |
| PUT | `./_kandev/v1/state/{key}` | `state` | Replace state |
| DELETE | `./_kandev/v1/state/{key}` | `state` | Delete state |

`If-Match` is a non-negative integer revision, either bare (`If-Match: 3`) or
quoted (`If-Match: "3"`). A wildcard, missing header, malformed value, or
negative value returns HTTP 428 with `plugin_state_precondition_required`.
The PUT body must be one valid JSON value. A revision mismatch returns HTTP
409 with:

```json
{"error":"plugin_state_conflict","current_revision":4}
```

State entries contain `key`, `value`, `revision`, `writer_kind`, and
`updated_at`. State is shared by all approved releases of the same canvas
instance. Keep values small and do not store secrets.

## Events and reconnect

`GET ./_kandev/v1/events` opens a bounded Server-Sent Events stream. Send the
last received event ID in the `Last-Event-ID` request header when reconnecting.
Normal events have an ID in `generation:sequence` form and contain the full
event envelope in the SSE data field:

```text
id: generation:12
event: task.updated
data: {"id":"generation:12","generation":"generation","sequence":12,"type":"task.updated","scope":{"instance_id":"instance-id"},"data":{}}
```

The envelope fields are `id`, `generation`, `sequence`, `type`, `scope`, and
`data`. The scope contains `instance_id` and can contain `workspace_id`,
`task_id`, `session_id`, and `repository_id`. The event data is specific to
the event type.

The server sends `: heartbeat` comments at the heartbeat interval. Streams
have bounded queues and a finite lifetime. A slow consumer can be closed.
When the requested cursor is invalid, expired, from another process
generation, or ahead of the current sequence, the server sends
`runtime.resync_required` with an empty SSE ID. Its data contains
`reason`, `generation`, and `reset: true`. On resync, clear the cursor,
refetch the authoritative data, and reconnect without a cursor.

Event delivery does not replace HTTP reads. Refetch after an event that can
change visible data, and stop using the iframe immediately when its host
reports a lifecycle or authority change.

## Actions and errors

`POST ./_kandev/v1/actions/{key}` accepts a JSON body only when the manifest
declares the matching `action:{key}` permission. The current static canvas
host has no action implementation and returns HTTP 501 with
`plugin_action_unavailable`. An undeclared action returns HTTP 403 with
`plugin_permission_denied`.

Other stable error codes are:

| Status | Error |
| --- | --- |
| 400 | `invalid_request` |
| 401 | `runtime_token_stale` |
| 403 | `plugin_permission_denied` |
| 404 | `not_found` |
| 405 | `method_not_allowed` |
| 409 | `plugin_state_conflict` |
| 428 | `plugin_state_precondition_required` |
| 413 or 500 | `response_too_large` when a host limit is exceeded |
| 503 | `runtime_unavailable` |

All request bodies are bounded. Treat unknown error codes as retryable only
when the operation is a read and the canvas is still mounted.
