---
status: draft
created: 2026-05-21
owner: jcfs
---

# Automations in Settings

## Why

Users want to schedule an agent to run a prompt on a cron (or on a GitHub PR event, or on a webhook) without first navigating to per-workspace settings, picking the right workspace, then drilling down into a workflow. They also want two execution flavors: **tracked work** that shows up on the kanban as a real task (current default), and **fire-and-forget runs** whose output is just informational — not kanban clutter.

The Automations feature, originating in PR #406, gives kandev a standalone trigger-based subsystem (cron, GitHub PR events, webhooks) that turns triggers into Tasks. This spec extends it with two changes: (1) a per-automation `execution_mode` choice between **task** (existing behavior) and **run** (new — creates an ephemeral kanban-hidden task that surfaces only through the AutomationRun row), and (2) a flat `/settings/automations` entry point that drops the per-workspace nesting from the sidebar.

## What

- Every automation has an `execution_mode` field — `task` (default) or `run`. The choice is per-automation, editable in the editor.
- `execution_mode = task`: trigger fires → a normal kanban task is created (current PR #406 behavior). Task is visible on the kanban, commentable, reviewable, and has full lifecycle.
- `execution_mode = run`: trigger fires → an ephemeral task is created (`is_ephemeral = true`, `origin = "automation_run"`) so the existing session pipeline still launches an agent. The kanban hides ephemeral tasks. The AutomationRun row is the surfaced artifact; the linked task is plumbing only.
- Run-mode automations **auto-start** their agent regardless of the workflow step's `auto_start_agent` setting — the user never opens the task to drag it, so the trigger MUST be the start signal.
- The sidebar exposes a single top-level **Automations** entry pointing at `/settings/automations`. The per-workspace `Automations` sub-link is removed (PR #406 added it; this spec drops it).
- `/settings/automations` is a server-side router:
  - 0 workspaces → empty state with "Create workspace" CTA.
  - 1 workspace → redirect to `/settings/workspace/<id>/automations`.
  - 2+ workspaces → workspace picker (grid of cards, click to enter).
- The automations table shows the execution mode as a badge column ("Task" / "Run") so the user can scan which automations clutter the kanban and which don't.

## Data model

Builds on PR #406's `internal/automation/` schema. Only one column added:

```
automations.execution_mode TEXT NOT NULL DEFAULT 'task'   -- 'task' | 'run'
```

Idempotent migration in `internal/automation/store.go::migrateExecutionModeSQL` — `ALTER TABLE automations ADD COLUMN execution_mode TEXT NOT NULL DEFAULT 'task'`. SQLite swallows duplicate-column errors on re-run.

The `tasks.is_ephemeral` and `tasks.origin` columns already exist (used by quick-chat). Run-mode automations set both at task-create time. New task origin constant `TaskOriginAutomationRun = "automation_run"` lives in `internal/task/models/models.go`.

`automation_runs.task_id` continues to reference the created task for both modes. Run mode just means that task is hidden.

## API surface

PR #406's WS-based API gets one new field — `execution_mode` — on:

- `automation.create` payload (input)
- `automation.update` payload (input)
- `automation.get` / `automation.list` responses (output)

No new endpoints. No HTTP routes change. Sidebar deep-links to `/settings/automations` (flat).

## State machine

Automation lifecycle unchanged. Run-mode and task-mode share the trigger → AutomationRun pipeline. The only branching is in `orchestrator/event_handlers_automation.go::handleAutomationTriggered`:

```
trigger fires
  → resolve repository
  → CreateReviewTask(IsEphemeral=mode==run, Origin=automation_run)
  → record AutomationRun (status=task_created, task_id set)
  → associate PR if github_pr trigger
  → if mode==run OR step.auto_start_agent: StartTask
```

## Permissions

Inherits PR #406's model (no per-action authorization gates). The flat `/settings/automations` page is reachable by anyone with workspace-list access, since it only lists workspaces and links into the per-workspace UI.

## Failure modes

| Dependency / invariant | Behavior |
|---|---|
| `listWorkspaces` fails on the flat page | Page renders the empty state (treating "couldn't load" as "no workspaces"). |
| Run-mode automation's task starts but agent fails | AutomationRun stays at `task_created` status; task error_message reflects the failure. User sees the failed AutomationRun row. |
| User manually drags a run-mode task on the kanban | Cannot happen — ephemeral tasks are hidden from the kanban. The "auto-start" rule fires once at trigger time; no manual recovery path. |
| Existing automation upgraded from pre-execution_mode version | Migration sets `execution_mode = 'task'` for all existing rows. UI shows them with "Task" badge. |
| User edits `execution_mode` from `task` to `run` on an enabled automation | Next firing uses the new mode. In-flight runs are unaffected. |

## Persistence guarantees

`automations.execution_mode` and `tasks.is_ephemeral` survive restart. Run-mode AutomationRuns and their hidden tasks persist normally. The kanban filter on `is_ephemeral` is applied at query time, not at write time — so re-marking a task non-ephemeral via direct DB update would reveal it.

## Scenarios

- **GIVEN** a user creates an automation with `execution_mode = "task"` and a cron trigger, **WHEN** the cron fires, **THEN** a normal kanban task appears with the rendered title; the user can click it, drag it, comment on it.
- **GIVEN** a user creates an automation with `execution_mode = "run"` and a cron trigger, **WHEN** the cron fires, **THEN** an ephemeral task is created (not visible on the kanban), the agent starts automatically, and the AutomationRun row in the automation's history shows the result.
- **GIVEN** a user opens `/settings/automations` in an install with one workspace, **WHEN** the page loads, **THEN** the browser redirects to `/settings/workspace/<id>/automations`.
- **GIVEN** a user opens `/settings/automations` in an install with three workspaces, **WHEN** the page loads, **THEN** a workspace picker is shown; clicking one navigates to its automations.
- **GIVEN** a user opens `/settings/automations` in a fresh install with zero workspaces, **WHEN** the page loads, **THEN** an empty-state card explains "create a workspace first" with a CTA.
- **GIVEN** a user toggles an existing task-mode automation to run mode in the editor, **WHEN** the next trigger fires, **THEN** the resulting task is hidden from the kanban; previously-created tasks (from task-mode firings) remain visible.
- **GIVEN** a run-mode automation triggered by a GitHub PR event, **WHEN** the trigger fires, **THEN** the PR is associated with the ephemeral task via `AssociatePRWithTask` exactly as in task mode.
- **GIVEN** an upgrade from a pre-execution_mode kandev version, **WHEN** the user opens the editor for an existing automation, **THEN** the execution-mode selector defaults to "Task" (preserving previous behavior).

## Out of scope

- **AutomationRun-as-true-session-owner** (instead of ephemeral task). The cleaner model — make `task_sessions.task_id` nullable, add `task_sessions.automation_run_id`, route run-mode bypassing tasks entirely — was considered and explicitly deferred to a future PR. It touches ~50+ files in the orchestrator + session pipeline + WS layer + frontend state, which is out of scope here. The ephemeral-task path is the pragmatic shim.
- **Agent-type primary picker.** PR #406's editor still picks an `agent_profile_id` (a fully configured profile), not a raw agent type (`claude` / `codex` / `opencode`). Switching to agent-type-primary requires plumbing changes in the orchestrator (which expects a profile id). Deferred.
- **Auto-provisioned default workspace.** When no workspaces exist, the flat page shows a CTA; it does not auto-create one. Most installs already have a workspace (workspace setup is part of onboarding), so the CTA is sufficient for now.
- **Cross-workspace automation listing** on the flat page. Multi-workspace installs see a picker, not a merged list. Merging would require a new list-all endpoint and a workspace column in the table.
- A standalone AutomationRun detail page (showing session output for run-mode firings). Run-mode automations currently link to the linked task's detail page; since the task is hidden from the kanban it's reachable only by direct URL.
- Webhook execution-mode override (e.g. webhook payload setting `execution_mode` per call). The execution mode is automation-level, not per-firing.

## Open questions

- (none)
