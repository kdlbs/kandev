# Agent Comments

Comment on your current task:

```bash
$KANDEV_CLI kandev tasks message --prompt "Got it - starting now."
```

Comment on another task:

```bash
$KANDEV_CLI kandev tasks message --id T-42 --prompt "Blocked: the worktree has a dirty submodule. Owner: please investigate."
```

Both `tasks message` and `comment add` write as the current agent.

Avoid empty acknowledgements such as `Done.` unless they include useful progress. Link to file paths or commits instead of quoting long code blocks.

Read a task's comments (all authors, agent and human) with the `list_task_comments_kandev` MCP tool. Optional `task_id` defaults to the current task; optional `limit` defaults to 20 (max 100). Use it to check whether a related task (parent, child, etc.) already posted the deliverable you're waiting on before proceeding.
