# 0034: Agent Client Protocol Codex ACP Package

**Status:** accepted
**Date:** 2026-07-10
**Area:** backend

## Context
Kandev launched Codex ACP through `@zed-industries/codex-acp`, the original Zed-maintained adapter package. By July 10, 2026, the Zed repository directs new installs to `agentclientprotocol/codex-acp`, and the npm package is deprecated in favor of `@agentclientprotocol/codex-acp`. The replacement package exposes the same `codex-acp` binary name, is maintained under the Agent Client Protocol organization, and includes a compatible `@openai/codex` dependency.

## Decision
Kandev launches Codex ACP with `npx -y @agentclientprotocol/codex-acp` from `apps/backend/internal/agent/agents/codex_acp.go`. Remote install scripts install both `@openai/codex` for the interactive `codex login --device-auth` flow and `@agentclientprotocol/codex-acp` for ACP sessions.

The old Zed adapter-specific `-c` profile CLI flags are no longer seeded for Codex ACP profiles. The new adapter configures approval and sandbox behavior through ACP session modes and config options, not those legacy argv flags. Existing user-saved custom `cli_flags` rows remain user-owned and are not rewritten by this dependency migration.

## Consequences
New Codex ACP launches and documentation follow the maintained Agent Client Protocol package. New and backfilled Codex ACP profiles do not show stale `-c approval_policy=never` or `-c sandbox_permissions=...` entries.

Kandev still keeps `@openai/codex` in the install script because the login UI invokes the user-facing `codex` binary directly. If Codex ACP later exposes a browserless login command with equivalent behavior, Kandev can revisit that separate install.

## Alternatives Considered
Continue using `@zed-industries/codex-acp`: rejected because its own repository and npm metadata point users to the Agent Client Protocol package for ongoing updates.

Rewrite old profile CLI flags into `CODEX_CONFIG` or a default session mode: rejected for this migration because `cli_flags` are persisted as user-owned profile data, and translating enabled legacy flags into mode/config values could silently change profile behavior. Profile mode/config management should be handled by the ACP config-option path.
