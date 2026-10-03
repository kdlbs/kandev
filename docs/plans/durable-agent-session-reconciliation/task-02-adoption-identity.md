---
id: "02-adoption-identity"
title: "Restore adopted delivery identity and capability"
status: complete
wave: 2
depends_on:
  - 01-merge-baseline
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.4
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.5
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/agents/system-design/harness-session-continuity.md
---

# Task 02: Restore adopted delivery identity and capability

## Summary

Adopt the original delivery owner without restarting or reinitializing the harness.

## In scope

Adopt the original delivery owner without restarting or reinitializing the harness.
- Extend authenticated delivery status and its client DTO with owner identity, storage health, version, bounds, and bounded submission evidence.
- Source incarnation/generation and original workspace from existing SQL records. Compare the authenticated instance descriptor with those records.
- Restore execution stream/incarnation/generation and current durable turn/submission correlation before event intake or prompt admission.
- Query the SQL projected cursor for the exact generation stream. Never substitute session ID or zero for a failed owner lookup.
- Distinguish absent SQL history from a database error. An empty cursor is valid only when retained bounds permit sequence one.
- Populate the new client's capability cache through status discovery. Do not call ACP initialize, load, resume, new, or prompt for adoption.
- Treat an unsupported status route as legacy only with positive old-peer evidence and no unresolved durable work.
- Reuse existing owner validation and recovery guards. Preserve native session identity and workspace allowlists.
- Recover the current submission for Stop, including a quiet turn with no replayed events. Read errors and multiple possible owners fail closed.

## Out of scope

New survival runtimes, new flags, automatic context continuation, and unrelated CI fixes.

## Acceptance

1. Partial-ACK adoption uses the original stream and projected cursor, including generations greater than one.
2. Compatible adopted clients advertise v1 before any submission decision. Missing or broken storage cannot become legacy mode.
3. Identity mismatch or unavailable evidence blocks admission without restarting the harness or clearing durable uncertainty.

## Test cases

Add `durable_adoption_test.go`:
- `TestDurableAdoptionRestoresProjectedCursor`: acknowledged prefix, retained suffix, generation two, and no provider restart.
- `TestDurableAdoptionCapabilityDiscovery`: compatible, genuine legacy, unsupported version, 401, timeout, corrupt/locked/full journal.
- `TestDurableAdoptionRejectsOwnerMismatch`: wrong session/incarnation/generation, missing SQL owner, and stale descriptor.
- `TestDurableAdoptionRestoresQuietSubmission`: no new events, Stop targets the exact prior submission.
Use a real journal and task repository for the cursor/identity round trip. Mock only the harness boundary.

## Verification

Run from the repository root. Proposed test files must exist before these commands pass.

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agentctl/server/api -run 'TestDurableAdoption|Test.*Delivery' -count=1)
(cd apps/backend && go test ./internal/orchestrator -run 'TestPrepareAgentDeliverySubmission' -count=1)
(cd apps/backend && golangci-lint run ./... --new-from-rev=origin/main --timeout=5m)
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/executor_standalone.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_lifecycle.go`
- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_activation.go`
- `apps/backend/internal/agent/runtime/lifecycle/durable_adoption_test.go`
- `apps/backend/internal/agent/runtime/agentctl/client_delivery.go`
- `apps/backend/internal/agent/runtime/agentctl/client_delivery_test.go`
- `apps/backend/internal/agentctl/server/api/durable_delivery.go`
- `apps/backend/internal/orchestrator/agent_delivery_submission.go`

## Dependencies

Task 01 must pass its acceptance before this work begins.

## Inputs

- [Plan and baseline](plan.md).
- [Adoption contract](../../specs/platform/system-design/durable-agent-delivery.md#surviving-process-adoption).
- Read the scoped AGENTS.md files and the tests next to every changed implementation.

## Risks

A fresh HTTP client has no capability cache. Unknown cache state must not be interpreted as legacy support.
A surviving process can emit events during status discovery. Capture identity consistently and validate events again on receipt.

## Parallelism

`sequential`

## Results

Completed 2026-09-14. Added authenticated delivery status discovery, bounded
recovery descriptors, capability caching, SQL identity and projected-cursor
validation, quiet-submission reconstruction, and fail-closed adoption. A
deterministic initial prompt submission ID bridges the lifecycle bootstrap
before its interactive SQL row exists; arbitrary missing SQL evidence remains
blocked.

`TestDurableAdoption*`, `TestDelivery*`, the lifecycle race suite, the exact
Task 02 backend tests, and the changed-package Go lint passed. No ACP
initialize, load, resume, new, or prompt call is used for adoption.
