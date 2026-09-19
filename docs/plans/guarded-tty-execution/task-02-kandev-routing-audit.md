---
id: "02-kandev-routing-audit"
title: "Add Kandev guarded TTY routing and audit"
status: complete
wave: 2
depends_on:
  - "01-codex-acp-bridge"
plan: "plan.md"
requirements:
  - REQ-AGENTS-GUARDED-TTY-EXECUTION-001
  - REQ-AGENTS-GUARDED-TTY-EXECUTION-002
acceptance_criteria:
  - AC-AGENTS-GUARDED-TTY-EXECUTION-001.1
  - AC-AGENTS-GUARDED-TTY-EXECUTION-001.2
  - AC-AGENTS-GUARDED-TTY-EXECUTION-001.3
  - AC-AGENTS-GUARDED-TTY-EXECUTION-002.1
  - AC-AGENTS-GUARDED-TTY-EXECUTION-002.3
  - AC-AGENTS-GUARDED-TTY-EXECUTION-002.4
  - AC-AGENTS-GUARDED-TTY-EXECUTION-002.5
system_design:
  - ../../specs/agents/system-design/guarded-tty-execution.md
---

# Task 02: Add Kandev guarded TTY routing and audit

## Summary

Expose the explicit tool only for an allowed, positively probed Codex ACP task
execution. Bind every call to trusted lifecycle identity, dispatch through the
exact agentctl stream, and persist one claim/finalization audit.

## In scope

- MCP capability, strict schema, backend action, and tool handler.
- Runtime and agentctl request/response contracts.
- Adapter capability probe and extension invocation.
- Task-message audit claim/finalization and denial tests.

## Out of scope

- Codex App Server bridge implementation.
- Managed runtime version pin and real-agent acceptance run.

## Acceptance

- Unsupported and stale executions never advertise or dispatch the tool.
- Allowed requests produce one validated receipt linked to one durable audit.
- Forged identities, malformed receipts, and restricted paths fail closed.

## Verification

```bash
cd apps/backend && go test ./internal/mcp/server ./internal/mcp/handlers ./internal/agent/runtime/agentctl ./internal/agentctl/server/adapter/transport/acp ./internal/orchestrator ./internal/task/service ./internal/task/repository/sqlite ./internal/integration
cd apps/backend && go test -race ./internal/agent/runtime/agentctl ./internal/agentctl/server/adapter/transport/acp
```

## Files likely touched

- `apps/backend/internal/mcp/profile/profile.go`
- `apps/backend/internal/mcp/server/guarded_tty_exec.go`
- `apps/backend/internal/mcp/handlers/guarded_tty_exec.go`
- `apps/backend/internal/agent/runtime/agentctl/agent.go`
- `apps/backend/internal/agentctl/server/api/agent.go`
- `apps/backend/internal/agentctl/server/adapter/adapter.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/guarded_tty_exec.go`
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/sqlite/message.go`
- `apps/backend/internal/task/service/service_messages.go`

## Dependencies

- Task 01 extension method and receipt schema.

## Risks

- Audit message visibility must not create an unintended UI contract.
- Nested requests require deterministic concurrency tests without sleeps.

## Parallelism

`sequential`

## Inputs

- Task 01 method names, schemas, version, and tests.
- Existing MCP identity, task scope, permission audit, and agentctl stream
  patterns.

## Results

Implemented the dual-gated MCP catalog, strict argv-only schema, trusted MCP
principal validation, execution-bound lifecycle and agentctl routing, exact ACP
advertisement/probe/receipt adapter, and durable audit claim/finalization. The
audit claim is written before dispatch and binds attestation, execution, task,
session, workspace, actor, fixed `codex-acp` agent identity, principal surface,
and argv. Finalization is compare-and-set, omits raw output, and records the
output digest plus provider dispatch evidence.

Focused package tests cover selection, live catalog changes, identity
forwarding, exact active-session routing, receipt validation, all stable denial
mappings, audit replay/mismatch rejection, sanitized errors, cancellation-safe
finalization, session replacement races, and concurrent ordinary prompts. The
affected race suite passes. Adversarial validation additionally requires the
receipt's exact contract and ACP-session identity, bridge version, process ID,
cwd, timestamps, and stream/output byte accounting at the Kandev boundary; the
tests first demonstrated that incomplete evidence was accepted, then passed
after the boundary was tightened. New-code backend lint reports zero issues.
Release pinning and credentialed real-agent evidence remain Task 03.
