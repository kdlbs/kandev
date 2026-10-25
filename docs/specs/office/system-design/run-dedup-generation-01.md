---
status: draft
system: office
requirements:
  - REQ-OFFICE-RUN-DEDUP-001
  - REQ-OFFICE-RUN-DEDUP-002
  - REQ-OFFICE-RUN-DEDUP-003
---

# Office Run Deduplication — Generation Identity System Design Part 1: the key contract

Part 1 covers what goes *into* a dedup key: the producer audit, the assignment
generation and how it is carried, the per-producer generation sources, producer
convergence, and the keyless path. Observability, failure and recovery, and the
test plan are in
[Part 2](run-dedup-generation-02.md). The full producer inventory and the exact Go
signatures this capability adds or widens are in
[Part 3](run-dedup-generation-03.md).

## Purpose and boundaries

This design changes what goes *into* a dedup key and what the queue *reports*
when it acts on one. It changes none of the queue's suppression mechanisms:
`CheckIdempotencyKey` (the 24-hour recent-duplicate lookup), `idx_run_idempotency`
and `idx_wakeup_idempotency` (the unbounded partial unique indexes), and
`CoalesceRun` / `shouldCoalesceRun` (the 5-second merge) are all unchanged. The
last is deliberate; see [Concurrency](#concurrency).

Adjacent contracts read and constrained but not owned:

- `internal/runs/service` — `QueueRun`, `QueueOutcome`,
  `errIdempotencyKeyConflict`.
- `internal/workflow/engine` — `idempotencyKey` and its `EntryID` /
  `OperationID` precedence; already generational, unchanged.
- `internal/orchestrator/event_handlers_workflow.go` —
  `officeAutoStartIdempotencyKey`; generational, unchanged apart from its legacy
  fallback.
- `internal/task/repository/sqlite` — the `tasks` table and the
  `workflow_step_participants` runner seat.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-RUN-DEDUP-001` | [Generation sources per producer](#generation-sources-per-producer), [Assignment generation](#assignment-generation) |
| `REQ-OFFICE-RUN-DEDUP-002` | [Convergent producers](#convergent-producers) |
| `REQ-OFFICE-RUN-DEDUP-003` | [Unresolvable generation](#unresolvable-generation), [Keyless causes per producer](run-dedup-generation-03.md#keyless-causes-per-producer) (Part 3) |
| `REQ-OFFICE-RUN-DEDUP-004` | [Observability](run-dedup-generation-02.md#observability) (Part 2) |

## Producer audit

The full inventory — every producer that reaches a run queue, including the four
that pass no key at all — moved to
[Part 3](run-dedup-generation-03.md#producer-audit) when it grew a fourth table.
Two entries are load-bearing here and are repeated rather than chased:
`queueTaskAssignedRun` and `QueueRunCtx`'s default are the non-generational
producers this design replaces, and `QueueRunCtx`'s default reaches every
reactivity wake except children-completed.

## Assignment generation

Assignment is the only occurrence in the audit with no durable identity to
borrow. `UpdateTaskAssignee` updates the runner seat in place, so
`workflow_step_participants.id` does not change across a reassignment, and a
plain reassignment writes no `task_step_transitions` row.

Add a monotonic counter to the task:

```text
tasks.assignment_generation  INTEGER NOT NULL DEFAULT 0
```

Applied through the existing additive migration mechanism
(`internal/task/repository/sqlite/base_migrations.go`), mirroring
`task_sessions.route_generation`, the same pattern already in the tree.

### The two bump sites

The runner seat has **two** writers, not one, and both bump:

| Writer | Reached from | Bump |
| --- | --- | --- |
| `office/repository/sqlite/tasks.go` `UpdateTaskAssignee` | every reassignment and unassignment | increment, inside its existing transaction |
| `task/repository/sqlite/task.go` `insertTaskTx` -> `upsertRunnerInTx` | `CreateTask` | insert `assignment_generation = 1` **under exactly the guard that writes the runner row** (`AssigneeAgentProfileID != "" && WorkflowStepID != ""`); any other create inserts `0` |

Both already run in a transaction. Each bumps on **every committed assignment
write**, including one setting the agent that already held the seat. That
unconditional rule is load-bearing: a guard of "only when the resolved runner
changes" would leave a repeat assignment minting an identical key, the permanent
no-op AC-001.3 forbids. Re-assigning a task to the agent that already holds it is
a real occurrence — the operator is asking for the work again.

An unassignment (`assigneeID == ""`, deleting the runner row) is an assignment
write and bumps too, so A -> unassigned -> A yields three distinct generations
and the final assignment is not suppressed by the first.

The occurrence's generation is stamped into:

```text
task_assigned:<taskID>:<agentProfileID>:<assignmentGeneration>
```

A different shape from the pre-existing `task_assigned:<task>:<agent>` rows, so
AC-001.8 holds without a backfill: an old row can never equal a new key. The
counter is task-level, not per-agent — A -> B -> A produces generations 1, 2, 3,
so the third assignment's key differs from the first's.

### Four writers of the same row that must NOT bump

`task/repository/sqlite/task.go` also calls `syncRunnerInTx` from `updateTaskTx`,
`UpdateTaskIfWorkflowStepHasCapacity`,
`PromoteQueuedTaskIfWorkflowStepHasCapacity` and
`RestoreTaskMessageRollbackIfSessionState`, each passing
`task.AssigneeAgentProfileID`. They write the same `role='runner'` row and are
provably inert as assignments: the field is **not a stored column** — `tasks` has
had none since ADR 0005 Wave F, every read derives it through `runnerProjection`
from the seat — so each of these paths writes back the value it just read.

No production caller can put a *new* agent into the seat through these four.
`task/service.UpdateTaskRequest` and `task/dto.UpdateTaskRequest` have no
`AssigneeAgentProfileID`, only `AssigneeUserID`, which its own doc comment
records as independent of the agent assignee. **Package-qualify that claim
wherever it is cited:** `office/dashboard.UpdateTaskRequest` is a different type
that *does* carry the field, but it routes to
`DashboardService.SetTaskAssigneeAsAgent` -> `UpdateTaskAssignee` — bump site one
— and never reaches `syncRunnerInTx`. Every production site supplying a new value
reaches either that path or `CreateTaskRequest` -> `insertTaskTx`, the second
bump site (`task/service/service_tasks.go`, `service_child_task.go` inheriting
the parent's, `backendapp/adapters_office.go`, `mcp/handlers/handlers.go`). Every
other non-test mention of the field is a read, a comparison, or a DTO projection.

Two of the four are step moves, which re-key the seat to a new `step_id`. That is
a seat migration, not an assignment; see
[What the counter does not cover](#what-the-counter-does-not-cover).

### Carrying the generation

**It is captured inside the assigning transaction and carried to the producers.
A producer never re-reads it.** The `UpdateTaskAssignee` bump site reads the new
value back inside its existing transaction, before `Commit`, and returns it; the
caller passes it to each producer explicitly. The create-path bump site returns
nothing and changes no signature: its generation is a constant the runner guard
already fixes (Part 3). On the dashboard path that caller is
`DashboardService.SetTaskAssigneeAsAgent` (`office/dashboard/service_tasks.go`).
Note that `office/service` declares a same-named `SetTaskAssigneeAsAgent` and a
`SetTaskAssignee` which also reach `UpdateTaskAssignee` but publish nothing and
run no reactivity; they are not producers, may discard the value, and neither
signature changes. Two channel-setup sites reach it as well and are likewise not
producers.

`UpdateTaskAssignee`'s widened signature, all five of its non-test call sites and
what each does with the value, and the behaviour when the read-back itself fails,
are specified in
[Part 3](run-dedup-generation-03.md#updatetaskassignee-and-the-bump-sites). The
wire encoding of the event field is specified in
[Part 3](run-dedup-generation-03.md#the-event-payload-field); `0` is a legal
generation and is never a sentinel for absent.

Two carriers, both **new fields this design adds** (neither exists today):

- `TaskReactivityChange.AssignmentGeneration` (`office/dashboard/service.go`),
  copied by `convertChangeToMutation` (`office/scheduler/dashboard_adapter.go`)
  onto a matching `TaskMutation.AssignmentGeneration`
  (`office/scheduler/reactivity.go`). Both `*int64`; nil means "not supplied". No
  struct is needed — the mutation already carries the assignee as the bare
  `NewAssigneeID *string` and the generation follows that shape.
- `assignment_generation` on the `task.created` / `task.updated` payload, decoded
  by `TaskUpdatedData` (`office/service/event_subscribers.go`) and published by
  `task/service/service_events.go`.

The field exists because the read is unsound: a producer reading
`assignment_generation` after the commit reads *the task's current* generation,
not *its occurrence's*. Given A -> B (1), B -> A (2), A -> B (3), a producer
still on occurrence 1 reads 3 and mints occurrence 3's key byte for byte; one is
then suppressed by the unique index, the direction REQ-OFFICE-RUN-DEDUP-003
forbids. A producer handed no generation takes the keyless path of
[Unresolvable generation](#unresolvable-generation) and does **not** read the
task row to recover one.

**The one producer that already re-reads stays keyless.** `handleTaskCreated`
calls `queueTaskAssignedRun` with `fallbackToStoredRunner=true`, so when the
`task.created` payload omits the assignee that producer recovers it through
`GetTaskExecutionFields` — a fresh read after the commit. Do **not** extend
`TaskExecutionFields` with the generation; that would make the unsound read above
a supported path. When the event carries no `assignment_generation` the producer
goes keyless with `cause=unresolved` even though it did recover an agent id: it
has an agent to wake and no occurrence identity to name.

### The reactivity gate

A new key alone is not sufficient. `ApplyTaskMutation`
(`office/scheduler/reactivity.go`) reaches `reactToAssigneeChange` only when the
agent differs — `change.NewAssigneeID != nil && *change.NewAssigneeID !=
task.AssigneeAgentProfileID` — and `DashboardReactivityAdapter` substitutes the
*pre-update* assignee into the snapshot. On a repeat assignment to the agent
already holding the seat both sides are equal, so **no `task_assigned` wake is
produced at all** and AC-001.3 fails before any key is built.

The gate drops the equality test and fires whenever `change.NewAssigneeID` is
non-nil. That is safe by the same call-site argument the bump sites use: of the
three `TaskReactivityChange` constructors in `office/dashboard/service_tasks.go`,
only `runReactivityForAssigneeChange` sets `NewAssigneeID`; the status and
comment constructors leave it nil, so a non-nil value already means "this
mutation is an assignment". An unassignment (non-nil but `""`) needs no new
guard: the pipeline's `queue` closure already returns early on an empty agent id.

**The interrupt needs a guard of its own, and does not have one today.**
`reactToAssigneeChange` also sets `res.InterruptSessionID`, hard-cancelling the
seat's previous session — and it does so on `task.AssigneeAgentProfileID != ""`
alone, never comparing `newAssigneeID`. That was safe only because the caller's
equality gate never delivered it a same-agent repeat. Removing that gate exposes
it, and because the adapter substitutes the pre-update assignee, on an A -> A
repeat the previous assignee is non-empty and the interrupt would cancel the
agent's own in-flight run. Move the comparison inside: set `InterruptSessionID`
only when `task.AssigneeAgentProfileID != ""` **and** `newAssigneeID !=
task.AssigneeAgentProfileID`. Only the wake becomes unconditional — this
capability changes dedup identity, not session lifecycle.

### What the counter does not cover

Three cases a builder would otherwise guess at. Ordering note:
`assignment_generation` is compared only for equality, never `>`/`<`. Nothing
reads it as a sequence, so no ordering or tiebreak over generations is defined.

**Two concurrent reassignments of one task.** Each bump site increments inside
its own transaction and writes are serialised, so the commits take distinct
generations N and N+1 — two keys, two runs, one per assignment. No extra locking.

**A producer handed no generation.** Because the value is carried, not read, the
only "stale value" case left is *absent* value — an event minted before this
shipped, or a nil `AssignmentGeneration`. It goes keyless.

**A step move.** It re-keys the seat row to the new `step_id`, and because
`runnerProjection` falls back to `workflow_steps.agent_profile_id` it can both
change the resolved runner with no participant write and materialise a per-task
row where none existed. Neither is an assignment: the occurrence is already
covered by `officeAutoStartIdempotencyKey`'s `stepTransitionID`. Do not bump to
compensate — that would re-wake an agent auto-start has already woken.

## Generation sources per producer

The replacement for each non-generational producer.

| Reason | New key | Generation source |
| --- | --- | --- |
| `task_assigned` | `task_assigned:<task>:<agent>:<assignmentGeneration>` | `tasks.assignment_generation` |
| `task_comment` (reactivity, assignee) | `task_comment:<commentID>:<agentID>` | comment row id, plus the recipient |
| `task_mentioned` | `task_mentioned:<commentID>:<agentID>` | comment row id, plus the recipient |
| `task_blockers_resolved` | `task_blockers_resolved:<blockedTask>:<agent>:<sha256 of blocker ids>` | blocker-set digest |
| `blockers_resolved` operation id | `blockers_resolved:<blockedTask>:<sha256 of blocker ids>` | same digest, same sort rule |
| `task_unblocked` | none (keyless) | no durable occurrence row |
| `task_reopened` | none (keyless) | no durable occurrence row |
| `task_reopened_via_comment` | `task_reopened_via_comment:<commentID>:<agent>` | comment row id |
| `task_review_requested` | none (keyless) | no durable occurrence row |

`task_mentioned` must carry the recipient: the mention wake and the assignee
comment wake describe the same comment but address different agents, and
including the agent keeps the fan-out independent when a comment mentions several
agents and one is later re-resolved. `task_comment` carries it for the same
reason — see [Convergent producers](#convergent-producers).

**AC-001.8 for the reshaped comment keys.** Only `task_assigned` gains a segment.
`task_comment`, `task_mentioned` and `task_reopened_via_comment` keep three
segments and merely swap the task id for a comment id, so the "different shape"
argument above does not carry over; all three hold instead because comment ids
and task ids come from disjoint uuid-keyed tables, so
`<reason>:<taskID>:<agentID>` can never equal `<reason>:<commentID>:<agentID>`
for a real pair. `task_comment`'s superseded form is the
`{reason}:{taskID}:{agentID}` default this design removes from `QueueRunCtx`.

`task_blockers_resolved` uses a set digest rather than the resolving blocker's
id, matching `childrenCompletedIdempotencyKey`'s already-argued shape: a wave is
identified by which blockers were in play, so a second wave that adds blockers
gets a distinct key. A wave re-resolving the *same* set digests identically and
is suppressed — the one collapse the requirements name under `## Out of scope`.
No producer may improvise around it.

Both blocker producers must sort the ids **lexicographically by
`blocker_task_id`** before digesting, and must not inherit `ListTaskBlockers`'s
ordering. That query is `ORDER BY created_at`, which is not unique: two blockers
added in one timestamp tick have undefined relative order, so two reads of one
unchanged set can digest differently and mint two keys for one occurrence.
`childrenCompletedIdempotencyKey` is safe only because `ListChildStates` orders
by `id`; the blocker path must impose its own.

Sorting is necessary but not sufficient: the two blocker producers live in
different packages and AC-002.1 requires them to derive an identical key, so they
call **one shared builder** rather than each implementing the same convention.
The delimiter, the digest encoding and the empty-set rule are binding and are
specified in
[Part 3](run-dedup-generation-03.md#the-shared-key-builders).

Three status-driven reasons are keyless. `ApplyTaskMutation` is called
synchronously after the mutation commits, never from the event bus, so a status
transition has no redelivery path and the key guarded nothing. Two guards
suffice: the pipeline's per-mutation `seen` map (`{agentID}:{taskID}:{reason}`)
stops a double-queue inside one mutation, and the 5-second coalescing window
absorbs a rapid repeat across mutations. A status-transition ledger is not
justified by the defect, and AC-003.2 elects a duplicate over a suppression.

`QueueRunCtx`'s `idempotencyKey == ""` default is **removed**, not replaced. Its
existence is what silently gave every future caller a permanent key. After this
change a `RunContext` with no `IdempotencyKey` is enqueued keyless, the safe
direction. Producers that want dedup set the field explicitly, and the
`RunContext.IdempotencyKey` doc comment records that empty now means "no dedup"
rather than "derive one for me".

### Agent-supplied keys

`SpawnAgentRun` prefixes a **non-empty** agent value with the calling run's id:

```text
agent:<callerRunID>:<agent-supplied key>
```

A retry of the same run reuses the run id and still dedupes, which is the case
the agent is protecting against; a later run gets a different prefix and is not
suppressed. With no caller run id the request is enqueued keyless.

An **empty** `input.IdempotencyKey` is not prefixed — it stays empty and goes
keyless. Prefixing it would collapse every no-dedup-intent `SpawnAgentRun` inside
one run onto the single key `agent:<callerRunID>:`, suppressing the second such
call: the run-scoped form of the exact defect this capability removes.

### Routine keys

Every routine fire commits a `RoutineRun` row (`dispatchRoutineRun` ->
`CreateRoutineRun`) before `materialiseLightweightRoutineRun` builds the key, so a
durable row is always available. But **that row identifies the dispatch attempt,
not always the occurrence**, and which is which depends on the source. The
discriminator is `RoutineRun.Source` (`shared.RoutineSourceCron` = `"cron"`,
`"manual"`, `"webhook"`), which `buildRoutineIdempotencyKey`'s current
`(routineID, triggerID string, startedAt *time.Time)` signature does not receive
and must gain.

| `run.Source` | New key | Generation source |
| --- | --- | --- |
| `cron` | `routine:<routineID>:<triggerID>:tick:<claimed tick, Unix seconds>` | the scheduled tick claimed off `office_routine_triggers.next_run_at` |
| `manual`, `webhook` | `routine:<routineID>:run:<routineRunID>` | `RoutineRun.ID` |

**Why cron does not use `RoutineRun.ID`.** `processCronTrigger` opens with
`ClaimTrigger`, a compare-and-swap (`SET next_run_at = NULL WHERE id = ? AND
next_run_at = ?`). Exactly one caller wins a given scheduled tick and the loser
returns before any row is written, so there is one `RoutineRun` per *slot*: the
row is the attempt, the slot is the occurrence. Keying on `run.ID` would give a
redelivery of one slot a fresh key, the direction AC-001.4 forbids. Catch-up does
not change this - `computeRoutineMissed` collapses N missed ticks into **one**
fire carrying `missedTicks`, so a catch-up is still one occurrence.

**Why manual and webhook do use `RoutineRun.ID`.** Neither claims a slot. Two
manual fires are two distinct intents, an operator asking twice, and
`RoutineRun.ID` is their only durable distinguishing identity. This is also the
collision the audit table names: both carry `triggerID == ""`, so today they
collapse onto one `routine:<id>:<minute>` key.

The consequence, stated rather than left to be inferred: a **retried** manual or
webhook request commits a second `RoutineRun` and therefore wakes twice, because
nothing upstream of `CreateRoutineRun` deduplicates the request itself. That is
the direction AC-003.2 elects, a duplicate wake over a suppressed one, and
narrowing it would need a request-level identity this capability does not
introduce. Two concurrent manual fires behave the same way: two rows, two keys,
two wakes, no locking. Cron is the only source where the CAS makes redelivery
converge.

**The claimed tick must be CARRIED, not re-read (AC-001.9).** `processCronTrigger`
calls `UpdateTriggerNextRun(advanceTo)` *before* dispatching, so by the time the
key is built the trigger row's `next_run_at` already names the **next** slot;
only the in-memory `*trigger.NextRunAt` still holds the claimed value. Thread it
from `processCronTrigger` through `DispatchRoutineRunWithMissed` ->
`dispatchRoutineRun` -> `materialiseRoutineRun` ->
`materialiseLightweightRoutineRun`. A producer that re-reads the trigger row
mints the next occurrence's key, the same unsound-read failure
[Carrying the generation](#carrying-the-generation) rules out for assignment.

`trigger` reaches `dispatchRoutineRun` and stops there, so the claimed tick
travels the last two hops as its own `*time.Time` parameter, exactly as
`missedTicks` already does. `materialiseRoutineRun` branches on `tmpl.Title`;
`materialiseHeavyRoutineRun` ignores the parameter, the treatment `missedTicks`
already receives on that branch, because the heavy path creates a real task and
builds no wakeup key.

**Nil and boundary.** `processCronTrigger` returns early when
`trigger.NextRunAt == nil`, so a live cron dispatch always has a claimed tick. The
exported `DispatchRoutineRunWithMissed` can still be called with
`source == "cron"` and a nil trigger or nil claimed tick; that enqueues
**keyless** with `cause=unresolved` (see
[Unresolvable generation](#unresolvable-generation)) rather than falling back to a
minute. Unix seconds suffice because cron granularity is one minute at finest, so
two distinct slots of one trigger can never share a value.

A `RoutineRun.Source` that is neither `cron` nor `manual`/`webhook` matches no
branch of the table above. It enqueues **keyless** with `cause=unresolved`, the
same direction as a cron dispatch with no claimed tick: an unrecognised source is
a source whose occurrence this design cannot name, and inventing a key for it
would be guessing at an identity rather than reading one.

`unix_minute` is removed from both branches; no fire keeps a processing-clock
key.

## Convergent producers

Two producers observe an assignment on the paths this section governs:
`queueTaskAssignedRun`
(`office/service/event_subscribers.go`, from `task.created` / `task.updated`) and
`reactToAssigneeChange` (`office/scheduler/reactivity.go`, through
`ApplyTaskMutation`). They converge on one key today by accident of format —
`fmt.Sprintf("task_assigned:%s:%s", …)` and `QueueRunCtx`'s
`fmt.Sprintf("%s:%s:%s", reason, taskID, agentID)` produce the same string — and
that convergence must survive (AC-002.2).

**They are driven by different entry paths, and no decision here may assume one
call sequence reaches both.** Verified:

| Entry path | Publishes / calls | Producer that fires |
| --- | --- | --- |
| Office dashboard `SetTaskAssigneeAsAgent` | `publishTaskUpdated` -> `events.OfficeTaskUpdated`, then `runReactivityForAssigneeChange` | reactivity **only** |
| Task creation with an assignee | `events.TaskCreated` | `queueTaskAssignedRun` **only** |
| Task service update | `events.TaskUpdated` | `queueTaskAssignedRun`, but see below |

`handleTaskUpdated` subscribes to `events.TaskUpdated`; `publishTaskUpdated`
emits `events.OfficeTaskUpdated`. Different subjects, so the dashboard assignment
path does **not** reach `queueTaskAssignedRun`.

**The `task.updated` row cannot carry an assignment *change*.** As
[Four writers](#four-writers-of-the-same-row-that-must-not-bump) establishes, no
production update path reaching `syncRunnerInTx` can put a new agent into the
seat. So `queueTaskAssignedRun`'s live assignment occurrence is task
**creation**; `handleTaskUpdated` stays subscribed as a redelivery and defensive
path, and if it fires without a carried generation it goes keyless with
`cause=unresolved`. Nothing here may assume a task-service update produces an
assignment occurrence.

**A third producer exists on the onboarding path and does not converge.**
`office/onboarding` creates a task with an assignee and enqueues its own
`task_assigned` run, while the same creation's `task.created` event drives
`queueTaskAssignedRun`. The onboarding producer is never handed the generation,
so it goes keyless rather than minting a divergent key. AC-002.1 binds only
producers that each derive a key, so a keyless third producer owes no
convergence; see
[Part 3](run-dedup-generation-03.md#onboarding-is-a-third-task_assigned-producer).

AC-002.2 therefore requires something narrower than "both always fire": *when one
assignment occurrence is observed by both producers — a redelivery, or a future
path that drives both — they must derive the same key.* Carrying the generation
delivers that: both stamp the value the assigning transaction committed rather
than each re-deriving one. Part 2's test case drives both producers directly
rather than through a dashboard call, because on that path only one fires.

To make the convergence structural rather than coincidental, both producers call
one exported builder instead of formatting the string themselves. Place it beside
`commentkeys` as a sibling package so the office service, the office scheduler
and the orchestrator all reach it without an import cycle;
`officeAutoStartIdempotencyKey` keeps its own shape, a step-driven auto-start
being a different occurrence from a reassignment.

**The comment wakes do not converge, and do not need to.** `queueCommentRun`
passes `commentkeys.TaskComment(commentID)` to `dispatchEngineTrigger` as an
*operation id*, which the engine expands to
`<opKey>:<step>:<task>:<agent>:<digest>` before persisting, so the shapes can
never be equal. They are already mutually exclusive by construction:
`runReactivityForComment` (`office/dashboard/service_tasks.go` — **not**
`handleCommentCreated`, which never touches the field) sets
`SkipAssigneeCommentWake` from `dispatchCommentEngineTrigger`'s return, and the
adapter carries it across as `SkipAssigneeWake`, so when the engine handled the
comment the reactivity assignee wake is skipped. One comment, one assignee wake,
enforced by a branch rather than a key collision. The engine is unchanged.

Consequently `task_comment` **does** carry the recipient. The occurrence is
"(this comment, this recipient)", not "this comment": one comment fans out to an
assignee and every mentioned agent, and a bare `task_comment:<commentID>` would
collide two recipients' wakes and suppress one. The `task_comment:` prefix is
preserved, so `shouldCoalesceRun`'s prefix test still excludes these from
coalescing, and `commentkeys` already parses salted keys (`CommentIDFromKey` cuts
at the first colon), so the extra segment needs no parser change.

### Concurrency

Two producers racing on one key resolve without locking, and this design adds
none. Both call `CheckIdempotencyKey` and may see no duplicate; both proceed;
`idx_run_idempotency` admits one; the loser's `CreateRun` returns a unique
violation, mapped to `errIdempotencyKeyConflict` and reported as
`QueueOutcomeDeduped` with a nil error.

That last step is today true **only** in `runs/service`. `queueRunInline` and
`SchedulerService.QueueRun` return the raw violation as an error, so a race on
the reactivity path surfaces as a failure rather than a dedupe. Both gain the
mapping via the shared helper (see
[Counters](run-dedup-generation-02.md#counters)); without it AC-002.3 does not
hold on the path this capability fixes.

**The loser can observe either suppression outcome, and AC-002.3 accepts both.**
The queue checks the key, *then* coalesces, *then* inserts. A loser whose lookup
ran after the winner's insert sees `Deduped`; one whose lookup ran before that
insert and whose coalesce check ran after it sees `Coalesced`, because the
winner's row is queued for the same agent, reason and task inside the 5-second
window. Both leave exactly one run row and neither returns an error. A test for
AC-002.3 must accept either and must not pin `Deduped`. Note also that
`SchedulerService.QueueRun` coalesces unconditionally where `runs/service` first
consults `shouldCoalesceRun`; neither is changed, and this outcome contract is
written to hold for both.

Neither caller aborts, and the observable result is the same whichever insert
committed first (AC-002.4): at most one row ever bears that key, and none does
when a coalescible run was queued for that agent, reason and task at each
producer's coalesce check, because a coalesced enqueue never reaches
`insertRun`. See
[Key durability](run-dedup-generation-02.md#key-durability), which owns that
boundary. No ordering or tiebreak is defined: the losing path has no output to
order. The run `id` is a fresh UUID per attempt, so nothing may depend on which
uuid won.

## Unresolvable generation

A producer that cannot resolve its generation component enqueues with an empty
key. It does **not** fall back to `<reason>:<task>:<agent>` (AC-003.1), because
that is the defect.

Concretely: an event carrying no `assignment_generation`, or a nil
`AssignmentGeneration` on a mutation; `officeAutoStartIdempotencyKey`'s zero
`stepTransitionID`; `SpawnAgentRun` with no caller run id; a reactivity comment
wake whose `MutationComment` is nil; and a `source == "cron"` routine dispatch
reaching [Routine keys](#routine-keys) with no claimed tick carried.

`SpawnAgentRun` with an **empty agent-supplied key** is deliberately absent from
that list: nothing failed to resolve, the agent expressed no dedup intent, so it
is `by_design`. Part 3's
[keyless causes](run-dedup-generation-03.md#keyless-causes-per-producer) assigns
every producer its side and agrees.

An empty key already means "no dedup" throughout the queue: `QueueRun` skips
`CheckIdempotencyKey`, `insertRun` writes a NULL `idempotency_key`, and both
unique indexes are partial and ignore NULL (AC-003.4). Coalescing still applies —
`shouldCoalesceRun` only excludes the `task_comment:` prefix.

**Where the keyless counter is incremented, and how `cause` reaches it.** The
cause is a property of the producer's *decision*, not of the request: by the time
an enqueue reaches a queue, "I never needed a key" and "I tried to build one and
failed" are both an empty string, and `RunContext` carries nothing separating
them. So the queue does **not** report keyless enqueues and nothing infers a cause
from an empty key. Instead `internal/runs/service` exports a reporter beside the
suppression helper of [Counters](run-dedup-generation-02.md#counters):

```text
runsservice.ReportKeylessEnqueue(reason string, cause KeylessCause, detail string)
```

with `KeylessCauseUnresolved` and `KeylessCauseByDesign`. Each producer calls it
where it decides to go keyless, naming its own cause, immediately before
enqueuing. `detail` names **what failed to resolve** — the short constant
[Part 3](run-dedup-generation-03.md#keyless-causes-per-producer) assigns each
producer — so the `Info` record [Part 2](run-dedup-generation-02.md#logs)
requires for `cause=unresolved` separates two producers that go keyless under one
reason. It is `""` for `cause=by_design`, which is not logged. Counters stay declared once in `internal/runs/service` — no parallel
map — and the import direction already holds: `runs/service` imports
`office/models` only, so `office/service`, `office/scheduler` and
`office/routines` can all call it without a cycle.

`cause=unresolved` is every site enumerated in *this* section. `cause=by_design`
is the three status reasons and any other producer that never had an occurrence to
name. **The earlier rule — that any `RunContext` setting no key is `by_design` —
is withdrawn**: with `QueueRunCtx`'s default removed a producer that failed to
resolve also arrives with no key, so that rule counted precisely the failures
AC-003.3 exists to isolate as normal traffic. A producer reaching a queue keyless
without having called the reporter is a bug; the key-format table test in
[Part 2](run-dedup-generation-02.md#testing) is where it is caught.

The counters themselves are defined in
[Part 2](run-dedup-generation-02.md#counters).
