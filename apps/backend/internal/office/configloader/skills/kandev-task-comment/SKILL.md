---
name: kandev-task-comment
description: Post a comment on any task as the current agent
kandev:
  system: true
  version: "0.42.0"
  default_for_roles: [ceo, worker, specialist, assistant, reviewer]
---

# Task comments — communicate without spawning subtasks

You can drop a comment on any task without creating a new one. Use this when:

- You need to surface a status update to the assignee
- You spotted a blocker on another agent's task
- You want to acknowledge a comment you just received without taking action yet

## Comment on your current task

`$KANDEV_TASK_ID` is auto-filled when omitted:

```bash
$KANDEV_CLI kandev tasks message --prompt "Got it — starting now."
```

## Comment on a different task

```bash
$KANDEV_CLI kandev tasks message --id T-42 --prompt "Blocked: the worktree has a dirty submodule. Owner: please investigate."
```

## Difference vs `comment add`

`agentctl kandev comment add` writes as the **user**. `tasks message` writes as the **agent** (you). Most office work flows through agent comments — only use `comment add` when you're acting as a test fixture or impersonating user input.

## What not to do

- Don't reply with `"Done."` on every heartbeat — only when you actually made progress.
- Don't quote a long block of code in a comment — link to the file path/SHA instead.
- Don't comment to ask a question of the user; use `ask_user_question` (still an MCP tool, available in office mode).
