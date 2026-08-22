---
status: draft
created: 2026-05-21
owner: jcfs
---

# Automations in Settings

Decision:
[ADR-2026-08-22-user-configured-automation-continuity](../../decisions/2026-08-22-user-configured-automation-continuity.md)

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
- A trigger fires and creates or selects a task with `origin = "automation_run"` so the existing session pipeline launches an agent. That task:
  - **SHALL NOT appear on the kanban or in the task list.** It is hidden by its `origin`, not by ephemerality. Automation output has its own destination (`automation-runs.md`); the board stays the human work list.
  - **SHALL keep its worktree when the turn ends**, subject to [Run retention](#run-retention). The files a run writes are usually the point of running it, and an agent that ends by asking a question needs a workspace in which to be answered. What is withdrawn is the *unconditional, permanent* retention of the original design — not reaping at end-of-turn.
  - **SHALL be repliable.** A run is a thread the user can open and continue, not a fire-and-forget transcript.
  - **SHALL reach a terminal run status on its own**, keyed on `origin`, so `max_concurrent_runs` frees up without a human archiving anything.
- Every firing produces a run row. A `new_task` firing also creates a task; a `reuse_thread` firing either binds the run to the saved task/session or creates a replacement thread. No agent turn is dispatched until its run row exists and can be bound to the accepted turn. A newly-created task whose run cannot be recorded is deleted rather than left hidden and unreachable.
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

### Continuation policy and automation MCP surface

- The create and edit forms always expose **Context between runs**. Visible text below the heading
  says: "Choose whether each run starts fresh or continues the same conversation and files."
  The control has two choices:
  - **Start a new task for every run** (`new_task`, default). Each firing creates its own task,
    primary session, and task environment. Its visible description says: "Each run starts with a
    separate conversation and files. Use this option for independent jobs and concurrent runs."
  - **Continue the previous session** (`reuse_thread`). The first firing creates one hidden
    automation task. Later firings send the current prompt as a new turn to that task's primary
    session and reuse its conversation context and task-owned worktree. Its visible description
    says: "Runs continue the same conversation and files, so the agent keeps prior context and
    changes. Runs execute one at a time."
- The create form selects `new_task` by default but never hides the choice. The user can choose
  `reuse_thread` before the automation is first saved.
- The setting changes future firings only. Switching to `new_task` leaves prior reusable history
  intact. Switching to `reuse_thread` creates a reusable thread on the next firing.
- `reuse_thread` requires `max_concurrent_runs = 1`. The editor fixes the value at 1 and explains
  why with visible text: "This option supports one active run at a time." Create/update
  requests with another value are rejected. A single task session cannot own two scheduled turns
  concurrently.
- If a saved task, session, runtime, or task environment is missing, cancelled, deleted, pruned, or
  incompatible with the current launch identity, Kandev creates a fresh reusable thread and runs
  the firing there. The run records that it replaced the thread and a safe reason. A fallback is a
  successful continuity recovery, not a failed run by itself.
- Supported agents use their normal context-compaction behavior inside the reused session. Kandev
  does not rotate a healthy thread by age or number of turns. When a runtime must synthesize a
  fallback resume prompt instead of using native session continuation, it includes the newest 50
  non-empty `user_message` or `agent_message` entries. It excludes tool calls, tool results, status
  events, and unknown entry types before it selects the window. Those entries do not consume the
  limit. Kandev restores chronological order and applies the existing per-message truncation. The
  new automation prompt appears separately under the current request and does not consume the 50
  message limit.
- Agent profile, executor profile, repository list, workflow, or workflow-step changes invalidate
  the saved continuation binding. The next firing creates a replacement using the new launch
  identity. Name, description, prompt, and trigger changes do not rotate the thread; their new
  values apply on its next resume.
- Every automation session receives the backend-owned `SurfaceAutomation` MCP profile. MCP access is
  not a per-automation setting and the agent cannot request a different profile.
- The fixed profile contains:

  | Purpose | Tools |
  |---|---|
  | Workspace inventory | `list_workspaces_kandev` (returns only the automation workspace), `list_workflows_kandev`, `list_workflow_steps_kandev`, `list_repositories_kandev`, `list_tasks_kandev` |
  | Read-only launch catalog | `list_agents_kandev`, `list_executors_kandev`, `list_executor_profiles_kandev` |
  | Task and session inspection | `list_related_tasks_kandev`, `get_task_conversation_kandev`, `list_task_sessions_kandev` |
  | Task coordination | `create_task_kandev`, `update_task_kandev`, `move_task_kandev`, `archive_task_kandev`, `add_task_dependency_kandev`, `remove_task_dependency_kandev`, `message_task_kandev`, `stop_task_kandev`, `spawn_session_kandev` |
  | Blocker coordination | `list_pending_questions_kandev`, `answer_question_kandev`, `list_pending_agent_permissions_kandev`, `resolve_agent_permission_kandev` |

- The fixed profile excludes permanent task deletion; workflow, agent, executor, and MCP
  configuration writes; task plans and walkthroughs; user/parent questions; reviews and rich
  output; branch/source mutation; step completion; title ownership; diagnostics; and provider PR/MR
  automation. It also does not load arbitrary plugin tools.
- The automation's own hidden task and all sessions on it are invalid targets for task/session
  mutation, messaging, stopping, session spawning, and blocker discovery or resolution.
  A session spawned on another task receives that target task's normal MCP profile and never
  inherits `SurfaceAutomation` from the caller.
- The Context between runs control is one stacked radio group in the existing Settings card on
  desktop and mobile. The section description and both option descriptions remain visible without
  hover or focus. Each description is programmatically associated with its heading or radio
  control. Each choice has at least a 44 px touch target. The page remains the only scroll owner;
  there is no drawer or nested scroller.

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

`automation_runs.task_id` references the task used by that firing, whether newly created or reused. The task is hidden from the board by its `origin`; it is otherwise an ordinary task.

Continuation adds these automation fields:

| Field | Type | Contract |
|---|---|---|
| `continuation_policy` | enum | `new_task` or `reuse_thread`; defaults to `new_task` |
| `continuation_task_id` | nullable string | Current reusable task; server-owned runtime state and never accepted from create/update payloads |

Each dispatched `automation_runs` row persists `session_id`, `turn_id`, and its rendered
`display_title`, plus a thread action (`created`, `resumed`, or `replaced`) and an optional safe
reason. Skipped or pre-dispatch failed runs may leave task/session/turn empty but still retain their
rendered title. Turn identity is the completion and summary authority when several runs share one
task. Creating or replacing a thread uses that run title for the new task; resuming does not rename
the shared task.

Automation deletion cleanup is durable rather than best-effort:

```text
automation_task_cleanup_jobs
  task_id       string PK
  workspace_id  string
  automation_id string audit identity only; no foreign key to the deleted automation
  last_error    nullable string
  created_at    timestamp
  updated_at    timestamp
```

The deletion transaction inserts one job for each distinct referenced hidden task before it
removes the automation and run references. Normal task/worktree cleanup deletes the job only after
it succeeds. Startup and ordinary orphan reconciliation retry remaining jobs after a restart.

The existing `AutomationRun.status = triggered` is the firing that was atomically admitted but has
not yet bound an accepted agent turn. `triggered` and `task_created` both count as active and display
under Running. A successful binding transitions `triggered` to `task_created`; pre-dispatch failures
transition it to `failed`.

## API surface

PR #406's WS-based API gets `repository_ids` (an ordered `string[]`, replacing the old singular `repository_id`) on the payloads below. `execution_mode` is accepted and ignored on input, and omitted from responses; the column stays only so no schema migration is needed. Accepting-and-ignoring is deliberate — an older client that still sends the field gets a successful write rather than a validation error it cannot act on. The behavioural consequence of ignoring it is covered in [Migration](#migration).

- `automation.create` payload (input) — `repository_ids?: string[]`
- `automation.update` payload (input) — `repository_ids?: string[]`; a present-but-empty array clears the automation's repositories, an absent field leaves them unchanged (matches the existing task-update `repositories` convention)
- `automation.get` / `automation.list` responses (output) — `repository_ids: string[]`, ordered by `position`
- `automation.create` payload — `continuation_policy?: "new_task" | "reuse_thread"`; omission means
  `new_task`
- `automation.update` payload — optional `continuation_policy`; an absent field leaves it unchanged
- `automation.get` / `automation.list` responses — canonical `continuation_policy`;
  `continuation_task_id` and the fixed MCP surface are not public configuration

No new endpoints. No HTTP routes change. Sidebar deep-links to `/settings/automations` (flat).

## State machine

Every firing first writes or atomically admits its run, then follows the selected continuation
branch through `orchestrator/event_handlers_automation.go::handleAutomationTriggered`:

```text
trigger fires
  → resolve repository
  → atomically admit AutomationRun(status=triggered) -- one active run in reuse_thread
  → new_task: CreateReviewTask + StartTask
  → reuse_thread:
      saved thread is resumable: PromptTask(saved primary session)
      no usable thread: CreateReviewTask + StartTask + bind replacement
  → persist task_id + session_id + accepted turn_id + thread action; status=task_created
  → associate PR if github_pr trigger
      ↳ if the launch fails, the run is marked failed immediately: no completion
        event is coming, and an open run holds a max_concurrent_runs slot forever
  → the matching turn's terminal outcome marks that AutomationRun succeeded/failed and stops
    the execution. The worktree stays, and a successful run's session parks in
    WAITING_FOR_INPUT rather than COMPLETED so the user can reply to it.
```

Admission and binding are serialized per automation in durable storage. Concurrent manual,
scheduled, or webhook firings cannot both claim the reusable session. A published event carries the
pre-created run ID, and terminal updates address that run ID or exact session/turn pair rather than
"the newest run for task."

## Permissions

Automation settings use the existing workspace authorization boundary. The flat
`/settings/automations` page is reachable by anyone with workspace-list access, since it only lists
workspaces and links into the per-workspace UI.

`SurfaceAutomation` is both a discovery and execution boundary. Before dispatch reaches a tool
handler, one shared resolver derives a trusted automation MCP principal from the execution's own
task and session. It contains the automation ID, workspace ID, caller task ID, caller session ID,
and surface. The shared authorization boundary constrains every target to
`automation.workspace_id`; individual handlers do not fall back to the owner's broader identity. A
caller cannot widen scope by supplying a workspace, task, session, user, surface, actor kind, or
audit source. Foreign and missing targets return the same not-found result. Tools outside the fixed
surface are never registered for the session.

The external MCP endpoint keeps its personal-access-token authorization. Automation calls use a
distinct trusted `automation_mcp` source and automation actor; they neither forge `external_mcp` nor
inherit install-wide administrator reach.

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
| The admitted run cannot be bound after a new task is created | The run is marked failed and the new task is deleted, so nothing survives that no run points at. A delete that itself fails is logged with both IDs. |
| `automation.create`/`automation.update` submits a `repository_ids` entry outside the automation's workspace, or a duplicate ID | Request is rejected with a validation error; no partial repository list is persisted.                                                                                                                              |
| Editor has 2+ repositories selected and the user picks an executor profile whose type doesn't support multi-repo | Editor disables that executor profile in the picker with the same reason text as the task-creation dialog; the WS API itself does not reject the combination if reached directly. |
| A `reuse_thread` firing finds no resumable saved thread | A new task/session/worktree is created and atomically becomes the continuation. The run records `thread_action=replaced` and a safe reason, then proceeds normally. |
| Two triggers race for one `reuse_thread` automation | Durable admission accepts at most one active turn. The other firing records the existing concurrency-cap skip result and never prompts the shared session. |
| Backend stops after run admission but before dispatch | Startup/run reconciliation marks the undispatched run failed and releases its slot. It never guesses that a turn was accepted. |
| A bound run has no live execution, open turn, or pending blocker after restart/reconciliation | The exact run is marked failed and releases its slot before the next firing is admitted. |
| A bound run remains genuinely live or blocked | It continues to own the single slot. The automation detail exposes a visible Stop current run action that cancels the exact turn and marks the run failed. |
| Fallback resume history contains more than 50 conversation messages | Only the newest 50 non-empty user or assistant text messages are included, returned to chronological order, and individually truncated. Tool events do not appear or consume slots. Durable transcript rows are unchanged. |
| An automation calls a tool outside `SurfaceAutomation` | The tool is absent from discovery and the MCP server rejects an unknown tool name without performing an action. |
| A coordinator names a task or session outside the automation workspace | The call returns the same not-found result as an unknown target and performs no read or mutation. |
| A coordinator names its own automation task or any session on it for a mutation or blocker action | The call returns not-found and performs no action. Self-inspection does not become self-approval or concurrent worktree mutation. |
| `reuse_thread` is submitted with `max_concurrent_runs != 1` | Create/update is rejected atomically; prior automation configuration remains unchanged. |
| A live referenced session cannot be stopped during automation deletion | The deletion fails before references are removed, so the automation remains visible and retryable. |
| Task/worktree cleanup fails after automation deletion commits | Its `automation_task_cleanup_jobs` row retains the task and workspace identity plus the safe error; reconciliation retries until cleanup succeeds. |

## Persistence guarantees

**Deleting an agent profile does not strand the automations bound to it.** Deleting a profile disables every automation referencing it *before* the profile row is removed, and a failed disable aborts the delete rather than proceeding. The ordering is the guarantee: the reverse order — delete, then best-effort disable — leaves a live automation firing at a profile that no longer exists, silently, on every future schedule, with no reconciliation path to notice. Watchers are handled the other way round on purpose; the dispatch coordinator's preflight genuinely re-resolves them on the next poll, so eager-disable-after-delete is safe there. Automations have no such preflight, and assuming they did was the original defect.

Note the shape of that claim. It is about the **deletion path**, and it is deliberately not the stronger sentence "an enabled automation is never bound to a deleted profile" — that stronger version is what an earlier draft of this spec asserted, and it was false. Deletion is not the only way to reach the bad state: an automation is bound by `agent_profile_id`, so the write path has to refuse a profile that does not exist or the invariant can be broken directly, without any deletion involved. Ordering the delete correctly and leaving the write path open would have produced a spec that reads as a guarantee and is not one.

Two residuals, stated rather than implied:

- The disable and the delete are ordered but **not transactional**, so a process death between the two writes is uncovered. That direction is the safe one — an automation disabled against a live profile, visible on the automations page and re-enabled with one toggle.
- Nothing serialises a concurrent enable or rebind against an in-flight delete. The window is small and both outcomes are recoverable and visible, but it is a window, not an impossibility.

AutomationRuns and their tasks persist normally, worktree included (within [Run retention](#run-retention)) — an automation run survives a restart exactly as a hand-created task does. The continuation task pointer and run session/turn bindings are durable. A backend restart resumes through the normal task/session recovery path; it does not create a scheduled run merely because a parked continuation exists. `automation_repositories` rows survive restart and automation edits (replaced transactionally on update, cascade-deleted with the automation). `SurfaceAutomation` is derived from the product registry for every launch or resume and has no per-automation persistence. The board filter is applied at query time against `origin`, not at write time, so the hiding is a read-side decision and nothing about the task row is special-cased on write.

Deleting one run removes only that run row when its task is still referenced by another run or by
`continuation_task_id`. A task may be deleted only after no run and no continuation pointer owns it.
Delete-all clears the continuation pointer and deletes each distinct associated task once. These
rules prevent one historical run from destroying a shared conversation.

Deleting an automation first prevents new admissions and stops live sessions on its referenced
hidden tasks. The service captures every distinct run task plus `continuation_task_id` before the
automation/run rows disappear. In the same deletion transaction, it inserts one
`automation_task_cleanup_jobs` row per distinct task and then removes the automation/run rows. The
service sends each queued task through normal task/worktree cleanup and deletes its job only after
success. Failed jobs survive restart and are retried by startup and ordinary orphan reconciliation,
so the hidden task and worktree cannot become permanently unreachable. Workspace automation
cleanup uses the same ownership path.

## Run retention

Keeping every run's worktree forever is not a policy, it is a leak. A five-minute schedule produces ~288 runs a day, and each one is a full checkout; left unbounded that exhausts the disk of an install that was working fine before the upgrade. The original design said "keep the worktree" as a correction to `run` mode reaping it instantly, and that correction was right — but "not instantly" was mistaken for "never".

**Policy: the newest `DefaultRunWorktreeRetention` (10) distinct terminal automation tasks per automation keep their worktree. Older terminal tasks have their checkout reclaimed. The current reusable continuation task is always protected.**

What is reclaimed is only the working copy. The run row, its status and error message, the task, and the full transcript all survive, and the branch is left intact (`removeBranch=false`) so commits a run produced remain reachable as a ref. A pruned run is still readable history; it is only no longer *repliable*, because replying needs a workspace to reply in. That is the accepted cost of the policy, and the reason the window is per-automation rather than global — a rarely-firing automation keeps its whole history live.

Mechanics that matter:

- The sweep hangs off the exact-run terminal transition, which every finalize path funnels through. Candidate selection deduplicates task IDs before applying the window, so 500 turns in one reusable task consume one retention slot rather than 500.
- `WAITING_FOR_INPUT` is deliberately **not** treated as "in use". It is where successful runs park, so excluding it would make the policy a no-op — every prunable run is in exactly that state.
- **Any** session in `STARTING` or `RUNNING` protects the task, not merely its primary one. A resume racing the primary flag, or a passthrough session running alongside, is still an agent holding that checkout.
- Liveness is re-checked immediately before *each* removal, not once per sweep. Selecting candidates and then deleting them is a time-of-check/time-of-use window, and a user replying to an aged-out run lands in exactly that gap. The worktree manager is no help here: its reference guard excludes the worktree's own session, which for an automation run is precisely the session that would be live. A run that has gone live aborts the whole task and waits for a later sweep; a failed check counts as live.
- Candidates are restricted to runs that still *have* a live checkout. Without that, every finalize re-attempted the same ~200 already-reclaimed runs forever while anything past that window was never reached at all. With it, reclaimed runs drop out, the window slides, and a pre-existing backlog drains across successive firings.
- The task named by `continuation_task_id` is excluded even while its session is parked in `WAITING_FOR_INPUT`. Superseded continuation tasks participate as distinct historical tasks.
- A removal is not believed on its word. The manager logs a failed directory removal at warn level and then marks the row deleted anyway, so a nil error does not mean the disk was freed. The path is checked afterwards; a surviving directory is logged as an error, not a reclaim, and queued for retry.
- Every prune failure is logged and stepped over. Reclaiming disk is never allowed to fail a run.

Known residuals, stated rather than implied:

- If startup reconciliation itself repeatedly cannot persist a terminal status, the admitted run
  remains visible and continues to occupy its concurrency slot. Kandev logs the run ID and retries;
  it does not guess that dispatch succeeded or start a second reusable turn.
- The time-of-check window is narrowed to a single query before a single removal, not eliminated. Closing it entirely needs a lock the worktree manager does not offer.
- The retry queue for removals that silently failed is in-memory and bounded. A restart drops it, and the directory then persists until someone acts on the error log — there is no sweeper to collect it, because the office garbage collector is never constructed in production.
- Branches accumulate. If ref growth becomes a problem it needs its own policy; conflating it with worktree retention would silently discard commits.
- A reused worktree is not reset or rebased between firings. This preserves uncommitted and committed
  agent work. The agent may fetch or update it when its assignment requires fresh remote state; a
  replacement thread starts from the repository's current configured base branch.

### Retention scenarios

- **GIVEN** an automation with 13 distinct terminal task environments, **WHEN** the 13th finalizes, **THEN** the 3 oldest have their worktrees reclaimed and the newest 10 keep theirs.
- **GIVEN** a run whose worktree has been reclaimed, **WHEN** the user opens it, **THEN** its transcript, status and error message are still shown.
- **GIVEN** a run whose agent is still `RUNNING`, **WHEN** another run for the same automation finalizes, **THEN** the running run's worktree is not touched regardless of its age.
- **GIVEN** the worktree manager returns an error while reclaiming, **WHEN** a run finalizes, **THEN** the run still reaches its terminal status and the failure is logged.
- **GIVEN** a run that becomes live *after* it was selected as a candidate, **WHEN** the sweep reaches it, **THEN** its checkout is left alone.
- **GIVEN** a removal that reports success but leaves the directory in place, **WHEN** the sweep completes, **THEN** it is not reported as reclaimed and it is retried.
- **GIVEN** an automation whose backlog exceeds one sweep window, **WHEN** successive runs finalize, **THEN** the oldest are eventually reached rather than stranded.
- **GIVEN** 100 terminal runs all share the current continuation task, **WHEN** the newest finalizes, **THEN** that one task keeps its worktree and consumes one retention slot.
- **GIVEN** a launch-identity edit replaces a reusable thread, **WHEN** enough later distinct task environments accumulate, **THEN** the superseded thread becomes eligible while the current continuation remains protected.

## Migration

Withdrawing `execution_mode` is a behaviour change for existing installs, and the size of it is easy to understate. `task` was the **default**, so this is not an exotic minority: every automation created before this change that nobody explicitly set to `run` was producing a visible kanban card, and after upgrade it will not. The automations feature has been in the product since 2026-05-22, long enough for that to be somebody's working setup rather than a hypothetical.

The runs are not lost — they move. A firing still creates a real, persistent, repliable task with its worktree; it is hidden from the board by `origin` and surfaced at `/automations/:id` and in the sidebar's Automations section instead. What disappears is the board card, not the work.

**Decision: one destination, migrated loudly.** Honouring the stored `execution_mode` was considered and rejected. It would reinstate exactly the dual-destination split this change exists to remove, and would charge for it permanently — two write paths, two places a run can live, and every future automations surface built twice — in order to serve a migration window that closes once. The complexity would outlive the problem.

What we do instead:

1. On upgrade, identify automations whose stored `execution_mode` is `task` (still readable — the column is retained, just unread by the runtime).
2. Show a one-time, dismissible notice on the automations surface for workspaces that own such automations, stating that their runs now appear here rather than on the board. The notice is per-workspace and dismissal is durable, so it informs once instead of nagging.
3. Carry the same statement in the release notes for the version that ships this.

The detection is a **read of a retained column at migration time only**. It deliberately does not become a runtime branch — nothing in the firing path consults `execution_mode`, so the single-destination invariant holds from the first line of the change.

The continuation migration is additive and backward compatible. Existing automations receive
`continuation_policy = new_task` and no continuation pointer. Existing run rows have no session/turn
binding; their current task-based summary and terminal-history projection remains the legacy
fallback for those rows only. New dispatched runs always persist the exact binding. Every
automation session receives the fixed `SurfaceAutomation` profile after the release. Export
document version 1 adds the optional continuation key with its default; runtime pointers, run
bindings, and the fixed MCP profile are never exported.

### Migration scenarios

- **GIVEN** an install with an automation whose stored `execution_mode` is `task`, **WHEN** the user next opens the automations surface for that workspace, **THEN** a dismissible notice states that automation runs now appear there instead of on the kanban.
- **GIVEN** that notice has been dismissed, **WHEN** the user returns to the same surface later, **THEN** it does not reappear.
- **GIVEN** an install whose automations were all `run` mode, **WHEN** the user opens the automations surface, **THEN** no notice is shown — nothing changed for them.
- **GIVEN** any automation at all, **WHEN** a trigger fires after upgrade, **THEN** the run is created at the single automation-run destination regardless of the stored `execution_mode` value.

## Scenarios

- **GIVEN** any automation with a cron trigger, **WHEN** the cron fires, **THEN** the selected continuation policy creates or reuses a task that does NOT appear on the kanban or in the task list, the agent turn starts automatically, and the firing appears in activity.
- **GIVEN** an automation run whose agent ended by asking a question and whose worktree is still within the retention window, **WHEN** the user opens that run, **THEN** they can reply to it and the agent continues in the same worktree.
- **GIVEN** an automation run that wrote files, **WHEN** the run finishes, **THEN** those files are still present in its worktree, and remain so until the run falls outside the retention window.
- **GIVEN** an automation at `max_concurrent_runs = 1` whose run has completed, **WHEN** the next scheduled firing is due, **THEN** it runs — no archiving required.
- **GIVEN** an automation agent finishes a turn with `stop_reason = "end_turn"`, **WHEN** the complete event is handled, **THEN** the AutomationRun row is marked `succeeded`, the agent execution is stopped instead of waiting for process exit, and the session is left answerable rather than `COMPLETED`.
- **GIVEN** a firing whose task is created but whose launch then fails, **WHEN** the error is handled, **THEN** the AutomationRun is marked `failed` with the launch error, so the automation's concurrency slot is released instead of jamming permanently.
- **GIVEN** a firing whose run is admitted and whose new task cannot be bound to it, **WHEN** the error is handled, **THEN** the task is deleted, no agent is launched, and the run is marked failed.
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
- **GIVEN** a user creates an automation, **WHEN** the editor opens, **THEN** Context between runs visibly defaults to Start a new task for every run and the user can select Continue the previous session before saving.
- **GIVEN** an existing automation after migration, **WHEN** it fires without the user changing settings, **THEN** it starts a new task as before and receives the fixed `SurfaceAutomation` tool profile.
- **GIVEN** `reuse_thread` is selected, **WHEN** the second scheduled firing occurs after the first completed, **THEN** a new run row and turn are created in the same task, session, and worktree.
- **GIVEN** a reusable agent compacts its own context, **WHEN** a later firing resumes it, **THEN** Kandev continues the same healthy thread rather than rotating it because of age or turn count.
- **GIVEN** fallback history has 75 non-empty user or assistant messages, **WHEN** Kandev synthesizes the resume prompt, **THEN** it includes messages 26 through 75 in chronological order with per-message truncation.
- **GIVEN** an assistant message is followed by 150 tool calls and results, **WHEN** Kandev synthesizes fallback context, **THEN** the assistant message remains eligible and no tool event consumes one of the 50 message slots.
- **GIVEN** the automation editor opens on desktop or mobile, **WHEN** it shows Context between runs, **THEN** the section and both choices show their descriptions without hover or focus.
- **GIVEN** the user selects Continue the previous session, **WHEN** concurrency changes to its fixed value, **THEN** visible text explains that one active run is supported.
- **GIVEN** a reusable session was deleted or is no longer resumable, **WHEN** the next firing occurs, **THEN** Kandev creates a replacement thread and the run exposes that fallback without failing solely because continuity was lost.
- **GIVEN** a reusable automation changes executor profile, **WHEN** it next fires, **THEN** it creates a replacement thread under the new launch identity instead of resuming the old environment.
- **GIVEN** an automation session starts or resumes, **WHEN** it discovers MCP tools, **THEN** both pending-question tools and both live-permission tools are present without a per-automation capability setting.
- **GIVEN** an automation has a live permission or question on its own session, **WHEN** it calls the matching resolution tool, **THEN** the target is treated as not-found and the prompt remains pending for a person.
- **GIVEN** an automation spawns a session on another task in its workspace, **WHEN** the session starts, **THEN** its MCP profile is derived from the target task and does not inherit `SurfaceAutomation`.
- **GIVEN** an automation tries to spawn a second session on its own reusable task, **WHEN** the tool is called, **THEN** the request is rejected and only the scheduled turn can use that worktree.
- **GIVEN** an automation calls `delete_task_kandev` or a workflow configuration mutation, **WHEN** MCP dispatch resolves the name, **THEN** the tool is unavailable and no target changes.
- **GIVEN** an automation names a question, permission, task, or session in another workspace, **WHEN** it calls an included coordination tool, **THEN** the backend returns not-found without reading or mutating the foreign target.
- **GIVEN** a bound run survives but its execution and open turn do not, **WHEN** liveness reconciliation runs, **THEN** the run becomes failed and the next scheduled firing can claim the reusable thread.
- **GIVEN** a bound run is still live but stuck, **WHEN** the user activates Stop current run from the automation detail on desktop or mobile, **THEN** the exact turn is cancelled, the run becomes failed, and its slot is released.
- **GIVEN** two firings share a task and render different title templates, **WHEN** the run list is shown, **THEN** each row keeps its own rendered title while the shared task retains its creation title.
- **GIVEN** an automation with a reusable continuation is deleted, **WHEN** deletion finishes, **THEN** its hidden task and worktree are deleted or durably queued for orphan cleanup and no new firing is admitted.
- **GIVEN** task cleanup fails after an automation row is deleted, **WHEN** the backend restarts, **THEN** the durable cleanup job still identifies the hidden task and reconciliation retries it.

## Out of scope

- **AutomationRun-as-true-session-owner** (instead of ephemeral task). The cleaner model — make `task_sessions.task_id` nullable, add `task_sessions.automation_run_id`, route automation runs bypassing tasks entirely — was considered and explicitly deferred to a future PR. It touches ~50+ files in the orchestrator + session pipeline + WS layer + frontend state, which is out of scope here. The origin-tagged-task path is the pragmatic shim.
- **Agent-type primary picker.** PR #406's editor still picks an `agent_profile_id` (a fully configured profile), not a raw agent type (`claude` / `codex` / `opencode`). Switching to agent-type-primary requires plumbing changes in the orchestrator (which expects a profile id). Deferred.
- **Auto-provisioned default workspace.** When no workspaces exist, the flat page shows a CTA; it does not auto-create one. Most installs already have a workspace (workspace setup is part of onboarding), so the CTA is sufficient for now.
- **Cross-workspace automation listing** on the flat page. Multi-workspace installs see a picker, not a merged list. Merging would require a new list-all endpoint and a workspace column in the table.
- A standalone AutomationRun detail page showing session output inline. A run links to its task's detail page, which is where the transcript already lives; `automation-runs.md` covers how a reader reaches it.
- Reinstating a per-automation choice about board placement. If an automation that *creates work* is wanted later, it should be asked as an outcome — "a report I read" vs "a task on my board" — not as an execution mode named after an internal enum.
- Automatically resetting, rebasing, or rotating a healthy reusable worktree. Continuation preserves
  agent state; remote synchronization is task behavior, not scheduler cleanup.
- Pruning durable task transcripts solely because an automation reuses one session. Transcript
  retention is a product-wide task-history policy and applies equally to isolated automation runs.

## Open questions

- (none)
