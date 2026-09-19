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

`next_cursor` is omitted when there is no next page. `limit` defaults to 50
and accepts any value from 1 to 200. Unlike the gRPC Host API, this surface
does not clamp an out-of-range `limit` into that window: a supplied value
outside 1..200 is rejected outright with HTTP 400 `invalid_request`, and only
an omitted `limit` falls back to the 50-row default. A task-scoped canvas is
restricted to its task. A workspace-scoped canvas is restricted to its
workspace.

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
`external_id`, `blocked`, `blocked_reason`, `depends_on`, `blocks`,
`depends_on_truncated`, `blocks_truncated`, and `start_when_unblocked`.
Repository entries contain `id`, `repository_id`, `base_branch`, `position`,
and `checkout_branch`.

`depends_on` and `blocks` are read-only dependency-edge lists. Each entry
contains `id`, `title`, `state`, and, on a `depends_on` entry only, `status`
(a `blocks` entry never carries `status`, since a task cannot be "pending" or
"resolved" against a task it blocks). Each list is capped at 512 entries;
`depends_on_truncated` and `blocks_truncated` report whether more edges exist
than were returned. `title` and `state` are blanked (empty string) on any
edge end the caller's canvas scope does not directly admit: a
workspace-scoped canvas admits an edge end sharing its workspace; a
repository- or session-scoped canvas admits an edge end only when it is also
returned as a directly readable task in the same response, never by
workspace equality alone; a task-scoped canvas admits none, seeing only the
edge's `id` and, for a `depends_on` entry, its `status`.
`blocked`, `blocked_reason`, and `start_when_unblocked` summarize the same
projection at the task level. When an internal read failure keeps the host
from deriving a verdict for a task a route does return, it substitutes a
withheld verdict rather than failing that route: `blocked: true`,
`blocked_reason: "unknown"`, empty `depends_on`/`blocks`, both truncation
flags `false`, and `start_when_unblocked: false`. Treat that shape as "no
answer," not as "task is actually blocked." A canvas lacking the read
capability never sees this verdict: every task route, including the `PATCH`
route, checks the read capability before returning any task and fails
outright with `403 plugin_permission_denied` when it is missing, so accessor
denial and a withheld verdict are never the same response. This is also
distinct from the fan-out limit described below, which fails the whole page
with `response_too_large` rather than substituting a withheld verdict onto
any task.

These seven fields exist on a task object only once the host's manifest
`min_kandev_version` floor is met; declare that floor for the release your
canvas expects them from, since an older host omits the fields entirely
rather than sending empty defaults for them. A host running a `dev` or other
non-release build always satisfies that floor check regardless of its actual
age, so a canvas can still receive zero-value dependency fields on such a
host even when the floor is declared correctly.

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

No event ever carries the dependency projection (`blocked`, `blocked_reason`,
`depends_on`, `blocks`, the truncation flags, `start_when_unblocked`) in its
data payload — those fields are refetch-on-signal only. Refetch a cached
task's dependency fields when: a `task.updated` event names that task or
either end of one of its edges; a `task.dependencies_resolved` or
`task.dependency_failed` event names that task; or a `task.state_changed`
event names any task ID present in that task's cached `depends_on` or
`blocks` list, since a predecessor or dependent simply advancing state is not
itself one of the first three signals. A single task read is not a
transactional snapshot: with no surrounding lock, an edge can change while
the read is being derived, so one response can show an edge asymmetrically
(for example, a predecessor
still listed as pending after it has already resolved). Treat what a
response returns as the union of independently-read facts, and resolve
staleness by refetching on the next matching signal rather than by trusting
any single response as authoritative.

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

On the task list route, `response_too_large` also covers dependency
derivation refusing a page: deriving `depends_on`/`blocks` for every task on
a page reads a bounded number of distinct task IDs across all of that page's
edges (shared edge ends count once), and a page that would cross that bound
fails the whole request with `response_too_large` rather than returning a
partial or withheld-verdict page. Retry with a smaller `limit`. A single-task
read or write is never subject to this bound, since it can only ever derive
one task's edges.

All request bodies are bounded. Treat unknown error codes as retryable only
when the operation is a read and the canvas is still mounted.
