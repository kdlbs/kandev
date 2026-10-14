---
id: "02-profile-contract"
title: "Profile preference and store contract"
status: complete
wave: 3
depends_on: ['00-compatibility-evidence']
plan: "plan.md"
requirements:
  - REQ-AGENTS-CURSOR-PLUGIN-MCP-002
acceptance_criteria:
  - AC-AGENTS-CURSOR-PLUGIN-MCP-002.1
  - AC-AGENTS-CURSOR-PLUGIN-MCP-002.5
system_design:
  - ../../specs/agents/system-design/cursor-plugin-mcp-import.md
---

# Task 02: Profile preference and store contract

## Summary

Persist the default-enabled preference across backend and browser data contracts, including execution-profile resolution. This work order does not render a new control.

## In scope

- Mirror CursorMCPAuthEnabled across models, DTO bool pointers, controller create/update/duplicate/conversion, SQLite initialization/migrations/table rebuild/copies/read/write/scan, runtime types and profile resolver.
- Update catalog defaults and generated settings-discovery contracts. Enumerate boot/event/action/import/export mappings that carry profile fields.
- Add browser types, API normalization and create/save mappings, draft state, dirty detection and conflict reconciliation. Omit an unchanged preference in both profile-page and agent-level saves.
- TDD: `sqlite_cursor_plugins_mcp_test.go` with `TestCursorPluginsMCPPreferenceRoundTrip` and `TestMigration_LegacyProfileDefaultsCursorPluginsMCPEnabled`; controller `cursor_plugins_mcp_profile_test.go` covers omitted create/patch, false, duplicate and unrelated updates; extend `TestStoreProfileResolver_ResolveProfile_Success` and existing browser normalization/save/reconciliation suites.

## Out of scope

Plugin installation, marketplace APIs, remote credential forwarding, unrelated CLI strategies, and production changes outside this work order.

## Acceptance

1. Existing rows and omitted creates default true; false survives restart, duplication and all round trips, and omitted updates preserve it.
2. Execution-profile resolution supplies the actual launch preference, including Office execution-profile cases.
3. Catalog generation/check and browser tests demonstrate complete field wiring without lost drafts or stale overwrites.

## Verification

Run from the repository root. Implementation work uses TDD for changed logic. Commands below are planned, not executed evidence.

```bash
(cd apps/backend && go test ./internal/agent/settings/... ./internal/settingscatalog/...)
(cd apps/backend && go test ./internal/agent/runtime/lifecycle/... -run 'TestStoreProfileResolver')
(cd apps/backend && go run ./cmd/settings-catalog --check)
(cd apps/web && pnpm exec vitest run lib/api/domains/agent-profile-normalize.test.ts components/settings/agent-profile-page-state.test.ts components/settings/agent-profile-reconciliation.test.ts 'app/settings/agents/[agentId]/agent-save-helpers.test.ts')
(cd apps/web && pnpm run typecheck)
```

## Files likely touched

- `apps/backend/internal/agent/settings/models/models.go`
- `apps/backend/internal/agent/settings/dto/`
- `apps/backend/internal/agent/settings/controller/`
- `apps/backend/internal/agent/settings/store/`
- `apps/backend/internal/settingscatalog/`
- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/profile_resolver.go`
- `apps/backend/internal/agent/runtime/lifecycle/profile_resolver_test.go`
- `apps/web/lib/types/agent-profile.ts`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/api/domains/agent-profile-normalize.ts`
- `apps/web/lib/settings-discovery/`
- `apps/web/components/settings/agent-profile-page-state.ts`
- `apps/web/components/settings/agent-profile-dirty.ts`
- `apps/web/components/settings/agent-profile-reconciliation.ts`
- `apps/web/app/settings/agents/[agentId]/agent-save-helpers.ts`

## Dependencies

00-compatibility-evidence.

## Risks

False/omitted confusion and field-enumerating copies can reset the preference. Install frozen workspace dependencies before any pnpm commands in a fresh worktree.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/cursor-plugin-mcp-import.md).
- [System design](../../specs/agents/system-design/cursor-plugin-mcp-import.md), especially the sections named by this work order.
- Existing Cursor auth preference and lifecycle tests; ADR 0014.
- [Plan](plan.md) for compatibility, test mapping and remaining evidence gates.

## Results

Added `cursor_plugins_mcp_enabled` to models, SQLite schema and migrations, DTOs, settings catalog, and frontend API types. Regenerated discovery contracts. All Go and Vitest contract/state tests passed.
