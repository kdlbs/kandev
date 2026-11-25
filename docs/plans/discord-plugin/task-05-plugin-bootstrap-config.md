---
id: "05-plugin-bootstrap-config"
title: "Plugin config parsing and Discord API seam (scaffold exists)"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-DISCORD-001
acceptance_criteria:
  - AC-INTEGRATIONS-DISCORD-001.1
  - AC-INTEGRATIONS-DISCORD-001.2
  - AC-INTEGRATIONS-DISCORD-001.3
system_design:
  - ../../specs/integrations/system-design/discord.md
---

# Task 05: Config parsing and Discord API seam

Repository: [`Fclem/kandev-plugin-discord`](https://github.com/Fclem/kandev-plugin-discord).
The scaffold was created on 2026-11-21 from `kdlbs/kandev-plugin-template`. It
contains the identity, manifest, config schema, and the README setup guide, and
it uses the standalone `github.com/kdlbs/kandev/pluginsdk` v0.1.0. Use
`/create-kandev-plugin` and `/tdd`.

- **Acceptance:**
  1. The manifest matches the spec "Plugin manifest" section:
     - `api_version: 2`;
     - the capabilities;
     - the `connection.test`/`connection.status` admin actions;
     - no webhooks;
     - the full `config_schema`.

     Capabilities that need unreleased Host additions (`api_write: execution`,
     `interactions`) stay commented out until the SDK and host release that
     ships them. `min_kandev_version` follows the template floor until then.
  2. `server/config` parses `workspace_guilds`:
     - comma or newline separators, trimmed;
     - one-to-one: duplicate workspaces or guilds are rejected;
     - guild IDs must be snowflakes;
     - errors never echo `bot_token`.
  3. `server/discordapi` defines the narrow interface (channels, messages,
     reactions, pins, roles, overwrites, commands, interaction responses) with
     a discordgo-backed implementation and a call-recording fake.
- **Verification:** run the template targets named in the repo's `AGENTS.md`: `make sdk-check check-format vet test build`.
- **Files likely touched:** `manifest.yaml`, `go.mod`, `go.sum`, `server/main.go`, `server/plugin.go`, `server/config/{config.go,config_test.go}`, `server/discordapi/{api.go,discordgo.go,fake.go}`.
- **Dependencies:** None.
- **Parallelism:** parallel-safe with the host tasks (different repository).
- **Inputs:** Spec "API surface"; plan "Architecture (plugin)"; the repo's `AGENTS.md`, `README.md`, and `Makefile`.
- **Risks:** The `bwmarrin/discordgo` dependency needs review (BSD-3-Clause license, maintenance activity).
- **Output contract:** summary, files changed, tests run, blockers, risks, and task/plan status update.

## Results
Partial, from the scaffold commit (2026-11-21):

- **Done:** the manifest identity; API v2; the `connection.test` admin action;
  the full `config_schema`; no webhooks.
- **Done:** `server/config.Parse`/`ParseBindings` for token, application ID,
  and one-to-one bindings, with token-free errors.
- **Remaining:** acceptance 3 (`server/discordapi` with discordgo and a
  fake). The scaffold has only a minimal REST client (`server/discord`). The
  other config fields are parsed when their features land.
- **Verification:** `make sdk-check check-format vet test build` passed:
  Go tests in 4 packages, 5 UI tests, and the package and release verifier
  negative tests.
- **Packaging:** `make verify-package-host` against a tooling export of
  `f0e2e774` wrote `kandev-plugin-discord-0.1.0.tar.gz`; checksums OK.
