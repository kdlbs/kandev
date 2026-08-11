---
status: draft
created: 2026-08-11
owner: Kandev
---

# External Agent Permission Resolution

Decision:
[ADR-2026-08-11-live-agent-permission-authority](../../decisions/2026-08-11-live-agent-permission-authority.md)

Implementation plan:
[External Agent Permission Resolution](../../plans/external-agent-permission-resolution/plan.md)

## Why

People using Kandev through an external MCP client cannot currently discover that an agent is
blocked on a command or tool permission prompt, show the provider's choices, or submit the
person's choice. They must open the Kandev session UI even when their active control surface is
another client such as Hermes.

## What

- The external Kandev MCP endpoint exposes one read-only tool that lists pending agent
  permission requests for a task, optionally limited to one task session.
- Each result identifies the exact task, task session, Kandev request generation, provider
  pending request, and originating tool call. It includes creation time, pending status, a safe
  human-readable action presentation, and every provider-offered choice with its immutable
  option ID, label, and kind.
- The external endpoint exposes one mutation tool that selects exactly one option from exactly
  one listed request. It accepts no command, tool arguments, replacement option, cancellation
  flag, or free-form approval text.
- A resolution is accepted only while the complete task/session/request/pending identity still
  names the live request and the selected option is byte-for-byte one of that request's offered
  option IDs.
- Listing and resolution enforce the same per-user task and task-session ownership rules as the
  web UI. Authentication-disabled installs retain their existing synthetic single-user scope;
  authenticated external clients use their personal access token identity.
- Competing, duplicate, replayed, replaced, withdrawn, and expired resolutions never act on a
  newer request. Each returns a stable, descriptive failure.
- The existing session UI continues to show and answer permission prompts with the same choices
  and provider-neutral behavior. It carries the Kandev request generation when resolving so it
  receives the same stale-request protection as MCP clients.
- Successful and failed resolution attempts are auditable in the permission-request transcript:
  actor identity, caller surface, task/session/request/pending identity, selected option ID and
  kind, selection/finalization times, and outcome are retained. Raw credentials, environment
  values, MCP arguments, and unsanitized command details are not audit fields or logs.
- Auto-approval remains opt-in and unchanged. Adding these tools does not enable or broaden
  automatic approval.

## Data model

### Live pending request

This state exists only in the owning agentctl process while the provider waits:

| Field | Type | Contract |
|---|---|---|
| `request_id` | string | Kandev-generated opaque generation ID, unique per live prompt |
| `pending_id` | string | Provider-originated or Kandev fallback pending ID |
| `tool_call_id` | string | Provider tool-call identity when supplied |
| `title` | string | Provider presentation title, redacted for public output |
| `action_type` | enum | `command`, `file_write`, `file_read`, `network`, `mcp_tool`, or `other` |
| `action_details` | internal object | Untouched provider data; never returned wholesale by the external contract |
| `options` | ordered array | Immutable copies of offered `option_id`, `name`, and `kind` |
| `created_at` | timestamp | UTC creation time |
| `state` | enum | `pending`, `resolving`, or terminal/removed |

The live entry is removed after response delivery or provider cancellation. Neither it nor its
answerability survives loss of the owning agent execution.

### Permission audit projection

The existing `permission_request` task-session message stores `request_id`, `pending_id`, the
display options, and status in metadata. Its resolution audit records:

| Field | Type | Contract |
|---|---|---|
| `claim_id` | string | Opaque identity for one serialized resolution attempt |
| `actor_user_id` | string | Kandev user identity; synthetic single-user identity is recorded as synthetic |
| `actor_kind` | enum | `browser`, `personal_access_token`, `automation`, or `synthetic` |
| `source` | enum | `web`, `external_mcp`, or existing internal automation source |
| `request_id` / `pending_id` | string | Exact request identities selected |
| `option_id` / `option_kind` | string | Exact provider-offered option identity and semantics |
| `selected_at` / `finalized_at` | timestamp | UTC audit times; finalization may be absent after abrupt shutdown |
| `result` | enum | `dispatching`, `accepted`, `stale`, `expired`, `failed`, or `indeterminate` |

Only one claim can move a pending message into resolution. Later attempts append or return their
non-mutating replay/stale outcome without replacing the original successful selection.

## API surface

### `list_pending_agent_permissions_kandev`

Input:

```json
{
  "task_id": "task-uuid",
  "session_id": "optional-task-session-uuid"
}
```

Output:

```json
{
  "task_id": "task-uuid",
  "permissions": [
    {
      "task_id": "task-uuid",
      "session_id": "task-session-uuid",
      "request_id": "kandev-request-uuid",
      "pending_id": "provider-pending-id",
      "tool_call_id": "provider-tool-call-id",
      "title": "Run command",
      "action": {
        "type": "command",
        "description": "Run a command",
        "command": "git status",
        "cwd": "/workspace/project",
        "redacted": false
      },
      "options": [
        {"option_id": "allow-once", "name": "Allow once", "kind": "allow_once"},
        {"option_id": "reject-once", "name": "Deny", "kind": "reject_once"}
      ],
      "created_at": "2026-08-11T12:00:00Z",
      "status": "pending"
    }
  ],
  "total": 1
}
```

Results are sorted by `created_at`, then `request_id`. An authorized task with no live requests
returns an empty array. `session_id`, when supplied, must belong to `task_id` even when neither
task nor session currently has a live execution.

`action` is an allowlisted projection. It may include a credential-redacted command and working
directory, a file path without file contents or diff, a network destination without headers or
query secrets, or MCP server/tool names without arguments. Unknown/provider-specific fields,
option metadata, headers, environment entries, and raw arguments are omitted. `redacted` is true
when any returned presentation text was changed to protect a credential.

### `resolve_agent_permission_kandev`

Input:

```json
{
  "task_id": "task-uuid",
  "session_id": "task-session-uuid",
  "request_id": "kandev-request-uuid",
  "pending_id": "provider-pending-id",
  "option_id": "allow-once"
}
```

Success output identifies the same tuple, the accepted option ID and kind, the resolver source,
and `status: "resolved"`. The tool never accepts `cancelled`, `rejected`, arbitrary option
metadata, or a caller-authored command. Reject choices work by selecting the provider's original
`reject_once` or `reject_always` option.

Stable failure codes are:

- `task_or_session_not_found`: missing or unauthorized task/session; does not reveal foreign IDs;
- `permission_not_found`: the request identity was never visible in this authorized session;
- `permission_stale`: the provider withdrew or replaced the exact request;
- `permission_already_resolved`: the exact request has a terminal audit result;
- `permission_resolution_in_progress`: another caller holds its audit claim;
- `permission_option_not_offered`: `option_id` is not in the immutable live option set;
- `permission_audit_failed`: the durable claim could not be recorded, so no option was sent;
- `permission_delivery_failed`: the live request disappeared after claim; the audit records the
  failed or indeterminate result.

## State machine

| Current state | Trigger | Next state | Observable result |
|---|---|---|---|
| absent | provider requests permission | pending | list includes the request |
| pending | authorized resolver claims an offered option | resolving | competing resolution fails without acting |
| resolving | agentctl accepts and forwards the exact option | resolved | request disappears; audit is finalized |
| pending/resolving | provider withdraws, turn ends, or execution is replaced | expired | request disappears; strict resolution fails stale/expired |
| resolved/expired | same or different client retries | unchanged | replay fails and never reaches a newer request |

## Permissions

- A caller may list a task only if the existing task-service authorization permits access to its
  workspace. Supplying `session_id` additionally requires the server-owned task/session relation.
- Resolution authorizes the task/session pair before runtime, message, or option lookup.
- Admin role does not grant access to another user's workspace. A personal access token carries
  only its owning user's scope.
- Agent-provided task, session, or user IDs are never trusted. Task/session identity comes from
  Kandev records and the owning live execution.
- The `external_mcp` audit source is derived only from the process-local transport attestation set
  by the authenticated external `/mcp` bridge. Shared in-session dispatcher calls and raw
  WebSocket traffic cannot supply or forge that source.
- Unauthorized and nonexistent task/session identities use the same not-found result.

## Failure modes

- A task or session without a live execution lists no requests; resolution fails without starting
  or resuming an agent.
- A malformed live request with no options remains visible with an empty options array but cannot
  be resolved through MCP. The existing provider cancellation behavior remains available
  internally.
- Audit claim persistence fails closed: no response is delivered to the agent.
- If the provider request disappears after the durable claim, the operation reports delivery
  failure and finalizes the audit as stale, failed, or indeterminate. It never retries against a
  different live request.
- If final audit persistence fails after delivery, the pre-delivery claim remains durable with an
  honest non-terminal/indeterminate result; the tool reports failure and replay remains blocked.
- Presentation redaction failure omits the affected field rather than returning raw details.
- One session failing to enumerate does not expose data from another session. The tool returns a
  descriptive failure rather than silently presenting a partial task-wide list as complete.

## Persistence guarantees

- Live answerability and immutable option authority belong to the active agentctl execution and do
  not survive loss of that execution.
- Permission messages and resolution audit metadata survive backend restarts according to existing
  task-session message retention.
- A restart never reconstructs an actionable permission solely from message history. Listing stays
  empty until a live runtime reports the same current request.
- Historical terminal and indeterminate audit records remain replay barriers for their exact
  `request_id`; they do not block unrelated later requests with a different generation.

## Scenarios

- **GIVEN** an authorized task with two live permission requests in different sessions, **WHEN** a
  client lists by task without `session_id`, **THEN** both safe request snapshots are returned in
  deterministic order with their owning session IDs.
- **GIVEN** one session has a live command request, **WHEN** a client lists that task and session,
  **THEN** it receives the exact request/pending IDs, redacted command presentation, cwd when
  available, and the provider's ordered option IDs/names/kinds without option metadata.
- **GIVEN** a command or MCP request contains credentials in headers, environment, URL query,
  arguments, or presentation text, **WHEN** it is listed or audited, **THEN** no credential or raw
  hidden environment value appears in the MCP result, persisted audit, or structured log.
- **GIVEN** an authorized live request and one of its offered options, **WHEN** the client resolves
  the complete identity tuple, **THEN** the provider receives that exact option once and the
  transcript audit identifies the resolver, option, time, source, and accepted result.
- **GIVEN** a caller can access a task but supplies a session belonging to another task, **WHEN** it
  lists or resolves, **THEN** the request fails before live runtime state is read.
- **GIVEN** a caller supplies another user's task or session, **WHEN** it lists or resolves, **THEN**
  the request returns the same not-found result as an unknown identity and changes nothing.
- **GIVEN** an option ID not offered by the live request, **WHEN** a client resolves it, **THEN** the
  call returns `permission_option_not_offered`, records no successful claim, and sends no response.
- **GIVEN** a provider reuses `pending_id` for a newer prompt, **WHEN** a client submits the old
  `request_id`, **THEN** the call returns `permission_stale` and the newer prompt remains pending.
- **GIVEN** two clients concurrently resolve the same request, **WHEN** their calls race, **THEN**
  exactly one option reaches the provider and the loser receives in-progress or already-resolved.
- **GIVEN** a successfully resolved request, **WHEN** the same tuple is replayed, **THEN** the call
  returns `permission_already_resolved` and performs no second provider action.
- **GIVEN** the provider withdrew or expired a listed request, **WHEN** a client resolves it,
  **THEN** the call returns `permission_stale` or `permission_delivery_failed`, finalizes the audit,
  and never targets a later request.
- **GIVEN** an authorized task or session has no live pending request, **WHEN** a client lists it,
  **THEN** the call succeeds with `permissions: []` and `total: 0`.
- **GIVEN** the existing web UI displays a permission prompt, **WHEN** the user selects one of its
  choices, **THEN** the UI submits the request generation and retains its existing pending,
  approved/rejected, and expired behavior.

## Out of scope

- Office approval records, workflow gates, clarification questions, or generic human-in-the-loop
  approval unification.
- Free-form commands, edited tool arguments, synthesized options, or approving a request by title.
- Globally enabling auto-approval or changing agent-profile approval defaults.
- Reconstructing provider permission prompts after their owning execution is gone.
