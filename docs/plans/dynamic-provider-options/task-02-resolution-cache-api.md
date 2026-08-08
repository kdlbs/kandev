---
id: dynamic-provider-options-02
title: Add resolution cache API
status: done
wave: 2
depends_on:
  - dynamic-provider-options-01
plan: docs/plans/dynamic-provider-options/plan.md
spec: docs/specs/agents/dynamic-provider-options.md
---

# Add resolution cache API

## Scope

Expose the model-aware probe through host utility resolution, a bounded
runtime-only cache, and the authenticated agent-settings HTTP endpoint.

## Acceptance conditions

1. Identical agent/context requests share cached or in-flight work, refresh
   bypasses/invalidate the entry, and the cache cannot persist data to SQLite.
2. `POST /api/v1/agent-models/:agentName/resolve` validates the request,
   returns a complete normalized snapshot on success, and sanitizes failures.
3. Existing `GET /api/v1/agent-models/:agentName` behavior remains compatible.

## Files

- `apps/backend/internal/agent/hostutility/types.go`
- `apps/backend/internal/agent/hostutility/cache.go`
- `apps/backend/internal/agent/hostutility/manager.go`
- `apps/backend/internal/agent/hostutility/public.go`
- `apps/backend/internal/agent/hostutility/manager_test.go`
- `apps/backend/internal/agent/settings/dto/dto.go`
- `apps/backend/internal/agent/settings/controller/agent_config.go`
- `apps/backend/internal/agent/settings/controller/agent_config_test.go`
- `apps/backend/internal/agent/settings/handlers/handlers.go`
- `apps/backend/internal/agent/settings/handlers/handlers_test.go`

## Dependencies and inputs

- `dynamic-provider-options-01` utility request/result.
- Existing host utility availability, refresh, authorization, and settings
  route conventions.

## Output contract

Provide a typed controller method and HTTP route matching the spec's request
context and response shape. Cache key construction must be deterministic for
maps, include future context fields, and have explicit TTL/invalidation tests.

## Checks

```bash
cd apps/backend && go test ./internal/agent/hostutility ./internal/agent/settings/controller ./internal/agent/settings/handlers
```

Results: `cd apps/backend && go test ./internal/agentctl/server/utility ./internal/agent/hostutility ./internal/agent/settings/controller ./internal/agent/settings/handlers -count=1` — passed.
