---
status: active
system: tasks
created: 2026-11-10
owners:
  - kandev
---

# Agent Worktree Adoption Requirements

## Overview

Agents running inside a task can create their own Git worktrees, for example to start a second
pull request after the first one is open. Today Kandev only follows the checkouts it created, so
changes made in an agent-created worktree never reach the Changes or Files panels and cannot be
committed, pushed, or opened as a pull request from Kandev.

This capability turns an agent-created worktree into an ordinary task branch. The agent can report
the worktree, Kandev detects it when the agent does not, and the user can remove a root that should
not be part of the task.

The tasks system owns this contract because the outcome is a change to the task's repository and
branch attachments. It extends [multi-branch tasks](multi-branch.md) and the legacy add-branch
action in [workspace source attachment](attach-workspace-sources.md). Workspace repository identity,
the agentctl workspace stream, and pull-request synchronization keep their existing owners.

## Terminology

- **Task workspace root:** The established task folder that contains the agent working directory
  and the task's checkouts. It is the containment boundary for detection.
- **Agent-created worktree:** A linked Git worktree of a repository already attached to the task.
  Kandev did not create it, and it is not yet a task branch.
- **Adopted worktree:** An agent-created worktree that Kandev has attached to the task as a branch.
  Kandev uses it in place and never moves or recreates it.
- **Root:** One checkout shown as its own group in the Changes panel and as its own folder in the
  Files panel. The primary root contains the agent's working directory.
- **Detach:** Removing a non-primary root from the task without touching its files, branch, or pull
  requests.

## Requirements

### REQ-TASKS-AGENT-WORKTREE-ADOPTION-001: Agent-reported worktrees

**Intent:** Let the agent tell Kandev about a worktree it already created, so the new branch becomes
a task branch at once.

**User story:** As an agent, I want to register the worktree I just created, so that the user sees
and can ship its changes from the task.

#### Acceptance criteria

- **AC-TASKS-AGENT-WORKTREE-ADOPTION-001.1:** When an agent calls `add_branch_to_task_kandev` with a
  `worktree_path` that names an existing linked worktree inside the task workspace root, on a named
  branch, of a repository attached to the task, the system shall attach that worktree as a task
  branch in place. The worktree's directory, branch, `HEAD`, and uncommitted changes shall stay
  unchanged.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-001.2:** When the reported path resolves outside the task
  workspace root (after symlink resolution), is not a linked worktree, belongs to a repository that
  is not attached to the task, or has a detached `HEAD`, the system shall reject the call with an
  actionable error and shall make no task change.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-001.3:** When `worktree_path` is supplied together with a
  `checkout_branch` that differs from the worktree's current branch, the system shall reject the
  call. A supplied `base_branch` shall become the root's comparison base; when omitted, the
  repository default branch shall be used.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-001.4:** When the call succeeds, the response shall return the
  adopted worktree path and the task workspace path. The agent working directory shall stay
  unchanged.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-001.5:** When `worktree_path` is omitted, the existing add-branch
  behavior, including Kandev-created sibling worktrees and its executor restrictions, shall stay
  unchanged.

### REQ-TASKS-AGENT-WORKTREE-ADOPTION-002: Automatic detection under the task workspace

**Intent:** Show an agent-created worktree even when the agent does not report it.

**User story:** As a user, I want changes from a worktree the agent created to start appearing in
the Changes panel without asking the agent to register it.

#### Acceptance criteria

- **AC-TASKS-AGENT-WORKTREE-ADOPTION-002.1:** When an agent-created worktree appears inside the task
  workspace root and is on a named branch, the system shall adopt it within 30 seconds while the
  task has a running agent turn or an open task view. Otherwise it shall adopt the worktree at the
  next observation of the workspace.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-002.2:** The system shall never adopt a worktree whose resolved
  path is outside the task workspace root. This includes `/tmp`, siblings of the task folder, the
  user's other checkouts, and other tasks' worktrees, even when they share the same Git repository.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-002.3:** The system shall not adopt a checkout that is already a
  task root, a submodule, a fresh clone or other repository not attached to the task, a worktree in
  detached `HEAD`, or a worktree the user detached from this task (see REQ-005). A worktree in
  detached `HEAD` shall be adopted once it checks out a named branch.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-002.4:** Detection shall not start, resume, or prompt an agent,
  and shall not create, move, or modify any file or Git reference.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-002.5:** Detection shall be available for host Worktree and
  Local executors. Other executors shall keep their current behavior without error.

### REQ-TASKS-AGENT-WORKTREE-ADOPTION-003: Reconciliation of detection and reports

**Intent:** Keep exactly one root per worktree, whichever path found it first.

#### Acceptance criteria

- **AC-TASKS-AGENT-WORKTREE-ADOPTION-003.1:** When an agent reports a worktree that the system has
  already detected, the call shall succeed and return the existing root. No duplicate root, branch
  row, tracker, or panel group shall appear.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-003.2:** When the system detects a worktree that the agent has
  already reported, the system shall make no change.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-003.3:** When an agent reports a worktree that was previously
  detached by the user, the system shall attach it again and clear the detachment. Detection alone
  shall never clear a detachment.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-003.4:** When an agent switches the branch inside an adopted
  worktree, the root shall show the new branch and its changes, matching the existing behavior for
  Kandev-created checkouts.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-003.5:** When an adopted worktree is removed from disk, its root
  shall disappear from the Changes and Files panels. Pull requests already linked to the task shall
  remain linked.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-003.6:** Adopted roots shall survive backend restart, session
  resume, and additional sessions on the same task without being detected again as new.

### REQ-TASKS-AGENT-WORKTREE-ADOPTION-004: Multiple roots in task panels

**Intent:** Let the user review and ship each adopted worktree like any other task branch.

**User story:** As a user, I want each worktree to appear as its own root, so that I can commit,
push, and open a pull request for it without leaving the task.

#### Acceptance criteria

- **AC-TASKS-AGENT-WORKTREE-ADOPTION-004.1:** When a task has an adopted worktree, the Changes panel
  shall show it as a separate repository group, labelled with its repository and branch, with its
  own uncommitted changes, commits, and comparison base.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-004.2:** The Files panel shall show each adopted worktree's
  folder at its location in the task workspace, marked as a repository root with its branch, and
  shall reveal it even when a parent folder is hidden (for example `.claude/worktrees`). Opening a
  file from that folder shall read the worktree's copy, not the primary checkout's copy at the same
  relative path.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-004.3:** An adopted root shall offer the same per-root stage,
  commit, push, pull, and create-pull-request actions as a Kandev-created branch root. A pull
  request created from it shall be linked to the task and appear in the task's pull-request
  selector.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-004.4:** New roots and removed roots shall appear or disappear
  without a page reload on desktop, phone, and coarse-pointer tablet layouts.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-004.5:** Right-clicking a root, either its Changes group header
  or its marked Files folder, shall open one root menu with the same items on both surfaces: Open
  in editor, Open terminal here (a terminal whose working directory is the root), Show in Files or
  Show in Changes (the other surface, scrolled to the same root), Copy full path, Copy relative
  path (relative to the task workspace root), Copy branch name, Push, Create PR or Open PR when one
  is linked, and Remove from task for non-primary roots. Items that are unavailable for the root's
  state shall be disabled with a visible reason rather than hidden.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-004.6:** A root folder in the Files panel shall not offer the
  generic folder Delete or Rename actions from any menu or shortcut. Folders and files inside a
  root keep their existing menu.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-004.7:** The root menu shall open with a long-press on touch
  layouts, presented as a bottom sheet on phone, and with the context-menu key or Shift+F10 on the
  focused row. Closing it shall return focus to the row.

### REQ-TASKS-AGENT-WORKTREE-ADOPTION-005: User detach of a root

**Intent:** Let the user remove a root that should not be part of the task, without risk to work on
disk.

**User story:** As a user, I want to remove a worktree from the task panels, so that the task shows
only the work I care about.

#### Acceptance criteria

- **AC-TASKS-AGENT-WORKTREE-ADOPTION-005.1:** Every non-primary repository root, adopted or
  Kandev-created, shall offer Remove from task as the last item of its root menu (AC-004.5), on
  desktop, keyboard, and touch.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-005.2:** Before removal, a confirmation shall state that files,
  the branch, and pull requests are kept. Cancelling shall leave the task unchanged.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-005.3:** When the user confirms, the root shall disappear from
  the Changes and Files panels. The system shall not delete the directory, the worktree
  registration, the branch, or any commit, and shall not close or unlink any pull request.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-005.4:** After removal, detection shall not adopt the same
  worktree path again for this task. An agent report of that path (REQ-003) shall attach it again.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-005.5:** The primary root shall not offer Remove from task. A
  removal request for it from any surface shall be rejected without change.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-005.6:** Removal shall be allowed while an agent turn is
  running. It shall not stop, restart, or prompt the agent.

### REQ-TASKS-AGENT-WORKTREE-ADOPTION-006: Agent guidance

**Intent:** Steer agents to report new worktrees so the task updates at once and without guessing.

#### Acceptance criteria

- **AC-TASKS-AGENT-WORKTREE-ADOPTION-006.1:** When a session runs on an executor that supports
  adoption, the first-turn Kandev context shall tell the agent to create any new Git worktree
  inside the task workspace and then report it with `add_branch_to_task_kandev` and
  `worktree_path`, or to let that tool create the worktree.
- **AC-TASKS-AGENT-WORKTREE-ADOPTION-006.2:** When the executor does not support adoption, the
  context shall not include this guidance.

## Out of scope

- Worktrees outside the task workspace root, on any executor.
- Remote and container executors (Docker, SSH, Kubernetes, Sprites).
- Fresh clones and repositories not already attached to the task.
- Deleting a worktree or its branch from Kandev, and stacking or merging pull requests across roots.
- A user action to add an existing worktree by path; agents report it, and detection covers the
  rest.
- Moving the agent working directory into an adopted worktree.
