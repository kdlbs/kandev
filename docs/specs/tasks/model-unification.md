---
status: draft
created: 2026-05-05
owner: cfl
---

# Task model unification

## Why

Kandev today carries two execution models on top of one task table.
*Kanban* tasks flow through a configurable workflow with per-step
prompts and a single agent runner. *Office* tasks live indefinitely
with comment-driven wakeups, multi-agent stages, and IDLE-between-turns
sessions. The two share `tasks`, `task_sessions`, comments, and the
executor record, which forces every shared code path to branch on
`IsOfficeTask()`. Each branch is a regression risk.

Looking at office and kanban side by side, they're not actually two
models — they're two configurations of the same model. Both are
event-driven state machines over a step graph; both produce agent runs
in response to events; both record outcomes. The differences are which
events count as triggers (kanban: step transitions; office: also
comments, blockers, children, approvals, heartbeats) and how many
agents participate per step (kanban: one runner; office: a primary
plus reviewers / approvers).

The unification: extend the workflow engine to subsume office's
event-driven model. Office becomes a *workflow style* — a set of step
shapes (work / review / approval) plus event triggers and multi-agent
participants — not a separate execution model.

We have no production office users. No data migration; this is
greenfield from office's perspective. The constraints that matter:
**the existing kanban experience must keep working unchanged**, and
**the kanban homepage's per-workflow swimlane UX is unchanged.**

## What

### Tasks always have a workflow

Every task has a `workflow_id` and a `workflow_step_id`. The workflow
defines the lifecycle: linear steps for kanban, event-driven steps with
multi-agent participation for office.

A workflow has a UX hint — `style` — with values `kanban`, `office`,
`custom`. This drives default presentation in the UI; the engine
doesn't branch on it.

Built-in workflow templates ship with kandev:

- **Kanban Default** — Backlog → In Progress → In Review → Done. Single
  primary agent per step. `on_enter` fires the agent. `on_turn_complete`
  may transition. No event triggers.
- **Office Default** — Backlog → Work → Review → Approval → Done.
  - `Work` (`stage_type='work'`): a primary agent (the assignee).
    Triggers: `on_enter`, `on_comment`, `on_blocker_resolved`,
    `on_children_completed`, `on_heartbeat`, `on_agent_error`. Each
    queues a run for the primary. Transitions: agent calls
    `move_task_kandev` (or workflow rule fires) to push to `Review`.
  - `Review` (`stage_type='review'`): no primary; multi-agent
    participants (reviewers) with `decision_required=1` each.
    Triggers: `on_enter` does `clear_decisions` (so quorum starts
    fresh on re-entry) then `queue_run_for_each_participant{
    role: reviewer }`. Transitions: `wait_for_quorum{ all_approve }`
    → `Approval`; `wait_for_quorum{ any_reject }` → back to `Work`.
  - `Approval` (`stage_type='approval'`): same shape as `Review` with
    `role: approver`. Quorum on all_approve → `Done`; any_reject →
    `Work`.
  - `Done` (`stage_type='custom'`): terminal.

Note: there is no `Ship` step. Shipping work (opening a PR, deploying)
happens inside `Work` directly — the assignee opens the PR as part of
its turn — or in a delegated child task with the Kanban Default
workflow.

Users can copy either template and edit it; existing custom kanban
workflows continue to work unchanged.

### New trigger types

The workflow engine learns seven new trigger types in addition to
today's four (`on_enter`, `on_turn_start`, `on_turn_complete`,
`on_exit`):

- `on_comment` — a user (or external) comment landed on the task while
  it's in this step. Trigger context carries the comment id, body, and
  author.
- `on_blocker_resolved` — all blockers on the task cleared.
- `on_children_completed` — all child tasks of this task reached terminal
  state. Payload includes the child summaries (identifiers, last
  comments, PR links if any).
- `on_approval_resolved` — an approval request the task owns resolved.
- `on_heartbeat` — periodic timer (cron-driven) for *background*
  check-ins when nothing else is happening. Fires per-(task, step)
  pair where the step has this trigger configured. Default cadence
  60s. **Most steps don't need this**: comments, blocker clears, child
  completions, approvals all fire their own dedicated triggers
  immediately, so heartbeats are reserved for "wake me even with no
  external event" patterns — primarily on coordination tasks for
  senior-agent oversight.
- `on_budget_alert` — budget threshold crossed. Fires per task with
  payload `{budget_pct, scope}`.
- `on_agent_error` — fired only after the runs queue's retry policy
  exhausts (4 attempts at [2m, 10m, 30m, 2h]). Fires on the *failing
  task* itself.

Each new trigger fires through `Engine.HandleTrigger` plumbing. Steps
that don't configure these triggers behave identically to today.

### Multi-agent steps

A workflow step has a `participants` list (zero or more) in addition to
its `primary_agent_profile_id`. Each participant has:

- `role` — `reviewer`, `approver`, `watcher`, `collaborator`.
- `agent_profile_id` — the actual agent.
- `decision_required` — boolean; if true, the agent's run must record a
  verdict before quorum can be evaluated.

A step with an empty participants list is a single-agent step =
today's kanban behaviour, unchanged.

A step with participants supports new actions:

- `queue_run` (generalises today's `auto_start`) — targets `primary`,
  `participant_role:<role>`, or a specific `agent_profile_id`. Optional
  `task_id` resolver: defaults to `this` (trigger's task) but can take
  a literal task id for cross-task wakeups.
- `queue_run_for_each_participant` — fans out runs at `on_enter` (or
  any other trigger) for all participants matching a role.
- `wait_for_quorum` — a transition guard that blocks until N-of-M
  decisions are recorded. Used as `if:` clauses on transitions:
  `if:'wait_for_quorum{role:reviewer, threshold:all_approve}'` then
  `goto:'approval'`.
- `clear_decisions` — clears `workflow_step_decisions` rows for the
  trigger's `(task, step)` pair. Used by `Review.on_enter` so quorum
  starts fresh when the task re-enters Review after a rejection.
- `record_participant_decision` — callback used by the office service
  when a reviewer's run completes with a verdict. Records into
  `workflow_step_decisions`.

### Agent-driven transitions: `move_task_kandev`

Agents transition tasks by calling the existing `move_task_kandev` MCP
tool. The repo updates `tasks.workflow_step_id`; the engine fires
`on_exit` on the old step and `on_enter` on the new step automatically.

This is the same path as engine-driven transitions (where a transition
rule on `on_turn_complete` fires). Agent-driven and rule-driven
transitions both go through the same plumbing.

A reviewer rejecting a Review:
1. Reviewer's run completes with verdict `rejected`.
2. `record_participant_decision` writes into `workflow_step_decisions`.
3. `wait_for_quorum{any_reject}` evaluates true; transition to `Work`
   fires.
4. `Review.on_exit`, `Work.on_enter`. The work step's `on_enter`
   queues a run for the assignee with the rejection feedback in the
   trigger payload.
5. Assignee fixes the issues, calls `move_task_kandev` to push back to
   Review.
6. `Work.on_exit`, `Review.on_enter`. The review step's `on_enter`
   first calls `clear_decisions` to reset quorum, then
   `queue_run_for_each_participant{role:reviewer}` re-queues runs for
   all reviewers.

### One generic `runs` queue

`office_runs` is renamed `runs`. Universal queue for engine-emitted
launches. Every `queue_run` action creates a row. Coalescing (5s
window for same agent + reason), idempotency (24h dedup window),
per-agent serialisation (one claimed run per agent), cooldown
(per-agent, default 10s), and atomic task checkout (one agent per
task at a time) all apply — same machinery as today's office_runs.

User-initiated launches (clicking Start on a kanban task) bypass the
queue and call `runtime.Launch` directly — those don't need
coalescing and shouldn't pay the scheduler's tick latency.

`run_events` (the audit log per run) keeps its current shape and
event-streaming behaviour. WS event subjects (`office.run.queued`,
`office.run.processed`, `office.run.event_appended.<run_id>`) keep
the `office.` prefix for now to avoid frontend churn; we can rename
later if desired.

### Cron-driven trigger handlers

Three handlers run on a shared cron tick loop:

- **Heartbeat handler.** Iterates `(task, step)` pairs where the step
  has `on_heartbeat` configured AND the agent's runtime allows
  (cooldown, status). Fires `Engine.HandleTrigger(TriggerOnHeartbeat,
  task)`. The step's action emits a run.
- **Budget handler.** Detects budget threshold crossings (workspace,
  project, agent). For each affected task, fires
  `Engine.HandleTrigger(TriggerOnBudgetAlert, task)`.
- **Routine handler.** Reads `office_routines` / `office_routine_triggers`
  / `office_routine_runs` exactly as today. On firing it creates a real
  task with the routine's template; the new task's `on_enter` does the
  rest. (Routines are upstream of the engine; today's routines spec
  needs no behavioural change.)

There is no "workspace-scoped scheduler" for waking agents without a
task. Every wakeup is task-scoped.

### Coordination tasks for senior-agent oversight

The CEO (and other senior agents) need to do workspace-level
reasoning: survey active tasks, decide priorities, intervene on
failures, allocate budget. Today this happened via "wake the CEO with
no task." In the unified model we model it as a real task: a
**coordination task**.

A coordination task is a real, standing task created at office
workspace setup, assigned to a senior agent, on a workflow whose step
has `on_heartbeat` (and optionally `on_budget_alert`,
`on_agent_error`) configured. Cron fires the heartbeat trigger; the
senior agent wakes on the coordination task with workspace-summary
context (active runs, pending wakeups, recent failures, budget) and
can comment on it, delegate child tasks from it, transition it.

Multiple senior roles → multiple coordination tasks (CEO has
"Workspace coordination," QA Lead has "QA coordination," etc.). Office
onboarding creates them when the workspace is set up. A "Done"
transition isn't expected on a coordination task; it's a standing
thread.

### `on_agent_error` — fire on the failing task

`on_agent_error` fires on the failing task itself, after the runs
queue exhausts retries (4 attempts with backoff). The CEO (or chosen
target) wakes up *on that task*, in that task's session, with full
history visible. Default actions for office workflows:

```json
[
  { "kind": "pause_agent", "agent_id": "{failed_agent_id}" },
  { "kind": "queue_run",
    "target": "agent_profile_id:{workspace.ceo_agent}",
    "task_id": "this",
    "reason": "agent_error",
    "payload": { "failed_agent_id": "...", "error_message": "..." } },
  { "kind": "create_inbox_item", "kind": "agent_error" }
]
```

Fallbacks: no CEO in the workspace → `queue_run` no-ops; the inbox
item is the only signal. The failing agent IS the CEO → same
fallback.

The default fires on the failing task because that gives the CEO
maximum context. For workflows that prefer a coordination-task view,
override the action's `task_id: "{coordination_task.id}"`.

### Two routes, shared body. Kanban board unchanged.

`/t/:id` (kanban shell, advanced default) and `/office/tasks/:id`
(office shell, simple default) both render any task. The body —
`<TaskHeader>`, `<TaskMetaRail>`, `<TaskBody>` — is shared. The meta
rail branches on `workflow.style` for default chrome; the body's mode
(simple / advanced) is per-route default with a URL override toggle.
`/tasks/:id` remains a compatibility alias that redirects to `/t/:id`.

The dockview (file tree, terminal, agent chat, changes) binds to
`task_sessions.current_execution_id`. When the agent is between turns
the dockview renders dormant (worktree readable, no live agentctl).

**The kanban homepage's board UX is unchanged.** Tasks are grouped by
workflow step within their workflow. Each workflow appears as its own
swimlane; an office workflow shows up as a new swimlane with its own
columns (Backlog | Work | Review | Approval | Done). Office tasks are
draggable across the columns the same way kanban tasks are draggable
across their workflow's columns; the drag fires the same `task.moved`
event the engine consumes.

### Cross-strategy delegation

A new workflow action `create_child_task` queues a new task as a child
of the current one with a chosen workflow. The Office Default's
`Work` step uses it for delegation: the agent emits a `delegate`
signal (a tool call defined in the office MCP toolset), the engine
creates a child task with the Kanban Default workflow, the worker
runs it, the existing `on_children_completed` trigger fires on the
parent and posts a bridged comment ("KAN-43 done → PR #198").

Reverse path (kanban task promoted to office) is a workflow swap:
change the task's `workflow_id` and `workflow_step_id` to match an
office workflow's starting step. Identity, comments, sessions are
preserved.

## Scenarios

- **GIVEN** a fresh workspace with the Kanban Default workflow,
  **WHEN** the user creates a task and clicks Start, **THEN** the agent
  runs through the linear steps as today. No regression. No new
  triggers fire because the kanban template doesn't configure them.

- **GIVEN** a workspace that adopts the Office Default workflow,
  **WHEN** the user creates a task assigned to the CEO, **THEN** the
  task lands in the `Work` step. The CEO runs `on_enter`. When the
  user posts a comment, `on_comment` fires and the CEO runs again
  through the unified `runs` queue.

- **GIVEN** a task in the Office Default's `Review` step with two
  reviewers configured, **WHEN** the step is entered, **THEN**
  `on_enter` first calls `clear_decisions`, then queues a run for each
  reviewer in parallel. As each reviewer's run completes with a
  verdict, `record_participant_decision` writes to
  `workflow_step_decisions`. When all `decision_required` reviewers
  have approved, `wait_for_quorum:all_approve` clears and the
  transition to `Approval` fires.

- **GIVEN** the Office Default's `Review` step, **WHEN** any reviewer
  rejects with feedback, **THEN** `wait_for_quorum:any_reject` fires,
  the task moves back to `Work`, and the primary agent receives the
  feedback as the next trigger context. The assignee fixes, calls
  `move_task_kandev` to push back to Review; Review's `on_enter`
  clears decisions and re-runs all reviewers.

- **GIVEN** the workspace has a CEO with a coordination task,
  **WHEN** the heartbeat handler ticks every 60s, **THEN** the
  coordination task's `on_heartbeat` trigger fires, queues a run for
  the CEO with workspace-summary context (active runs, pending
  wakeups, recent failures, budget), and the CEO wakes up on the
  coordination task. The CEO can comment, delegate, or take no
  action.

- **GIVEN** a single-task workspace where agent A is running on task
  T, **WHEN** Anthropic returns 429 (rate limit), **THEN**:
    1. The runs queue retries with backoff (2m, 10m, 30m, 2h).
    2. Most cases: a retry succeeds; the agent resumes via ACP
       `session/load`; the user sees a brief delay; no engine
       involvement.
    3. If all 4 retries exhaust, the engine fires `on_agent_error`
       on T at its current step. Default actions: pause A, queue a
       run for the CEO on T, create an inbox item.
    4. The CEO wakes on T with full session history visible and
       decides next steps (reassign, comment, ask user, mark
       blocked).
    5. If no CEO exists, only the inbox item surfaces; the user
       picks it up.

- **GIVEN** a CEO agent in the Office Default `Work` step decides to
  delegate, **WHEN** it emits a `create_child_task` signal, **THEN**
  the engine creates a child task with `parent_id = T_office.id` and
  the Kanban Default workflow. The worker runs it, opens a PR,
  completes. `on_children_completed` fires on the parent and posts a
  bridged comment summarising the result.

- **GIVEN** a workspace with both a kanban workflow and the Office
  Default workflow in use, **WHEN** the user opens the kanban
  homepage, **THEN** they see two swimlanes — one per workflow —
  each with its own columns. Office tasks appear in the office
  workflow's columns; kanban tasks appear in the kanban workflow's
  columns. Same per-workflow swimlane UX as today.

- **GIVEN** a routine "Daily Dep Update" with cron `0 9 * * *` and
  assignee Frontend Worker, **WHEN** the clock reaches 09:00 UTC,
  **THEN** a real task is created with the routine's template, the
  task's first step's `on_enter` queues a run for Frontend Worker.

- **GIVEN** the user opens any task's advanced mode, **WHEN** the
  task's primary agent has an active execution, **THEN** the dockview
  binds to `task_sessions.current_execution_id` and shows live state.
  When the execution ends, the dockview renders dormant.

## Out of scope

- Per-workspace strategy lock UI. The unified model serves mixed
  workspaces without one.
- Editing participants while a step is in flight. Static for v1; new
  participants take effect at the next step entry.
- Workspace-scoped wakeups (no task_id). Every wakeup is task-scoped;
  workspace-level reasoning lives on coordination tasks.
- Cross-workspace `parent_id`.
- Auto-resume an office agent on user comment when the assignee is
  paused (`office-optimistic-comments` Phase 2 territory).
- Editing comments after submission, richer attachments — separate
  features.
- Migration of existing office data. None exists in production; the
  dev DB is wiped on rebuild.
- Board grouping changes. Existing per-workflow swimlane UX
  unchanged.

## Open questions

These are flagged in the spec; the plan resolves them.

- **Quorum semantics edge cases.** When a `decision_required` reviewer
  is removed mid-flight, does the quorum advance? When a reviewer's
  run fails after recording a partial verdict, does it retry or accept
  the verdict? The plan picks defaults; we may revisit.

- **Coordination task creation policy.** Does office onboarding always
  create a CEO coordination task, or only if the workspace has a CEO
  agent? Plan default: only when a CEO agent exists; user can create
  coordination tasks for other senior roles manually.

- **`on_heartbeat` cadence config.** Per-step config drives the
  default cadence; per-agent override via `office_agent_runtime`.
  Resolved: most steps don't configure `on_heartbeat` at all — event
  triggers (`on_comment`, etc.) drive runs immediately. The 60s
  default applies only to coordination tasks. Heartbeats are NOT the
  latency floor for new work; the runs scheduler's event-driven claim
  path (plan B3.5) keeps the latency floor under 100ms.

- **Backward compat for office service's existing event subscribers.**
  The comment-created subscriber today posts directly to `office_runs`.
  After unification, it must instead trigger
  `Engine.HandleTrigger("on_comment", task)` and let the engine emit
  `queue_run`. The plan tracks this rewiring carefully because the
  office UI's reactive paths depend on the resulting WS events.
