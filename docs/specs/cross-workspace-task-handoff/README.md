---
status: active
system: cross-workspace-task-handoff
specification_version: 1
migration: complete
owners:
  - nova28
---

# Cross-workspace task handoff

## Purpose

An authorized Office agent hands a piece of work to a **different** workspace
by creating a delivery task there — a kanban task, startable from that
workspace's board, carrying provenance back to the task that created it. This
system owns that one action: the CLI surface that exposes it, the runtime
route it calls, the authorization gates on both, the resolution of the
target-workspace resources it names, the provenance it records in both
workspaces, and its idempotent-create and start-reporting contracts.

## Ownership

This system owns:

- The `kandev task handoff` CLI subcommand and its request shape.
- The authenticated `POST /api/v1/office/runtime/handoffs` route and its
  authorization, validation, and resolution order.
- The `can_handoff_tasks` permission and `handoff_task` runtime capability.
- The `handoff_source` (delivery task) and `handoffs` (source task) provenance
  records and their compare-and-set write discipline.
- The idempotent-create and start-reporting response contract for this one
  action.

## Exclusions

- The external-id idempotency mechanism this action reuses belongs to the
  [task system](../tasks) — see
  [External task ID idempotency](../tasks/requirements/external-id-idempotency.md).
  This system does not restate, reinterpret, or extend it.
- Office agent identity, roles, and the general permission surface belong to
  the [Office system](../office); this system adds one permission key to that
  existing surface.
- Same-workspace task creation (`create_task_kandev`, `POST /runtime/tasks`)
  is unchanged and out of scope.
- An MCP tool for this capability is permanently excluded — see
  [Command and route surface](requirements/command-and-route-surface.md).
- A UI surface for the handoff, a settings toggle for the runtime capability,
  and a deadline or reverse-direction link for an open handoff are all out of
  scope; see the exclusions recorded in each requirement document.

## Specification map

### Requirements

- [Command and route surface](requirements/command-and-route-surface.md)
- [Authorization and target-workspace resolution](requirements/authorization-and-capability.md)
- [Agent and executor profile resolution](requirements/profile-resolution.md)
- [Provenance recording](requirements/provenance-recording.md)
- [Same-workspace refusal](requirements/same-workspace-refusal.md)
- [Idempotent creation and settlement](requirements/idempotent-creation.md)
- [Reverse-link integrity](requirements/reverse-link-integrity.md)
- [Start semantics and the response contract](requirements/start-semantics.md)

### System design

- [Handoff mechanism](system-design/handoff-mechanism.md)
- [Failure modes and verification](system-design/failure-modes-and-verification.md)

## History

Revisions 1-4 specified `handoff_task_kandev` as an MCP tool. Revision 5
withdrew that mechanism in favor of a CLI subcommand (`kandev task handoff`)
over a new authenticated Office runtime route
(`POST /api/v1/office/runtime/handoffs`), because the Office prompt already
directs all Office state changes through `$KANDEV_CLI` and forbids the
discovery of further Kandev MCP tools — a second, tool-shaped path to the same
effect contradicted that contract. The MCP withdrawal is complete, not a
deprecation; see
[Command and route surface](requirements/command-and-route-surface.md).
This directory (split from a single legacy `spec.md` under revision 6) is the
current, authoritative form of the specification.
