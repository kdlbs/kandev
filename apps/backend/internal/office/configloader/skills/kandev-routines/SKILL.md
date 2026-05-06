---
name: kandev-routines
description: Schedule recurring work via cron and webhook triggers
kandev:
  system: true
  version: "0.42.0"
  default_for_roles: [ceo]
---

# Routines — schedule recurring tasks

Routines fire on cron or webhook and create a fresh task each time, assigned to a specific agent. Use them when work needs to happen on a clock (daily standups, weekly reports, hourly health checks).

## List existing routines

```bash
$KANDEV_CLI kandev routines list
```

## Create a daily standup routine

```bash
$KANDEV_CLI kandev routines create \
  --name "Daily standup" \
  --task-title "What did each agent ship today?" \
  --task-description "Summarise yesterday's commits and PRs across the team. Post to #engineering." \
  --assignee A-ceo \
  --cron "0 9 * * MON-FRI" \
  --timezone "Europe/Lisbon"
```

This creates the routine and attaches a cron trigger in one command. The CLI does two API calls under the hood (routine + trigger); the response is the trigger row.

## Pause, resume, delete

```bash
$KANDEV_CLI kandev routines pause  --id R-1
$KANDEV_CLI kandev routines resume --id R-1
$KANDEV_CLI kandev routines delete --id R-1
```

## Required inputs

- **`--name`** — display name; users see this on the Routines page.
- **`--task-title`** — title for every task this routine creates. Supports `{{name}}` and `{{date}}` placeholders.
- **`--task-description`** — agent instructions for each fire.
- **`--assignee`** — the agent ID that receives the task. For coordinator work, this is the CEO; for distributed checks, a worker or specialist.
- **`--cron`** — standard 5-field cron expression.

## When NOT to create a routine

- One-off work — just `tasks create` directly.
- A reaction to a specific event — use a workflow trigger or webhook channel, not a routine.
- Something the CEO heartbeat already covers (e.g. checking for new comments — that's automatic).
