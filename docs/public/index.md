---
title: "Kandev Documentation"
description: "Choose a guide for installing Kandev, running agent work, reviewing changes, or contributing to Kandev."
---

# Kandev Documentation

Kandev lets you assign repository work to coding agents, review the result, and keep control of the environment and credentials.

## Start here

1. [Get started](use-kandev.md) — install Kandev, add a repository, and run a first task.
2. [Tasks and workflows](tasks-and-workflows.md) — create work, choose a workflow, and use plans.
3. [Sessions and review](sessions-and-review.md) — supervise an agent, inspect changes, and finish safely.
4. [Security and trust](security.md) — choose an appropriate access boundary before sharing Kandev with others.

<details>
<summary>More guides for running work</summary>

### Run work in Kandev

Choose the guide that matches the job in front of you:

- [Get started](use-kandev.md): install Kandev, add a local repository, configure an agent, and run a first task.
- [Tasks and workflows](tasks-and-workflows.md): create tasks, use plans, configure workflow steps, and understand the Office-only document and label boundary.
- [Sessions and review](sessions-and-review.md): work with named sessions, chat, files, terminal, changes, preview, pull requests, and walkthroughs.
- [Coordinate work](coordination.md): use parallel sessions, targeted messages, subtasks, dependencies, multiple repositories, and additional branches.
- [Agent communication](agent-communication.md): send cross-task prompts, receive replies, and negotiate contracts with built-in Kandev MCP tools.
- [Agents and profiles](agents-and-profiles.md): configure agent CLIs, models, modes, permissions, environment, passthrough, and credentials.
- [Executors](executors.md): choose local, worktree, Docker, SSH, or Sprites execution and understand the isolation boundary.
- [Integrations](integrations.md): configure Azure DevOps, GitHub, GitLab, Jira, Linear, Sentry, and Slack for a workspace.
- [Developer tools](developer-tools.md): use quick chat, prompts, utility agents, voice input, editors, terminal, shortcuts, and notifications.
- [Automation and MCP](automation-and-mcp.md): create scheduled or event-driven work, use task MCP, and connect an external MCP client.
- [Feature status](feature-status.md): check support boundaries, dependencies, experimental features, and unfinished work.
- [Security and trust](security.md): choose a safe deployment boundary, constrain agent access, protect credentials, and preserve human review.

Installation and operation: [CLI](cli.md), [desktop app](desktop-app.md), [configuration](configuration.md), [service](run-as-a-service.md), [Docker](docker.md), [Kubernetes](k8s.md), and [operations](operations.md).

</details>

<details>
<summary>Contributor guides</summary>

## Contribute to Kandev

These pages describe the current source tree and its development conventions:

- [Contribution workflow](contributing.md): prepare a development environment and submit a focused change.
- [Architecture](architecture.md): understand the Go backend, embedded web app, persistence, events, agent runtime, and external boundaries.
- [Backend development](backend-development.md): follow handler, service, repository, event, migration, and WebSocket conventions.
- [Web development](web-development.md): work with the React app, state, API clients, WebSocket handlers, responsive task UI, and design system.
- [Testing](testing.md): choose targeted Go, Vitest, CLI, desktop, or Playwright coverage.
- [Remote cloud development](remote-cloud-environment.md): bootstrap and test the Kandev source tree in an ephemeral VM or cloud workspace.
- [Extension guides](extending-kandev.md): add agents, executors, integrations, settings, MCP tools, prompts, and workflow behavior.
- [Adding an agent CLI](add-agent-cli.md): implement an ACP or TUI integration end to end.
- [Release process](release-process.md): understand versioning and the CLI, runtime, desktop, npm, Homebrew, and container release paths.
- [Public docs guide](README.md): update or add a published page.

</details>

## Supported product boundary

The regular Kanban workbench, task sessions, review surfaces, and task-scoped Kandev MCP are the supported product path. Some capabilities still depend on a particular agent, executor, provider, platform, credential, or install channel; those differences are called out on their topic pages.

**Office is separate, disabled in the production runtime profile, feature-flagged, and still in progress.** Its source, tests, specifications, and internal plans do not make persistent teams, routines, budgets, or coordinator-led autonomy a supported production contract. See [Feature status](feature-status.md) for that boundary and [Office provider routing](office-provider-routing.md) for the feature-flagged routing surface.

<details>
<summary>Version matching</summary>

## Match docs to the running version

These docs describe the current `main` branch and are not versioned. A released build can lag behind them. Check **Settings > System > About** or run `kandev --version`, then use the matching GitHub tag when exact historical behavior matters.

</details>
