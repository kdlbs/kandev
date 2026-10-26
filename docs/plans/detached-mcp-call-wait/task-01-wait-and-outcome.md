---
id: "01-wait-and-outcome"
title: "Wait for attach and report unknown outcomes"
status: todo
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DETACHED-MCP-WAIT-001
  - REQ-PLATFORM-DETACHED-MCP-WAIT-003
acceptance_criteria:
  - AC-PLATFORM-DETACHED-MCP-WAIT-001.1
  - AC-PLATFORM-DETACHED-MCP-WAIT-001.2
  - AC-PLATFORM-DETACHED-MCP-WAIT-001.3
  - AC-PLATFORM-DETACHED-MCP-WAIT-001.4
  - AC-PLATFORM-DETACHED-MCP-WAIT-001.5
  - AC-PLATFORM-DETACHED-MCP-WAIT-001.6
  - AC-PLATFORM-DETACHED-MCP-WAIT-001.7
  - AC-PLATFORM-DETACHED-MCP-WAIT-003.1
  - AC-PLATFORM-DETACHED-MCP-WAIT-003.2
  - AC-PLATFORM-DETACHED-MCP-WAIT-003.3
  - AC-PLATFORM-DETACHED-MCP-WAIT-003.4
  - AC-PLATFORM-DETACHED-MCP-WAIT-003.5
  - AC-PLATFORM-DETACHED-MCP-WAIT-003.6
system_design:
  - ../../specs/platform/system-design/detached-mcp-call-wait.md
---

# Task 01: Wait for attach and report unknown outcomes

## Summary

Replace the fixed 5-second send timeout with a send loop driven by the
attach snapshot, and bind each request to its stream before writing it.

## In scope

- `ChannelBackendClient.RequestPayload` loop, `SetAttachmentSnapshotter`
  closure wired in `cmd/agentctl/main.go`, `ErrKandevCallOutcomeUnknown`,
  `FailRequestNotSent`, bind-before-write in `writeAgentStreamMCPRequest`.
- Tasks 01 and 02 ship in one PR: the snapshot's `Exhausted` channel and the
  budget error (004.6) are produced by Task 02's tracker. Task 01 builds the
  loop against the snapshot interface and a test double.
- Epoch and writer-ready handshake (`MarkWriterReady`), the no-wait option for
  internal best-effort notices (001.7), and withdraw/bind atomicity under
  `pendingMu`.

## Out of scope

Budget expiry, keepalive, and configuration.

## Verification

- `go test -trimpath ./internal/mcp/server/ ./internal/agentctl/server/api/ -run 'Attach|Outcome|MCPStream' -count=1`
