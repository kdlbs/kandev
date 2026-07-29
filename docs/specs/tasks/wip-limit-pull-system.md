---
status: accepted
created: 2026-07-27
updated: 2026-07-29
owner: kandev
---

# WIP Limits and Visible Overflow Queues

## Why

Workflow-step WIP limits currently prevent task creation once the configured
number of tasks occupies the step. This keeps the step below its limit, but it
also means integration fan-out is not represented on the Kanban board. A GitHub
review watch that discovers seven pull requests for a WIP-2 review step creates
only two tasks; the other five remain invisible until a later poll.

Users expect all discovered or submitted work to become durable, visible tasks
while the WIP limit controls how much of that work is admitted for active
processing. The same contract must apply to ordinary workflow task creation,
API and MCP creation, and integration watchers rather than living in
GitHub-specific retry logic.

## What

- `wip_limit` is the maximum number of active, WIP-admitted tasks in a workflow
  step. It is not a limit on the number of visible cards in that step and does
  not replace profile-wide agent-session concurrency.
- `wip_limit: 0` remains unlimited.
- Creating a non-ephemeral task for a limited step with capacity places and
  admits the task in that step.
- When the requested step is full and has `pull_from_step_id`, creation places
  the task in that feeder and records the requested step as its queue
  destination. The task is immediately durable and visible.
- When the requested step is full and has no feeder, creation still places the
  task in the requested step but records it as queued rather than admitted.
  The column may therefore contain more cards than its WIP limit.
- A queued task does not auto-start, consume a WIP slot, create a session, or
  prepare a workspace or executor until it is promoted.
- When capacity opens, Kandev promotes queued tasks into the limited step until
  the WIP limit is reached. Same-step promotion clears the queued state without
  changing columns. Feeder promotion moves the task into the destination step
  and clears the queued state.
- Promotion order is deterministic: position ascending, priority rank
  (`critical`, `high`, `medium`, `low`, none/unknown), queue time ascending,
  then task ID ascending.
- A promoted task follows the destination step's normal `on_enter` behavior.
  If the original create request explicitly requested an agent start, that
  deferred launch intent also becomes eligible when the task is promoted.
- A single accepted create must auto-start at most one agent session, even when
  creation synchronously promotes the task into an auto-start destination. A
  watcher's synchronous auto-start and the promotion's event-driven auto-start
  must not both launch. This applies whether the destination has capacity at
  creation (direct admission) or opens capacity during the same create call
  (feeder placement immediately promoted). See "Auto-start idempotency" below.
- The Kanban board shows queued state on each affected task and continues to
  show WIP consumption as `admitted/limit`. The total number of cards may be
  larger than the admitted count.
- The rule applies to task creation through the UI, HTTP, WebSocket, MCP, and
  integration watchers, including requests that resolve the workflow start
  step implicitly.
- Manual moves, drag/drop moves, bulk moves, and workflow-engine transitions
  into a full limited step retain the existing capacity conflict. This feature
  changes creation overflow, not explicit move semantics.
- Ephemeral tasks remain outside workflow WIP and queue behavior.

### One-hop feeder assumption

This draft uses one-hop overflow routing. If a full target step names a feeder,
Kandev attempts that feeder only. If the feeder is itself WIP-full, creation
returns the existing capacity conflict and creates nothing; Kandev does not
walk a chain of feeders. This remains an explicit assumption because the user
did not choose a recursive policy.

### Auto-start idempotency

When a task is created for an auto-start destination step, two independent code
paths can each try to launch the agent:

- the integration watcher (e.g. GitHub PR review) calls the start path
  synchronously after `CreateTask` returns, because the returned task reflects
  its post-promotion placement (`queued_for_step_id` cleared, resident in the
  auto-start destination);
- the promotion that ran inside that same `CreateTask` publishes the normal
  `task.moved` / `task.queue_promoted` event, whose handler independently
  auto-starts a task that is admitted in an auto-start step.

Both paths resolve to the same task in the same admitted state, so without a
shared guard each mints its own session. This produces two agents for one
review task, which the "start exactly once" contract forbids.

- The two auto-start paths must share one race-safe, per-task claim. Whichever
  path claims first launches; the other observes the claim and does nothing.
- The claim is atomic against concurrent claimers (same mechanism class as the
  existing deferred-launch metadata claim) so the two paths cannot both succeed
  even when they run within milliseconds of each other.
- A user-initiated start after the task has genuinely finished its automated run
  is not blocked by this claim: the guard scopes to the automated
  create-and-promote launch, not to every future launch of the task.
- If the claiming path's launch fails, the claim is released so the task can be
  retried instead of being left permanently unstarted.
- The guard is independent of whether the task was directly admitted or promoted
  from a feeder; both admission routes are covered.

## Data model

The task model gains durable queue/admission metadata:

```text
wip_admitted       boolean   true when the task consumes its current step's WIP
queued_for_step_id string    destination step while queued; empty when admitted
queued_at          timestamp ordering key for queued promotion; empty when admitted
```

- Existing active, non-archived, non-ephemeral tasks are migrated as admitted.
- Unlimited steps do not require admission bookkeeping; tasks in them normalize
  to admitted with no queue destination.
- A same-step overflow task has
  `workflow_step_id == queued_for_step_id` and `wip_admitted == false`.
- A feeder overflow task has `workflow_step_id` set to the feeder,
  `queued_for_step_id` set to the requested limited step, and
  `wip_admitted == true` only if it consumes capacity in a limited feeder.
  Its queue destination, rather than feeder residence alone, prevents another
  destination sharing that feeder from stealing it.
- Existing untagged tasks in a configured feeder remain eligible under legacy
  pull semantics. Destination-tagged overflow tasks are eligible only for their
  recorded destination.

Explicit create-and-start requests that overflow also persist a deferred launch
record associated with the task. The record preserves the resolved agent
profile, executor and executor profile, prompt, plan-mode choice, priority, and
attachments needed for the eventual launch. It is created atomically with the
queued task and consumed idempotently after promotion. No session, workspace,
repository checkout, container, or agent process is prepared while queued.

The exact table layout for deferred launches is an implementation detail, but
the task and its launch intent must commit or roll back together.

## API surface

Existing task-create requests do not gain a required field.

Task DTOs returned through HTTP, WebSocket boot/events, MCP, and integration
adapters gain:

```json
{
  "wip_admitted": false,
  "queued_for_step_id": "step-review",
  "queued_at": "2026-07-28T10:00:00Z"
}
```

- `workflow_step_id` always reports the task's actual visible column.
- `queued_for_step_id` is omitted or empty for an admitted task.
- Create responses return success for both admitted and queued tasks.
- HTTP, WebSocket, and MCP return the existing conflict classification only
  when one-hop placement cannot succeed, such as a configured feeder that is
  also full.
- `task.created` includes the actual placement and queue metadata.
- Promotion emits the normal `task.moved` event for feeder-to-destination
  movement. Same-step promotion emits a task update event so clients clear the
  queued badge and refresh the admitted count without inventing a move.
- Workflow step configuration, import, export, and MCP configuration continue
  to preserve `wip_limit` and `pull_from_step_id`.

## State machine

```text
create for target with capacity
  -> admitted in target

create for full target with feeder capacity
  -> admitted/resident in feeder + queued_for target
  -> destination capacity opens
  -> move to target + admitted + clear queue

create for full target without feeder
  -> resident in target + not admitted + queued_for target
  -> destination capacity opens
  -> admitted in place + clear queue

create for full target with full feeder
  -> conflict + no task
```

Capacity reconciliation runs after every event that can open or add slots:

- an admitted task moves out of the limited step;
- an admitted task is archived or deleted;
- a step's WIP limit is increased or changed from unlimited/limited;
- `pull_from_step_id` changes;
- backend startup or recovery finds a limited step below capacity with eligible
  queued tasks.

Reconciliation is idempotent and transactionally claims one slot and one queue
candidate at a time. Concurrent reconcilers cannot over-admit a step or launch
the same deferred intent twice.

If a user manually moves a queued task away from its current column, Kandev
cancels its queue destination and deferred auto-start intent before applying the
move. The explicit move then follows normal WIP admission rules at the new
destination.

## Permissions

- Queue placement does not grant permissions beyond the underlying task-create
  request.
- Promotion is an internal workflow action and reuses the task's existing
  workspace and workflow authorization context.
- Queue metadata is visible anywhere the user can already read the task.
- Existing permissions for changing workflow WIP and feeder configuration are
  unchanged.

## Failure modes

- If the configured feeder does not exist or belongs to another workflow,
  workflow validation rejects the configuration; task creation does not
  silently choose another step.
- If the configured feeder is full, one-hop placement returns a capacity
  conflict and persists no task, association, session, launch intent, or
  integration reservation.
- If target or feeder capacity changes concurrently, the repository retries or
  returns the typed conflict without exceeding either admitted limit.
- If promotion loses a race for the destination slot, the task stays queued and
  is eligible for the next reconciliation.
- If deferred launch fails after promotion, the task remains admitted and the
  launch intent remains retryable and visible through existing task/session
  error reporting. It is not silently returned to the queue.
- Deleting a destination step cancels queue metadata and deferred launch intents
  that target it before removing the step. The affected tasks remain in their
  current visible columns and do not auto-route elsewhere.
- Archiving or deleting a queued task removes its deferred launch intent.
- Changing a destination to unlimited admits all same-step queued tasks and
  pulls destination-tagged feeder tasks in deterministic batches without
  launching any task more than once.
- A watcher treats a successfully queued create as accepted work: it attaches
  its durable task/reservation identity and does not rediscover duplicate work.

## Persistence guarantees

- Accepted work is visible as a task before the create call reports success.
- Queue destination, queue order, and any deferred launch intent survive
  backend restarts.
- Task creation, integration linkage/reservation, queue metadata, and deferred
  launch intent use idempotent boundaries so a retry cannot create duplicate
  tasks or launches.
- Promotion updates task admission and destination atomically. Launching occurs
  only after the committed promotion is observable.
- Startup reconciliation resumes queued promotion without depending on a new
  watcher poll or browser connection.

## Scenarios

- **GIVEN** an empty auto-start `Review` step with `wip_limit: 2` and no feeder,
  **WHEN** a GitHub review watch discovers seven pull requests, **THEN** seven
  review tasks appear in `Review`, exactly two are admitted and started, and
  five show `Queued for Review` without sessions or prepared workspaces.
- **GIVEN** the review scenario above, **WHEN** one running review task leaves
  `Review`, **THEN** the next queued task is admitted in place and starts
  automatically without waiting for the watch's next poll.
- **GIVEN** an ordinary workflow whose WIP-2 start step has no feeder, **WHEN**
  seven tasks are created through a mix of UI, HTTP, and MCP requests,
  **THEN** all seven are durable and visible in the start column while at most
  two consume WIP slots.
- **GIVEN** a full WIP-2 `Review` step whose `pull_from_step_id` is `Backlog`,
  **WHEN** five more tasks target `Review`, **THEN** they are created visibly in
  `Backlog`, tagged `Queued for Review`, and promoted one at a time as Review
  capacity opens.
- **GIVEN** two limited destination steps share a feeder, **WHEN** overflow
  tasks are created for each destination, **THEN** each task is promoted only
  to its recorded destination.
- **GIVEN** a full destination and a configured feeder that is also WIP-full,
  **WHEN** a new task targets the destination, **THEN** creation returns a
  conflict and no task is persisted; a second feeder is not traversed.
- **GIVEN** an auto-start `Review` step with a WIP limit and a `pull_from_step_id`
  feeder that has capacity, **WHEN** a GitHub review watch creates a task that is
  placed in the feeder and immediately promoted into `Review` during the same
  create call, **THEN** exactly one agent session is auto-started for that task,
  even though both the watcher's synchronous start and the promotion's
  event-driven start attempt to launch it.
- **GIVEN** a same-step queued task with an explicit `start_agent` request,
  **WHEN** the backend restarts before capacity opens, **THEN** the task remains
  queued with no runtime resources and starts exactly once after promotion.
- **GIVEN** a queued task, **WHEN** a user moves it manually to another step,
  **THEN** its queue destination and deferred auto-start intent are cancelled
  and the move follows the new step's normal WIP rule.
- **GIVEN** a full limited step, **WHEN** a user drags an unrelated task into
  it, **THEN** the move is rejected and rolled back as it is today.
- **GIVEN** a step with `wip_limit: 0`, **WHEN** tasks are created or moved into
  it, **THEN** they are admitted immediately and no WIP queue is created.
- **GIVEN** a narrow mobile viewport, **WHEN** the user opens a feeder or
  limited column, **THEN** queued state and destination are readable on the
  shared task card without horizontal page overflow or hover-only disclosure.

## Out of scope

- A global or profile-wide task/session concurrency limit.
- Replacing `agent_profiles.max_concurrent_sessions`.
- Recursive feeder-chain routing.
- Automatically creating feeder steps.
- Redirecting manual or workflow-engine moves into queues.
- A GitHub-specific `max_inflight_tasks` replacement; watcher throttles remain
  independent safeguards.
- Reordering queued tasks through a new dedicated queue-management UI.

## Decision

See
[`../../decisions/2026-07-28-visible-wip-overflow-queues.md`](../../decisions/2026-07-28-visible-wip-overflow-queues.md).

## Implementation plan

See
[`../../plans/wip-overflow-queues/plan.md`](../../plans/wip-overflow-queues/plan.md).

Auto-start idempotency repair (single agent per review task):
[`../../plans/review-watcher-double-autostart/plan.md`](../../plans/review-watcher-double-autostart/plan.md).
