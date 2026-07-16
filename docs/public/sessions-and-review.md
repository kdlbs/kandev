---
title: "Sessions and Review"
description: "Work with agent chat, files, terminals, diffs, plans, pull requests, and code walkthroughs in one task."
---

# Sessions and Review

A Kandev session is an agent conversation attached to a task and its execution environment. The workbench keeps conversation, repository state, commands, files, and review feedback visible together so a final agent message is never the only evidence of completion.

## Start and resume sessions

The first workflow action can create a primary session automatically. You can also start another session from task detail and choose a compatible agent profile, model, mode, and executor context.

Structured ACP sessions can resume when the agent/runtime provides a resume identifier. SSH and Sprites environments are treated as resumable runtimes at the environment level. A stopped process, removed worktree, expired cloud environment, or agent that does not support session resume can still require a fresh turn.

Multiple sessions on one task can share its task environment. They also share the underlying files, so do not run two writers concurrently unless the prompts and file ownership make that safe.

CLI passthrough is different from structured chat: Kandev hosts the agent's native terminal interface in a PTY. It retains task/worktree tracking and can expose task MCP, but message rendering and resume behavior depend on the CLI.

## Workbench surfaces

Desktop task detail can combine these panels:

- **Agent chat:** prompts, tool calls, permission requests, clarification questions, todos, usage, and session status.
- **Files and editor:** browse, search, open, edit, and inspect text or image files. Language servers can be configured under Editors settings.
- **Terminal:** run repository commands in the task environment; remote/container executors route the shell through agentctl.
- **Changes:** inspect working-tree, staged, commit, branch, and pull-request diffs, grouped by repository for multi-repo tasks.
- **Plan/documents:** read and edit the task's structured plan and other documents.
- **Preview/browser:** open a forwarded application port when the executor and task expose one.
- **Pull request:** inspect linked PR state and review feedback when the integration is available.

Panel presets and layout are user preferences. Mobile uses task-native sheets and bottom navigation for sessions, files, terminal, and changes instead of shrinking the desktop dock layout.

## Give review feedback with context

Feedback can be attached to:

- changed lines in a diff;
- selected editor content or files;
- plan text;
- pull-request review comments;
- a code-walkthrough step;
- images or file attachments.

Kandev formats those context items into the next message so the agent sees the relevant path, line range, source text, and comment. Review the composed prompt before sending; stale line numbers or a changed diff base can make old comments ambiguous.

## Plan before implementation

Plan mode asks the agent to analyze and record an implementation plan rather than modify the repository immediately. In workflows such as Plan & Build or Feature Dev, the plan is a shared task document and the next step can be blocked on human review.

Use plan mode when requirements, architecture, migrations, or cross-repository ownership need agreement. It is not a sandbox: the selected agent still has the executor permissions configured for that session, so permission policy remains relevant.

## Request a code walkthrough

The Changes and expanded review surfaces can ask the agent to create an anchored walkthrough of the current change. Kandev sends the built-in `changes-walkthrough` prompt, and an agent with task MCP calls `show_walkthrough_kandev` with a title and ordered file/range steps.

When the walkthrough arrives:

1. Open the walkthrough launcher.
2. Move through the ordered steps.
3. Kandev opens the referenced file and highlights the anchored range when it can match the path.
4. Add feedback to an individual step; the next agent message includes the walkthrough explanation and anchor context.
5. Close the walkthrough to keep it available, or discard it after confirmation to remove the persisted tour.

The mobile UI presents the same walkthrough as a bottom sheet with an anchored editor range. A walkthrough is agent-authored explanation, not proof that the change is correct; validate the diff and tests separately.

## Review Git and pull-request state

The Changes panel can issue pull, push, rebase, merge, and abort operations through agentctl. These operations run in the task workspace and are serialized per environment. Read [Git operations](git-operations.md) before resolving conflicts or rewriting branch history.

For a multi-repository task, select the correct repository before committing, pushing, or opening a pull request. Each repository has its own base, task branch, diff, and PR association.

Before marking a task done:

- inspect all changed and untracked files;
- run the project's required checks in the task environment;
- confirm the diff base and branch are correct;
- review agent-generated commit and PR text;
- check linked CI and review feedback;
- keep human approval outside the agent when policy requires it.

## Common failures

- **No terminal or file data:** the environment may still be preparing, agentctl may be unavailable, or the task has no repository.
- **Resume starts fresh:** the agent or runtime did not retain a usable resume token, or its environment was removed.
- **Changes are empty:** select the right repository and diff target, then confirm the agent wrote inside the materialized task path.
- **Preview does not load:** start the application on a reachable interface, confirm the port, and create/inspect the forward for remote executors.
- **Walkthrough does not appear:** the agent must have task MCP, call the walkthrough tool successfully, and reference files Kandev can match.
- **Permission prompt blocks the turn:** answer it in chat, or change the profile policy only after assessing the command and trust boundary.
