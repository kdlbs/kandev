---
status: draft
system: office
requirements:
  - REQ-OFFICE-AUTOMATIONS-SETTINGS-001
created: 2026-05-21
owners:
  - jcfs
---
# Automations in Settings System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-AUTOMATIONS-SETTINGS-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AUTOMATIONS-SETTINGS-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Migration

Withdrawing `execution_mode` is a behaviour change for existing installs, and the size of it is easy to understate. `task` was the **default**, so this is not an exotic minority: every automation created before this change that nobody explicitly set to `run` was producing a visible kanban card, and after upgrade it will not. The automations feature has been in the product since 2026-05-22, long enough for that to be somebody's working setup rather than a hypothetical.

The runs are not lost — they move. A firing still creates a real, persistent, repliable task with its worktree; it is hidden from the board by `origin` and surfaced at `/automations/:id` and in the sidebar's Automations section instead. What disappears is the board card, not the work.

**Decision: one destination, migrated loudly.** Honouring the stored `execution_mode` was considered and rejected. It would reinstate exactly the dual-destination split this change exists to remove, and would charge for it permanently — two write paths, two places a run can live, and every future automations surface built twice — in order to serve a migration window that closes once. The complexity would outlive the problem.

What we do instead:

1. On upgrade, identify automations whose stored `execution_mode` is `task` (still readable — the column is retained, just unread by the runtime).
2. Show a one-time, dismissible notice on the automations surface for workspaces that own such automations, stating that their runs now appear here rather than on the board. The notice is per-workspace and dismissal is durable, so it informs once instead of nagging.
3. Carry the same statement in the release notes for the version that ships this.

The detection is a **read of a retained column at migration time only**. It deliberately does not become a runtime branch — nothing in the firing path consults `execution_mode`, so the single-destination invariant holds from the first line of the change.

### Migration scenarios

- **GIVEN** an install with an automation whose stored `execution_mode` is `task`, **WHEN** the user next opens the automations surface for that workspace, **THEN** a dismissible notice states that automation runs now appear there instead of on the kanban.
- **GIVEN** that notice has been dismissed, **WHEN** the user returns to the same surface later, **THEN** it does not reappear.
- **GIVEN** an install whose automations were all `run` mode, **WHEN** the user opens the automations surface, **THEN** no notice is shown — nothing changed for them.
- **GIVEN** any automation at all, **WHEN** a trigger fires after upgrade, **THEN** the run is created at the single automation-run destination regardless of the stored `execution_mode` value.

## Scenarios

- **GIVEN** any automation with a cron trigger, **WHEN** the cron fires, **THEN** a task is created that does NOT appear on the kanban or in the task list, the agent starts automatically, and the run appears in the automation's activity.
- **GIVEN** an automation run whose agent ended by asking a question and whose worktree is still within the retention window, **WHEN** the user opens that run, **THEN** they can reply to it and the agent continues in the same worktree.
- **GIVEN** an automation run that wrote files, **WHEN** the run finishes, **THEN** those files are still present in its worktree, and remain so until the run falls outside the retention window.
- **GIVEN** an automation at `max_concurrent_runs = 1` whose run has completed, **WHEN** the next scheduled firing is due, **THEN** it runs — no archiving required.
- **GIVEN** an automation agent finishes a turn with `stop_reason = "end_turn"`, **WHEN** the complete event is handled, **THEN** the AutomationRun row is marked `succeeded`, the agent execution is stopped instead of waiting for process exit, and the session is left answerable rather than `COMPLETED`.
- **GIVEN** a firing whose task is created but whose launch then fails, **WHEN** the error is handled, **THEN** the AutomationRun is marked `failed` with the launch error, so the automation's concurrency slot is released instead of jamming permanently.
- **GIVEN** a firing whose task is created but whose run row cannot be written, **WHEN** the error is handled, **THEN** the task is deleted and no agent is launched, so no hidden task survives that nothing can reach or finalize.
- **GIVEN** a user opens `/settings/automations` in an install with one workspace, **WHEN** the page loads, **THEN** the browser redirects to `/settings/workspace/<id>/automations`.
- **GIVEN** a user opens `/settings/automations` in an install with three workspaces, **WHEN** the page loads, **THEN** a workspace picker is shown; clicking one navigates to its automations.
- **GIVEN** a user opens `/settings/automations` in a fresh install with zero workspaces, **WHEN** the page loads, **THEN** an empty-state card explains "create a workspace first" with a CTA.
- **GIVEN** a user opens `/settings/automations` in a multi-workspace install, **WHEN** the page loads, **THEN** it renders the picker from the already-loaded workspace list and issues **no** additional `GET /api/v1/workspaces` request on load (guards against the render/refetch loop that a server-style `await listWorkspaces()` in the page body caused after the SPA migration).
- **GIVEN** an automation triggered by a GitHub PR event, **WHEN** the trigger fires, **THEN** the PR is associated with the created task via `AssociatePRWithTask` as before.
- **GIVEN** a scheduled automation with `repository_ids` set to one repo, **WHEN** the cron fires, **THEN** the resulting task is pinned to that repo's default branch — regardless of whether the workspace has other repositories.
- **GIVEN** a scheduled automation with `repository_ids` set to two or more repos, **WHEN** the cron fires, **THEN** the resulting task is created with all of them attached, each pinned to its own default branch, same as a manually created multi-repository task.
- **GIVEN** a scheduled automation with `repository_ids = []` in a multi-repo workspace, **WHEN** the cron fires, **THEN** the task uses the workspace's first repository (legacy fallback) and a warning is logged.
- **GIVEN** an automation with `repository_ids` set and a `github_pr` trigger, **WHEN** a PR event fires, **THEN** the task uses the PR's own repository, not the configured `repository_ids` — the editor disables the picker for PR triggers with a hint.
- **GIVEN** the editor's selected executor profile type is `worktree`, `local_docker`, `ssh`, or `sprites`, **WHEN** the user opens the repository picker, **THEN** it renders as a repeatable list and "Add repository" is enabled.
- **GIVEN** the editor's selected executor profile type is `local`, `local_pc`, or `remote_docker`, **WHEN** the user opens the repository picker, **THEN** it renders as a single dropdown and there is no "Add repository" control.
- **GIVEN** the editor has two or more repositories selected, **WHEN** the user opens the Executor Profile picker, **THEN** profiles whose type doesn't support multi-repo are disabled with an explanatory reason, matching the task-creation dialog's guard text.
- **GIVEN** an automation created before `repository_ids` existed with a non-empty legacy `repository_id`, **WHEN** the schema migration runs, **THEN** the editor shows that repository pre-selected as the sole row after upgrade.
- **GIVEN** a user picks a discovered (not-yet-registered) repository in the editor and clicks Save, **WHEN** the save flow runs, **THEN** the discovered repo is registered with the workspace first (`createRepositoryAction`), its new id is written onto the automation, and the picker selection is promoted to `registered` so re-saving doesn't duplicate the registration.
- **GIVEN** an automation created before this change, **WHEN** the user opens the editor, **THEN** no execution-mode selector is shown and the automation behaves like every other one.

## Out of scope

- **AutomationRun-as-true-session-owner** (instead of ephemeral task). The cleaner model — make `task_sessions.task_id` nullable, add `task_sessions.automation_run_id`, route automation runs bypassing tasks entirely — was considered and explicitly deferred to a future PR. It touches ~50+ files in the orchestrator + session pipeline + WS layer + frontend state, which is out of scope here. The origin-tagged-task path is the pragmatic shim.
- **Agent-type primary picker.** PR #406's editor still picks an `agent_profile_id` (a fully configured profile), not a raw agent type (`claude` / `codex` / `opencode`). Switching to agent-type-primary requires plumbing changes in the orchestrator (which expects a profile id). Deferred.
- **Auto-provisioned default workspace.** When no workspaces exist, the flat page shows a CTA; it does not auto-create one. Most installs already have a workspace (workspace setup is part of onboarding), so the CTA is sufficient for now.
- **Cross-workspace automation listing** on the flat page. Multi-workspace installs see a picker, not a merged list. Merging would require a new list-all endpoint and a workspace column in the table.
- A standalone AutomationRun detail page showing session output inline. A run links to its task's detail page, which is where the transcript already lives; `automation-runs.md` covers how a reader reaches it.
- Reinstating a per-automation choice about board placement. If an automation that *creates work* is wanted later, it should be asked as an outcome — "a report I read" vs "a task on my board" — not as an execution mode named after an internal enum.

## Open questions

- (none)
