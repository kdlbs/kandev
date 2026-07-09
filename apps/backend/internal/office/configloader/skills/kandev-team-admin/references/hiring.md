# Hiring

Hiring is gated by workspace approval policy. A new agent usually enters `pending_approval` and becomes `idle` only after approval.

Required inputs:

- Name: display name, such as `Frontend-Worker`.
- Role: `worker`, `specialist`, `assistant`, or `reviewer`.
- Reason: one concrete sentence surfaced on the approval row.

Hire a worker:

```bash
$KANDEV_CLI kandev agents create \
  --name "Frontend-Worker" \
  --role worker \
  --reason "Three frontend tasks queued and the existing worker is at capacity"
```

Check approval state:

```bash
$KANDEV_CLI kandev approvals list --status pending
$KANDEV_CLI kandev agents list
```

Do not hire when the work is one-off, budget is tight, or an existing specialist already matches.
