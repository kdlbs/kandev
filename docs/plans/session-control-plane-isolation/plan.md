---
created: 2026-10-25
status: draft
requirements:
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-001
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-002
  - REQ-EXECUTORS-SESSION-CONTROL-ISOLATION-003
system_design:
  - ../../specs/executors/system-design/session-control-plane-isolation.md
legacy_specs: []
---

# Plan: session control-plane isolation

## Overview

Two independent results: a per-instance MCP token and a broker relay on the
instance listener. REQ-003 records the bind decision; its follow-up card is filed by
Task 01. Executor route guards and workspace-file carriers are deferred (follow-up
task `ff57f168-9eff-4d0f-9e75-ddaaca481c2a`).

## Work orders

- [ ] [Task 01: Session MCP token](task-01-session-mcp-token.md) (wave 1)
- [ ] [Task 02: Broker relay](task-02-broker-relay.md) (wave 2, after 01)

## Verification

`(cd apps/backend && go test ./internal/agentctl/... ./internal/agent/runtime/lifecycle/... ./internal/orchestrator/executor/... ./internal/task/handlers/... -count=1)`
`bash scripts/lint-go-changed <changed-go-files>`

## Risks

- Every supported agent must carry a header; Task 01 inventories them first. Cursor and Pi fail closed until the deferred follow-up lands.
- Lost derivation key replaces retained instances.
- The first required-capability addition interrupts every retained local control server once on upgrade.
- Each adopting path (local, remote recovery, Kubernetes reattach, SSH resume) needs the shared verifier (task 01).
