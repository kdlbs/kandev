---
name: kandev-hiring
description: Hire new agents via agentctl, gated by the workspace approval policy
kandev:
  system: true
  version: "0.42.0"
  default_for_roles: [ceo]
---

# Hiring — grow the team

You can hire new agents to expand capacity. Hiring is **gated** by the workspace's approval policy: in most workspaces a new agent enters `pending_approval` status and only becomes `idle` after a human approves the `hire_agent` request.

## Required inputs

Before calling the API, decide:
- **Name** — what to call the agent (e.g. `Reviewer`, `Frontend-Worker`)
- **Role** — `worker`, `specialist`, `assistant`, `reviewer` (CEOs hire each of these; you do not hire other CEOs)
- **Reason** — one sentence justifying the hire; surfaced on the approval row

## Hire a worker

```bash
$KANDEV_CLI kandev agents create \
  --name "Frontend-Worker" \
  --role worker \
  --reason "Three frontend tasks queued and the existing worker is at capacity"
```

The response includes the new agent's `id` and `status`. If `status: pending_approval`, the agent is queued — do **not** assign work to it until it flips to `idle`.

## Check whether the approval landed

```bash
$KANDEV_CLI kandev approvals list --status pending
```

Or just re-list the team and look at the new agent's status:

```bash
$KANDEV_CLI kandev agents list
```

## When NOT to hire

- The task you're trying to delegate is one-off — use `agentctl kandev tasks message` to ask an existing agent.
- Budget is already tight (see `kandev-budget`).
- A specialist with matching skills already exists.

Hiring without a clear reason will be rejected at the approval step.
