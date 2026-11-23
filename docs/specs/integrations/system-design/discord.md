---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-DISCORD-001
  - REQ-INTEGRATIONS-DISCORD-002
  - REQ-INTEGRATIONS-DISCORD-003
  - REQ-INTEGRATIONS-DISCORD-004
  - REQ-INTEGRATIONS-DISCORD-005
  - REQ-INTEGRATIONS-DISCORD-006
  - REQ-INTEGRATIONS-DISCORD-007
  - REQ-INTEGRATIONS-DISCORD-008
  - REQ-INTEGRATIONS-DISCORD-009
  - REQ-INTEGRATIONS-DISCORD-010
created: 2026-11-21
owners:
  - Fclem
---

# Discord Task Channels System Design (plugin `kandev-plugin-discord`)

Behavior and acceptance criteria are in
`docs/specs/integrations/requirements/discord.md`. The Host contract the plugin
relies on is in `docs/specs/plugins/system-design/chat-bridge-host-controls.md`.

Implementation plan: [`plan.md`](../../../plans/discord-plugin/plan.md).

Plugin repository: [`Fclem/kandev-plugin-discord`](https://github.com/Fclem/kandev-plugin-discord).

> **Baseline.** Host facts in this spec were verified against `kdlbs/kandev`
> `origin/main` at `018808f8` (2026-11-21).

## Why

Teams that coordinate in Discord have to switch to Kandev to:

- follow an agent's progress;
- answer its questions;
- approve a review step;
- stop it.

They lose context and react late to permission prompts. Without the Kandev UI
they also cannot steer work from a phone.

## What

The feature ships as a runtime plugin in a dedicated repository (manifest id
`kandev-plugin-discord`). It can move to `kdlbs/` if the maintainers adopt it.
It also needs a small set of Kandev Host API additions in `kdlbs/kandev`; see
"Host API additions requested".

- **Workspace ↔ guild binding.** An admin binds each opted-in Kandev workspace
  to exactly one Discord guild (server) that the bot has been invited to.
  - Discord no longer lets applications create guilds: `POST /guilds` was
    removed on 2025-07-15. "Create a server" therefore means the plugin guides
    the operator to create one in Discord and invite the bot.
  - The plugin then provisions the bound guild: an operator role, a task
    category, an archive category, a `#kandev-status` channel, and
    guild-scoped slash commands.
  - Provisioning is idempotent and reuses objects the plugin created earlier.
- **One text channel per task.** For each non-ephemeral task in a bound
  workspace, including subtasks, the plugin SHALL create exactly one text
  channel in that guild.
  - The name is derived from the task identifier and title.
  - The topic carries the title, the identifier, the parent task (for
    subtasks), and a deep link when `kandev_base_url` is configured.
  - Title changes rename the channel at most once per 10 minutes per channel.
- **Channel closing.** When a task is archived or deleted, or reaches the
  configured `close_trigger` state, the plugin SHALL close the channel:
  - `archive` moves it into the archive category and removes send permission;
  - `delete` deletes it.

  Archived channels are deleted after `archive_retention_days`; `0` keeps them
  indefinitely. An unarchived task reopens its channel.
- **Kandev → Discord.** The channel receives:
  - final agent conversation messages of type `message` or `content`;
  - user messages typed in Kandev, when `mirror_user_messages` is on;
  - compact status lines for session start, turn completion, failure, and stop;
  - one pinned task card showing the title, task state, workflow step, session
    state, queue depth, and link;
  - pending agent permission requests, with one button per option;
  - a "Step *<from>* awaits approval → *<to>*" message with an Approve button
    when a workflow review step is waiting for a human.

  The plugin never posts `thinking` content, and it posts tool-call summaries
  only when `post_tool_activity` is on. It never echoes messages it delivered
  itself. Every bot message disables mention parsing, so agent text cannot ping
  `@everyone`, roles, or users.
- **Discord → Kandev.** A message posted in a task channel by an operator is
  delivered to the task's primary session as a user message through the
  existing Host `Messages().Send` path. It is prefixed with the Discord display
  name as `<name> (Discord): …`. That path applies Kandev's existing rules:

  | Session state | Result | Discord reaction |
  |---|---|---|
  | `RUNNING` / `STARTING` | Queued FIFO for the next turn boundary (default limit: 10 per session) | 🕒 |
  | Idle / `WAITING_FOR_INPUT` | Sent immediately | ✅ |
  | Never started | Starts the session | ✅ |
  | `FAILED` / `CANCELLED` | Rejected | ⚠️ with an explanatory reply |
  | Queue full | Rejected | ⚠️ with an explanatory reply |

  Discord never interrupts a running turn.
- **Slash commands.** They are guild-scoped. `/link` works in any channel of
  a bound guild; the others only act inside a task channel.
  - `/link <code>`: redeems a link code generated on the plugin's settings
    page in Kandev and links the caller's Discord account to that Kandev user.
    The reply is ephemeral and never echoes the code.
  - `/status`: replies ephemerally with the task, workflow step, session
    states, queue depth, and pending interactions.
  - `/stop [scope:turn|task]`:
    - `turn` (the default) cancels the current turn and leaves the session
      promptable. This needs the new `CancelTurn` RPC.
    - `task` stops the running execution of every session of the task through
      the existing exact `StopTaskRun`.
  - `/approve [request]` and `/deny [request]` cover everything awaiting a
    human:
    - Agent permission requests. When exactly one request is pending,
      `request` may be omitted; otherwise autocomplete lists them. The buttons
      do the same thing.
    - The workflow review step. When the task's current step awaits human
      approval, `/approve` performs the same transition as the Kandev UI
      Approve button. `/deny` does not apply to steps; posting a message
      requests changes.
    - With no argument, pending permission requests come first, then the step.
    - Both kinds require the linked-account human-response flow described
      under "Host API additions". An unlinked operator gets a deep link.
- **Quick setup and connection test.** Settings > Plugins > Discord combines
  two parts:
  - Kandev's schema-driven form (admin-only; saving restarts the plugin) for
    the bot token (masked), the application ID, and the
    `workspace_guilds` lines.
  - The plugin's `plugin-settings` card above it, which:
    - lists workspace IDs with a "Copy line" button;
    - shows each binding with its invite link;
    - offers **Test connection** per binding, which invokes the plugin's
      `admin`, workspace-scoped action `connection.test`. Actions have no
      instance scope.

  That action:
  - verifies the token with `GET /users/@me`;
  - checks that the token's application matches `application_id`;
  - checks the Message Content intent through the `GET /applications/@me`
    flags;
  - reports bot membership and missing permissions for that workspace's
    guild through `GET /users/@me/guilds`;
  - returns an invite URL when the bot is missing or lacks permissions;
  - once the gateway exists, also reports gateway state, with `4014` meaning
    a disallowed intent.

  The card renders one pass/fail/skip line per check.
- **Health.** A `#kandev-status` channel in each bound guild receives connect,
  disconnect, provisioning, and capacity messages. Fatal Discord errors are
  logged and returned by `connection.status`.

## Data model

All plugin persistence uses Kandev Host state (plugin-namespaced, backed up,
removed on uninstall). Values are JSON objects.

| Scope / scopeID / key | Fields |
|---|---|
| `workspace` / `<workspace_id>` / `guild` | `guild_id`; `operator_role_id`; `viewer_role_id` (optional); `status_channel_id`; `task_category_ids[]` (ordered, ≤50 channels each); `archive_category_ids[]`; `commands_hash`; `provisioned_at` |
| `task` / `<task_id>` / `channel` | `workspace_id`; `guild_id`; `channel_id`; `status` (`creating \| active \| archived \| deleted \| capacity_blocked \| failed`); `synced_title`; `last_renamed_at`; `card_message_id`; `outbound_cursor` `{created_at, message_id}`; `inbound_cursor` (last handled Discord message snowflake); `closed_at`; `delete_after`; `last_error`; `updated_at` |
| `task` / `<task_id>` / `interaction_prompts` | `items[]`: `{interaction_id, kind, session_id, discord_message_id, posted_at}` (source of truth: Host `Interactions()`) |
| `task` / `<task_id>` / `step_approval` | `{session_id, from_step_id, to_step_id, discord_message_id, posted_at}`; at most one |
| `instance` / `""` / `health` | `{token_valid, gateway_state, last_close_code, identify_count_24h, bindings[{index, bot_member, provisioned, missing_permissions[]}], checked_at}` |
| `instance` / `""` / `channel:<channel_id>` | `{task_id, workspace_id}` reverse index |

Invariants:

- There is at most one `channel` record per task, and at most one index entry
  per Discord channel.
- `outbound_cursor` only moves forward.
- The plugin process is the only writer and serializes writes per task.
- Uninstall deletes Host state; Discord channels and roles remain.

## API surface

### Plugin manifest (summary)

```yaml
id: kandev-plugin-discord
api_version: 2
min_kandev_version: "<first release containing the Host additions>"
capabilities:
  events: [task.created, task.updated, task.state_changed, task.deleted, task.moved,
           turn.completed, task_session.state_changed, message.queue.status_changed,
           "permission_request.received.*", clarification.answered]
  api_read: [tasks, sessions, messages, interactions, workspaces, workflows]
  api_write: [messages, execution, interactions, human_responses]
  # Host v2 grants: write:execution, write:interactions, write:human_responses
  state: true
actions:
  - { key: connection.test,   scope: workspace, access: admin }
  - { key: connection.status, scope: workspace, access: admin }
ui: { bundle: /ui/bundle.js }   # plugin-settings quick-setup card
# no webhooks: Discord traffic uses the gateway
```

### Plugin config (`config_schema`)

| Key | Type | Default | Notes |
|---|---|---|---|
| `bot_token` | string, secret, required | — | Discord bot token |
| `application_id` | string, required | — | Command registration and invite URL |
| `workspace_guilds` | string, required | — | Comma- or newline-separated `<workspace_id>=<guild_id>` pairs. 1:1 mapping: each workspace and each guild appears at most once |
| `operator_role_name` | string | `Kandev Operator` | Role created or reused |
| `viewer_role_name` | string | empty | Optional read-only role |
| `allowed_user_ids` | string | empty | Comma-separated Discord user IDs treated as operators |
| `kandev_base_url` | string | empty | Enables deep links |
| `close_trigger` | enum `archived \| terminal` | `archived` | `terminal` also closes on `COMPLETED`/`CANCELLED` |
| `closed_channel_action` | enum `archive \| delete` | `archive` | |
| `archive_retention_days` | integer | `30` | `0` = keep |
| `mirror_user_messages` | boolean | `true` | |
| `post_tool_activity` | boolean | `false` | |
| `live_update_interval_seconds` | integer | `30` | `0` = turn boundaries only |
| `inbound_catchup_max_age_minutes` | integer | `15` | |
| `include_ephemeral_tasks` | boolean | `false` | Quick-chat tasks |

### Existing Host surfaces used (already on `origin/main`)

| Need | Host surface |
|---|---|
| Task list/get with archive state and step | `Tasks().List/Get`; `Task.ArchivedAt`, `Task.WorkflowStepID`, `TaskFilter.IncludeArchived` |
| Deliver Discord messages | `Messages().Send` (`api_write: messages`); queue, send, or start semantics |
| Outbound content | `Messages().List` (sanitized) |
| Pending permission requests and questions | `Interactions().ListPending/Get` (`api_read: interactions`) |
| Stop a task's execution | `HostExecutionCommands(host).StopTaskRun` (`api_write: execution` + `host.v2.write:execution`) |
| Settings card calls | Manifest `actions` with `access: admin`; plugin config routes are admin-only |

### Host API additions (`kdlbs/kandev`)

Decided by the maintainers on
[#5420](https://github.com/kdlbs/kandev/issues/5420#issuecomment-7083766483)
and recorded in
[ADR 2026-11-22-plugin-external-chat-task-control](../../../decisions/2026-11-22-plugin-external-chat-task-control.md).

1. **`CancelTurn`.** Interrupt the current turn without stopping the session,
   matching WebSocket `agent.cancel` and `orchestrator.Service.CancelAgent`.
   It belongs to the exact execution commands (`host.v2.write:execution`) and
   is fenced like `StopTaskRun` by the session resource version, the
   execution ID, and the management claim.
2. **Human responses from linked Kandev users.**
   - A workspace manager grants `host.v2.write:human_responses` (manifest
     `api_write: [human_responses]`) in Settings > Plugins > Workspace access.
   - A Kandev user generates a short-lived link code on the plugin's settings
     page and runs `/link <code>` in Discord; the plugin redeems it with
     `LinkExternalIdentity`. The user and an instance admin can revoke links.
   - With the grant and an active link, the plugin calls
     `RequestHumanResponseReceipt` for one target revision and exact payload.
     The Host mints a single-use, five-minute receipt only when the linked
     Kandev user holds `session.control` on the workspace.
   - Exact `RespondPermission`/`AnswerClarification`/`ApproveStep` consume
     the receipt. Kandev records the Kandev user and the surface, and the
     resolved interaction shows them ("alice via discord").
   - An unlinked Discord user gets a deep link to Kandev instead of an action.
3. **Workflow step approval.**
   - `ApproveStep` reuses `task/service.ApproveSession`, the path behind
     `POST /api/v1/sessions/:id/approve` and the UI Approve button, with a
     `step_approval` receipt from item 2.
   - `GetStepApproval(session_id)` (`api_read: sessions`) returns
     `{awaiting, from/to step id and name}`. `awaiting` is true when the
     session is not running, `review_status` is not `approved`, and either
     `review_status` is `pending` or the current step has an
     `on_turn_complete` transition with `requires_approval=true` and a turn
     completed after the task entered the step.
   - Office approval stages (participant quorum) return `Unimplemented`.
4. **`Message.Source`** on the `Message` read DTO. It carries the persisted
   `metadata.source` only (for example `plugin:<id>`), so a bridge can
   suppress its own echoes without subscribing to the high-volume
   `message.*` events.
5. **`raw_content` stays** in `message.*` event payloads to avoid breaking
   existing consumers. The plugin reads message text from `content` or
   `Messages().List`, never `raw_content`. The public docs say so.
6. **Docs** for items 1–5 in `docs/public/plugins-authoring.md` and
   `plugins-manifest.md`.

Error mapping for the additions follows the exact-command conventions:

- typed outcomes `APPLIED`, `ALREADY_APPLIED`, `CONFLICT`, `UNAVAILABLE`,
  `UNSUPPORTED`;
- `PermissionDenied`, or a `DENIED` command result, without the capability,
  the grant, or an active link;
- `CONFLICT` with reason `step_not_awaiting_approval` (the command form of
  `FailedPrecondition`) when nothing awaits approval;
- `Unimplemented` for office stages on the read, `UNSUPPORTED` on the command.

### Discord surface

- Gateway v10 with intents `GUILDS | GUILD_MESSAGES | MESSAGE_CONTENT`.
  `MESSAGE_CONTENT` is privileged. Enabling it in the Developer Portal is
  sufficient for an unverified bot in fewer than 100 guilds.
- The invite URL is
  `https://discord.com/oauth2/authorize?client_id=<application_id>&scope=bot%20applications.commands&permissions=<P>`.
  `P` covers `VIEW_CHANNEL`, `SEND_MESSAGES`, `READ_MESSAGE_HISTORY`,
  `ADD_REACTIONS`, `EMBED_LINKS`, `MANAGE_CHANNELS`, `MANAGE_ROLES`, and
  `PIN_MESSAGES`. It does not include `ADMINISTRATOR`.
- Interactions (commands, buttons, autocomplete) arrive over the gateway. There
  is no HTTP interactions endpoint and no public webhook.

## State machine (task channel)

| From | Trigger | To |
|---|---|---|
| (none) | `task.created`, or reconcile finds an open task without a record | `creating` |
| `creating` | Channel created and index written | `active` |
| `creating` | Error `30013` (500 channels) or no category capacity | `capacity_blocked` (retried by reconcile; reported in `#kandev-status`) |
| `creating` | Non-retryable Discord error (403) | `failed` (retried after config change or restart) |
| `active` | Task archived, deleted, or matching `close_trigger` | `archived` or `deleted` (per `closed_channel_action`) |
| `archived` | Task unarchived or leaves the terminal state | `active` |
| `archived` | `delete_after` passed | `deleted` |
| any | Channel deleted manually in Discord | `deleted`; the next reconcile recreates it if the task is still open and notes it in `#kandev-status` |

## Permissions

- Only events from bound guilds are processed. DMs, other guilds, and messages
  from bots or webhooks (including the plugin's own) are ignored.
- **Operator**: a member with the operator role, or whose ID is in
  `allowed_user_ids`.
  - Only operators can send task messages, use `/stop`, `/approve`, and
    `/deny`, and press the approval buttons.
  - Roles are checked from each gateway payload on every action.
- A non-operator who posts gets one ⛔ reaction, and nothing is delivered.
  Mutating commands from non-operators get an ephemeral refusal.
- `/status` is available to anyone who can see the channel.
- Category overwrites:
  - `@everyone`: `VIEW_CHANNEL` denied;
  - operator role: view and send;
  - viewer role: view only;
  - the bot: what it needs.
- **Trust boundary.** Discord operators act with the plugin's authority on
  every task in the bound workspace, regardless of their Kandev membership,
  for chat messages and `/status`.
  - Their messages are recorded as `author_type=user` with
    `source=plugin:kandev-plugin-discord`.
  - Anyone who can grant the operator role in the guild can control tasks:
    the guild owner and holders of `MANAGE_ROLES`.
  - `/stop` uses the plugin's `host.v2.write:execution` grant.
  - Approvals and answers act for a **linked Kandev user** only. They need a
    workspace manager's `host.v2.write:human_responses` grant, an active link
    for the Discord user, and that Kandev user's `session.control` on the
    workspace. Kandev records the Kandev user and the Discord surface.
- **Kandev-side config.** Plugin config, enable/disable, and install are
  admin-only on `origin/main` (`RequireAdmin` on `/api/plugins/*` mutations).
  The settings card's actions use `access: admin`.
- **Data egress.** Agent output leaves Kandev for Discord, a third party.
  Thinking is never posted, and tool activity is posted only when opted in.

## Failure modes

- **Gateway disconnect.**
  - The plugin resumes with `resume_gateway_url` and `session_id`, or
    re-identifies with jittered backoff.
  - It stays under 1000 identifies per 24 h and refuses to connect after 900.
  - Close codes `4004`, `4013`, and `4014` are fatal: the plugin stops and
    reports instead of retrying.
- **Plugin down or Kandev restart.** Discord does not replay gateway events
  after a new identify.
  - On `READY` the plugin reconciles channels, outbound cursors, and pending
    interactions (from `Interactions().ListPending`).
  - It fetches each active channel's messages after `inbound_cursor` and
    delivers those no older than `inbound_catchup_max_age_minutes`.
  - Older messages get ⚠️ and the reply "not delivered — the bridge was
    offline".
- **Dropped Kandev events.** Events are triggers only. Outbound content comes
  from `Messages().List` after `outbound_cursor`, and a reconcile every
  5 minutes closes any gap.
- **Discord rate limits.**
  - The plugin honors per-route buckets and `retry_after`, and keeps a global
    budget of at most 40 req/s.
  - Sends are serialized per channel. Card edits are debounced to 10 s and
    renames to 10 min.
  - A 401 halts all REST calls. A 403 or 404 on a channel marks it `failed`.
    Together these stay far from the 10,000 invalid requests per 10 min
    Cloudflare ban.
- **Kandev rejections.**
  - Queue full or terminal session: ⚠️ reply.
  - An interaction already resolved (`CONFLICT` or stale revision): the prompt
    is edited to "resolved elsewhere".
  - `ApproveStep` `CONFLICT` `step_not_awaiting_approval`: ephemeral reply,
    and the prompt is edited to "no longer pending".
  - Missing grant: ephemeral reply "an admin must grant Discord approvals in
    Kandev". Unlinked account: ephemeral reply with a deep link and `/link`
    instructions.
- **Content limits.** Messages are split at 2000 characters on line or
  code-fence boundaries, with at most 5 chunks, followed by "truncated — open
  in Kandev".
- **Interaction deadline.** Every interaction is deferred within 3 s;
  follow-ups use the 15-minute token.

## Persistence guarantees

- Survives restart and upgrade (Host state): provisioning records, channel
  mappings, cursors, prompt records, and cached health.
- Does not survive: the event dedupe set, the gateway session (a restart
  re-identifies), and send queues. Reconcile restores them from cursors and
  `Interactions()`.

## Scenarios

### Channel lifecycle

- **GIVEN** a bound workspace and a provisioned guild, **WHEN** task "Fix
  login" (`KAN-42`) is created, **THEN** channel `kan-42-fix-login` appears
  under the task category within 10 s, with a pinned task card.
- **GIVEN** the plugin was down while three tasks were created, **WHEN** it
  starts, **THEN** reconcile creates three channels and no duplicates.
- **GIVEN** `close_trigger=archived` and `closed_channel_action=archive`,
  **WHEN** the task is archived, **THEN** the channel moves to the archive
  category and becomes read-only.
- **GIVEN** the guild has 500 channels, **WHEN** a task is created, **THEN**
  its record is `capacity_blocked` and `#kandev-status` reports it.

### Kandev → Discord

- **GIVEN** an agent turn that produced two agent messages, **WHEN**
  `turn.completed` arrives, **THEN** both messages are posted in order, and
  re-delivery posts nothing new.
- **GIVEN** an agent message containing `@everyone`, **WHEN** it is posted,
  **THEN** no mention notification is sent.

### Discord → Kandev

- **GIVEN** an operator posts while the session is `RUNNING`, **WHEN** the
  plugin delivers it, **THEN** it is queued, gets 🕒, and is not echoed back.
- **GIVEN** `WAITING_FOR_INPUT`, **WHEN** an operator posts, **THEN** a new
  turn starts and the message gets ✅.
- **GIVEN** a non-operator posts, **WHEN** the plugin processes it, **THEN**
  nothing reaches Kandev and the message gets ⛔.
- **GIVEN** a full queue (10 entries), **WHEN** an operator posts, **THEN** ⚠️
  and a "queue full" reply.

### Stop

- **GIVEN** a running turn, **WHEN** an operator runs `/stop`, **THEN** the
  turn is cancelled, the session becomes `WAITING_FOR_INPUT`, and a status line
  is posted.
- **GIVEN** a running task, **WHEN** `/stop scope:task`, **THEN** each running
  session's execution stops.

### Approvals

- **GIVEN** a pending permission request posted with buttons, the grant, and
  an operator linked to a Kandev user with `session.control`, **WHEN** the
  operator clicks "Allow once", **THEN** Kandev records the response for that
  Kandev user via Discord, the buttons are removed, and the message shows who
  approved.
- **GIVEN** an operator whose Discord account is not linked, **WHEN** they
  click a button, **THEN** an ephemeral reply links to the request in Kandev
  and explains `/link`, and nothing changes in Kandev.
- **GIVEN** the request was answered in the Kandev UI, **WHEN** an operator
  clicks a button, **THEN** the reply is "already resolved" and the message is
  marked resolved.
- **GIVEN** a step whose `on_turn_complete` transition has
  `requires_approval=true`, **WHEN** the turn completes, **THEN** the channel
  shows "Step *<from>* awaits approval → *<to>*" with Approve.
- **GIVEN** that message and a linked operator, **WHEN** the operator
  approves, **THEN** the task moves to *<to>* exactly as with the Kandev
  Approve button, `review_status=approved`, and the message shows who
  approved.
- **GIVEN** the step was already approved in Kandev, **WHEN** `/approve`,
  **THEN** "nothing awaiting approval", and the task does not move.
- **GIVEN** no workspace manager granted `human_responses`, **WHEN** an
  operator approves anything, **THEN** an ephemeral reply explains the missing
  grant, and nothing changes in Kandev.
- **GIVEN** a Kandev user generated a link code, **WHEN** they run
  `/link <code>` in the bound guild within ten minutes, **THEN** the plugin
  confirms the link, and the link appears on the plugin's settings page in
  Kandev, where the user or an admin can revoke it.

### Setup and health

- **GIVEN** an admin saved token, application ID, and a `workspace_guilds`
  line, **WHEN** they click Test connection for that binding, **THEN** the
  card shows token, application, intent, membership, and permission results
  within 15 s (the action timeout), plus an invite link when needed.
- **GIVEN** the token is revoked, **WHEN** the gateway closes with `4004`,
  **THEN** the plugin stops reconnecting and makes no further REST calls.

## Out of scope

- Bots creating guilds (Discord no longer allows it); guild ownership
  transfer.
- Creating Kandev tasks from Discord.
- Attachments.
- Voice and DMs.
- Several guilds per workspace or several workspaces per guild.
- Linking a Discord account to several Kandev users, or linking from
  Discord without a code generated in Kandev.
- Answering clarification bundles from Discord. The delegation covers them,
  but the UI for multi-question forms is deferred.
- Interrupting a running turn with a message.
- Streaming token-level edits.
- A public webhook or HTTP interactions endpoint.
- A `forum_thread` mode (rejected 2026-11-21).
- Office approval stages (participant quorum).
- Arbitrary `/move`.

## Decisions (2026-11-21)

Requester decisions:

1. **Text channels.** One text channel per task; no forum-thread mode.
2. **`/approve` scope.** `/approve` covers agent permission requests *and*
   workflow review-step approvals; see Host additions 2–3.
3. **Admin-gating plugin management.** Left to the maintainers. On
   `origin/main` plugin management mutations are already admin-only, so
   nothing remains to decide for this feature.

Planner defaults for the minor questions:

4. **Binding.** 1:1 workspace ↔ guild.
5. **Subtasks.** Subtasks get their own channel, and the topic names the
   parent.
6. **Attribution.** Chat messages carry the Discord display name as a text
   prefix. Approvals and answers are attributed by the Host to the linked
   Kandev user plus the Discord surface.
7. **Health.** Health goes through plugin logs, `#kandev-status`, and the
   admin `connection.status` action; there is no `ReportStatus` RPC.

## Maintainer decisions (2026-11-22)

From [#5420](https://github.com/kdlbs/kandev/issues/5420#issuecomment-7083766483):

- Item 2: account linking gated by the admin grant. A plugin-attested actor
  alone and Host-verified Discord signatures are rejected.
- Item 1: `CancelTurn` is accepted with the same fencing and grants as
  `StopTaskRun`.
- Item 3: `ApproveStep`/`GetStepApproval` are accepted on top of the item 2
  receipt; Office stages return `Unimplemented`.
- Item 4: `Message.Source` is accepted, limited to `metadata.source`.
- Item 5: `raw_content` stays in `message.*` payloads; the docs are corrected.
