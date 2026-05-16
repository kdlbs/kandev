# 0004: Task model unification — office is a workflow style, not a separate model

**Status:** proposed
**Date:** 2026-05-05
**Area:** backend, frontend

## Context

Kandev today carries two execution models on top of one task table:

- **Kanban** tasks flow through a configurable workflow. The workflow engine fires triggers (`on_enter`, `on_turn_start`, `on_turn_complete`, `on_exit`) at step boundaries. At each step the agent runs once, possibly with a step-specific prompt prefix/suffix and an agent-profile override. The user is the primary actor: they create the task, click Start, watch the agent run, advance the workflow.

- **Office** tasks live indefinitely with no workflow. They have stages (`work`, `review`, `approval`, `ship`) defined by an execution policy. Comments, blocker clearings, child completions, approval decisions, and heartbeats queue runs against the assignee through `office_runs`. Sessions go IDLE between turns; the agent process is torn down. Multiple agents participate per stage (assignee in work, reviewers in review, approvers in approval). Agent autonomy is the primary actor.

The two models cohabit `tasks`, `task_sessions`, `task_session_messages`, `task_session_turns`, `task_comments`, and `executors_running`. Roughly half the columns on `tasks` are office-only (`assignee_agent_instance_id`, `execution_policy`, `execution_state`, `requires_approval`, `checkout_agent_id`, `checkout_at`, `model_profile`, `project_id`); they're `NULL`/`""` for kanban. The discriminator pattern leaks into shared code paths everywhere — `IsOfficeTask()` branches in the orchestrator, the executor, the prompt builder, the DTOs.

We've patched around this for a month: the missing `agent_instance_id` on `TaskSessionDTO` letting the office UI silently misclassify sessions as kanban; `LaunchPreparedSession` not reading `executors_running.resume_token` because kanban didn't need it and office did; the prompt builder re-sending `AGENTS.md` on every wakeup because the office resume detection hadn't been threaded through. Each fix is correct in isolation. The pattern — shared tables forcing shared code paths — is what's broken.

Reading office and kanban side-by-side, they're not actually two models. Both are event-driven state machines over a step graph; both produce agent runs in response to events; both record outcomes. The differences are which events count as triggers (kanban: step transitions; office: also comments, blockers, children, approvals, heartbeats) and how many agents participate per step (kanban: one runner; office: a primary plus reviewers / approvers). Office is a *richer event-driven configuration* of the same workflow concept the kanban engine already implements.

We have no production office users. The dev DB can be wiped without consequence. There is no migration cost for the office side.

## Decision

Restructure tasks around four architectural seams. They are listed roughly in dependency order.

### 1. The agent runtime as a single shared interface

Extract a runtime package — `internal/agent/runtime/` — combining today's `internal/agent/lifecycle/`, the runtime-touching parts of `internal/orchestrator/executor/`, and `internal/agentctl/client/`. Public surface is one interface:

```go
package runtime

type Runtime interface {
    Launch(ctx, LaunchSpec)        (ExecutionRef, error)
    Resume(ctx, executionID, prompt) error
    Stop(ctx, executionID, reason) error
    SetMcpMode(ctx, executionID, mode) error
    GetExecution(ctx, executionID) (*Execution, error)
    SubscribeEvents(ctx, executionID) (<-chan Event, error)
}
```

`LaunchSpec` carries everything the runtime needs to run an agent: profile, executor, workspace, prompt, prior ACP session id (for resume), MCP mode, metadata. The runtime knows nothing about tasks, workflows, or office stages. Both the workflow engine and (separately) cron-driven trigger handlers call `Launch`.

The runtime owns the *execution* tables: `agent_executions` (renamed `executors_running`), the message and turn family that's currently `task_session_messages` / `task_session_turns`, and `task_session_worktrees`. These are generic — they describe an agent conversation, not a task lifecycle.

The naming "runtime" is concrete: it's the runtime layer for agents, sitting under the workflow engine and atop agentctl + executor backends. Replaces the earlier draft's "kernel" (too generic).

### 2. Office is a workflow style — engine generalisation

The workflow engine becomes the universal coordinator for task-scoped agent runs. Existing kanban trigger types (`on_enter`, `on_turn_start`, `on_turn_complete`, `on_exit`) are preserved exactly. Seven new trigger types are added:

- `on_comment` — a user (or external) comment landed on the task.
- `on_blocker_resolved` — all blockers cleared.
- `on_children_completed` — all child tasks completed.
- `on_approval_resolved` — an approval request resolved.
- `on_heartbeat` — periodic timer (cron-driven; per-(task, step) pair where the step has this trigger configured).
- `on_budget_alert` — budget threshold crossed; the runs scheduler detects threshold and fires per-task.
- `on_agent_error` — fired only after the runs queue's retry policy exhausts (4 attempts at [2m, 10m, 30m, 2h] with jitter). The trigger fires on the *failing task* itself.

Each new trigger fires through the existing `Engine.HandleTrigger` plumbing; the trigger payload carries the contextual data (`comment_id`, `blocker_id`, `failed_agent_id`, etc.). All trigger types are opt-in per step. A workflow that uses none of them behaves exactly as today's kanban.

Workflow steps gain two additional descriptors:

- **`stage_type`** ENUM(`work`, `review`, `approval`, `custom`) — a semantic hint for UX rendering. The engine doesn't branch on it. A step typed `review` tells the UI to surface participants and decision affordances. Note: `ship` is intentionally not a stage_type — shipping work happens inside `work` (or in a delegated child task), not in its own stage.
- **A participants list** — zero or more agents in addition to the step's primary agent. Each participant has a role (`reviewer`, `approver`, `watcher`), an optional `decision_required` flag, and an `agent_profile_id`. An empty participants list = single-agent step = today's kanban behaviour.

Three new action types support multi-agent and cross-task scenarios:

- `queue_run` — generalises today's `auto_start` callback. Targets `primary`, `participant_role:<role>`, or `agent_profile_id:<id>`. Optionally takes a `task_id` resolver: `this` (default — the trigger's task) or a literal task id. Emits a row into the unified `runs` queue with the supplied trigger context. `auto_start` becomes a thin alias.
- `queue_run_for_each_participant` — fans out runs for an `on_enter` action on a multi-agent step. Reads the step's participants list and emits one `queue_run` per matching role.
- `clear_decisions` — clears `workflow_step_decisions` rows for a `(task, step)` pair. Used by the Review step's `on_enter` so quorum starts fresh when the task re-enters Review after a rejection round.

A new transition guard — `wait_for_quorum` — blocks transitions until N-of-M decisions are recorded. Decisions are written via a new `record_participant_decision` callback, called by the office service when a reviewer's run completes with a verdict.

The first-transition-wins, idempotent-by-`OperationID`, `EvaluateOnly`-mode contract on `Engine.HandleTrigger` is preserved.

### 3. One generic `runs` queue

`office_runs` becomes `runs`. It's the universal queue for engine-emitted launches. Schema is unchanged from today's `office_runs`: `(id, agent_profile_id, task_id, workflow_step_id, reason, status, idempotency_key, payload, requested_at, claimed_at, finished_at, error_message, retry_count, scheduled_retry_at, …)`.

Two entry paths:

- **Engine-emitted runs** — every `queue_run` action lands here. Coalescing (5s window for same agent + reason), per-agent serialisation (one claimed run per agent), idempotency (24h dedup window), cooldown (per-agent), and atomic task checkout (one agent per task at a time) all apply. The scheduler claims runs and dispatches via `runtime.Launch`.
- **User-initiated launches** — a user clicking Start on a kanban task bypasses the queue and calls `runtime.Launch` directly. The queue's coalescing isn't appropriate when latency matters and the user explicitly chose timing.

`run_events` (the audit log we already have) keeps its current shape. Per-run streaming via `office.run.event_appended.<run_id>` works unchanged. The WS event subjects (`office.run.queued`, `office.run.processed`, `office.run.event_appended.*`) keep the `office.` prefix for now to preserve frontend stability — they're string constants we can rename later if we want.

### 4. Cron-driven triggers + coordination tasks

Cron-driven events (heartbeats, budget alerts, routine firings) feed the engine through dedicated handlers. Three handlers run on a shared cron tick loop:

- **Heartbeat handler.** For each `(task, step)` pair where the step has `on_heartbeat` configured AND the agent's runtime allows (cooldown, status), fire `Engine.HandleTrigger(TriggerOnHeartbeat, task)`. The step's action emits a run.
- **Budget handler.** Detects budget thresholds crossed (workspace, project, agent). For each affected task, fire `Engine.HandleTrigger(TriggerOnBudgetAlert, task)` with payload `{budget_pct, scope}`.
- **Routine handler.** On cron tick, fire eligible routines. Each firing creates a real task with the routine's template. The new task's first step's `on_enter` fires; standard engine path from there. (Routines are *upstream* of the engine, not parallel to it. Today's `office-routines` spec needs no behavioural change.)

For senior agents that need *workspace-level* reasoning (e.g. the CEO surveying multiple tasks) we use a **coordination task pattern**: a real, standing task assigned to the senior agent at workspace setup. Its workflow has `on_heartbeat` configured; the heartbeat handler fires the trigger; the agent wakes on the coordination task with workspace-summary context (active runs, pending approvals, recent failures) and can comment on it, delegate child tasks from it, transition it.

The coordination task replaces the previous "wake the CEO with no task" model. Every wakeup is task-scoped; agents never "look at the workspace and pick something" — they're given a specific task to act on.

### 5. `tasks` shape (final)

```sql
CREATE TABLE tasks (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    workflow_id TEXT NOT NULL,                  -- always set
    workflow_step_id TEXT NOT NULL,             -- always set
    title TEXT NOT NULL,
    description TEXT DEFAULT '',
    state TEXT NOT NULL DEFAULT 'TODO',
    priority TEXT NOT NULL DEFAULT 'medium',
    position INTEGER DEFAULT 0,
    metadata TEXT DEFAULT '{}',
    is_ephemeral INTEGER NOT NULL DEFAULT 0,
    parent_id TEXT DEFAULT '',
    archived_at TIMESTAMP,
    created_by TEXT,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    origin TEXT DEFAULT 'manual',
    labels TEXT DEFAULT '[]',
    identifier TEXT,
    -- Generic single-runner lock; not office-specific anymore.
    -- Held while an agent's run is claimed against this task.
    checkout_agent_id TEXT,
    checkout_at DATETIME
);
```

Gone:
- `assignee_agent_instance_id` — the workflow step's `primary_agent_profile_id` is the assignee. For tasks where the user wants a "primary owner" view, derive it from the workflow step.
- `execution_policy`, `execution_state` — replaced by workflow step graph + transitions.
- `requires_approval` — expressed as a transition rule (`when:'turn_complete', if:'approval.granted', goto:'done'`).
- `model_profile` — moves into workflow step's `model_profile_override`.
- `project_id` — kept in office's project table; tasks reference projects via a dedicated table or `metadata.project_id`.
- `execution_strategy` — never added.

Workflow steps:

```sql
CREATE TABLE workflow_steps (
    id TEXT PRIMARY KEY,
    workflow_id TEXT NOT NULL,
    name TEXT NOT NULL,
    position INTEGER NOT NULL,
    primary_agent_profile_id TEXT NOT NULL DEFAULT '',
    prompt_prefix TEXT DEFAULT '',
    prompt_suffix TEXT DEFAULT '',
    plan_mode INTEGER DEFAULT 0,
    triggers TEXT DEFAULT '{}',                 -- existing JSON; gains new trigger keys
    transitions TEXT DEFAULT '[]',
    -- NEW:
    stage_type TEXT DEFAULT 'custom'
        CHECK (stage_type IN ('work','review','approval','custom')),
    model_profile_override TEXT DEFAULT ''
);

CREATE TABLE workflow_step_participants (
    id TEXT PRIMARY KEY,
    step_id TEXT NOT NULL REFERENCES workflow_steps(id) ON DELETE CASCADE,
    -- Wave 8 dual-scope (deviation, see Alternative F below):
    --   task_id = ''  → template-level row, applies to every task at the step
    --   task_id != '' → per-task override, applies only to that task
    -- Per-task rows take precedence on (role, agent_profile_id) ties.
    task_id TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL CHECK (role IN ('reviewer','approver','watcher','collaborator')),
    agent_profile_id TEXT NOT NULL,
    decision_required INTEGER NOT NULL DEFAULT 0,
    position INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE workflow_step_decisions (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    step_id TEXT NOT NULL,
    participant_id TEXT NOT NULL,
    decision TEXT NOT NULL,                     -- e.g. 'approved', 'rejected', 'comment'
    note TEXT DEFAULT '',
    decided_at TIMESTAMP NOT NULL
);
```

Workflows table gains a `style` UX hint:

```sql
ALTER TABLE workflows ADD COLUMN style TEXT NOT NULL DEFAULT 'kanban'
    CHECK (style IN ('kanban','office','custom'));
```

`workflow.style` is read by the frontend to decide the default presentation mode (simple for office workflows, advanced for kanban). The engine doesn't branch on it.

### 6. UI: two routes, shared body, kanban board unchanged

`/t/:id` (kanban shell, advanced default) and `/office/tasks/:id` (office shell, simple default) both render any task. `/tasks/:id` remains a compatibility alias that redirects to `/t/:id`. The task body — `<TaskHeader>`, `<TaskMetaRail>`, `<TaskBody>` — is shared. The meta rail branches on `workflow.style` (or `step.stage_type`) to show workflow chrome vs. stage chrome. The body's mode (simple / advanced) is per-route default with a URL toggle.

The dockview is runtime-tier UI bound to `task_sessions.current_execution_id`. For tasks where the agent is between turns, `current_execution_id` is null and the dockview renders a dormant view (worktree on disk, no live agentctl).

**The kanban homepage's board UX is unchanged.** Tasks are grouped by workflow step within their workflow. Each workflow appears as its own swimlane; a workspace running both a kanban-style workflow and the Office Default workflow renders two swimlanes side-by-side, each with its own columns. No "filter by kind," no "group by status." Office tasks naturally show up alongside kanban tasks because they both live in `tasks` with a `workflow_id`/`workflow_step_id`.

### 7. `on_agent_error` semantics — fire on the failing task

`on_agent_error` is the trigger that fires after the runs queue exhausts retries (4 attempts at [2m, 10m, 30m, 2h]). Worth a precise specification because failure handling is where systems get muddled.

Fire location: **the failing task** itself, at its current step. The CEO (or whatever target the action specifies) wakes up *on that task*, with the task's full session history visible. The `queue_run` action with default `task_id: this` is the right shape.

Default actions on `on_agent_error` for office workflows:
1. `pause_agent { agent_id: failed_agent_id }` — auto-pause the failing agent (today's consecutive-failure policy).
2. `queue_run { target: workspace.ceo_agent, task_id: this, reason: 'agent_error', payload: {failed_agent_id, error_message} }` — wake the CEO on the failing task.
3. `create_inbox_item { kind: 'agent_error' }` — workspace-level UI surface.

Fallbacks:
- No CEO agent in the workspace: `queue_run` no-ops; the inbox item is the only signal.
- The failing agent IS the CEO: same as no-CEO; user inbox is the only signal.

Worked example, rate-limit on a single task:
1. Agent A's run on task T fails (429 from Anthropic).
2. The runs queue's retry mechanism schedules attempts 1–4 with backoff.
3. Most cases: one retry succeeds, T's session resumes via ACP `session/load`, the user sees a brief delay; no engine involvement. The badge re-enters "Working…" then "finished."
4. All 4 retries exhaust: the engine fires `on_agent_error` on T at its current step. The default actions pause A, queue a run for the CEO on T, and create an inbox item. The CEO wakes up on T's session with full context, decides what to do (reassign, comment, ask user, mark blocked).

Cross-task variant: if a workflow author wants the CEO to wake on its coordination task instead (with the failing task as data, not as the run's location), they configure `task_id: coordination_task_id` on the `queue_run` action. This is the variant for "I want the CEO to evaluate the failing task in the context of the whole workspace." The default fires on the failing task because that's the most actionable view.

### 8. Tool naming convention

Existing MCP tools keep their `_kandev` suffix for disambiguation when agents have multiple MCP servers wired up: `move_task_kandev`, `update_task_kandev`, `list_workflow_steps_kandev`, etc. The `move_task_kandev` tool is the agent-callable primitive for transitions; calling it updates `tasks.workflow_step_id` and the engine fires `on_exit` on the old step + `on_enter` on the new step automatically.

## Consequences

### Positive

- **One execution model.** No discriminator anywhere. The workflow engine is the universal coordinator for task-scoped agent runs; cron-driven handlers feed it from outside; the runtime handles execution. Three layers, clear seams.
- **`IsOfficeTask()` disappears.** Every shared code path operates on tasks-with-workflows. Branching on UX style (`workflow.style`) happens only at the rendering edges.
- **Multi-agent stages are first-class.** The participants list + quorum machinery are workflow-engine features, available to any workflow that wants them. A kanban workflow can opt in (e.g., a code review step with two reviewers) and benefit from the same affordances office uses.
- **The runs queue is universal but well-bounded.** Engine-emitted runs go through the queue's coalescing/idempotency rules; user-initiated launches stay direct. The two paths are explicit.
- **Cross-strategy delegation is a workflow action.** A `create_child_task` action produces a child task with a chosen workflow. The Office Default's `Work` step uses it for delegation; kanban can use it too.
- **The kanban board is unchanged.** Existing per-workflow swimlane UX continues; office workflows appear as new swimlanes. No board-level UX rework.
- **Coordination tasks unify the "agent surveys the workspace" pattern.** Standing tasks with `on_heartbeat` triggers; agents always wake task-scoped.
- **Backward compatibility.** Every kanban workflow today keeps working. New trigger types and participant lists are opt-in; their absence reproduces today's behaviour exactly.

### Negative

- **The workflow engine grows.** Seven new trigger types, three new action types, multi-agent participation, decision tracking, quorum guards, cross-task `task_id` resolver. Roughly +2000 lines of engine code on top of today's. Mitigated by extension-only design: existing surface is unchanged.
- **One engine bug now affects both products.** Counterargument: today's two parallel scheduling planes mean two places to fix similar bugs. Net change is roughly neutral; concentration is easier to harden.
- **Office's existing service code is largely thrown away.** `service.SchedulerIntegration`, `service.QueueRun`, the per-trigger event subscribers — all become trigger handlers feeding the engine. Not literally deleted, but heavily restructured. This is OK because office is dev-only; no migration cost.
- **Coordination tasks add a "ceremonial" task per senior agent.** Some workspaces may find the standing task confusing. Mitigated by clear naming and read-only-style UI on those tasks (no `Done` transition; they live forever).

### Risks

- **Engine generalisation regresses kanban.** Mitigated by: (a) extension-only changes — existing trigger / action signatures untouched; (b) the existing kanban test suite must pass at every checkpoint; (c) we add explicit "kanban-only" regression tests that pin today's behaviour for each existing trigger.
- **Multi-agent quorum is genuinely new state-machine territory.** Decision tracking + transition guards + `clear_decisions` for re-entry need careful design. Mitigated by writing the state-machine spec explicitly in the plan before code moves.
- **The runs queue's two entry paths are subtle.** A new contributor might assume all launches go through the queue and add features that break user-initiated kanban latency. Mitigated by a clear comment on `runtime.Launch` and an integration test that pins kanban Start latency below a threshold.
- **`workflow.style` is a UX hint that could become a behavioural invariant by accident.** Same risk as `IsOfficeTask()`. Mitigated by lint: only frontend code may read `workflow.style`; backend code that branches on it is rejected at review.

## Alternatives considered

### Alternative A: `execution_strategy` discriminator with per-strategy meta tables

The first version of this ADR. Rejected because it preserves the two-execution-models invariant we want to escape. The discriminator would either need to be load-bearing forever (= today's pain in a tidier package) or removed in a future generalisation phase that does the work this ADR proposes anyway.

### Alternative B: Two task tables (`kanban_tasks`, `office_tasks`)

Considered earlier in the design conversation. Rejected because office tasks need to appear on the kanban board (as their own workflow's swimlane), parent/child crosses kinds for delegation, and search/filter/archive want to operate on tasks-as-a-whole. Splitting the row turns every cross-cutting query into a UNION.

### Alternative C: Status quo with discipline

We've been doing this for a month and the pattern is "every office feature ships a kanban regression." Rejected.

### Alternative D: Office completely separate (no shared runtime)

Rejected because the runtime is genuinely shared and good at its job — lifecycle + executor + agentctl + MCP. Splitting it would duplicate a complex layer that has nothing to do with the kanban-vs-office distinction.

### Alternative E: Workspace-scoped wakeups (no coordination task)

Originally proposed: heartbeats and budget alerts wake agents without a task_id, the agent looks at the workspace and decides. Rejected because the user wanted the explicit invariant "agents are spawned by the scheduler on specific tasks; agents do not pick tasks." The coordination task pattern preserves the workspace-survey use case while keeping every wakeup task-scoped.

### Alternative F: Per-task reviewer/approver assignment outside the engine

Wave 8 of the implementation hit a real-world need that the original ADR did not anticipate: the office UX lets users add a reviewer or approver to *one specific task*, not to the workflow template. The original `workflow_step_participants` schema was step-scoped only, so per-task assignment had nowhere to land.

Two paths considered:
1. Keep the legacy `office_task_participants` table forever for per-task overrides, with the engine reading from both tables. Rejected — the engine should have one source of truth for "who's involved at this step, for this task."
2. Add a `task_id` column to `workflow_step_participants` so the same table carries both template-level rows (`task_id=''`) and per-task overrides (`task_id != ''`). Per-task takes precedence on `(role, agent_profile_id)` ties.

We chose #2 in Wave 8. The deviation from the original ADR is documented inline in the schema (Section 5 above). The engine's `ParticipantStore.ListStepParticipants` now takes `(stepID, taskID)` and the repository merges the two row sets with the precedence rule before returning.

## Out of scope for this ADR

- Per-workspace strategy lock UI ("this workspace is office-only"). The unified model serves mixed workspaces without it. May be added later as a UX nicety.
- Workflow-step prompt templating beyond today's prefix/suffix. Office's PromptContext-driven prompt building (per-trigger prompts) keeps working as a separate prompt-construction layer that wraps step prefix/suffix.
- Cross-workspace `parent_id`. Stays same-workspace.
- Editing the participants list while a step is in flight. Static for v1.
- Board grouping changes. Existing per-workflow swimlane UX continues unchanged.

## References

- Conversation thread on 2026-05-04/05 working through the design.
- `docs/specs/task-model-unification/spec.md` — feature spec (the user-visible changes).
- `docs/specs/task-model-unification/plan.md` — phased implementation plan.
- ADR 0003 — `executors_running` as the single source of truth for execution identity. Continues into this ADR's `agent_executions` rename inside the runtime package.
- `docs/specs/office-overview/spec.md` — original office product framing; this ADR re-expresses its goals in workflow terms.
- `docs/specs/office-task-session-lifecycle/spec.md` — IDLE-between-turns model that this ADR formalises into `current_execution_id` on `task_sessions`.
- `docs/specs/office-scheduler/spec.md` — the wakeup queue mechanics this ADR preserves verbatim under the new `runs` name.
- `docs/specs/office-routines/spec.md` — routines feed the engine via task creation; no behavioural change.
- `apps/backend/internal/workflow/engine/` — the engine being generalised.
