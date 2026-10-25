---
id: "03-instance-stop"
title: "Idempotent agent instance stop"
status: done
wave: 3
depends_on:
  - "02-agent-stderr-exit"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AGENTCTL-INSTANCE-STOP-001
acceptance_criteria:
  - AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.1
  - AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.2
  - AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.3
  - AC-PLATFORM-AGENTCTL-INSTANCE-STOP-001.4
system_design:
  - ../../specs/platform/system-design/agentctl-instance-stop.md
---

# Task 03: Idempotent agent instance stop

## Summary

Make an overlapping stop call succeed when the same instance has already been
fully removed by another stop operation. Keep unknown-instance responses and
real cleanup failures distinct.

## In scope

- Serialize and recheck the original instance pointer after `stopMu`.
- Treat removal of that same pointer as an already-satisfied stop.
- Release ports once and retain the current failure retry behavior.
- Add deterministic manager and control-boundary regression tests.

## Out of scope

- Changing the `ControlClient` lost-response retry contract.
- Changing process or HTTP-server grace periods.
- Changing instance persistence, IDs, or agent protocol behavior.

## Acceptance

- Overlapping stops for one instance do not produce a 500 solely because the
  first stop removed the instance.
- Unknown IDs remain non-mutating 404 responses, and a different instance
  reusing the ID is never mistaken for the original instance.
- Genuine cleanup failures remain HTTP 500/error-level and retain the instance
  and port for retry; successful cleanup releases the port once.

## Verification

```bash
cd apps/backend && go test ./internal/agentctl/server/instance ./internal/agentctl/server/api -count=1
```

## Files likely touched

- `apps/backend/internal/agentctl/server/instance/manager.go`
- `apps/backend/internal/agentctl/server/instance/manager_shutdown_test.go`
- `apps/backend/internal/agentctl/server/api/control_server.go`
- `apps/backend/internal/agentctl/server/api/*_test.go`

## Dependencies

Task 02 is a prior plan wave. No data migration or external service is
required.

## Risks

- A missing map entry is only benign when it follows teardown of the exact
  pointer captured before waiting; an ID reuse must remain an error.

## Parallelism

`sequential`

## Inputs

- `docs/specs/platform/requirements/agentctl-instance-stop.md`.
- `docs/specs/platform/system-design/agentctl-instance-stop.md`.
- Existing `Manager.StopInstance` failure-retention tests and
  `ControlClient.DeleteInstance` 404 retry test.

## Results

Refactored stop handling so the initial instance pointer is passed through a
serialized stop operation. A pointer that was already removed by a successful
stop now returns success, while a replacement pointer still returns not found.
Added manager concurrency, port-reuse, replacement-safety, and unknown-control-
route coverage.

Verification:

- `go test ./internal/agentctl/server/instance ./internal/agentctl/server/api -count=1` passed with 463 tests.
