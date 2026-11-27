---
id: "10-plugin-inbound-delivery"
title: "Discord → Kandev delivery with authorization"
status: pending
wave: 5
depends_on: ["08-plugin-reconciler", "09-plugin-outbound-mirroring"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-DISCORD-006
acceptance_criteria:
  - AC-INTEGRATIONS-DISCORD-006.1
  - AC-INTEGRATIONS-DISCORD-006.2
  - AC-INTEGRATIONS-DISCORD-006.3
system_design:
  - ../../specs/integrations/system-design/discord.md
---

# Task 10: Discord → Kandev delivery

Repository: `Fclem/kandev-plugin-discord`. Use `/tdd`.

- **Acceptance:**
  1. `MESSAGE_CREATE` handling:
     - Messages are dropped unless they come from a bound guild, in an
       `active` mapped channel, from a non-bot, non-webhook author, with
       non-empty content.
     - `authz.IsOperator(member.roles, user.id)` gates delivery. A
       non-operator gets one ⛔ reaction, and nothing is sent to Kandev.
  2. Delivery:
     - The plugin calls `Messages().Send(taskID, "", "<display name> (Discord): <content>")`.
     - Results map to reactions: `queued` → 🕒, `sent`/`started` → ✅,
       queue-full or terminal-session errors → ⚠️ plus a short reply.
     - `inbound_cursor` advances only after a final outcome.
     - A Discord message ID that was already processed is never sent twice
       (LRU plus cursor).
  3. Catch-up on `READY`: the plugin fetches messages after `inbound_cursor`
     for each active channel. Messages younger than
     `inbound_catchup_max_age_minutes` go through the same path. Older ones
     get ⚠️ and a "not delivered — bridge was offline" reply.
- **Verification:** `make test-backend`
- **Files likely touched:** `server/inbound/{handler.go,catchup.go,*_test.go}`, `server/authz/{authz.go,authz_test.go}`.
- **Dependencies:** task-08 (`READY` hook), task-09 (echo-suppression contract).
- **Parallelism:** sequential.
- **Inputs:**
  - Spec "Discord → Kandev", "Permissions", and the inbound scenarios.
  - The Send semantics in `apps/backend/internal/backendapp/adapters_plugin_messenger.go:58-173`.
  - The queue cap (default 10; `KANDEV_QUEUE_MAX_PER_SESSION`) in
    `internal/orchestrator/messagequeue/service.go`.
- **Risks:**
  - Classify queue-full and terminal-session errors from the gRPC status and
    message. If the codes are ambiguous, add stable codes to the Send adapter
    in the kandev repository rather than string-matching.
- **Output contract:** summary, files changed, tests run, blockers, risks, and task/plan status update.

## Results
Pending.
