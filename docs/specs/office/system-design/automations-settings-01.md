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

Users want to schedule an agent to run a prompt on a cron (or on a GitHub PR event, or on a webhook) without first navigating to per-workspace settings, picking the right workspace, then drilling down into a workflow. What they get back is informational — a report to read — so it must not land on the board as if it were work someone has to move.

The Automations feature, originating in PR #406, gives kandev a standalone trigger-based subsystem (cron, GitHub PR events, webhooks) that turns triggers into Tasks. This spec covers the automation object itself — its fields, triggers, and firing semantics. Where those runs are read and watched is `automation-runs.md`; that split is by concern, not by location, so keep both in step when either changes.

An earlier revision offered two execution flavours, `task` and `run`. That choice is withdrawn: it welded *where a run appears* to *how a run lives*, which no label could carry, and the clutter problem it existed to solve is now handled by giving runs their own destination rather than by hiding them.

## What

- Every automation produces the same kind of run. There is no execution-mode choice — the task/run selector is removed, and the field is ignored. A control that decided both *where a run appeared* and *how it lived* could not be labelled honestly: "Run" silently meant the worktree was destroyed, "Task" silently meant the schedule jammed after one firing.
- Every automation has an optional `repository_ids` field — an ordered list of repository IDs. When non-empty, scheduled and webhook firings pin the task to every listed repository (each on its own default branch, mirroring how the task-creation dialog resolves an explicit repository). When empty, falls back to the workspace's first repository (legacy behavior). `github_pr` triggers always use the PR's own repository and ignore `repository_ids`.
- The editor's repository picker matches the task-creation dialog's UX: lists both registered workspace repositories AND filesystem-discovered repositories under the workspace's roots. Picking a discovered repo registers it with the workspace at automation-save time (one round-trip via `createRepositoryAction`), then stores the resulting id on the automation. After the first save, the selection is promoted from `discovered` to `registered` so subsequent edits don't try to re-register.
- **Multi-repository selection is gated on the selected executor profile's capability**, reusing the task-creation dialog's `getMultiRepoExecutorDisabledReason` predicate (`apps/web/components/task-create-dialog-multi-repo-guard.ts`): `worktree`, `local_docker`, `ssh`, and `sprites` executor types support sibling repositories; `local`, `local_pc`, and `remote_docker` do not.
  - When the selected executor profile's type supports multi-repo, the picker renders as a repeatable list (0..N rows) with an "Add repository" control. Each row independently picks a registered or discovered repository; a repository already selected in another row is marked and not independently selectable a second time (mirrors the task-creation dialog's "Already added" marker).
  - When the selected executor profile's type does not support multi-repo, the picker renders as today's single dropdown (registered/discovered repos, or "Auto").
  - The Executor Profile picker disables executor profiles that don't support multi-repo whenever two or more repositories are currently selected, with the same disabled-reason text as the task-creation dialog, so a user cannot silently strand a multi-repository automation on an incompatible executor.
  - Automations created via the WS API directly (bypassing the editor) may still combine an incompatible executor with multiple repository IDs; this is a client-side authoring guard, not a backend rejection. Task launch on an incompatible executor fails the same way manual multi-repo task creation would.
- A trigger fires → a task is created with `origin = "automation_run"` so the existing session pipeline launches an agent. That task:
  - **SHALL NOT appear on the kanban or in the task list.** It is hidden by its `origin`, not by ephemerality. Automation output has its own destination (`automation-runs.md`); the board stays the human work list.
  - **SHALL keep its worktree when the turn ends**, subject to [Run retention](#run-retention). The files a run writes are usually the point of running it, and an agent that ends by asking a question needs a workspace in which to be answered. What is withdrawn is the *unconditional, permanent* retention of the original design — not reaping at end-of-turn.
  - **SHALL be repliable.** A run is a thread the user can open and continue, not a fire-and-forget transcript.
  - **SHALL reach a terminal run status on its own**, keyed on `origin`, so `max_concurrent_runs` frees up without a human archiving anything.
- A firing produces **both** a task and its run row, or neither. The run row is the only thing that makes the work reachable — the task is hidden from every board and list by its `origin` — so a task without one is invisible, unfinalizable, and holds no concurrency slot anyone can see. If the run row cannot be written, the task created for it is deleted rather than left behind.
- Workflow and workflow step are therefore optional for every automation: no automation run is placed on a board, so no automation needs a starting column.
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
  position      integer 0-based order, preserved for resolution and UI row order
  created_at    timestamp
  UNIQUE(automation_id, repository_id)
```

The legacy `automations.repository_id TEXT NOT NULL DEFAULT ''` column is dropped by the `automation_repositories` migration. Every pre-existing automation with a non-empty `repository_id` is backfilled into `automation_repositories` (position 0) before the column is dropped, so no automation silently loses its configured repository across the upgrade.

`CreateAutomation`/`UpdateAutomation` validate that every submitted repository ID belongs to the automation's `workspace_id` and reject unknown, foreign, or duplicate IDs.

The `tasks.origin` column already exists (used by quick-chat); the origin constant `TaskOriginAutomationRun = "automation_run"` lives in `internal/task/models/models.go`.

`is_ephemeral` previously carried two unrelated meanings — "hide from the board" AND "reap the worktree, never finalize the run". Those are now separated. Automation tasks are hidden by `origin`, keep their worktree, and finalize on `origin`. `is_ephemeral` is no longer set for automation runs and retains only its original quick-chat meaning.

`automation_runs.task_id` continues to reference the created task. The task is hidden from the board by its `origin`; it is otherwise an ordinary task.

## API surface

PR #406's WS-based API gets `repository_ids` (an ordered `string[]`, replacing the old singular `repository_id`) on the payloads below. `execution_mode` is accepted and ignored on input, and omitted from responses; the column stays only so no schema migration is needed. Accepting-and-ignoring is deliberate — an older client that still sends the field gets a successful write rather than a validation error it cannot act on. The behavioural consequence of ignoring it is covered in [Migration](#migration).

- `automation.create` payload (input) — `repository_ids?: string[]`
- `automation.update` payload (input) — `repository_ids?: string[]`; a present-but-empty array clears the automation's repositories, an absent field leaves them unchanged (matches the existing task-update `repositories` convention)
- `automation.get` / `automation.list` responses (output) — `repository_ids: string[]`, ordered by `position`

No new endpoints. No HTTP routes change. Sidebar deep-links to `/settings/automations` (flat).

## State machine

One pipeline, no branches. Every firing takes the same path through
`orchestrator/event_handlers_automation.go::handleAutomationTriggered`:

```text
trigger fires
  → resolve repository
  → CreateReviewTask(Origin=automation_run)          -- never ephemeral
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
| User manually drags an automation's task on the kanban                    | Cannot happen — automation tasks are hidden from the board by their `origin`. The "auto-start" rule fires once at trigger time; recovery is by replying to the run, not by dragging.  |
| Automation that previously ran in `task` mode                             | Its next firing no longer produces a board card. Cards already on the board are left alone; they are ordinary tasks now and can be archived by hand.                                 |
| Automation that previously ran in `run` mode                              | Its next firing keeps its worktree instead of having it reaped, so the run's output survives and the run can be answered.                                                            |
| A finished run parks in `WAITING_FOR_INPUT` so it stays answerable          | That state is in the "active session" set, so agent-profile deletion and its blocker list exclude automation runs **in that state only**. One nightly report must not permanently block deleting its profile, nor appear as a blocker the user cannot find on any board. The accepted consequence is that deleting the profile makes those parked conversations non-resumable — replying to an old run afterwards fails. |
| Agent-profile deletion while an automation run is `CREATED`, `STARTING` or `RUNNING` | Blocked, and the run is named in the confirmation. The run is using the profile right now, so this is the same hazard as for any other live task; only the parked state is exempt, not the origin. |
| A run left in `task_created` before this change                           | Finalization now keys on `origin`, so the stuck run reaches a terminal status on the next completion event and stops holding `max_concurrent_runs`.                                  |
| The run row cannot be written after the task is created | The task is deleted and the firing is abandoned, so nothing survives that no run points at. A delete that itself fails is logged with both ids — the task is then genuinely orphaned and needs manual cleanup, which is why it is reported rather than swallowed. |
| `automation.create`/`automation.update` submits a `repository_ids` entry outside the automation's workspace, or a duplicate ID | Request is rejected with a validation error; no partial repository list is persisted.                                                                                                                              |
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
