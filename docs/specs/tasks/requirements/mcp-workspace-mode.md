---
status: draft
system: tasks
created: 2026-09-11
owners:
  - kandev
---

# MCP task workspace modes

## Overview

Task creation must distinguish session callers from external MCP callers.
The task system owns creation admission. Office retains its skills, CLI,
runtime permissions, assignment rules, and scheduler.

The [review discussion](https://github.com/kdlbs/kandev/pull/3526#discussion_r3980112224)
identifies a Kanban session creating work in an Office workspace.
The user confirmed that Office agents must continue creating tasks through
skills and CLI, with no Office task-creation MCP tool.

## Terms

- Workspace mode: Kanban or Office, determined from stored workspace identity.
- Session caller: an agent running within a Kandev task session.
- External caller: a client entering through the external MCP endpoint.
- Materialized workspace: a checkout or worktree. The existing
  `workspace_mode` argument controls this, not Kanban versus Office.

## Requirements

### REQ-TASKS-MCP-WORKSPACE-MODE-001: Mode-bound creation admission

**Intent:** Prevent session callers from creating work through the wrong surface.

#### Acceptance criteria

- **AC-TASKS-MCP-WORKSPACE-MODE-001.1:** A Kanban session shall create only
  Kanban tasks in authorized Kanban workspaces. Existing permitted selection
  between Kanban workspaces shall remain available.
- **AC-TASKS-MCP-WORKSPACE-MODE-001.2:** An Office session shall not create
  tasks through MCP, including direct backend calls. Its skills and CLI shall
  remain the creation path, bound to its Office workspace and run permissions.
- **AC-TASKS-MCP-WORKSPACE-MODE-001.3:** Supplied task, session, parent,
  workspace, or profile arguments shall not change caller authority.
  Missing or inconsistent trusted identity shall fail without mutation.
- **AC-TASKS-MCP-WORKSPACE-MODE-001.4:** Explicit and inherited destination
  references shall agree. Missing, inaccessible, mixed-workspace, or disallowed
  mode references shall fail before repository registration, creation, or launch.
- **AC-TASKS-MCP-WORKSPACE-MODE-001.5:** Missing workspace input shall resolve
  from an authorized parent, then the session workspace. An external root shall
  require a workspace unless exactly one authorized workspace exists across
  both modes. Invalid explicit input shall not fall back.
- **AC-TASKS-MCP-WORKSPACE-MODE-001.6:** Rejected requests, including retries
  with an external ID, shall not expose forbidden tasks or create task,
  repository, assignment, session, event, or queued-run side effects.

### REQ-TASKS-MCP-WORKSPACE-MODE-002: Existing creation surfaces

**Intent:** Preserve the deliberate split between task MCP and Office CLI.

#### Acceptance criteria

- **AC-TASKS-MCP-WORKSPACE-MODE-002.1:** Kanban sessions shall expose
  `create_task_kandev`. Office sessions shall expose no task-creation MCP
  tool. Stale catalogs and direct backend actions shall not bypass admission.
- **AC-TASKS-MCP-WORKSPACE-MODE-002.2:** Authorized Kanban creation shall
  retain its profile inheritance, deferred launch, worktree, and external-ID behavior.
- **AC-TASKS-MCP-WORKSPACE-MODE-002.3:** Office skills shall continue directing
  agents to CLI creation with existing create, subtask, assignment, and task-scope
  rules. This change shall not expand those permissions.
- **AC-TASKS-MCP-WORKSPACE-MODE-002.4:** Office CLI creation shall retain its
  existing task identity, project inheritance, workflow, events, and scheduler behavior.
- **AC-TASKS-MCP-WORKSPACE-MODE-002.5:** No new Office creation tool or
  caller-selectable authority parameter shall be introduced. A launch profile
  shall not be interpreted as an Office assignee.

### REQ-TASKS-MCP-WORKSPACE-MODE-003: External access to both modes

**Intent:** Preserve external creation and management across authorized workspaces.

#### Acceptance criteria

- **AC-TASKS-MCP-WORKSPACE-MODE-003.1:** External MCP shall retain
  `create_task_kandev` for authorized Kanban and Office workspaces.
  `parent_id="self"` shall fail without current-task context.
- **AC-TASKS-MCP-WORKSPACE-MODE-003.2:** Existing external workspace/task
  discovery, conversation/session reads, move, state-update, archive, and delete
  operations shall remain available for authorized tasks in both modes.
- **AC-TASKS-MCP-WORKSPACE-MODE-003.3:** The new creation gate shall not grant
  management permissions or bypass existing lifecycle rules. It shall not
  convert task or workspace mode.
- **AC-TASKS-MCP-WORKSPACE-MODE-003.4:** External access shall retain user,
  tenant, workspace, and token permissions. Missing session identity shall not
  imply external authority. Mode admission shall also apply with user auth disabled.
- **AC-TASKS-MCP-WORKSPACE-MODE-003.5:** Visible disallowed destinations shall
  return a useful restriction message. Unknown and inaccessible destinations
  shall remain indistinguishable. Documentation shall distinguish session MCP,
  Office skills/CLI, external MCP, and materialized-workspace policy.

## Exclusions

- New Office MCP tools or external assignment fields.
- Office CLI, permission, scheduler, or lifecycle redesign.
- Changes to external creation schemas, launch behavior, or idempotency contracts.
- Workspace conversion, historical migration, rendered UI, or new endpoints.
- Changes to automation/configuration catalogs or their established authority.

## Design and delivery

- [System design](../system-design/mcp-workspace-mode.md)
- [Implementation plan](../../../plans/mcp-workspace-mode/plan.md)
