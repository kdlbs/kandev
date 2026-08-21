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
