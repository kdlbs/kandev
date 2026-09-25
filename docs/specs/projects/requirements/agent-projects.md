---
status: active
system: projects
created: 2026-09-24
owners:
  - kandev
---

# Agent Projects Requirements

## Overview

A user can keep one coordinator conversation and a visible tree of worker tasks
for a long-running body of repository work. This system owns the project
lifecycle and shared context; existing task, workspace, and agent systems
provide execution. An Agent Project is distinct from an Office project.

## Terminology

- **Agent Project:** A named, workspace-scoped coordinator and its selected
  remote repositories, profiles, tasks, and persistent context.
- **Coordinator task:** The project's single main task and conversation.
- **Worker task:** A child task created for a piece of project work.
- **Context:** Files shared by the coordinator and every project worker.

## Requirements

### REQ-PROJECTS-AGENT-PROJECTS-001: Project creation and editing

**Intent:** Make project setup explicit and recoverable.

#### Acceptance criteria

- **AC-PROJECTS-AGENT-PROJECTS-001.1:** When a user creates a project, the system shall require a name, at least one selected remote repository, and valid coordinator, economy, and frontier agent profiles in the same workspace; it shall identify the primary repository used for agent startup and reject a profile or default executor that cannot access every required project path.
- **AC-PROJECTS-AGENT-PROJECTS-001.2:** After successful creation, the system shall show one project row and a durable coordinator conversation. Opening the project before sending a prompt shall not start an agent turn.
- **AC-PROJECTS-AGENT-PROJECTS-001.3:** A user shall be able to rename a project and change its three profiles. A profile edit shall apply to new coordinator sessions and newly created workers. An existing coordinator session or worker shall keep its assigned profile when stopped, resumed, or messaged.
- **AC-PROJECTS-AGENT-PROJECTS-001.4:** A user shall be able to change the repository selection only while no project task has a running agent; existing task checkouts and branches shall remain intact, and removed repositories shall remain available to tasks already using them.
- **AC-PROJECTS-AGENT-PROJECTS-001.5:** If creation or editing fails, the UI shall report the failure without displaying a partly configured project or losing the user's draft.

### REQ-PROJECTS-AGENT-PROJECTS-002: Coordinator and worker routing

**Intent:** Keep orchestration distinct from execution while preserving user control.

#### Acceptance criteria

- **AC-PROJECTS-AGENT-PROJECTS-002.1:** A project coordinator shall use the configured coordinator profile and receive project instructions that identify its context path, task boundaries, and worker tools. A worker shall use the selected economy or frontier profile for its launch.
- **AC-PROJECTS-AGENT-PROJECTS-002.2:** The coordinator shall be able to create a direct worker with an economy or frontier choice, title, and prompt. By default the worker shall receive all current project repositories, with each checkout based on that repository's configured default branch. The coordinator may choose a subset and an available remote base branch for each; the resulting worker shall use independent checkouts and appear beneath the project row.
- **AC-PROJECTS-AGENT-PROJECTS-002.3:** The coordinator shall have only project-scoped coordination tools. Requests to create a worker with an arbitrary profile, repository, parent, or workspace shall be rejected, including direct API/MCP calls that bypass the UI. Generic task creation under a project task and workflow operations on project tasks shall also be rejected.
- **AC-PROJECTS-AGENT-PROJECTS-002.4:** A worker's completion, cancellation, or failure shall remain visible in the project tree and shall not by itself start or advance the coordinator. The coordinator shall be able to inspect each worker's state, latest result or error, repository branches, and linked change requests. A user can resume or message the coordinator explicitly.
- **AC-PROJECTS-AGENT-PROJECTS-002.5:** Native subagents inside an agent execution shall remain distinct from Kandev worker tasks and shall not create a project task row.
- **AC-PROJECTS-AGENT-PROJECTS-002.6:** When eight project worker sessions are starting or running, a further create-and-start request shall fail with a visible capacity error and shall not create another task. Stopped and completed workers shall not consume this concurrent limit.

### REQ-PROJECTS-AGENT-PROJECTS-003: Shared context and file views

**Intent:** Give project tasks a durable common knowledge base without mixing it into Git changes.

#### Acceptance criteria

- **AC-PROJECTS-AGENT-PROJECTS-003.1:** The coordinator and every worker shall read and write the same persistent project context, including `notes.md`; changes shall be visible to later turns and other project tasks without copying files manually.
- **AC-PROJECTS-AGENT-PROJECTS-003.2:** The coordinator Files panel shall show `Context` and `Workspace` at its root. A worker Files panel shall open at its repository workspace. Both shall allow navigation to project context through an explicit control or path.
- **AC-PROJECTS-AGENT-PROJECTS-003.3:** The Changes panel shall show Git changes for the selected task's repository worktrees only. Context edits shall never appear as repository changes.
- **AC-PROJECTS-AGENT-PROJECTS-003.4:** Agents shall launch in the primary repository checkout so repository `AGENTS.md` and skills are discoverable; other selected repositories shall be accessible from that task's workspace.
- **AC-PROJECTS-AGENT-PROJECTS-003.5:** A missing, unsafe, or inaccessible context link shall block agent launch with a recoverable error instead of silently starting with an empty or different context. Project context shall survive task/session restart and worker deletion.
- **AC-PROJECTS-AGENT-PROJECTS-003.6:** Before launching a coordinator or worker, the system shall grant its agent profile read/write access to project context and every selected repository checkout. If an agent type cannot receive that access, launch shall fail with an actionable error before the agent starts.

### REQ-PROJECTS-AGENT-PROJECTS-004: Workflow-free project tasks

**Intent:** Keep the project conversation and workers outside board workflow stages.

#### Acceptance criteria

- **AC-PROJECTS-AGENT-PROJECTS-004.1:** Project coordinator and worker tasks shall have no selectable workflow or workflow step; create/edit dialogs, task topbars, row menus, and mobile equivalents shall omit workflow controls for those tasks.
- **AC-PROJECTS-AGENT-PROJECTS-004.2:** Project tasks shall remain usable for normal session conversation, files, Changes, and stop actions. Individual worker archive/delete actions shall leave the coordinator and context intact. Coordinator archive/delete shall be available only through a project-level action.
- **AC-PROJECTS-AGENT-PROJECTS-004.3:** Project tasks shall not appear on ordinary workflow boards, Office project lists, or Office cost/routing projections. Existing tasks shall retain current workflow behavior.
- **AC-PROJECTS-AGENT-PROJECTS-004.4:** Project archive/delete shall require confirmation that names the coordinator and worker count. Archive shall include all project tasks, retain context, and make the project available for restore. Delete shall include all project tasks, offer a separate retain/remove-context choice, block while any project task is running, and report the result. A worker shall never become an orphaned workflow-free task.

### REQ-PROJECTS-AGENT-PROJECTS-005: Navigation and rollout

**Intent:** Make project status accessible on desktop and phone while keeping the initial release isolated.

#### Acceptance criteria

- **AC-PROJECTS-AGENT-PROJECTS-005.1:** When the Projects flag is enabled, desktop and phone navigation shall show one section labeled Projects. Each Agent Project shall have one row labeled with its name; opening that row shall open its coordinator conversation. A project with worker tasks shall allow those tasks to expand directly beneath its row, each showing its own state. A project with no workers shall have no empty child row. Create and edit actions shall remain available.
- **AC-PROJECTS-AGENT-PROJECTS-005.2:** On a phone, creation and editing shall use a focused, scrollable surface with touch-accessible controls; project and child rows shall be reachable without hover, and back/dismiss shall restore navigation focus.
- **AC-PROJECTS-AGENT-PROJECTS-005.3:** When the flag is disabled, project routes, APIs, MCP tools, and launch paths shall reject new project operations before side effects; normal task and Office behavior shall remain available. A project created while enabled shall remain stored and become available again when re-enabled.
- **AC-PROJECTS-AGENT-PROJECTS-005.4:** If a configured profile, repository, or stored executor profile becomes unavailable, the project shall remain readable and show the missing dependency; a new launch shall be blocked until the user supplies a valid replacement.

## Out of scope

- Local repository sources, remote executors, cross-machine context sync, subscriptions, scheduled work, worker auto-resume, and thousands-of-workers scale claims.
- Automatic preference learning, automatic PR/CI follow-up, and exposing arbitrary tool catalogs to the coordinator.
- Converting existing Office projects or ordinary tasks into Agent Projects.
