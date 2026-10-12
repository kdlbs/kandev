---
id: "03-settings-discovery"
title: "Expose settings discovery"
status: done
wave: 3
depends_on:
  - "02-profile-mutations"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-001
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-002
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-003
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-007
acceptance_criteria:
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-001.1
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-001.2
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-001.3
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-001.4
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-001.6
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-001.7
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-001.8
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-002.3
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-002.6
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-003.3
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-003.5
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-007.1
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-007.2
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-007.3
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-007.4
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-007.5
system_design:
  - ../../specs/platform/system-design/agent-settings-parity.md
---

# Task 03: Expose settings discovery

## Summary

Expose bounded metadata search, detailed setting descriptions, and explicit saved-value reads.
Use the existing capability resolver for model-dependent choices.

## In scope

- Register `search_settings_kandev`, `describe_setting_kandev`, and `get_settings_kandev` for configuration and external profiles.
- Add `list_settings_resources_kandev` with authorized, bounded targets and scope-bound cursors.
- Use generic targets and validated description context. No profile-specific fields appear in the advertised envelopes.
- Compose these with Task 02's compact updater and hide adopted legacy read/update tools from the configuration surface.
- Keep lifecycle tools and unadopted domains available. Preserve existing external tool registrations.
- Wire metadata search and target-aware reads through the agent domain adapter.
- Add query limits, stable results, covered-domain metadata, typed failures, and sensitive-value projections.
- Describe current defaults, model inheritance, provider status, and change timing.
- Return compact search results with resource type, field path, match reason, and support status.
- Supply dependencies and optional full operation schemas through description, including create-time required fields.
- Maintain an independent search-query set with ranking, ambiguity, and no-match assertions.
- Add target-selection and no-existence-leak tests through both trusted session context and external authenticated calls.
- Update the configuration prompt and public docs for the new tools and initial scope.

## Out of scope

- New mutation tools, other settings domains, embeddings, and file import/export.
- Provider-specific choice tables or secret resolution.

## Acceptance

- Search and description return bounded, deterministic metadata and explicit support limits without saved-value search.
- Profile reads and model-context discovery use existing domain services, preserve failure states, and redact sensitive values in new projections.
- Only the intended MCP surfaces expose the tools. Prompt and public docs describe compact updates, lifecycle operations, and external compatibility.

## Verification

Run from the repository root.

```bash
rtk proxy go -C apps/backend test ./internal/settingscatalog ./internal/agent/settings/catalog ./internal/mcp/server ./internal/mcp/handlers ./internal/mcp/profile ./internal/agent/settings/controller
rtk proxy go -C apps/backend run ./cmd/settings-catalog --check
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
rtk git diff --check
```

Include tests for missing targets, unknown keys, cursor bounds, unavailable capabilities, redaction, and create-time model context.
Include the curated queries in the design and prove that search never embeds full resource schemas or selects a target.
Use the existing profile matrix and prompt synchronization tests to prove registration consistency.
Update public docs coverage for every new tool.

## Files likely touched

- `apps/backend/internal/settingscatalog/catalog.go` and tests (new).
- `apps/backend/internal/settingscatalog/search_quality_test.go` (new).
- `apps/backend/internal/agent/settings/catalog/`.
- `apps/backend/internal/mcp/server/settings_discovery.go` and tests (new).
- `apps/backend/internal/mcp/server/settings_resources.go` and tests (new).
- `apps/backend/internal/mcp/handlers/settings_discovery.go` and tests (new).
- `apps/backend/internal/mcp/server/server.go` and profile/metadata tests.
- `apps/backend/internal/mcp/handlers/handlers.go`.
- `apps/backend/pkg/websocket/actions.go` and backend composition.
- `apps/backend/config/prompts/config-context.md`.
- `docs/public/automation-and-mcp.md`, `agents-and-profiles.md`, `websocket-api.md`, and `coverage.json`.

## Dependencies

Tasks 01 and 02 establish metadata and functioning mutation operations.

## Risks

- A saved profile is not an active session's effective state.
- Provider probe failures must not remove static metadata.
- Redaction markers must never become values suitable for full-document replacement.
- Existing raw MCP-document tools retain their documented exposure contract.
- Later work orders in this package adopt workflow and executor groups. Task 03 is an intermediate slice, not delivery completion.

## Parallelism

`sequential`

## Inputs

- System design: Catalog, Discovery tools, Dynamic choices, Sensitive values.
- Existing saved-prompt discovery tools and `Controller.ResolveAgentModelConfig`.
- `/docs-maintainer` and public-docs coverage conventions.

## Results

Completed on 2026-09-09.

- Added compact search, description, saved-value read, and authorized resource lookup tools.
- Added closed outer schemas with catalog-backed dynamic descriptions and target validation.
- Contract, catalog, handler, server, and desktop/mobile MCP transport checks passed.
