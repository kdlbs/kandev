---
title: "Automation and MCP"
description: "Automate task creation and let agents coordinate Kandev through task-scoped and external MCP endpoints."
---

# Automation and MCP

Kandev has two automation layers and two MCP contexts. Keep their scopes distinct:

- workflow events act on a task already moving through a workflow;
- workspace automations create work from schedules, provider events, or webhooks;
- task MCP lets an active coding agent coordinate its task and related work;
- external MCP lets a separate client control Kandev through the backend.

## Workflow events

Workflow-step events can react when a task enters a step, a user sends a message, or an agent turn completes. Depending on the workflow, actions can start an agent, send a configured prompt, stop work, or move the task.

Use them for predictable task-local transitions such as Plan to human approval to Implement to Review. Inspect all events on both source and destination steps before enabling auto-start behavior so a move does not create an accidental loop.

## Workspace automations

**Settings > Automations** and the workspace automation editor define rules that create task-backed or run-only executions. An automation selects a workspace, workflow and step, repository, agent profile, executor profile, prompt, title template, concurrency limit, and one or more triggers.

Current trigger types are:

| Trigger | Filters or configuration |
|---|---|
| Schedule | Cron expression and optional timezone. |
| GitHub pull request | Events, repositories, base branches, authors, labels, and draft exclusion. |
| GitHub push | Repositories and branch globs. |
| GitHub CI | Repositories, conclusions, and optional check names. |
| Webhook | Signed endpoint with an optional filter expression. |

Execution mode **Task** creates visible kanban work. **Run** uses the same session pipeline for a lighter, hidden execution whose result appears in automation run history. Concurrency and deduplication reduce duplicate work, but they do not make external actions idempotent by themselves.

Start with a narrow repository and read-only prompt. Inspect run history and generated tasks before enabling write credentials or widening filters.

## Task MCP

Kandev injects a task-aware MCP server into compatible ACP and passthrough sessions. It gives the agent structured coordination tools instead of requiring it to infer board state from prose.

Depending on the current task and policy, tools cover:

- listing workspaces, workflows, steps, profiles, executors, and related tasks;
- creating and updating tasks or subtasks;
- moving, archiving, or deleting tasks;
- attaching sibling repositories or additional branches;
- adding dependencies and selecting a diff base;
- sending cross-task messages and reading conversations;
- creating and updating task plans/documents;
- asking the user a structured clarification question;
- creating a code walkthrough;
- inspecting associated pull requests and signalling step completion.

The server injects calling-task identity for operations that require it. Tools still enforce task relationships, workspace rules, executor constraints, and normal provider permissions. An MCP call does not bypass GitHub branch protection or human approval.

## Profile and executor MCP policy

An agent profile can add external MCP servers. An executor profile can restrict allowed transports and destinations. Supported transport forms include stdio, SSE, HTTP, and streamable HTTP where the selected agent supports them.

Passthrough agents load MCP differently. Kandev may pass command-line configuration or merge a project-local MCP file for the specific CLI. Inspect the profile UI before assuming that an arbitrary TUI receives task tools.

## External MCP

The Kandev backend exposes a streamable HTTP endpoint at:

```text
http://<kandev-host>:<backend-port>/mcp
```

SSE compatibility endpoints are available at `/mcp/sse` and `/mcp/message`. **Settings > External MCP** provides client-specific configuration snippets.

External MCP is useful when an agent outside a Kandev task needs to inspect configuration or create and coordinate Kandev work. It runs against the backend's own reachability and credentials, not a task worktree.

## Security boundary

The external MCP endpoint currently has no Kandev user-authentication boundary. Anything that can reach the backend endpoint can attempt to use its exposed tools.

- Bind Kandev to loopback for a single-user local install.
- For remote access, place it behind a VPN, firewall, or authenticated reverse proxy that also supports long-lived streaming requests.
- Do not publish `/mcp`, `/mcp/sse`, or the backend port directly to the public internet.
- Use a separate network segment and scoped provider credentials for unattended automation.
- Review reverse-proxy logs and automation run history for unexpected clients or task creation.

The agentctl-local MCP server used inside a task is a different endpoint and is protected by the task runtime's control channel. Do not expose agentctl ports either.

## Operating checklist

Before enabling unattended automation:

1. Limit trigger repositories, branches, authors, labels, or issue query.
2. Use a dedicated agent and executor profile with minimal credentials.
3. Set a conservative concurrency limit.
4. Keep a human gate before merge, release, or production actions.
5. Test duplicate events and failed retries.
6. Confirm logs and run history contain enough context to audit an execution.
7. Verify the external MCP endpoint is unreachable outside the intended network.

Related: [Tasks and workflows](tasks-and-workflows.md), [Coordination](coordination.md), [Agents and profiles](agents-and-profiles.md), and [Integrations](integrations.md).
