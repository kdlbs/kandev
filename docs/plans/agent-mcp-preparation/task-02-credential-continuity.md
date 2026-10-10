---
id: "02-credential-continuity"
title: "Preserve native credential refresh"
status: complete
wave: 2
depends_on: ["01-profile-contracts"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-PREP-003
acceptance_criteria:
  - AC-AGENTS-MCP-PREP-003.4
system_design:
  - ../../specs/agents/system-design/agent-mcp-preparation.md
---

# Task 02: Preserve native credential refresh

## Summary

Preserve native credential refresh according to the linked requirements and design.

## In scope

Own mcpconfig/cursor_auth_bridge.go, cursor_auth_selection_test.go and new credential provenance files/tests only. Track source identity/fingerprint and projected fingerprint without storing secrets in sidecar. Preserve a native-updated master only while its eligible source object is unchanged; removed source drops, changed source reselects, invalid provenance cannot authorize master fallback. Preserve existing regular-file task auth and containment/atomic permission guarantees.

## Out of scope

Other work-order ownership, commits, personal credential changes and unrelated refactors.

## Acceptance

- Satisfy every referenced acceptance criterion within this boundary.
- Add failing behavior tests first, implement, then pass targeted checks.
- Preserve existing dirty work and report exact validation evidence.

## Verification

```bash
(cd apps/backend && GOCACHE=/tmp/kandev-cursor-review-go-cache go test -race ./internal/agent/mcpconfig -count=1)
```

## Files likely touched

- `apps/backend/internal/agent/mcpconfig/cursor_auth_bridge.go`
- `apps/backend/internal/agent/mcpconfig/cursor_auth_selection_test.go`

## Dependencies

01-profile-contracts

## Risks

See plan compatibility and concurrency risks; escalate contract changes to coordinator.

## Parallelism

parallel-safe after Task 01 freezes shared contracts; disjoint implementation ownership.

## Inputs

- [Requirements](../../specs/agents/requirements/agent-mcp-preparation.md).
- [System design](../../specs/agents/system-design/agent-mcp-preparation.md).
- Existing scoped AGENTS.md, TDD skill and neighboring test patterns.

## Results

Implemented and verified on 2026-09-28. The Cursor auth bridge now writes a
0600 credential-free provenance sidecar with per-server source IDs and
fingerprints. It preserves whole native-refreshed objects only while the
selected source object remains unchanged, records the last projected
fingerprint, reselects changed sources, and drops removed sources. Removing the
last eligible source deletes the owned master so existing task symlinks fail
closed. Pending/invalid provenance cannot authorize old master data. Master
publication rechecks the observed file fingerprint immediately before atomic
replacement and fails safely if a concurrent native write was observed; Cursor
does not share Kandev's process lock, so an update after that final check cannot
be serialized by this process.

Validation passed:

- `(cd apps/backend && GOCACHE=/tmp/kandev-cursor-review-go-cache go test -race ./internal/agent/mcpconfig -count=1)`
- `(cd apps/backend && GOCACHE=/tmp/kandev-cursor-review-go-cache GOLANGCI_LINT_CACHE=/tmp/kandev-cursor-review-lint-cache golangci-lint run ./internal/agent/settings/... ./internal/agent/mcpconfig/...)`
