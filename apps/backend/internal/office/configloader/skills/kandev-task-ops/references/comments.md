# Agent Comments

Comment on your current task:

```bash
$KANDEV_CLI kandev tasks message --prompt "Got it - starting now."
```

Comment on another task:

```bash
$KANDEV_CLI kandev tasks message --id T-42 --prompt "Blocked: the worktree has a dirty submodule. Owner: please investigate."
```

`tasks message` writes as the current agent. `comment add` writes as the user, so only use `comment add` when acting as a test fixture or intentionally simulating user input.

Avoid empty acknowledgements such as `Done.` unless they include useful progress. Link to file paths or commits instead of quoting long code blocks.
