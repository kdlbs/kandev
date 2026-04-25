---
status: draft
created: 2026-04-25
owner: cfl
---

# Orchestrate: Personal Assistant Agent, Channels & Agent Memory

## Why

Users interact with kandev through a web UI, but many want to manage their agent fleet from wherever they already are -- Telegram while commuting, Slack during meetings, or email on mobile. There is no way to message an agent outside of the kandev web app, no way for an agent to proactively reach out to the user through external channels, and no way for agents to accumulate knowledge across sessions.

Orchestrate needs a personal assistant agent that acts as the user's always-available interface to the system. The assistant answers questions, delegates work to the CEO or worker agents, relays status updates, and handles proactive tasks (daily digests, monitoring alerts). It communicates through external messaging channels (Telegram, Slack, etc.) and builds up persistent memory so it improves over time.

## What

### Personal assistant agent

- The personal assistant is an agent instance with `role=assistant`.
- It is the user's primary conversational interface outside the web UI.
- Unlike worker agents (which execute tasks and exit), the assistant maintains long-running conversational context through channel tasks.
- The assistant can:
  - Answer questions about workspace state (task status, agent activity, budget).
  - Delegate work by creating tasks and assigning them to the CEO or worker agents.
  - Relay status updates from agents back to the user via the active channel.
  - Run proactive work on a schedule via routines (email digests, monitoring).
  - Act as a control plane: "hire a frontend agent", "pause all agents", "what did the CEO do today".

### Channels

A channel is an external messaging integration that bridges a chat platform to an agent instance via task comments.

#### Channel model

- Each channel is a configuration entry:
  - `id`: unique identifier.
  - `workspace_id`: scoped to workspace.
  - `agent_instance_id`: which agent receives messages from this channel.
  - `platform`: `telegram`, `slack`, `discord`, `email`, `webhook` (generic).
  - `config`: platform-specific JSON:
    - Telegram: `{bot_token, chat_id}`.
    - Slack: `{bot_token, channel_id, signing_secret}`.
    - Discord: `{bot_token, channel_id}`.
    - Email: `{inbound_address, smtp_config}`.
    - Webhook: `{secret}` (generic HTTP ingress).
  - `status`: `active`, `paused`.
  - `task_id`: the long-lived channel task (auto-created when channel is set up).

#### Channel task

- Each channel has a dedicated long-lived task that serves as the conversation thread.
- The task is auto-created when the channel is configured. It stays open indefinitely (never moves to `done`).
- Inbound messages from the platform become comments on the channel task with `source=<platform>` and `reply_channel_id=<channel_id>`.
- The agent's responses (comments posted on the task) with a matching `reply_channel_id` are relayed back to the platform.

#### Message flow

```
User sends Telegram message
  -> Telegram webhook hits kandev endpoint
  -> Create comment on channel task (author=user, source=telegram, reply_channel_id=<id>)
  -> task_comment wakeup fires for the assistant agent
  -> Agent reads comment, processes request
  -> Agent posts reply comment (reply_channel_id=<id>)
  -> Channel relay picks up the comment and sends it back via Telegram API
```

#### Webhook ingress

- Each platform has a webhook endpoint: `POST /api/channels/<channel_id>/inbound`.
- Platform-specific signature verification (Telegram: token validation, Slack: HMAC signing secret, Discord: Ed25519, generic: bearer token or HMAC).
- The endpoint parses the platform-specific payload into a normalized comment (author display name, message text, optional attachments).
- Rate limiting per channel to prevent abuse.

#### Outbound relay

- A background process watches for new comments on channel tasks where the comment was posted by the agent and has a `reply_channel_id`.
- The relay formats the comment for the target platform (markdown -> platform-native formatting) and sends it via the platform API.
- Delivery failures are logged in the activity log. Retries with exponential backoff (3 attempts).

#### Multi-channel

- An agent instance can have multiple channels (e.g. Telegram for personal, Slack for team).
- Each channel has its own channel task, so conversations are separate.
- The agent sees which channel a message came from via the comment's `source` field and can tailor responses accordingly.

### Managing the workspace via chat

- The assistant (or any agent with appropriate permissions) can receive management commands via channels.
- Examples:
  - "What's the status of the auth migration?" -> assistant queries task state, replies with summary.
  - "Hire a frontend agent" -> assistant creates a hire request (approval flow kicks in).
  - "Pause all agents" -> assistant calls the API to pause all worker instances.
  - "What did the CEO do today?" -> assistant queries activity log, replies with digest.
- The assistant's skill defines which commands it understands and how to route them to the orchestrate API. This is instruction-based (skill content), not a hardcoded command parser.

### Proactive work via routines

- The assistant can be the assignee of routines for proactive tasks:
  - **Daily email digest**: routine fires at 8am, assistant creates a summary of overnight agent activity and sends it to the user via their preferred channel.
  - **Monitoring alerts**: routine fires every hour, assistant checks for stuck agents, failed sessions, budget alerts, and notifies the user if anything needs attention.
  - **Weekly report**: routine fires on Monday, assistant compiles a weekly summary of completed tasks, cost breakdown, and agent performance.
- These are standard routines (see [orchestrate-routines](../orchestrate-routines/spec.md)) with the assistant as the assignee. The routine creates a task, the assistant processes it, and posts the result as a comment on the channel task (which relays to the platform).

### Agent memory

Agents need persistent knowledge that survives across sessions -- who the user is, what they prefer, what happened last week, lessons learned from past mistakes. Memory is stored in the database (not the filesystem), because agents run in task workspace directories (repositories, project folders) where agent state should not be mixed with user code.

#### Memory storage

- Memory entries are stored in an `agent_memory` table, scoped per agent instance.
- Each entry:
  - `id`: unique identifier.
  - `agent_instance_id`: which agent owns this memory.
  - `layer`: `knowledge`, `session`, or `operating`.
  - `key`: topic/namespace (e.g. `preferences/formatting`, `people/cfl`, `session/2026-04-25-auth-task`).
  - `content`: text content (markdown).
  - `metadata`: JSON (timestamp, source session ID, status active/superseded, access count).
  - `created_at`, `updated_at`.
- Three layers (PARA-inspired):

**Layer 1: Knowledge** (`layer=knowledge`)
- Structured facts organized by topic key (entities, projects, preferences).
- Facts have metadata: timestamp, source session, status (active/superseded).

**Layer 2: Session notes** (`layer=session`)
- Per-session summaries: what happened, what was decided, what was learned.
- Written by the agent at the end of each session.

**Layer 3: Operating knowledge** (`layer=operating`)
- How the user operates: communication preferences, review style, coding conventions, things that went wrong before.
- Updated when the agent discovers new patterns.
- Loaded into every session as high-priority context.

#### Memory access via skill + CLI

- Memory is accessed through a **memory skill** -- a skill in the registry that teaches the agent the memory protocol (when to read, when to write, what format to use).
- The skill instructs the agent to use a dedicated CLI tool (`kandev-memory` or `curl` against the orchestrate API) to read and write memory entries. This avoids using MCP tools, saving tokens -- the agent calls the CLI via shell commands as instructed by the skill.
- API endpoints exposed by the backend:
  - `GET /api/orchestrate/agents/<id>/memory?layer=<layer>&key=<key>` -- read entries.
  - `PUT /api/orchestrate/agents/<id>/memory` -- create or update an entry.
  - `DELETE /api/orchestrate/agents/<id>/memory/<entry_id>` -- remove an entry.
  - `GET /api/orchestrate/agents/<id>/memory/summary` -- return operating knowledge + recent knowledge entries (for session bootstrap).
- The agent authenticates via the per-run JWT already provided in the session environment.
- The memory skill's SKILL.md contains the full API contract and usage examples, so the agent knows how to call the endpoints without needing MCP tool definitions.

#### Skills over MCP tools

- The memory system deliberately uses a skill (instructions + CLI/curl) instead of MCP tools.
- Skills are cheaper: the agent reads instructions once and calls shell commands. MCP tools add per-call overhead (tool definitions in context, structured I/O parsing).
- This pattern should be preferred going forward for orchestrate capabilities: teach the agent via skill instructions, expose simple API endpoints, let the agent call them with curl or a lightweight CLI.

#### Memory lifecycle

- Agents without the memory skill operate statelessly (current behavior). Memory is opt-in via skill assignment.
- Memory is scoped to the agent instance -- each agent has its own entries. The assistant's memory includes user preferences; a worker's memory includes technical knowledge about the codebase.
- The memory skill teaches the agent when to extract facts, write session summaries, and update operating knowledge.

### Agent self-improvement

- Agents with the permission `can_manage_own_skills` can create or edit skills in the workspace skill registry.
- The agent creates a skill via the orchestrate API (same endpoint the UI uses) and adds it to its own `desired_skills` list.
- Use cases:
  - The assistant learns that the user always wants PR links in task summaries -> creates a "formatting-preferences" skill with those instructions.
  - A developer agent discovers a recurring build pattern -> creates a "project-build" skill with the build commands and conventions.
  - The CEO agent learns delegation patterns that work well -> creates a "delegation-playbook" skill.
- Skill creation by agents goes through the same approval flow if `require_approval_for_skill_changes=true` (workspace setting, default true). The user reviews the proposed skill in the inbox before it takes effect. See [orchestrate-inbox](../orchestrate-inbox/spec.md) for the `skill_creation` approval type and workspace settings.
- Agents can only edit skills they created (tracked via `created_by_agent_instance_id` on the skill). They cannot modify user-created or other agents' skills.

### UI

- `/orchestrate/agents/[id]` (agent detail page) is the central hub for an agent's configuration and state. It shows:
  - **Overview**: name, role, status, org position, current task, budget gauge.
  - **Skills tab**: assigned skills with enable/disable toggles. Agent-created skills marked with an indicator.
  - **Runs tab**: session/run history with status, duration, cost, linked task.
  - **Memory tab**: browsable view of the agent's memory entries, grouped by layer (operating, knowledge, session notes). Each entry shows key, content preview, last updated timestamp. Actions:
    - **View/expand**: read the full content of a memory entry.
    - **Delete**: remove individual entries.
    - **Clear all**: wipe all memory for this agent (with confirmation).
    - **Export**: download all memory entries as JSON or markdown (for backup or migration to another agent).
    - **Search**: filter entries by key or content text.
  - **Channels tab**: list of configured channels with status, platform icon, and setup/edit controls.
- Channel setup wizard: select platform, enter credentials (bot token, chat ID), test connection, activate.
- The sidebar "Agents" section shows channel indicators on agent cards (e.g. a Telegram icon if the assistant has a Telegram channel).

## Scenarios

- **GIVEN** a personal assistant with a Telegram channel configured, **WHEN** the user sends "what's the status of the auth task?" via Telegram, **THEN** the message arrives as a comment on the channel task, the assistant is woken, reads the task state, and replies via Telegram with a status summary.

- **GIVEN** a personal assistant with a Telegram channel, **WHEN** the user sends "hire a QA agent", **THEN** the assistant creates a hire request (approval in inbox). The assistant replies via Telegram: "Submitted a hire request for a QA agent. I'll let you know when it's approved."

- **GIVEN** a routine "Daily Digest" assigned to the assistant with a cron trigger at 8am, **WHEN** the routine fires, **THEN** the assistant creates a task, compiles overnight activity (completed tasks, agent errors, cost summary), and posts the digest as a comment on the Telegram channel task. The digest is relayed to Telegram.

- **GIVEN** an assistant with the memory skill, **WHEN** the user says "I prefer bullet points over paragraphs in summaries", **THEN** the assistant calls `curl PUT /api/orchestrate/agents/<id>/memory` to store this preference as an operating knowledge entry. On subsequent sessions, the assistant reads `GET .../memory/summary` at startup and formats summaries as bullet points.

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
- Filesystem-based memory (agents run in task workspace directories; memory lives in the database to avoid polluting repositories).

## Related specs

- [orchestrate-agents](../orchestrate-agents/spec.md) -- agent instances and permissions (`can_manage_own_skills`)
- [orchestrate-skills](../orchestrate-skills/spec.md) -- skill registry (agents create skills via the same API)
- [orchestrate-scheduler](../orchestrate-scheduler/spec.md) -- `task_comment` wakeup drives the chat flow
- [orchestrate-routines](../orchestrate-routines/spec.md) -- proactive work (digests, monitoring)
- [orchestrate-inbox](../orchestrate-inbox/spec.md) -- approval flow for skill changes and hire requests via chat
