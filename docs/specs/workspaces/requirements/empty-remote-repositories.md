---
status: draft
system: workspaces
created: 2026-08-30
owners:
  - kandev
---

# Empty Remote Repository Requirements

## Overview

The workspace system owns remote repository materialization and the Git state used for task worktrees. An empty remote repository has no commit for a task branch.

Kandev must let a user start work without changing the remote during task launch. Kandev can initialize the remote only during an explicit publication action.

## Terminology

- **Empty remote:** An authenticated Git remote that advertises zero refs.
- **Local baseline:** A Kandev-created empty commit that anchors the selected base branch.
- **Bootstrap marker:** A local Kandev ref that identifies the exact local baseline commit.
- **First publication:** The first explicit Push or Create PR action that publishes the local baseline and task branch.

## Requirements

### REQ-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001: Empty Remote Task Launch

**Intent:** Let users work in an empty remote repository while Kandev preserves normal task worktree isolation.

**User story:** As a workspace user, I want to start a task from an empty remote repository, so that an agent can create the first project files.

#### Acceptance criteria

- **AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.1:** When an authenticated remote advertises zero refs, Kandev shall classify it as empty instead of reporting a missing base branch.
- **AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.2:** When the remote is empty, Kandev shall create one empty local baseline commit on the selected base branch.
- **AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.3:** The local baseline shall contain no user files, generated project files, README, license, or `.gitignore`.
- **AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.4:** Kandev shall create the task worktree and task branch from the local baseline through the normal worktree lifecycle.
- **AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.5:** Initial launch, resume, and worktree recreation shall not publish a ref or commit to the remote.
- **AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.6:** When Kandev cannot prove that the remote advertises zero refs, the normal required-refresh failure rules shall apply.
- **AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.7:** In a multi-repository task, Kandev shall apply empty-remote preparation only to each repository that advertises zero refs.

### REQ-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002: Explicit First Publication

**Intent:** Initialize the remote base branch only when the user explicitly publishes task work.

**User story:** As a workspace user, I want Push and Create PR to initialize an empty remote safely, so that later Git and review operations use a valid base branch.

#### Acceptance criteria

- **AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.1:** When the user selects Push and the remote remains empty, Kandev shall publish the marked baseline to the selected base branch before it publishes the task branch.
- **AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.2:** When the user selects Create PR and the remote remains empty, Kandev shall publish the marked baseline before it publishes the task branch and creates the change request.
- **AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.3:** First publication shall use the task runtime Git credential route. Clone or read access alone shall not authorize publication.
- **AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.4:** Kandev shall never force-push the baseline branch during first publication.
- **AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.5:** When the remote gains refs before first publication, Kandev shall stop automatic initialization and preserve all local and remote history.
- **AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.6:** When baseline publication fails, Kandev shall keep the task branch local and show a credential-safe recovery error.
- **AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.7:** When baseline publication succeeds but task-branch publication fails, Kandev shall report the partial result and preserve the task branch for retry.
- **AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.8:** Desktop and mobile users shall reach first publication through the existing Changes push and change-request controls.
- **AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.9:** Provider-specific change-request creation shall begin only after both required Git refs are present on the remote.

## Out of scope

- Creating a hosted repository.
- Adding project templates, files, licenses, or README content.
- Publishing during task launch, resume, or worktree recreation.
- Force-pushing or deleting a remote branch during bootstrap recovery.
- Automatically merging, rebasing, or resetting when another actor initializes the remote.
- Adding change-request support for a provider that Kandev does not support.

## Related requirements

- [Worktree Base Refresh Requirements](worktree-base-refresh.md)

## System design

- [Empty Remote Repository System Design](../system-design/empty-remote-repositories.md)
