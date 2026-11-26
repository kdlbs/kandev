---
id: "11-plugin-status-stop-commands"
title: "/status and /stop commands"
status: pending
wave: 5
depends_on: ["06-plugin-gateway-provisioning", "02-host-control-rpcs"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-DISCORD-007
acceptance_criteria:
  - AC-INTEGRATIONS-DISCORD-007.1
  - AC-INTEGRATIONS-DISCORD-007.2
  - AC-INTEGRATIONS-DISCORD-007.3
  - AC-INTEGRATIONS-DISCORD-007.4
system_design:
  - ../../specs/integrations/system-design/discord.md
---

# Task 11: `/status` and `/stop`

Repository: `Fclem/kandev-plugin-discord`. Use `/tdd`.

- **Acceptance:**
  1. **Deferral and placement.** Every interaction is deferred
     (ephemerally) within 3 s. Outside a mapped task channel the plugin
     replies that commands work only in task channels.
  2. **`/status`.** Anyone who can see the channel may run it. It replies
     ephemerally with:
     - the title and identifier;
     - the task state and workflow step name (`Workflows().ListSteps`);
     - each session's state;
     - the queue count cached from `message.queue.status_changed`;
     - the pending interaction count (`Interactions().ListPending`);
     - the link.
  3. **`/stop`.** Operators only; others get an ephemeral refusal.
     - `scope:task` uses `HostExecutionCommands(host).StopTaskRun` for each
       running session's execution. That is already on Kandev `main` and
       needs `api_write: execution` plus the `host.v2.write:execution` grant.
     - The default `scope:turn` uses the new `CancelTurn` (host task-02).
     - Typed outcomes map to friendly replies: `NO_CHANGE` → "nothing
       running"; `CONFLICT` → "state changed, retry"; a missing grant → "an
       admin must grant execution control".
- **Verification:** `make test-backend`
- **Files likely touched:** `server/commands/{registry.go,status.go,stop.go,*_test.go}`, `server/kandev/exact.go` (capability context: approval revision, manifest digest, resource versions), `manifest.yaml`.
- **Dependencies:** task-06; task-02 for `scope:turn`. `scope:task` can ship before task-02.
- **Parallelism:** parallel-safe with task-10, apart from `authz` (created by task-10; import it read-only).
- **Inputs:** Spec "Slash commands"; `docs/public/plugins-authoring.md` "Exact execution controls"; capability discovery through `pluginsdk.HostExactQueries`/`GetCapabilityContext`.
- **Output contract:** summary, files changed, tests run, blockers, risks, and task/plan status update.

## Results
Pending.
