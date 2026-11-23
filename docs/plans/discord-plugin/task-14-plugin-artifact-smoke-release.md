---
id: "14-plugin-artifact-smoke-release"
title: "Artifact smoke on disposable Kandev + test guild; release"
status: pending
wave: 7
depends_on: ["10-plugin-inbound-delivery", "12-plugin-permission-approvals", "13-plugin-settings-card", "04-host-plugin-docs"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-DISCORD-010
acceptance_criteria:
  - AC-INTEGRATIONS-DISCORD-010.1
  - AC-INTEGRATIONS-DISCORD-010.2
  - AC-INTEGRATIONS-DISCORD-010.3
system_design:
  - ../../specs/integrations/system-design/discord.md
---

# Task 14: Artifact smoke and release

Repositories: `Fclem/kandev-plugin-discord`, plus `kdlbs/kandev` for the
catalog entry, only when the user requests publishing. Follow the
`/create-kandev-plugin` "Verify The Artifact" section.

- **Acceptance:**
  1. Install `make package-host` output into a disposable dev Kandev
     (`KANDEV_MOCK_AGENT=true`, a fresh `~/.cache`-backed home). Never use
     the primary instance or credentials. Use a dedicated test bot and test
     guild.
  2. Exercise every spec scenario reachable with the mock agent, including
     the failure cases:
     - Lifecycle and recovery: create, rename, archive, reopen; downtime
       reconcile; plugin restart (config edit).
     - Inbound delivery: queued, sent, and queue-full results; the
       non-operator ⛔; catch-up of stale messages.
     - Commands: `/status`, both `/stop` scopes, approval through a button
       and through `/approve`, and an expired approval.
     - Discord-side failures: mention suppression, an invalid token
       (`4004`), and disallowed intents (`4014`).
  3. Upgrade by bumping the version and reinstalling; Host state is
     preserved. Disable and re-enable, then uninstall; state is removed and
     the Discord objects remain. Run the full cross-platform `make package`.
     Write the README setup guide: create a server, enable `MESSAGE_CONTENT`,
     invite the bot, set `workspace_guilds`, assign the operator role, and
     read the security notes.
- **Verification:** `make test && make verify-package` (with the tooling checkout described in the repo `AGENTS.md`). Record a per-scenario
  smoke checklist in Results.
- **Files likely touched:** `README.md`, `docs/` in the plugin repository, `.github/workflows/release.yml` (template), and `plugin-registry/plugins.yaml` in `kdlbs/kandev` (publishing only).
- **Dependencies:** tasks 04, 10, 12, 13.
- **Parallelism:** sequential.
- **Risks:**
  - Live Discord tests consume identifies and may hit rate limits. Keep the
    run short and use one bot.
  - Tagging a release, creating the repository, and submitting the catalog
    entry are external side effects that require user approval.
- **Output contract:** summary, files changed, tests run, artifact name, host platform tested, paths not exercised, blockers, risks, and task/plan status update.

## Results
Pending.
