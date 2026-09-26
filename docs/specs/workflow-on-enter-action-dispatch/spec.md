---
status: draft
created: 2026-08-19
updated: 2026-08-31
owner: Kandev
---

# Workflow `on_enter` Action Dispatch

Decisions:

- [ADR-0004 task-model-unification](../../decisions/0004-task-model-unification.md) — establishes the
  workflow engine as "the universal coordinator for task-scoped agent runs" and defines
  `queue_run_for_each_participant` and `clear_decisions` as `on_enter` actions.

## Why

A workflow step declares what should happen when a task arrives at it, under
`events.on_enter`. Today that declaration is only partly honoured, and which part
depends on **how the task arrived at the step**. There are two implementations of
`on_enter` in the tree and neither is complete:

| Arrival path | Dispatcher | Actions honoured |
|---|---|---|
| Agent-turn auto-advance (`on_turn_complete` → transition) | `orchestrator.processOnEnter` | `reset_agent_context`, `configure_session`, `enable_plan_mode`, `set_session_mode`, `auto_start_agent` |
| Manual move / WIP-queue promotion | `orchestrator.processOnEnter` | same five |
| `switch_workflow` action | `engine.HandleTrigger(TriggerOnEnter)` | **nine of ten** — every type except `configure_session` |

The third row is not a typo, and it is the single most misread fact in this area.
`switchWorkflowDispatcher` calls `engine.HandleTrigger(TriggerOnEnter)` with
`processOnEnter` **absent from the call path entirely**, and the engine's registry is not
the reduced one it might appear to be. `buildWorkflowCallbacks` registers
`enable_plan_mode`, `disable_plan_mode`, `reset_agent_context`, `auto_start_agent`,
`set_workflow_data` and `set_session_mode` **unconditionally** — unlike the Phase-2 kinds,
which are nil-guarded on their adapters — and `compileOnEnter` compiles all nine
non-`configure_session` types. So `switch_workflow` already executes the five
session-lifecycle types through engine callbacks, *and* is the only path that honours
`clear_decisions` and `queue_run_for_each_participant`.

Two consequences follow, and both are load-bearing:

1. The ownership partition below is stated **per entry path**, not globally. A partition
   naming one owner per action type with no reference to arrival path would be false for
   `switch_workflow` on five of ten types, and a builder enforcing it literally would
   delete four working callback registrations and silently regress that path.
2. `configure_session` is the one type `switch_workflow` genuinely drops. That is a real
   AC-A2 gap; it is named and excluded (Out of scope) rather than left silent, and
   AC-A6 requires it to warn on that path.

On the ordinary path, `processOnEnter` handles session actions and classifies engine-owned
actions. Before this fix, `ensure_participant_seat` lacked a no-op case and triggered a
false warning.

The `Office Default` workflow depends entirely on the actions that fall through. Its
Review and Approval steps declare:

```yaml
on_enter:
  - type: clear_decisions
  - type: queue_run_for_each_participant
    config: { role: reviewer, reason: review_started }
```

Neither executes on an ordinary transition. No reviewer is ever woken, so no decision is
ever recorded, so the `wait_for_quorum` guard on `on_turn_complete` can never be
satisfied, so the task never leaves Review. Every Office task stalls permanently at the
first review gate, with nothing anywhere saying why.

This was confirmed on 2026-08-19 by driving a real task (Epic: Git-sync for Kandev
automations, workspace `95542bf3`) end to end against `Office Default`
(workflow `7561d51f`). The card advanced Work → Review correctly and the reviewer was
attached to the Review step; no run was ever queued and no diagnostic was produced.

Three things make this expensive to diagnose, and the fix must remove all three:

1. **The declaration validates.** `config/workflows/loader.go` accepts
   `clear_decisions` and `queue_run_for_each_participant` under `on_enter`
   (`validOnEnter`), and the workflow import path accepts them too. Authoring gives a
   clean bill of health for a step that will do nothing.
2. **The capability exists on the happy path only.** `engine.ClearDecisionsCallback` and
   `engine.QueueRunForEachParticipantCallback` are implemented, their adapters are wired
   in `backendapp`, and `TestOfficeDefaultWorkflow_FullCycleSmoke` walks the shipped
   `Office Default` template Work → Review → Approval → Done and asserts the fan-out
   count. It passes. It calls `HandleTrigger(TriggerOnEnter)` directly — an entry point
   no ordinary transition ever reaches. A green test certifies an unreachable path, and
   it certifies only the path where nothing fails: the failure-isolation behaviour this
   spec requires (AC-C1, AC-C3) is the **opposite** of what those callbacks do today, so
   "the capability exists" must not be read as "the capability is complete". See
   [Scope reality](#scope-reality).
3. **The behaviour is inconsistent, not merely absent.** The same step, with the same
   declaration, behaves differently depending on arrival route. Anyone who reproduces
   through `switch_workflow` sees it work.

## Ownership decision

**On every entry path where `processOnEnter` runs, it is the single owner of the five
session-lifecycle actions and the workflow engine is the single owner of the other five.
On the one entry path where `processOnEnter` does not run at all (`switch_workflow`), the
engine owns every type it compiles.** Each action type has exactly one owner *per entry
path*, no action type is executed twice within one step entry, and the partition is stated
in full below.

Rationale, and why the alternative is rejected:

- ADR-0004 already assigns the coordinating role to the engine, and defines both missing
  actions specifically as `on_enter` actions.
- Adding the two missing cases to `processOnEnter` closes this card and leaves the trap.
  `run_code_review` is already the next victim: it is compiled by the engine, dispatched
  by no orchestrator path, and reachable through imported and git-synced workflows. A
  third partial switch is not a fix.
- Two implementations of one trigger is the underlying defect. The observable contract
  below is written so that it cannot be satisfied by a second partial switch: it requires
  identical action coverage across all arrival paths.

This decision constrains *where action semantics live*. It does **not** move session
lifecycle work. `processOnEnter` remains responsible for the session concerns it owns
today — agent-profile switching, the stale-`RUNNING` flip, plan-mode resolution,
passthrough vs ACP prompt delivery, waiting-state publication and queued-message drain.
Those are preserved unchanged (AC-D1).

### The ownership partition

This table is normative, and it is indexed by **entry path** because ownership genuinely
differs between them (see [Why](#why)). `E1`–`E3` are the paths on which `processOnEnter`
runs today; `E4` is `switch_workflow`, on which it does not. **`E5` joins the
`processOnEnter` column**: those two sites dispatch no `on_enter` at all today, and AC-A2
requires them to, so the dispatch this card adds there is `processOnEnter`'s — same owner,
same fixed positions, same partition. Read every `E1`–`E3` below as `E1`–`E3`, `E5`.

| Action type | Owner on `E1`–`E3`, `E5` | Owner on `E4` | Notes |
|---|---|---|---|
| `reset_agent_context` | `processOnEnter` | engine (`resetAgentContextCallback`) | Fixed position, first, on `E1`–`E3` (AC-A9) |
| `configure_session` | `processOnEnter` | **nobody** | Fixed position, second, on `E1`–`E3` (AC-A9). Not compiled by `compileOnEnter`; warns on `E4` (AC-A6), and closing that is Out of scope |
| `enable_plan_mode` | `processOnEnter` | engine (`enablePlanModeCallback`) | Session-scoped |
| `set_session_mode` | `processOnEnter` | engine (`setSessionModeCallback`) | Session-scoped |
| `auto_start_agent` | `processOnEnter` | engine (`autoStartAgentCallback`) | Session-scoped; prompt delivery differs between the two owners, deliberately (Out of scope) |
| `clear_decisions` | engine | engine | |
| `queue_run_for_each_participant` | engine | engine | |
| `queue_run` | engine | engine | |
| `run_code_review` | engine | engine | |
| `ensure_participant_seat` | engine | engine | |

Two properties this table exists to guarantee, neither of which is "one global owner":

- **No double execution within one entry (AC-A11).** The two dispatchers **share one
  callback registry**: `buildWorkflowCallbacks` registers `enable_plan_mode`,
  `reset_agent_context`, `auto_start_agent` and `set_session_mode` unconditionally, and
  `compileOnEnter` compiles all four from the same declarations. A `processOnEnter` that
  delegated its whole declared list to `HandleTrigger` would execute those four **twice** —
  once through its own direct calls, once through the engine callbacks — and
  `autoStartAgentCallback` is a third, different auto-start path from `autoStartStepPrompt`
  / `autoStartPassthroughPrompt`. The `E1`–`E3` column is what forbids that outcome.
- **The `E4` column is a description of today, and is preserved, not changed (AC-A13).**
  Those four registrations stay wired. Removing them to enforce a global "one owner"
  reading would strip `switch_workflow` of plan-mode, context-reset, session-mode and
  auto-start behaviour, which is a regression this card does not make and AC-D1 does not
  license.

`configure_session` is **deliberately** absent from `compileOnEnter`, so on `E1`–`E3` it is
orchestrator-owned and needs no engine case. Its absence is only a defect on `E4`, where it
is the one declared type that reaches no executor; AC-A6 requires that to warn, AC-A2
exempts it by name, and giving `E4` a `configure_session` executor is Out of scope. This is
a named hole, not a silent one.

## What

### Scope

A **step entry** is any event that makes a task **admitted to** step `S` when it was not
admitted to `S` immediately before. Admission, not the field write, is what defines it.
That phrasing is deliberate and it covers **two** shapes, only the first of which is a
`workflow_step_id` change:

- **(a) field change with admission** — `workflow_step_id` becomes `S` from some other
  step, with `wip_admitted` true. This is the ordinary case.
- **(b) admission in place** — the task is already parked at `S` with `wip_admitted`
  false and `queued_for_step_id` set, and a later promotion flips it to admitted
  **without touching `workflow_step_id` at all**. This is a real entry: it is the moment
  the task begins occupying the step, and it is when `on_enter` must dispatch.

An earlier revision of this spec defined an entry as a field change *and* admission,
which silently excluded shape (b) — while the Execution model simultaneously named
shape (b) as the place the record is allocated. The two statements contradicted each
other, and the path they disagreed about (`workflowStore.promoteSameStepTask`) is a
shipped WIP-queue path. Shape (b) is an entry. A placement that sets `workflow_step_id`
while leaving the task queued is **not** an entry on its own — nothing dispatches and no
record is allocated — but the promotion that later admits it **is**, under shape (b).

The complete, exhaustive list of code sites that constitute an entry, and the equally
explicit list of step-field writes that do **not**, is in the Execution model's
[write-site inventory](#the-write-site-inventory). Five entry paths exist:

- `E1` agent-turn auto-advance (`on_turn_complete` → `applyEngineTransition`)
- `E2` manual move (`move_task_kandev`, board drag, API)
- `E3` WIP-queue promotion (a task admitted after waiting on a step's WIP limit)
- `E4` `switch_workflow` action
- `E5` step change through a path that dispatches no `on_enter` today — the generic task
  update API (`UpdateTask` with `workflow_step_id`) and task start
  (`startTask` → `moveTaskToWorkflowStep`). These are entries by the definition above, so
  AC-A2 requires them to dispatch and AC-B7 requires them to allocate. They are named
  here rather than left implicit because they are the paths this spec's previous revision
  missed entirely

The ten `on_enter` action types in scope are the eight the embedded-YAML allow-list
accepts (`enable_plan_mode`, `auto_start_agent`, `reset_agent_context`,
`set_session_mode`, `clear_decisions`, `queue_run_for_each_participant`, `queue_run`,
`ensure_participant_seat`)
plus `run_code_review` and `configure_session`, which are declarable through the
import/sync path though not through `validOnEnter`.

### Execution model

This section defines the mechanism the acceptance criteria in group B and group F depend
on. It exists because those criteria are otherwise unsatisfiable: the only operation
marker in the tree today (`workflowStore.IsOperationApplied` /
`MarkOperationApplied`) is backed by an in-memory `sync.Map` that is empty after a
restart, is read before execution and written only after every action succeeds, and so
provides neither durability nor an atomic claim.

**What this model covers, and what it deliberately does not.** It governs the five
**engine-owned** action types only — `clear_decisions`, `queue_run_for_each_participant`,
`queue_run`, `run_code_review`, `ensure_participant_seat`. The five orchestrator-owned
types are **outside** it
(AC-B8). That boundary is a decision, not an omission, and it is what keeps the protocol
below free of a lease and a reaper: `reset_agent_context` and `auto_start_agent` are not
safe to re-execute after a crash, so admitting them would force a liveness mechanism this
card does not need. They already carry their own guards, AC-D1 requires them unchanged, and
they are not what stalls the Office loop.

**The step-entry record.** Every step entry has exactly one durable record. It carries:

- the **entry identity**: the tuple `(task_id, step_id, entry_seq)`, where `entry_seq` is
  an integer allocated at entry and strictly greater than any `entry_seq` previously
  allocated for that `(task_id, step_id)` pair. Monotonicity is enforced by a **UNIQUE
  constraint on `(task_id, step_id, entry_seq)`**, not by the allocator reading a maximum
  and adding one: two allocators racing on the same pair would both read the same maximum
  and write the same successor, and only the constraint makes the loser fail. A losing
  allocator's transition fails as a unit under AC-B7 and is recoverable by replay;
- an **action marker per engine-owned `on_enter` action**, keyed by the pair
  `(position in the declared list, action kind)` — see *Marker keying* below;
- a **declaration digest**: a canonical digest over the ordered list of
  `(position, action kind)` pairs of the whole `on_enter` declaration as read at
  allocation. It is what makes a position number mean the same action later that it meant
  at allocation — see *Marker keying* below;
- a **terminal flag** for the entry, set when every engine-owned position **in the step's
  declaration as read at that moment** holds a terminal marker. Evaluating the flag against
  the current declaration rather than against a snapshot means a declaration that shrank
  cannot leave an entry permanently non-terminal on account of a position that no longer
  exists. (A declaration that shrank also fails the digest comparison, which terminalises
  the entry outright; the flag rule is the belt to the digest's braces.) The flag exists so
  recovery is one indexed query rather than a per-entry join;
- a foreign key on `task_id` to `tasks` with **`ON DELETE CASCADE`**, so records cannot
  outlive the task they describe (AC-B9(e)).

**Who allocates it, and when.** The step-entry record is allocated **by the step
transition itself, in the same transaction as the `workflow_step_id` change**. This is
the load-bearing rule, and it is what makes replay and re-entry distinguishable at all.

The naive alternative — "a record already exists for `(task_id, step_id)`, so this is a
replay" — is **wrong**, and wrong in the direction that silently re-creates this card's
stall. The transition commits before `on_enter` dispatches (Persistence guarantees), so
at a genuine re-entry the task's `workflow_step_id` is *already* the destination step and
a record from the previous occupancy *already* exists. Both halves of that test are true
for a re-entry, so it would classify every Review → reject → Work → Review round trip as
a replay, reuse the old `entry_seq`, and let the 24-hour idempotency window suppress the
fan-out. No reviewer would be woken on any rejection round.

Allocating at the transition avoids this because a step *change* is exactly what defines
an entry:

- A **new entry** changed `workflow_step_id`, so it allocates a fresh `entry_seq`.
- A **replayed transition** finds the task already at the destination step, changes
  nothing, and therefore allocates nothing.
- A **replayed `on_enter` dispatch** reads the record the transition already committed and
  reuses its `entry_seq` verbatim.

The identity is therefore **deterministic** in the sense AC-B1 requires — it is *read
from committed state*, never recomputed from transition fields — while still being
distinct per entry. That is what lets AC-B2 and AC-B3 hold at the same time.

All five entry paths `E1`–`E5` shall allocate the record this way, at every site in the
[write-site inventory](#the-write-site-inventory)'s entry table. A path that admits a task
to a step without allocating one leaves that entry with no replay protection and no claim,
and does not satisfy AC-B2 or AC-F1.

A record is required only for a step whose `on_enter` declares at least one **engine-owned**
action. A step with an empty `on_enter` list (AC-E5), or one declaring only
orchestrator-owned types, has nothing this model governs and allocates nothing — which also
keeps the AC-B9 scan proportional to Office-style steps rather than to every transition on
every board.

**Ordering guarantee.** The step-entry record is committed **before** any `on_enter`
action for that entry executes — which the allocation rule above gives for free, since
the transition commits first. A crash between the transition and the dispatch leaves a
committed record with no markers at all, and recovery runs the entry from its first
engine-owned action.

**Persistence shape — mandated, because AC-B6 and AC-B7 together over-constrain it.**
The record and its markers shall live in a dedicated table in the **same database, reached
through the same writer handle**, as `tasks` and `workflow_step_decisions`. This is not a
free choice:

- AC-B7 requires allocation to share the *transition's* write, which is issued through the
  task repository;
- AC-B6 requires the `clear_decisions` marker to share the *delete's* transaction, which is
  issued through the workflow repository (`Repository.ClearStepDecisions`).

`backendapp/storage.go` builds both repositories over one shared `writer`/`reader`
`*sqlx.DB` pair, so a single transaction spanning both is possible — but **no plumbing for
one exists today**, and there is not one place to put it.

### The write-site inventory

**There is no single "the transition".** This is the most under-estimated fact about this
card, and a plan that misses it ships a fix that works on some entry paths and silently
does nothing on others.

The inventory below was rebuilt in Spec round 5 from an exhaustive
`grep -rn '\.WorkflowStepID = \|SET workflow_step_id'` over `apps/backend/internal/`
with `_test.go` excluded. The previous revision of this section was wrong in a way worth
recording, because the mistake is easy to repeat: it named `applyAdmissionPlacement` as
the `E3` WIP-promotion site "inside `PromoteQueuedTaskIfWorkflowStepHasCapacity`".
`applyAdmissionPlacement` has exactly **one** caller — `CreateTaskWithWorkflowStepAdmission`,
the task **creation** path — and the promoter neither calls it nor makes any placement
decision. Everything the previous revision said about "the site that decides the landing
step inside its own transaction" was therefore true of task creation and false of `E3`.
Do not re-derive this table from the function names alone; check the callers.

**Sites that constitute a step entry and therefore allocate a record.** Integration
watch-config writes (github, gitlab, jira, linear, sentry, azuredevops) are excluded
throughout: they write watch rows, not tasks.

| # | Write site | Entry path | Shape today |
|---|---|---|---|
| 1 | `orchestrator/event_handlers_workflow.go` `executeStepTransition` → `updateTransitionTaskWithCapacity` | `E1` (`on_turn_complete`, `on_turn_start`) | writes the field, then a repository update; **no transaction** |
| 2 | `orchestrator/workflow_store.go` `applyTransition` → `updateTransitionTask` → `repo.UpdateTask`, reached from `engine.applyTransition` | `E1` (engine), `E4` re-entry | **non-transactional** GetTask → mutate → `UpdateTask` |
| 3 | `task/service/service_workflow.go` `MoveTaskWithOptions` → `updateMovedTask` | `E2` manual move | delegates a changed-step move to `UpdateTaskWithWorkflowStepAdmissionAndState` → `updateTaskWithWorkflowStepAdmission`, which **owns a `BeginTx`**; commits, *then* publishes `task.moved`, and the orchestrator dispatches `on_enter` later off that event |
| 4 | `orchestrator/workflow_store.go` `pullOneFeederTask` | `E3` promotion | sets the landing step **before** the transaction, then calls the promoter |
| 5 | `task/service/service_workflow.go` `promoteFeederQueuedTask` | `E3` promotion | same shape as site 4, in a different package |
| 6 | `orchestrator/workflow_store.go` `promoteSameStepTask` | `E3` admission in place | **writes no step field at all** — sets `WIPAdmitted`, clears `QueuedForStepID` / `QueuedAt`, then calls the promoter. This is Scope's shape (b) |
| 7 | `task/repository/sqlite/task.go` `PromoteQueuedTaskIfWorkflowStepHasCapacity` | `E3` (shared) | owns a `BeginTx`; writes the **caller-supplied** `task.WorkflowStepID`; makes **no** placement decision. Four production call sites: `promoteQueuedTaskAtomically`, `promoteFeederQueuedTask`, `pullOneFeederTask`, `promoteSameStepTask` |
| 8 | `task/repository/sqlite/workflow.go` `AddTaskToWorkflow` | `E4` `switch_workflow` | owns a `BeginTx`, on a **different repository** from `workflowStore`; receives IDs only |
| 9 | `task/service/service_tasks.go` `UpdateTask` | `E5` generic API | sets `task.WorkflowStepID` unconditionally from `req.WorkflowStepID`; **no admission logic, no `on_enter` dispatch today** |
| 10 | `orchestrator/task_operations.go` `moveTaskToWorkflowStep`, reached from `startTask` | `E5` task start | guards on the field actually differing, plain `UpdateTask`, publishes `task.updated`; **no `on_enter` dispatch today** |

Sites 4, 5 and 6 are the real `E3` shape, and they share one property that matters: **the
landing step is chosen by the caller, before the transaction, in a package above the
repository.** Site 7 is the transaction they all funnel through, and it is where the
record must be written — but it cannot decide, on its own, which step the entry is for.
That is the opposite of the previous revision's claim and it changes the allocation
contract; see [Allocation inputs](#allocation-inputs).

**`E5` is new in this revision and it is deliberately named rather than left silent.**
Sites 9 and 10 change a task's step through paths that dispatch no `on_enter` at all
today. They are entries by Scope's definition, so under AC-A2 they must dispatch, and
under AC-B7 they must allocate. Naming them is what stops the next reader concluding the
inventory was sloppy; excluding them silently is precisely how this card's own defect got
into the tree.

**Step-field writes that are NOT entries, and why.** A named exclusion is a contract;
silence is a defect. These are the remaining non-test writes:

| Write site | Why it is not an entry |
|---|---|
| `task/repository/sqlite/task.go` `applyAdmissionPlacement`, from `CreateTaskWithWorkflowStepAdmission` | Task **creation**. There is no previous occupancy to leave, so nothing is re-entered. Creation into a step declaring engine-owned `on_enter` actions is **Out of scope** for this card and named there |
| `applyAdmissionPlacement`'s queued branch (sets `workflow_step_id`, leaves `wip_admitted` false, sets `queued_for_step_id`) | The task is waiting, not occupying. Scope's shape (b) covers the later promotion that admits it, and that promotion is site 6 |
| `orchestrator/task_operations.go` `advanceTaskWorkflowStep`, from `StartSessionForWorkflowStep` | Its guard returns early unless the step differs, and **every** current caller supplies the task's own step: the internal caller is `autoStartAgentCallback` passing `in.Step.ID` after the `E4` transition, and the external `LaunchSessionRequest` constructors pass the task's just-resolved or current step. It is a no-op on every path that exists, so allocating there would produce records for entries that never happen. **If a future caller supplies a differing step id, it becomes site 11** — `ResolveIntent` routes any request carrying both `SessionID` and `WorkflowStepID` here, so the API surface does not prevent it |
| `task/service/service_tasks.go` `RestoreTaskMessageRollback` / `task/repository/sqlite/task.go` `RestoreTaskMessageRollbackIfSessionState` | Message-rollback restore returns a task to a step it previously occupied as part of undoing history, not as a workflow advance. Re-dispatching `on_enter` there would re-fan-out reviewers for a round the user is explicitly rewinding. **Out of scope**, named |

**AC-B7's transaction requirement binds every site in the entry table.** Sites 3, 7 and 8
already own a transaction and need the allocation added inside it. Sites 1 and 2 own none
and need one introduced. Sites 4, 5 and 6 do not write the field themselves — they mutate
the in-memory task and hand it to site 7 — so their allocation happens inside site 7's
transaction, with the landing step and declaration passed down (see Allocation inputs).
Sites 9 and 10 own no transaction and need one introduced.

Two named consequences of missing a site, because both are silent failures rather than
errors:

- A path that admits a task to a step **without** allocating a record leaves that entry
  with no marker to compare-and-set. There is then nothing for AC-F1 to suppress and
  nothing for AC-B2 to make idempotent, and a dispatcher that requires a record has none.
  `E2` is the path a human uses to unstick a stalled Office card, so an `E2` gap
  reproduces this card's own stall on the recovery route.
- `E4` is today the **only** path that executes these actions correctly. A fix that leaves
  site 8 unallocated regresses working behaviour, which AC-A13 forbids.

### Allocation inputs

The record carries a declaration digest computed at allocation, and a record is required
only for a step whose `on_enter` declares at least one engine-owned action. Both facts
mean **the allocating code must have the landing step's `on_enter` declaration in hand at
the moment it writes**. Who supplies it is a contract, not an implementation detail,
because the two sites that own the transaction live in packages that cannot read it.

Verified constraints that make this load-bearing:

- `task/repository/sqlite` has no access to workflow declarations. It imports `wfmodels`
  only for `NewWIPLimitError`, never for `Events` / `StepEvents` or their JSON. The stub
  `workflow_steps` table its own `base_schema.go` creates carries an `events` column, but
  the comment there states it exists only so the workflow repository's later
  `ALTER ADD COLUMN`s become no-ops; nothing in the package reads or parses it.
- `AddTaskToWorkflow` (site 8) receives `taskID, workflowID, workflowStepID, position` —
  IDs only.
- `PromoteQueuedTaskIfWorkflowStepHasCapacity` (site 7) receives a task plus step ids and
  a limit, and the landing step was already chosen by its caller (sites 4, 5, 6).

**The contract.** Allocation takes exactly two inputs, both resolved by the caller in the
package that owns the workflow domain, and both passed down to the site that writes:

1. **the resolved landing step id** — the step the task will actually be admitted to, as
   that call path has already decided it, never the originally requested target; and
2. **the ordered `on_enter` declaration snapshot** for that step, read once, through the
   workflow repository's normal accessor — the same decode path every other reader uses.
   The allocating site does not re-read it, does not parse `events` JSON itself, and does
   not issue SQL against `workflow_steps`.

A caller that cannot resolve one of these shall not allocate and shall fail the
transition under AC-B7 rather than allocate a record with an unknown declaration.

Two consequences follow, and both are the point of stating this:

- **A site cannot decide for itself whether a record is needed.** "Does this step declare
  an engine-owned action?" is answered from input 2, by the caller, before the write.
- **Sites 4, 5 and 6 resolve both inputs and hand them to site 7.** They already choose
  the landing step, they already sit in packages that can read the declaration, and site 7
  is the transaction. Site 6 resolves the step it is admitting the task to even though it
  writes no step field, because under Scope's shape (b) that is the entry.

The alternative shapes are both rejected explicitly, so a builder does not have to weigh
them: raw `SELECT events FROM workflow_steps` inside the repository transaction duplicates
a decode path that will drift the first time the declaration shape changes; and a
cross-repository call reaching from the task repository into the workflow repository
mid-transaction is the plumbing this section's own persistence note says does not exist,
and this card does not add it.

`task.Metadata[models.MetaKeyAppliedDeferredMoves]` remains the in-tree precedent for the
**compare-and-set pattern** — `markDeferredMoveApplied` performs exactly the check-and-set
shape this model needs and returns `errDeferredMoveAlreadyApplied` on a duplicate — but it
is explicitly **not** the recommended *location*: a marker on the task row cannot share a
transaction with a `workflow_step_decisions` delete, so it cannot satisfy AC-B6.

**Marker keying, and the declaration digest that makes positions meaningful.** A marker is
keyed by `(entry identity, position, action kind)`. But a position number is only a stable
name for an action *while the declaration is unchanged*, and workflows in this repo are
git-synced, so a declaration can change between an entry and a later recovery. The record
therefore also carries a **declaration digest**, computed at allocation over the ordered
list of `(position, action kind)` pairs of the **whole** `on_enter` declaration.

"Position" throughout this spec means the index in the **whole** declared `on_enter` list,
including orchestrator-owned entries, not the index among engine-owned actions only. The
digest covers that whole list for the same reason: inserting an orchestrator-owned action
shifts every engine-owned position after it.

The digest's **inputs** are fixed by this spec — the ordered `(position, action kind)`
pairs, kinds serialized by their declared type string. Its **algorithm and encoding** are
the builder's choice, exactly as AC-H7 leaves them, because neither has an observable
consequence as long as it is stable within a process and across a restart.

The digest, not the per-position kind, is the authority:

- **The declaration is read once at the start of a dispatch, and that read serves the whole
  dispatch.** The digest is compared once, against that read; the declaration is not
  re-read between positions. Re-reading mid-dispatch would let a sync land between two
  actions of one entry and leave half of it executed against each version. This mirrors
  AC-F2's read-once rule for the participant set, and for the same reason.
- Every dispatch and every recovery re-dispatch recomputes the digest from the declaration
  as read at that moment and compares it to the record's.
- **If it matches**, positions name the same actions they named at allocation and the
  markers are honoured as written.
- **If it differs**, the entry is finished. One WARNING is emitted for the entry naming
  the workflow id, step id, task id and both digests (AC-A6), no further `on_enter` action
  executes for that entry, and the entry sets its terminal flag.
  **The position set to be marked is the one from the declaration just read for this
  digest comparison** — the same read, not a reconstruction of the declaration in force at
  allocation. This is stated explicitly because the record persists a *digest*, not the
  declaration, so the allocation-time position list is not recoverable from it, and a
  builder who reads this bullet in isolation will reasonably ask which set is meant. Every
  engine-owned position of the current declaration that does not already hold a terminal
  marker is marked `skipped(declaration_changed)`; markers left over from positions that no
  longer exist in the current declaration are never read again and are not cleaned up. This
  is the same "as read at that moment" convention the terminal-flag bullet above uses, and
  for the same reason.
- A marker whose kind does not match its position's current kind cannot occur while the
  digest matches. If one is observed anyway it is data corruption, and it takes the same
  disposition: `skipped(declaration_changed)`, warn, entry terminal.

**Per-position kind matching alone is not sufficient, and this is why.** It detects a
*substitution* at a fixed index. It does not detect an insertion, a deletion, or a reorder
of same-kind actions, which is an ordinary template edit. Worked example: an entry runs
`[reset_agent_context, clear_decisions, queue_run_for_each_participant]`, commits
`clear_decisions` at position 1 as `done`, and the backend crashes. The template is then
synced to
`[reset_agent_context, clear_decisions, clear_decisions, queue_run_for_each_participant]`.
Under per-position matching, position 1's `done` marker matches the *newly inserted*
declaration's kind, so a `clear_decisions` that has never run is treated as complete; and
position 2, which carries no marker and whose kind also matches, is the *already-committed*
one, so it is executed again — a second **committed** `clear_decisions` delete for one
entry, which AC-B2 forbids in those words. A deletion shifts positions the other way, so a
`done` marker lands on an action that never ran and that action is skipped forever, which
is this card's own bug class reintroduced by its fix. The digest closes both directions
with one comparison.

**What the digest deliberately does not cover.** It is computed over kinds and order only,
**not** over action configuration. A change to an action's `config` at the same position and
kind therefore does **not** invalidate the entry — the entry already executed that action
under the configuration in force at the time, and re-running it under new configuration is
worse than not running it. (The separate per-action config digest of AC-H7 serves the
idempotency key and has nothing to do with this one.)

**The cost, stated rather than discovered.** A declaration edited mid-entry abandons that
entry's remaining actions instead of guessing which marker belongs to which action. The
remedy is a fresh step entry, which is the same remedy AC-B5's terminal `skipped` already
prescribes and which Out of scope already names. Abandoning is the safe direction: the
alternative is executing the wrong action or silently skipping a real one, and both are
worse than a warned no-op.

**Marker states.** A marker is in exactly one of:

| State | Meaning | Terminal? |
|---|---|---|
| absent | never attempted for this entry | no |
| `in_progress(epoch)` | claimed by the backend process identified by `epoch`; outcome unknown | no |
| `done` | executed successfully | yes |
| `skipped(reason)` | reached its dispatcher and was determined not to execute | yes |
| `failed(cause)` | executed and failed | yes |

`skipped` reasons are `unrecognised` (AC-A6), `malformed` (AC-E3), `no_executor`
(AC-A6(b)), `declaration_changed` (above), `step_left` (AC-B9(a1)), `superseded`
(AC-B9(a2)), and `unresolvable` (AC-B9(e)). The list is closed: a builder needing an eighth
has found a case this spec did not anticipate and should route it back rather than
inventing one.

**Claim — per action, and it is the only suppression mechanism.** Before executing an
engine-owned action a dispatcher performs an atomic compare-and-set on that action's
marker from `absent` to `in_progress(this process's epoch)`. Only the winner executes.
After the effect commits it compare-and-sets `in_progress` to one of the three terminal
states.

There is deliberately **no entry-level exclusive lock**. An entry-level claim cannot work
here, because the step-entry record is created by the *transition*, so by the time any
dispatcher runs, the record always already exists — creation can therefore never serve as
the dispatcher's claim. Per-action compare-and-set is what actually delivers AC-F1, and it
delivers it without a lock that a dead claimant could hold forever.

**A dispatcher that loses a claim stops. It does not skip ahead.** When a compare-and-set
fails because another live dispatcher holds the marker, the losing dispatcher shall
**abandon the whole entry and return** — it shall not advance to the next position, and it
shall not attempt any later action of that entry. This is the rule, not an implementation
detail, and it is stated because "the claim attempt returns" and "the dispatcher returns"
are both readable into a per-action mechanism and only one of them is correct.

Skipping ahead breaks two criteria at once. AC-A3 requires the `clear_decisions` delete to
commit *before any `on_enter` action declared after it is executed*; a loser that advances
past a `clear_decisions` still held by the winner can enqueue the reviewer fan-out against
uncleared decisions. AC-A12 requires declared order across the whole list; a loser running
position 2 while the winner is still on position 1 violates it directly. And if the
winner's `clear_decisions` then fails, the loser has already executed exactly the actions
AC-C2 says must not run.

Nothing is lost by stopping: the winner is working through the same declaration in the same
order and will reach every remaining position itself. If the winner dies mid-entry its
markers become stale by epoch and AC-B9's startup scan resumes the entry. The loser is not
a fault and emits no diagnostic (AC-F1).

**Epoch, and how a dead claimant's marker is distinguished from a live peer's.** The
`epoch` recorded in an `in_progress` marker identifies the backend **process** that wrote
it. It shall be a value generated fresh at each backend start and **guaranteed not to
repeat across restarts** — a random identifier minted at boot satisfies this; a process id
does not, because operating systems reuse them, and neither does a whole-second timestamp,
because a fast restart can land inside the same second. Reusing an epoch would make a dead
process's marker indistinguishable from a live one's and strand the entry. The epoch
changes on every backend start. Exactly one backend process owns the database at a
time — `internal/backendapp/ownershiplock` acquires an OS lock over the runtime target at
startup and fails with "already owned by another backend" otherwise — so:

- an `in_progress` marker whose epoch **equals** the current process's epoch was written by
  a live peer goroutine. The observer does **not** re-claim, does **not** execute, and per
  the stop rule above **abandons the entry** rather than moving to the next position; it
  returns without a diagnostic. This is AC-F1's losing caller, and it is a normal outcome.
- an `in_progress` marker whose epoch **differs** from the current process's is definitionally
  stale — the process that wrote it is gone. It may be re-claimed by compare-and-set from
  `in_progress(old_epoch)` to `in_progress(current_epoch)`.

No lease, no timeout, and no periodic reaper is required, because staleness is decided by
identity rather than by elapsed time.

**Why re-claiming is safe, per action type.** Re-claiming means the effect may run a second
time, so each engine-owned type must be safe under that:

- `clear_decisions` — safe. AC-B6 puts the marker in the same transaction as the delete, so
  an `in_progress` marker proves the delete did **not** commit. This is the reason AC-B6 is
  stated as a transaction requirement rather than an ordering preference.
- `queue_run_for_each_participant` and `queue_run` — safe. Every run they enqueue carries
  the entry-scoped idempotency key of AC-B1, so a duplicate is suppressed by the runs queue
  and reported as a success (AC-B4).
- `run_code_review` — **not** idempotent, and this is an accepted, named outcome. It is
  at-most-once in normal operation and at-least-once across a crash: a backend that dies
  between launching a review pass and marking it may launch a second pass on recovery. A
  duplicate review is wasteful, not corrupting, and AC-A8 already makes a failure to start
  non-blocking. Making it exactly-once is Out of scope.

**Recovery — a bounded startup scan, not a reaper.** On backend start, once the store is
open and before the workflow engine begins serving triggers, the system selects step-entry
records whose terminal flag is unset and re-dispatches each one. This is a single indexed
query executed once per process start. It is not periodic, holds no lease, and applies no
timeout. Re-dispatch resumes in declared order, skipping every position already in a
terminal state, and re-claiming stale `in_progress` markers per the epoch rule above.

Without this scan a crash mid-entry would leave an Office task stalled at Review with no
diagnostic — the exact defect class this card exists to remove — so the scan is in scope
and AC-B9 states it.

**The scan runs before the engine serves triggers, so its error behaviour decides whether
the backend starts.** That makes the failure rules part of the contract rather than an
implementation detail:

- **The scan shall never prevent the backend from starting.** No record, and no number of
  records, may block boot. A backend that refuses to start because one row is unreadable
  has converted a stalled card into a dead installation.
- **A record whose task, step, workflow, or `on_enter` declaration cannot be resolved** —
  the row is gone, or a git sync removed the step — is marked terminal with
  `skipped(unresolvable)` and warns once (AC-A6). It is not executed and it is not
  re-selected on the next start.
- **A record that fails to process for a transient reason** — a repository read error, a
  contended write — is logged at ERROR, left non-terminal so a later start retries it, and
  the scan **continues to the next record**. One bad record does not abandon the rest.
- **If the scan's own query fails**, the failure is logged at ERROR and startup proceeds.
  The consequence is that entries left incomplete by the previous crash are not resumed
  until the next start, which is strictly better than not starting.

**Orphan records are prevented at the schema AND handled at the scan, and both are
required.** The step-entry table's `task_id` carries a foreign key to `tasks` with
`ON DELETE CASCADE` — the established in-tree pattern for task-scoped side tables
(`gitlab/store_mr_automation.go`, `office/repository/sqlite/tree_holds.go` both declare
exactly this against `tasks(id)`).

The cascade alone is **not** sufficient, and this is a repo-specific hazard rather than a
general one. SQLite enforces foreign keys only when `PRAGMA foreign_keys` is on for the
connection, and this codebase already carries explicit workarounds for databases where it
was not on at attach time — `linear/store_issue_watch.go` and `jira/store.go` both perform
a guard child `DELETE` with a comment saying exactly that. A `ON DELETE CASCADE` that is
silently a no-op on an older database would leave records whose task no longer exists, and
the startup scan would then dereference a missing task on every boot.

So both mechanisms are load-bearing: the cascade is the primary and keeps the table from
accumulating a row per deleted task forever, and AC-B9(e)'s `skipped(unresolvable)` is the
**backstop that must not be omitted on the grounds that the cascade makes it unreachable**.
`skipped(unresolvable)` also covers what no cascade can: a step or workflow removed by a
git sync, and a transient read failure.

### Acceptance criteria

#### A. Dispatch completeness

- **AC-A1** The system shall execute every `on_enter` action declared on the destination
  step, in the order declared, exactly once per step entry — subject to the **six** stated
  carve-outs, and to no others: AC-A9 (fixed positions for three types, on `E1`–`E3`,
  `E5`), AC-A13 (which component executes a type on `E4`, and in what order), AC-H4/AC-H8
  (a repeated declaration of `reset_agent_context` or `configure_session` executes once in
  total, not once per declaration), AC-A6/AC-E3 (an action that reaches no executor, or is
  malformed, does not execute and is marked terminal instead), **AC-A8** (`run_code_review`
  is at-most-once in normal operation and at-least-once across a backend crash), and
  **AC-B8** (for the five orchestrator-owned types, "exactly once" is guaranteed within a
  single dispatch and not across a crash, because they hold no marker).
  The last two are carve-outs against the *durability* of "exactly once" rather than
  against its ordering, and they are enumerated here because an earlier revision said "the
  four stated carve-outs, and to no others" while AC-A8 and AC-B8 each stated a fifth and
  sixth — making the criterion unsatisfiable as written. Without the AC-H8 carve-out an
  implementation satisfying AC-A1 literally would fire `configure_session` twice for a
  step declaring it twice, which AC-A11 and AC-D1 forbid.
- **AC-A2** WHEN a task enters a step through any of `E1`–`E5`, the system shall execute
  the same set of `on_enter` actions as it would through any other entry path. "Same set"
  means action *coverage*: no entry path may silently discard a declared action that
  another entry path executes. It does not require unifying prompt-delivery mechanics that
  already differ per path (see Out of scope).
  **Two exemptions, both by name, and no others:**
  (i) `configure_session` is not executed on `E4`. The discard is not silent — AC-A6
  requires it to warn on that path — and giving `E4` a `configure_session` executor is Out
  of scope.
  (ii) **Ordering is not required to be identical across entry paths**, because AC-A9's
  fixed positions exist only where `processOnEnter` runs. On `E1`–`E3` and `E5`,
  `reset_agent_context` and `configure_session` are hoisted to the front and
  `auto_start_agent` is deferred to the end; on `E4` the engine executes the compiled list
  in declared order with no hoisting. A step declaring
  `[clear_decisions, reset_agent_context]` therefore runs reset-then-clear on `E1`–`E3` /
  `E5` and clear-then-reset on `E4`, and **that is conforming**. What this criterion
  requires across paths is coverage and per-path internal ordering (AC-A9, AC-A12), not a
  single global order.
  This exemption is stated rather than resolved because the alternative is worse in both
  directions: hoisting inside `compileOnEnter` would change `E4` behaviour that works
  today, which AC-A13 forbids, and dropping AC-A9's hoist on `E1`–`E3` would change
  `reset_agent_context`-before-`auto_start_agent` ordering that AC-D1 freezes. Closing the
  divergence is Out of scope.
  No type other than (i) may be discarded by any path.
- **AC-A3** WHEN a step declaring `clear_decisions` is entered, the system shall delete
  every `workflow_step_decisions` row whose `task_id` is the entering task and whose
  `step_id` is the **destination** step, before any `on_enter` action declared after it
  is executed. It shall not clear decisions for the step being left.
- **AC-A4** WHEN a step declaring `queue_run_for_each_participant` with `role: R` is
  entered, the system shall enqueue exactly one run for each **distinct** participant of
  that step whose role equals `R`, and shall enqueue no run for any participant whose
  role does not equal `R`. Distinctness is defined by AC-A5.
- **AC-A5** The participant set for AC-A4 shall be the merged template-level
  (`task_id = ''`) and per-task (`task_id = <entering task>`) rows for the step, with the
  per-task row winning on a `(role, agent_profile_id)` collision, enumerated in
  `role ASC, position ASC, id ASC` order. The merged list shall then be **de-duplicated
  on `(role, agent_profile_id)`, retaining the first row in that order**; there is no
  UNIQUE constraint on `workflow_step_participants`, so two rows in the same tier can
  carry the same pair. Runs shall be enqueued in the resulting order. This
  de-duplication applies to the fan-out only. It shall not change the participant set
  `evaluateWaitForQuorum` reads, which is out of scope for this card; a builder shall not
  "fix" quorum's input while implementing AC-A4.
- **AC-A6** IF a step declares an `on_enter` action that does not reach execution, THEN
  the system shall emit a log record at **WARNING** naming the workflow id, step id, step
  name, and action type, and shall continue executing the remaining `on_enter` actions.
  This covers **both** points at which an action is discarded today:
  (a) an action type no dispatcher translates into an executable action at all,
  and (b) a translated action for which no callback is registered in the running
  configuration. Both are silent today; both must warn.
  Whether a type is a candidate for this warning is decided **per entry path**, by the
  ownership partition's column for that path, not globally. A type with an owner on the
  entry path in question reaches execution and shall not warn: on `E1`–`E3` and `E5` that
  includes
  `configure_session` and `reset_agent_context`, neither of which shall ever warn there
  even though `compileOnEnter` does not compile the former. A type with no owner on the
  path in question shall warn: on `E4` that is exactly `configure_session`, and the record
  shall additionally name the entry path so the reader can tell this expected, excluded
  gap from an unexpected one.
- **AC-A7** WHEN a step declaring `queue_run` on `on_enter` is entered, the system shall
  enqueue one run against the action's resolved target, using the same target resolution
  (`primary`, `workspace.ceo_agent`, `participant_role:<role>`, `agent_profile_id:<id>`)
  the engine applies to `queue_run` under any other trigger. The run it enqueues shall
  carry the AC-B1 entry-scoped idempotency key and shall be excluded from the coalescing
  window under AC-H9, on the same terms as a fan-out run. A resolution failure is an
  ordinary action failure under AC-C3. This criterion exists as a separate statement
  because specifying entry identity for the fan-out alone would leave an `on_enter`
  `queue_run` with no replay protection under AC-B2 and coalescing-eligible on re-entry,
  reproducing this card's stall in miniature for that action type.
- **AC-A8** WHEN a step declaring `run_code_review` on `on_enter` is entered, the system
  shall start a code-review pass for the task. "Started" is observable as a
  `task_review_runs` row carrying the entering task's id, the destination step's id as
  `workflow_step_id`, and trigger `workflow_step`; that row is the artifact a test asserts
  on. A failure to start shall not block the entry and shall not fail the action —
  `runCodeReviewCallback` logs and returns nil today, and that is preserved. This action is
  at-most-once per entry in normal operation and at-least-once across a backend crash
  (Execution model, *Why re-claiming is safe*); a duplicate review pass after a crash is an
  accepted outcome, not a defect.
- **AC-A9** **On the entry paths where `processOnEnter` runs — `E1`–`E3` and `E5`, and
  not `E4`** — three action types keep their current fixed position regardless of where
  they appear in the declared list:
  `reset_agent_context` **first**, then `configure_session`, both before every other
  `on_enter` action; and `auto_start_agent` **last**, after every other `on_enter` action.
  AC-A1's declared-order rule applies to the remaining actions among themselves.
  All three positions are today's behaviour, verifiable in `processOnEnter`: the first two
  are dispatched ahead of the action loop, and `auto_start_agent` is not executed in the
  loop at all — the loop only records that it was declared, and the prompt is built and
  dispatched after the loop ends. An earlier revision named only the first two, which left
  AC-A12 requiring `auto_start_agent` in declared order against a structure that cannot
  produce it without moving prompt delivery — a change AC-D1 explicitly freezes.
  **On `E4` this criterion does not apply**: the engine compiles and executes the declared
  list in order, with no hoisting and no deferral, and AC-A13 preserves that. AC-A2(ii) is
  the corresponding cross-path exemption.
- **AC-A10** `clear_decisions`, `queue_run_for_each_participant`, `queue_run`,
  `run_code_review`, and `ensure_participant_seat` do not depend on a live agent
  session. WHEN a task enters a step
  declaring one of them and the task has no running session, the system shall still
  execute it. Session-scoped actions (`enable_plan_mode`, `set_session_mode`,
  `reset_agent_context`, `configure_session`, `auto_start_agent`) remain skipped in that
  case, as today.

  Today the whole `on_enter` sequence is abandoned when no session resolves
  (`fromStepAndTargetForTaskMoved` returns early on `session == nil`), so an Office task
  moved to Review by hand — a normal path — queues nothing even once the dispatch defect
  is fixed. Meeting this card's stated acceptance for entry path `E2` requires closing
  this.
- **AC-A11** Each `on_enter` action type shall be executed by exactly one component per
  step entry, per the ownership partition. No action type shall be executed by both
  `processOnEnter` and the engine for the same entry. Specifically: the action list
  `processOnEnter` delegates to the engine shall contain only engine-owned kinds, and the
  five orchestrator-owned kinds shall not be executed through a shared callback registry
  as part of that delegation.
- **AC-A12** AC-A1's declared order shall hold **across** the ownership partition, not
  only within each owner's subset. Splitting the declared list by owner and running all
  orchestrator-owned actions before all engine-owned ones does not satisfy AC-A1 unless
  the declared list already has that shape. Delegating a contiguous run of engine-owned
  actions in a single call is permitted, because that preserves declared order; delegating
  a non-contiguous set as one batch is not. `reset_agent_context`, `configure_session` and
  `auto_start_agent` keep their fixed positions and are exempt (AC-A9) — the first two
  leading, `auto_start_agent` trailing. Concretely, a step declaring
  `[auto_start_agent, clear_decisions]` conforms when the clear runs first and the agent
  starts after it, on the paths where AC-A9 applies; this criterion does not require the
  auto-start dispatch to be re-ordered, and re-ordering it would breach AC-D1.
  **A test for this criterion shall not place `auto_start_agent` last in its declared
  list**, since that is the one position where the exemption and a violation are
  indistinguishable.
- **AC-A13** On `E4` (`switch_workflow`), where `processOnEnter` is not in the call path,
  the engine shall remain the executor of every type it compiles, and the unconditional
  registrations of `enable_plan_mode`, `reset_agent_context`, `auto_start_agent` and
  `set_session_mode` in `buildWorkflowCallbacks` **shall remain wired**. Removing them to
  enforce a single global owner per action type is a regression, not a conformance fix: it
  would strip `switch_workflow` of plan-mode, context-reset, session-mode and auto-start
  behaviour that works today. AC-A11's prohibition is on one *entry* executing a type
  twice; it is not a prohibition on two entry paths having different executors.

#### B. Identity, idempotency, and re-entry

- **AC-B1** Each step entry shall carry the entry identity defined in Execution model.
  Every run enqueued by **any** `on_enter` action during that entry — the
  `queue_run_for_each_participant` fan-out and a bare `queue_run` alike — shall carry an
  idempotency key derived from that identity together with the step id, task id, the
  action's position in the declared list, the target agent profile id, and the action's
  canonical configuration digest (AC-H7). The position is part of the key so that two
  declarations at different positions that resolve identically are still distinguishable
  from a replay of one of them.
- **AC-B2** IF the same step entry is processed more than once — a replayed event, a
  retried transition, or a process restart mid-entry — THEN the system shall not enqueue
  a second run for a participant already enqueued for that entry, and shall not perform a
  second *committed* `clear_decisions` delete for that entry. The markers that guarantee
  this shall be durable across process restart.
  "Second committed delete" is the precise form, and the precision matters: a
  `clear_decisions` whose marker is `in_progress` after a crash **is** re-executed on
  recovery, and that is correct rather than a violation, because AC-B6 puts marker and
  delete in one transaction, so an `in_progress` marker proves the earlier attempt never
  committed. What this criterion forbids is a delete running again after one has committed,
  which the terminal `done` marker prevents (AC-B5).
- **AC-B3** WHEN a task re-enters a step it has previously occupied (the Review →
  rejection → Work → Review loop), the system shall treat it as a new step entry per the
  distinguishing rule in Execution model: a new `entry_seq`, a fresh `clear_decisions`,
  and a fresh fan-out to every matching participant. The runs enqueued by that fan-out
  shall not be suppressed by the 24-hour idempotency window applied to the previous
  entry's runs, because their keys differ in `entry_seq`.
- **AC-B4** IF the runs queue idempotency-suppresses a fan-out run — the 24-hour window
  on an identical AC-B1 key — THEN the system shall treat that as a successful enqueue,
  not a failure, and shall not retry it. Because keys differ in `entry_seq` (AC-B3) and in
  declared position (AC-B1), an identical key means exactly **one** thing: a replay of the
  same action, at the same position, within the same entry (AC-B2). It never means a
  distinct entry, and it never means a second declaration within one entry — those carry
  different positions and therefore different keys (AC-H5). A suppression observed under
  any other circumstance is a defect in the key, not a duplicate to be tolerated.
- **AC-B5** The system shall record a durable marker for each engine-owned `on_enter`
  action position it *reaches* within a step entry — not only for those it completes —
  using the state machine and keying defined in Execution model. It shall not execute an
  action whose marker for that entry is already terminal (`done`, `skipped`, or `failed`).
  Specifically:
  (a) an action that executed successfully is marked `done`;
  (b) an action that reached its dispatcher and was determined not to execute is marked
  `skipped(reason)`, drawn from the closed list in Execution model — `unrecognised` and
  `no_executor` (AC-A6), `malformed` (AC-E3), `declaration_changed` (Execution model),
  `step_left` (AC-B9(a1)), `superseded` (AC-B9(a2)), `unresolvable` (AC-B9(e)). `skipped`
  is **terminal**: recovery
  does not resume at it, and its AC-A6 / AC-E3 diagnostic is emitted once, at the
  transition into `skipped`, not on every subsequent observation. `declaration_changed` is
  the one reason applied to an entry's whole remaining tail at once rather than to a single
  position, because the digest comparison that produces it is an entry-level fact;
  (c) an action that executed and failed is marked `failed(cause)`, which is likewise
  **terminal**. Recovery does not retry it, consistent with Out of scope's exclusion of
  retry. For a partially-failed fan-out this means the participants whose enqueue succeeded
  keep their runs, and the participants whose enqueue failed are not retried and are
  recorded only by their AC-C1 ERROR records; the `cause` recorded in the marker is the
  attempted count, the failed count, and the first failure's cause in AC-A5 order, as
  AC-C1 specifies.
  Both halves of this are deliberate and both have a cost: marking a non-executing action
  terminal means an action that would become executable later — a session appears, an
  adapter is wired — does **not** run for that entry, and the remedy is a fresh entry. Not
  marking it would leave recovery resuming at it forever, so the entry would never reach a
  terminal state and the AC-A6 / AC-E1 diagnostics would repeat on every replay, degrading
  the signal group G exists to provide. Terminal-on-reach is the chosen trade.
- **AC-B6** The completion marker for `clear_decisions` shall be committed in the **same
  transaction** as the delete it marks. A marker written after a separately-committed
  delete leaves a window in which a crash re-runs the clear on recovery and discards
  decisions recorded in between — the exact corruption AC-C2 and AC-F3 exist to prevent.
  Actions whose own effect is independently idempotent (the fan-out and `queue_run`, whose
  runs carry the AC-B1 key) do not require this and may mark completion separately.
  This is also what makes re-claim safe for `clear_decisions`: because marker and delete
  share one transaction, an `in_progress` marker proves the delete did not commit, so a
  recovering process may re-run it (Execution model, *Why re-claiming is safe*). Satisfying
  this criterion constrains where the marker lives — a marker on the task row cannot share
  a transaction with a `workflow_step_decisions` delete — which is why Execution model
  mandates the store rather than leaving the shape free.
- **AC-B7** IF the step-entry record cannot be allocated, THEN the step transition that
  would have carried it shall fail as a unit — record and step change share one
  transaction, so neither lands — and the system shall emit a log record at **ERROR**
  naming the task id and step id. This is the one case where the step change does not
  stand, and it does not contradict AC-C4: AC-C4 governs a *committed* transition whose
  `on_enter` actions then fail, whereas here the transition itself never committed. Executing actions
  without a committed record would leave them with no replay protection at all, which is
  worse than not executing them; the entry is recoverable by a later replay, and the
  ERROR record is what makes the stall visible rather than silent.
- **AC-B8** The five orchestrator-owned action types shall **not** participate in the
  step-entry marker system. They write no marker, are not consulted by recovery, and their
  behaviour under a replayed or crash-recovered dispatch is exactly today's. In particular
  a session action skipped for want of a live session (AC-A10) records nothing durable, so
  it can never be resurrected by a later dispatch and can never execute after an
  engine-owned action, which is what keeps AC-A9's fixed leading position and AC-A12's
  declared order intact under recovery. This exclusion is what allows the model to work
  without a lease or a reaper: `reset_agent_context` and `auto_start_agent` are not safe to
  re-execute after a crash, and admitting them would require one. Their exclusion means
  AC-A1's "exactly once" is, for these five and only these five, guaranteed only within a
  single dispatch and not across a crash — stated here rather than left to be discovered.
- **AC-B9** On backend start, once the store is open and before the workflow engine begins
  serving triggers, the system shall select every step-entry record whose terminal flag is
  unset and re-dispatch it, resuming in declared order and skipping positions already in a
  terminal state. This shall be a single query executed once per process start; it shall not
  be periodic, shall hold no lease, and shall apply no timeout. Without it a backend crash
  mid-entry leaves the task stalled at Review with no diagnostic, which is the defect class
  this card exists to remove. Six rules bound what it may do:
  (a) **It shall not re-dispatch an entry the task has left, and it shall not re-dispatch a
  superseded entry into a step the task has since re-entered.** Two separate tests, both
  required:
  (a1) a record whose `step_id` is not the task's current `workflow_step_id` shall be marked
  terminal with `skipped(step_left)` and not executed. Without this rule a crash during
  Review followed by a manual move to Done would, on the next start, fan out reviewers for a
  step the task no longer occupies.
  (a2) among the records for a `(task_id, step_id)` pair that survives (a1), **only the one
  with the greatest `entry_seq` may be re-dispatched**; every other non-terminal record for
  that pair shall be marked terminal with `skipped(superseded)` and not executed. The
  greatest `entry_seq` is the task's current occupancy of that step, because `entry_seq` is
  allocated monotonically per pair and the task is presently at that step.
  Test (a1) alone is **not** sufficient, and this is the case it misses: an entry left
  non-terminal, whose task then leaves the step and returns before the next restart, has a
  `step_id` that once again equals the task's current step. It passes (a1), and rule (c)'s
  `entry_seq` ordering then re-dispatches the **older** record first — re-running
  `clear_decisions` against the *live* entry's decisions and fanning the reviewers out a
  second time. That is not a crash-only path: rule (f) deliberately leaves a transiently
  failed record non-terminal so a later start retries it, and the task can leave and return
  in between, so the design's own recovery behaviour produces the input that corrupts it.
  (b) **It shall not resume an entry holding a `failed` marker on a `clear_decisions`
  position** (AC-C2), which it shall mark terminal instead.
  (c) Records shall be processed in `(task_id, step_id, entry_seq)` order so the scan is
  deterministic and reproducible in a test.
  (d) **It shall not resume an entry whose declaration has changed.** A record whose
  declaration digest does not match the step's current `on_enter` declaration shall be
  marked terminal per the Execution model's digest rule, and not executed.
  (e) **It shall not resume an entry it cannot resolve.** A record whose task, step,
  workflow, or `on_enter` declaration cannot be loaded shall be marked terminal with
  `skipped(unresolvable)` and shall emit the AC-A6 WARNING naming the task id and step id.
  (f) **It shall not prevent the backend from starting, under any condition.** A record
  that fails to process for a transient reason shall be logged at **ERROR**, left
  non-terminal so a later start retries it, and the scan shall continue with the remaining
  records; a failure of the scan's own query shall be logged at **ERROR** and startup shall
  proceed. A backend that refuses to boot because one step-entry row is unreadable has
  turned a stalled card into a dead installation, which is strictly worse than the defect
  this scan exists to fix.

#### C. Failure isolation

- **AC-C1** IF enqueuing a run for one participant fails, THEN the system shall still
  attempt every remaining matching participant, and shall emit a log record at **ERROR**
  naming the task id, step id, participant id, agent profile id, and the cause. This
  requires changing `QueueRunForEachParticipantCallback` from its current
  abort-on-first-error loop to collect-and-continue; the fan-out reports failure only
  after every matching participant has been attempted.
  **AC-C1 is the complete diagnostic contract for `queue_run_for_each_participant`, and
  AC-C3 does not additionally apply to it.** A fan-out in which some participants failed
  emits its per-participant ERROR records and its one AC-G1 INFO record, and **no** AC-C3
  action-level ERROR record. Stated because AC-C3 is otherwise written broadly enough to
  cover the fan-out, which would produce N+1 error records for one event with no rule for
  which is authoritative.
  **The `failed(cause)` marker AC-B5(c) writes for a partial fan-out shall record the
  number of participants attempted (the size of the AC-A5 de-duplicated matching set, not
  the number of participants on the step), the number that failed, and the cause of the
  **first** failure in the AC-A5 enumeration order.** That order is deterministic, so the
  recorded cause is reproducible in a test rather than being whichever goroutine lost a
  race. A single scalar cause cannot represent three participants failing three different
  ways, and the alternative — concatenating every cause into one field — produces an
  unbounded value in a marker that is read by recovery.
- **AC-C2** IF `clear_decisions` fails, THEN the system shall not execute **any**
  remaining `on_enter` action for that step entry, and shall emit a log record at
  **ERROR** naming the workflow id, step id, task id, and cause. (The field list is stated
  because AC-G2 requires "the identifying fields named in that criterion" to be discrete
  fields, and every other diagnostic criterion names its own; without it AC-G2 has nothing
  to assert against for this record.) Stale decisions plus a fresh fan-out can satisfy a quorum guard that no
  reviewer has actually voted for in this round; the entry stops rather than risk a
  spurious advance. This is deliberately broader than "skip the queue actions": a
  `run_code_review` declared after a failed `clear_decisions` starts an agent pass
  against a step whose decision state is unknown, so it is stopped too. AC-C3 does not
  apply to actions following a failed `clear_decisions`. Those remaining actions are left
  with **no marker at all** — they never reached their dispatcher, so AC-B5's
  terminal-on-reach rule does not apply to them. The entry therefore never sets its
  terminal flag, so AC-B9 would otherwise re-dispatch it on the next backend start and run
  those actions against a step whose decisions were never cleared — a worse outcome than
  stopping. AC-B9's scan shall therefore treat an entry holding a `failed` marker on a
  `clear_decisions` position as terminal and shall not resume it.
- **AC-C3** IF an `on_enter` action other than `clear_decisions` **and other than
  `queue_run_for_each_participant`** fails, THEN the system shall attempt the remaining
  declared actions and shall emit a log record at **ERROR** naming the workflow id, step
  id, action type, and cause. The fan-out is excluded because AC-C1 already specifies its
  complete diagnostic contract; the continue-past-failure half of this criterion still
  governs the fan-out, which is to say a failed fan-out does not stop the actions declared
  after it.
- **AC-C4** IF any `on_enter` action fails, THEN the step transition shall remain
  committed. The task shall not be rolled back to its previous step.
- **AC-C5** The continue-past-failure behaviour required by AC-C1 and AC-C3 shall apply
  to the `on_enter` dispatch path only. `engine.evaluateActions` is shared by every
  trigger and aborts on the first callback error today; `on_turn_start`,
  `on_turn_complete`, `on_exit`, and the Phase-2 event triggers shall keep that
  abort-on-first-error behaviour unchanged. A change that makes all triggers
  continue-on-error does not satisfy this criterion.

#### D. Non-regression

- **AC-D1** For a step whose `on_enter` declares only the action types
  `processOnEnter` recognises today, the observable behaviour shall be unchanged:
  agent-profile session switching, the stale-`RUNNING` → `WAITING_FOR_INPUT` flip, plan
  mode, session permission mode, agent-context reset ordering (before `auto_start_agent`),
  conditional session configuration, ACP vs passthrough prompt delivery, the
  profile-switch auto-launch when no `auto_start_agent` is declared, waiting-state event
  publication, and queued-message drain. In particular none of these shall fire twice
  (AC-A11).
- **AC-D2** WHERE `features.office` is disabled and the run-queue, participant, and
  decision adapters are consequently unwired, kanban workflows shall behave exactly as
  before, and a step declaring an office-only `on_enter` action shall produce the AC-A6
  warning rather than an error, a stall, or a failed transition.
- **AC-D3** The regression test for this card shall be: a task entering a step whose
  `on_enter` declares `queue_run_for_each_participant{role: reviewer}` with exactly one
  reviewer participant attached, arriving through an ordinary `on_turn_complete`
  transition, produces exactly one queued run, for that reviewer's agent profile, with
  the action's configured reason.
- **AC-D4** A second entry into the same step for the same task, following a rejection
  round, shall produce a further run for that same reviewer. This criterion exists
  because AC-D3 alone passes under an entry identity that suppresses every re-entry
  inside the 24-hour idempotency window, which would silently reproduce the stall this
  card fixes.

#### E. Empty, nil, and boundary behaviour

- **AC-E1** IF `queue_run_for_each_participant` matches zero participants, THEN the
  system shall enqueue no runs, shall not fail the entry, and shall emit a log record at
  **WARNING** naming the step id, step name, and configured role. A fan-out that matches
  nobody is indistinguishable at runtime from a stalled step, and is a misconfiguration
  worth naming.
- **AC-E2** IF a matching participant has an empty `agent_profile_id`, THEN the system
  shall skip that participant, emit the AC-C1 ERROR record, and continue with the
  remaining participants.
  **Such a participant counts as a failure**, on the same terms as an enqueue that
  returned an error: it is included in the failed count of the AC-B5(c) `failed(cause)`
  marker, it is eligible to be the first failure whose cause that marker records (in AC-A5
  order), and it is **excluded** from AC-G1's "runs enqueued" count while still being
  included in AC-G1's "matching participants" count. Consequently a fan-out in which
  **every** matching participant has an empty profile enqueues nothing, emits N ERROR
  records plus one AC-G1 INFO record with enqueued = 0, and marks the action
  `failed(cause)` — terminal, so AC-B9 does not resume it. This is stated because
  "skip … and continue" reads as a benign no-op, and the choice between `done` and
  `failed` decides whether recovery ever revisits the entry.
- **AC-E3** IF `queue_run_for_each_participant` is declared with an empty or absent
  `role`, THEN the system shall enqueue no runs and shall emit a log record at **ERROR**
  naming the workflow id and step id. An empty role shall not be interpreted as "all
  roles".
- **AC-E4** WHEN `clear_decisions` runs against a `(task, step)` pair with no recorded
  decisions, the system shall complete successfully as a no-op, and shall record its
  AC-B5 completion marker.
- **AC-E5** WHEN a step declares an empty or absent `on_enter` list, the system shall
  behave exactly as it does today (AC-D1's session-lifecycle path, including the
  no-`on_enter` early return and its active-turn guard). No step-entry record is required
  for such a step, nor for a step whose `on_enter` declares only orchestrator-owned types,
  because the execution model governs engine-owned actions only (AC-B8). A non-empty list
  containing at least one engine-owned action does require a record, even if every action
  in it turns out to be `skipped`.

#### F. Concurrency

- **AC-F1** IF two dispatches **of the same step entry** — the same
  `(task_id, step_id, entry_seq)` — are processed concurrently, THEN at most one shall
  execute each engine-owned `on_enter` action. The suppression shall come from the
  per-action compare-and-set defined in Execution model: not from a read-then-write pair
  over an in-memory map, which both callers can pass, and not from an entry-level exclusive
  lock, which cannot work here because the step-entry record is created by the transition
  and therefore always already exists by the time any dispatcher runs. The caller that
  loses a claim is a normal outcome, not a fault: it shall not emit a WARNING or ERROR
  record, so that the diagnostics AC-A6 and AC-C1–C3 introduce stay meaningful.
  **A caller that loses a claim shall abandon the entry and shall not attempt any later
  position of it.** "At most one executes each action" is a per-action guarantee, but a
  loser that skipped to the next position could execute a later action while the winner is
  still committing an earlier one — enqueueing the fan-out before the `clear_decisions`
  delete AC-A3 requires to precede it, and violating AC-A12's declared order. The winner is
  walking the same declaration in the same order and will reach the remaining positions
  itself; if it dies first, AC-B9's scan resumes them.
  This criterion governs duplicate dispatches of **one** entry only. Two *distinct* entries
  carry different `entry_seq` values, hold independent markers, and shall **both** execute
  in full; suppressing the second would contradict AC-B3 and re-create this card's stall on
  the rejection round.
- **AC-F2** IF a step entry's `on_enter` actions run concurrently with a participant
  being added to or removed from that step, THEN the fan-out shall act on the participant
  set as read once at the start of that action's execution, in a single query. A
  participant added after that read shall not receive a run for that entry, and a
  participant removed after that read may still receive one. The system shall not
  partially re-read the set mid-fan-out.
- **AC-F3** IF a step entry's `on_enter` actions run concurrently with a decision being
  recorded for the same `(task, step)`, THEN the `DELETE` statement's commit is the
  linearization point and every decision row falls on exactly one side of it: a row
  committed **before** it shall be deleted, and a row committed **after** it is unaffected
  and may exist alongside a successful clear. The system shall not produce a state in which
  `clear_decisions` reports success while a decision row committed **before** the delete
  still remains.
  The prohibition is scoped to rows committed before the delete deliberately. Stated
  without that scope it would forbid its own permitted outcome — a decision inserted an
  instant after the delete commits both "survives it" and "remains while the clear reported
  success" — and would be unsatisfiable by any implementation short of serialising the
  whole step.
  This guarantee rests on the clear being a **single** `DELETE` statement over
  `(task_id, step_id)`, which is atomic in SQLite; a clear implemented as a read followed
  by a delete of the read ids does not satisfy this criterion.

#### G. Observability

- **AC-G1** WHEN `queue_run_for_each_participant` finishes — whether it succeeded, or
  failed for some participants under AC-C1 — the system shall emit a log record at **INFO**
  naming the task id, step id, configured role, the number of matching participants, the
  number of runs enqueued, and the number of runs idempotency-suppressed. "Enqueued" counts
  every participant whose enqueue returned success, **including** those suppressed by the
  24-hour window, because AC-B4 declares a suppressed run a successful enqueue; the separate
  suppressed count is what keeps the two distinguishable to a reader. The record is emitted
  on the partial-failure path too, so an absent record means the action never finished
  rather than that it matched nobody.
- **AC-G2** Every diagnostic required by AC-A6, AC-C1, AC-C2, AC-C3, AC-E1, and AC-E3
  shall be a structured record carrying the identifying fields named in that criterion as
  discrete fields, not interpolated into a message string.

#### H. Defaults and repeated declarations

- **AC-H1** WHEN `queue_run_for_each_participant` omits `reason`, the system shall use the
  literal string `on_enter` as the queued run's reason. It shall not leave the reason
  empty and shall not substitute the step name. (This is what
  `queueRunForEachParticipantReason` produces today via `string(in.Trigger)`; it is stated
  so it survives the dispatch change.)
- **AC-H2** WHEN `queue_run_for_each_participant` omits `payload`, the system shall
  enqueue the run with no payload (SQL `NULL` / absent), not an empty object.
- **AC-H3** The `queue_run_for_each_participant` fan-out shall select participants by role
  alone. It shall **not** filter on `decision_required`, and shall not default that flag
  to any value. (See "Related stalls" for the consequence.)
- **AC-H4** WHEN a step declares the same `on_enter` action type more than once, each
  declaration shall be executed in declared order, subject to AC-H8.
- **AC-H5** IF two `queue_run_for_each_participant` declarations on the same step resolve
  to the same role, reason, and payload, THEN **both shall fan out**, and each matching
  participant shall receive one run per declaration. The two declarations sit at different
  positions in the declared list, AC-B1 puts that position in the idempotency key, so their
  keys differ and the runs queue suppresses neither.
  This criterion is a **consequence of AC-B1**, not a separate mechanism: a builder shall
  add neither a de-duplication pass to collapse the two, nor a position-blind key to make
  the runs queue collapse them. Both would be a second mechanism fighting the first.
  It is stated explicitly because the natural reading of "the same action declared twice"
  is that the second is redundant. It is not treated as redundant here, and the reason is
  that this spec cannot tell an accidental duplicate from a deliberate one: the same
  action at two positions with different neighbours (a `clear_decisions` between them, say)
  is a legitimate declaration whose second fan-out is the point. A step that declares the
  same fan-out twice and did not mean to is a misconfiguration, and it is visible — AC-G1's
  INFO record is emitted once per action, so two records name it.
- **AC-H6** The 24-hour idempotency window the runs queue applies to fan-out runs is fixed
  at its current value and is not made configurable by this change.
- **AC-H7** The configuration digest in the AC-B1 key shall be canonical: computed over
  the action's `role`, `reason`, and `payload` **after defaults are applied**, so an
  omitted `reason` and an explicit `reason: on_enter` produce the same digest (AC-H1), and
  an omitted `payload` and an explicit null produce the same digest (AC-H2). Map keys
  shall be serialized in sorted order so the digest is stable across runs. Two
  declarations that differ only in a field not named here shall not produce different
  keys.
- **AC-H8** `reset_agent_context` and `configure_session` shall each execute at most once
  per step entry regardless of how many times they are declared. Their fixed position
  (AC-A9) makes "each declaration in declared order" meaningless for them, so AC-H4 does
  not apply to these two types.
- **AC-H9** Runs enqueued by `queue_run_for_each_participant` shall be **excluded from the
  5-second coalescing window**. Coalescing is keyed on `(agent_profile_id, reason)`, which
  carries no entry identity, so a re-entry whose previous run is still `status='queued'`
  would otherwise be merged into it and produce no new runnable run — contradicting AC-B3
  and reproducing this card's stall in miniature. It applies to every run enqueued by an
  `on_enter` action, fan-out and bare `queue_run` alike (AC-A7, AC-B1). Idempotency
  suppression (AC-B4) is unaffected and still applies.
  **No general exclusion mechanism exists today and this card must add one.** The only
  opt-out currently implemented is specific to task comments —
  `shouldCoalesceRun(req)` returns `!commentkeys.HasTaskCommentPrefix(req.IdempotencyKey)`,
  with a matching `idempotency_key NOT LIKE '<comment prefix>%'` in `CoalesceRun` — and
  `QueueRunRequest` carries no other field that could signal exclusion. The mechanism this
  criterion needs is therefore precisely the one it forbids reusing: overloading the comment
  prefix would make a fan-out run indistinguishable from a comment run in both predicates
  and in the `runs` table, and is not permitted.
  The exclusion shall be carried by a **discrete field on the request**, read by both
  `shouldCoalesceRun` and `CoalesceRun`'s SQL exclusion, and not encoded in the idempotency
  key. The field's name is the builder's choice; its shape is not. `QueueRunRequest` is
  declared **twice** — `internal/runs/service/service.go` and
  `internal/workflow/engine/adapters.go`, which the service's own doc comment requires to
  match — so the field lands in both, and a plan budgeting for one will not compile.

### Related stalls this does **not** fix

These are real, reproduce today, and survive this change. They are named here so a
builder does not assume the Office loop is end-to-end after this card, and so a reviewer
does not file them as regressions.

- **Quorum requires `decision_required`, fan-out does not.**
  `queue_run_for_each_participant` selects participants by role alone;
  `evaluateWaitForQuorum` counts only participants with `decision_required = true`. A
  reviewer attached with `decision_required = false` will be woken and will vote, and its
  vote will be ignored by the guard.
- **Zero required participants stalls permanently.** `evaluateWaitForQuorum` fails closed
  when the required set is empty. A Review step with no `decision_required` reviewer never
  advances, no matter how many runs are queued. After this change AC-E1 at least makes it
  visible.
- **`auto_start_agent` on Office tasks** and the status-driven review path have their own
  defects, tracked separately.

## Scope reality

The originating card framed this as two missing `switch` cases. It is larger, and the
plan should be sized accordingly:

- durable per-entry state with a per-action compare-and-set (Execution model) — **in
  scope**, a new table, not a reuse of `appliedOps`;
- **record allocation at TEN separate step-change write sites, not one.** This is the
  single largest item and the easiest to under-budget. The Execution model's
  [write-site inventory](#the-write-site-inventory) enumerates them with their transaction
  shapes; do not re-derive it from function names, because the revision that did so named a
  task-creation helper as the WIP-promotion site. They span three packages
  (`orchestrator`, `task/service`, `task/repository/sqlite`) and two repositories. Three
  sites already hold a transaction; four hold none and need one introduced; one commits the
  step change in a different call stack from the `on_enter` dispatch that follows it; three
  choose the landing step above the repository and hand it down. Two of the ten (`E5`) do
  not dispatch `on_enter` at all today and need the dispatch added, not just the
  allocation. **A plan that budgets for `applyTransition` alone covers one of the ten and
  silently leaves `E2`, `E3`, `E4` and `E5` with no step-entry record at all**;
- **transaction plumbing that does not exist yet** at sites 1 and 2: `applyTransition` is a
  non-transactional GetTask → mutate → `UpdateTask` today, and AC-B7 requires the record to
  share the transition's transaction while AC-B6 requires the `clear_decisions` marker to
  share the delete's. Both repositories sit on one shared writer handle, so this is
  possible, but no method currently spans them;
- **a new discrete field on `QueueRunRequest` in both of its declarations**, plus the
  matching `shouldCoalesceRun` predicate and `CoalesceRun` SQL exclusion (AC-H9) — the
  coalescing opt-out this card needs does not exist today in any general form;
- a bounded startup re-dispatch scan for entries left incomplete by a crash (AC-B9);
- rewriting `QueueRunForEachParticipantCallback`'s fan-out loop from abort-on-first-error
  to collect-and-continue (AC-C1);
- introducing continue-past-failure on the `on_enter` dispatch path **without** changing
  `evaluateActions` for the other triggers (AC-C5);
- resolving the shared-registry double-execution hazard (AC-A11) **without** unwiring the
  four registrations `E4` depends on (AC-A13).

None of these is "route the call somewhere else". A plan that budgets only for dispatch
routing will under-scope this card.

## Out of scope

Named exclusions, not silence. Each of these is a decision to leave the behaviour as it
is today.

- **Retrying a failed fan-out.** AC-C1 requires the failure be attempted-past and logged.
  It does not introduce a re-queue, a backoff, or a parked state for the entry.
- **Migrating session-lifecycle work into the engine.** The ownership decision covers
  action semantics only; `processOnEnter` keeps everything AC-D1 enumerates.
- **A metric or expvar counter for fan-out.** Structured logs (AC-G) are the required
  surface. Adding `workflow_on_enter_*` counters alongside the existing `workflow_*`
  family is a reasonable follow-up and is not required here.
- **Changing `wait_for_quorum` semantics**, including the `decision_required` /
  role-filter mismatch and the fail-closed empty-set behaviour described above.
- **Widening `validOnEnter`** to admit `run_code_review` or `configure_session` in
  embedded-YAML templates. AC-A8 covers dispatching them when they arrive through the
  import or git-sync path, which is how they are declarable today.
- **Unifying `auto_start_agent` prompt delivery across entry paths.** `E1`–`E3` deliver
  the step prompt through `processOnEnter`, which handles passthrough sessions by writing
  to the PTY; `E4` delivers it through `autoStartAgentCallback`, which skips passthrough
  entirely. That divergence predates this card, AC-A2 deliberately does not require
  closing it, and closing it here would put a passthrough-launch change in a defect fix.
- **Retiring `appliedOps`.** The in-memory operation marker keeps its current role for
  the triggers that use it today. This card adds durable per-entry state for `on_enter`;
  it does not migrate the rest.
- **A lease, a timeout, or a periodic reaper for abandoned claims.** Staleness is decided
  by process epoch rather than elapsed time (Execution model), so none is needed. The
  bounded **startup** re-dispatch scan of AC-B9 is a different thing and is **in** scope.
- **Making `run_code_review` exactly-once across a crash.** It is at-most-once in normal
  operation and at-least-once under recovery (AC-A8). A duplicate review pass is wasteful,
  not corrupting, and giving it an entry-scoped guard would mean a schema change in the
  review subsystem for no correctness gain on this card.
- **Executing `configure_session` on `E4`.** It is the one declared type `switch_workflow`
  drops. AC-A6 requires it to warn there and AC-A2 exempts it by name, so the gap is
  visible rather than silent; giving the engine a `configure_session` executor is a
  separate change.
- **Bringing the five orchestrator-owned action types into the marker system.** AC-B8
  excludes them. Their "exactly once" is guaranteed within a dispatch and not across a
  crash, which is today's behaviour and is stated rather than left to be discovered.
- **Resurrecting a `skipped` action later in the same entry.** AC-B5 makes `skipped`
  terminal, so an action that found no session or no registered callback does not run if
  one appears afterwards. The remedy is a fresh step entry.
- **Reconciling an entry against an edited declaration.** When the declaration digest
  differs, the entry is abandoned rather than re-aligned. Working out which surviving
  marker corresponds to which action after an insert, delete or reorder is a diff problem
  with no safe default, and getting it wrong either re-runs a committed `clear_decisions`
  or silently skips an action that never ran. Abandoning is a warned no-op; the remedy is a
  fresh entry.
- **Aggregating every cause of a partially-failed fan-out into the marker.** AC-C1 records
  the per-participant causes in full; the marker keeps counts plus the first cause in AC-A5
  order. Concatenating N causes into a durable field read by recovery would make its size
  a function of participant count for no gain the ERROR records do not already provide.
- **A second diagnostic for a failed fan-out.** AC-C3 is scoped away from
  `queue_run_for_each_participant` deliberately. Emitting both an action-level record and
  the per-participant records would double-report one event with no rule for which is
  authoritative.
- **Adding a UNIQUE constraint to `workflow_step_participants`.** AC-A5 de-duplicates at
  read time instead; changing that table is a migration this card does not need. (The new
  step-entry table does carry a UNIQUE constraint, on `(task_id, step_id, entry_seq)`;
  that is a different table and AC-B7 depends on it.)
- **Pruning or retaining terminal step-entry records.** They accumulate one row per step
  entry per task and are never read once terminal. A retention policy is a reasonable
  follow-up; this card neither prunes them nor promises to.
- **Any frontend change.** No new panel, indicator, or copy. The user-visible consequence
  is that an Office reviewer's run appears where today nothing appears.
- **Dispatching `on_enter` on task creation.** A task created directly into a step whose
  `on_enter` declares engine-owned actions does not dispatch them and allocates no record.
  Creation is not a step entry — there is no previous occupancy to leave — and the shipped
  Office flow creates tasks into the first step, not into Review. Named because
  `applyAdmissionPlacement`, the creation-path helper, was misidentified as the WIP
  promotion site in an earlier revision, and a reader who follows that mistake back will
  land here.
- **Re-dispatching `on_enter` on a message-rollback restore.** Returning a task to a step
  it previously occupied as part of undoing history is not a workflow advance; re-running
  the fan-out would wake reviewers for a round the user is explicitly rewinding.
- **Closing the `E4` ordering divergence.** AC-A2(ii) names it as conforming. Hoisting
  `reset_agent_context` / `configure_session` inside `compileOnEnter`, or deferring
  `auto_start_agent` there, would change `switch_workflow` behaviour that works today,
  which AC-A13 forbids; dropping the hoist on `E1`–`E3` would change ordering AC-D1
  freezes. Either direction is a separate change.
- **Seeding reviewer or approver participants into the `Office Default` template.**
  Participants are attached per task; a task with no reviewer attached still stalls, and
  that is existing intended behaviour.

## Failure modes

| Condition | Behaviour | Criterion |
|---|---|---|
| Unrecognised `on_enter` action type | WARNING, marked `skipped(unrecognised)`, remaining actions continue | AC-A6, AC-B5 |
| Type with an owner on this entry path | Executes via that owner, no warning | AC-A6, partition |
| `configure_session` on `E4` only | WARNING naming the entry path, no execution | AC-A6, AC-A2, Out of scope |
| Step declaration changed between entry and recovery (kind substituted, action inserted, removed, or reordered) | Declaration digest differs; every non-terminal position marked `skipped(declaration_changed)`, one WARNING, entry terminal | AC-A6, AC-B9(d), Execution model |
| Action's `config` changed, kinds and order unchanged | Digest matches; markers honoured; entry continues | Execution model |
| `clear_decisions` fails | ERROR, **all** later actions skipped, transition stands | AC-C2, AC-C4 |
| One participant's enqueue fails | ERROR, remaining participants attempted | AC-C1 |
| Fan-out matches zero participants | WARNING, no runs, entry succeeds | AC-E1 |
| Fan-out role empty or absent | ERROR, no runs | AC-E3 |
| Participant has no agent profile | ERROR for that participant, others continue | AC-E2 |
| Duplicate `(role, agent_profile_id)` rows | De-duplicated at read, one run | AC-A5 |
| Run idempotency-suppressed (same entry replay) | Treated as success, no retry | AC-B4 |
| Re-entry within 5s, prior run still queued | New run enqueued; coalescing excluded | AC-H9 |
| Re-entry within 24h | New run enqueued; keys differ in `entry_seq` | AC-B3, AC-D4 |
| Office adapters unwired (kanban-only) | AC-A6 warning, no error, no stall | AC-D2 |
| Entry replayed after restart | No duplicate runs, no second `clear_decisions` | AC-B2, AC-B5 |
| Two concurrent dispatches of **one** entry | At most one executes each action, via per-action CAS; loser abandons the entry and is silent | AC-F1, Execution model |
| Loser of a claim reaches a later position | Forbidden: it stops at the lost claim and does not advance | AC-F1, AC-A3, AC-A12 |
| Two **distinct** entries, same task and step | Both execute in full; keys differ in `entry_seq` | AC-F1, AC-B3 |
| Backend dies mid-entry | Startup scan re-dispatches; stale `in_progress` re-claimed by epoch | AC-B9, Execution model |
| Recovered entry whose task has since left the step | Marked `skipped(step_left)`, not executed | AC-B9(a) |
| Two allocators race the same `(task_id, step_id)` | UNIQUE constraint fails one; its transition fails as a unit | AC-B7, Execution model |
| Step declares only orchestrator-owned types | No step-entry record allocated | AC-E5, AC-B8 |
| Action fails | Marked `failed`, terminal, not retried on recovery | AC-B5, AC-C1, AC-C3 |
| Entry whose `clear_decisions` is marked `failed` | Treated as terminal; not resumed | AC-C2, AC-B9 |
| Orchestrator-owned action after a crash | No marker, no recovery; today's behaviour | AC-B8 |
| Step-entry record cannot be allocated | ERROR, transition and record fail as a unit | AC-B7 |
| Step entry with no running session | Session-independent actions still run | AC-A10 |
| Entry path changes the step without allocating a record | Forbidden: that entry has no claim and no replay protection | AC-B7, Execution model |
| WIP promotion into a full target lands the task in the feeder step | Record allocated for the **feeder**, the step actually occupied | AC-B7, Execution model |
| Placement sets `workflow_step_id` but leaves the task queued, not admitted | Not a step entry; no record, no dispatch. The later same-step promotion allocates, under Scope shape (b) | AC-E5, Execution model |
| Queued task admitted in place, `workflow_step_id` unchanged | **Is** a step entry (shape (b)); record allocated, `on_enter` dispatches | Scope, AC-B7 |
| Step changed through the generic task-update API or task start (`E5`) | Is a step entry; record allocated and `on_enter` dispatches, neither of which happens today | Scope, AC-A2, AC-B7 |
| `advanceTaskWorkflowStep` called with the task's own current step | Not an entry; guard returns early, no record | Execution model exclusion table |
| Task created directly into a step declaring engine-owned actions | Not an entry; no record, no dispatch | Out of scope |
| Message-rollback restore returns a task to a former step | Not an entry; no record, no dispatch | Out of scope |
| Recovery finds two non-terminal entries for one `(task, step)` after a leave-and-return | Only the greatest `entry_seq` resumes; older ones `skipped(superseded)` | AC-B9(a2) |
| Every matching participant has an empty `agent_profile_id` | N ERROR records, one INFO with enqueued = 0, action marked `failed`, terminal | AC-E2, AC-C1, AC-G1, AC-B5(c) |
| Same declared list entered via `E1`–`E3`/`E5` and via `E4` | Same action coverage; **ordering may differ** — hoists apply only where `processOnEnter` runs | AC-A2(ii), AC-A9, AC-A13 |
| Step declares `auto_start_agent` before another action | On `E1`–`E3`/`E5` the agent still starts last; conforming | AC-A9, AC-A12 |
| Declaration mutated between two positions of one dispatch | Dispatch completes against the version read at its start | Execution model |
| Recovered record whose step, workflow, or declaration cannot be loaded | Marked `skipped(unresolvable)`, WARNING, not executed | AC-B9(e) |
| Recovered record fails to process for a transient reason | ERROR, left non-terminal for a later start, scan continues | AC-B9(f) |
| Startup scan's own query fails | ERROR, **startup proceeds**; entries resume on a later start | AC-B9(f) |
| Task deleted while a non-terminal record exists | Record removed by `ON DELETE CASCADE`; never seen by the scan | AC-B9(e), Execution model |
| Same fan-out declared twice on one step | **Both** fan out; keys differ in declared position | AC-H5, AC-B1 |
| Fan-out partially fails | AC-C1 records only, no AC-C3 record; marker cause = attempted/failed counts + first cause in AC-A5 order | AC-C1, AC-B5, AC-C3 |

## Persistence guarantees

- A **step-entry record** is allocated by the step transition, in the same transaction as
  the admission it records, **at every site in the Execution model's write-site inventory
  entry table**, and is therefore committed before any `on_enter` action for that entry
  executes. It carries the entry identity `(task_id, step_id, entry_seq)`, a marker per
  engine-owned action position, the declaration digest, and a terminal flag. It lives in a
  dedicated table in the same database, on the same writer handle, as `tasks` and
  `workflow_step_decisions` — a constraint AC-B6 and AC-B7 jointly impose — with a
  `task_id` foreign key to `tasks` `ON DELETE CASCADE`. It survives restart. The
  **per-action** compare-and-set on its markers is what AC-F1 relies on; creating the
  record is not, because the transition always creates it first.
- A run enqueued by AC-A4 or AC-A7 is a row in `runs` carrying `agent_profile_id`,
  `task_id`, `workflow_step_id`, the action's `reason`, the AC-B1 idempotency key, and the
  AC-H9 coalescing-exclusion field. It survives restart and is claimed by the runs
  scheduler.
- A code-review pass started by AC-A8 is a row in `task_review_runs` carrying the task id,
  the step id as `workflow_step_id`, and trigger `workflow_step`.
- `clear_decisions` is a committed single-statement delete of `workflow_step_decisions`
  rows for the `(task_id, step_id)` pair.
- The step transition is committed before `on_enter` actions execute and is never rolled
  back by their failure (AC-C4).

## Verification

Every criterion above is observable without reading application logs *except* the
diagnostic criteria, which are observable by asserting on the structured log record.

Every criterion has at least one line below. The list is written to be checkable against
the criteria themselves: a criterion appearing in no line is a defect in this section.

- AC-A1, AC-A3, AC-A4, AC-A5, AC-B2, AC-B3, AC-B5, AC-E1–E4, AC-C1–C3, AC-F2, AC-H1–H5,
  AC-H7–H9:
  backend Go tests over the dispatch path reached through an **ordinary transition**, not
  through a direct `HandleTrigger(TriggerOnEnter)` call. A test that calls the engine
  directly does not satisfy these criteria — that is precisely the gap
  `TestOfficeDefaultWorkflow_FullCycleSmoke` left open.
- AC-A2: the coverage assertion executed once per entry path `E1`–`E5`. Ordering is
  **not** asserted across paths — AC-A2(ii) exempts it — so a test that asserts one global
  order across `E1`–`E5` is asserting something this spec does not require and shall not be
  written. `E5`'s two sites need the `on_enter` dispatch added, so this line fails today for
  reasons the other paths do not share.
- AC-A9: a step declaring `clear_decisions` *before* `reset_agent_context` and
  `auto_start_agent` *before* `clear_decisions`, entered via `E1`, asserting reset runs
  first and the agent starts last. **Asserted on `E1`–`E3` and `E5` only.** The same
  declaration entered via `E4` shall be asserted to run in **declared** order — that is
  AC-A2(ii)'s exemption, and asserting a single global order instead would fail a
  conforming implementation.
- AC-A2(ii) and AC-A13 together: the identical declared list driven through `E1` and `E4`,
  asserting the same action *coverage* on both and explicitly asserting the **different**
  orders. This is the test that fails if a builder "fixes" the divergence by hoisting
  inside `compileOnEnter`.
- Allocation inputs: a step whose `on_enter` declares only orchestrator-owned types
  asserted to allocate **no** record on every entry site, which is only decidable if the
  declaration reached the allocator; plus a promotion whose landing step differs from the
  requested target, asserting the digest stored is the **landing** step's.
- AC-A10: a manual move (`E2`) of a task with no running session into a step declaring
  `queue_run_for_each_participant`, asserting the fan-out still happens.
- AC-A11: a step declaring `reset_agent_context`, `enable_plan_mode`, `set_session_mode`
  and `auto_start_agent` alongside an engine-owned action, asserting each session action
  takes effect exactly once — a counter or spy on the underlying service method, not just
  an end-state check, because a double-fire of an idempotent setter is invisible in the
  end state.
- AC-A6: asserted at both discard points named in that criterion, not just one, plus two
  negative assertions and one positive: `configure_session` produces no warning on
  `E1`–`E3`, `reset_agent_context` produces none on any path, and `configure_session`
  **does** warn on `E4` with the entry path named in the record.
- AC-A7: a step declaring `queue_run` on `on_enter`, entered through an ordinary
  transition, asserting one `runs` row against the resolved target carrying an idempotency
  key that differs across two entries of the same step (the AC-B3 shape applied to
  `queue_run`), and carrying the AC-H9 exclusion field.
- AC-A8: a step declaring `run_code_review` on `on_enter`, asserting a `task_review_runs`
  row with the entering task's id, the destination step's id, and trigger `workflow_step`;
  plus a launch forced to fail, asserting the entry still completes and later actions run.
- AC-A13: `switch_workflow` into a step declaring `enable_plan_mode`,
  `reset_agent_context`, `set_session_mode` and `auto_start_agent`, asserting all four
  still take effect on that path. This is the regression test for the partition being
  misread as global; it fails if the four registrations are unwired.
- AC-B1: two entries of the same step asserted to produce different idempotency keys, and
  two declarations at different positions asserted to produce keys differing in position.
- AC-H5: a step declaring `queue_run_for_each_participant{role: reviewer}` **twice** with
  identical config, one reviewer attached, asserted to produce **two** runs for that
  reviewer — not one. This is the assertion that fails under a de-duplication pass or a
  position-blind key, which are the two ways a builder can re-create the contradiction
  this criterion was rewritten to remove.
- AC-B4: an identical key re-submitted within the window, asserting the action reports
  success and does not retry. Plus the negative that keeps AC-B4's "exactly one thing"
  honest: the two-identical-declarations case of AC-H5 asserted **not** to be an
  idempotency suppression.
- AC-B8: a crash-recovery dispatch asserted **not** to re-run `reset_agent_context` or
  `auto_start_agent` through the marker system, because they hold no marker — the negative
  that keeps the exclusion honest.
- AC-B9: a step-entry record left non-terminal, a freshly constructed store standing in for
  a restart, asserting the scan re-dispatches it, resumes at the first non-terminal
  position, and re-claims an `in_progress` marker written under a different epoch. Plus the
  AC-C2 case: an entry whose `clear_decisions` is `failed` is **not** resumed; and the
  AC-B9(a) case: a non-terminal record whose `step_id` is not the task's current step is
  marked `skipped(step_left)` and its actions do not run.
- AC-B9(d) and the declaration digest: an entry with position 1 marked `done` and position
  2 absent, recovered after an action of the **same kind** is inserted earlier in the
  declaration, asserting the entry is terminalised with `skipped(declaration_changed)` and
  that **no second `clear_decisions` delete commits**. The same test with an action
  **removed** instead, asserting no position is executed under a marker that belongs to a
  different action. A per-position kind check passes both of these while doing the wrong
  thing, so asserting the digest comparison alone does not satisfy this line — assert the
  observable effect. Plus the negative: an action's `config` changed with kinds and order
  unchanged, asserting the entry is **not** terminalised and its remaining actions run.
- AC-B9(e) and AC-B9(f): a non-terminal record whose step has been deleted, asserting
  `skipped(unresolvable)` and the WARNING; a record whose processing raises a transient
  error, asserting an ERROR record, the record still non-terminal, and **the remaining
  records still processed**; and the scan's query itself forced to fail, asserting startup
  completes. The startup assertion is the load-bearing one — an implementation that
  propagates the error would fail it and no other line in this section would catch that.
- Task-delete cascade: a task with a non-terminal step-entry record deleted, asserting the
  record is gone and the next scan does not see it.
- AC-C4: an `on_enter` action forced to fail, asserting the task's `workflow_step_id` is
  still the destination step afterwards. This is the criterion that keeps a failed fan-out
  from rolling the card back, and it needs its own assertion rather than being assumed.
- AC-F3: a decision committed before the delete asserted deleted, and a decision committed
  after it asserted to survive alongside a successful clear — both, since the criterion has
  two sides and asserting one would pass an implementation that violates the other.
- AC-G1: the INFO record asserted on both the all-succeeded path and the
  partial-failure path, with the enqueued and suppressed counts asserted separately.
- AC-G2: each record required by AC-A6, AC-C1, AC-C2, AC-C3, AC-E1 and AC-E3 asserted to
  carry its named fields as discrete structured fields, not interpolated into the message.
- AC-B9(a2): two non-terminal records for one `(task_id, step_id)` — the older left
  `in_progress` under a previous epoch, the newer current — with the task presently at that
  step; asserting the older is marked `skipped(superseded)`, that its `clear_decisions`
  does **not** run, and that the newer one resumes. A scan implementing only the `step_id`
  equality test passes AC-B9(a1) and fails this line, which is the whole point of splitting
  them.
- AC-E2 accounting: a fan-out of three matching participants of which one has an empty
  `agent_profile_id`, asserting one ERROR record for it, an AC-G1 INFO record with matching
  = 3 and enqueued = 2, and a `failed(cause)` marker counting it among the failures. Plus
  the all-empty case, asserting enqueued = 0 and a terminal `failed` marker that AC-B9 does
  not resume.
- AC-A1's carve-outs: `run_code_review` asserted to run a second time across a simulated
  crash (AC-A8), and `reset_agent_context` asserted **not** to (AC-B8) — the pair that
  makes AC-A1's "exactly once" honest about what it does and does not guarantee across a
  restart.
- AC-H6: no test; it is a statement that a constant is not made configurable.
- AC-A12: a step whose declared list interleaves an orchestrator-owned and an
  engine-owned action (for example `set_session_mode`, `clear_decisions`,
  `auto_start_agent`), asserting the three take effect in declared order. A test whose
  declared list happens to be owner-contiguous cannot distinguish a conforming
  implementation from a split-by-owner one and does not satisfy this criterion.
- AC-B2 and AC-B5: the entry dispatched twice against the same committed step-entry
  record, asserting one `clear_decisions` and one run per participant. The restart case is
  covered by constructing the second dispatch against a freshly built store that shares
  only the durable state.
- AC-B6: a `clear_decisions` whose delete commits and whose marker does not, asserting
  that recovery does not re-clear. Reachable by asserting the marker and the delete share
  one transaction rather than by fault injection, if the former is simpler.
- AC-B7: record allocation forced to fail, asserting the task's `workflow_step_id` is
  unchanged, no `on_enter` action ran, and the ERROR record carries the task and step ids.
  Plus the race: two allocators against one `(task_id, step_id)`, asserting the UNIQUE
  constraint rejects one rather than both committing the same `entry_seq`.
- AC-B3 and AC-D4: enter, reject, return, re-enter **inside** the 24-hour window, and
  assert a second run is enqueued. This is the criterion that fails if the entry identity
  is derived from transition fields alone.
- AC-C5: the existing `evaluateActions` behaviour for a non-`on_enter` trigger asserted
  unchanged — a failing action still aborts the remaining actions for that trigger.
- AC-F1: two concurrent dispatches of the same entry, asserting exactly one execution per
  action and no WARNING or ERROR record from the losing caller; plus two *distinct* entries
  asserted to both execute in full, which is the half that fails if the claim is scoped to
  `(task_id, step_id)` instead of to the entry. **Plus the stop rule**: a step declaring
  `[clear_decisions, queue_run_for_each_participant]`, dispatcher A holding the
  `clear_decisions` claim and blocked mid-delete, dispatcher B arriving and losing that
  claim — assert B enqueues **no** runs before A's delete commits. A test that only counts
  final executions passes under skip-ahead, because the winner eventually runs everything
  and the totals come out right; the assertion has to be about ordering, not totals.
- AC-C1 vs AC-C3 precedence: a fan-out with three matching participants of which two fail
  for different reasons, asserting exactly two AC-C1 ERROR records, **zero** AC-C3
  action-level ERROR records, one AC-G1 INFO record, and a `failed(cause)` marker carrying
  attempted=3, failed=2, and the first failure's cause in AC-A5 order. Asserting the marker
  cause pins the enumeration order, which is what makes the value reproducible.
- AC-B7 across every entry path: record allocation asserted on **every site in the
  write-site inventory's entry table**, driven through each site's real entry point —
  `on_turn_complete`, an engine transition, a manual move, both feeder promotions, the
  same-step admission, `switch_workflow`, the generic task update, and task start. This is
  the line that fails if a plan budgets for `applyTransition` alone, and it is deliberately
  written per-site rather than per-path because three sites serve `E3` alone.
  Plus the two cases a happy-path WIP test will not reach: a promotion into a **full**
  target that lands the task in the feeder step, asserting the record's `step_id` is the
  feeder and not the requested target; and a **queued** placement (`wip_admitted` false,
  `queued_for_step_id` set), asserting **no** record is allocated then and that one **is**
  allocated by the later same-step promotion that admits the task — which is Scope's
  shape (b) and writes no step field, so a test keyed on the field changing will miss it.
  Plus the negatives that keep the exclusion table honest: `advanceTaskWorkflowStep`
  invoked with the task's own current step, asserting **no** record is allocated; and a
  message-rollback restore, asserting the same.
- Declaration read-once: a dispatch of a multi-action entry with the step's declaration
  mutated between two positions, asserting the dispatch completes against the version read
  at its start and does not execute a mix of the two.
- AC-D1, AC-D2, AC-E5: the existing orchestrator `processOnEnter` suite must pass
  unchanged; no assertion in it may be weakened to accommodate the new dispatch.
- AC-D3: the named single-reviewer regression test.
