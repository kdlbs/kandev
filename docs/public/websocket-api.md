---
title: "WebSocket API"
description: "Reference for Kandev WebSocket requests, notifications, delivery, and connection behavior."
---

# Kandev WebSocket API

Kandev's web application uses one persistent WebSocket connection for most interactive requests and live notifications. This is an internal application protocol, not a versioned public SDK: action payloads can change with the backend and web client. For supported use, prefer the Kandev UI, CLI, MCP tools, and documented HTTP routes. Use this page when integrating a local client, reverse proxy, or diagnostic tool with the current implementation.

The normal local endpoint is:

```text
ws://localhost:38429/ws
```

Use `wss://` when the page is served over HTTPS. Workflow import/export, workflow sync, downloads, and several administrative operations remain HTTP APIs; a WebSocket action does not exist for every backend route.

The source of truth is the combination of:

- [`message.go`](../../apps/backend/pkg/websocket/message.go) for the envelope.
- [`actions.go`](../../apps/backend/pkg/websocket/actions.go) for names and standard error codes.
- Handler registration in [`gateway.go`](../../apps/backend/internal/backendapp/gateway.go) and each domain's `handlers` package for actions clients can actually request.
- The concrete request structs and validation in those handlers for payload fields.
- [`client.ts`](../../apps/web/lib/ws/client.ts) for the first-party web client's timeout, reconnect, and resubscription behavior.

An action constant alone is not evidence that an action is registered or emitted. The request catalog below was checked against non-test `RegisterFunc` calls and the gateway's special subscription dispatch, not just the constant list.

## Security and network boundary

The current `/ws` upgrade handler does **not authenticate clients**. It reads `?token=` or the `Authorization` header but does not validate or use the value; JWT validation is still a code TODO. The default backend host is `0.0.0.0`, so a default process can listen on every interface even though examples use `localhost`.

Treat every client that can reach the backend as fully trusted. WebSocket actions can create and delete data, start agents and shells, read and change files, run Git operations, reveal stored secrets, and invoke configured integrations. Do not expose port `38429` directly to an untrusted LAN or the internet. Bind to loopback, firewall the port, or put Kandev behind an authenticated reverse proxy that terminates TLS and restricts access. See [Configuration](configuration.md) and [Run as a Service](run-as-a-service.md).

The upgrade does enforce an origin policy for browser clients:

- A missing `Origin` header is accepted for non-browser clients. This is why origin checks are not authentication.
- A browser origin is accepted when its hostname exactly matches the request hostname; ports are intentionally ignored.
- Different loopback names or addresses are accepted when both origin and request hosts are loopback, such as `localhost` and `127.0.0.1` on different ports.
- Origins must be well-formed `http` or `https` origins with no path, query, fragment, or user information. Other cross-site origins are rejected.

When proxying, preserve a request host that matches the public page's origin and forward WebSocket upgrades. Do not rely on the ignored token parameter for access control.

## Wire envelope

Send one JSON object in each WebSocket text frame. The server writes each response or notification as one text frame. Its reader currently ignores whether an inbound frame was text or binary, but text JSON is the protocol contract.

### Request

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "type": "request",
  "action": "workflow.list",
  "payload": {
    "workspace_id": "workspace-uuid"
  }
}
```

### Response

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "type": "response",
  "action": "workflow.list",
  "payload": {
    "workflows": [],
    "total": 0
  },
  "timestamp": "2026-07-16T09:00:00Z"
}
```

### Notification

```json
{
  "type": "notification",
  "action": "task.updated",
  "payload": {
    "id": "task-uuid",
    "title": "Update authentication"
  },
  "timestamp": "2026-07-16T09:00:01Z"
}
```

### Error

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "type": "error",
  "action": "task.get",
  "payload": {
    "code": "NOT_FOUND",
    "message": "Task not found"
  },
  "timestamp": "2026-07-16T09:00:01Z"
}
```

| Field | Type | Current contract |
|-------|------|------------------|
| `id` | string | Required for a useful request. Generate a unique value and correlate the response or error with it. Notifications omit it. |
| `type` | string | Send `request`. Server output uses `response`, `notification`, or `error`. The current dispatcher does not reject an inbound frame solely because `type` is missing or wrong, so do not treat it as an authorization control. |
| `action` | string | Exact registered action name; names are case-sensitive. |
| `payload` | JSON | Action-specific data. Most handlers expect an object and validate required fields themselves. |
| `timestamp` | RFC 3339 string | Included by the server. Optional and ignored on client requests. |
| `metadata` | object | Optional string-to-string metadata used by selected internal flows. It is not a general request-header mechanism. |

Malformed JSON produces a `BAD_REQUEST` error with empty `id` and `action`, because correlation fields could not be parsed. An unregistered action produces `UNKNOWN_ACTION`. Standard codes are `BAD_REQUEST`, `NOT_FOUND`, `INTERNAL_ERROR`, `UNAUTHORIZED`, `FORBIDDEN`, `VALIDATION_ERROR`, `CONFLICT`, and `UNKNOWN_ACTION`; individual handlers may add codes or return an ordinary response with `success:false` instead. Always inspect both envelope type and response payload.

## Transport, concurrency, and loss behavior

| Behavior | Current value | Client consequence |
|----------|---------------|--------------------|
| Maximum inbound message | 32 MiB | A larger request closes/fails the connection. Base64 attachments consume the same limit. |
| Write deadline | 10 seconds | A peer that cannot accept a frame in time is disconnected. |
| Pong deadline | 60 seconds | The connection closes if the peer stops answering pings. |
| Server ping interval | 54 seconds | WebSocket libraries must process ping/pong control frames. Browsers do this automatically. |
| Per-client outbound queue | 256 frames | When full, the new frame is dropped and the connection stays open. Responses and notifications can therefore be lost under backpressure. |
| Hub-wide broadcast queue | 256 messages | Global publishers block when this internal queue is saturated; delivery to each client can still drop at that client's queue. |
| Request dispatch | One goroutine per inbound message | Handlers execute concurrently and responses can arrive out of request order. Correlate only by `id`. |

There is no sequence number, durable replay, acknowledgement, or exactly-once guarantee. Notifications are invalidation hints: after a reconnect, gap, or dropped frame, refetch authoritative state through the appropriate list/get request or HTTP route.

Request handlers use the server hub's lifetime context, not the socket's lifetime. If a client disconnects after sending a mutation, Git command, or `session.launch`, that work normally continues until completion even though its response cannot reach the old socket. Do not blindly retry an uncertain mutation. First reconcile state with a get/list/status request; use application-level idempotency only where the specific handler provides it.

## Subscriptions and routing

Subscription actions are handled by the gateway before the normal dispatcher:

| Action | Payload | Result |
|--------|---------|--------|
| `task.subscribe` / `task.unsubscribe` | `{"task_id":"..."}` | Records or removes task interest. Current ordinary task/workflow lifecycle broadcasters are global, so this subscription is not a filter for `task.updated`. |
| `session.subscribe` / `session.unsubscribe` | `{"session_id":"..."}` | Routes session-scoped traffic. Subscribe also pushes currently available initial session data, such as Git status. |
| `session.focus` / `session.unfocus` | `{"session_id":"..."}` | Marks an actively viewed session, changes its backend polling tier, and on focus pushes fresh session data. Focused clients receive session-scoped traffic even during a transient subscribe handoff. |
| `user.subscribe` / `user.unsubscribe` | `{}` or `{"user_id":"default-user"}` | Empty uses Kandev's default user. Any other user is rejected with `FORBIDDEN`. |
| `run.subscribe` / `run.unsubscribe` | `{"run_id":"..."}` | Routes future `run.event.appended` messages for one Office run. No snapshot is replayed. |
| `system.metrics.subscribe` / `system.metrics.unsubscribe` | `{}` | Starts/stops delivery of live `system.metrics.updated` snapshots and contributes to metrics collection interest. |

All subscriptions are connection-local and are removed on disconnect. `session.subscribe` and `session.focus` can send an initial live snapshot, but other subscriptions do not replay missed notifications. For an Office run, fetch the REST snapshot before subscribing and reconcile again after a gap.

The first-party web client reconnects by default: at most 10 attempts, starting at 1 second, multiplying delay by 1.5, capped at 30 seconds. On open it flushes frames queued while disconnected, then resubscribes to tasks, sessions, focus state, runs, the default user, and metrics. Its ordinary request timeout defaults to 5 seconds; selected Git and PR requests use longer timeouts. A custom client must implement its own backoff, resubscription, refetch, and timeout policy.

## Core task and session requests

The typical product flow is workspace → workflow → task → session → messages. IDs are opaque strings; do not derive one resource ID from another.

### Create a task

`task.create` requires `workspace_id`, `workflow_id`, and `title`; the handler rejects an empty string, and first-party clients trim it. Normal clients also provide `workflow_step_id`. Optional fields include `description`, `priority`, `state`, `position`, `metadata`, `parent_id`, `plan_mode`, and `repositories`. Each repository entry must provide at least one of `repository_id`, `local_path`, or `github_url`; supported repository fields also include `base_branch`, `checkout_branch`, `pr_number`, `name`, and `default_branch`.

Set `start_agent:true` to start immediately; that requires `agent_profile_id`. `executor_id`, `executor_profile_id`, and `attachments` affect that initial launch. The response embeds the task and, when launch succeeds, adds `session_id` and `agent_execution_id`.

Creation and immediate launch are not one rollback boundary. If the task is created but agent launch fails, the request returns an error while the task can remain in the workspace. List tasks before retrying creation.

```json
{
  "id": "create-task-1",
  "type": "request",
  "action": "task.create",
  "payload": {
    "workspace_id": "workspace-uuid",
    "workflow_id": "workflow-uuid",
    "workflow_step_id": "step-uuid",
    "title": "Implement feature X",
    "description": "Implement it and add focused tests",
    "priority": "high",
    "repositories": [
      {
        "repository_id": "repository-uuid",
        "base_branch": "main"
      }
    ]
  }
}
```

Task states on the wire are `TODO`, `CREATED`, `SCHEDULING`, `IN_PROGRESS`, `REVIEW`, `BLOCKED`, `WAITING_FOR_INPUT`, `COMPLETED`, `FAILED`, and `CANCELLED`. Use `task.move` to change the workflow step and `task.state` to change runtime state; these are separate operations.

### Launch a session

`session.launch` always requires `task_id`. Explicit `intent` values are `prepare`, `start`, `start_created`, `resume`, `workflow_step`, and `restore_workspace`. Other fields are `session_id`, `agent_profile_id`, `executor_id`, `executor_profile_id`, `prompt`, `plan_mode`, `workflow_step_id`, `priority`, `launch_workspace`, `skip_message_record`, `auto_start`, and `attachments`.

If `intent` is omitted, the backend infers it from those fields. That inference exists for first-party compatibility; an external client should send an explicit intent so a later field change does not select a different path.

```json
{
  "id": "launch-session-1",
  "type": "request",
  "action": "session.launch",
  "payload": {
    "task_id": "task-uuid",
    "intent": "start",
    "agent_profile_id": "profile-uuid",
    "prompt": "Implement the task and run focused tests"
  }
}
```

A successful response contains `success`, `task_id`, `state`, and usually `session_id`; it can also contain `agent_execution_id`, `worktree_path`, and `worktree_branch`. Session states use uppercase values such as `CREATED`, `STARTING`, `RUNNING`, `WAITING_FOR_INPUT`, `COMPLETED`, `FAILED`, and `CANCELLED`.

### Send a user turn

`message.add` requires `task_id`, `session_id`, and either non-whitespace `content` or at least one attachment. Optional fields are `author_id`, `model`, `plan_mode`, `has_review_comments`, `attachments`, and `context_files`.

```json
{
  "id": "message-1",
  "type": "request",
  "action": "message.add",
  "payload": {
    "task_id": "task-uuid",
    "session_id": "session-uuid",
    "content": "Also add regression coverage"
  }
}
```

If the agent is busy, use the `message.queue.*` operations rather than retrying `message.add`. Permission prompts are represented in persisted/session message data; answer one with `permission.respond`. Its payload requires `session_id` and `pending_id`, plus `option_id` unless `cancelled:true`; optional `rejected:true` distinguishes an explicit denial from dismissing the prompt.

For a raw diagnostic session, install a WebSocket client such as `websocat`, connect, then paste one request object per line:

```bash
websocat ws://127.0.0.1:38429/ws
```

```json
{"id":"health-1","type":"request","action":"health.check","payload":{}}
```

`websocat` is a third-party diagnostic dependency, not bundled with Kandev. Remember that an originless tool is accepted only because the current endpoint trusts its network boundary.

## Registered request action catalog

The following 309 unique action names have concrete dispatcher registrations in the current backend. The 12 subscription/focus actions in the previous table are additional gateway-handled requests. Availability can still depend on a configured integration, handler mode, or service; registration does not supply credentials, provider installation, a running executor, or permission to external systems.

Payloads are not uniform. Read the corresponding handler request struct before building a non-first-party client. Names below are exact, including `vscode.openFile` and underscore-separated `user_shell.*` actions.

### Workspaces, workflows, and tasks

```text
health.check

workspace.create
workspace.delete
workspace.get
workspace.list
workspace.update

workflow.create
workflow.delete
workflow.get
workflow.history.list
workflow.list
workflow.reorder
workflow.step.create
workflow.step.get
workflow.step.list
workflow.template.get
workflow.template.list
workflow.update

task.archive
task.create
task.delete
task.get
task.list
task.move
task.plan.create
task.plan.delete
task.plan.get
task.plan.implementation_started
task.plan.revert
task.plan.revision.get
task.plan.revisions.list
task.plan.update
task.repository.update
task.session.list
task.session.status
task.state
task.update
task.walkthrough.delete
task.walkthrough.get
```

There are no ordinary dispatcher registrations for direct workflow-step update, delete, or reorder requests. Those operations are available through the workflow HTTP/configuration surfaces and relevant MCP tools.

### Sessions, messages, agents, and orchestration

```text
session.commit_diff
session.cumulative_diff
session.delete
session.ensure
session.file_review.get
session.file_review.reset
session.file_review.update
session.git.commits
session.git.snapshots
session.launch
session.recover
session.rename
session.reset_context
session.set_plan_mode
session.set_primary
session.shell.status
session.stop

message.add
message.list
message.queue.add
message.queue.append
message.queue.cancel
message.queue.drain
message.queue.get
message.queue.remove
message.queue.update
message.search

agent.cancel
agent.launch
agent.list
agent.logs
agent.resize
agent.status
agent.stdin
agent.stop
agent.types

orchestrator.queue
orchestrator.status
orchestrator.stop

permission.respond
```

`agent.launch`, `agent.stdin`, and related calls are lower-level agent controls. Task clients should normally use `session.launch`, `message.add`, and `session.stop`. Constants such as `agent.prompt`, `message.get`, and `session.set_mode` currently have no dispatcher registration and are deliberately absent.

### Repositories, executors, files, Git, and terminals

```text
repository.create
repository.delete
repository.get
repository.list
repository.script.create
repository.script.delete
repository.script.get
repository.script.list
repository.script.update
repository.update

executor.create
executor.delete
executor.get
executor.list
executor.profile.create
executor.profile.delete
executor.profile.get
executor.profile.list
executor.profile.list_all
executor.profile.update
executor.update

environment.create
environment.delete
environment.get
environment.list
environment.update

ssh.sessions.list
ssh.test

workspace.file.create
workspace.file.delete
workspace.file.get
workspace.file.get_at_ref
workspace.file.rename
workspace.file.update
workspace.files.search
workspace.tree.get

worktree.abort
worktree.commit
worktree.create_pr
worktree.discard
worktree.merge
worktree.pull
worktree.push
worktree.rebase
worktree.rename_branch
worktree.reset
worktree.revert_commit
worktree.stage
worktree.unstage

shell.input
shell.subscribe

user_shell.create
user_shell.destroy
user_shell.list
user_shell.park
user_shell.rename
user_shell.resume
user_shell.stop

port.list
port.tunnel.list
port.tunnel.start
port.tunnel.stop

vscode.openFile
vscode.start
vscode.status
vscode.stop
```

`ssh.test` checks an SSH profile and `ssh.sessions.list` reads the backend's active SSH-session view; both are registered with literal action names rather than `actions.go` constants. `user_shell.stop` is a compatibility alias for destroy. Port-tunnel handlers require the tunnel service to be wired. Git semantics and destructive behavior are documented in [Git Operations](git-operations.md); executor-specific filesystem and credential boundaries are documented in [Executors](executors.md).

### Settings, secrets, and automations

```text
user.get
user.settings.update

secrets.create
secrets.delete
secrets.list
secrets.reveal
secrets.update

automation.create
automation.delete
automation.disable
automation.enable
automation.get
automation.list
automation.run.delete
automation.runs.delete_all
automation.runs.list
automation.trigger
automation.trigger.add
automation.trigger.delete
automation.trigger.update
automation.trigger_types
automation.update
automation.webhook.reveal_secret
```

These are trusted local-administration operations. In particular, `secrets.reveal` and `automation.webhook.reveal_secret` make the lack of WebSocket authentication security-critical.

### GitHub and GitLab

```text
github.action_presets.list
github.action_presets.reset
github.action_presets.update
github.check_session_pr
github.cleanup.issue_tasks
github.cleanup.review_tasks
github.issue_watches.create
github.issue_watches.delete
github.issue_watches.list
github.issue_watches.trigger
github.issue_watches.trigger_all
github.issue_watches.update
github.pr_commits.get
github.pr_feedback.get
github.pr_files.get
github.pr_watches.delete
github.pr_watches.list
github.review_watches.create
github.review_watches.delete
github.review_watches.list
github.review_watches.trigger
github.review_watches.trigger_all
github.review_watches.update
github.stats
github.status
github.task_pr.get
github.task_pr.sync
github.task_prs.list

gitlab.action_presets.list
gitlab.action_presets.reset
gitlab.action_presets.update
gitlab.cleanup.issue_tasks
gitlab.cleanup.review_tasks
gitlab.issue_watches.create
gitlab.issue_watches.delete
gitlab.issue_watches.list
gitlab.issue_watches.trigger
gitlab.issue_watches.trigger_all
gitlab.issue_watches.update
gitlab.mr.approve
gitlab.mr.discussion.new
gitlab.mr.discussion.resolve
gitlab.mr.merge
gitlab.mr.set_assignees
gitlab.mr.set_labels
gitlab.mr.unapprove
gitlab.mr_commits.get
gitlab.mr_feedback.get
gitlab.mr_files.get
gitlab.mr_watches.delete
gitlab.mr_watches.list
gitlab.project.branches
gitlab.project.merge_methods.get
gitlab.projects.list
gitlab.projects.search
gitlab.review_watches.create
gitlab.review_watches.delete
gitlab.review_watches.list
gitlab.review_watches.trigger
gitlab.review_watches.trigger_all
gitlab.review_watches.update
gitlab.stats
gitlab.status
gitlab.task_mr.get
gitlab.task_mr.sync
gitlab.task_mrs.list
```

Provider actions make outbound calls with the backend's configured GitHub or GitLab identity. Status and registration do not imply a provider is authenticated, reachable, or authorized for a repository.

### Jira, Linear, Slack, and Sprites

```text
jira.config.delete
jira.config.get
jira.config.set
jira.config.test
jira.projects.list
jira.ticket.get
jira.ticket.transition

linear.config.delete
linear.config.get
linear.config.set
linear.config.test
linear.issue.get
linear.issue.transition
linear.teams.list

slack.config.delete
slack.config.get
slack.config.set
slack.config.test

sprites.instances.destroy
sprites.instances.list
sprites.network_policy.get
sprites.network_policy.update
sprites.status
sprites.test
```

Configuration actions can persist or test credentials and can cause outbound network requests. Sprites actions can inspect or destroy remote instances and change their network policy.

### MCP transport actions

```text
mcp.add_branch_to_task
mcp.archive_task
mcp.ask_user_question
mcp.clarification_timeout
mcp.create_agent_profile
mcp.create_executor_profile
mcp.create_task
mcp.create_task_plan
mcp.create_workflow
mcp.create_workflow_step
mcp.delete_agent_profile
mcp.delete_executor_profile
mcp.delete_task
mcp.delete_task_plan
mcp.delete_walkthrough
mcp.delete_workflow
mcp.delete_workflow_step
mcp.get_mcp_config
mcp.get_task_conversation
mcp.get_task_document
mcp.get_task_plan
mcp.get_walkthrough
mcp.import_workflow
mcp.list_agent_profiles
mcp.list_agents
mcp.list_executor_profiles
mcp.list_executors
mcp.list_related_tasks
mcp.list_repositories
mcp.list_task_documents
mcp.list_tasks
mcp.list_workflow_steps
mcp.list_workflows
mcp.list_workspaces
mcp.message_task
mcp.move_task
mcp.reorder_workflow_steps
mcp.show_walkthrough
mcp.spawn_session
mcp.step_complete
mcp.update_agent
mcp.update_agent_profile
mcp.update_executor_profile
mcp.update_mcp_config
mcp.update_repository_base_branch
mcp.update_task
mcp.update_task_plan
mcp.update_task_state
mcp.update_workflow
mcp.update_workflow_step
mcp.write_task_document
```

These registrations back Kandev's agent/MCP bridge. The subset registered in a process depends on its MCP handler mode and enabled capabilities. They are internal transport shims, not a reason to tunnel MCP calls manually over `/ws`; use the MCP tools exposed to the agent so tool schemas, task/session scoping, and compatibility handling remain intact.

## Emitted notifications and recipients

The following catalog lists actions with current non-test emission paths. It intentionally excludes constants for which no active emitter was found, including the old `acp.*` compatibility constants, `permission.requested`, `input.requested`, `agent.updated`, and `office.activity.created`. Permission and clarification state currently arrives through session message records instead.

### Global broadcasts

Their normal live event path broadcasts to every connected client, which must filter by IDs in the payload. Session subscribe/focus hydration can also send selected state actions directly to the requesting client.

```text
workspace.created
workspace.updated
workspace.deleted
workflow.created
workflow.updated
workflow.deleted
workflow.step.created
workflow.step.updated
workflow.step.deleted
agent.profile.created
agent.profile.updated
agent.profile.deleted
task.created
task.updated
task.deleted
task.state_changed
task.plan.created
task.plan.updated
task.plan.deleted
task.plan.revision.created
task.plan.reverted
task.walkthrough.created
task.walkthrough.updated
task.walkthrough.deleted
repository.created
repository.updated
repository.deleted
repository.script.created
repository.script.updated
repository.script.deleted
executor.created
executor.updated
executor.deleted
executor.profile.created
executor.profile.updated
executor.profile.deleted
executor.prepare.progress
executor.prepare.completed
environment.created
environment.updated
environment.deleted
session.state_changed
session.turn.started
session.turn.completed
session.poll_mode_changed
agent.available.updated
agent.install.started
agent.install.output
agent.install.finished
github.task_pr.updated
github.task_ci_options.updated
github.rate_limit.updated
system.job.update
```

Current lifecycle code broadcasts `session.turn.started` and `session.turn.completed` globally even though their payloads identify a session. `task.subscribe` does not change this global routing.

Office workspace notifications are also global; clients filter on `workspace_id`:

```text
office.task.updated
office.task.created
office.task.moved
office.task.status_changed
office.task.decision_recorded
office.task.review_requested
office.comment.created
office.agent.completed
office.agent.failed
office.agent.updated
office.approval.created
office.approval.resolved
office.cost.recorded
office.run.queued
office.run.processed
office.routine.triggered
office.provider.health_changed
office.route_attempt.appended
office.routing.settings_updated
```

### Session-scoped broadcasts

Clients subscribed to or focused on the matching `session_id` receive:

```text
session.message.added
session.message.updated
session.message.deleted
session.agentctl_starting
session.agentctl_ready
session.agentctl_error
message.queue.status_changed
session.git.event
session.workspace.file.changes
session.shell.output
session.process.output
session.process.status
session.available_commands
session.mode_changed
session.agent_capabilities
session.models_updated
session.info_updated
session.todos_updated
session.prompt_usage
```

File changes are batched for up to 100 ms and flushed immediately at 50 entries. `session.shell.output` also represents shell exit events through its payload. Treat all stream messages as lossy and refresh session/Git status after a gap.

### User-, run-, and metrics-scoped broadcasts

| Routing key | Action | Notes |
|-------------|--------|-------|
| subscribed user | `user.settings.updated` | Requires `user.subscribe`. |
| subscribed user | `session.waiting_for_input` | Local user notification with task/session/title/body fields; it is not delivered by `session.subscribe`. |
| subscribed run | `run.event.appended` | Future events only; there is no replay cursor. |
| metrics subscribers | `system.metrics.updated` | Live resource snapshot; collection interest follows subscribers. |

Routing is an efficiency mechanism, not an access-control boundary. The server does not authenticate resource ownership, global messages can contain IDs for other workspaces, and a client can request arbitrary subscription IDs.

## Reconnect and troubleshooting

- **Upgrade returns 403:** inspect the browser `Origin` and proxy `Host`. The hostnames must match exactly or both be loopback; ports may differ. A scheme other than `http`/`https` or an origin containing a path is rejected.
- **Connection works locally but is unsafe remotely:** this is expected with the current unauthenticated handler and `0.0.0.0` default. Add a protected proxy or bind/firewall the backend before allowing network access.
- **Request times out but the mutation happened:** the response may have been dropped or the socket may have closed while the server-side handler continued. Query current state before deciding whether to retry.
- **Notifications stop or state looks stale:** reconnect, resubscribe, and refetch. Check whether the client is consuming frames quickly enough to avoid the 256-frame drop-new queue.
- **Session stream is missing:** send `session.subscribe`; for an actively displayed session also send `session.focus`. Verify the payload's `session_id` matches exactly.
- **Unknown action:** confirm that the name appears in the registered catalog, not merely as a constant in `actions.go`, and check whether the required optional service was configured.
- **Large attachment disconnects:** keep the complete JSON frame under 32 MiB and obey the action's lower attachment-count and data-size validation too.
- **Responses arrive in the wrong order:** this is normal concurrent dispatch. Match by unique `id`, never arrival order.

Dedicated `/terminal/*target` and `/lsp/:sessionId` WebSockets, plus `/vscode/:sessionId/*path` and `/port-proxy/:sessionId/:port/*path` proxies, are separate protocols. They do not use this JSON envelope and should not be sent `/ws` actions.

Related guides: [Configuration](configuration.md), [Executors](executors.md), [Git Operations](git-operations.md), [Operations](operations.md), [Workflow Import / Export](workflow-import-export.md), and [Workflow Sync](workflow-sync.md).
