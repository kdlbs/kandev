---
status: draft
created: 2026-05-05
owner: tbd
---

# Tasks Without Repositories

## Why

Not every piece of agent work is tied to a codebase. Users want to brainstorm a design, ask a research question, summarize a Linear ticket, or run a one-off shell command without picking a repo first — and today the create-task flow forces a repo selection. The repo requirement also blocks new users who haven't connected a workspace yet from trying the product. Utility agents (slack triage) already prove the agent-launch path works without a repo; this feature exposes the same capability through the normal task UI.

## What

- Users can create a task with no repository attached. The repo selector in the create-task dialog is optional, with a clear "no repository" path.
- When choosing "no repository", the user can optionally pick a starting folder on their machine. The agent launches in that folder as its working directory. Files the agent reads, writes, and runs commands against are the user's real files — kandev does not clone, copy, or git-manage the directory.
- If no folder is picked, the task gets a fresh scratch workspace owned by kandev — empty, no git, persists for the task's lifetime so the agent can write files and reference its own outputs across turns.
- Repo-dependent UI (diff viewer, branch picker, git status, PR creation, worktree controls) is hidden — not greyed out — for repo-less tasks.
- Repo-less tasks use the workspace's default executor. The `worktree` executor is unavailable for them (it inherently needs a repo); if the workspace default is `worktree`, fall back to `local`.
- Picking a starting folder is a local-executor-only feature. Container-based executors (`local_docker`, `remote_docker`, `sprites`) do not expose the folder picker — they always get a fresh scratch workspace.
- The MCP / tool surface is unchanged — repo-less tasks get the same tools as repo-bound tasks. The agent ignores git-specific tools that don't apply.
- Existing repo-bound tasks are unaffected. The repo selector remains the default in the create-task dialog.

## Scenarios

- **GIVEN** a user opens the create-task dialog, **WHEN** they choose "no repository" without picking a folder and submit a title, **THEN** the task is created with an empty `Repositories` slice, the agent launches in a fresh scratch workspace, and the task detail view hides all git/diff/PR panels.
- **GIVEN** a user chooses "no repository" and picks `~/Documents/research/` as the starting folder, **WHEN** they submit the task, **THEN** the agent launches with `~/Documents/research/` as its working directory and can read existing files there.
- **GIVEN** a repo-less task with no folder picked is running, **WHEN** the agent writes a file (e.g. `notes.md`) and the user sends a follow-up turn, **THEN** the file is still present in the scratch workspace and the agent can read it back.
- **GIVEN** a repo-less task, **WHEN** the user opens the executor picker, **THEN** the `worktree` option is hidden or disabled with a tooltip explaining it requires a repository.
- **GIVEN** a user creates a repo-less task in a workspace whose default executor is `worktree`, **WHEN** the task launches, **THEN** the executor falls back to `local` and the task starts normally.

## Out of scope

- Attaching a repo to a repo-less task after creation (no "promote to repo task" flow in v1).
- Detaching repos from existing repo-bound tasks.
- Cross-task scratch workspaces or shared scratch dirs — each repo-less task without a picked folder gets its own isolated scratch workspace, wiped when the task is deleted.
- Sandboxing or write-protection of a user-picked folder. The agent has full read/write access to whatever the user picks — same trust model as running an editor against the folder.
- A folder picker for container-based executors. Mounting arbitrary host paths into Docker/Sprites is out of scope for v1.
- Special "quick chat" UX distinct from a normal task (no separate inbox, no ephemeral mode). A repo-less task is a normal task that happens to have zero repos.
- Slack `!kandev` triage is unchanged; it continues to use utility agents directly, not the task system.

## Open questions

- **Scratch workspace location and lifecycle.** Per-task scratch dir under the kandev data directory (e.g. `~/.kandev/data/scratch/<task-id>/`)? Wiped on task delete, or retained until manual cleanup? Per-executor — does `local_docker` get a named volume, or a tmpfs?
- **Folder picker mechanism.** Backend endpoint that lists local directories (browse from `$HOME`)? Or a free-text path input with server-side validation? Does kandev's existing repo-add flow already have a directory browser we can reuse?
- **Storage of the picked folder.** New nullable `task_sessions.workspace_path` column, or reuse an existing field?
