# Office Wave 7: Assistant, Channels, Memory, Config Sync

**Date:** 2026-04-26
**Status:** proposed
**Specs:** `office-assistant`, `office-config`
**UI Reference:** `docs/plans/2026-04-office-ui-reference.md` (settings page for config sync)
**Depends on:** Wave 5 (inbox, approvals), Wave 6 (routines for proactive work)

## Problem

The core office system is functional after Waves 1-6. Wave 7 adds the extended features: personal assistant with external chat channels, persistent agent memory, agent self-improvement, and configuration portability.

## Scope

### 7A: Channels (backend + frontend, parallelizable)

**Backend:**

**Repository** (`internal/office/repository/sqlite/channels.go`):
- Full CRUD for `office_channels`
- `GetByPlatform(ctx, agentInstanceID, platform)` -- find channel for agent+platform
- `GetByTaskID(ctx, taskID)` -- find channel for a channel task (for reply relay)

**Channel task creation** (`internal/office/service/channels.go`):
- `SetupChannel(ctx, channel)`:
  1. Create the channel row
  2. Create a long-lived task (title: "[Platform] Channel - [Agent Name]", status: in_progress, never completes)
  3. Link task_id on the channel row
  4. Validate platform credentials (test connection)

**Webhook ingress** (`internal/office/handlers/channel_webhook.go`):
- `POST /api/v1/office/channels/:channelId/inbound`
- Platform-specific payload parsing:
  - Telegram: extract message text, author display name from Update object
  - Slack: extract text from event payload, verify signing secret
  - Discord: extract content, verify Ed25519 signature
  - Generic webhook: extract body as message text
- Create comment on channel task with `source=<platform>`, `reply_channel_id=<channelId>`
- Comment triggers `task_comment` wakeup for the agent (existing Wave 4 subscriber)

**Outbound relay** (`internal/office/service/channel_relay.go`):
- Subscribe to comment creation events
- Filter: comment on a channel task, posted by an agent, has `reply_channel_id`
- Format comment for target platform (markdown -> platform-native)
- Send via platform API:
  - Telegram: `POST https://api.telegram.org/bot<token>/sendMessage`
  - Slack: `POST https://slack.com/api/chat.postMessage`
  - Discord: `POST https://discord.com/api/channels/<id>/messages`
- Retry with exponential backoff (3 attempts)
- Log delivery to activity log

**Frontend:**
- Channels tab on agent detail page (`/office/agents/[id]`):
  - List of configured channels with platform icon, status
  - Setup wizard: select platform, enter credentials, test connection, activate
  - Edit/delete controls
- Sidebar agent entries show platform icons if channels configured

### 7B: Agent Memory (backend + frontend, parallelizable)

**Backend:**

**Repository** (`internal/office/repository/sqlite/memory.go`):
- Full CRUD for `office_agent_memory`
- `Get(ctx, agentInstanceID, layer, key)` -- single entry
- `List(ctx, agentInstanceID, layer)` -- all entries for a layer
- `Search(ctx, agentInstanceID, query)` -- search key and content
- `GetSummary(ctx, agentInstanceID)` -- operating layer entries + recent knowledge entries
- `DeleteAll(ctx, agentInstanceID)` -- clear all memory
- `Export(ctx, agentInstanceID)` -- all entries as structured JSON

**API endpoints** (already stubbed in Wave 1):
- `GET /api/v1/office/agents/:id/memory?layer=&key=`
- `PUT /api/v1/office/agents/:id/memory` -- create or update (upsert by agent+layer+key)
- `DELETE /api/v1/office/agents/:id/memory/:entryId`
- `GET /api/v1/office/agents/:id/memory/summary` -- for session bootstrap

**Memory skill** (system-provided skill):
- Create `skills/memory/SKILL.md` content that teaches agents:
  - When to read: at session start, call `GET .../memory/summary`
  - When to write: at session end, extract facts and call `PUT .../memory`
  - Format: layer (knowledge/session/operating), key (topic path), content (markdown)
  - How to handle conflicting facts (supersede old, keep both if different)
  - Authentication: use `$KANDEV_API_KEY` (per-run JWT from environment)
- Register as system skill in skill registry on startup

**Frontend:**
- Memory tab on agent detail page:
  - Grouped by layer (operating, knowledge, session)
  - Each entry: key, content preview, updated timestamp
  - Expand to view full content
  - Delete individual entries
  - "Clear All" button (with confirmation dialog)
  - "Export" button (downloads JSON)
  - Search input (filters by key/content)

### 7C: Agent Self-Improvement (backend, depends on 7B)

**Backend** (`internal/office/service/self_improvement.go`):
- Extend skill creation: if `created_by_agent_instance_id` is set:
  - Check agent has `can_manage_own_skills` permission
  - If workspace `require_approval_for_skill_changes=true`: create `skill_creation` approval instead of directly creating skill
  - On approval: create skill, add to agent's `desired_skills`
  - On rejection: notify agent via `approval_resolved` wakeup
- Agents can only edit skills where `created_by_agent_instance_id` matches their ID

**No frontend changes:** approval flow uses existing inbox UI.

### 7D: Config Export/Import (backend + frontend, parallelizable)

**Backend:**

**Export** (`internal/office/service/config_export.go`):
- `ExportBundle(ctx, workspaceID) -> []ConfigFile`:
  - Serialize `kandev.yml` (workspace settings)
  - Serialize each agent instance to `agents/<name>.yml`
  - Serialize each skill: inline -> `skills/<slug>/SKILL.md`, git/path -> metadata only
  - Serialize each routine to `routines/<name>.yml`
  - Serialize each project to `projects/<name>.yml`
  - Cross-reference by name (not ID): `reports_to: ceo`, `desired_skills: [code-review]`
  - Agent profiles matched by signature: `agent_name + model + mode`
- `ExportZip(ctx, workspaceID) -> io.Reader` -- zip archive of the bundle

**Import** (`internal/office/service/config_import.go`):
- `PreviewImport(ctx, workspaceID, files) -> ImportPreview`:
  - Parse all files
  - Diff against current state: created, updated, deleted entities
  - Return preview without applying
- `ApplyImport(ctx, workspaceID, files, options) -> ImportResult`:
  - Options: `deleteUnmatched bool` (whether to remove entities not in the bundle)
  - Deduplicate by name: update if exists, create if new
  - Match agent profiles by signature
  - Resolve skill slugs and `reports_to` references
  - Log activity for each change

**Config-sync skills** (system-provided):
- `kandev-config-export` skill: SKILL.md with API endpoints for reading config + YAML format spec
- `kandev-config-import` skill: SKILL.md with file format spec + API endpoints for applying changes

**Frontend** (`/office/company/settings`):
- Export button: downloads `.kandev.zip`
- Import: upload zip, show preview diff (created/updated/deleted), confirm to apply
- Config source section: repo URL + branch + path input, test connection, sync status

## Tests

- Channel setup: create channel, create channel task
- Channel inbound: webhook -> comment created -> wakeup queued
- Channel outbound: agent comment -> relay to platform API
- Memory CRUD: create, read, update, delete, search, export
- Memory summary: returns operating + recent knowledge
- Self-improvement: agent creates skill -> approval if configured -> skill added
- Config export: correct YAML format, cross-references by name
- Config import: preview shows diff, apply creates/updates entities
- Config import: deduplication by name
- Config import: agent profile matching by signature

## Verification

1. `make -C apps/backend test` passes
2. Set up Telegram channel for assistant -> send message -> agent responds -> reply arrives in Telegram
3. Agent with memory skill: stores preferences, recalls them in next session
4. Agent creates a skill for itself -> approval in inbox -> approved -> skill active
5. Export config -> import into fresh workspace -> all entities recreated correctly
6. Import preview shows accurate diff
