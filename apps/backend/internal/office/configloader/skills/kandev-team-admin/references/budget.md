# Budget

Workspace summary:

```bash
$KANDEV_CLI kandev budget get
```

Per-agent spend:

```bash
$KANDEV_CLI kandev budget get --agent-id A-1
```

Check spend before hiring, approving a `budget_grant`, weekly reporting, or investigating an agent that is working for a long time without progress.

This CLI is read-only. Adjust per-agent caps with `$KANDEV_CLI kandev agents update --budget-monthly-cents`. Workspace-level budget caps live in Settings > Costs.
