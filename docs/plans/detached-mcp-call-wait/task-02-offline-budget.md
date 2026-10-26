---
id: "02-offline-budget"
title: "Offline budget tracker and configuration"
status: todo
wave: 2
depends_on:
  - "01-wait-and-outcome"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DETACHED-MCP-WAIT-004
acceptance_criteria:
  - AC-PLATFORM-DETACHED-MCP-WAIT-004.1
  - AC-PLATFORM-DETACHED-MCP-WAIT-004.2
  - AC-PLATFORM-DETACHED-MCP-WAIT-004.3
  - AC-PLATFORM-DETACHED-MCP-WAIT-004.4
  - AC-PLATFORM-DETACHED-MCP-WAIT-004.5
  - AC-PLATFORM-DETACHED-MCP-WAIT-004.6
  - AC-PLATFORM-DETACHED-MCP-WAIT-004.7
  - AC-PLATFORM-DETACHED-MCP-WAIT-004.8
  - AC-PLATFORM-DETACHED-MCP-WAIT-004.9
  - AC-PLATFORM-DETACHED-MCP-WAIT-004.10
  - AC-PLATFORM-DETACHED-MCP-WAIT-004.11
  - AC-PLATFORM-DETACHED-MCP-WAIT-004.12
system_design:
  - ../../specs/platform/system-design/detached-mcp-call-wait.md
---

# Task 02: Offline budget tracker and configuration

## Summary

Add the budget tracker beside `attachment.go`, cancel the running turn on
expiry, fail waiting calls, and deliver `offline_budget_minutes` from the
executor profile to agentctl.

## In scope

- Tracker with stability timer and the expiry decision under one mutex.
- Decision recorded first under the tracker mutex, then a conditional
  `adapter.Cancel` (the `handleWSCancel` call, not `cancelDeliveryPressureTurn`)
  plus `CancelPendingPermissions`; terminal stamped at the journal commit as
  `cancelled` with `cancel_cause: offline_budget`. Cancel failure logs
  `budget_cancel_failed` (004.11).
- Creation-time reaper-versus-budget warning (004.12); reapers unchanged (004.8).
- Ships in one PR with Task 01.
- Profile key, strict parser (`ErrInvalidOfflineBudget`), launch metadata,
  an `offline_budget_minutes` field in the `CreateInstanceRequest` JSON body (no start flag; an older agentctl ignores it and keeps the 5-second failure).

## Out of scope

Chat notices (Task 04) and UI for configuring the value beyond the existing
profile config form.

## Verification

- `go test -trimpath ./internal/agentctl/server/process/ ./internal/orchestrator/executor/ -run 'OfflineBudget|Attachment' -count=1`
