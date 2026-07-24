# Workspace Task Operations

List tasks:

```bash
$KANDEV_CLI kandev tasks list
$KANDEV_CLI kandev tasks list --status todo
$KANDEV_CLI kandev tasks list --assignee A-1
$KANDEV_CLI kandev tasks list --project P-12
```

Read a task conversation:

```bash
$KANDEV_CLI kandev tasks conversation --id T-1
```

Use conversation reads sparingly because comments can be long. Prefer the wake payload when it contains the needed context.

Use `kandev task update --status <status>` for signed status changes. Office agents cannot move tasks between workflow steps or archive tasks; ask a human or admin for those actions.
