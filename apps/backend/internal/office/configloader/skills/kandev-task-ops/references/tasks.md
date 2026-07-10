# Workspace Task Operations

List tasks:

```bash
$KANDEV_CLI kandev tasks list
$KANDEV_CLI kandev tasks list --status todo
$KANDEV_CLI kandev tasks list --assignee A-1
$KANDEV_CLI kandev tasks list --project P-12
```

Move between workflow steps:

```bash
$KANDEV_CLI kandev tasks move --id T-1 --step ws-review
$KANDEV_CLI kandev tasks move --id T-1 --step ws-review --prompt "Please review this PR; gating on QA."
```

The optional `--prompt` is queued as handoff context.

Archive a task:

```bash
$KANDEV_CLI kandev tasks archive --id T-1
```

Read a task conversation:

```bash
$KANDEV_CLI kandev tasks conversation --id T-1
```

Use conversation reads sparingly because comments can be long. Prefer the wake payload when it contains the needed context.
