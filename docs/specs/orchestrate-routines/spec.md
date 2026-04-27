---
status: draft
created: 2026-04-25
owner: cfl
---

# Orchestrate: Recurring Scheduled Tasks (Routines)

## Why

Many development tasks are repetitive: daily dependency updates, weekly security scans, post-deploy smoke tests, periodic code quality sweeps. Today, users must manually create each task and trigger each agent run. There is no way to define a recurring task template that fires on a schedule, on a webhook, or on an event -- and automatically assigns the resulting task to an agent.

Routines let users define task templates that fire automatically, creating real kandev tasks assigned to agent instances. Combined with the wakeup queue, routines enable fully unattended recurring work.

## What

### Routine definition

- A routine is a task template with a trigger configuration.
- Routine fields:
  - `id`: unique identifier.
  - `workspace_id`: scoped to workspace.
  - `name`: human-readable label (e.g. "Daily Dependency Update", "Weekly Security Scan").
  - `description`: detailed description of what the routine does.
  - `task_template`: JSON defining the task to create on each run:
    - `title`: task title, supports `{{variable}}` interpolation.
    - `description`: task description, supports `{{variable}}` interpolation.
    - `priority`: task priority.
    - `project_id`: optional project to assign the task to.
    - `repository_ids`: which repositories the task works on.
    - `skill_ids`: which skills the agent needs for this task.
  - `assignee_agent_instance_id`: which agent instance gets the created task.
  - `status`: `active` or `paused`.
  - `concurrency_policy`: how to handle overlapping runs (see below).
  - `variables`: declared template variables with types and defaults.

### Routine config vs operational state

Routine data is split between the filesystem and DB:

- **Filesystem** (`routines/<name>.yml`): name, description, task template, assignee, concurrency policy, variables, trigger config (cron expression, webhook settings). Editable by users, versionable via git.
- **DB** (`orchestrate_routine_triggers`): operational trigger state -- `next_run_at`, `last_fired_at`, `public_id` (for webhooks), `enabled`. Needs atomic claims for cron scheduling.
- **DB** (`orchestrate_routine_runs`): run history -- status, linked task, trigger payload, timestamps.

On startup and config reload, the reconciliation service syncs triggers from YAML to DB: creates triggers for new routines, updates triggers for changed config (e.g. cron expression changed), deletes triggers and orphan runs for routines removed from filesystem.

### Triggers

Each routine has one or more triggers that cause it to fire:

- **Cron trigger**: a cron expression + timezone. The scheduler evaluates it on each tick and fires when the wall clock crosses `next_run_at`.
  - Fields: `cron_expression`, `timezone`, `next_run_at` (computed), `last_fired_at`.
  - Example: `0 9 * * MON` in `America/New_York` = every Monday at 9am ET.

- **Webhook trigger**: exposes a URL that external systems can call to fire the routine.
  - Fields: `public_id` (URL path component), `signing_mode` (`none`, `bearer`, `hmac_sha256`), `secret` (for signature verification).
  - URL: `POST /api/routine-triggers/<public_id>/fire`.
  - The webhook payload is available as variables in the task template.

- **Manual trigger**: fired only through the UI or API. Useful for routines that are usually cron-triggered but sometimes need to run on demand.

### Variables

- Routines support template variables using `{{name}}` syntax in the task title and description.
- Variable types: `text`, `number`, `boolean`, `select` (with predefined options).
- Built-in variables available in all routines:
  - `{{date}}`: today's date in ISO format.
  - `{{datetime}}`: current datetime in ISO format.
- Declared variables have a type, optional default value, and a required flag.
- Resolution order (later wins): built-ins -> declared defaults -> provided values (from manual trigger UI or webhook payload).
- Adding a `{{new_var}}` to the title/description auto-creates the variable declaration on save (with type `text`, no default, not required).

### Routine runs

- Each trigger firing creates a routine run record:
  - `id`: unique identifier.
  - `routine_id`: which routine.
  - `trigger_id`: which trigger fired it.
  - `source`: `cron`, `webhook`, or `manual`.
  - `status`: `received` -> `task_created` | `skipped` | `coalesced` | `failed`.
  - `trigger_payload`: the resolved variable values used for this run.
  - `linked_task_id`: the kandev task created by this run (if any).
  - `coalesced_into_run_id`: if this run was coalesced into another active run.
  - `dispatch_fingerprint`: hash of the resolved template + assignee, used for concurrency checks.
  - `started_at`, `completed_at`.

### Concurrency policies

- Controls what happens when a routine fires while a previous run's task is still active:
  - `skip_if_active`: do not create a new task. The run is marked `skipped`.
  - `coalesce_if_active` (default): do not create a new task. The run is marked `coalesced` and linked to the existing active task. Useful for "latest wins" patterns.
  - `always_create`: always create a new task regardless of active runs. Use for independent work items.
- "Active" means the linked task's status is not terminal (`done`, `cancelled`).
- The `dispatch_fingerprint` (hash of template content + assignee) determines uniqueness. Two runs with different variable values (e.g. different `{{date}}`) have different fingerprints and are not considered duplicates.

### Catch-up policy

- If the scheduler was down and missed cron ticks:
  - `skip_missed` (default): only fire for the current tick, ignore past missed fires.
  - `enqueue_missed` (with cap): fire all missed ticks up to a cap (default 25). Prevents thundering herd after long outages.

### Task creation flow

1. Trigger fires (cron tick, webhook, or manual).
2. Variables are resolved.
3. Task title and description are interpolated.
4. Concurrency check: if an active task exists for this fingerprint, apply the concurrency policy.
5. If proceeding: create a kandev task with `origin=routine`, `routine_run_id` set, and `assignee_agent_instance_id` set.
6. The task assignment triggers a `task_assigned` wakeup for the agent instance (via the wakeup queue).
7. The agent picks up the task and executes it like any other task.
8. When the task reaches a terminal state (`done`, `cancelled`), the routine run's status is updated accordingly.

### UI at `/orchestrate/routines`

- Routine list showing: name, trigger type, schedule (for cron), status (active/paused), last run time, next run time, assignee agent.
- Click to view routine detail: full config, run history table (with status, trigger payload, linked task), edit controls.
- "Create Routine" form: name, description, task template fields, trigger configuration, concurrency policy, variable declarations.
- Toggle to pause/resume a routine.
- "Run Now" button for manual triggering (with variable input form if the routine has required variables).
- Run history shows clickable task links that navigate to `/t/[taskId]`.

## Scenarios

- **GIVEN** a routine "Daily Dep Update" with cron `0 9 * * *` in UTC and assignee "Frontend Worker", **WHEN** the clock reaches 09:00 UTC, **THEN** a task titled "Daily Dep Update - 2026-04-25" is created, assigned to Frontend Worker, and a `task_assigned` wakeup is queued.

- **GIVEN** a routine with `concurrency_policy=skip_if_active` and an active task from a previous run, **WHEN** the routine fires again, **THEN** no new task is created. The run is recorded with status `skipped`.

- **GIVEN** a routine with a webhook trigger, **WHEN** an external system POSTs to the trigger URL with payload `{"branch": "release/2.0"}`, **THEN** the routine fires with `{{branch}}` resolved to "release/2.0" in the task template.

- **GIVEN** the scheduler was down for 3 hours and a routine has `catch_up_policy=skip_missed`, **WHEN** the scheduler restarts, **THEN** only the current tick fires. Missed cron ticks are ignored.

- **GIVEN** a routine with `catch_up_policy=enqueue_missed` that missed 5 cron ticks, **WHEN** the scheduler restarts, **THEN** 5 tasks are created (subject to the concurrency policy and cap).

- **GIVEN** a user on the routines page, **WHEN** they click "Run Now" on a routine with a required `{{reason}}` variable, **THEN** a modal prompts for the variable value before creating the task.

- **GIVEN** a routine "Daily Digest" with cron `0 8 * * *` and assignee "Personal Assistant", **WHEN** the routine fires at 8am, **THEN** the assistant creates a task, compiles overnight activity (completed tasks, agent errors, cost summary), and posts the digest as a comment on the Telegram channel task. The digest is relayed to the user via Telegram.

## Out of scope

- Complex workflow chains (routine A triggers routine B). Routines create independent tasks; inter-task dependencies use the blocker system.
- Routine templates shared across workspaces.
- Event-based triggers from external systems beyond webhooks (e.g. GitHub event subscriptions -- these can use webhooks as the transport).
- Routine-level budget limits (use agent or project budgets instead).
