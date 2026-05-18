---
name: kandev-approvals
description: List pending approvals and decide them (approve or reject)
kandev:
  system: true
  version: "0.42.0"
  default_for_roles: [ceo]
---

# Approvals — clear the queue

Sensitive office mutations (hire requests, budget grants, certain task transitions) are gated behind approvals. The CEO is the default decider.

## List the queue

```bash
$KANDEV_CLI kandev approvals list --status pending
```

Returns rows with `id`, `type` (e.g. `hire_agent`), `requester_agent_id`, `reason`, and `created_at`.

## Approve or reject

```bash
$KANDEV_CLI kandev approvals decide --id AP-1 --decision approve --note "Backlog supports the hire."
$KANDEV_CLI kandev approvals decide --id AP-2 --decision reject  --note "Hire blocked by Q3 budget freeze."
```

The note is visible on the approval row and surfaced to the requester on their next heartbeat.

## Decision criteria

For `hire_agent`:
- Does the role match the workload? (`agents list` to verify org composition first)
- Is the monthly budget available? (`kandev-budget`)
- Is the reason concrete?

For `budget_grant`:
- Does workspace spend justify the lift?
- Is there a less expensive alternative (smaller model, fewer agents)?

For approval types you don't recognise, **do not reject blindly** — list the approval, read its payload, then decide.

## Cadence

Approvals don't auto-fire reminders. If the CEO leaves them sitting, the requesters block indefinitely. Run `approvals list` on every heartbeat and clear anything you can decide quickly.
