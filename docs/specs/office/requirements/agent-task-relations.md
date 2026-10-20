---
status: active
system: office
created: 2026-10-17
owners:
  - kandev
---

# Office Agent Task Relations Requirements

## Overview

Office agents create tasks through the signed Office runtime task-create
endpoint, reached from `$KANDEV_CLI kandev task create`. Today that endpoint
accepts a title, description, parent, assignee, and project, and refuses every
other field. An agent therefore cannot order the work it creates, cannot make
one task wait for several others (fan-in), and cannot give a child task its own
worktree.

This capability lets an agent declare, at creation time, which tasks the new
task waits for and whether the new task gets its own workspace. It adds fields
without changing what an omitted field does today.
Office owns it because the contract is the Office runtime action surface and
its scope model; the dependency store and workspace-policy machinery it reuses
belong to the task system.

## Terminology

- **Runtime task create:** `POST /api/v1/office/runtime/tasks`, authenticated
  by the run's signed token. The token, not the request, supplies the
  workspace, the calling agent, and the run's task.
- **Run task:** the task the calling run belongs to.
- **Blocker:** a task named in `blocked_by`. The created task depends on it.
- **Read relation:** the shared Office rule that decides which tasks a run task
  may read: itself, its ancestors, its descendants, its siblings under a
  non-empty parent, and its blockers, always within one workspace.
- **Task scope:** the tasks a run may mutate: its run task plus the task IDs (or
  wildcard) carried in its token.

## Requirements

### REQ-OFFICE-AGENT-TASK-RELATIONS-001: Blockers on agent-created tasks

**Intent:** Let an agent order the tasks it creates and build a join task that
waits for several others, without a human adding edges and without an agent
gaining reach over tasks it cannot already see.

**User story:** As an Office coordinator agent, I want to create a task that is
blocked by the tasks I just fanned out, so that it runs once they are all done.

#### Acceptance criteria

- **AC-OFFICE-AGENT-TASK-RELATIONS-001.1:** When a run with the required create
  capability sends runtime task create with `blocked_by` naming permitted tasks,
  the system shall create the task with exactly one dependency edge to each
  named task.
- **AC-OFFICE-AGENT-TASK-RELATIONS-001.2:** The system shall permit a blocker
  only when it is in the run's workspace and is either within the run's task
  scope or satisfies the read relation from the run task.
- **AC-OFFICE-AGENT-TASK-RELATIONS-001.3:** When any blocker is not permitted,
  including a task that does not exist or belongs to another workspace, the
  system shall create no task, return a forbidden status with one message that
  does not distinguish those cases, and record a `runtime.denied` event.
- **AC-OFFICE-AGENT-TASK-RELATIONS-001.4:** When `blocked_by` contains an empty
  ID, an ID longer than 128 characters, or more than 512 IDs, the system shall
  create no task and return a bad-request status naming the offending field.
- **AC-OFFICE-AGENT-TASK-RELATIONS-001.5:** When `blocked_by` names the same
  task more than once, the system shall record a single edge to that task.
- **AC-OFFICE-AGENT-TASK-RELATIONS-001.6:** When a blocker is deleted between
  validation and edge creation, the system shall leave no created task behind
  and shall not report success.
- **AC-OFFICE-AGENT-TASK-RELATIONS-001.7:** Edge creation shall pass through the
  task system's single dependency validator, so self-edges, cross-workspace
  edges, and cycles are rejected by the same rule every other surface uses.
- **AC-OFFICE-AGENT-TASK-RELATIONS-001.8:** When `blocked_by` names the new
  task's parent or another ancestor, and that task is otherwise permitted, the
  system shall create the edge.

### REQ-OFFICE-AGENT-TASK-RELATIONS-003: Workspace mode for agent-created tasks

**Intent:** Let an agent give a child task its own worktree and branch, without
changing where a child goes when the agent does not ask.

**User story:** As an Office planner agent, I want each parallel batch child to
work in its own worktree, so that their branches do not collide and I can merge
them when the children complete.

#### Acceptance criteria

- **AC-OFFICE-AGENT-TASK-RELATIONS-003.1:** When runtime task create for a
  child sets `workspace_mode` to `new_workspace`, the system shall record that
  mode and the child shall launch in its own task environment rather than its
  parent's.
- **AC-OFFICE-AGENT-TASK-RELATIONS-003.2:** When runtime task create for a
  child sets `workspace_mode` to `inherit_parent`, the system shall bind the
  child to its parent's workspace.
- **AC-OFFICE-AGENT-TASK-RELATIONS-003.5:** When a child is created through the
  Office agent path without `workspace_mode`, the system shall bind it to the
  parent's workspace, as it does today.
- **AC-OFFICE-AGENT-TASK-RELATIONS-003.6:** When `workspace_mode` is set on a
  task with no parent, the system shall accept `new_workspace` and reject
  `inherit_parent` with a bad-request status.
- **AC-OFFICE-AGENT-TASK-RELATIONS-003.7:** When `workspace_mode` has any other
  value, including `shared_group`, the system shall create no task and return a
  bad-request status naming the field and the accepted values.

### REQ-OFFICE-AGENT-TASK-RELATIONS-004: CLI support for relation fields

**Intent:** The CLI sends the two relation fields the runtime now honors. Fields
the runtime still refuses keep their existing pre-send refusal; whether to
support or remove them is tracked in
[#4613](https://github.com/kdlbs/kandev/issues/4613).

#### Acceptance criteria

- **AC-OFFICE-AGENT-TASK-RELATIONS-004.1:** `kandev task create` shall offer
  `--blocked-by` (comma-separated task IDs) and `--workspace-mode`, and shall
  send them to runtime task create.
- **AC-OFFICE-AGENT-TASK-RELATIONS-004.2:** When `kandev task create` receives
  `--priority`, `--workspace-group-id`, `--default-child-workspace`, or
  `--default-child-ordering`, it shall fail with `--<flag> is not supported by
  Office runtime task create` before any request is sent, as it does today.
- **AC-OFFICE-AGENT-TASK-RELATIONS-004.3:** When runtime task create receives
  `priority`, `workspace_group_id`, `default_child_workspace`, or
  `default_child_ordering`, the system shall create no task and return a
  bad-request status naming the field.
- **AC-OFFICE-AGENT-TASK-RELATIONS-004.4:** When `--blocked-by` contains an
  empty entry after splitting on commas and trimming, the CLI shall fail before
  sending a request.
- **AC-OFFICE-AGENT-TASK-RELATIONS-004.5:** The Office skills and context that
  document `task create` shall describe the supported fields and the blocker
  scope rule, and shall not document the unsupported flags as usable.

## Out of scope

- When a task created with unfinished blockers first wakes.
  `REQ-OFFICE-SCHEDULER-002` (assignment wake admission) owns that, for every
  creation surface. This document covers only creating the edges.
  `REQ-OFFICE-AGENT-TASK-RELATIONS-002` is intentionally unassigned and is not
  reused.
- `priority`, `workspace_group_id` / `shared_group`, `default_child_workspace`,
  and `default_child_ordering` on runtime task create, and whether the CLI keeps
  defining their flags (tracked in
  [#4613](https://github.com/kdlbs/kandev/issues/4613)).
- Changing how an omitted `workspace_mode` resolves on any child-creation path.
  The HTTP task API applies a parent's `default_child_workspace`, while MCP
  `create_task_kandev` and `CreateChildTask` (the Office agent path and workflow
  `create_child_task`) always use `inherit_parent`. Reconciling that is
  tracked in [#4606](https://github.com/kdlbs/kandev/issues/4606). `AC-OFFICE-AGENT-TASK-RELATIONS-003.3` and `003.4` are
  intentionally unassigned and are not reused.
- Adding blockers to, or removing them from, an existing task through the
  Office runtime. Edges are only declared on the task being created, so an agent
  can hold back only work it creates.
- Detecting a wait that combines a blocker edge with the parent/child
  hierarchy, such as a parent whose agent waits for its children while a child
  is blocked by that parent.
- Office web UI changes.
