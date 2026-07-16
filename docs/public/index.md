---
title: "Kandev Documentation"
description: "Choose a guide for running agentic development work or contributing to Kandev itself."
---

# Kandev Documentation

Kandev is an open-source workbench for planning, running, coordinating, and reviewing software work with coding agents. It runs on infrastructure you control, works with multiple agent providers, and keeps the human review surface close to the repository state.

Choose the path that matches what you are doing now.

## Use Kandev

Start here when you want to install Kandev, connect repositories and tools, run agents, or design a team workflow.

- [Get started](use-kandev.md): install Kandev and complete a first task.
- [Tasks and workflows](tasks-and-workflows.md): choose a workflow, create tasks, and move work through gates.
- [Sessions and review](sessions-and-review.md): use chat, files, terminal, changes, plans, pull requests, and code walkthroughs.
- [Coordinate work](coordination.md): use subtasks, dependencies, multiple repositories, multiple branches, plans, and handoffs.
- [Agents and profiles](agents-and-profiles.md): select models and modes, configure CLI flags and environment, and understand permission controls.
- [Executors](executors.md): choose local, worktree, Docker, SSH, or Sprites execution.
- [Integrations](integrations.md): connect GitHub, GitLab, Jira, Linear, Sentry, and Slack.
- [Developer tools](developer-tools.md): quick chat, prompts, utility agents, voice, editors, terminal, shortcuts, and notifications.
- [Automation and MCP](automation-and-mcp.md): automate workflow transitions and let agents coordinate Kandev tasks.
- [Feature status](feature-status.md): see what is supported, optional, experimental, or still in progress.

For installation and deployment details, use the [CLI reference](cli.md), [desktop guide](desktop-app.md), [configuration reference](configuration.md), or one of the runtime guides under **Run and Operate**.

## Contribute to Kandev

Start here when you are changing Kandev, extending an integration, or maintaining its public documentation.

- [Contribution workflow](contributing.md): prepare a development environment and open a focused pull request.
- [Architecture](architecture.md): understand the unified Go backend, embedded web app, persistence, events, agent runtime, and external boundaries.
- [Backend development](backend-development.md): follow handler, service, repository, event, migration, and WebSocket conventions.
- [Web development](web-development.md): work with the Vite/React app, state slices, API clients, WebSocket handlers, responsive task UI, and design system.
- [Testing](testing.md): choose targeted Go, Vitest, CLI, desktop, or Playwright coverage and use isolated fixtures.
- [Extension guides](extending-kandev.md): add agents, executors, integrations, settings, MCP tools, prompts, and workflow behavior.
- [Adding an agent CLI](add-agent-cli.md): implement a new ACP or TUI integration end to end.
- [Release process](release-process.md): understand versioning, release automation, runtime bundles, desktop artifacts, npm, Homebrew, and containers.
- [Public docs guide](README.md): update a page or add a new entry to this navigation.

## Product Boundary

The regular task workbench and task-scoped Kandev MCP are shipped features. **Office mode is separate and remains feature-flagged and in progress.** Internal plans, ADRs, and experiments can explain direction, but they are not promises that a feature is generally available. The [feature status page](feature-status.md) records that boundary explicitly.

## Current Version

These docs describe the current `main` branch and are not versioned yet. Commands and configuration are verified against source, but a released build can lag `main`. Check the version in **Settings > System > About** or run `kandev --version`, then consult the corresponding GitHub tag when exact historical behavior matters.
