# Kandev Feature Guide

This page expands the short feature list in the README without turning the README into a catalog.

## Agent And Task Workflows

- **Parallel task execution:** run and review multiple agent tasks at once.
- **Kanban and pipeline workflows:** define workflow steps, prompts, automations, and agent handoffs per step.
- **Workflow import/export:** export one workflow or every workflow in a workspace as portable YAML, then import it into another workspace or Kandev install. See [workflow-import-export.md](workflow-import-export.md).
- **Subtasks:** split work into child tasks that inherit the parent task's workspace, workflow, agent profile, executor, and repositories by default.
- **Multi-repository tasks:** attach several repositories to one task. Each repository gets its own worktree and is grouped separately in changes, review, and PR surfaces.
- **Multi-branch tasks:** attach multiple branches from the same repository to one task when the work needs to produce several PRs.
- **Task documents:** tasks can hold multiple markdown documents with revision history, including plans, specs, notes, reviews, and custom documents.
- **Task labels:** workspace labels are reusable, filterable, and visible on task cards and task detail.
- **Public share links:** publish a redacted task conversation snapshot as a secret GitHub Gist after previewing exactly what will be shared. Existing shares can be revoked.

## Agent Interfaces

- **ACP agents:** Kandev supports ACP-native and ACP-adapter agents such as Claude Code, Codex, GitHub Copilot, Gemini CLI, Amp, Auggie, OpenCode, Cursor, Qwen, Factory Droid, iFlow, Kilocode, Pi, Kimi, Kiro, Qoder, Trae, and Oh My Pi.
- **Bring-your-own TUI agents:** any agent CLI can run in a PTY terminal through CLI passthrough, even without ACP support.
- **Voice mode:** dictate chat prompts from the composer. Supported engines include browser Web Speech, local in-browser Whisper Web, and server-side Whisper. Settings include language, click-to-toggle or hold-to-talk activation, auto-send, Whisper model size, and keyboard shortcut.
- **Utility agents:** one-shot helpers can generate or improve prompt text, branch names, commit messages, commit descriptions, PR titles, PR descriptions, and session summaries. Users can choose a default model, override models per action, and add custom utility agents.
- **Custom prompts:** reusable prompts can be created in settings and invoked from chat.
- **Secrets:** named secrets can be stored once and reused by profiles or integrations without pasting values into every task.

## Executors And Runtime

- **Executor types:** run agents as local host processes, git worktrees, Docker containers, remote SSH sessions, or Sprites cloud environments.
- **Executor profiles:** save per-runtime configuration, including prepare scripts, environment variables, credentials, and profile-specific settings.
- **Worktree isolation:** concurrent agents work in isolated git worktrees so their changes do not collide.
- **Per-task repository setup:** repositories can copy selected ignored files, such as `.env` files, into newly created worktrees.
- **Resource monitor:** optionally show CPU, memory, disk, CPU temperature, and load metrics in desktop task and kanban topbars. Kandev can also collect execution-environment metrics for Docker, SSH, Sprites, and other remote runtimes.

## Integrations And MCP

- **Integrations:** GitHub, GitLab, Jira, Linear, Sentry, and Slack connect external work back into Kandev tasks.
- **External MCP:** Kandev exposes streamable HTTP and SSE MCP endpoints so external coding agents can manage Kandev from outside the app. Settings include copyable snippets for Claude Code, Cursor, Codex, Auggie CLI, OpenCode, and GitHub Copilot CLI.
- **Automatic session MCP:** agents launched normally inside Kandev receive the Kandev task MCP automatically.
- **Passthrough MCP:** passthrough agents can also use the Kandev MCP endpoint while keeping their native CLI interface.

## What Task Agents Can Do Through Kandev MCP

Agents running on regular kanban tasks receive a task-scoped MCP surface. The available tools let agents coordinate work without leaving Kandev:

- **Discover workspace context:** list workspaces, workflows, workflow steps, repositories, agents, executor profiles, and tasks.
- **Work with multi-repo tasks:** task responses include attached repositories, base branches, checkout branches, and positions so agents can reason about each worktree separately.
- **Create tasks and subtasks:** create top-level tasks or child tasks, optionally auto-starting an agent. Subtasks inherit the parent workspace, workflow, agent profile, executor, and repositories unless the agent supplies an override.
- **Target sibling repositories:** create a subtask in a different repository while keeping it under the same parent task and workspace.
- **Coordinate dependencies:** create tasks with blocker relationships and inspect related parent, child, sibling, blocker, and blocked-by tasks.
- **Attach more branches:** add another `(repository, branch)` worktree to an existing worktree task. Use this for multiple PRs from one task, either in the same repository or across repositories.
- **Adjust diff bases:** update a task repository's base branch so changes, ahead/behind counts, and review context compare against the right target branch.
- **Move, archive, or delete tasks:** move tasks across workflow steps, including optional handoff prompts for the next agent, or clean up tasks when appropriate.
- **Message other tasks:** send a prompt to another task's primary session. Running tasks queue the message for the next turn; idle tasks receive it immediately; created sessions start with it.
- **Read conversations:** fetch a task's conversation history, with pagination and message-type filters.
- **Record structured plans:** create, read, update, and delete task plans.
- **Signal completion:** explicitly tell Kandev that a workflow step is complete, with summary, handoff notes, and blockers.
- **See associated PRs:** task-listing and related-task responses include associated GitHub pull requests when available, including PR number, URL, title, state, and merge time.

## In Progress: Office Mode

Office mode is currently feature-flagged and not documented as a live supported feature yet. The planned direction is a higher-level autonomy workspace with:

- Persistent agent instances with roles, permissions, skills, instructions, memory, status, and executor preferences.
- Agent dashboards, run history, inbox items, approvals, and human review gates.
- Task delegation across agent teams, including parent/child task coordination.
- Routines and scheduler-driven recurring work.
- Cost tracking, model usage visibility, and budget guardrails.
- Workspace configuration import/export and sync for agent/team setup.

We'll move Office into the live feature inventory after it ships.

## System Management

- **Feature toggles:** runtime feature flags can be viewed and overridden from Settings > System, with restart prompts when required.
- **Updates:** Kandev can check for newer releases and, in supported service installs, apply an update through the UI.
- **Backups and restore:** Kandev creates snapshots before version upgrades and supports manual snapshots, download, restore, and deletion from the system settings page.
- **Disk usage:** inspect storage used by worktrees, repositories, sessions, tasks, quick chat, and backups.
- **Logs, database status, about, and licenses:** system settings include operational views useful for self-hosted installs.
