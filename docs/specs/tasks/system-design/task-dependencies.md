---
status: current
system: tasks
requirements:
  - REQ-TASKS-TASK-DEPENDENCIES-001
created: 2026-08-09
owners:
  - kandev
---
# Task Dependencies and Auto-Start Chains System Design

## Purpose and boundaries

This design record preserves the technical source for the capability mapped to REQ-TASKS-TASK-DEPENDENCIES-001 while the task system completes its migration.

## Requirement mapping

| Requirement | Design source |
| --- | --- |
| REQ-TASKS-TASK-DEPENDENCIES-001 | Migrated legacy design detail below |

## Migrated design source

## Why

A user who splits work into ordered steps — "scaffold the schema", then "write
the API on top of it", then "wire the UI" — has no way to express that order in
Kandev. The parent/child subtask hierarchy says *"B is part of A"*, never *"B
cannot start until A finishes"*. So the user either starts everything at once
and lets agents collide in the same repository, or babysits the board and
presses Start by hand each time a predecessor lands. Both defeat the point of
running agents unattended.

## What

- A task SHALL be able to declare that it **depends on** one or more other
  tasks. The relationship is peer-to-peer and independent of `parent_id`: a
  dependency may exist between two unrelated tasks, between siblings, or
  between a task and its own child.
- The dependency graph is a **general DAG**, not a linear chain. A task may
  have any number of predecessors and any number of dependents. Cycles SHALL be
  rejected at edge-creation time with the offending path surfaced to the user.
- A task with at least one unresolved dependency is **blocked**. Blocked SHALL
  be visible on the Kanban card, in the task detail view, and in the task API
  payload.
- A blocked task SHALL NOT be auto-started by any automated path — workflow
  `on_enter: auto_start_agent`, WIP queue promotion, integration watchers, or
  dependency resolution itself. The gate applies to every automated launch, not
  only to the dependency-resolution path.
- A user MAY still start a blocked task manually. Manual start is an explicit
  override, not a bug; the UI names it as such.
- A task MAY carry the intent **"start automatically when unblocked"**. When
  every dependency of such a task resolves successfully, Kandev launches it
  exactly once with the agent profile, executor, executor profile, prompt, and
  plan-mode choice recorded when the intent was created.
- Setting that intent on each link of a chain produces sequential execution:
  A finishes → B starts → B finishes → C starts. No separate "chain" entity
  exists; a chain is a path through the DAG.
- Auto-start on unblock respects every existing admission rule. A task whose
  dependencies have resolved but which is not WIP-admitted (`wip_admitted =
  false`) stays queued and launches when the WIP queue promotes it. Dependency
  resolution grants **eligibility**, never a bypass.
- A dependency resolves only on **successful** completion of the predecessor.
  `FAILED`, `CANCELLED`, and archival do not resolve a dependency. The
  dependent task stays blocked and its blocked reason changes to name the
  failed predecessor, so a chain never proceeds on a failed step.
- A dependency-failed state SHALL raise a notification and SHALL require
  explicit human action to clear (retry the predecessor, remove the edge, or
  start the dependent manually). Kandev never auto-retries a failed
  predecessor and never silently drops the edge.
- Users declare dependencies in the **task-create dialog** ("Depends on") and
  manage them afterwards over MCP. The task detail view deliberately has no edge
  editor: dependencies describe how work was planned, and the surfaces that read
  them (card badge, chip, graph) stay read-only.
- An open task SHALL show a **dependency chip** in the status row directly above
  its chat composer, alongside the PR status chip. The chip reports both
  directions — the tasks this task is blocked by, and the tasks it blocks — and
  opens to a list of them with their states, each navigable to that task. It is
  absent when the task has no edges in either direction.
- Agents SHALL be able to declare and manage dependencies over MCP: set them
  when creating a task, and add or remove them on an existing task. The tools
  SHALL teach when a dependency is the right relationship rather than a subtask,
  because an agent that reaches for `parent_id` to express ordering gets neither
  the gate nor the chain.
- Creating a task with dependencies and an agent-start request SHALL record the
  start as a start-when-unblocked intent rather than launching immediately. An
  automated caller that asks for both is describing a chain step, not a task to
  run now.
- Deleting a task removes its edges in both directions. Dependents whose last
  unresolved dependency was the deleted task become unblocked, and a
  `start-when-unblocked` intent on such a dependent SHALL NOT fire — deletion
  is not success.

### Relationship to existing mechanisms

This feature deliberately extends four shipped mechanisms rather than adding
parallel ones. The reuse is part of the contract because the guarantees below
(single launch, restart survival, WIP interaction) are inherited from them.

| Existing mechanism | Role here |
|---|---|
| `task_blockers` table + BFS cycle detection (`office/dashboard.AddTaskBlocker`) | The dependency edge store and cycle validator, promoted from Office-only to a core task relationship. |
| Deferred launch intent (`tasks.metadata.deferred_launch`, claimed atomically) | Storage and idempotent consumption of "start automatically when unblocked". |
| The single auto-start chokepoint (`orchestrator.autoStartTaskForStep`) | Where the dependency gate lives and where dependency-triggered launches enter, so the existing auto-start claim guard prevents double launches. |
| WIP admission and queue promotion ([`tasks/wip-limit-pull-system`](wip-limit-pull-system.md)) | Decides whether an unblocked, auto-start-intent task launches now or on promotion. |

Two existing behaviors are intentionally *not* changed:

- **Parent/child completion signalling** ([`tasks/subtask-completion-trigger`](../requirements/subtask-completion-trigger.md))
  fires `on_children_completed` when all children reach any terminal state,
  including `FAILED`. Dependency resolution uses a stricter definition
  (success only). The two must not be unified.
- **Office's blocker reaction** (`office/scheduler.cascadeBlockersResolved`)
  queues an Office run to the blocked task's *assignee* when a blocker's status
  becomes done. It stays as-is. For a task that qualifies for both reactions,
  the shared auto-start claim guarantees exactly one launch.

## Data model

### `task_blockers` (existing, reused)

```text
task_id          TEXT      the dependent task (blocked)
blocker_task_id  TEXT      the predecessor task (blocking)
created_at       TIMESTAMP
PRIMARY KEY (task_id, blocker_task_id)
CHECK (task_id != blocker_task_id)
```

- The row reads "`task_id` is blocked by `blocker_task_id`".
- Edges are workspace-local: both tasks MUST belong to the same workspace.
- No new column is added. Edge deletion on task delete/archive is enforced by
  the owning repository, not by a SQLite `ON DELETE CASCADE`, because the table
  predates the foreign key.
- The table currently ships from the Office repository's
  `createTaskExtensionTables`. Reading and writing it becomes a core task
  concern; the Office reader methods stay valid against the same rows.

### `tasks.metadata.deferred_launch` (existing, reused)

The start-when-unblocked intent is a `deferred_launch` metadata record on the
**dependent** task, identical in shape to the one WIP overflow already
persists: resolved agent profile, executor, executor profile, prompt,
plan-mode choice, priority, and attachments. It is claimed atomically before
launch and restored on launch failure.

A task therefore has at most one launch intent regardless of how many
predecessors it has. "Which edge triggers the start" is not a question this
model can ask — the answer is always "the last one to resolve".

### Derived state (not persisted)

`blocked`, `blocked_reason`, and the predecessor list are computed from
`task_blockers` plus the predecessors' own state on every read. There is no
denormalized `is_blocked` column: a stale copy of it would gate launches
incorrectly, which is the one failure this feature must not have.

A predecessor is **resolved** when it satisfies the existing successful-completion
convention: `state = COMPLETED`, or resident in a final workflow step whose name
is `Done`, `Complete`, `Completed`, or `Approved` (`wfmodels.IsTerminalStepName`,
which also persists `state = COMPLETED`).

A predecessor is **failed** when `state` is `FAILED` or `CANCELLED`.

A predecessor is **pending** in every other case, including archived.

## API surface

### HTTP

Existing Office-scoped routes are promoted to task-scoped equivalents. The
Office paths remain for compatibility with the Office task pane.

```text
POST   /api/v1/tasks/:id/dependencies          {"depends_on_task_id": "<id>"}
DELETE /api/v1/tasks/:id/dependencies/:depId
```

- `POST` returns `409` with `{"error": "...", "cycle": ["A","B","C","A"]}` when
  the edge would close a cycle. The `cycle` body shape matches what
  `BlockersPicker` already parses.
- `POST` returns `400` for a self-edge or a cross-workspace edge.
- `DELETE` of an absent edge is a success no-op.

### Task payload

Task DTOs returned over HTTP, WebSocket boot, WebSocket events, and MCP gain:

```json
{
  "blocked": true,
  "blocked_reason": "pending",
  "depends_on": [
    { "id": "task-a", "title": "Add the schema migration", "state": "COMPLETED", "status": "resolved" },
    { "id": "task-b", "title": "Expose the edges on the API", "state": "TODO", "status": "pending" }
  ],
  "blocks": [{ "id": "task-d", "title": "Render the chip", "state": "TODO" }],
  "start_when_unblocked": true
}
```

- `blocked_reason` is one of `"pending"` (at least one predecessor unfinished),
  `"failed"` (no predecessor unfinished, at least one failed), `"unknown"` (the
  dependency store could not be read, so the gate failed closed), or omitted
  when `blocked` is false. A client renders `"unknown"` as blocked without a
  named cause; it must not present it as resolved.
- `depends_on` lists direct predecessors only, in `created_at` order.
- `blocks` lists direct dependents only, in `created_at` order. It exists so the
  dependency chip can show both directions without a second round trip, and so
  an agent can see what is waiting on it.
- Both lists carry enough per-entry detail for the chip to render without
  fetching each task: id, title, and state. Neither list is transitive.
- The projection is **absent, not empty, when a payload does not compute it**.
  Every boot payload and list read computes it; lightweight `task.updated` events
  do not. A client that reads an omitted projection as "no edges" erases the real
  edges, so absent MUST mean "unknown" and an explicit empty list MUST mean "no
  edges". A reader that needs certainty re-reads the task.
- `start_when_unblocked` reflects the presence of a launch intent tied to
  dependency resolution. It is read-only on the task payload; it is set through
  the create request or the dependency picker.

### Task create

`POST /api/v1/tasks` already accepts `blocked_by: ["<id>", …]`. It gains:

```json
{ "blocked_by": ["task-a"], "start_when_unblocked": true }
```

When `start_when_unblocked` is true **and `blocked_by` is non-empty**, the
create request's agent/executor/prompt resolution is recorded as the deferred
launch intent instead of launching now.

`start_when_unblocked: true` with an empty `blocked_by` records no intent. The
task is born unblocked, so there is no later moment at which it would fire; a
create that also asked to start an agent launches immediately, exactly as it
would without the flag, and one that did not launches nothing. The flag defers
a start, it never invents one. Adding a dependency to that task afterwards
gates its *automated* starts but does not retroactively create an intent.

When `blocked_by` is non-empty and `start_when_unblocked` is omitted, it defaults
to the request's agent-start intent: a create that asked to start an agent
records the intent, and a create that did not records no intent. The response
reports `started` and `start_when_unblocked` so the caller never has to infer
which happened. This rule is what makes an automated caller's habitual
`start_agent: true` build a chain instead of launching every step at once.

### WebSocket events

- `task.updated` with `fields: ["dependencies"]` on edge add/remove.
- `task.dependencies_resolved` — published when the last unresolved dependency
  of a task resolves successfully. Payload: `{task_id, resolved_by_task_id}`.
  This is the event whose handler enters the auto-start chokepoint.
- `task.dependency_failed` — published when a predecessor of a blocked task
  reaches `FAILED` or `CANCELLED`. Payload:
  `{task_id, failed_task_id, failed_state}`.

### Notifications

A new notification event type `task.dependency_failed` follows the existing
constants in `internal/notifications/service`. It fires once per
(dependent, predecessor) failure transition.

### MCP

`create_task_kandev`'s handler already reads a `blocked_by` array from its
payload, but the tool schema never declares it, so no agent can discover or pass
it. Declaring it is the minimum; the tools below make chains expressible.

- `create_task_kandev` declares `blocked_by` (array of task IDs) and
  `start_when_unblocked` (boolean).
- **`blocked_by` plus `start_agent: true` records a start-when-unblocked intent
  and launches nothing now.** `start_agent` defaults to true and every agent
  passes it by habit, so the alternative — launching a blocked task immediately —
  would silently break every chain an agent tries to build. The create response
  reports `started: false` with `start_when_unblocked: true` so the caller can
  see what happened. Passing `start_when_unblocked: false` alongside
  `blocked_by` creates the edges with no launch intent at all.
- `add_task_dependency_kandev` — `{task_id?, depends_on_task_id}`. `task_id`
  defaults to the calling task. Returns the resulting `depends_on` list.
  Rejects a cycle with the cycle path, a self-edge, and a cross-workspace edge.
- `remove_task_dependency_kandev` — `{task_id?, depends_on_task_id}`. Removing
  an absent edge succeeds. Removing the last edge unblocks the task without
  launching it.
- `list_related_tasks_kandev` is the read side and already returns `blockers` and
  `blocked_by`. Those groups become populated in Kanban-only installs, where
  they are empty today because the blocker repository is wired inside the
  Office-enabled branch.
- Tool descriptions SHALL state when to use a dependency instead of a subtask: a
  subtask means "part of", a dependency means "not until". Decomposing a plan
  into ordered phases is N sibling tasks chained with `blocked_by` and
  `start_when_unblocked`, not N subtasks started at once. The description SHALL
  also state that a failed predecessor halts the chain and needs human action, so
  an agent does not build a chain expecting it to self-heal.
- An agent cannot set `start_when_unblocked` on a task it did not create; it uses
  the dependency tools for edges and leaves launch intent to the task's owner.

## State machine

Dependency state is derived, so the machine below describes the *dependent*
task's gating status rather than a persisted column.

```text
                    edge added                predecessor resolved (not last)
   unblocked ──────────────────────> blocked ──────────────────────────> blocked
       ^                              │  ^                                  │
       │ last edge removed            │  │ predecessor reopened             │
       │ or last predecessor resolved │  │                                  │
       └──────────────────────────────┘  └──────────────────────────────────┘
                                         │
                        predecessor FAILED/CANCELLED
                                         v
                              blocked (reason=failed)
                                         │
                    predecessor retried and resolved, OR edge removed
                                         v
                                     unblocked
```

Transitions and their actors:

| From | To | Trigger | Actor |
|---|---|---|---|
| unblocked | blocked | edge created | user, agent (`blocked_by`), API |
| blocked | blocked | a predecessor resolves, others remain | system |
| blocked | blocked (reason=failed) | a predecessor enters `FAILED`/`CANCELLED` | system |
| blocked | unblocked | last unresolved predecessor resolves successfully | system |
| blocked | unblocked | last blocking edge removed | user, agent, API |
| unblocked | blocked | a resolved predecessor is reopened to a non-terminal state | user, system |

On the `blocked -> unblocked` transition caused by successful resolution, and
only then, Kandev publishes `task.dependencies_resolved` and evaluates
auto-start:

```text
task.dependencies_resolved
  -> task has deferred launch intent?      no  -> stop (task is merely unblocked)
  -> task is WIP-admitted?                 no  -> stop; WIP promotion will retry
  -> claim deferred launch intent          lost -> stop; the winner launches
  -> launch with the recorded parameters
```

Removing the last edge unblocks the task but does **not** fire auto-start: the
intent is consumed by dependency *resolution*, not by the absence of
dependencies. A user who removes the edge is taking manual control.

## Permissions

- Creating or removing an edge requires the same authorization as updating both
  tasks; both must be in a workspace the caller can access. Cross-workspace
  edges are rejected, so no new cross-tenant surface exists.
- Agents create edges through task creation (`blocked_by`), the two MCP tools
  (`add_task_dependency_kandev`, `remove_task_dependency_kandev`), and the
  existing Office blocker endpoints. They cannot set `start_when_unblocked` on
  a task they did not create.
- Every one of those paths authorizes BOTH task IDs against the caller's
  identity before any read or write, and denials surface as not-found so an
  edge attempt cannot be used to probe for another user's tasks. In a
  task-bound MCP session the dependent end is the session's own task, taken
  from the `AgentExecution` rather than from an agent-supplied identifier; the
  predecessor is necessarily another task and is authorized on its own.
- The dependency gate is not a permission: a user with start rights can start a
  blocked task manually.

## Failure modes

| Condition | Required behavior |
|---|---|
| Edge would create a cycle | `409` with the cycle path; no row written. |
| Edge is a self-edge or crosses workspaces | `400`; no row written. |
| Predecessor enters `FAILED`/`CANCELLED` | Dependent stays blocked with `blocked_reason: "failed"`; `task.dependency_failed` published; notification raised; no launch. |
| Predecessor is archived while a dependent is blocked | Treated as pending, not resolved. The dependent stays blocked and the reason names the archived predecessor. |
| Predecessor is deleted | Edge is removed. The dependent may become unblocked but its launch intent does not fire. |
| Dependent has no deferred launch intent | It unblocks and waits for a user or for its step's normal `on_enter` behavior on a later move. |
| Dependent is not WIP-admitted when its dependencies resolve | No launch. The existing WIP promotion path launches it once capacity opens; the intent survives. |
| Dependent's step has a WIP limit that is full | Same as above — the dependency gate and the WIP gate are independent and both must pass. |
| Two paths race to launch the same dependent (dependency resolution and WIP promotion) | The existing atomic deferred-launch claim admits one. The loser logs and stops. |
| Launch fails after the claim | The claim is restored so the task remains retryable and visibly unstarted, matching the WIP-overflow contract. |
| Predecessor lookup fails while evaluating resolution | No launch, warning logged. A later state change on any predecessor re-evaluates. |
| Blocker repository is unavailable | Edge writes fail with an error. Reads that fail return the task as **blocked with reason unknown** rather than unblocked — the gate fails closed, because failing open would launch work whose predecessor may not have run. |
| A predecessor is reopened after the dependent already launched | No retroactive stop. The dependent shows blocked again; the running session is untouched. |
| Dependency graph is large | Cycle detection keeps its existing BFS walk limit; a walk that exceeds the limit rejects the edge rather than accepting an unverified one. |

## Persistence guarantees

- Dependency edges are durable and survive restart.
- Start-when-unblocked intents are durable (task metadata) and survive restart,
  including across a restart that happens between a predecessor's completion
  and the dependent's launch: startup reconciliation re-evaluates blocked tasks
  with intents and launches those whose dependencies are satisfied.
- Derived `blocked` state is never persisted and is recomputed on read, so a
  restart cannot resurrect a stale gate.
- An intent is consumed at most once. A restart between claim and launch leaves
  the task unstarted with the claim restored, not double-started.
- `task.dependencies_resolved` and `task.dependency_failed` are in-memory
  events and are not replayed after restart; the startup reconciliation above
  is the recovery path, and a failed-predecessor state is re-derived on read.

## Scenarios

Edges and blocking:

- **GIVEN** tasks A and B in the same workspace, **WHEN** the user adds "B
  depends on A", **THEN** B's card shows a blocked badge naming one
  predecessor and B's payload reports `blocked: true`, `blocked_reason:
  "pending"`, `depends_on: ["A"]`.
- **GIVEN** "B depends on A", **WHEN** the user tries to add "A depends on B",
  **THEN** the request is rejected with the cycle path `A → B → A` and no edge
  is created.
- **GIVEN** "B depends on A" and "C depends on B", **WHEN** the user tries to
  add "A depends on C", **THEN** the request is rejected with the cycle path
  `A → C → B → A`.
- **GIVEN** tasks in two different workspaces, **WHEN** the user tries to link
  them, **THEN** the request is rejected and no edge is created.
- **GIVEN** B depends on A and on X, **WHEN** A completes successfully,
  **THEN** B remains blocked with `depends_on: ["A","X"]` and
  `blocked_reason: "pending"`.

Gating:

- **GIVEN** B depends on an unfinished A and B's workflow step has
  `on_enter: auto_start_agent`, **WHEN** B is moved into that step, **THEN** no
  session is created and the skip is logged.
- **GIVEN** B depends on an unfinished A and B is queued for a WIP-limited
  auto-start step, **WHEN** capacity opens and B is promoted, **THEN** B is
  admitted, its queued badge clears, and no session is created.
- **GIVEN** B depends on an unfinished A, **WHEN** the user clicks Start on B,
  **THEN** B starts, because manual start is an explicit override.

Auto-start and chaining:

- **GIVEN** B depends on A, B has a start-when-unblocked intent, and B is
  WIP-admitted, **WHEN** A completes successfully, **THEN** exactly one session
  is created for B using the intent's recorded agent profile, executor,
  executor profile, prompt, and plan-mode choice.
- **GIVEN** A → B → C each with a start-when-unblocked intent, **WHEN** A
  completes, **THEN** B starts; **WHEN** B completes, **THEN** C starts, and A
  is never restarted.
- **GIVEN** B depends on A and B has **no** start-when-unblocked intent,
  **WHEN** A completes, **THEN** B unblocks, no session is created, and B's
  card shows no blocked badge.
- **GIVEN** B depends on A, B has an intent, and B is queued (not WIP-admitted)
  for a full step, **WHEN** A completes, **THEN** no session is created and B
  stays queued; **WHEN** capacity later opens and B is promoted, **THEN**
  exactly one session is created.
- **GIVEN** B depends on A and B has an intent, **WHEN** A completes and the
  backend restarts before B launches, **THEN** after startup exactly one
  session is created for B.
- **GIVEN** B depends on A and on X, both with A and X unfinished, **WHEN** A
  completes and then X completes, **THEN** B launches exactly once, on X's
  completion.
- **GIVEN** B depends on A and B has an intent, **WHEN** the user removes the
  edge instead of A completing, **THEN** B unblocks and no session is created.

Failure paths:

- **GIVEN** B depends on A and B has an intent, **WHEN** A ends `FAILED`,
  **THEN** B stays blocked with `blocked_reason: "failed"`, no session is
  created, `task.dependency_failed` is published, and a notification is raised.
- **GIVEN** B is blocked with `blocked_reason: "failed"` after A failed,
  **WHEN** A is retried and completes successfully, **THEN** B unblocks and its
  intent fires exactly once.
- **GIVEN** B depends on A and A is `CANCELLED`, **WHEN** the dependency state
  is read, **THEN** B is blocked with `blocked_reason: "failed"`.
- **GIVEN** B depends on A, **WHEN** A is archived, **THEN** B stays blocked
  and A is reported as a pending predecessor.
- **GIVEN** B depends on A and B has an intent, **WHEN** A is deleted, **THEN**
  the edge is removed, B unblocks, and no session is created.
- **GIVEN** A has completed and B has launched, **WHEN** A is reopened to
  `IN_PROGRESS`, **THEN** B reports blocked again and B's running session is
  not stopped.
- **GIVEN** the blocker store returns an error, **WHEN** an automated path
  evaluates whether a task may auto-start, **THEN** the launch is skipped and a
  warning is logged.

UI:

- **GIVEN** a blocked task on the Kanban board, **WHEN** the user hovers or taps
  its blocked badge, **THEN** the predecessor titles and their states are
  listed, and the badge coexists with the WIP queued badge without truncating
  either.
- **GIVEN** the task-create dialog, **WHEN** the user opens "Depends on", **THEN**
  the board's other tasks are offered as candidates, selecting one shows a
  removable chip, and the copy states that starting an agent will run the task
  automatically once every selection completes.
- **GIVEN** a task created with dependencies and an agent start, **WHEN** creation
  succeeds, **THEN** no session is started and the response reports
  `start_when_unblocked: true`.
- **GIVEN** an open task B where B depends on A and D depends on B, **WHEN** the
  task view renders, **THEN** a dependency chip appears in the status row above
  the composer next to the PR status chip, and opening it lists A under
  blocked-by and D under blocks, each with its state and each navigable.
- **GIVEN** an open task with no dependency edges in either direction, **WHEN**
  the task view renders, **THEN** no dependency chip appears, and a status row
  that would otherwise be empty still does not render.
- **GIVEN** an open task whose board copy carries no dependency projection (a
  lightweight `task.updated` overwrote it, or arrived before hydration), **WHEN**
  the task view renders, **THEN** the chip still shows the task's edges, because a
  reader that cannot trust its cached copy re-reads them.
- **GIVEN** an open blocked task whose predecessor failed, **WHEN** the user opens
  the dependency chip, **THEN** the failed predecessor is distinguished from
  merely pending ones.
- **GIVEN** a narrow mobile viewport, **WHEN** the user taps the dependency chip,
  **THEN** the list opens in the same drawer pattern the PR status chip uses on
  mobile, not a hover card.

MCP:

- **GIVEN** an agent calls `create_task_kandev` with `blocked_by: ["A"]` and no
  explicit `start_agent`, **THEN** the task is created with the edge, no session
  is started, and the response reports `started: false` and
  `start_when_unblocked: true`.
- **GIVEN** an agent calls `create_task_kandev` with `blocked_by: ["A"]` and
  `start_when_unblocked: false`, **THEN** the edge is created, no launch intent
  is recorded, and completing A starts nothing.
- **GIVEN** an agent calls `create_task_kandev` with `start_when_unblocked: true`
  and no `blocked_by`, **THEN** no launch intent is recorded and the task starts
  or does not start purely per `start_agent`; the response reports
  `start_when_unblocked: false`.
- **GIVEN** an agent decomposes a plan into three chained sibling tasks over
  three `create_task_kandev` calls, **WHEN** the first task completes
  successfully, **THEN** the second starts and the third does not.
- **GIVEN** an agent calls `add_task_dependency_kandev` omitting `task_id`,
  **THEN** the edge is added to the calling task and the resulting `depends_on`
  list is returned.
- **GIVEN** an agent calls `add_task_dependency_kandev` with an edge that would
  close a cycle, **THEN** the call fails with the cycle path and no edge is
  created.
- **GIVEN** an agent calls `remove_task_dependency_kandev` for an edge that does
  not exist, **THEN** the call succeeds.
- **GIVEN** a Kanban-only install with `Features.Office` false, **WHEN** an agent
  calls `create_task_kandev` with `blocked_by`, **THEN** the task and edge are
  created and `list_related_tasks_kandev` reports them.
- **GIVEN** a narrow mobile viewport, **WHEN** the user opens a column
  containing a blocked task, **THEN** the blocked state and predecessor count
  are readable on the card without hover-only disclosure and without horizontal
  overflow.

## Out of scope

- **A whole-graph board view.** A layered DAG view was built and then removed
  before shipping: without drawn connectors it read as columns of text, and it
  could only show one workflow at a time, so it did not answer "show me this
  chain" any better than the per-task chip does. The chip (both directions, one
  hop) and `list_related_tasks_kandev` are the shipped ways to read the graph.
  A future view should draw real connectors, using the existing
  `components/kanban/graph2-connector.tsx` primitives, and be able to focus a
  single chain.

- **Office mode changes.** Office is feature-flagged and not in the live
  feature inventory ([`docs/features.md`](../../../features.md), "In Progress:
  Office Mode"), and it already has its own blocker reaction and run queue.
  v1 makes the relationship and the Kanban reaction work in Kanban mode and
  leaves `cascadeBlockersResolved`, Office agent assignment, and Office task
  delegation untouched. Unifying the two reactions is a follow-up that should
  wait until Office ships.
- Optional or "soft" dependencies (start-anyway-after-N-hours, OR semantics
  across predecessors). v1 is AND-only over all direct predecessors.
- Cross-workspace dependencies.
- Passing a predecessor's output (summary, diff, PR link) into the dependent's
  prompt. The dependent launches with the prompt recorded in its intent. Agents
  that need predecessor context read it through `list_related_tasks_kandev`.
- Stopping or rolling back a dependent that already launched when its
  predecessor is reopened or reverted.
- Automatic retry of a failed predecessor, and automatic edge removal on
  failure.
- A dependency-aware scheduling policy (critical-path ordering, priority
  inheritance along a chain, WIP reservation for a whole chain).
- Templated chains — saving "A → B → C" as a reusable pipeline definition.
  A chain is a set of edges between concrete tasks in v1.
- Gantt or timeline rendering, and estimated-duration modelling.
- Editing dependencies from the multi-select toolbar or via bulk operations.
- An edge editor on the task detail view. Declaring dependencies is a planning
  act that belongs to task creation (or to MCP for an agent decomposing work);
  the task view only reports them.

## Implementation plan

See [`../../plans/task-dependencies/plan.md`](../../../plans/task-dependencies/plan.md).