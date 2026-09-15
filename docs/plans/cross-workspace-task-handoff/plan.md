---
status: superseded
requirements:
  - REQ-TASKS-CROSS-WORKSPACE-HANDOFF-001
  - REQ-TASKS-CROSS-WORKSPACE-HANDOFF-002
system_design:
  - ../../specs/cross-workspace-task-handoff/system-design/handoff-mechanism.md
  - ../../specs/cross-workspace-task-handoff/system-design/failure-modes-and-verification.md
legacy_specs: []
---

# Implementation Plan: Cross-workspace task handoff

## Status

Superseded by revision 5 of the cross-workspace task handoff spec, which
withdrew the mechanism this plan delivered. The requirements and the system
design are unchanged; only the transport and the gating vocabulary moved. The
spec now lives as `docs/specs/cross-workspace-task-handoff/README.md` plus its
`requirements/` and `system-design/` subdirectories (the single-file
`spec.md` this section originally pointed at was retired by the revision-6
spec split). No work order has been written for the replacement yet, so this
plan describes the gap rather than a schedule.

## Overview

The capability is unchanged: an Office agent that reaches a GO decision creates a
delivery task in a different workspace, with two-way provenance, idempotent
outcomes, and a retained agent profile when the handoff starts later.

What changed is where it lives. Revisions 1-4 specified an Office-only MCP tool,
`handoff_task_kandev`. Revision 5 replaces it with `kandev task handoff`, a
subcommand of the existing `task` group in
`apps/backend/cmd/agentctl/kandev_task.go`, because `office-context.md` already
tells Office agents that Office state changes go through `$KANDEV_CLI` and that
no further Kandev MCP tools exist to be found. The withdrawn tool was the first
Office-mutation MCP tool in a prompt saying such tools do not exist.

No authorization is lost by the move. `contextFromRequest` re-derives the
capability set from the signed run token on every request, giving the route the
same never-trust-the-payload property the MCP principal provided.

## Delivered against the withdrawn mechanism

The tree currently implements revisions 1-4: `internal/mcp/server/handoff_task_tool.go`,
the `handleHandoffTask` handler in `internal/mcp/handlers/`, the
`mcpprofile.CapabilityHandoffTask` gate, the `office-handoff` entry in
`profileToolGroups`, and the `officeHandoffToolInstruction` prompt line in
`internal/sysprompt/sysprompt.go`. AC-1a makes this a deletion rather than a
deprecation, so that surface is removed rather than left alongside the CLI.

The provenance records, reverse-link compare-and-set
(`task/repository/sqlite/task_handoffs_cas.go`), activity events, permission
resolution, and deferred-start profile retention are mechanism-independent and
carry forward unchanged.

## Gap to the specified mechanism

- Add `kandev task handoff` to `runTaskCmd`'s switch alongside `get`, `update`,
  `create` and `decision`, named in that function's usage line, obtaining
  credentials through `newKandevClient()` and issuing through `client.do`. No new
  transport, client or credential path (AC-1).
- Mount `POST /api/v1/office/runtime/handoffs` in `runtime.RegisterRoutes`
  alongside `POST /runtime/tasks`, deriving every identity field from the signed
  run token and honouring no caller identity, workspace, agent, role or
  capability field from the request (AC-2).
- Define the runtime capability `CapabilityHandoffTask = "handoff_task"` in
  `internal/office/runtime/capabilities.go`, with the `CanHandoffTasks` field on
  `runtime.Capabilities`, a case in `Capabilities.Allows`, and an `AllowedKeys()`
  entry at the position matching the struct field order (AC-8).
- Remove the `mcpprofile.Capability` value so no MCP surface can gate, advertise
  or reach the feature (AC-8a).
- Keep `can_handoff_tasks` as the operator-visible grant (AC-7), now feeding the
  runtime capability through `FromAgent` rather than an MCP profile capability.

## Work orders

- [Task 01: Cross-workspace task handoff](task-01-cross-workspace-task-handoff.md)
  — delivered the withdrawn MCP mechanism. Superseded; it is retained as the
  record of what shipped, not as work to schedule.

No work order exists for the CLI mechanism. Writing one is the next planning
step.

## Delivery notes

The source specification is `docs/specs/cross-workspace-task-handoff/README.md`
plus its `requirements/` and `system-design/` subdirectories. No public UI or
public documentation surface is part of this delivery.
