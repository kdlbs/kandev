---
id: "01-copilot-context-tier-discovery-and-flags"
title: "Copilot context-tier discovery, CLI flags, and command preview"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-COPILOT-CONTEXT-TIER-001
  - REQ-AGENTS-COPILOT-CONTEXT-TIER-002
acceptance_criteria:
  - AC-AGENTS-COPILOT-CONTEXT-TIER-001.1
  - AC-AGENTS-COPILOT-CONTEXT-TIER-001.2
  - AC-AGENTS-COPILOT-CONTEXT-TIER-001.3
  - AC-AGENTS-COPILOT-CONTEXT-TIER-001.4
  - AC-AGENTS-COPILOT-CONTEXT-TIER-002.1
  - AC-AGENTS-COPILOT-CONTEXT-TIER-002.2
  - AC-AGENTS-COPILOT-CONTEXT-TIER-002.3
system_design:
  - ../../specs/agents/system-design/copilot-context-tier.md
---

# Task 01: Copilot context-tier discovery, CLI flags, and command preview

## Summary

Synthesize and preserve Copilot's normalized `context_tier` (`Context Window`:
`Default` / `Long Context`) across `acpcompat`, `agentctl` ACP session/update
snapshots, and host utility model-config probes when Copilot omits a native wire
option. Wire `ConfigOptions["context_tier"]` into `CopilotACP` (and `MockAgent`
Copilot alias) CLI/passthrough `--context` flag builders and plumb
`config_options` through the Agent Profile `CommandPreviewCard`.

## In scope

- Add `apps/backend/internal/agentctl/acpcompat/copilot.go` (and wire into
  `acpcompat.go`) with `NormalizeCopilotConfigOptions`,
  `ParseCopilotContextTierFromArgs`, and `IsValidCopilotContextTier`, deferring
  to any native `context_tier` / `contextTier` / `context` wire option.
- Update `copilotDialect` (`dialect_copilot.go`), `adapter_session.go`
  (`emitAuthoritativeConfigOptionsWithPolicy`), and `adapter_updates.go`
  (`ConfigOptionUpdate`) so `context_tier` and the subprocess's launch
  `currentValue` survive `session/new`, `session/load`, `SetModel`,
  `SetConfigOption`, and `ConfigOptionUpdate` frames.
- Update `apps/backend/internal/agentctl/server/utility/acp_executor.go` to pass
  `requestedConfigOptions` into `CommandOptions.ConfigOptions` and seed
  `acpcompat` with the requested/CLI `context_tier` without sending an
  unsupported `session/set_config_option("context_tier", ...)` RPC.
- Add `ConfigOptions map[string]string` to `agents.CommandOptions` and
  `agents.PassthroughOptions`, and append `--context <default|long_context>` in
  `CopilotACP.BuildCommand`, `CopilotACP.BuildPassthroughCommand`, and
  `MockAgent` (`copilot-acp` alias) unless `CLIFlagTokens` already specifies
  `--context` or `--context=...`.
- Add `config_options` to backend `CommandPreviewRequest` (`agent_config.go`)
  and frontend `CommandPreviewRequest` / `CommandPreviewCard`
  (`settings-api.ts`, `app/actions/agents.ts`, `command-preview-card.tsx`,
  `agent-profile-page.tsx`, and browser-demo route parity if applicable).

## Out of scope

- Lifecycle `StartAgentProcess` / `RestartAgentProcess` merging of session
  runtime `ConfigOptions` and live-session idle subprocess reload (owned by
  Task 02).
- Playwright E2E specs and `mock-agent` `session/load` reload fixtures (owned by
  Task 02).

## Acceptance

- Capability probes (`GET /api/v1/agent-models/copilot-acp` and
  `POST /api/v1/agent-models/copilot-acp/resolve`) and `agentctl` Copilot ACP
  sessions expose `context_tier` (`Default`, `Long Context`) with accurate
  `currentValue` across initial load and post-model updates, while leaving any
  native wire option untouched.
- `CopilotACP.BuildCommand` and `BuildPassthroughCommand` emit
  `--context default` or `--context long_context` when `ConfigOptions["context_tier"]`
  is set, omit `--context` when unset/invalid, and yield to explicit user
  `--context` CLI flags.
- `CommandPreviewCard` sends `config_options` and updates the displayed command
  when `Context Window` changes in the Copilot profile editor.

## ASCII UI preview

Excerpt from [`plan.md#ascii-ui-preview`](plan.md#ascii-ui-preview) covering
`UI-01` (profile/settings selector) and `UI-02` (`CommandPreviewCard`) for
`AC-AGENTS-COPILOT-CONTEXT-TIER-001.4` and `AC-AGENTS-COPILOT-CONTEXT-TIER-002.3`:

```text
UI-02: Copilot profile command preview card
+------------------------------------------------------------------+
| >_ Command preview                                               |
|                                                                  |
| Flags configured here are passed to the ACP bridge.              |
| +--------------------------------------------------------------+ |
| | copilot --acp --model gpt-6.2-sol --context long_context [C] | |
| +--------------------------------------------------------------+ |
+------------------------------------------------------------------+
```

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/agentctl/acpcompat/... ./internal/agentctl/server/adapter/transport/acp/... ./internal/agent/agents/... ./internal/agent/utility/... ./internal/agent/settings/controller/...)
bash scripts/lint-go-changed \
  apps/backend/internal/agentctl/acpcompat/copilot.go \
  apps/backend/internal/agentctl/acpcompat/acpcompat.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_copilot.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go \
  apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_updates.go \
  apps/backend/internal/agent/agents/agent.go \
  apps/backend/internal/agent/agents/copilot_acp.go \
  apps/backend/internal/agent/agents/mock.go \
  apps/backend/internal/agent/utility/acp_executor.go \
  apps/backend/internal/agent/settings/controller/agent_config.go
(cd apps/web && pnpm exec vitest run "app/settings/agents/[agentId]/profiles/[profileId]/command-preview-card.test.tsx" components/settings/agent-profile-save-catalogue.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check)
```

## Files likely touched

- `apps/backend/internal/agentctl/acpcompat/copilot.go`
- `apps/backend/internal/agentctl/acpcompat/copilot_test.go`
- `apps/backend/internal/agentctl/acpcompat/acpcompat.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_copilot.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_copilot_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_updates.go`
- `apps/backend/internal/agent/agents/agent.go`
- `apps/backend/internal/agent/agents/copilot_acp.go`
- `apps/backend/internal/agent/agents/copilot_acp_test.go`
- `apps/backend/internal/agent/agents/mock.go`
- `apps/backend/internal/agent/utility/acp_executor.go`
- `apps/backend/internal/agent/utility/acp_executor_test.go`
- `apps/backend/internal/agent/settings/controller/agent_config.go`
- `apps/backend/internal/agent/settings/controller/agent_config_test.go`
- `apps/web/lib/api/domains/settings-api.ts`
- `apps/web/app/actions/agents.ts`
- `apps/web/components/settings/agent-profile-page.tsx`
- `apps/web/app/settings/agents/[agentId]/profiles/[profileId]/command-preview-card.tsx`
- `apps/web/app/settings/agents/[agentId]/profiles/[profileId]/command-preview-card.test.tsx`

## Dependencies

None.

## Risks

- `check-agentctl-deps.sh` enforces strict dependency boundaries on
  `internal/agentctl`; `acpcompat/copilot.go` must use only stdlib and
  `internal/agentctl/types` without importing `internal/agent/agents`.

## Parallelism

`sequential`

## Inputs

- [`docs/specs/agents/requirements/copilot-context-tier.md`](../../specs/agents/requirements/copilot-context-tier.md)
- [`docs/specs/agents/system-design/copilot-context-tier.md`](../../specs/agents/system-design/copilot-context-tier.md)
- Pattern reference: `apps/backend/internal/agentctl/acpcompat/grok.go` and
  `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_grok.go`

## Results

Completed with strict Red-Green-Refactor TDD:
- Added `apps/backend/internal/agentctl/acpcompat/copilot.go` and `copilot_test.go` (`NormalizeCopilotConfigOptions`, `IsSynthesizedCopilotContextOption`, `ParseCopilotContextTierFromArgs`, `HasCopilotContextCLIFlag`, `IsValidCopilotContextTier`), deferring to any native `context_tier` / `contextTier` / `context` wire option.
- Propagated `LaunchArgs` from `process.Manager.buildAdapterConfig` into `acp.NewAdapter` / `copilotDialect` (`dialect_copilot.go`, `adapter_session.go`, `adapter_updates.go`) and preserved `context_tier` in `internal/agentctl/server/utility/acp_executor.go` (`copilot_probe_context_tier_test.go`) without sending unsupported `SetConfigOption("context_tier", ...)` RPCs.
- Added `ConfigOptions` to `agents.CommandOptions` and `agents.PassthroughOptions`, emitting `--context <default|long_context>` in `CopilotACP` and `MockAgent("copilot-acp")` unless overridden by `CLIFlagTokens` (`copilot_acp_test.go`).
- Plumbed `config_options` through backend `CommandPreviewRequest` (`agent_config.go`, `handlers.go`) and frontend `CommandPreviewCard` (`settings-api.ts`, `app/actions/agents.ts`, `command-preview-card.tsx`, `agent-profile-page.tsx`, `command-preview-card.test.tsx`).
- Verified `bash scripts/check-agentctl-deps.sh`, `TMPDIR=/private/tmp go -C apps/backend test -race ./internal/agentctl/acpcompat/... ./internal/agentctl/server/adapter/transport/acp/... ./internal/agentctl/server/utility/... ./internal/agent/agents/... ./internal/agent/settings/...`, `bash scripts/lint-go-changed ...` (`0 issues.`), Vitest (`command-preview-card.test.tsx`), `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet`.
