---
title: "Get Started"
description: "Install Kandev, connect a repository, run a first agent task, and review the result."
---

# Get Started with Kandev

Kandev coordinates coding-agent work around real repositories, branches, tasks, and review state. The fastest useful setup is one local repository, one agent profile, and the built-in Kanban workflow.

## Prerequisites

- Git and access to the repository you want to use.
- Credentials for at least one supported coding agent. The agent CLI must be installed or installable by the command shown in **Settings > Agents**.
- Node.js/npm only when using the npm or `npx` distribution. The desktop and Homebrew bundles include the Kandev runtime.
- Docker only when you choose a Docker executor.

## Install and start

Choose one channel:

```bash
# macOS or Linux with Homebrew
brew install kdlbs/kandev/kandev
kandev

# One-off latest npm release
npx kandev@latest

# Global npm install
npm install -g kandev@latest
kandev
```

Kandev starts its backend, serves the web UI, and opens it in your browser. By default, persistent data lives under `~/.kandev`; the exact paths and startup options are in the [CLI reference](cli.md).

The [desktop app](desktop-app.md) is another local entry point. For an always-on or remote installation, see [Run as a service](run-as-a-service.md), [Docker](docker.md), or [Remote environments](remote-cloud-environment.md).

## Complete the initial setup

1. Open **Settings > Workspaces** and create or select a workspace. A workspace owns repositories, workflows, integrations, labels, and task history.
2. Under the workspace, add a repository by local path or remote URL. Confirm the default branch and authentication before starting work.
3. Open **Settings > Agents**. Select an installed agent and create or inspect a profile. Choose its model, mode, CLI flags, environment variables, passthrough behavior, and permission policy deliberately.
4. Open **Settings > Executors**. For the first run, use a worktree profile when you want branch isolation or a local profile when you explicitly want the agent to work in the selected checkout.
5. Keep the built-in **Kanban** workflow for the first task. Use [Workflow tips](workflow-tips.md) after the basic loop is familiar.

## Run a first task

1. Select **New task** from the board or command panel.
2. Give the task a concrete outcome and enough acceptance criteria to review it.
3. Select the repository and base branch. Kandev creates or uses the execution workspace required by the selected executor.
4. Select a workflow, agent profile, executor, and executor profile. The dialog remembers compatible recent choices, but review them when changing repositories or trust boundaries.
5. Create and start the task. Workflow events can move it to the running step and launch the agent automatically.
6. Follow the session in the integrated workbench. Inspect chat, terminal commands, files, and the live Changes panel rather than relying only on the final message.
7. Review the diff. Add line, plan, pull-request, or walkthrough feedback to the next prompt when changes are needed.
8. Run the repository's tests, create or inspect commits, and open the pull request from the task when the result is ready.
9. Move the task through its review gate and archive it only after the branch and pull request state are understood.

## Choose the right workflow

- **Kanban** is the shortest run-review loop.
- **Plan & Build** pauses after a structured plan so a human can edit or approve the approach before implementation.
- **Feature Dev** separates planning, implementation, review, QA, PR creation, and CI repair into focused steps.
- **PR Review** gives an existing branch or pull request a dedicated review pass.
- **Architecture** produces a design without implying that implementation is complete.

Workflow steps can select different agent profiles and prompts. See [Tasks and workflows](tasks-and-workflows.md) for the operating model and [Workflow import/export](workflow-import-export.md) for portable definitions.

## Credentials and trust

Kandev does not make an agent safer than the permissions of its process. An agent can use the files, network, credentials, environment variables, MCP tools, and approval policy exposed by its selected profiles and executor.

- Prefer scoped tokens and secret references over plaintext environment values.
- Treat **Auto approve** and permission-skipping CLI flags as trust-boundary changes, not convenience toggles.
- A local executor inherits more of the host environment than an isolated container or remote sandbox.
- A worktree isolates Git changes from other Kandev tasks, but it is not an operating-system security boundary.
- Review prepare and cleanup scripts before sharing an executor profile with a team.

See [Agents and profiles](agents-and-profiles.md), [Executors](executors.md), and [Configuration](configuration.md) before operating Kandev on shared infrastructure.

## If the first task does not start

Check these in order:

1. **Settings > System > Status** for backend, database, filesystem, and runtime health.
2. The agent profile's install/login status and model selection.
3. Repository access by running `git fetch` with the same user or remote credentials.
4. Executor-profile prerequisites such as Docker, SSH connectivity, a Sprites token, image availability, or prepare-script success.
5. The session and system logs under **Settings > System > Logs**.
6. Disk usage under system settings; worktrees, repositories, sessions, and backups all consume the Kandev data directory.

The [operations guide](operations.md) covers backups, updates, logs, resource metrics, and recovery in more detail.
