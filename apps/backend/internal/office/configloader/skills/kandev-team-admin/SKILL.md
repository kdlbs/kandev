---
name: kandev-team-admin
description: Manage the Office roster when delegating work, hiring agents, changing budgets or concurrency, retiring agents, or checking spend before approvals.
kandev:
  system: true
  version: "0.42.0"
  default_for_roles: [ceo]
---

# Team Admin

Use this skill when you need fresh organization state or need to change the agent roster.

## First checks

```bash
$KANDEV_CLI kandev agents list
$KANDEV_CLI kandev budget get
```

Before hiring, confirm no existing idle specialist or worker already fits. Before approving budget grants, check workspace and per-agent spend.

## Common actions

- List and inspect agents: read `references/team.md`.
- Hire a new worker, specialist, assistant, or reviewer: read `references/hiring.md`.
- Rename, cap, or retire an agent: read `references/agent-edit.md`.
- Review workspace or per-agent spend: read `references/budget.md`.

## Guardrails

- Do not hire for one-off work that an existing agent can handle.
- Do not assign work to agents in `pending_approval`.
- Do not silently pause or cap an over-budget agent. Leave a task comment explaining the reason first.
