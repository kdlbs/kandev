---
id: "09-plugin-outbound-mirroring"
title: "Kandev → Discord mirroring"
status: pending
wave: 4
depends_on: ["07-plugin-channel-lifecycle", "17-host-message-source"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-DISCORD-005
acceptance_criteria:
  - AC-INTEGRATIONS-DISCORD-005.1
  - AC-INTEGRATIONS-DISCORD-005.2
  - AC-INTEGRATIONS-DISCORD-005.3
  - AC-INTEGRATIONS-DISCORD-005.4
system_design:
  - ../../specs/integrations/system-design/discord.md
---

# Task 09: Kandev → Discord mirroring

Repository: `Fclem/kandev-plugin-discord`. Use `/tdd`.

- **Acceptance:**
  1. Triggers:
     - `turn.completed`, `task_session.state_changed`, the reconcile pass,
       and (while a session is `RUNNING`) a ticker every
       `live_update_interval_seconds` pull `Messages().List` filtered by the
       task's session IDs and `Since=outbound_cursor.created_at`.
     - Already-posted IDs at the same timestamp are skipped.
     - Messages are posted in order and the cursor advances after each
       successful send.
     - A duplicate trigger posts nothing.
  2. Filtering:
     - Posted: agent messages of type `message` or `content`.
     - Posted only if `mirror_user_messages` is on: user messages whose
       `Source` (host task-17) is not `plugin:kandev-plugin-discord`.
     - Posted only if `post_tool_activity` is on: tool types.
     - Never posted: `thinking`.
  3. Rendering and status:
     - The chunker splits at 2000 characters, respects code fences, caps
       output at 5 chunks plus a truncation line, and always sends
       `allowed_mentions.parse=[]`.
     - Status lines are posted for session start, completion, failure, and
       stop.
     - The pinned task card is created once and edited with a 10 s debounce.
     - Senders are serialized per channel, and a global limiter caps the rate
       at 40 req/s or less.
- **Verification:** `make test-backend`
- **Files likely touched:** `server/outbound/{puller.go,render.go,chunk.go,sender.go,card.go,*_test.go}`.
- **Dependencies:** task-07, and task-17 for `Message.Source`. Until that
  host release, mirror only agent messages and keep `mirror_user_messages`
  off.
- **Parallelism:** sequential.
- **Inputs:**
  - Spec "Kandev → Discord".
  - Plan "Investigation findings → Events": do not subscribe to `message.*`.
  - Message DTO: `pluginsdk/data_types.go` in the standalone SDK (`Message`, `MessageFilter`).
- **Risks:**
  - Messages that share one RFC3339 second need ID tie-breaks.
- **Output contract:** summary, files changed, tests run, blockers, risks, and task/plan status update.

## Results
Pending.
