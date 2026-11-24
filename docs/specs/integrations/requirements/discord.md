---
status: draft
system: integrations
created: 2026-11-23
owners:
  - Fclem
---

# Discord Task Channels Requirements

> Technical design, data model, and decisions:
> `docs/specs/integrations/system-design/discord.md`.
> Plugin repository:
> [`Fclem/kandev-plugin-discord`](https://github.com/Fclem/kandev-plugin-discord).

## Overview

Teams that coordinate in Discord have to switch to Kandev to follow an agent,
answer its questions, approve a review step, or stop it. The Discord plugin
mirrors each task into a Discord channel and relays chat, commands, and
approvals both ways, using only the Host contract.

The plugin acts on Kandev only through the linked-account controls in
`docs/specs/plugins/requirements/chat-bridge-host-controls.md`.

## Terminology

- **Binding:** a one-to-one pair of a Kandev workspace and a Discord guild.
- **Operator:** a Discord member who holds the configured operator role.
- **Task channel:** the Discord channel that mirrors one task.

## Requirements

### REQ-INTEGRATIONS-DISCORD-001: Configuration and Discord API seam

The plugin has a validated configuration and a narrow Discord API boundary.

- **AC-INTEGRATIONS-DISCORD-001.1:** When the plugin is packaged, its manifest shall
  declare API version 2, its capabilities, the `connection.test` and
  `connection.status` admin actions, no webhooks, and the full configuration
  schema.
- **AC-INTEGRATIONS-DISCORD-001.2:** When `workspace_guilds` is parsed, the plugin
  shall accept comma or newline separators, reject duplicate workspaces or
  guilds, require snowflake guild IDs, and never echo the bot token in an error.
- **AC-INTEGRATIONS-DISCORD-001.3:** When the plugin calls Discord, it shall use one
  narrow interface with a call-recording fake for tests.

### REQ-INTEGRATIONS-DISCORD-002: Gateway connection and guild provisioning

The plugin connects to Discord and prepares each bound guild.

- **AC-INTEGRATIONS-DISCORD-002.1:** When the plugin starts, it shall load the
  configuration once, open the gateway with the required intents, stop
  reconnecting on close codes 4004, 4013, and 4014 with a redacted reason, halt
  calls after a REST 401, and refuse to connect after 900 identifies in 24
  hours.
- **AC-INTEGRATIONS-DISCORD-002.2:** When a bound guild becomes ready, the plugin
  shall create or reuse the operator and viewer roles, the task and archive
  categories with their permission overwrites, the status channel, and the guild
  commands, and a second run shall create nothing.
- **AC-INTEGRATIONS-DISCORD-002.3:** When the bot is not a member of a configured
  guild, the plugin shall log the invite URL; events from unbound guilds shall be
  ignored.

### REQ-INTEGRATIONS-DISCORD-003: Task channel lifecycle

Each task has one channel that follows the task's life.

- **AC-INTEGRATIONS-DISCORD-003.1:** When a task is created in a bound workspace, the
  plugin shall create one channel with a lowercase name of at most 100
  characters, set its topic, record it, create nothing for a duplicate event, and
  mark the record capacity-blocked on Discord's capacity error.
- **AC-INTEGRATIONS-DISCORD-003.2:** When a category reaches 50 channels, the plugin
  shall create the next category.
- **AC-INTEGRATIONS-DISCORD-003.3:** When a task title changes, the plugin shall
  rename the channel at most once per ten minutes, with the last title winning.
- **AC-INTEGRATIONS-DISCORD-003.4:** When a task is archived, deleted, or reaches its
  close trigger, the plugin shall archive or delete the channel as configured,
  reopen it on unarchive, honor the delete delay, and mark the record deleted
  when Discord deletes the channel.

### REQ-INTEGRATIONS-DISCORD-004: Reconciliation

The plugin converges channels and prompts with Kandev after downtime.

- **AC-INTEGRATIONS-DISCORD-004.1:** When the gateway is ready and every five
  minutes, the plugin shall create channels for open tasks without one, close
  channels of archived, deleted, and terminal tasks, and recreate channels whose
  record is deleted.
- **AC-INTEGRATIONS-DISCORD-004.2:** When the plugin reconciles prompts, it shall post
  a prompt for each pending interaction that has none, mark resolved ones as
  resolved elsewhere, apply retention deletes, and retry capacity-blocked
  records.
- **AC-INTEGRATIONS-DISCORD-004.3:** When a second run sees no change, the plugin
  shall make no Discord mutation.

### REQ-INTEGRATIONS-DISCORD-005: Outbound mirroring

Agent output appears in the task channel in order, without duplicates.

- **AC-INTEGRATIONS-DISCORD-005.1:** When a turn completes, a session changes state,
  a reconcile pass runs, or a live-update interval elapses while a session runs,
  the plugin shall post new messages in order, advance its cursor after each
  send, and post nothing for a duplicate trigger.
- **AC-INTEGRATIONS-DISCORD-005.2:** When the plugin filters messages, it shall post
  agent messages, post user messages only when mirroring is enabled and the
  source is not this plugin, post tool activity only when enabled, and never post
  thinking.
- **AC-INTEGRATIONS-DISCORD-005.3:** When the plugin renders output, it shall split at
  2000 characters, respect code fences, cap output at five chunks plus a
  truncation line, and always disable mentions.
- **AC-INTEGRATIONS-DISCORD-005.4:** When a session starts, completes, fails, or
  stops, the plugin shall post a status line, keep one pinned task card edited
  with a ten-second debounce, and keep sends within Discord's rate limits.

### REQ-INTEGRATIONS-DISCORD-006: Inbound delivery

Operators can talk to the agent from the task channel.

- **AC-INTEGRATIONS-DISCORD-006.1:** When a message arrives, the plugin shall drop it
  unless it comes from a bound guild, an active mapped channel, a human author,
  and has content; a non-operator shall get one refusal reaction and nothing
  shall reach Kandev.
- **AC-INTEGRATIONS-DISCORD-006.2:** When an operator's message is delivered, the
  plugin shall send it to the task with the author's display name, map queued,
  sent, and failure results to reactions, advance its cursor only after a final
  outcome, and never send the same Discord message twice.
- **AC-INTEGRATIONS-DISCORD-006.3:** When the gateway becomes ready, the plugin shall
  deliver messages that arrived while it was offline and are younger than the
  configured age, and mark older ones as not delivered.

### REQ-INTEGRATIONS-DISCORD-007: Status and stop commands

Anyone in the channel can see status, and operators can stop work.

- **AC-INTEGRATIONS-DISCORD-007.1:** When a command arrives, the plugin shall defer
  the interaction within three seconds, and outside a task channel reply that
  commands work only in task channels.
- **AC-INTEGRATIONS-DISCORD-007.2:** When anyone runs `/status`, the plugin shall
  reply ephemerally with the title, identifier, state, step, session states,
  queue count, pending interaction count, and link.
- **AC-INTEGRATIONS-DISCORD-007.3:** When an operator runs `/stop`, the plugin shall
  stop the task's running executions for the task scope and cancel the current
  turn for the turn scope, and refuse non-operators.
- **AC-INTEGRATIONS-DISCORD-007.4:** When a stop returns no change, a conflict, or a
  missing grant, the plugin shall reply with a friendly message for each.

### REQ-INTEGRATIONS-DISCORD-008: Approvals

Operators can answer permission prompts and approve review steps as their
linked Kandev account.

- **AC-INTEGRATIONS-DISCORD-008.1:** When a permission interaction is pending, the
  plugin shall post one prompt with a button per option, within Discord's limits,
  record it, and post nothing for a duplicate.
- **AC-INTEGRATIONS-DISCORD-008.2:** When an operator answers, the plugin shall
  request a receipt for the operator's linked account with the observed revision
  and exact option, then respond; an unlinked operator shall get a deep link and
  link instructions.
- **AC-INTEGRATIONS-DISCORD-008.3:** When the answer succeeds, the plugin shall edit
  the prompt to show the actor; when the interaction changed, it shall mark it as
  resolved elsewhere.
- **AC-INTEGRATIONS-DISCORD-008.4:** When a session awaits step approval, the plugin
  shall post one approval prompt, clear it when the task moves, and approve
  through a step-approval receipt; an Office stage shall direct the operator to
  Kandev.
- **AC-INTEGRATIONS-DISCORD-008.5:** When the grant or the link is missing, the
  plugin shall reply ephemerally and change nothing in Kandev.

### REQ-INTEGRATIONS-DISCORD-009: Settings card

An admin can set up and test the connection from the plugin settings.

- **AC-INTEGRATIONS-DISCORD-009.1:** When an admin opens the plugin settings, the
  card shall show workspace IDs with a copy action, the parsed bindings using the
  same rules as the backend, and an invite link per binding.
- **AC-INTEGRATIONS-DISCORD-009.2:** When an admin runs Test, the plugin shall report
  token validity, the application and intent match, membership and missing
  permissions, an invite URL when needed, and the gateway state, without token
  material.
- **AC-INTEGRATIONS-DISCORD-009.3:** When the card initializes or is destroyed twice,
  it shall create no duplicate registration or poller, and its layout shall be
  usable at phone width.

### REQ-INTEGRATIONS-DISCORD-010: Artifact verification and release

The packaged plugin is proven against a real Kandev and a real guild.

- **AC-INTEGRATIONS-DISCORD-010.1:** When the plugin is verified, it shall be
  installed into a disposable Kandev with a dedicated test bot and guild, never
  the primary instance.
- **AC-INTEGRATIONS-DISCORD-010.2:** When the plugin is verified, every scenario
  reachable with the mock agent shall run, including lifecycle, recovery,
  inbound delivery, commands, approvals, and the Discord-side failures.
- **AC-INTEGRATIONS-DISCORD-010.3:** When the plugin is released, an upgrade shall
  preserve Host state, an uninstall shall remove it, the cross-platform package
  shall build, and the README shall carry the setup guide and security notes.

## Exclusions

- Bots creating guilds, and guild ownership transfer.
- Creating Kandev tasks from Discord.
- Attachments, voice, and direct messages.
- Several guilds per workspace, or several workspaces per guild.
- Linking one Discord account to several Kandev users, or linking without a code
  generated in Kandev.
- Answering multi-question clarification forms from Discord.
- Interrupting a running turn with a message.
- Token-level streaming edits.
- A public webhook or HTTP interactions endpoint.
- A forum-thread mode.
- Office approval stages.
- An arbitrary move command.
