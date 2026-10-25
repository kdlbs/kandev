---
created: 2026-10-25
status: draft
requirements:
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-001
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-002
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-003
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-004
system_design:
  - ../../specs/executors/system-design/session-control-plane-isolation.md
legacy_specs: []
---

# Plan: session control-plane isolation

## Overview

Three independent results: a per-instance MCP token, a broker relay on the
instance listener, and scope plus interlock on executor mutations. REQ-004
records the bind decision and needs no code.

## Work orders

- [ ] [Task 01: Session MCP token](task-01-session-mcp-token.md) (wave 1)
- [ ] [Task 02: Broker relay](task-02-broker-relay.md) (wave 2, after 01)
- [ ] [Task 03: Executor route guards](task-03-executor-route-guards.md) (wave 1)

## Verification

`(cd apps/backend && go test ./internal/agentctl/... ./internal/agent/runtime/lifecycle/... ./internal/orchestrator/executor/... ./internal/task/handlers/... -count=1)`
`bash scripts/lint-go-changed <changed-go-files>`

## Risks

- Every supported agent must carry a header; Task 01 inventories them first.
- Removing WebSocket mutations requires moving three web call sites to HTTP (task 03).
- Lost derivation key replaces retained instances.
- The first required-capability addition interrupts every retained local control server once on upgrade.
- Remote executors' recovery paths each need the shared verifier at the two consuming points (task 01).
