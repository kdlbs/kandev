---
status: draft
created: 2026-04-25
owner: cfl
---

# Office: Personal Assistant Agent, Channels & Agent Memory

## Why

Users interact with kandev through a web UI, but many want to manage their agent fleet from wherever they already are - Telegram while commuting, Slack during meetings, or email on mobile. There is no way to message an agent outside of the kandev web app, no way for an agent to proactively reach out to the user through external channels, and no way for agents to accumulate knowledge across sessions.

Office needs a personal assistant agent that acts as the user's always-available interface to the system. The assistant answers questions, delegates work to the CEO or worker agents, relays status updates, and handles proactive tasks (daily digests, monitoring alerts). It communicates through external messaging channels (Telegram, Slack, etc.) and builds up persistent memory so it improves over time.

## What

- A personal assistant agent (`role=assistant`) is the user's primary conversational interface outside the web UI.
- The assistant maintains long-running conversational context through channel tasks, unlike worker agents which execute tasks and exit.
- Channels SHALL bridge external chat platforms (Telegram, Slack, Discord, email, webhook) to an agent instance via task comments.
- Inbound messages from a channel become comments on the channel task; agent replies are relayed back through the platform.
- The assistant can answer state questions, delegate work, relay status, and run proactive routines.
- Agents with the memory skill SHALL persist knowledge across sessions in a per-agent memory store.
- Agents with `can_manage_own_skills` SHALL create/edit skills in the workspace skill registry, subject to the approval flow when configured.

## Data model

### Channel

| Field | Type | Notes |
|---|---|---|
| `id` | string | PK |
| `workspace_id` | string | FK |
| `agent_instance_id` | string | which agent receives messages |
| `platform` | enum | `telegram` \| `slack` \| `discord` \| `email` \| `webhook` |
| `config` | JSON | platform-specific (tokens, IDs, signing secrets) |
| `status` | enum | `active` \| `paused` |
| `task_id` | string | the long-lived channel task (auto-created at channel setup) |

Platform configs:
- Telegram: `{bot_token, chat_id}`.
- Slack: `{bot_token, channel_id, signing_secret}`.
- Discord: `{bot_token, channel_id}`.
- Email: `{inbound_address, smtp_config}`.
- Webhook: `{secret}` (generic HTTP ingress).

### Channel task

- Auto-created when channel is configured; stays open indefinitely (never `done`).
- Inbound messages become comments with `source=<platform>` and `reply_channel_id=<channel_id>`.
- Agent comments with matching `reply_channel_id` are relayed back to the platform.
- An agent instance can have multiple channels; each has its own channel task.

### `office_agent_memory`

Stored in DB, scoped per agent instance. Optional filesystem export via Sync UI for backup and sharing.

Filesystem layout (one markdown file per entry):

```
agents/ceo/memory/
  operating/
    communication-style.md
    delegation-patterns.md
  knowledge/
    people-cfl.md
    projects-api-migration.md
```

Each file has YAML frontmatter + markdown body:

```markdown
---
category: preference
status: active
created: 2026-04-26
source_session: sess_abc123
---

User prefers bullet points over paragraphs in summaries.
```

Two layers (PARA-inspired):

- **Knowledge** (`layer=knowledge`): structured facts by topic key (entities, projects, preferences). Facts carry timestamp, source session, status (active/superseded).
- **Operating knowledge** (`layer=operating`): how the user operates - communication preferences, review style, coding conventions, things that went wrong before. Loaded into every session as high-priority context.

One file per entry minimizes git conflicts: operating knowledge changes infrequently; knowledge entries are per-topic.

## API surface

### Webhook ingress (per platform)

`POST /api/channels/<channel_id>/inbound`

- Platform-specific signature verification (Telegram token, Slack HMAC signing secret, Discord Ed25519, generic bearer/HMAC).
- Parses platform-specific payload into a normalized comment (author display name, message text, optional attachments).
- Rate limited per channel to prevent abuse.

### Outbound relay

A background process watches for new agent-authored comments on channel tasks that carry a `reply_channel_id`. The relay:
- Formats the comment for the target platform (markdown -> platform-native formatting).
- Sends via the platform API.
- Logs delivery failures in the activity log.
- Retries with exponential backoff (3 attempts).

### Memory API (called from a skill, not MCP)

The agent uses a **memory skill** (instructions + shell commands) rather than MCP tools. This saves tokens: the agent reads the skill once and calls shell commands.

- `GET /api/office/agents/<id>/memory?layer=<layer>&key=<key>` - read entries.
- `PUT /api/office/agents/<id>/memory` - create or update an entry.
- `DELETE /api/office/agents/<id>/memory/<entry_id>` - remove an entry.
- `GET /api/office/agents/<id>/memory/summary` - operating knowledge + recent knowledge entries (session bootstrap).

The agent authenticates via the per-run JWT in its session environment. The skill SKILL.md carries the full API contract and usage examples so the agent does not need MCP tool definitions.

### Skill creation by agents

Agents with `can_manage_own_skills` create/edit skills via the same office API the UI uses, then add them to their own `desired_skills` list. When `require_approval_for_skill_changes=true` (workspace setting, default true), skill creation goes through the `skill_creation` approval flow (see `office/inbox.md`). Agents can only edit skills they created (tracked via `created_by_agent_instance_id` on the skill).

### Message flow

```
User sends Telegram message
  -> Telegram webhook hits /api/channels/<id>/inbound
  -> Create comment on channel task (author=user, source=telegram, reply_channel_id=<id>)
  -> task_comment wakeup fires for the assistant agent
  -> Agent reads comment, processes request
  -> Agent posts reply comment (reply_channel_id=<id>)
  -> Outbound relay picks up the comment and sends it back via Telegram API
```

### Managing the workspace via chat

Assistant commands (skill-instruction-based, not hardcoded):
- "What's the status of the auth migration?" -> queries task state, replies with summary.
- "Hire a frontend agent" -> creates a hire request (approval flow).
- "Pause all agents" -> calls API to pause all worker instances.
- "What did the CEO do today?" -> queries activity log, replies with digest.

### Proactive work via routines

The assistant can be the assignee of routines for proactive tasks:
- **Daily email digest** at 8am - assistant compiles overnight activity and sends via preferred channel.
- **Monitoring alerts** every hour - assistant checks for stuck agents, failed sessions, budget alerts.
- **Weekly report** Mondays - completed tasks, cost breakdown, agent performance.

Standard routines (see `office/routines/` spec) with assistant as assignee. Routine creates task; assistant processes it; result posted as comment on channel task (relayed to platform).

### UI routes

- `/office/agents/[id]` - agent detail hub:
  - **Overview**: name, role, status, org position, current task, budget gauge.
  - **Skills tab**: assigned skills with enable/disable toggles. Agent-created skills marked.
  - **Runs tab**: session/run history with status, duration, cost, linked task.
  - **Memory tab**: browse memory entries grouped by layer (operating, knowledge, session notes). Actions: view/expand, delete entry, clear all (with confirmation), export (JSON or markdown), search by key/content.
  - **Channels tab**: list of channels with status, platform icon, setup/edit controls.
- Channel setup wizard: select platform, enter credentials, test connection, activate.
- Sidebar "Agents" section shows channel indicators on agent cards (e.g. Telegram icon if the assistant has a Telegram channel).

## Permissions

- `can_manage_own_skills` (per agent): gates skill creation by the agent.
- `require_approval_for_skill_changes` (workspace, default true): gates agent-created skills via inbox approval flow.
- Memory is scoped per agent instance: the assistant's memory includes user preferences; a worker's memory includes codebase knowledge. Agents cannot read another agent's memory.

## State machine

The assistant has three lifecycles running in parallel: the agent instance lifecycle (shared with every office agent), the channel lifecycle (per channel row), and the conversation lifecycle (per inbound message on a channel task).

### Agent instance lifecycle

The assistant uses the same state machine as every office agent (`pending_approval` -> `idle` -> `working` -> `paused` -> `stopped`). See [agents.md](./agents.md#state-machine). The only assistant-specific rule: an assistant should never be `stopped` while any of its channels is `active`, because inbound messages would queue wakeups against an inactive agent. Stopping the assistant does not auto-pause its channels — the user must do that explicitly, or accept that inbound messages will create comments that go un-processed until the assistant is reactivated.

### Channel lifecycle

A channel row in `office_channels` has its own status, independent of the agent it's bound to.

- **active**: webhook ingress is accepted, inbound messages create comments, outbound relay delivers agent replies.
- **paused**: webhook ingress returns 200 OK but the inbound message is dropped (no comment, no wakeup); outbound relay still delivers (so the agent can finish a turn already in progress).

Transitions:

| From | To | Trigger | Actor |
|---|---|---|---|
| (none) | active | `POST /office/agents/:id/channels` | user (channel setup wizard) |
| active | paused | `PATCH` status -> `paused` | user |
| paused | active | `PATCH` status -> `active` | user |
| any | (deleted) | `DELETE /office/agents/:id/channels/:channelId` | user |
| any | (deleted) | reconciler sweep when the bound agent row is gone | reconciler |

The channel task created at setup time is never deleted by channel deletion: it remains as a long-lived task in the workspace so the comment history is preserved. Setup is two-phase: insert the channel row, create the task, link `task_id` back, set the assignee. If task creation fails the channel row is rolled back; if the assignee update fails the channel row is rolled back; if linking fails the orphan task is left behind and the user sees the failure surfaced through the setup wizard.

### Conversation lifecycle (per inbound message)

Each inbound message on an `active` channel walks a strict pipeline:

1. **received**: webhook POST to `/api/channels/<channel_id>/inbound`. Body capped at 64 KiB. The `?author=` query param is ignored — the comment author is always `"external"` to prevent spoofing.
2. **verified**: if `webhook_secret` is set, the signature on the incoming request is checked (Telegram: raw token in `X-Telegram-Bot-Api-Secret-Token`; Slack/Discord/webhook: HMAC-SHA256 in `X-Webhook-Signature` or `X-Hub-Signature-256`). Failure -> 401 + activity log entry, conversation ends.
3. **commented**: a row is inserted into the channel task's comments with `author_type="user"`, `author_id="external"`, `source=<platform>`, `reply_channel_id=<channel.id>`.
4. **woken**: the `task_comment` event fires; the assistant's `service.event_subscribers.handleCommentCreated` enqueues a wakeup keyed by `task_comment:<comment_id>` (idempotent — duplicate webhook deliveries do not duplicate wakeups).
5. **processing**: the scheduler claims the wakeup when the assistant has capacity (`status=idle` or `working` with a free slot). Agent runs the standard session preparation flow.
6. **replied**: agent posts one or more comments. Replies destined for the platform carry `reply_channel_id=<channel.id>`; comments without it are kanban-internal (no relay).
7. **relayed**: the channel relay picks up agent-authored comments with a matching `reply_channel_id`, formats per platform, and sends. Retries with exponential backoff up to 3 attempts. Each attempt + final outcome is recorded in `office_activity_log` (`channel.message_relayed` or `channel.delivery_failed`).
8. **done**: the run completes; `office_runs` and `office_run_events` retain the conversation turn for auditing.

The pipeline has no shared `status` column — state is implicit in which tables have rows for the inbound `comment_id`: a verified-but-unprocessed message is a comment without a corresponding run; a processing message is a run in `running`; a delivered reply is an activity log row of type `channel.message_relayed`.

## Failure modes

- **Channel webhook signature invalid**: request rejected with 401; activity log records the rejection.
- **Outbound relay delivery fails**: retried with exponential backoff (3 attempts); persistent failure logged in activity log; comment remains on the channel task with a relay-failed marker.
- **Long channel task history** (500+ comments): the wakeup's `context_snapshot` includes only recent comments (last N or since last session), not the entire history. Agent memory provides long-term context.
- **Memory API call fails**: agent retries via its skill instructions; treated like any other shell command failure.
- **Agent without memory skill**: operates statelessly (current behavior); memory is opt-in.

## Persistence guarantees

The assistant agent itself follows the same rules as any office agent — see [agents.md](./agents.md#persistence-guarantees). Channel-specific and memory-specific guarantees:

What survives a backend restart:

- **Channel rows** persist in `office_channels` (PK `id`): `workspace_id`, `agent_profile_id` (the assistant the channel routes to), `platform`, `config` JSON (bot tokens, chat IDs, signing secrets — stored as opaque JSON), `webhook_secret` (auto-generated 32-byte hex on setup if the caller does not supply one), `status`, `task_id`, `created_at`, `updated_at`. A paused channel stays paused across restart; webhook URL + secret are stable.
- **Channel tasks** persist as normal `tasks` rows with state `IN_PROGRESS`. Channels reference them via `task_id`. Channel tasks never auto-transition to `done`; deleting a channel does not delete its task (history is retained).
- **Channel task comments** persist in `task_comments` with `source=<platform>` and `reply_channel_id=<channel.id>` on inbound rows. Both inbound and agent-authored relay-tagged comments survive restart so the conversation history is intact and the relay can be replayed by an operator if needed.
- **Memory entries** persist in `office_agent_memory` (`UNIQUE(agent_profile_id, layer, key)`). Fields: `id`, `agent_profile_id`, `layer` (`operating` | `knowledge` | session-scoped layers), `key`, `content`, `metadata` JSON, timestamps. Upserts are by `(agent, layer, key)`; deletes are by primary key or owned-by-agent guard (`DeleteAgentMemoryOwned`) to prevent cross-agent deletion.
- **Reconciliation at startup** drops `office_channels` rows whose `agent_profile_id` is no longer present in `agent_profiles` (`infra.Reconciler.reconcileChannels`). Orphan channel rows do not accumulate. The channel task and its comments survive that sweep — only the channel row, webhook secret, and platform config are removed.
- **Filesystem-exported memory** (`agents/<name>/memory/<layer>/<key>.md` produced by the Sync UI) is a downstream snapshot, not authoritative. Wiping or reverting the filesystem copy never changes the DB; re-running Outgoing sync regenerates the files.

What does NOT survive a restart:

- **In-flight outbound relay attempts**: if the backend exits mid-retry, the comment remains in `task_comments` with `reply_channel_id` set but no `channel.message_relayed` activity row. There is no relay queue persisted independently of the comment table; recovery is whatever activity-log inspection or manual re-trigger the user does. The channel relay is not automatically replayed on boot.
- **Per-platform connection state** for streaming integrations (Slack RTM, Discord gateway). All v1 platforms are webhook-driven (kandev exposes ingress) or HTTP-driven (kandev posts outbound) so there is no long-lived socket to lose, but any future streaming transport must treat its connection as ephemeral.
- **Webhook delivery deduplication beyond what the platform sends**: replays of the same webhook by the upstream platform are deduped only at the wakeup layer (`task_comment:<comment_id>` idempotency key). A comment that was created but whose wakeup had not yet been enqueued at shutdown will get a wakeup on the next event-bus re-emission only if the event subscriber re-scans the comment — there is no automatic catch-up scan at boot.

Repo-backed workspaces can sync memory through git via the Sync UI; team review of `agents/*/memory/*.md` in PRs is the intended pattern for sharing learned operating knowledge across team members' kandev instances. The DB remains the source of truth on each member's machine; `ApplyIncoming` overwrites DB rows from disk, `ApplyOutgoing` overwrites disk from DB. Neither direction is run automatically.

There are no TTLs on channels, channel tasks, channel comments, or memory entries. Retention is by user action only.

## Scenarios

- **GIVEN** a personal assistant with a Telegram channel configured, **WHEN** the user sends "what's the status of the auth task?" via Telegram, **THEN** the message arrives as a comment on the channel task, the assistant is woken, reads the task state, and replies via Telegram with a status summary.

- **GIVEN** a personal assistant with a Telegram channel, **WHEN** the user sends "hire a QA agent", **THEN** the assistant creates a hire request (approval in inbox). The assistant replies via Telegram: "Submitted a hire request for a QA agent. I'll let you know when it's approved."

- **GIVEN** a routine "Daily Digest" assigned to the assistant with a cron trigger at 8am, **WHEN** the routine fires, **THEN** the assistant creates a task, compiles overnight activity (completed tasks, agent errors, cost summary), and posts the digest as a comment on the Telegram channel task. The digest is relayed to Telegram.

- **GIVEN** an assistant with the memory skill, **WHEN** the user says "I prefer bullet points over paragraphs in summaries", **THEN** the assistant calls `curl PUT /api/office/agents/<id>/memory` to store this preference as an operating knowledge entry. On subsequent sessions, the assistant reads `GET .../memory/summary` at startup and formats summaries as bullet points.

- **GIVEN** an assistant with `can_manage_own_skills=true` and `require_approval_for_skill_changes=true`, **WHEN** the assistant creates a new skill "daily-digest-format" with specific formatting instructions, **THEN** an approval appears in the inbox. On approval, the skill is added to the registry and to the assistant's `desired_skills`.

- **GIVEN** a user with both Telegram and Slack channels on the same assistant, **WHEN** the user messages via Telegram and a colleague triggers an alert via Slack, **THEN** the assistant handles each on its own channel task independently. Replies go back to the originating platform.

- **GIVEN** a channel task with 500+ comments (long conversation history), **WHEN** the assistant is woken for a new message, **THEN** the wakeup's `context_snapshot` includes only recent comments (last N or since last session), not the entire history. The agent's memory provides long-term context instead.

## Out of scope

- Voice/audio channels (phone, voice assistants).
- Media attachments beyond text (images, files sent via chat platforms). Text-only for v1.
- End-to-end encryption for channel messages.
- Multiple users per channel (one user per workspace; team channels are a separate feature).
- Agent-to-agent direct messaging via channels (agents communicate via tasks and comments, not chat platforms).
- Automatic memory pruning or garbage collection (agent manages its own memory via skill instructions).
- Filesystem-based memory as primary store (memory lives in the database to avoid polluting repositories; filesystem export is for backup/sharing only).
