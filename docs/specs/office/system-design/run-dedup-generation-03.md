---
status: draft
system: office
requirements:
  - REQ-OFFICE-RUN-DEDUP-001
  - REQ-OFFICE-RUN-DEDUP-002
  - REQ-OFFICE-RUN-DEDUP-003
  - REQ-OFFICE-RUN-DEDUP-004
created: 2026-09-07
owners:
  - kandev
---

# Office Run Deduplication — Generation Identity System Design Part 3: producer inventory and API contracts

Part 3 owns two things Parts 1 and 2 refer to but do not spell out: the complete
inventory of every producer that reaches a run queue, and the exact Go contracts
this capability adds or widens. The conceptual key contract is in
[Part 1](run-dedup-generation-01.md); observability, key durability and the test
plan are in [Part 2](run-dedup-generation-02.md).

Everything here is a contract a builder would otherwise have to invent. Where a
name or a byte encoding is given, it is binding — two producers that must agree
(AC-OFFICE-RUN-DEDUP-002.1) cannot agree on a convention nobody wrote down.

## Producer audit

Every site that reaches a run queue. "Generational" means the key already varies
with the occurrence. The fourth table is the one Parts 1 and 2 previously
omitted: producers that pass no key at all, which
AC-OFFICE-RUN-DEDUP-003.3 nonetheless obliges to report a cause.

### Already generational — no change

| Producer | Key shape | Generation component |
| --- | --- | --- |
| `office/service` `queueCommentRun` | op id `task_comment:<commentID>`, engine-expanded on persist | comment row id |
| `office/service` `handleApprovalResolved` | `approval_resolved:<approvalID>` | approval row id |
| `office/approvals/service.go` `queueApprovalRun` | `approval:<approvalID>` | approval row id |
| `office/dashboard/decisions.go` `decisionRunIdempotencyKey` | `decision:<decisionID>` | decision row id |
| `office/scheduler` `childrenCompletedIdempotencyKey` | `task_children_completed:<parent>:<agent>:<sha256 of child ids>` | child-set digest |
| `workflow/engine/phase2_callbacks.go` `idempotencyKey` | `<entryID or operationID>:<step>:<task>:<agent>:<digest>` | step-entry / operation id |
| `orchestrator` `officeAutoStartIdempotencyKey` | `task_assigned:<task>:<agent>:<step>:<stepTransitionID>` | step-transition row id |
| `orchestrator` step-entry operation id | `step_entry:<entryID>:<position>` | entry id and position |
| `scheduler/cron/heartbeat.go` operation id | `heartbeat:<task>:<step>:<unix seconds>` | tick time (AC-001.6) |
| `office/service/scheduler_wake_reconciler.go` `wakeOperationID` | `task_children_completed:<parent>:<child-id digest>` op id, shared with `queueChildrenCompletedRun` | child-set digest |

`officeAutoStartIdempotencyKey`'s `legacy:<task.UpdatedAt>` branch fires when
`stepTransitionID` is zero. It is time-derived, which AC-001.5 disallows for an
occurrence that has a durable row. Replace it with the keyless path of
[Unresolvable generation](run-dedup-generation-01.md#unresolvable-generation); do
not otherwise touch this producer.

### Not generational — must change

| Producer | Key today | Failure |
| --- | --- | --- |
| `office/service` `queueTaskAssignedRun` | `task_assigned:<task>:<agent>` | permanent per pair |
| `office/scheduler/run.go` `QueueRunCtx` default | `<reason>:<task>:<agent>` | permanent per (reason, task, agent) |
| `office/service` blocker-resolved dispatch | op id `blockers_resolved:<blockedTask>` | permanent per blocked task |

`QueueRunCtx`'s default is the larger of the two: every `RunContext` that does
not set `IdempotencyKey` inherits it — every reactivity wake except
children-completed.

### Unconstrained — must be scoped

| Producer | Key today | Failure |
| --- | --- | --- |
| `office/runtime/actions.go` `SpawnAgentRun` | verbatim `input.IdempotencyKey` | an agent reusing a literal permanently suppresses its own later wakes |
| `office/routines/service.go` `buildRoutineIdempotencyKey` | `routine:<id>[:<trigger>]:<unix_minute>` | the minute comes from the **processing** clock (`run.StartedAt`, set to `time.Now()` inside `dispatchRoutineRun`), not from the occurrence. A cron fire's occurrence is its claimed scheduled tick; a manual or webhook fire's is `RoutineRun.ID`. Both are durable and neither is what the key uses. The reachable collision today is two manual fires of one routine inside one minute — they share `triggerID == ""` and the same minute |

### Keyless today — must report a cause

These four call a run queue **directly** with an empty key. None of them passes
through `QueueRunCtx`, so the blanket entry in the second table does not reach
them, and none was previously enumerated anywhere in this design. Each is
covered by [Keyless causes per producer](#keyless-causes-per-producer) below, and
each is a row the key-format table test of
[Part 2](run-dedup-generation-02.md#testing) must iterate — under that test's
keyless assertion (empty key, plus the reported `cause` and `detail`), never its
varying-segment one, which a producer with no key cannot satisfy.

| Producer | Reason | Key today | Disposition |
| --- | --- | --- | --- |
| `office/onboarding/service.go` `maybeCreateOnboardingTask` | `task_assigned` | `""` | stays keyless, `cause=unresolved` |
| `office/service/scheduler_recovery.go` recovery sweep | `task_assigned` | `""` | stays keyless, `cause=by_design` |
| `office/service/retry.go` CEO error escalation | `agent_error` | `""` | **becomes generational** on the failed run id |
| `office/service/failure.go` `requeueRunForTask` | `manual_resume_after_failure` | `""` | stays keyless, `cause=by_design` |

`office/onboarding` also declares its own `runReasonTaskAssigned` constant, a
third copy of the same string. Consolidating it is out of scope for the same
reason the requirements already give for the `office/scheduler` and
`office/service` blocks.

### Writers that reach no queue at all

`UpdateTaskAssignee` has five non-test call sites and only one of them is a
producer. The other four build no key and wake nobody, so they never appear in
the tables above; they are enumerated in
[UpdateTaskAssignee and the bump sites](#updatetaskassignee-and-the-bump-sites)
because they still take the widened signature.

## Keyless causes per producer

AC-OFFICE-RUN-DEDUP-003.3 splits keyless enqueues into `unresolved` — a
generation that should have been available and was not — and `by_design` — a
producer that never had an occurrence to name. The split only pays for itself if
each producer is assigned a side deliberately, so each is assigned one here.

| Producer | Reason | `cause` | Why |
| --- | --- | --- | --- |
| reactivity status wakes | `task_unblocked`, `task_reopened`, `task_review_requested` | `by_design` | a status transition has no redelivery path; see Part 1 |
| `office/service/scheduler_recovery.go` | `task_assigned` | `by_design` | see [The recovery sweep](#the-recovery-sweep-must-not-borrow-the-assignment-generation) |
| `office/service/failure.go` | `manual_resume_after_failure` | `by_design` | an operator resume; the call site holds only a task id, and no row records the resume |
| `office/onboarding/service.go` | `task_assigned` | `unresolved` | the occurrence **does** have a generation; this producer simply is not handed it |
| `handleTaskCreated` with `fallbackToStoredRunner` | `task_assigned` | `unresolved` | Part 1 |
| `officeAutoStartIdempotencyKey`, zero `stepTransitionID` | `task_assigned` | `unresolved` | Part 1 |
| `SpawnAgentRun`, no caller run id | agent-supplied | `unresolved` | Part 1 |
| `SpawnAgentRun`, empty agent key | agent-supplied | `by_design` | the agent expressed no dedup intent; nothing failed to resolve |
| routine dispatch, `cron` with no claimed tick | routine reasons | `unresolved` | Part 1 |
| routine dispatch, unrecognised `Source` | routine reasons | `unresolved` | Part 1 |
| reactivity comment wake, nil `MutationComment` | `task_comment` | `unresolved` | Part 1 |

**The `detail` argument, per producer.** Part 1's
[`ReportKeylessEnqueue`](run-dedup-generation-01.md#unresolvable-generation)
takes `detail` so the `cause=unresolved` log record names *what* failed to
resolve, which `reason` alone cannot: four distinct producers above go keyless
under `reason=task_assigned`. Each `unresolved` row reports a fixed lowercase
snake_case constant naming the producer's own failure, not a formatted message:

| Producer | `detail` |
| --- | --- |
| `office/onboarding/service.go` | `onboarding_no_generation` |
| `handleTaskCreated` with `fallbackToStoredRunner` | `event_missing_generation` |
| `officeAutoStartIdempotencyKey`, zero `stepTransitionID` | `zero_step_transition` |
| `SpawnAgentRun`, no caller run id | `no_caller_run` |
| routine dispatch, `cron` with no claimed tick | `cron_no_claimed_tick` |
| routine dispatch, unrecognised `Source` | `unrecognised_routine_source` |
| reactivity comment wake, nil `MutationComment` | `nil_mutation_comment` |

Every `by_design` row passes `""`: that cause is counted and never logged, so it
has nothing to distinguish. `detail` is a log field only and **not** a counter
label — these seven values would multiply the keyless counter's cardinality for
no operational question that `cause` and `reason` do not already answer.

### The recovery sweep must not borrow the assignment generation

`office/service/scheduler_recovery.go` re-queues tasks it finds unstarted. It is
level-triggered from current task state, the same shape as `ParentWakeReconciler`,
and by the time it runs, `tasks.assignment_generation` for that task is readable.
Reading it is nonetheless **forbidden**, and not only because AC-001.9 rules out
recovering a generation by re-reading a row.

The decisive argument is that it would defeat the sweep. The sweep exists to
recover a task whose assignment wake was lost or whose run never started. If it
minted `task_assigned:<task>:<agent>:<generation>` from the current row, that is
byte-for-byte the key the original assignment already persisted on the run that
failed. `idx_run_idempotency` is unbounded, so the recovery enqueue would be
rejected as a durable dedup hit and the sweep would recover nothing — it would be
a permanent no-op with a counter, which is the defect this whole capability
exists to remove, reintroduced in the one path meant to be the backstop.

So the sweep enqueues keyless with `cause=by_design`. Its occurrence is "this
task was still unstarted at this sweep", which no durable row records. The cost
is a possible duplicate wake if the sweep runs twice before a run is claimed;
AC-003.2 elects exactly that trade. The 5-second coalescing window absorbs the
common case.

### Onboarding is a third `task_assigned` producer

`maybeCreateOnboardingTask` calls `CreateOfficeTask` -> `taskservice.CreateTask`
with `AssigneeAgentProfileID` set, then enqueues a `task_assigned` run itself.
Because that creation publishes `events.TaskCreated`, `queueTaskAssignedRun` also
observes the same occurrence and — after this capability — mints the keyed
`task_assigned:<task>:<agent>:1`.

Part 1's [Convergent producers](run-dedup-generation-01.md#convergent-producers)
says two producers observe an assignment. For a creation occurrence reached
through onboarding there are **three**, and the third cannot converge with the
other two: `CreateOfficeTask` returns only the task id, so onboarding never
receives the generation the creating transaction committed. Widening that
adapter is not undertaken here.

Onboarding therefore enqueues keyless with `cause=unresolved` — `unresolved`
rather than `by_design` precisely because the occurrence *does* have a
generation and this producer merely is not handed it, which is the distinction
AC-003.3 exists to make countable. This does not violate AC-002.1: that criterion
binds two producers that **each derive a key**, and a producer enqueueing with no
key is governed by REQ-003 instead. The scoping is in the criterion's own text,
so the onboarding pair needs no exemption of its own. The consequence is stated rather than left to
be discovered: onboarding's wake and `queueTaskAssignedRun`'s wake are two
enqueues for one occurrence, and only one carries a key. Both fire on one call
path within milliseconds, for one agent, reason and task, so `CoalesceRun` merges
them into a single queued run in the ordinary case. If they are ever separated by
more than the coalescing window, the operator gets a duplicate onboarding wake.
That is AC-003.2's elected direction, and the counter makes it visible.

### `agent_error` becomes generational

`office/service/retry.go` escalates a run failure to the workspace CEO. Its
payload already carries `run_id`, so the occurrence has a durable identity and
the audit's own rule applies: a producer with an occurrence to name must name it.

```text
agent_error:<failedRunID>:<ceoAgentProfileID>
```

The CEO agent id is included to keep the key's shape uniform with the other
recipient-addressed keys (`task_comment`, `task_mentioned`), which is the whole
of its justification. **It is not doing collision-avoidance work**, and a builder
must not go looking for the fan-out that would make it necessary: a workspace has
at most one CEO — `office/agents/service.go` and `office/service/agents.go` both
reject a second with `ErrAgentCEOAlreadyExists` — and `queueCEOAgentError`
(`office/service/retry.go`) escalates to `ceos[0]` alone, with no fan-out.
`agent_error:<failedRunID>` would be equally correct today; the salt is retained
because it costs nothing and survives a CEO seat being replaced.

A redelivery of one failure escalation is suppressed; a second, distinct failure
of the same agent produces a different `run_id` and wakes again.

This producer moves out of the keyless path entirely and takes no `cause`. AC-001.8
needs no argument here: every `agent_error` row persisted before this capability
carries a NULL `idempotency_key`, and both unique indexes are partial and ignore
NULL, so an old row can never collide with the new key.

## Go API contracts

### `UpdateTaskAssignee` and the bump sites

`office/repository/sqlite/tasks.go` and the interface declaration in
`office/dashboard/service.go` both widen to return the generation the
transaction committed:

```go
UpdateTaskAssignee(ctx context.Context, taskID, assigneeID string) (int64, error)
```

The returned value is read back **inside the existing transaction, after the
UPDATE and before `Commit`**. It is the generation of *this* assignment
occurrence, which is the only value a producer may key on.

All five non-test call sites, and what each does with the value:

| Call site | Role | Disposition |
| --- | --- | --- |
| `office/dashboard/service_tasks.go` (reached from `DashboardService.SetTaskAssigneeAsAgent`) | **producer** | carries the value into `TaskReactivityChange.AssignmentGeneration` |
| `office/service/task_assignee.go` `SetTaskAssignee` | non-producer | discards it (`_`); its own signature is unchanged |
| `office/service/task_assignee.go` `SetTaskAssigneeAsAgent` | non-producer | discards it (`_`); its own signature is unchanged |
| `office/service/channels.go` channel setup | non-producer | discards it (`_`) |
| `office/channels/service.go` channel setup | non-producer | discards it (`_`) |

**The channel-task flow is explicitly out of scope as a producer, and in scope as
a bump.** Both channel sites assign a long-lived channel task's runner seat
through `UpdateTaskAssignee`, so both bump the counter — that is correct, a
channel task's runner genuinely is being assigned. Neither publishes an
assignment event nor runs the reactivity pipeline, so neither builds a key and
neither wakes anybody. They take the widened signature and discard the value. No
`ReportKeylessEnqueue` call is made, because no enqueue happens.

**A read-back failure fails the assignment.** The read is a `SELECT` of one
column on the row just written, in the same transaction, so it can only fail if
that transaction is already unusable. When it does fail, the transaction rolls
back and `UpdateTaskAssignee` returns the error with a zero generation: the
assignment does **not** commit. Committing an assignment whose generation could
not be read would produce an occurrence that no producer can ever name, which is
strictly worse than a failed call the caller can retry.

The create-path bump site, `task/repository/sqlite/task.go` `insertTaskTx` ->
`upsertRunnerInTx`, inserts the literal `1` under the guard that writes the
runner row — `task.AssigneeAgentProfileID != "" && task.WorkflowStepID != ""` —
so it needs no read-back and neither signature changes.

**What carries the generation onto `task.created`.** Not a post-commit re-read;
AC-001.9 forbids it and it is not needed, because the value is a constant the
guard above already determines:

- the guard fired -> a runner row committed at generation `1`;
- the guard did not fire -> no runner row, and the task keeps the column default,
  generation `0`.

`CreateTask` (`task/service/service_tasks.go`) holds the same two fields the
guard reads, on the very `*models.Task` it handed the transaction, so it
re-evaluates that condition without a query and publishes the result through
`publishTaskEventWithExtra`
(`task/service/service_events.go`), whose `extra map[string]interface{}` is the
carrier. The create site calls `publishTaskEvent` today and moves to the
`WithExtra` form; `publishTaskEvent` already delegates to it, so no new plumbing
is introduced.

**A task created unassigned publishes generation `0`, not `1`.** `0` means "never
assigned" and can never name an assignment occurrence, so no `task_assigned`
producer keys on it — a creation that assigned nobody produces no assignment
occurrence to wake for, and the task's first real assignment goes through
`UpdateTaskAssignee` and commits `1`. Publishing `1` for an unassigned creation
would let one task mint `task_assigned:<task>:<agent>:1` twice — once at a
creation that assigned nobody, once at its genuine first assignment — recreating
the durable collision this capability exists to remove.

This satisfies AC-001.9 rather than bending it. The prohibition is on recovering
a generation by re-reading a row a later occurrence has since moved; nothing is
re-read here. The value is derived from in-memory fields of the struct the
creating transaction was given, so it cannot observe a later occurrence, and for
a creation it is a constant either way.

### The event payload field

`assignment_generation` on the `task.created` / `task.updated` payload is a
**nullable** JSON number, decoded into the `*int64` that Part 1 specifies:

- absent, or JSON `null` -> `nil` -> the producer goes keyless with
  `cause=unresolved`;
- a JSON number -> that generation, used verbatim.

`0` on the wire is a literal generation zero and **is not** a sentinel for
"absent". A task at generation `0` has never been assigned, so it can never be
the subject of a `task_assigned` occurrence; but a decoder that mapped `0` to
"absent" would silently convert a malformed payload into a keyless enqueue, and
one that mapped "absent" to `0` would mint `...:0` keys that collide across every
unassigned task. Neither is permitted: the field is nullable on the wire and
optional in the decoder, and the two states stay distinct.

### The shared key builders

Two exported builders live beside `internal/runs/commentkeys`, reachable from
`office/service`, `office/scheduler` and `orchestrator` without an import cycle.

**The assignment key.** Both `task_assigned` producers call it rather than
formatting the string themselves, which is what makes their convergence
structural instead of coincidental (AC-002.1).

**The blocker-set digest.** Both blocker producers — `task_blockers_resolved` in
`office/scheduler` and the `blockers_resolved` operation id in `office/service` —
call **one shared builder**. They live in different packages, and
AC-002.1 requires them to derive an identical key for one occurrence, so a
convention each implements separately is not sufficient: two packages choosing
`,` and `:` as a delimiter produce different digests for the same blocker set and
fail invisibly, presenting as a durable dedup miss rather than as an error.

The encoding is binding, and is the encoding
`childrenCompletedIdempotencyKey` already uses, so the two agree by construction:

1. take the `blocker_task_id` of each blocker in the wave;
2. sort ascending by byte value (**not** by `ListTaskBlockers`'s `created_at`,
   which is not unique);
3. join with a single `,` (U+002C);
4. SHA-256 over the UTF-8 bytes of that string;
5. render the full 32-byte digest as lowercase hex (Go `%x`), never truncated.

No de-duplication step is needed before sorting: `task_blockers` is
`PRIMARY KEY (task_id, blocker_task_id)`, so one task cannot list the same
blocker twice and two reads of an unchanged set always digest the same input.

**An empty blocker set is not an occurrence and is not enqueued.** A wave with no
blockers has nothing to have resolved, and the alternative is worse than a
missing wake: the digest of the empty string is a constant, so an empty-set key
would be `task_blockers_resolved:<task>:<agent>:<one fixed digest>` — permanently
unique per (task, agent), which is precisely the defect this capability removes.
A producer that finds an empty set returns without enqueuing and without calling
`ReportKeylessEnqueue`, because no enqueue was attempted.

**The set is digested at the readiness read, and is never re-read to build the
key.** Each blocker producer already reads the blocker rows to decide whether the
task is ready; that read is the snapshot the digest uses. This is the
capture-and-carry AC-001.9 imposes on the assignment generation and the cron
claimed tick, applied to a set rather than a scalar, and it is what stops a
producer digesting a set that a later wave has already moved.

- `office/service` `resolveAndWakeIfUnblocked` (`event_subscribers.go`) already
  holds the slice it read from `ListTaskBlockers`, and digests that slice.
- `office/scheduler` `allBlockersResolvedExcept` (`reactivity.go`) reads the same
  rows and currently **discards** them, returning only a boolean. It returns the
  slice alongside that boolean, and `cascadeBlockersResolved` passes it to the
  shared builder rather than issuing a second read.

Two producers observing one occurrence therefore digest what each saw at its own
readiness decision, and the consequence is stated rather than left to be
inferred. A blocker edge **added** between the two reads makes the later producer
find the task not ready at all, so it enqueues nothing. An edge **removed**
between them makes it digest a smaller set and mint a second key, which is a
duplicate wake — the direction AC-003.2 elects. A set that differs between the
two reads is a different wave, so AC-002.1's "same occurrence" premise does not
hold across it and no convergence is owed. Nothing here may lock the blocker
table or re-read to reconcile the two.

**A failed blocker read is not an empty set, and the two must not collapse.** An
empty set returns without enqueuing because there was no occurrence; a failed
read means the producer does not know whether there was one. On a read error the
producer propagates the error and enqueues nothing — it does **not** fall through
to the empty-set arm, does not enqueue keyless, and does not call
`ReportKeylessEnqueue`, because no enqueue was attempted in either case.

It must nonetheless be visible, and on one of the two paths it is not today.
`resolveAndWakeIfUnblocked` returns the error and `queueBlockersResolvedRuns`
logs it at `Error` with the blocked task id. `cascadeBlockersResolved` instead
drops it with a bare `continue` on `err != nil || !ready`, recording nothing, so
a wake lost to a read failure is indistinguishable from a task that was
legitimately still blocked. That arm gains an `Error` log naming the blocked task
and the error. This is the only behaviour change this section makes to the
readiness check itself; the check's own logic is unchanged.

### The suppression reporter and the no-op outcome

Part 2 requires one shared helper so three queue implementations report one
behaviour. It is exported from `internal/runs/service`, beside
`ReportKeylessEnqueue`, as three functions rather than one — the suppressions
are reached at different points and carry different evidence, and one of them is
classified by its caller rather than here:

```go
type QueueSource string

const (
    QueueSourceRuns   QueueSource = "runs"
    QueueSourceWakeup QueueSource = "wakeup"
)

// The recent-duplicate lookup matched. Counts kind="windowed", logs at Info,
// and returns QueueOutcomeDeduped.
func ReportWindowedDedup(q QueueSource, reason, key string) QueueOutcome

// Classifies the error from an insert. A unique violation on idempotency_key
// counts kind="durable", logs at Warn with key, reason and agent, and returns
// (QueueOutcomeDeduped, nil). Any other non-nil error is returned UNCHANGED
// with QueueOutcomeNone and moves no counter. A nil error returns
// (QueueOutcomeQueued, nil).
func ReportInsertResult(
    q QueueSource, reason, key, agentProfileID string, err error,
) (QueueOutcome, error)

// A durable conflict the CALLER has already classified. Counts kind="durable",
// logs at Warn, and returns QueueOutcomeDeduped. ReportInsertResult delegates
// here once its own classification succeeds.
func ReportDurableDedup(q QueueSource, reason, key, agentProfileID string) QueueOutcome
```

`ReportInsertResult` is the single place the *runs* unique violation is
recognised and mapped, which is what makes AC-002.3 hold on `queueRunInline` and
`SchedulerService.QueueRun` as well as in `runs/service`. Neither function ever
returns `QueueOutcomeCoalesced`: all three queues order
`CheckIdempotencyKey` -> `CoalesceRun` -> `insertRun`, so an enqueue that
coalesced returns from the coalesce arm and never reaches an insert to classify. A non-conflict error is
deliberately passed through untouched: a disk error is not a dedup decision and
must not be counted as one, nor swallowed into a successful-looking outcome.

**The wakeup path classifies its own conflict and calls `ReportDurableDedup`**,
not `ReportInsertResult` (AC-004.6). `ReportInsertResult` recognises a conflict
with `runssqlite.IsIdempotencyKeyUniqueViolation`
(`internal/runs/repository/sqlite`), which matches `idx_run_idempotency` on the
`runs` table and is already the classifier `runs/service` uses. It does **not**
recognise `ErrWakeupIdempotencyConflict`
(`office/repository/sqlite/wakeup_requests.go`) — a wrapped sentinel raised by a
different table's index, in the office tree — and it must not be taught to:
importing that sentinel would reverse the import direction this design pins
(`runs/service` imports only `office/models` from the office tree).

So the classification stays where the sentinel already is. The wakeup caller
lives in the office tree, already receives the error, does its own
`errors.Is(err, ErrWakeupIdempotencyConflict)`, and calls `ReportDurableDedup`
with `QueueSourceWakeup`. Its `kind` is always `durable`: that table has no
windowed lookup. This is reporting only; per AC-004.4 and AC-004.5 the wakeup
signature itself is not widened and keeps returning that sentinel, and the caller
keeps treating it as a suppression rather than a failure.

**The no-op outcome.** `QueueOutcome` is a string type whose zero value is `""`,
which is not one of `queued`, `deduped` or `coalesced`. That zero value is given
a name and a meaning rather than being left for a builder to rediscover:

```go
// QueueOutcomeNone means no enqueue was attempted, or the attempt returned an
// error. It is the zero value, so it is what a widened signature yields on any
// path that returns before deciding an outcome.
const QueueOutcomeNone QueueOutcome = ""
```

**It is declared in both copies of the type.** `QueueOutcome` exists twice —
`internal/runs/service/service.go` and `internal/workflow/engine/adapters.go` —
and each declaration carries the doc invariant "both MUST match".
`QueueOutcomeNone` is therefore added to **both**, with the same value and the
same doc comment. Part 1 lists `internal/workflow/engine` as already generational
and unchanged; that is a statement about its *keys*, and adding this constant is
the only edit this capability makes to that package. Adding it to one side alone
would break an invariant the code asserts in its own comments.

This is additive and changes no acceptance criterion. AC-004.4's obligation to
report queued, deduplicated or coalesced is conditioned on a caller enqueuing a
wake; a path that enqueues nothing never meets that condition. The paths that
return `QueueOutcomeNone` are the widened signatures returning a non-nil error,
including `ReportInsertResult`'s non-conflict arm.

**`QueueRunCtx` is not one of them.** It has no early return on an empty resolved
agent id. That guard sits one level up, in the reactivity pipeline's `queue`
closure (`office/scheduler/reactivity.go`), which returns *before* calling
`QueueRunCtx` at all — so an empty agent id yields no outcome to report rather
than `QueueOutcomeNone`, and `ApplyTaskMutationResult.Runs` gains nothing because
no enqueue was attempted. Part 1's
[reactivity gate](run-dedup-generation-01.md#the-reactivity-gate) states this
correctly and is the reference.

No caller may treat `QueueOutcomeNone` as success. In particular the reactivity
pipeline's `queue` closure appends to `ApplyTaskMutationResult.Runs` only for
`QueueOutcomeQueued`, so neither a suppression nor a no-op is reported as a
queued run. Returning `QueueOutcomeQueued` from a path that inserted nothing
would put a phantom run in that slice, which is the specific mis-report Part 2
added the `Queued`-only rule to prevent.
