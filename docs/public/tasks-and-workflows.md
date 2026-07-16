---
title: "Tasks and Workflows"
description: "Model development work with workspaces, workflows, steps, tasks, sessions, automations, and review gates."
---

# Tasks and Workflows

Kandev separates the thing being delivered from the process used to deliver it. A **task** holds the goal and repository context. A **workflow** defines the steps, prompts, events, and agent behavior that move that task from intake to completion.

## Core concepts

| Concept | Purpose |
|---|---|
| Workspace | Scope for repositories, workflows, integrations, labels, settings, and tasks. |
| Workflow | Reusable process made of ordered steps and transition rules. |
| Workflow step | Current process state, such as Todo, Plan, Work, Review, or Done. |
| Task | Goal, description, labels, repository/branch attachments, documents, dependencies, and sessions. |
| Task repository | One base branch and one materialized task branch/worktree within the task. A task can have several. |
| Session | One agent conversation or passthrough terminal run attached to the task. |
| Task environment | Executor-owned workspace that can be reused by multiple sessions for the same task. |

The board and pipeline are two views of the same workflow state. Moving a card changes its workflow step; it does not by itself prove that code was committed, pushed, reviewed, or merged.

## Create a task deliberately

The task dialog requires a title, workspace workflow, and repository selection unless **No repository** is chosen. Depending on the workflow and task kind, it also selects:

- starting workflow step;
- one or more repositories and base branches;
- agent and agent profile;
- executor and executor profile;
- initial prompt or description;
- labels and integration context.

Use **No repository** for research, planning, ticket triage, or other work that genuinely does not need a checkout. Repository-aware tools, Changes, branch operations, and pull-request creation are naturally unavailable in that mode.

For local repositories, Kandev checks branch state before materializing a fresh branch. Read the warning if the selected checkout is dirty or the requested base cannot be resolved. For remote repositories, the chosen credentials must be able to clone and fetch the base branch.

## Workflow events and automations

A step can react to events such as:

- a task entering the step;
- the user sending a message;
- an agent turn completing;
- a task or session reaching a relevant state.

Actions can start or stop an agent, send a configured prompt, move the task, or run a workflow automation. This is why a card can move without a manual drag and why sending feedback in Review can return a task to Work.

Review every step's events before enabling a workflow for a team. A loop that starts an agent on entry and moves back into the same start condition can spend compute repeatedly. Keep an explicit human gate where a person must inspect or approve the result.

For complete built-in examples, see [Workflow tips](workflow-tips.md). Workflows can be [exported and imported](workflow-import-export.md) or [synchronized from GitHub](workflow-sync.md).

## Plans and task documents

Tasks can hold multiple Markdown documents with revision history. Built-in document kinds include plans and reviews, and workflows or agents can create additional structured notes.

In a planning workflow:

1. The agent inspects the repository and records a plan through Kandev task MCP.
2. The plan appears as a task document rather than disappearing into chat history.
3. A human can edit the shared plan before implementation.
4. The implementation agent retrieves the current revision, including those edits.

Documents are coordination state, not executable guarantees. Confirm that the implementation and review still match the accepted plan.

## Labels, filters, and task lists

Workspace labels can classify tasks by area, urgency, owner, or any team convention. Labels are visible on cards and task detail and can be used with board/list filters.

The Tasks view supports sorting and grouping independently of the kanban board. Use it to find archived or cross-workflow work without changing task state.

## Sharing a task snapshot

Kandev can publish a redacted conversation snapshot as a secret GitHub Gist. The flow presents a preview before creation and lets you revoke an existing share.

"Secret" Gists are unlisted, not access-controlled. Anyone with the URL can read one. Inspect the preview for source code, credentials, customer data, repository names, and agent tool output before publishing. Revoke the share when its purpose is complete.

## Archive, unarchive, and delete

- **Archive** hides completed or parked work while preserving its task history.
- **Unarchive** restores the task. If its local branches no longer exist, the next session can start from the configured base rather than recreating deleted local state automatically.
- **Delete** removes task records and associated runtime state according to the confirmation flow. Use it for disposable tasks, not as a substitute for understanding the branch or pull request.

Kandev can warn before archive through the task-action preference in General settings. Keep that safeguard enabled when unfinished branches or open pull requests are common.

## Troubleshooting workflow behavior

- If no agent starts, inspect the step events and confirm the task has a compatible agent and executor profile.
- If the task moves unexpectedly, inspect every automation attached to the source and destination steps.
- If a workflow imported but profiles are unresolved, map its portable agent reference to an installed profile.
- If a synchronized workflow cannot update, read its warning; tasks in renamed or removed steps can block a destructive change.
- If an agent runs but cannot see the expected repository, inspect all task-repository attachments and the executor's materialized paths.

Related: [Sessions and review](sessions-and-review.md), [Coordination](coordination.md), and [Workflow import/export](workflow-import-export.md).
