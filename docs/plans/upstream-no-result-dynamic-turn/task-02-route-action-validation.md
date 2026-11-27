---
id: "02-route-action-validation"
title: "Report route actions on non-dynamic sessions as validation errors"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.11
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-02.md
---
# Task 02: Report route actions on non-dynamic sessions as validation errors

## Summary

Return a typed, named rejection for a route action on a session whose selected profile is not dynamic, and map it to a validation error at the WebSocket boundary.

## In scope

- Write `TestWSRouteActionReportsNonDynamicProfileAsValidation` first and confirm that it fails on the base commit.
- `apps/backend/internal/orchestrator/service.go`: add the `ErrRouteActionRequiresDynamicProfile` sentinel next to `ErrRouteActionActiveTurn`.
- `apps/backend/internal/backendapp/dynamic_routing.go`: return the sentinel from `loadDynamicRouteActionSession` instead of an untyped error.
- `apps/backend/internal/orchestrator/handlers/handlers.go`: map the sentinel to `ErrorCodeValidation` with its message.
- Tests in `backendapp/dynamic_routing_test.go` and `orchestrator/handlers/handlers_test.go`.

## Out of scope

- Route action behavior for dynamic sessions, stale generations, and active turns.
- Frontend changes.

## Acceptance

- A route action on a concrete-profile session returns a validation error naming the reason.
- No route or session state changes, and the handler does not log it as an internal failure.
- Active-turn conflicts and internal failures keep their current codes.

## Validation

```bash
cd apps/backend
go test ./internal/backendapp -run 'RouteAction|DynamicRoute' -count=1
go test ./internal/orchestrator/handlers -run 'RouteAction|Ws' -count=1
go build ./... && go vet ./internal/backendapp/... ./internal/orchestrator/...
```
