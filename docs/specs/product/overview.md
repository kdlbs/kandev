# Product overview

**Status:** Proposed baseline for product review.

This document describes Kandev-wide product context. It does not replace the
requirements owned by the systems listed in the [specification catalog](../INDEX.md).

## Product purpose

Kandev is a server-first development workbench. It helps a developer assign
repository work to coding agents, provide a controlled execution environment,
inspect the work as it happens, and decide what is safe to keep, merge, or
publish.

The product coordinates the work around an agent. It does not try to replace
the agent CLI, Git, the code host, the user's editor, or the user's judgment.

## The problem Kandev solves

Agent-assisted development crosses several boundaries at once:

- work needs a durable task, repository, and workflow context;
- agents need profiles, credentials, models, tools, and execution environments;
- different providers and executors expose different capabilities and risks;
- users need to see progress, steer an agent, inspect files and commands, and
  review changes before an irreversible action; and
- restarts, missed events, provider failures, and cleanup must not lose work.

Kandev gives these concerns one product flow while keeping their ownership
separate. Durable work belongs to tasks and workspaces. Agent identity belongs
to profiles. Process isolation belongs to executors. External service behavior
belongs to integrations. Presentation belongs to the UI and desktop surfaces.

## Current product boundary

The supported product path is the regular Kanban workbench:

1. Create or select a workspace and attach repositories.
2. Configure an agent profile and an executor profile appropriate for the trust
   boundary.
3. Create a task with a clear outcome, repository context, and workflow.
4. Start one or more agent sessions and provide direction when needed.
5. Inspect the agent's files, terminal activity, plan, and changes.
6. Test and review the result, then commit, open a pull request, or continue
   the task under human control.

The same backend serves the browser UI, the Tauri desktop shell, the native
CLI, HTTP and WebSocket APIs, and task-scoped MCP tools. These are different
surfaces over the same product authority, not separate products.

See [Tasks](../tasks/README.md), [Workspaces](../workspaces/README.md),
[Agents](../agents/README.md), [Executors](../executors/README.md), and the
[public product documentation](../../public/index.md).

## Product qualities

Kandev is successful when it makes agent work:

- **Understandable:** the task, workflow position, active session, and changes
  are visible and attributable.
- **Reviewable:** the user can inspect the work before merge, release, deploy,
  or another irreversible action.
- **Contained:** credentials, repositories, commands, and network access match
  the selected profile and executor boundary.
- **Recoverable:** restart, reconnect, provider failure, or cleanup does not
  silently destroy task work or workspace state.
- **Interoperable:** provider, agent, executor, integration, and plugin
  differences are handled at explicit boundaries.
- **Accessible:** desktop and mobile surfaces preserve the same core capability,
  and product copy is localized where the UI exposes it.

## Explicit non-goals

Kandev is not currently:

- a general-purpose source hosting or CI/CD platform;
- a replacement for Git, a code-host review system, or an agent provider;
- a guarantee that an executor removes all permissions from an agent;
- a multi-user identity and role platform; or
- a supported autonomous software team in the production runtime.

Office contains the evolving autonomy, routines, budgets, coordinator, and
team-oriented work. It is feature-flagged, disabled in the production profile,
and remains separate from the supported regular Kanban contract. See [feature
status](../../public/feature-status.md).

## Open product questions

- Which product capabilities should receive explicit adoption and reliability
  targets?
- What is the intended future boundary for multi-user identity and roles?
- What evidence is required before Office becomes part of the supported product
  path?
- Which providers, integrations, and executor types are release commitments
  versus dependency-bound capabilities?
