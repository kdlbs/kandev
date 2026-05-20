---
name: kandev-tasks
description: List, move, archive, and message tasks via agentctl
kandev:
  system: true
  version: "0.42.0"
  default_for_roles: [ceo, worker, specialist]
---

# Tasks — the bulk operations

The singular `agentctl kandev task` group handles a *single* task (the one you're working on). This plural group handles tasks across the workspace.

## List

```bash
# Everything
$KANDEV_CLI kandev tasks list

# Filtered
$KANDEV_CLI kandev tasks list --status todo
$KANDEV_CLI kandev tasks list --assignee A-1
$KANDEV_CLI kandev tasks list --project P-12
```

## Move between workflow steps

```bash
$KANDEV_CLI kandev tasks move --id T-1 --step ws-review
$KANDEV_CLI kandev tasks move --id T-1 --step ws-review --prompt "Please review this PR; gating on QA."
```

The optional `--prompt` is queued as the first comment the destination step sees — use it for handoff context.

## Archive

```bash
$KANDEV_CLI kandev tasks archive --id T-1
```

Idempotent: archiving an already-archived task is a no-op.

## Send a message to a task (without claiming it)

```bash
$KANDEV_CLI kandev tasks message --id T-1 --prompt "Heads up: design changed, see comment #42."
```

The message lands as an agent comment on the task. The assignee's next heartbeat will see it.

## Read the conversation

```bash
$KANDEV_CLI kandev tasks conversation --id T-1
```

Use sparingly — comments can be long. Prefer the inline batch on comment-driven wakes.
