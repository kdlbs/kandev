---
title: Built-in MCP packages
description: Choose Kandev's native tool packages for a workspace, task, or automation and check what a session actually received.
---

# Choose built-in MCP packages

Use built-in packages to give a task the native Kandev tools it needs. You need
permission to edit the workspace policy or the task choices. Agents cannot grant
these packages to themselves.

## Choose packages for a task

1. Open the MCP button beside the task's chat composer, then **Customize**.
2. Under **Built-in packages**, set a package switch to **On** or **Off**.
3. To remove a task override, select **Use inherited value** below that package.
4. Select **Save changes**.

You can toggle packages directly in the list. Select a package name to inspect its tools and details.

Package rows show their effective On or Off value and whether they inherit
workspace policy. The source note shows the workspace value. Changing the switch creates a task override.
**Use inherited value** removes that override, so later workspace defaults apply again.
Changed choices use the settings change highlight. The compact
save bar appears while edits are unsaved; its **Reset** restores saved choices.
**Reset all packages to defaults** stages a separate change that still needs Save.

In **New Task**, expand **Advanced settings**, then open **MCP servers**.
Use the switches under **Built-in packages** to choose tools before the first start.
Those choices are saved as part of task creation. A rejected choice prevents that
creation from starting an agent.

Core is required. Configuration starts off for regular tasks; choosing **On**
and saving allows that task to use tools that can change shared Kandev settings
and resources. Each operation still needs its normal permissions.

| Package | Native server name | Regular-task default | Other eligible contexts |
| --- | --- | --- | --- |
| Core | `kandev` | Required | Required in Office tasks, configuration chats, and private automations |
| Configuration | `kandev-configuration` | Off; explicit human selection | Required in configuration chats; explicit selection in private automations |
| Rich Output | `kandev-rich-output` | On | On by default in Office tasks; explicit selection in private automations |
| Walkthroughs | `kandev-walkthroughs` | On | Explicit selection in private automations |
| Diagnostics | `kandev-diagnostics` | On | Explicit selection in private automations |

Private automations can select every optional package, including Configuration, in their native automation editor. Core remains required. Their choices apply at the next process start, while normal-task templates apply to newly created tasks. See [Choose MCP access](automation-and-mcp.md#choose-mcp-access).

Core supplies shared discovery, lifecycle, plans, questions, and coordination
tools. Regular tasks can discover visible executors without enabling Configuration.
Rich Output supplies native charts, comparisons, checks, and file previews. Walkthroughs
supplies guided explanations of changes. Diagnostics supplies diagnostic bundles.
Package detail identifies the package and its task choice. Expand the registered
tool list or **Package details** for its server name, ownership, and eligible
contexts. Profile capabilities can restrict the tools further.

## Set workspace limits and defaults

Open **Settings > Workspaces > your workspace > MCP servers > Configured**,
then use the switches in **Built-in packages**.

- Turn **Allow tasks to use this package** off to block it for regular tasks.
- Turn it on to permit explicit task selection. The package initially stays off by default.
- Turn **On by default** on to include it when a task inherits workspace policy.
  This switch is unavailable while the package is blocked.

Configuration cannot be on by default for regular tasks. Configuration chats
always receive Core and Configuration; workspace task policy does not change
that fixed set. Office supports Core and optional Rich Output.

Configuration chats and regular tasks use the same Configuration package. Selecting
it adds configuration tools without changing the task's identity or permissions.
Core tools remain subject to the caller's capabilities and resource access.

Use the shared **Save changes** bar to save workspace edits. Discard restores
the saved values. Reset restores the package defaults in the draft; save to apply
them. A conflict or denied write keeps the draft unsaved. Reload discards it and
loads current values. Reopening Customize refreshes saved choices when there are
no unsaved edits. An unsaved draft stays intact when the explorer reopens.

## Check when changes apply

Choices apply at the next normal agent-process start, across all normal sessions
of the task. Saving, reconnecting, or continuing a retained process does not
replace its accepted packages. Legacy retained processes keep their combined
Kandev server until their next normal start.

Open **Connections** and select a native server to compare **Next process start**
with **Current process accepted**. These are separate from connection status and
observed tools. A selected package is not proof of a healthy connection, and
acceptance is not proof that `tools/list` succeeded. Missing tool counts are
unknown, not zero. One package's evidence does not describe another package.

Connections also lists pending package additions and removals under **Next process
start**. Select a pending package to review its choice. An added package stays
separate from the current process's connections until a new process accepts it.
If the current agent's package set is unknown, the saved selections remain
visible with an Unknown explanation. They are not reported as new connections.

Walkthrough requests use the accepted packages while the agent process is live.
After the process stops, requests use the saved choices for its next start. An
unavailable runtime check leaves availability unknown.

If a Walkthrough request is unavailable, inspect its task choice and current
process acceptance. After changing the choice, start a new process. Refresh
Connections to check again. Existing walkthroughs remain readable; queued
requests retain their Walkthrough requirement and do not bypass it on delivery.

Kandev injects native packages separately from workspace MCP definitions. The
native names are reserved: an external definition cannot replace them. If an old
external entry uses one of these names, rename that entry before starting again.
Coordinator, managed-conversation, and external MCP surfaces retain their existing catalogs and authority.

See [Agents and profiles](agents-and-profiles.md) for external server selection,
and [Automation and MCP](automation-and-mcp.md) for tools and session evidence.
