---
status: draft
system: office
created: 2026-05-02
owners:
  - cfl
---
# Office Tasks Requirements

## Overview

Office tasks are the unit of work in office mode: a single task carries a description, a workspace, an assignee, optional reviewers and approvers, blockers, parent/child relationships, a chat thread, and a per-agent session of working memory. Users and agents need to drive these tasks end-to-end - hand off context across a tree of subtasks, request review and approval, edit properties inline, see the agent's work in the chat, react to property changes without an agent having to poll, and preserve each participant's conversation across many short-lived agent runs.

This spec consolidates the office task surface: lifecycle, parent/child handoffs, approval flow, advanced execution mode, blocker cycle detection, the reactivity pipeline, per-(task, agent) session identity, inline editable properties, and the chat / activity views.

## Requirements

### REQ-OFFICE-TASKS-001: Office Tasks

**Intent:** Office tasks are the unit of work in office mode: a single task carries a description, a
workspace, an assignee, optional reviewers and approvers, blockers, parent/child relationships, a
chat thread, and a per-agent session of working memory. Users and agents need to drive these tasks
end-to-end - hand off context across a tree of subtasks, request review and approval, edit
properties inline, see the agent's work in the chat, react to property changes without an agent
having to poll, and preserve each participant's conversation across many short-lived agent runs.
This spec consolidates the office task surface: lifecycle, parent/child handoffs, approval flow,
advanced execution mode, blocker cycle detection, the reactivity pipeline, per-(task, agent) session
identity, inline editable properties, and the chat / activity views.

#### Acceptance criteria

- **AC-OFFICE-TASKS-001.1:** A task progresses through statuses `todo → in_progress → in_review → done`, with `blocked` and `cancelled` as branchable states. Status transitions are user- or agent-driven and feed the reactivity pipeline (section E).
- **AC-OFFICE-TASKS-001.2:** Every office task has zero or more **task sessions**: one per `(task_id, agent_instance_id)` pair. A session represents one agent's persistent conversation thread on the task, not a single launch.
- **AC-OFFICE-TASKS-001.3:** Sessions cycle through `CREATED → STARTING → RUNNING → IDLE → RUNNING → IDLE → ...` for as many turns as the agent is woken. A session is terminal (`COMPLETED` / `FAILED` / `CANCELLED`) only when the agent leaves the task's participants list.
- **AC-OFFICE-TASKS-001.4:** Wakeups (section E) drive transitions IDLE → RUNNING. Turn-complete events drive RUNNING → IDLE, tearing down the executor and agent process entirely; the conversation is preserved via the stored ACP session token.
- **AC-OFFICE-TASKS-001.5:** Kanban / quick-chat sessions keep their per-launch model and `WAITING_FOR_INPUT` semantics - office's IDLE state is office-scoped.
- **AC-OFFICE-TASKS-001.6:** Tasks form a tree via `parent_id`. Parent tasks act as the default home for shared specs, plans, and coordination documents.
- **AC-OFFICE-TASKS-001.7:** Child tasks can read parent-owned documents and write parent-owned coordination documents by default. Document handoffs reuse the existing **blocker mechanism**: a consumer task is blocked-by the producer task and reads the resulting documents from its wakeup prompt context. There is no separate "required documents" data type.
- **AC-OFFICE-TASKS-001.8:** Agents can list related tasks (parent, children, siblings, blockers, blocked) and can read/write allowed task documents via MCP or CLI tools.

## System design

The migrated technical source is split into [part 1](../system-design/tasks-01.md), [part 2](../system-design/tasks-02.md).
