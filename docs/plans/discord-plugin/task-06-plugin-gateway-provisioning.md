---
id: "06-plugin-gateway-provisioning"
title: "Gateway lifecycle and guild provisioning"
status: pending
wave: 2
depends_on: ["05-plugin-bootstrap-config"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-DISCORD-002
acceptance_criteria:
  - AC-INTEGRATIONS-DISCORD-002.1
  - AC-INTEGRATIONS-DISCORD-002.2
  - AC-INTEGRATIONS-DISCORD-002.3
system_design:
  - ../../specs/integrations/system-design/discord.md
---

# Task 06: Gateway lifecycle and guild provisioning

Repository: `Fclem/kandev-plugin-discord`. Use `/tdd`.

- **Acceptance:**
  1. Startup and connection:
     - `SetHost` triggers a single start: load config, then open the gateway
       with intents `GUILDS | GUILD_MESSAGES | MESSAGE_CONTENT`.
     - Close codes `4004`, `4013`, and `4014` stop reconnecting and log a
       fatal, redacted reason.
     - A REST 401 sets a halt flag that every `discordapi` call checks.
     - A persisted identify counter (instance-scoped state) refuses to
       connect after 900 identifies in a rolling 24 h.
  2. On `READY`/`GUILD_CREATE`, each bound guild is provisioned idempotently.
     The plugin creates or reuses:
     - the operator role, and the viewer role if configured;
     - `Kandev Tasks` and `Kandev Archive` categories with the overwrites
       from the spec "Permissions" section;
     - a `#kandev-status` channel;
     - guild commands, registered only when `commands_hash` changes.

     The result is stored in `workspace/<id>/guild`. A rerun performs zero
     create calls on the fake.
  3. A configured guild the bot is not a member of is logged with the invite
     URL. Events from unbound guilds are ignored.
- **Verification:** `make test-backend && make vet`
- **Files likely touched:** `server/gateway/{gateway.go,gateway_test.go}`, `server/provision/{provision.go,provision_test.go}`, `server/store/{store.go,store_test.go}`, `server/plugin.go`.
- **Dependencies:** task-05.
- **Parallelism:** sequential.
- **Inputs:**
  - Spec "Discord surface", "Permissions", and "Failure modes".
  - Plan "Discord API constraints".
- **Risks:**
  - The host may inject `Host` late: `Serve` retries the broker dial for up
    to 30 s.
  - No shutdown hook exists. The process is killed, so the gateway is not
    closed cleanly; this is acceptable because every start re-identifies.
- **Output contract:** summary, files changed, tests run, blockers, risks, and task/plan status update.

## Results
Pending.
