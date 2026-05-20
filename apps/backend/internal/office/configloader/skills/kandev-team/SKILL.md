---
name: kandev-team
description: List and inspect agents in the workspace before delegating or hiring
kandev:
  system: true
  version: "0.42.0"
  default_for_roles: [ceo]
---

# Team — list and inspect agents

You manage a team. Before delegating work, check who's available. Before hiring, check whether someone on the roster already fits.

## List the whole org

```bash
$KANDEV_CLI kandev agents list
```

Returns every agent in the workspace with `id`, `name`, `role`, `status`, `budget_monthly_cents`, and `desired_skills`.

## Filter by role or status

```bash
$KANDEV_CLI kandev agents list --role worker
$KANDEV_CLI kandev agents list --status idle
```

## When to use

- Before `agentctl kandev tasks message --id …` — confirm the assignee is `idle`, not `working` on a different task.
- Before `agentctl kandev agents create` — confirm you actually need a new agent (often a `specialist` already exists).
- When budget pressure rises (see `kandev-budget`) — list agents and see which ones could be paused.

Do not call this on every heartbeat — only when you need fresh org state. The list does not change between routine fires.
