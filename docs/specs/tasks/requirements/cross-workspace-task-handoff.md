---
status: active
system: tasks
created: 2026-09-13
owners:
  - kandev
---

# Cross-workspace task handoff requirements

## Overview

An authorized Office agent can create a delivery task in another workspace.
The task system must preserve the caller's authority, the selected profiles,
and a durable link between the source and delivery tasks.

## Terms

- **Source task:** The task and session that request the handoff.
- **Delivery task:** The new task created in the target workspace.
- **Handoff record:** The provenance data stored on the delivery task and the
  reverse-link entry stored on the source task.
- **Deferred start:** A handoff that creates a task without starting its agent.

## Requirements

### REQ-TASKS-CROSS-WORKSPACE-HANDOFF-001: Authorized handoff creates an auditable delivery task

**Intent:** Allow the Office workflow to create work across workspace
boundaries while keeping authorization, provenance, and retries explicit.

#### Acceptance criteria

- **AC-TASKS-CROSS-WORKSPACE-HANDOFF-001.1:** When an Office session has the
  handoff permission, the system shall expose `handoff_task_kandev` and shall
  derive the caller identity from the trusted session context.
- **AC-TASKS-CROSS-WORKSPACE-HANDOFF-001.2:** The system shall reject a target
  workspace, workflow, repository, agent profile, or executor profile that is
  absent, inaccessible, inconsistent, or outside the authorized scope before
  it creates or starts the delivery task.
- **AC-TASKS-CROSS-WORKSPACE-HANDOFF-001.3:** A successful handoff shall store
  source task, workspace, session, agent profile, executor profile, and
  handoff time on the delivery task, and shall append a matching reverse link
  to the source task.
- **AC-TASKS-CROSS-WORKSPACE-HANDOFF-001.4:** A retry with the same external
  identifier shall return the existing outcome without creating a second
  delivery task, and a reverse-link repair shall preserve the stored handoff
  time.

### REQ-TASKS-CROSS-WORKSPACE-HANDOFF-002: Deferred handoff keeps its selected profile

**Intent:** Ensure that a handoff which starts later uses the profiles selected
by the original authorized call.

#### Acceptance criteria

- **AC-TASKS-CROSS-WORKSPACE-HANDOFF-002.1:** When a handoff supplies an agent
  profile and sets `start_agent` to false, the later task start shall retain
  that profile even when workflow step defaults differ.
- **AC-TASKS-CROSS-WORKSPACE-HANDOFF-002.2:** A failed deferred start shall
  keep the explicit profile marker for a later retry, and a successful start
  shall clear it after the agent launch is accepted.

## Exclusions

- Same-workspace task creation and its existing MCP or HTTP contracts.
- A new UI surface for creating or reviewing handoffs.
- Changing Office role policy, workflow selection, or workspace ownership rules.

## Related documents

- [Cross-workspace handoff source specification](../../cross-workspace-task-handoff/spec.md)
- [System design](../system-design/cross-workspace-task-handoff.md)
- [Implementation plan](../../../plans/cross-workspace-task-handoff/plan.md)
