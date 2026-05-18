---
name: kandev-budget
description: Check workspace and per-agent spend before expensive operations
kandev:
  system: true
  version: "0.42.0"
  default_for_roles: [ceo]
---

# Budget — know the spend before you spend more

Every model call costs tokens. The CEO is responsible for keeping the workspace within budget.

## Workspace summary

```bash
$KANDEV_CLI kandev budget get
```

Returns: total `input_tokens`, `output_tokens`, `cached_tokens`, `total_cost_cents` for the workspace.

## Per-agent slice

```bash
$KANDEV_CLI kandev budget get --agent-id A-1
```

Returns the same breakdown scoped to a single agent. Useful when one agent burns the budget faster than others.

## When to check

- Before hiring (`kandev-hiring`) — confirm there's room for another monthly seat.
- Before approving a `budget_grant` (`kandev-approvals`).
- On a weekly heartbeat — surface spend in your routine output.
- When an agent looks stuck — check whether it's racking up cost without progress (`agent budget high, status working, last comment > 1h old` → investigate).

## What you can't do here

- This CLI is read-only. Adjust monthly caps with `agents update --budget-monthly-cents`.
- Workspace-level budget caps are set in the UI under Settings → Costs.
- Don't pause an over-budget agent silently — leave a comment first (`tasks message`), explain why, then `agents update --budget-monthly-cents 0` if needed.
