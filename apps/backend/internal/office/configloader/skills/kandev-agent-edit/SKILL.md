---
name: kandev-agent-edit
description: Edit or remove existing agents — rename, adjust budget, retire
kandev:
  system: true
  version: "0.42.0"
  default_for_roles: [ceo]
---

# Agent editing — adjust, rename, retire

You can edit an existing agent's settings or remove an agent that's no longer needed.

## Rename or re-icon

```bash
$KANDEV_CLI kandev agents update --id A-1 --name "Senior Reviewer"
```

## Adjust the monthly budget

```bash
$KANDEV_CLI kandev agents update --id A-1 --budget-monthly-cents 5000
```

Pass `0` to mean "unlimited" (subject to workspace policy). Pass `-1` and the flag is ignored (allows scripted updates with optional fields).

## Cap concurrent sessions

```bash
$KANDEV_CLI kandev agents update --id A-1 --max-concurrent-sessions 1
```

## Retire an agent

```bash
$KANDEV_CLI kandev agents delete --id A-1
```

The backend rejects deletes for agents that are currently `working`. Use `kandev agents list` to confirm `status: idle` first.

## When to edit vs replace

- Permission / role mistakes → edit (you cannot change `role` after create — re-hire).
- Budget overrun → edit `--budget-monthly-cents`.
- Skill mismatch → use the Skills page (this CLI does not toggle individual skills).
- Agent never picks up work → delete + re-hire with a different role.
