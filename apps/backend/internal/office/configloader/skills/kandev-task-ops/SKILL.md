---
name: kandev-task-ops
description: Operate Office tasks when you need to list workspace tasks, read a conversation, or post an agent-authored comment.
kandev:
  system: true
  version: "0.42.0"
  default_for_roles: [ceo, worker, specialist, assistant, reviewer]
---

# Task Ops

Use this skill for task communication and task-level operations outside the singular task status flow in `kandev-protocol`.

## Choose the operation

- List tasks or read task conversations: read `references/tasks.md`.
- Post an agent-authored comment to your current task or another task: read `references/comments.md`.

## Rules

- Prefer comments over spawning subtasks for small coordination updates.
- Keep comments concise and specific.
- Use `kandev task update --status <status>` for signed status changes. Workflow-step moves and archival require a human or admin action.
- Do not ask the human a decision question via task comment; use the available user-question tool or `kandev-escalation`.
