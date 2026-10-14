---
id: "01-profile-contracts"
title: "Persist profile selection"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-PREP-001
acceptance_criteria:
  - AC-AGENTS-MCP-PREP-001.1
  - AC-AGENTS-MCP-PREP-001.4
system_design:
  - ../../specs/agents/system-design/agent-mcp-preparation.md
---

# Task 01: Persist profile selection

## Summary

Persist profile selection according to the linked requirements and design.

## In scope

Own settings model/store migrations/DTO/controller contracts, generated backend/web profile contract, lifecycle AgentProfileInfo/resolver, and their tests. Persist and validate inherit|selected plus exact selected IDs. Handle explicit empty PATCH, duplication, migration rebuild and existing enabledOnlyUpdate omission. Export neutral sanitized discovery DTOs in mcpconfig if required; coordinate names before consumers start. Do not edit profile form state/UI or runtime import functions.

## Out of scope

Other work-order ownership, commits, personal credential changes and unrelated refactors.

## Acceptance

- Satisfy every referenced acceptance criterion within this boundary.
- Add failing behavior tests first, implement, then pass targeted checks.
- Preserve existing dirty work and report exact validation evidence.

## Verification

```bash
(cd apps/backend && GOCACHE=/tmp/kandev-cursor-review-go-cache go test ./internal/agent/settings/... ./internal/agent/runtime/lifecycle -run 'Profile|Migration|Cursor' -count=1)
```

## Files likely touched

- `apps/backend/internal/agent/settings/models/models.go`
- `apps/backend/internal/agent/settings/store/sqlite.go`
- `apps/backend/internal/agent/settings/dto/profile_contract.go`
- `apps/backend/internal/agent/settings/controller/profile_crud.go`
- `apps/backend/internal/agent/runtime/lifecycle/profile_resolver.go`
- `apps/backend/internal/agent/runtime/lifecycle/types.go`

## Dependencies

None

## Risks

See plan compatibility and concurrency risks; escalate contract changes to coordinator.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/agents/requirements/agent-mcp-preparation.md).
- [System design](../../specs/agents/system-design/agent-mcp-preparation.md).
- Existing scoped AGENTS.md, TDD skill and neighboring test patterns.

## Results

Implemented and verified on 2026-09-28. The profile model/store now persist
`mcp_selection_mode` (`inherit` default) and `mcp_selected_servers` (empty
default), with migration rebuild preservation. Create defaults, PATCH omission
and explicit empty selection, validation, duplicate, lifecycle resolution, and
the generated web profile contract are covered. Existing Cursor credential and
plugin import booleans remain independent and are preserved through duplicates.

Validation passed:

- `(cd apps/backend && GOCACHE=/tmp/kandev-cursor-review-go-cache go test ./internal/agent/settings/... ./internal/agent/runtime/lifecycle -run 'Profile|Migration|Cursor' -count=1)`
- `(cd apps/backend && GOCACHE=/tmp/kandev-cursor-review-go-cache go run ./cmd/sqlguard ./internal)`
- `(cd apps/backend && GOCACHE=/tmp/kandev-cursor-review-go-cache go test -race ./internal/persistence/storeconformance -count=1)`
- `(cd apps/web && pnpm exec vitest run lib/settings-discovery/profile-contract.test.ts)`
- `(cd apps/backend && GOCACHE=/tmp/kandev-cursor-review-go-cache go run ./cmd/settings-catalog)`
