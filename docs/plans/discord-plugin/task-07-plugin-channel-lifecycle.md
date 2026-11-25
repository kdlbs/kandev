---
id: "07-plugin-channel-lifecycle"
title: "Task channel create, rename, close, reopen, retention"
status: pending
wave: 3
depends_on: ["06-plugin-gateway-provisioning"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-DISCORD-003
acceptance_criteria:
  - AC-INTEGRATIONS-DISCORD-003.1
  - AC-INTEGRATIONS-DISCORD-003.2
  - AC-INTEGRATIONS-DISCORD-003.3
  - AC-INTEGRATIONS-DISCORD-003.4
system_design:
  - ../../specs/integrations/system-design/discord.md
---

# Task 07: Task channel lifecycle

Repository: `Fclem/kandev-plugin-discord`. Use `/tdd`.

- **Acceptance:**
  1. Channel creation:
     - `task.created` for a bound workspace (non-ephemeral unless configured)
       creates one channel named `<identifier>-<slug(title)>`. The name is
       lowercase, limited to `[a-z0-9-]`, and at most 100 characters.
     - The topic is set from the title, identifier, and link.
     - The channel record and the `channel:<id>` index are written.
     - Duplicate events create nothing.
     - Error `30013` or full categories leave the record in
       `capacity_blocked`, with a status-channel note.
     - When a category reaches 50 channels, the plugin creates the next
       category (`Kandev Tasks 2`, …).
  2. Title changes in `task.updated` rename the channel at most once per
     10 min. The last title wins.
  3. Closing and reopening:
     - Archive, delete, or `close_trigger` closes the channel by archiving it
       (move plus send-deny) or deleting it.
     - Unarchiving reopens it.
     - `delete_after` is honored.
     - `CHANNEL_DELETE` marks the record `deleted`.
- **Verification:** `make test-backend`
- **Files likely touched:** `server/channels/{naming.go,lifecycle.go,*_test.go}`, `server/dispatch/{dispatcher.go,dispatcher_test.go}`, `server/store/store.go`.
- **Dependencies:** task-06. Archive detection on `task.updated` reads
  `archived_at` from the event payload. The reconciler (task-08) uses
  `Task.ArchivedAt`, which is already on Kandev `main`.
- **Parallelism:** sequential.
- **Inputs:**
  - Spec "State machine (task channel)" and the scenarios on creation,
    archiving, and capacity.
  - Event payload fields: `task.updated` carries `archived_at`, and
    `task.state_changed` carries `old_state`/`new_state`
    (`apps/backend/internal/task/service/service_events.go` on `origin/main`).
- **Risks:**
  - The rename limit is undocumented. The plugin relies on 429
    `retry_after` handling in addition to the debounce.
- **Output contract:** summary, files changed, tests run, blockers, risks, and task/plan status update.

## Results
Pending.
