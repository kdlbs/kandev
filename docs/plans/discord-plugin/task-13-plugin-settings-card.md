---
id: "13-plugin-settings-card"
title: "Quick-setup settings card with connection test"
status: in_progress
wave: 6
depends_on: ["06-plugin-gateway-provisioning"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-DISCORD-009
acceptance_criteria:
  - AC-INTEGRATIONS-DISCORD-009.1
  - AC-INTEGRATIONS-DISCORD-009.2
  - AC-INTEGRATIONS-DISCORD-009.3
system_design:
  - ../../specs/integrations/system-design/discord.md
---

# Task 13: Quick-setup settings card with connection test

Repository: `Fclem/kandev-plugin-discord`. Use `/mobile-parity` and `/tdd`.

- **Acceptance:**
  1. `ui/bundle.js` registers one `plugin-settings` component. It sits above
     Kandev's admin-only schema form, which edits the token, application ID,
     and `workspace_guilds`. The card shows:
     - workspace IDs from `host.store`, with "Copy line";
     - bindings parsed from `GET /api/plugins/{id}/config` (masked) using the
       same one-to-one snowflake rules as the backend;
     - an invite link per binding.
  2. **Test** calls `host.api.invokeAction("connection.test", { workspaceId })`
     per binding. The backend `server/health` returns:
     - token validity (`GET /users/@me`);
     - the application match and Message Content intent (`GET /applications/@me`
       flags);
     - membership and missing permissions (`GET /users/@me/guilds`);
     - an invite URL when needed;
     - once task-06 lands, gateway state (`4014`/`4004`), cached in
       `instance/health` and served without Discord calls by
       `connection.status`.

     Results contain no token material. Both actions are
     `access: admin, scope: workspace`.
  3. **Lifecycle and layout.** `initialize`/`destroy` can run twice without
     duplicate registrations or pollers. The layout is touch-usable on phone
     width.
- **Verification:** `make test && make verify-package-host` (see the repo `AGENTS.md` for the tooling checkout `verify-package-host` needs).
- **Files likely touched:** `ui/bundle.js`, `ui/*.css`, `tests/*.test.js`, `server/health/{health.go,health_test.go}`, `server/plugin.go` (action handler), `manifest.yaml` (`actions`).
- **Dependencies:** task-06.
- **Parallelism:** parallel-safe with tasks 07–12, except for `server/plugin.go` wiring.
- **Inputs:**
  - Spec "Quick setup and connection test" and the setup scenario.
  - `docs/public/plugins-authoring.md` "Authenticated declared actions" and
    "Plugin settings page".
- **Risks:**
  - Saving restarts the plugin, and actions fail while it restarts. Retry
    with backoff for up to 30 s.
- **Output contract:** summary, files changed, tests run, blockers, risks, and task/plan status update.

## Results
Partial, from the scaffold commit (2026-11-21):

- **Done:** the `plugin-settings` card lists workspace IDs (Copy line), the
  per-binding invite links, and "Test connection" through the
  `connection.test` admin action. Settings are edited in Kandev's own schema
  form below the card, not in a custom form.
- **Done:** the backend `server/health.Test` checks token, application match,
  Message Content intent (application flags), membership, and permissions
  (`GET /users/@me/guilds`).
- **Remaining:** gateway state in the report (needs task-06), cached
  `connection.status`, and a live-host visual check. The card was not
  rendered in a running Kandev ≥0.99. Only node contract tests ran.
