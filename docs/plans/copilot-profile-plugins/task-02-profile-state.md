---
id: "02-profile-state"
title: "Persist Copilot plugin selections across profile contracts"
status: completed
wave: 3
depends_on: ["01-inventory-discovery"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-COPILOT-PLUGINS-002
  - REQ-AGENTS-COPILOT-PLUGINS-007
acceptance_criteria:
  - AC-AGENTS-COPILOT-PLUGINS-002.1
  - AC-AGENTS-COPILOT-PLUGINS-002.2
  - AC-AGENTS-COPILOT-PLUGINS-002.3
  - AC-AGENTS-COPILOT-PLUGINS-002.4
  - AC-AGENTS-COPILOT-PLUGINS-002.5
  - AC-AGENTS-COPILOT-PLUGINS-002.6
  - AC-AGENTS-COPILOT-PLUGINS-007.1
system_design:
  - ../../specs/agents/system-design/copilot-profile-plugins.md
---

# Task 02: Persist profile selections

## Summary

Deliver the complete saved preference and browser draft contract before rendering its selector.
Preserve omitted values in concurrent ordinary writes and through every database lifecycle path.

## In scope

- Add `copilot_selected_plugins` to domain models, DTOs, field contracts, controller requests/conversions, duplication, and runtime profile snapshots.
- Reject null, invalid shape, excessive bytes/count, duplicates, and nonempty inapplicable selections. Normalize valid sets deterministically.
- Add fresh schema, additive migration, replay coverage, legacy table rebuild/copy, shared insert/update/select/scan, and upgrade fixture updates.
- Preserve omission at the atomic SQL write boundary and return/event the committed value.
- Extend HTTP/MCP settings mutations, reads, boot/events, domain catalog, fallback catalog, and generated discovery contracts.
- Extend browser types, normalization, create/update serialization, agent-level saves, dirty state, save acknowledgements, and external-conflict reconciliation.
- Cover an Office concrete execution-profile resolution without adding state to dynamic routing profiles.

## Out of scope

Rendered UI, native root persistence, plugin commands during save, global activation, or new discovery schema unrelated to this preference.

## Acceptance

1. Empty defaults and explicit clearing survive create/update/duplicate/restart/migration/replay, while omitted concurrent updates preserve committed selections.
2. HTTP/MCP/catalog/browser contracts agree on applicability, limits, replacement semantics, and process-start timing.
3. Dirty/Reset/conflict and in-flight edit tests preserve the selection set without introducing JSX or a new save contributor.

## Verification

Start with failing contract and concurrency tests. Run from the repository root.
Install workspace dependencies once before the first package command in a fresh worktree.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -trimpath ./internal/agent/settings/... ./internal/settingscatalog/... ./internal/mcp/handlers ./internal/mcp/server -run 'TestCopilotPluginSelection|TestProfileContract' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agent/settings/store ./internal/agent/settings/controller -run TestCopilotPluginSelectionConcurrentOmission -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle -run 'TestStoreProfileResolver' -count=1)
(cd apps/backend && go run ./cmd/settings-catalog --check)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
(cd apps/web && pnpm exec vitest run lib/api/domains/agent-profile-normalize.test.ts lib/settings-discovery/profile-contract.test.ts components/settings/agent-profile-dirty.test.ts components/settings/agent-profile-reconciliation.test.ts components/settings/agent-profile-page-state.test.ts components/settings/agent-profile-save-catalogue.test.tsx 'app/settings/agents/[agentId]/agent-save-helpers.test.ts')
(cd apps/web && pnpm run typecheck)
git diff --check
```

Generate catalog JSON with `(cd apps/backend && go run ./cmd/settings-catalog)` before the check.
For dialect-sensitive persistence changes, add the environment-gated PostgreSQL cases required by backend guidance.
Run the race/conformance commands with `KANDEV_TEST_POSTGRES_DSN` when a disposable PostgreSQL database is configured.
Record unavailable PostgreSQL coverage explicitly. Do not claim it passed from SQLite evidence.
Run changed-file Go lint with the final explicit file list after targeted checks.

## Files likely touched

- `apps/backend/internal/agent/settings/{models,dto,controller,store,catalog}/` and focused `copilot_plugins_test.go` files.
- `apps/backend/internal/settingscatalog/defaults.go` and settings binding tests.
- `apps/backend/internal/mcp/handlers/config_agent_handlers.go`, tool schema/contract tests in `internal/mcp/server/`.
- `apps/backend/internal/agent/runtime/lifecycle/{types.go,profile_resolver.go,profile_resolver_test.go}`.
- `apps/backend/internal/persistence/storeconformance/` and its required upgrade fixtures/manifests.
- `apps/web/lib/types/{agent-profile.ts,backend.ts}` and `lib/api/domains/agent-profile-normalize.ts`.
- `apps/web/lib/settings-discovery/{profile-contract.ts,contract.generated.json,profile-contract.generated.json}`.
- `apps/web/components/settings/{agent-profile-page-state.ts,agent-profile-dirty.ts,agent-profile-reconciliation.ts,agent-profile-page.tsx}` (field mapping only).
- `apps/web/app/settings/agents/[agentId]/agent-save-helpers.ts`, profile creation state, and associated tests.

## Dependencies

WO-01 fixes the canonical ID contract. Do not duplicate its parser or identity rules.

## Risks

Pointer fields alone do not distinguish JSON null or protect a read/modify/write race.
Catalog generation replaces the shared fallback descriptor with the domain descriptor.
Table recreation and full field enumerations can otherwise silently remove the new preference.

## Parallelism

`sequential`

## Inputs

- [Design](../../specs/agents/system-design/copilot-profile-plugins.md#profile-state-and-persistence) and editor-state section.
- Existing Cursor field round-trip tests and profile update/duplication/migration tests.
- Backend scoped guidance and `.agents/skills/tdd/references/backend-tests.md` for persistence coverage.

## Results

Completed 2026-10-27.

- `go test -trimpath ./internal/agent/settings/... ./internal/settingscatalog/... ./internal/mcp/handlers ./internal/mcp/server -run 'TestCopilotPluginSelection|TestProfileContract|TestHandle.*AgentProfile' -count=1`: passed.
- `go test -trimpath -race ./internal/agent/settings/store -run TestCopilotPluginSelectionConcurrentOmission -count=1`: passed.
- `go run ./cmd/settings-catalog --check`: passed after regeneration.
- `go run ./cmd/sqlguard ./internal`: passed.
- `go test -trimpath -race ./internal/persistence/storeconformance -count=1`: passed.
- Browser normalization, dirty state, reconciliation, save payloads, duplication, and the SQL omission-preserving update are covered.
- PostgreSQL conformance evidence is limited to `storeconformance`; no live PostgreSQL DSN was configured for a separate race run.
