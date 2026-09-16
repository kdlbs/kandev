---
status: done
requirements:
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-SAME-WORKSPACE-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-IDEMPOTENCY-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-REVERSE-LINK-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-START-001
system_design:
  - ../../specs/cross-workspace-task-handoff/system-design/handoff-mechanism.md
  - ../../specs/cross-workspace-task-handoff/system-design/failure-modes-and-verification.md
legacy_specs: []
---

# Implementation Plan: Cross-workspace task handoff

## Status

Delivered against revision 6 of the cross-workspace task handoff spec. An
earlier iteration of this plan targeted a withdrawn Office-only MCP tool,
`handoff_task_kandev` (spec revisions 1-4); revision 5 replaced it with
`kandev task handoff`, a subcommand of the existing `task` group, and
revision 6 completed the spec's split into
`docs/specs/cross-workspace-task-handoff/README.md` plus its `requirements/`
and `system-design/` subdirectories (the single-file `spec.md` this section
originally pointed at was retired by that split). [Task 01](task-01-cross-workspace-task-handoff.md)
records the CLI/route mechanism actually delivered; no MCP tool, profile
capability, or tool-group entry backs this capability on any surface.

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

## Withdrawn mechanism (superseded before merge)

An earlier iteration of this branch implemented revisions 1-4:
`internal/mcp/server/handoff_task_tool.go`, the `handleHandoffTask` handler in
`internal/mcp/handlers/`, the `mcpprofile.CapabilityHandoffTask` gate, the
`office-handoff` entry in `profileToolGroups`, and the
`officeHandoffToolInstruction` prompt line in
`internal/sysprompt/sysprompt.go`. AC-1a made this a deletion rather than a
deprecation, so that surface was removed rather than left alongside the CLI;
no MCP tool, profile capability, or tool-group entry for this capability
remains on any surface.

The provenance records, reverse-link compare-and-set
(`task/repository/sqlite/task_handoffs_cas.go`), activity events, permission
resolution, and deferred-start profile retention are mechanism-independent and
carried forward unchanged from that iteration.

## Delivered mechanism

- `kandev task handoff` is a case in `runTaskCmd`'s switch alongside `get`,
  `update`, `create` and `decision`, named in that function's usage line,
  obtaining credentials through `newKandevClient()` and issuing through
  `client.do`. No new transport, client or credential path (AC-1).
- `POST /api/v1/office/runtime/handoffs` is mounted in `runtime.RegisterRoutes`
  alongside `POST /runtime/tasks`, deriving every identity field from the signed
  run token and honouring no caller identity, workspace, agent, role or
  capability field from the request (AC-2).
- The runtime capability `CapabilityHandoffTask = "handoff_task"` is defined in
  `internal/office/runtime/capabilities.go`, with the `CanHandoffTasks` field on
  `runtime.Capabilities`, a case in `Capabilities.Allows`, and an `AllowedKeys()`
  entry at the position matching the struct field order (AC-8).
- The `mcpprofile.Capability` value is removed so no MCP surface can gate,
  advertise or reach the feature (AC-8a).
- `can_handoff_tasks` remains the operator-visible grant (AC-7), now feeding the
  runtime capability through `FromAgent` rather than an MCP profile capability.

## Work orders

- [Task 01: Cross-workspace task handoff](task-01-cross-workspace-task-handoff.md)
  — delivers the CLI/route mechanism described above.

## Delivery notes

The source specification is `docs/specs/cross-workspace-task-handoff/README.md`
plus its `requirements/` and `system-design/` subdirectories. No public UI or
public documentation surface is part of this delivery.
