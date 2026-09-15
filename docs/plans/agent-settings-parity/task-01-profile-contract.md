---
id: "01-profile-contract"
title: "Establish the profile settings contract"
status: done
wave: 1
depends_on:
  - "00-coverage-inventory"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-001
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-002
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-005
acceptance_criteria:
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-001.5
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-002.1
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-002.2
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-005.1
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-005.3
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-005.4
system_design:
  - ../../specs/platform/system-design/agent-settings-parity.md
---

# Task 01: Establish the profile settings contract

## Summary

Create one typed contract for profile create/update requests and their discovery metadata.
Move the existing HTTP adapter onto that contract without changing its domain semantics.

## In scope

- Export shared request DTOs and one conversion path into controller requests.
- Define catalog metadata types and agent-domain descriptors for the complete profile inventory.
- Generate schemas from typed contracts using existing dependencies and the existing Draft 7 compiler.
- Add deterministic metadata export with `--check` and a frontend snapshot at `apps/web/lib/settings-discovery/profile-contract.generated.json`.
- Add independent DTO/response field coverage and explicit noneditable classifications.
- Define shared target, context, and operation-option metadata for all domains before freezing the compact envelopes.
- Export the all-domain snapshot alongside the profile subset. Pending domain adapters remain explicit until their work orders complete.
- Reject duplicate keys, missing owners, unresolved bindings, and writable entries without validators or authority rules.
- Wire registry and freshness checks into existing backend/frontend CI, including relevant path filters and a wiring guard test.

## Out of scope

- MCP registration changes, new reads, editor state, and other domain adapters.
- Changes to profile defaults, database schema, or provider runtime behavior.

## Acceptance

- HTTP profile requests use the shared contracts and retain creation defaults, update omission, clearing, and existing null behavior.
- Every profile field maps to catalog metadata or a reasoned exclusion, including nested types and the separate MCP document.
- Added-field and removed-descriptor fixtures fail coverage. Generated schemas compile and metadata export is deterministic.

## Verification

Run from the repository root. These commands target new and existing profile contracts.

```bash
rtk proxy go -C apps/backend test ./internal/agent/settings/dto ./internal/agent/settings/catalog ./internal/agent/settings/handlers ./internal/agent/settings/controller ./internal/settingscatalog
rtk proxy go -C apps/backend run ./cmd/settings-catalog --check
rtk node --test scripts/settings-contract-ci.test.mjs
rtk git diff --check
```

The exporter resolves its output relative to the backend module root. `--check` never writes files.

## Files likely touched

- `apps/backend/internal/agent/settings/dto/profile_contract.go` and tests (new).
- `apps/backend/internal/agent/settings/controller/profile_crud.go` and adjacent tests.
- `apps/backend/internal/agent/settings/handlers/handlers.go` and profile tests.
- `apps/backend/internal/agent/settings/catalog/` (new).
- `apps/backend/internal/settingscatalog/` (new).
- `apps/backend/cmd/settings-catalog/` (new).
- `apps/web/lib/settings-discovery/profile-contract.generated.json` (new).
- `.github/workflows/backend-tests.yml` and `frontend-tests.yml`.
- `scripts/settings-contract-ci.test.mjs` (new).

## Dependencies

Task 00 supplies the complete control and domain inventory.

## Risks

- Null and omission can collapse during schema generation or decoding.
- Computed response fields must not become writable merely because they appear in a DTO.
- Dynamic map contents need typed value schemas without a frozen provider-key list.

## Parallelism

`sequential`

## Inputs

- System design: Catalog, Shared requests, Coverage.
- `internal/mcp/toolschema/schema.go` and its compilation tests.
- Existing HTTP profile request bodies and controller defaults.
- `.github/AGENTS.md` before CI workflow edits.

## Results

Completed on 2026-09-09.

- Added shared profile create/update DTOs and omission-preserving contract tests.
- Added the domain catalog, deterministic exporter, generated snapshots, and CI freshness checks.
- Shared profile fields are checked against the runtime registry.
- The focused backend suite and catalog freshness check passed.
