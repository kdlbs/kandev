---
status: draft
system: office
requirements:
  - REQ-OFFICE-AUTOMATIONS-SETTINGS-001
created: 2026-05-21
owners:
  - jcfs
---

# Automations in Settings System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-AUTOMATIONS-SETTINGS-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AUTOMATIONS-SETTINGS-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Why

Users want to schedule an agent to run a prompt on a cron (or on a GitHub PR event, or on a webhook) without first navigating to per-workspace settings. Most results are reports that belong only in automation history. Some automations intentionally create work that must appear as a normal task in a selected workflow.

The Automations feature, originating in PR #406, gives kandev a standalone trigger-based subsystem (cron, GitHub PR events, webhooks) that turns triggers into Tasks. This spec covers the automation object itself — its fields, triggers, and firing semantics. Where those runs are read and watched is `automation-runs.md`; that split is by concern, not by location, so keep both in step when either changes.

The target-mode amendment in `../requirements/automation-target-modes.md` restores an explicit product outcome without restoring the old lifecycle ambiguity: a firing either creates a hidden automation-run task or a normal workflow task. Both outcomes still have an `AutomationRun` and exact turn identity.

## What

- Every firing creates an `AutomationRun`. `task_mode = automation_run` creates a hidden task surfaced only through automation history. `task_mode = normal_task` creates an ordinary task that appears in the selected workflow, kanban, and sidebar.
- Every automation has an ordered `repositories` list of explicit repository/base-branch pairs. An empty list means repository-free execution in task-owned scratch space. Kandev never selects the workspace's first repository implicitly. `github_pr` triggers use the repository and branch context supplied by the event.
- The repository, workflow, agent, and executor controls reuse the searchable New Task selectors. Repository rows pair a repository with its base branch. The workflow control previews its steps but does not select one; normal tasks use the configured start step.
- The editor applies the New Task multi-repository executor guard. The API keeps the normal task-launch behavior for incompatible direct requests.
- In hidden mode, a trigger creates a task with `origin = "automation_run"` so the existing session pipeline launches an agent. That task:
  - **SHALL NOT appear on the kanban or in the task list.** It is hidden by its `origin`, not by ephemerality. Automation output has its own destination (`automation-runs.md`); the board stays the human work list.
  - **SHALL keep its worktree when the turn ends**, subject to [Run retention](#run-retention). The files a run writes are usually the point of running it, and an agent that ends by asking a question needs a workspace in which to be answered. What is withdrawn is the *unconditional, permanent* retention of the original design — not reaping at end-of-turn.
  - **SHALL be repliable.** A run is a thread the user can open and continue, not a fire-and-forget transcript.
  - **SHALL reach a terminal run status on its own**, keyed on `origin`, so `max_concurrent_runs` frees up without a human archiving anything.
- A firing produces **both** a task and its run row, or neither. The run row is the only thing that makes the work reachable — the task is hidden from every board and list by its `origin` — so a task without one is invisible, unfinalizable, and holds no concurrency slot anyone can see. If the run row cannot be written, the task created for it is deleted rather than left behind.
- Workflow is optional for hidden automation-run tasks and required for normal tasks. The backend resolves the normal task's start step from the workflow configuration. A workflow step is not stored as an automation authoring choice.
- Because the run row is the surfaced artifact, it MUST actually surface the artifact: each row carries the tail of the agent's last message and links to that run's transcript. Hiding a task from the board is not a reason to withhold the only route to what it said. The reading surface is specified in `automation-runs.md`.
- The run log offers a status filter that includes **Skipped**, **Archived** and **Cancelled**. A scheduled firing turned away by the concurrency cap writes a run row and nothing else, so without it a paused automation is indistinguishable from one that was never due.
- Automations **auto-start** their agent regardless of any workflow step's `auto_start_agent` setting — nobody opens the task to drag it, so the trigger MUST be the start signal.
- The sidebar exposes a single top-level **Automations** entry pointing at `/settings/automations`. The per-workspace `Automations` sub-link is removed (PR #406 added it; this spec drops it).
- `/settings/automations` is a client route that branches on the workspace list already loaded into the SPA (from the boot payload / store) — it does **not** fetch the workspace list on load:
  - 0 workspaces → empty state with "Create workspace" CTA.
  - 1 workspace → redirect to `/settings/workspace/<id>/automations`.
  - 2+ workspaces → workspace picker (grid of cards, click to enter).
- Firing an automation by hand reports whether a run actually started. A trigger turned away by the concurrency cap, by dedup, or because the automation is disabled is reported as skipped with a reason — never as a successful fire, and it does not advance `last_triggered_at`.
- A scheduled trigger carries a timezone alongside its cron expression. An empty timezone means UTC. The editor composes the schedule as a sentence and states the resolved next firing in both the chosen zone and UTC, because a cron expression alone never says which instant it means.
- Cron expressions are validated with the scheduler's own parser at add/update time. Client-side validation MUST NOT be the only gate: an expression the scheduler cannot parse would otherwise save and then silently never fire.
- The workflow offered for an automation MUST belong to that automation's workspace, enforced server-side. A UI filter is not an authorization boundary, and a workflow name present in two workspaces makes the wrong one look right.

## Data model

Builds on PR #406's `internal/automation/` schema. `execution_mode` is retained in the canonical `CREATE TABLE` so no *schema* migration is required, but nothing reads it. That is a narrower claim than it looks: the column being safe to leave in place says nothing about behaviour, and withdrawing it **does** change what existing automations do. See [Migration](#migration).

```sql
automations.execution_mode TEXT NOT NULL DEFAULT 'task'   -- retained so no schema migration is needed; no longer read
```

Repository selection moved from a single column to a join table:

```text
automation_repositories
  id            string  PK
  automation_id string  FK -> automations.id (ON DELETE CASCADE)
  repository_id string  FK-by-id to repositories, not enforced at the DB layer
  base_branch   string  explicit branch selected with this repository
  position      integer 0-based order, preserved for resolution and UI row order
  created_at    timestamp
  UNIQUE(automation_id, repository_id)
```

The legacy `automations.repository_id TEXT NOT NULL DEFAULT ''` column is dropped by the `automation_repositories` migration. Every pre-existing automation with a non-empty `repository_id` is backfilled into `automation_repositories` (position 0) before the column is dropped, so no automation silently loses its configured repository across the upgrade.

`CreateAutomation`/`UpdateAutomation` validate that every submitted repository belongs to the automation's `workspace_id`, has a base branch, and is not duplicated.

The `tasks.origin` column already exists (used by quick-chat); the origin constant `TaskOriginAutomationRun = "automation_run"` lives in `internal/task/models/models.go`.

`is_ephemeral` previously carried two unrelated meanings — "hide from the board" AND "reap the worktree, never finalize the run". Those are now separated. Automation tasks are hidden by `origin`, keep their worktree, and finalize on `origin`. `is_ephemeral` is no longer set for automation runs and retains only its original quick-chat meaning.

`automation_runs.task_id` continues to reference the created task. The task is hidden from the board by its `origin`; it is otherwise an ordinary task.

## API surface

The WS API uses canonical `repositories` entries with `repository_id` and `base_branch`, plus `task_mode` and `repository_mode`. Legacy `repository_ids` input remains compatible and resolves each ID to that repository's configured default branch.

- `automation.create` payload (input) — `repositories?: { repository_id, base_branch }[]`, `task_mode`, and derived `repository_mode`
- `automation.update` payload (input) — a present-but-empty `repositories` array clears repository access; an absent field leaves it unchanged
- `automation.get` / `automation.list` responses (output) — ordered repository/base-branch pairs; legacy repository IDs remain a compatibility projection

No new endpoints. No HTTP routes change. Sidebar deep-links to `/settings/automations` (flat).

## State machine

Every firing uses the same admission and exact-run lifecycle. Dispatch branches only when it creates the target task through `orchestrator/event_handlers_automation.go::handleAutomationTriggered`:

```text
trigger fires
  → resolve repository
  → CreateReviewTask(Origin=automation_run or normal) -- never ephemeral
  → record AutomationRun (status=task_created, task_id set)
  → associate PR if github_pr trigger
  → StartTask                                        -- unconditional; the trigger IS the start signal
      ↳ if the launch fails, the run is marked failed immediately: no completion
        event is coming, and an open run holds a max_concurrent_runs slot forever
  → agent terminal turn outcome marks the AutomationRun succeeded/failed and stops
    the execution. The worktree stays, and a successful run's session parks in
    WAITING_FOR_INPUT rather than COMPLETED so the user can reply to it.
```

## Permissions

Inherits PR #406's model (no per-action authorization gates). The flat `/settings/automations` page is reachable by anyone with workspace-list access, since it only lists workspaces and links into the per-workspace UI.

## Failure modes

| Dependency / invariant                                                    | Behavior                                                                                                                                                                             |
| ------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| No workspaces are loaded into the SPA store when the flat page renders    | Page renders the empty state (treating "none loaded" as "no workspaces"). The page reads the store and never fetches on load, so there is no per-render fetch that can fail or loop. |
| Automation's task starts but the agent fails                              | AutomationRun transitions from `task_created` to `failed`; the row surfaces the failure instead of remaining "Running".                                                              |
| Automation's agent completes its turn successfully                        | AutomationRun transitions from `task_created` to `succeeded`; the agent execution is stopped. The worktree stays, and the session parks in `WAITING_FOR_INPUT` so the run can be answered. |
| Automation's turn is cancelled by the user                                | AutomationRun transitions from `task_created` to `failed` with a cancellation error; the hidden session is marked `CANCELLED` and the agent execution is torn down.                  |
| User looks for a hidden automation-run task on the kanban                 | It is intentionally absent and remains available through the automation run transcript. Normal-task targets are ordinary visible workflow tasks.                              |
| Automation that previously ran in `task` mode                             | Its next firing no longer produces a board card. Cards already on the board are left alone; they are ordinary tasks now and can be archived by hand.                                 |
| Automation that previously ran in `run` mode                              | Its next firing keeps its worktree instead of having it reaped, so the run's output survives and the run can be answered.                                                            |
| A finished run parks in `WAITING_FOR_INPUT` so it stays answerable          | That state is in the "active session" set, so agent-profile deletion and its blocker list exclude automation runs **in that state only**. One nightly report must not permanently block deleting its profile, nor appear as a blocker the user cannot find on any board. The accepted consequence is that deleting the profile makes those parked conversations non-resumable — replying to an old run afterwards fails. |
| Agent-profile deletion while an automation run is `CREATED`, `STARTING` or `RUNNING` | Blocked, and the run is named in the confirmation. The run is using the profile right now, so this is the same hazard as for any other live task; only the parked state is exempt, not the origin. |
| A run left in `task_created` before this change                           | Finalization now keys on `origin`, so the stuck run reaches a terminal status on the next completion event and stops holding `max_concurrent_runs`.                                  |
| The run row cannot be written after the task is created | The task is deleted and the firing is abandoned, so nothing survives that no run points at. A delete that itself fails is logged with both ids — the task is then genuinely orphaned and needs manual cleanup, which is why it is reported rather than swallowed. |
| `automation.create`/`automation.update` submits a repository outside the automation's workspace, a duplicate repository, or an empty base branch | Request is rejected with a validation error; no partial repository list is persisted.                                                                                                              |
| Editor has 2+ repositories selected and the user picks an executor profile whose type doesn't support multi-repo | Editor disables that executor profile in the picker with the same reason text as the task-creation dialog; the WS API itself does not reject the combination if reached directly. |

## Persistence guarantees

**Deleting an agent profile does not strand the automations bound to it.** Deleting a profile disables every automation referencing it *before* the profile row is removed, and a failed disable aborts the delete rather than proceeding. The ordering is the guarantee: the reverse order — delete, then best-effort disable — leaves a live automation firing at a profile that no longer exists, silently, on every future schedule, with no reconciliation path to notice. Watchers are handled the other way round on purpose; the dispatch coordinator's preflight genuinely re-resolves them on the next poll, so eager-disable-after-delete is safe there. Automations have no such preflight, and assuming they did was the original defect.

Note the shape of that claim. It is about the **deletion path**, and it is deliberately not the stronger sentence "an enabled automation is never bound to a deleted profile" — that stronger version is what an earlier draft of this spec asserted, and it was false. Deletion is not the only way to reach the bad state: an automation is bound by `agent_profile_id`, so the write path has to refuse a profile that does not exist or the invariant can be broken directly, without any deletion involved. Ordering the delete correctly and leaving the write path open would have produced a spec that reads as a guarantee and is not one.

Two residuals, stated rather than implied:

- The disable and the delete are ordered but **not transactional**, so a process death between the two writes is uncovered. That direction is the safe one — an automation disabled against a live profile, visible on the automations page and re-enabled with one toggle.
- Nothing serialises a concurrent enable or rebind against an in-flight delete. The window is small and both outcomes are recoverable and visible, but it is a window, not an impossibility.

AutomationRuns and their tasks persist normally, worktree included (within [Run retention](#run-retention)) — an automation run survives a restart exactly as a hand-created task does. `automation_repositories` rows survive restart and automation edits (replaced transactionally on update, cascade-deleted with the automation). The board filter is applied at query time against `origin`, not at write time, so the hiding is a read-side decision and nothing about the task row is special-cased on write.

## Run retention

Keeping every run's worktree forever is not a policy, it is a leak. A five-minute schedule produces ~288 runs a day, and each one is a full checkout; left unbounded that exhausts the disk of an install that was working fine before the upgrade. The original design said "keep the worktree" as a correction to `run` mode reaping it instantly, and that correction was right — but "not instantly" was mistaken for "never".

**Policy: the newest `DefaultRunWorktreeRetention` (10) terminal runs per automation keep their worktree. Older terminal runs have their checkout reclaimed.**

What is reclaimed is only the working copy. The run row, its status and error message, the task, and the full transcript all survive, and the branch is left intact (`removeBranch=false`) so commits a run produced remain reachable as a ref. A pruned run is still readable history; it is only no longer *repliable*, because replying needs a workspace to reply in. That is the accepted cost of the policy, and the reason the window is per-automation rather than global — a rarely-firing automation keeps its whole history live.

Mechanics that matter:

- The sweep hangs off `markAutomationRunTerminal`, which every finalize path funnels through. That is precisely the moment one run enters the window and pushes another out, so no scheduler is needed.
- `WAITING_FOR_INPUT` is deliberately **not** treated as "in use". It is where successful runs park, so excluding it would make the policy a no-op — every prunable run is in exactly that state.
- **Any** session in `STARTING` or `RUNNING` protects the task, not merely its primary one. A resume racing the primary flag, or a passthrough session running alongside, is still an agent holding that checkout.
- Liveness is re-checked immediately before *each* removal, not once per sweep. Selecting candidates and then deleting them is a time-of-check/time-of-use window, and a user replying to an aged-out run lands in exactly that gap. The worktree manager is no help here: its reference guard excludes the worktree's own session, which for an automation run is precisely the session that would be live. A run that has gone live aborts the whole task and waits for a later sweep; a failed check counts as live.
- Candidates are restricted to runs that still *have* a live checkout. Without that, every finalize re-attempted the same ~200 already-reclaimed runs forever while anything past that window was never reached at all. With it, reclaimed runs drop out, the window slides, and a pre-existing backlog drains across successive firings.
- A removal is not believed on its word. The manager logs a failed directory removal at warn level and then marks the row deleted anyway, so a nil error does not mean the disk was freed. The path is checked afterwards; a surviving directory is logged as an error, not a reclaim, and queued for retry.
- Every prune failure is logged and stepped over. Reclaiming disk is never allowed to fail a run.

Known residuals, stated rather than implied:

- A run stranded at `task_created` — for instance by a backend crash mid-flight — never becomes terminal and so is never pruned. Its worktree persists.
- The time-of-check window is narrowed to a single query before a single removal, not eliminated. Closing it entirely needs a lock the worktree manager does not offer.
- The retry queue for removals that silently failed is in-memory and bounded. A restart drops it, and the directory then persists until someone acts on the error log — there is no sweeper to collect it, because the office garbage collector is never constructed in production.
- Branches accumulate. If ref growth becomes a problem it needs its own policy; conflating it with worktree retention would silently discard commits.

### Retention scenarios

- **GIVEN** an automation with 13 terminal runs, **WHEN** the 13th finalizes, **THEN** the 3 oldest have their worktrees reclaimed and the newest 10 keep theirs.
- **GIVEN** a run whose worktree has been reclaimed, **WHEN** the user opens it, **THEN** its transcript, status and error message are still shown.
- **GIVEN** a run whose agent is still `RUNNING`, **WHEN** another run for the same automation finalizes, **THEN** the running run's worktree is not touched regardless of its age.
- **GIVEN** the worktree manager returns an error while reclaiming, **WHEN** a run finalizes, **THEN** the run still reaches its terminal status and the failure is logged.
- **GIVEN** a run that becomes live *after* it was selected as a candidate, **WHEN** the sweep reaches it, **THEN** its checkout is left alone.
- **GIVEN** a removal that reports success but leaves the directory in place, **WHEN** the sweep completes, **THEN** it is not reported as reclaimed and it is retried.
- **GIVEN** an automation whose backlog exceeds one sweep window, **WHEN** successive runs finalize, **THEN** the oldest are eventually reached rather than stranded.

## Migration

This section records the earlier migration away from the legacy
`execution_mode` lifecycle. It does not override the current `task_mode`
outcomes defined by the target-mode amendment.

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
- **GIVEN** any automation during that migration, **WHEN** a trigger fires after upgrade, **THEN** the stored legacy `execution_mode` does not choose its lifecycle or destination.

## Scenarios

- **GIVEN** a hidden automation-run target with a cron trigger, **WHEN** the cron fires, **THEN** its task does NOT appear on the kanban or in the sidebar, the agent starts automatically, and the run appears in the automation's activity.
- **GIVEN** a normal-task target with a selected workflow, **WHEN** the trigger fires, **THEN** a normal task appears in that workflow and the sidebar at the workflow's configured auto-start step.
- **GIVEN** a normal-task target without a workflow, **WHEN** the user saves it, **THEN** validation rejects the automation.
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
- **GIVEN** a scheduled automation with one repository/base-branch pair, **WHEN** the cron fires, **THEN** the resulting task uses that exact repository and branch regardless of other workspace repositories.
- **GIVEN** a scheduled automation with two or more repository/base-branch pairs, **WHEN** the cron fires, **THEN** the resulting task is created with all pairs attached in the saved order, like a manually created multi-repository task.
- **GIVEN** a scheduled automation with an empty repository list in a workspace that has repositories, **WHEN** the cron fires, **THEN** the task uses task-owned scratch space and no workspace repository is attached.
- **GIVEN** a repository-free automation using a Worktree executor profile, **WHEN** it fires, **THEN** it runs in scratch space without creating a Git worktree or failing for lack of a repository.
- **GIVEN** an automation with configured repositories and a `github_pr` trigger, **WHEN** a PR event fires, **THEN** the task uses the event's repository context rather than the configured pairs.
- **GIVEN** the editor's selected executor profile type is `worktree`, `local_docker`, `ssh`, or `sprites`, **WHEN** the user opens the repository picker, **THEN** it renders as a repeatable list and "Add repository" is enabled.
- **GIVEN** the editor's selected executor profile type does not support multiple repositories, **WHEN** the user opens the repository picker, **THEN** it retains the shared paired-chip presentation and prevents an incompatible multi-repository save.
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

## Open questions

- (none)
