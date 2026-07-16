---
title: "Coordinate Work"
description: "Split work across subtasks, dependencies, repositories, branches, plans, messages, and pull requests."
---

# Coordinate Work

Kandev supports both human-guided workflows and coordinator-led agent patterns. Both use the same task model: explicit repository scope, workflow state, profiles, dependencies, messages, and review surfaces.

## Subtasks

A child task is appropriate when a parent goal contains work that should have its own conversation, status, branch, or pull request.

By default, a subtask inherits the parent task's:

- workspace and workflow;
- agent and executor profiles;
- repository attachments and base branches;
- materialized task workspace when `workspace_mode` is `inherit_parent`.

Use inherited workspace mode when the child is a focused phase or collaborator that must see the same uncommitted files. Concurrent writers can conflict because they are editing the same worktrees.

Use a new materialized workspace when the child needs independent branches and filesystem state. A coordinator can also target a sibling repository explicitly so a frontend or infrastructure subtask stays under one parent while working in another repo.

## Dependencies

Blocker relationships express order independently of the board column. Use them when a task cannot produce a valid result until another task finishes. Related-task views and task MCP expose parent, child, sibling, blocker, and blocked-by context.

A dependency is coordination metadata, not a merge queue. The receiving task still needs to fetch, rebase, cherry-pick, or otherwise consume the prerequisite code according to the repository workflow.

## Multiple repositories and branches

One task can attach several repositories. Kandev materializes one workspace per attachment and groups files, diffs, review, and pull requests by repository.

One task can also attach another branch from the same repository. Use that when the task intentionally produces separate PRs or needs to compare/work across branch lines. Each attachment records its own base branch; changing a diff base changes review context and ahead/behind calculations, not the actual commits.

Before an agent works across attachments, state:

- which repository owns each deliverable;
- whether worktrees are shared or isolated;
- the expected base and PR target for every branch;
- the order in which dependent PRs should merge;
- which tests must run in each repository.

## Shared plans and handoffs

Task plans are versioned documents that humans and agents can edit. A coordinator can create or update the parent plan, then give each child a scoped description and file-ownership boundary.

Handoffs can move a task to another workflow step with a prompt for the next agent. Keep the handoff factual: completed work, current branch/commit, tests run, known failures, required inputs, and files that must not be overwritten.

Do not rely on conversation summaries as the only durable state. Put decisions in the plan or repository and make commits before switching isolated branches.

## Cross-task messages and conversations

Task MCP can send a message to another task's primary session and read task conversations with pagination and message-type filters. A running task queues the message for a subsequent turn; an idle task can receive it immediately.

Use messages for bounded requests or status, not as an implicit shared-memory bus. Include the task/repository/branch identifiers and the expected response. Avoid sending secrets or large source dumps through cross-task messages.

## Coordinator-led pattern

A typical coordinator flow is:

1. Inspect workspace, workflow, repositories, available profiles, and related tasks.
2. Record a plan with clear deliverables and ownership.
3. Create child tasks with the correct repository and workspace mode.
4. Add dependencies where order matters.
5. Monitor conversations and associated PR state.
6. Ask blocked children for concrete status or provide missing context.
7. Integrate and verify each branch in dependency order.
8. Signal workflow completion with a summary, handoff, and blockers.

The coordinator has only the tools and permissions exposed to its selected profile and Kandev task MCP. It does not bypass branch protection, GitHub approval, executor credentials, or human workflow gates.

## Human-guided pattern

The same primitives support a more controlled process:

- a human creates and scopes each task;
- Plan or Review steps pause for explicit approval;
- agents can propose subtasks but a human reviews their scope and profile;
- protected credentials stay outside broad agent profiles;
- a human decides what merges and what ships.

Choose the level of autonomy per workflow and repository risk. The task model does not force one operating style.

## Limits and safety

- Shared workspaces do not prevent file conflicts between agents.
- Separate worktrees do not isolate host credentials or processes unless the executor does.
- Cross-repository work still needs compatible credentials for every remote.
- Associated PR data depends on configured GitHub/GitLab access and branch association.
- Office mode is a separate feature-flagged autonomy workspace; do not assume its persistent teams, budgets, or routines are part of regular task MCP coordination.

Related: [Automation and MCP](automation-and-mcp.md), [Tasks and workflows](tasks-and-workflows.md), and [Sessions and review](sessions-and-review.md).
