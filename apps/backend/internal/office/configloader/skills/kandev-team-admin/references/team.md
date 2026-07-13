# Team Roster

List the whole organization:

```bash
$KANDEV_CLI kandev agents list
```

Returns every agent in the workspace with `id`, `name`, `role`, `status`, `budget_monthly_cents`, and `desired_skills`.

Filter by role or status:

```bash
$KANDEV_CLI kandev agents list --role worker
$KANDEV_CLI kandev agents list --status idle
```

Use this before delegation, hiring, or budget adjustments. Do not call it on every heartbeat when the existing wake payload already has enough context.
