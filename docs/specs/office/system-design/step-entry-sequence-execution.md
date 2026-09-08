---
status: draft
system: office
requirements:
  - REQ-OFFICE-STEP-ENTRY-001
---

# Step Entry Sequence Execution System Design

## Purpose and boundaries

A workflow step declares an `on_enter` sequence. The compiler turns that
sequence into typed actions. On a real arrival in production, most of those
actions are never executed. This design makes them execute, exactly once per
arrival, without changing what any step declares and without touching any
trigger other than step entry.

It is separated from `review-participant-seats-01.md` because it is a
workflow-engine contract, not an Office one: every shipped workflow is affected
by it, and Office review seats are only the first consumer to depend on it. That
design links here rather than restating any of it.

Two things are settled here. **Which compiled actions are dispatched** on entry,
and which are excluded because the arriving session already ran them inline.
And **what identifies one entry**, which is what every at-most-once criterion in
this system is counted over.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-OFFICE-STEP-ENTRY-001 | Why a declared entry sequence does not execute today; Entry identity; One failing action does not cancel the rest |

## Why a declared entry sequence does not execute today

The workflow engine compiles a step's `on_enter` list into typed actions,
including `clear_decisions` and `queue_run_for_each_participant`. Dispatching
those compiled actions on a real step entry is a different code path, and in
production it does not happen. Every production route by which a task arrives
at a step converges on the orchestrator's own entry handler, which switches on
a small set of session-shaped action kinds - plan mode, session mode,
auto-start agent, agent-context reset, session configuration - and never asks
the engine to handle a step-entry trigger. The only production caller that does
dispatch a step-entry trigger does so for workflow switching. The engine's
transition processing likewise reports which step was entered without chaining
into that step's entry sequence.

The consequence is that today's Office `review` step already declares
`clear_decisions` and a reviewer fan-out, and in a running installation neither
runs. The engine-level test that appears to cover this constructs the trigger
by hand, which is why the gap survived.

So the reported defect has two independent causes, and fixing either alone
changes nothing observable:

1. Nothing writes a reviewer seat. That is the card's finding.
2. Nothing executes the step's entry sequence, so even a correctly written seat
   would not be woken.

**Design position.** Step-entry execution is split by whether an action needs
the arriving session. The two halves are disjoint by construction, and the
split is the contract.

*The session-shaped half is not moved, not re-run, and not dispatched.* The
entry handler already executes `enable_plan_mode`, `set_session_mode`,
`auto_start_agent` and `reset_agent_context` inline, and the callback registry
holds production callbacks for those same four kinds. Handing a step's whole
compiled entry sequence to that registry would therefore execute each of them a
second time: `office-default`'s own `work` step declares `auto_start_agent`, as
does a step in every other shipped workflow, so the agent would be launched or
prompted twice. `AC-OFFICE-STEP-ENTRY-001.3` forbids exactly that outcome, so
these four kinds are **excluded from dispatch by an explicit exclusion list**,
not by an assumption about what the registry happens to hold
(`AC-OFFICE-STEP-ENTRY-001.5`).

*The session-independent half is what this design newly dispatches:*
`clear_decisions`, `queue_run_for_each_participant`, `queue_run`,
`run_code_review`, and the seat-ensuring action introduced here. None of them
reads or mutates the arriving session; every one is task-scoped.

The two lists together are exhaustive over the kinds the compiler emits for
step entry, and a test asserts that: every compiled step-entry kind appears in
exactly one list. A kind added to the compiler later must be placed
deliberately, rather than defaulting into double execution or into silence.

**Disclosed consequence.** `queue_run` and `run_code_review` are compiled for
step entry today and, like the participant actions, never dispatched. No
shipped workflow declares either of them on entry, so this change is inert on
shipped configuration; an operator-authored step that declares one of them on
entry starts running it. That is what the configuration already asks for, and
it is named here rather than found later.

**Where the dispatch runs.** `AC-OFFICE-STEP-ENTRY-001.1` is quantified over
arrivals, not over one handler, and the entry handler is not on every arrival
path. It requires a live session, and the manual-move handler returns without
reaching it when the task has none. That is a supported route into the two
steps this capability exists for: `review` and `approval` both declare
`allow_manual_move: true` and neither declares `auto_start_agent`, so a task
moved into Review by hand with no live session takes it. Putting the
session-independent dispatch inside the entry handler would leave that task
unseated, and would make the acceptance test's result depend on how the test
happens to drive the task.

The session-independent dispatch is therefore driven from the arrival itself:
the committed change of `tasks.workflow_step_id`, which the system already
records exactly once per arrival in its step-transition ledger (see *Entry
identity*). Every route that changes a task's step writes that row; a route
that writes no row did not move the task. The dispatch runs once per committed
row, after it commits.

That binds `AC-OFFICE-STEP-ENTRY-001.1` to something checkable: one
step-entry dispatch per ledger row **that names a destination step**, no more
and no less. The qualifier is load-bearing rather than pedantic: the same
ledger also records a task being detached from its workflow, and such a row
carries no destination, so the task enters no step and there is no entry
sequence to run. A test written to the unqualified sentence fails on a detach
row. Nothing writes one today - the detach writer has no production caller -
which is exactly why the qualifier has to be in the contract rather than
discovered when one appears. A test asserts the correspondence, and it is the
test that fails if a future route changes a task's step without dispatching.

**How the dispatch is delivered.** Naming the trigger is not naming the
mechanism, and the mechanism is a real fork: a post-commit call, a poller over
unprocessed rows, and a transactional outbox all satisfy "once per committed
row, after it commits" and differ in cost, latency and crash behavior. This
design picks one.

*The dispatch is a synchronous post-commit call, made by the ledger's own
registered writers.* The set of writers permitted to change a task's step is
already closed and already enumerated in the code that writes the ledger; each
one, having committed, calls the step-entry dispatch on the same goroutine
before returning to its caller. No new table, no new process, no queue.

*Why not an outbox or a poller.* Both survive a crash between the commit and
the dispatch, and both cost more than the guarantee is worth here. An outbox
adds a table, a writer and a drainer to a system that already has no
asynchronous step-entry machinery; a poller adds latency that the acceptance
test would then have to be told to wait for, and a polling interval is a number
nobody has evidence for. The synchronous call has neither problem: when the
writer returns, the entry sequence has already run, so the pinned Playwright
assertion has a defined observation moment and needs no polling.

*The crash contract, stated rather than left to be discovered.* **A dispatch
lost to a process failure between the commit and the call is not recovered.**
The step change stands; the entry sequence does not run for that arrival. Such
a task is recovered the same way the requirement's out-of-scope line already
recovers every other stranded task - by arriving again - and there is no sweep.
This is the one guarantee traded away for the simplicity above, and it is
traded knowingly: the window is the duration of a function call, and the
alternative buys a table and a drainer to close it.

**Exactly one component dispatches.** `AC-OFFICE-STEP-ENTRY-001.5` requires
each declared action to take effect exactly once per arrival, and the exclusion
list alone does not deliver that, because it is keyed on action *kind* while
the risk is on the *route*. A workflow switch fires two paths for one arrival:
the writer that moves the task commits the step change and its ledger row
together, which is what the new dispatch keys on, **and** the switch's own
callback asks the engine to handle a step-entry trigger, which runs the whole
compiled sequence through the callback registry - including `clear_decisions`
and the participant fan-out, the two session-independent kinds this design
newly dispatches. Left alone, one arrival at `review` would clear decisions
twice and queue every reviewer's run twice, and the exhaustiveness test over
kinds would pass while it happened. The pre-existing applied-operation check on
that path does not prevent it either: it is keyed on the switch's own operation
identifier, not on entry identity.

So the switch route **stops dispatching the step's entry sequence**. The
ledger-driven path already covers it - a workflow switch commits a step change
and therefore writes a ledger row - so nothing is lost by removing the second
dispatcher, and the "one dispatch per ledger row" correspondence becomes
literally true rather than approximately true. `AC-OFFICE-STEP-ENTRY-001.8`
states the rule, and its test is on the route axis: drive a workflow switch and
assert the entry sequence ran once. A test over kinds cannot see this.

This is still the narrowest form of the fix in the dimension the out-of-scope
line names: it touches step entry and no other trigger.

## Entry identity

Both the at-most-once criteria and the fan-out's own deduplication need a value
that names one entry. Today an ordinary transition supplies none: the store's
public transition method passes an empty move identifier through to the engine,
and the engine treats an empty operation identifier as "do not deduplicate at
all" in both its already-applied check and its idempotency key. So today,
ordinary entries are not deduplicated, and a redelivery would re-run
everything.

**Design position.** The entry identity is the identifier of the ledger row
that recorded the arrival.

**Which ledger.** The system keeps two step histories and they are not
interchangeable, so this design names one.

- `task_step_transitions` is **task-scoped**, carries `task_id`,
  `to_workflow_step_id` and `occurred_at`, is indexed on `(task_id,
  occurred_at, id)`, and is written **synchronously** inside the same
  transaction that commits the step change, by the registered set of writers
  allowed to change a task's step. The row is committed before anything
  downstream can observe the arrival. **This is the one.**
- `session_step_history` is **session-scoped** - its model carries no task
  identifier at all - and is written onto a bounded channel that drops rows
  when full, as best-effort telemetry. It cannot answer a question about a
  task, and a stale or dropped entry would make a genuine re-entry look like a
  redelivery, which silently reproduces the defect this card reports. A design
  that said "the transition history" without naming which one could land here.

**Why the row identifier and not a count of rows.** A count-then-use read is a
read-modify-use with no serialization: two arrivals for the same task and step
could read the same count, derive the same identity, and one would be discarded
as a redelivery of the other; a redelivery straddling the write could derive a
new identity and double-queue the fan-out. The row identifier is allocated by
the insert itself, inside the transaction that commits the step change, so no
two arrivals can share one and none is assigned before its step change commits
(`AC-OFFICE-STEP-ENTRY-001.7`). There is also no off-by-one to decide,
because nothing is counted.

This satisfies the two criteria that pull in opposite directions:

- A redelivery of one arrival carries that arrival's row identifier, so the
  seats requirement's at-most-once criteria
  (`AC-OFFICE-REVIEW-SEATS-003.2`, `AC-OFFICE-REVIEW-SEATS-004.7`) hold.
- A task that leaves a gated step and returns writes a **new** ledger row and
  therefore carries a different identity, therefore a fresh fan-out run, which
  is what `AC-OFFICE-REVIEW-SEATS-003.6` requires and what a rejected-then-
  reworked task needs. Re-entry is a new entry, not a duplicate of the old one.

A random per-dispatch identifier would satisfy the second and break the first;
a `(task, step)` pair would satisfy the first and break the second; a count of
prior entries would satisfy both only under a serialization the writer does not
provide.

**Where "this entry already ran" is recorded.** Settling what the identity *is*
does not settle what remembers it, and the two are separate decisions. The
system's one existing mechanism for "this operation was already applied" is an
in-process map held by the orchestrator's workflow store. **It is deliberately
not used here**, and the reason is in this requirement's own Terminology: a
crash-recovery re-dispatch is named as a redelivery, and after a restart that
map is empty, so every identity looks unseen and precisely the case the
Terminology singles out would be the one case the deduplication misses.

**Design position.** There is no separate already-ran marker. Each action the
step-entry dispatch runs is individually idempotent against **durable state
keyed by the entry identity**, so the deduplication survives a restart because
the record of it is the database rather than a process's memory:

- *Seat-ensuring* deduplicates against the seat rows themselves, by the
  workflow-scoped write check inside the committing transaction and the natural
  key's unique index behind it (`AC-OFFICE-REVIEW-SEATS-001.3`, `.4`).
- *The participant fan-out* deduplicates against the queued runs, by the entry
  identity carried on the run it queues for a seat: a run already queued for
  that seat and that entry is not queued again
  (`AC-OFFICE-REVIEW-SEATS-003.2`), while a later entry carries a different
  identity and therefore queues afresh (`AC-OFFICE-REVIEW-SEATS-003.6`).
- *`clear_decisions`* is idempotent by its own nature: clearing twice leaves
  the same state as clearing once, so it needs no key.
- *`queue_run` and `run_code_review`* are inert on shipped configuration and
  carry the entry identity as their idempotency key when an operator-authored
  step declares them.

The consequence worth stating plainly: this design adds no table and no
process-lifetime state, and `AC-OFFICE-STEP-ENTRY-001.10` requires the
recognition to be durable rather than in-process, so a build that reaches for
the existing in-memory map fails a criterion instead of shipping a defect that
only appears after a restart.

**Two consequences a builder must handle.** The ledger writer does not return
the inserted row's identifier today, so it must; retrieval is
dialect-appropriate, since this path runs on SQLite and Postgres alike and the
code that creates this table already branches on the dialect. And the writer is
deliberately a no-op when the step does not actually change - a position-only
reorder, or a move re-issued to the step the task already stands on, writes no
row. That is the correct boundary and the requirement adopts it: such a move is
not an arrival, so no entry sequence runs, no seat is written, and no fan-out
is queued.

## One failing action does not cancel the rest

`AC-OFFICE-STEP-ENTRY-001.4` requires the remaining actions to be attempted
after one fails, and the failure to be recorded. The engine's shared action
loop returns on the first callback error. That loop also serves
`on_turn_start`, `on_turn_complete` and `on_exit`, so relaxing it in place
would change guard and transition failure semantics for every trigger, which
this requirement's out-of-scope line forbids.

**Design position.** The step-entry dispatch evaluates its own action list with
a record-and-continue loop, and the shared loop is left exactly as it is
(`AC-OFFICE-STEP-ENTRY-001.6`). A test asserts both halves: a failing action at
step entry leaves the rest of the sequence attempted, and a failing action on
`on_turn_complete` still aborts the rest. Without both halves the change reads
as a general relaxation of failure handling, which it is not.

## Observability

Each dispatched entry sequence records the arrival's entry identity, the step,
the action kinds attempted, and any action that failed with the reason. An
excluded kind is not logged as skipped work: exclusion is the contract, not an
anomaly. A redelivery discarded by entry identity is counted, because a
non-zero count there is the signal that a delivery path is retrying.

## Related decisions

- **ADR-0004, task model unification.** The step-entry execution path is one
  ADR-0004 already specifies; this design completes it rather than introducing
  a new boundary. If the build finds that completing it requires changing how
  triggers are driven generally, a decision record becomes necessary and the
  build should stop and say so.
- **ADR-0027, replayable schema migrations across SQLite and Postgres.**
  Binding on returning the ledger row's identifier, which must be retrieved in
  a dialect-appropriate way on both engines.
