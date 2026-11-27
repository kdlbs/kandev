---
id: "01-discovery-cache-coherence"
title: "Keep custom-agent discovery cache coherent"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-CUSTOM-ACP-001
acceptance_criteria:
  - AC-AGENTS-CUSTOM-ACP-001.2
system_design:
  - ../../specs/agents/system-design/custom-acp-agents.md
---

# Task 01: Keep custom-agent discovery cache coherent

## Summary

Invalidate discovery after a custom-agent persistence failure rolls back the
temporary registry entry. This prevents an in-flight discovery sweep from
publishing the failed agent after the registry has removed it.

## Acceptance

- A registry change drops cached discovery results before the cache TTL expires.
- A sweep that started before invalidation cannot repopulate the cache with its
  superseded membership or capability data.
- A failed custom-agent create operation removes the temporary registry entry
  and invalidates discovery before returning the persistence error.

## Verification

```bash
cd apps/backend
go test -race -count=1 ./internal/agent/discovery/...
go test -count=1 -run 'CustomTUI|Discovery' ./internal/agent/settings/controller/
```

## Files

- `apps/backend/internal/agent/discovery/discovery.go`
- `apps/backend/internal/agent/discovery/discovery_test.go`
- `apps/backend/internal/agent/settings/controller/custom_tui.go`
- `apps/backend/internal/agent/settings/controller/custom_tui_test.go`
