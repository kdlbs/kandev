---
id: "08-plugin-reconciler"
title: "Reconciler: channels, cursors, pending interactions, retention"
status: pending
wave: 4
depends_on: ["07-plugin-channel-lifecycle"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-DISCORD-004
acceptance_criteria:
  - AC-INTEGRATIONS-DISCORD-004.1
  - AC-INTEGRATIONS-DISCORD-004.2
  - AC-INTEGRATIONS-DISCORD-004.3
system_design:
  - ../../specs/integrations/system-design/discord.md
---

# Task 08: Reconciler

Repository: `Fclem/kandev-plugin-discord`. Use `/tdd`.

- **Acceptance:**
  1. On `READY` and every 5 minutes, for each bound workspace, the reconciler
     lists tasks with `TaskFilter.IncludeArchived=true` (already on Kandev
     `main`) and diffs them against the channel records:
     - open tasks without a channel → create;
     - archived tasks (`ArchivedAt`), deleted tasks (`Get` → `NotFound`), and
       terminal tasks → close;
     - open tasks whose record is `deleted` → recreate.
  2. It reconciles prompt records against `Interactions().ListPending` and
     `Get`: prompts are posted for pending interactions that have none, and
     resolved ones are marked "resolved elsewhere". It also applies retention
     deletes and retries `capacity_blocked` records.
  3. A second run against the same fakes performs no Discord mutations.
- **Verification:** `make test-backend`
- **Files likely touched:** `server/reconcile/{reconcile.go,reconcile_test.go}`, `server/plugin.go`.
- **Dependencies:** task-07.
- **Parallelism:** sequential with task-09 (shared `server/plugin.go`).
- **Inputs:** Spec "Failure modes" and "Persistence guarantees"; plan "Investigation findings" (Task reads, Interactions).
- **Risks:** Use `PageInfo.NextCursor` pagination. Reconcile must share the per-task workers.
- **Output contract:** summary, files changed, tests run, blockers, risks, and task/plan status update.

## Results
Pending.
