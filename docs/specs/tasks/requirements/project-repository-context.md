---
status: draft
system: tasks
created: 2026-10-07
owners:
  - kandev
---

# Project repository context requirements

## Overview

Project-backed tasks need source files at their first launch.
The task system owns this contract because it owns task creation and launch behavior.
Office owns project configuration. Workspaces own repository identity and source preparation.

The [Office project design](../../office/system-design/overview-01.md#projects)
already describes project repository inheritance. This document defines its creation-time acceptance criteria.

## Terminology

- **Project sources:** The selected project's ordered local repository paths or remote Git URLs.
- **Root task:** A task without a parent.
- **Automatic selection:** A root creation request omits repositories or supplies null, without an explicit workspace path or shared workspace.
- **Explicit selection:** A supplied repository list, including an empty list, or an explicit workspace source.

## Requirements

### REQ-TASKS-PROJECT-REPOSITORIES-001: Project sources at task creation

**Intent:** A project-backed task receives its source context before an agent can start work.

#### Acceptance criteria

- **AC-TASKS-PROJECT-REPOSITORIES-001.1:** When automatic selection names a project, creation shall attach every supported project source before publishing task creation or permitting launch.
- **AC-TASKS-PROJECT-REPOSITORIES-001.2:** When a project source already identifies a repository in the task's workspace, creation shall reuse that repository. Duplicate project entries shall produce one attachment per repository, preserving first occurrence order.
- **AC-TASKS-PROJECT-REPOSITORIES-001.3:** When supported project sources are unregistered, creation shall resolve them through normal workspace repository admission. Task branches shall use normal repository defaults and policies.
- **AC-TASKS-PROJECT-REPOSITORIES-001.4:** When creation supplies explicit sources, the system shall preserve that selection without adding project repositories. An explicit empty root list shall remain repositoryless.
- **AC-TASKS-PROJECT-REPOSITORIES-001.5:** When a subtask omits repositories, existing parent repository and workspace inheritance shall apply. Creation shall not replace or supplement that context from the project.
- **AC-TASKS-PROJECT-REPOSITORIES-001.6:** When a project is missing, unreadable, or belongs to another workspace, automatic selection shall fail before task creation. Invalid or unsupported project sources shall also fail instead of silently disappearing.
- **AC-TASKS-PROJECT-REPOSITORIES-001.7:** When no project is selected, or its source list is empty, existing repositoryless task behavior shall apply.
- **AC-TASKS-PROJECT-REPOSITORIES-001.8:** When a repository-backed task first launches, its prepared environment shall contain the project files under the selected executor's existing semantics. `local_pc` shall use its source checkout. Worktree shall use an isolated worktree. An inheriting child shall retain the parent's prepared source context.
- **AC-TASKS-PROJECT-REPOSITORIES-001.9:** Desktop and phone Office creation shall receive the same source attachments through the existing task form. Creation errors shall use the existing error presentation.

## Compatibility and exclusions

Existing tasks receive no automatic backfill. Changing a task's project or editing
project sources does not replace its attachments. Existing task external-ID retry
semantics remain unchanged.

Executor selection, repository-provider support, plain-folder project support,
branch policy, and multi-branch explicit selections are outside this change.
Unsupported source formats produce a creation error.

## System design and implementation

- [Project repository context design](../system-design/project-repository-context.md)
- [Issue 4289 implementation package](../../../plans/office-project-repository-context/plan.md)
