---
title: Built-in MCP packages
description: Choose Kandev's native tool packages for a workspace or task and check what a session actually received.
---

# Choose built-in MCP packages

Use built-in packages to give a task the native Kandev tools it needs. You need
permission to edit the workspace policy or the task choices. Agents cannot grant
these packages to themselves.

## Choose packages for a task

1. Open the MCP button beside the task's chat composer, then **Customize**.
2. Open **Built-in** and select a package.
3. Choose **Inherit workspace policy**, **On**, or **Off**.
4. Select **Save task choices**.

In **New Task**, expand **Built-in** to choose packages before the first start.
Those choices are saved as part of task creation. A rejected choice prevents that
creation from starting an agent.

Core is required. Configuration starts off for regular tasks; choosing **On**
and saving allows that task to use tools that can change shared Kandev settings
and resources. Each operation still needs its normal permissions.

| Package | Native server name | Regular-task default | Other eligible contexts |
| --- | --- | --- | --- |
| Core | `kandev` | Required | Required in Office tasks and configuration chats |
| Configuration | `kandev-configuration` | Off; explicit human selection | Required in configuration chats |
| Rich Output | `kandev-rich-output` | On | On by default in Office tasks |
| Walkthroughs | `kandev-walkthroughs` | On | Regular tasks only |
| Diagnostics | `kandev-diagnostics` | On | Regular tasks only |

Core supplies lifecycle, plans, questions, and coordination tools. Rich Output
supplies native charts, comparisons, checks, and file previews. Walkthroughs
supplies guided explanations of changes. Diagnostics supplies diagnostic bundles.
The package detail lists registered tools and eligible contexts; profile
capabilities can restrict the tools further.

## Set workspace limits and defaults

Open **Settings > Workspaces > your workspace > MCP servers > Configured**,
then select a **Built-in** package.

- **Blocked** prevents regular tasks from selecting that package.
- **Allow explicit task selection** keeps it off unless a human chooses On for a task.
- **On by default** includes it when a task inherits workspace policy.

Configuration cannot be on by default for regular tasks. Configuration chats
always receive Core and Configuration; workspace task policy does not change
that fixed set. Office supports Core and optional Rich Output.

Use the shared **Save changes** bar to save workspace edits. Discard restores
the saved values. Reset restores the package defaults in the draft; save to apply
them. A conflict or denied write keeps the draft unsaved. Reload discards it and
loads current values.

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

If a Walkthrough request is unavailable, inspect its task choice and current
process acceptance. After changing the choice, start a new process. Refresh
Connections to check again. Existing walkthroughs remain readable; queued
requests retain their Walkthrough requirement and do not bypass it on delivery.

Kandev injects native packages separately from workspace MCP definitions. The
native names are reserved: an external definition cannot replace them. If an old
external entry uses one of these names, rename that entry before starting again.
Hidden automation, coordinator, managed-conversation, and external MCP surfaces
retain their separate contracts.

See [Agents and profiles](agents-and-profiles.md) for external server selection,
and [Automation and MCP](automation-and-mcp.md) for tools and session evidence.
