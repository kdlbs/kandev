---
spec: docs/specs/tasks/system-design/wip-limit-pull-system.md
created: 2026-07-28
status: in_progress
---

# Implementation Plan: WIP Overflow Queues

## Overview

The WIP implementation now represents accepted overflow work
on the Kanban board unless an explicitly configured feeder is itself full.

WIP becomes explicit admission state:

- a task created with destination capacity is admitted in that destination;
- overflow uses the configured feeder when one exists;
- overflow remains visibly queued in the destination when no feeder exists;
- only admitted tasks consume the displayed WIP count or become eligible for
  automatic launch;
- promotion is durable, deterministic, restart-safe, and generic across all
  creation adapters.

Manual moves and workflow-engine transitions keep their existing hard-cap
conflict. The GitHub review watcher becomes a consumer of the generic admission
result rather than maintaining a poll-time retry queue.

The draft assumes one-hop feeder overflow. A full configured feeder rejects
creation rather than recursively walking another feeder.

## Confirmed current behavior

- `task/service.Service.CreateTask` resolves the destination and calls
  `CreateTaskIfWorkflowStepHasCapacity`.
- The repository transaction counts resident active tasks; it does not persist
  an admitted-versus-queued distinction.
- HTTP, WebSocket, and MCP map a full destination to a conflict.
- GitHub review and issue watchers release their reservation after that
  conflict, so the work is retried on a later poll but has no Kanban task.
- Pull reconciliation runs mainly when a task moves out of a limited step. It
  does not comprehensively reconcile archive, delete, WIP edits, or startup.
- Create-and-start adapters launch immediately after `CreateTask`, so a queued
  task needs a durable launch-intent boundary before creation can return
  success safely.
- The Kanban count assumes resident-card count equals WIP consumption.

## Backend design

### Explicit admission and queue metadata

Add task-domain fields for `wip_admitted`, `queued_for_step_id`, and
`queued_at`. Backfill existing active workflow tasks as admitted. Update SQLite
and PostgreSQL-compatible migrations, repository scans, DTO conversion, event
payloads, and indexes used to select queue candidates.

Do not infer queue ownership solely from feeder residence: two limited
destinations may share a feeder. Destination-tagged tasks are eligible only for
their recorded destination; existing untagged feeder tasks keep legacy pull
eligibility.

### Atomic creation placement

Replace the capacity-only create primitive with an admission primitive that
locks the destination and, when needed, the configured feeder in stable ID
order:

1. Admit into the requested step when it has capacity or is unlimited.
2. If full and a feeder is configured, validate and capacity-admit into the
   feeder while tagging the requested destination.
3. If full and no feeder exists, insert into the requested step as non-admitted
   same-step overflow.
4. If the configured feeder is limited and full, return the typed WIP conflict
   and insert nothing.

The task row, queue metadata, repository associations that belong to the create
transaction, and any deferred-launch record must share an atomic success
boundary. Concurrency tests must cover destination admission, same-step
overflow, and feeder overflow on both dialect paths.

### Durable deferred launch

Introduce a task-owned deferred launch record rather than preparing a session
or workspace for queued work. It stores the already resolved launch inputs
needed by HTTP, WebSocket, MCP, UI, and watcher create-and-start paths.

Promotion claims and consumes that intent idempotently after admission commits.
Launch failure keeps a retryable intent and uses existing task/session error
reporting; it does not demote the task. Manual relocation, archive, or deletion
of queued work cancels the intent.

### Reconciliation

Consolidate the duplicated task-service and orchestrator pull logic behind one
queue reconciler. It must:

- select same-step and feeder candidates for one destination deterministically;
- atomically claim a destination slot and promote exactly one task;
- emit `task.moved` for feeder promotion and a task update for same-step
  promotion;
- run until capacity is full or no eligible candidate remains;
- run after move-out, archive, delete, WIP or feeder configuration changes, and
  backend startup/recovery;
- remain safe under concurrent triggers and repeated startup recovery.

Manual moves into full steps remain conflict-based. Moving a queued task away
first clears its queue target and deferred launch intent transactionally.

### Creation adapters and watchers

Every create adapter must use the returned actual placement and queue state.
An admitted create may follow existing immediate-start behavior. A queued create
returns success but persists its launch intent and performs no runtime work.

GitHub review, GitHub issue, and generic watcher dispatch attach their
reservation/task identity after either admitted or queued success. They skip
direct auto-start for queued work. Promotion later flows through the generic
launch path, so no new poll is required.

## Frontend

- Extend frontend task types, boot/event mappers, and store updates with
  admission and queue metadata.
- Change limited-column counts to `admitted/limit`; total cards remain visible
  in the column body.
- Add a compact task-card badge such as `Queued for Review`. The same shared
  card renders on desktop and mobile.
- Create success feedback distinguishes immediate placement from overflow:
  `Review is full; queued in Review` for same-step overflow and
  `Review is full; queued in Backlog` for feeder overflow.
- Update workflow WIP/feeder help text to explain that WIP controls admitted
  work, overflow remains visible, configured feeders take precedence, and a
  full feeder can reject creation.
- Preserve the existing mobile focused-column/tab pattern and column scroll
  ownership. Queue status must not rely on hover or create horizontal page
  overflow.

## Test strategy

### Backend unit and integration tests

- Atomic WIP-2 creation admits exactly two synchronized creates and persists all
  remaining no-feeder creates as same-step queued tasks.
- A configured feeder receives overflow atomically and tags the intended
  destination.
- A full configured feeder returns a conflict and creates no partial task or
  launch intent.
- Shared feeders do not cross-promote destination-tagged tasks.
- Same-step and feeder queues promote in deterministic order without
  over-admission.
- Move-out, archive, delete, WIP edit, feeder edit, and startup each reconcile
  available slots idempotently.
- Queued explicit-start creation prepares no runtime resources and launches
  exactly once after promotion, including across restart.
- Manual relocation/archive/delete cancels queue and launch state.
- HTTP, WebSocket, MCP, review watcher, issue watcher, and generic watcher
  surfaces classify queued success and unrecoverable feeder-full conflicts
  consistently.

### Frontend unit tests

- Task DTO/store conversion preserves queue metadata.
- Limited-column count uses admitted tasks instead of raw card count.
- Queue badges name the intended destination.
- Create feedback names actual placement for same-step and feeder overflow.
- Workflow setting copy and accessibility names remain usable without hover.

### Browser E2E

- Desktop: create more tasks than a no-feeder WIP step allows, verify every card
  appears, only the admitted count reaches the limit, and the next queued card
  promotes after capacity opens.
- Desktop feeder: verify overflow appears in the feeder and later moves to the
  intended destination.
- Mobile: repeat the no-feeder flow through focused column tabs, verify queued
  badges and count text, create/move interaction, no page-level horizontal
  overflow, and usable tap targets.

## Public documentation

Update:

- `docs/public/tasks-and-workflows.md`
- `docs/public/workflow-tips.md`

Document WIP as admitted work, same-column queues without a feeder, explicit
feeder placement, one-hop feeder-full conflicts, deterministic promotion, and
the distinction from agent-profile session concurrency.

## Implementation waves

All tasks are sequential because each consumes persistence or domain contracts
introduced by the previous task.

- [x] [Task 01: Persist WIP admission state](task-01-persist-wip-admission-state.md)
- [x] [Task 02: Implement atomic overflow placement](task-02-implement-atomic-overflow-placement.md)
- [x] [Task 03: Reconcile queued promotion](task-03-reconcile-queued-promotion.md)
- [x] [Task 04: Defer queued task launch](task-04-defer-queued-task-launch.md)
- [x] [Task 05: Adapt integration watchers](task-05-adapt-integration-watchers.md)
- [x] [Task 06: Expose visible queue UX](task-06-expose-visible-queue-ux.md)
- [ ] [Task 07: Verify browser flows and documentation](task-07-verify-browser-flows-and-documentation.md)

No task is marked `parallel-safe`; waves do not authorize subagent execution.

## Risks

- Treating resident-card count as WIP anywhere after migration would either
  strand queued work or over-admit it. All capacity queries need one shared
  admitted-count definition.
- Queued start parameters and attachments can be lost if launch intent is
  persisted after task creation instead of in the same atomic boundary.
- Two pull implementations currently exist. Updating only one would make
  manual moves and workflow-engine transitions reconcile differently.
- Reconciliation triggers can race. A read-then-update design without an
  atomic claim would over-admit or double-launch.
- Existing over-limit warning UI assumes every card consumes WIP; it must not
  flag intentional queued cards as data corruption.
- Legacy feeder tasks have no destination tag. Compatibility must not let
  destination-tagged tasks be stolen or make old pull workflows stop working.
- Startup reconciliation must be bounded/batched so a large queue does not
  delay backend readiness indefinitely.

## Out of scope

- Recursive feeder routing.
- Queueing explicit moves into full destinations.
- A new standalone queue page or queue reordering UI.
- Replacing profile-level session concurrency.
- Removing watcher-specific inflight safeguards.

## Draft assumption requiring confirmation

If a configured feeder is itself full, placement stops after one hop and
returns a capacity conflict. The implementation must not begin until this
assumption is accepted or changed in the spec and ADR.
