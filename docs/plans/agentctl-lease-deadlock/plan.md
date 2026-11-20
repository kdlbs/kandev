---
created: 2026-10-20
status: done
requirements:
  - REQ-PLATFORM-AGENTCTL-CLIENT-LEASE-001
system_design:
  - ../../specs/platform/system-design/agentctl-client-lease.md
legacy_specs: []
---

# Implementation Plan: Agentctl client lease deadlock

## Overview

A recoverable agent failure starts a terminal stop that waits for the write
side of the execution's agentctl client lease. A queued prompt for the same
session can resume the execution through `RecoverAgentPromptStream` at the same
time. `restoreRecoveredFailedExecution` held a client read lease while marking
the execution ready, and that path persists the row, which reads
`AgentctlURL()` under the execution store lock. `AgentctlURL()` took the read
lease again. With the writer pending, the nested read blocks forever while the
store lock is held, and every store reader stalls, so the backend stops
launching and streaming.

## Scope

In scope: `apps/backend/internal/agent/runtime/lifecycle/types.go`
(`AgentctlURL`, plus the URL publication in client replacement and detach),
`manager_interaction.go` (`restoreRecoveredFailedExecution`), and a permanent
unit test.

Excluded: other lease users, the terminal stop path, and any change to the
lease semantics of client replacement or detach.

## Technical approach

- `AgentctlURL` never takes the client lease. The base URL is immutable per
  client, so it reads the atomic override first, then the construction-time
  client under a try-locked source read.
- A replacement or detach publishes its URL with an atomic before taking the
  source write side, and a read that cannot take the read side returns that
  value. Detach retains the installed URL until it clears the client under
  the source lock, then publishes empty. Persistence therefore never blanks
  `executors_running.agentctl_url` for a live execution.
- `restoreRecoveredFailedExecution` releases the lease right after
  `GetStatus`, before marking the execution ready.

## Work orders

- [x] [Task 01: Keep agentctl URL reads off the client lease](task-01-agentctl-url-lease.md)

## Changed-path coverage

The work order owns the lifecycle files above, the new
`agentctl_url_lease_test.go`, `agentctl_url_persistence_test.go`,
`agentctl_recovery_lease_test.go`, and the new agentctl client lease requirement
and system design.
