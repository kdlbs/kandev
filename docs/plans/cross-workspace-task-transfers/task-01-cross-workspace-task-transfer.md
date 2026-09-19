---
id: "01-cross-workspace-task-transfer"
title: "Cross-workspace task transfer"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-001
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-002
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-003
acceptance_criteria:
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.1
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.2
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.3
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.4
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.5
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-002.1
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-002.2
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-002.3
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.1
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.2
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.3
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.4
  - AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.5
system_design:
  - ../../specs/tasks/system-design/cross-workspace-task-transfer.md
---

# Task 01: Cross-workspace task transfer

## Summary

Implement the audited cross-workspace task transfer end to end: the additive
task-store ledger, the atomic repository transaction with placement, lane
equivalence, boundary, relation, and capacity validation, server-attested
service authorization, the `transfer_task_kandev` MCP tool, idempotent replay,
redacted attempt auditing, and post-commit broadcast/cache/WIP reconciliation
that survives caller cancellation.

## In scope

- `apps/backend/internal/task/repository/sqlite/task_transfer*.go`
- `apps/backend/internal/task/service/service_transfer.go`
- `apps/backend/internal/mcp/handlers/task_transfer_handler.go`
- `apps/backend/internal/mcp/server/` transfer tool registration and counts
- `apps/backend/internal/backendapp/task_transfer_coordinator.go`
- `apps/backend/internal/gateway/websocket/task_notifications.go`
- `apps/backend/pkg/websocket/actions.go`
- `docs/public/automation-and-mcp.md`

## Out of scope

Task recreation, cross-owner movement, workspace-owned relation mapping, any
frontend surface, and same-workspace move behavior.

## Acceptance

1. A bound request from an authorized human or server-attested coordinator
   transfers one exact task UUID into an equivalent same-owner lane atomically
   and returns the receipt; every stale, mismatched, unauthorized, or
   idempotency-conflicting retry fails closed with the stable conflict result
   and no existence leak.
2. Storage identity, sessions, running turns, plans, relations, repositories,
   worktrees, PR links, pending moves, status summaries, and terminal timing
   survive the transfer; destination projections are rebound in the same
   transaction.
3. An exact retry returns the stored receipt, a changed retry conflicts, every
   attempt is audited without transcript content, and post-commit
   reconciliation runs exactly once even under caller cancellation.

## Verification

From `apps/backend`:

    go test ./internal/task/repository/sqlite ./internal/task/service ./internal/mcp/handlers ./internal/mcp/server ./internal/backendapp -run 'TransferTask|TaskTransfer|TestServerMode' -count=1
    go test -race ./internal/task/repository/sqlite ./internal/task/service ./internal/mcp/handlers -run 'TransferTask|TaskTransfer' -count=1
    go run ./cmd/sqlguard ./internal
    go build ./...

Likely files: the repository transfer set, the service transfer slice, the
MCP handler and server registration, the backendapp coordinator, the gateway
notification wiring, and the public automation document.

## Results

Implemented and verified per the plan's QA receipt: focused suites, race
builds, SQL guard, schema conformance, full backend build, and public-docs
validation pass at the reviewed head.
