---
id: "08-required-store-health"
title: "Expose required-store health"
status: done
wave: 8
depends_on:
  - "04-required-store-bootstrap"
  - "06-catalog-conformance"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007
acceptance_criteria:
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.4
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003.1
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.1
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.2
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.3
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.4
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.5
system_design:
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
---

# Task 08: Expose required-store health

## Summary

Add runtime probes, readiness projection, authenticated diagnostics, and stateful request gating. Preserve the liveness endpoint.

## In scope

- Run immediate and periodic pool and table probes.
- Add aggregate runtime health to the tracker.
- Extend `/ready` with a safe persistence failure shape.
- Add authenticated persistence diagnostics under the system API.
- Return stable `503` errors for stateful HTTP and WebSocket callers.
- Recover readiness and request service after a successful probe.
- Add structured transition logs and cleanup for the probe loop.

## Out of scope

- Return raw SQL or driver errors from public endpoints.
- Change `/health` status, body, or desktop token behavior.
- Repair missing tables at runtime.

## Acceptance

- One unhealthy catalog store makes readiness and stateful traffic unavailable.
- Diagnostics list every catalog entry in stable order with sanitized errors.
- A later successful probe restores readiness while `/health` remains unchanged.

## Verification

```bash
go test -race ./internal/persistence/requiredstores ./internal/backendapp ./internal/system -run '^Test(PersistenceHealth|ReadyHandlerPersistence|PersistenceDiagnostics|PersistenceUnavailableMiddleware|HealthHandler)' -v
```

## Files likely touched

- `apps/backend/internal/persistence/requiredstores/health.go`
- `apps/backend/internal/persistence/requiredstores/health_test.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/health_test.go`
- `apps/backend/internal/backendapp/middleware.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/system/persistence/handler.go`
- `apps/backend/internal/system/persistence/handler_test.go`
- `apps/backend/internal/system/system.go`

## Dependencies

- Task 04 supplies tracker results and initialized stores.
- Task 06 proves catalog table ownership.

## Risks

- The middleware can block its own diagnostic route if exclusions run after the gate.

## Parallelism

`sequential`

## Inputs

- System design sections: Runtime health, Caller errors, Diagnostics.
- Existing `/health` and `/ready` tests.

## Results

Implemented immediate and periodic probes, readiness projection, authenticated
diagnostics, stable stateful-request `503` responses, recovery, and liveness
preservation. The required-store, backendapp, system, and middleware tests
passed, including the full backendapp package suite.
