---
status: shipped
created: 2026-07-14
owner: kandev
---

# Quick Chat Repository Context

## Why

Quick chat is useful for focused questions, but its agent-only picker does not explain the
surface or let a user ground the conversation in repository code. Users need optional code
context without changing their checked-out branches or creating a kanban task.

## What

- A new quick chat presents a searchable agent-profile selector and requires an agent before
  it can start.
- A user may add zero, one, or multiple imported workspace repositories. Each selected
  repository has one explicit base branch and can appear at most once in a chat.
- A workspace with no existing quick chats explains that quick chat is for discussing an idea,
  question, or codebase outside the task board.
- Agent and repository fields include concise helper copy. Repository copy explains that the
  optional selection focuses the conversation on specific code and branches.
- Repo-backed quick chats run in Kandev-owned isolated worktrees. They never switch or modify
  the branch checked out in the user's source directory.
- Repository context contains committed branch state. Uncommitted files from the user's
  working directory are not copied into the chat.
- Repo-less quick chats retain their current scratch workspace and default-executor behavior.
- The setup flow supports desktop and mobile layouts without horizontal page scrolling or
  hover-only actions.

Decision: [ADR 0038](../../decisions/0038-quick-chat-repository-isolation.md).

## API surface

`POST /api/v1/workspaces/:id/quick-chat` accepts:

```json
{
  "title": "Codex - Chat 1",
  "agent_profile_id": "profile-id",
  "repositories": [
    { "repository_id": "repo-1", "base_branch": "main" },
    { "repository_id": "repo-2", "base_branch": "develop" }
  ]
}
```

`agent_profile_id` is required after workspace-default resolution. `repositories` is optional
and ordered. The legacy singleton repository fields remain accepted, but callers cannot mix
the singleton and plural shapes in one request.

The response remains:

```json
{ "task_id": "task-id", "session_id": "session-id" }
```

## Failure modes

- An incomplete repository row keeps Start Chat disabled.
- A missing agent, foreign repository, duplicate repository, invalid branch, or mixed request
  shape fails validation and does not launch an agent.
- If repository preparation or agent launch fails, the backend deletes the ephemeral task and
  its materialized worktrees. The setup remains visible with its selections and reports the
  error.
- A superseded in-flight start still deletes the completed orphan task.

## Persistence guarantees

Quick-chat task/session rows and their isolated worktrees survive backend restarts under the
existing task runtime rules. Closing, deleting, workspace deletion, or idle expiration cleans
them through the existing quick-chat task deletion path.

## Scenarios

- **GIVEN** a workspace with no quick chats, **WHEN** the user opens Quick Chat, **THEN** the
  setup explains its purpose and shows agent and repository helper copy.
- **GIVEN** an agent and no repositories, **WHEN** the user starts a chat, **THEN** the chat uses
  the existing repo-less scratch workspace.
- **GIVEN** an agent and two repository/branch selections, **WHEN** the user starts a chat,
  **THEN** the agent workspace contains both repositories at committed state from those branches.
- **GIVEN** the user's source checkout is on another branch or dirty, **WHEN** a repo-backed chat
  starts, **THEN** the source checkout and its uncommitted files remain unchanged.
- **GIVEN** one repo-backed chat is active, **WHEN** another chat targets the same repository and
  base branch, **THEN** both start in distinct Kandev-owned worktrees.
- **GIVEN** an existing quick chat, **WHEN** the user opens a new chat tab, **THEN** the long
  first-use introduction is omitted while field helper copy remains.
- **GIVEN** a mobile viewport, **WHEN** the user selects an agent and repository branch, **THEN**
  all controls remain touch-accessible and the page has no horizontal overflow.

## Out of scope

- Remote repository URL entry or arbitrary local folders.
- Including uncommitted working-directory changes.
- Selecting the same repository more than once in a single quick chat.
- Choosing an executor in the quick-chat setup.
