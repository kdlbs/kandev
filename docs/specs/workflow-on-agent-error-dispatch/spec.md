---
status: draft
created: 2026-09-02
updated: 2026-09-03
owner: Kandev
---

# Workflow `on_agent_error` Dispatch for Non-Office Tasks

Decisions:

- [ADR-0004 task-model-unification](../../decisions/0004-task-model-unification.md) — establishes the
  workflow engine as "the universal coordinator for task-scoped agent runs", declares
  `on_agent_error` as one of the seven Phase 2 triggers, and specifies in §7 that it
  "fires on the failing task **at its current step**".

Related spec (same defect class, different trigger):

- [`docs/specs/workflow-on-enter-action-dispatch/spec.md`](../workflow-on-enter-action-dispatch/spec.md)
  — `on_enter` declared, compiled, only partly dispatched. Its AC-A6 (warn rather than silently
  discard) is reused here; see [Action vocabulary](#action-vocabulary).

## Why

`on_agent_error` is the workflow trigger that recovers from an agent session failure. Every layer
around it works. The dispatch does not.

Verified 2026-09-02 by `grep -rn TriggerOnAgentError --include=*.go` over `apps/backend`, minus
`_test.go`. The **only** production fire site is `internal/office/service/event_subscribers.go:677`
(`dispatchAgentErrorTrigger`), gated on an Office `*models.Run` resolved by `resolveLifecycleRun`
(`event_subscribers.go:162`), which returns `sql.ErrNoRows` for a task with no claimed Office run.
`internal/orchestrator` and `internal/workflow` contain zero fire sites. The remaining non-test hits
are the constant, the payload type, the trigger-map entry and the Office dispatcher's
session-resolution branches — telling those apart from a fire site is the regression guard's whole
problem; see [Verification](#verification).

Everything else round-trips: the YAML loader parses it (`config/workflows/loader.go:302`), the engine
compiles it (`engine/types.go:299`), it persists (`workflow/models/models.go:158`), exports through
git-sync (`export.go:275`), and appears over the API (`task/dto/dto.go:1099`). And **no path except
embedded YAML validates the action type at all.** So a kanban step can declare `on_agent_error`, have
it validate, compile, survive an export/import cycle — and silently never run. An inert recovery path
reads as configured, so nobody looks for the missing safety net.

### Current exposure, measured

Queried against the local install's SQLite database on 2026-09-02: exactly **one** step row declares
`on_agent_error`, `Office Default / Work` (`queue_run` → `workspace.ceo_agent`). `Fix / Chore` and
`New Feature Dev` are real kanban workflows there and neither declares it; no shipped template
declares it outside `config/workflows/office-default.yml:53`. So this is a **latent trap, not an
active regression**: the first operator to configure `on_agent_error` on a kanban board gets a clean
bill of health for a step that will do nothing. The ACs are therefore written against the mechanism,
not against a board someone has to edit first.

## Prior art

**Leg 1 (wiki) and Leg 2 (`saas-kb`) DID NOT RUN** — tooling-unavailable, not empty results; receipts
are in the task plan under `## Prior art — receipts`.

**Leg 3 — in-repo prior reasoning (ran, and it outranks fresh design here).** ADR-0004 already
specified this trigger and governs three decisions:

- §7 "fire on the failing task **at its current step**" — adopted literally; see [AC-A4](#a-dispatch).
- The engine is "the universal coordinator for task-scoped agent runs. **No discriminator
  anywhere.**" An Office-only dispatcher is a discriminator, which is why the answer is a second fire
  site rather than a kanban-specific parallel mechanism.
- §7's default action list names `pause_agent`, `queue_run` and `create_inbox_item`. **We depart
  here**: the first and third are not `ActionKind` constants and do not exist in
  `compileGenericActions`. Adding action kinds is out of scope, so the vocabulary is what the
  compiler already accepts.

**Two departures, named because a builder will otherwise trip on them.** ADR-0004 also says
`on_agent_error` fires "only after the runs queue's retry policy exhausts (4 attempts at
[2m, 10m, 30m, 2h])". The shipped Office path does **not** — `event_subscribers.go:654`, "Office
failure path (v1): every agent error is terminal", backed by `AC-OFFICE-RUNTIME-001.1`. This spec
follows shipped Office behaviour, so both paths have the same firing condition.

## Dispatch decision

**Terminal agent-session failure for a non-Office task fires `TriggerOnAgentError` from
`orchestrator.Service.handleRecoverableFailureLocked`, gated on a fail-closed Office-ownership check
and on the failure not being user-initiated.**

Why that function and not `handleAgentFailedLocked`, the more obvious reading of "the orchestrator's
agent-failure path": it is **the single terminal funnel**. All six routes reaching a terminal
session-backed kanban failure converge here ([Scope](#scope)), so a site placed after the
`handleRecoverableFailureLocked(...)` call inside `handleAgentFailedLocked` would miss five. It is
also where the card said to look — "alongside where it already reconciles task state for runtime
failures": it completes the turn, persists the last agent error, sets session state, writes task
REVIEW, and reconciles a pending step-completion signal.

**A retry in flight never reaches it, but a retry that has given up does.** The first delivery of a
transient provider error returns at `handleTransientFailure`. The retry loop's own **terminal exits**
reach it through `handleRecoverableFailure`; two of those three are genuine failures and the third is
a user pressing Cancel. See [AC-A8](#a-dispatch), [AC-A9](#a-dispatch), [AC-A10](#a-dispatch).

Why this cannot become a third path overlapping Office's two. Path A (`dispatchAgentErrorTrigger`)
requires a claimed Office `*models.Run`; Path B (`HandleRunFailure` → `escalateFailure`) queues its
own CEO run and never reaches Path A. Both are Office-run-scoped by construction. But the Office
subscriber and the orchestrator watcher both subscribe to `events.AgentFailed`
(`event_subscribers.go:264`, `watcher/watcher.go:330`), so **both handlers run on every failure** and
without an explicit gate the new site fires alongside Path A on every Office failure. The two
operation-id namespaces are disjoint by construction, so a shared key can never absorb that overlap:
the gate is the only thing preventing it, which is why [AC-B1](#b-office-isolation) fails closed.

## What

### Scope

A **terminal agent-session failure** is a failure that reaches `handleRecoverableFailureLocked`
with a non-empty session id. Exactly six production routes do so, and the dispatch must be
correct on all six:

| # | Route | Site | Dispatches? |
|---|---|---|---|
| R1 | bus-driven terminal failure | `handleAgentFailedLocked` (`event_handlers_agent.go:1411`) | yes |
| R2 | managed-npm-runtime start failure | `handleAgentStartFailed` (`:2088`) | yes |
| R3 | auth-error start failure | `handleAgentStartFailed` (`:2105`) | yes |
| R4 | transient retry, no cached prompt | `retryTransientPrompt` (`event_handlers_transient.go:269`) | yes |
| R5 | transient retry, prompt failed synchronously | `retryTransientPrompt` (`:317`) | yes |
| R6 | **user cancelled the retry loop** | `CancelTransientRetry` (`:565`) | **no** — AC-A8 |

R4 and R5 are the transient loop giving up: the agent genuinely failed and no retry remains, so they
are terminal and fire. R6 is a human pressing Cancel on the recovery banner, and a recovery trigger
must not move a card out from under the person who just asked the retries to stop. R6 reaches the
handler through the same `handleRecoverableFailure` wrapper as R4/R5 and is otherwise
indistinguishable from them — which is why AC-A8 requires an explicit marker, not inference.

A transient retry still in flight, a dynamic-route fallback, a dropped rotated execution, a dropped
terminal-session event, and a cancellation-in-flight deferral all return before this function and are
**not** terminal failures.

A **non-Office task** is one for which `models.Task.IsFromOffice` is false — the
`IsFromOfficePredicate` projection (`internal/task/repository/sqlite/task.go:138`), computed at read
time as `project_id != '' OR workflow_id == workspace.office_workflow_id`, the same predicate
`writeTaskReviewState` and `isOfficeTask` use. No second notion of Office ownership is introduced.

**Session-backed only.** `Engine.handleTrigger` errors unless both `TaskID` and `SessionID` are
non-empty (`engine.go:249`), and `handleRecoverableFailureLocked` is also reachable with an empty
session id (the wrapper's `data.SessionID == ""` branch). That call cannot dispatch — named
exclusion, see [AC-A5](#a-dispatch).

### Execution model

**Position: last.** The dispatch is the final action of `handleRecoverableFailureLocked` — after the
session-state write, after `writeTaskReviewState`, after the pending step-completion reconciliation,
and **after the `go s.cleanupAgentExecution(...)` launch that currently ends the function**. That
goroutine runs concurrently, touches only agent-execution teardown, and the dispatch must neither
wait on it nor assume it finished. Two reasons, and the second decides it. First, it cannot mask or
race the failure bookkeeping already committed — Office's `dispatchAgentErrorTrigger` states the same
rationale for its own position. Second, **it picks the right step**: a `step_complete_kandev` signal
that landed mid-turn is reconciled by `reconcileStepCompletionSignalLocked` a few lines earlier, and
that reconciliation can transition the task. Firing before consults the step the agent failed on;
firing after consults the step the task now occupies. The latter is correct — if the signal
reconciled, the agent's work at the old step was accepted, and moving the card again from the old
step's recovery config would fight the transition that just committed. ADR-0004 §7's "at its current
step" is read as *current at dispatch time*.

**How the engine sees that step.** The dispatch follows `processOnTurnCompleteViaEngine`
(`event_handlers_workflow.go:4721-4751`): `EvaluateOnly: true` with an explicit `PreloadedState`.
`PreloadedState` **replaces** `LoadState` — `loadExecutionContext` (`engine.go:333`) uses one or the
other, never both — so AC-A4's freshness comes from this handler, not the engine. State is built as
the dispatch's first act, *after* the reconciliation: reload the session and then the task, in that
order and with the short-circuit [Guard evaluation order](#guard-evaluation-order) fixes, then
`buildMachineState(ctx, task, session)`. `LoadStep` still reads the step through the store, keyed on
the reloaded task's `WorkflowStepID` — `assembleMachineState` copies that field into
`MachineState.CurrentStepID`; `models.Task` has no field of that name.

**That reload is ONE read of each, reused, and it is keyed on the EVENT.** The task is read by
`data.TaskID` and the session by `data.SessionID` — the same two fields the surrounding handler
already uses for `persistLastAgentError`, `writeTaskReviewState` and the reconciliation. The dispatch
shall **not** re-derive the task id from the loaded session's `TaskID`, and shall not compare the
two. Using exactly one source makes a mismatched pair unrepresentable inside the dispatch, which
matters because the two are consumed differently downstream — callbacks act on `in.State.TaskID`
while transitions use `HandleInput.TaskID`, so a pair drawn from two sources could start work on one
task while moving another. A row whose `task_id` disagrees with the event is a repository-integrity
failure the whole handler already assumes away; see [Out of scope](#out-of-scope).

The object each read produces is the *same* one the AC-B1 ownership check, the AC-F2 / AC-F3 / AC-F4
shape guards and `buildMachineState` consult; the dispatch shall not re-read either mid-flight,
because two reads of one row can observe two versions and let the ownership gate and the engine's
state disagree about the same task.
[Guard evaluation order](#guard-evaluation-order) places that read; [AC-A7](#a-dispatch) /
[AC-F6](#f-guards-nil-empty-and-boundaries) give its failure behaviour.

**Idempotency.** The dispatch passes an `OperationID` to `Engine.HandleTrigger`, which short-circuits
to `HandleResult{Idempotent: true}` when `TransitionStore.IsOperationApplied` reports the key applied
(`engine.go:252`). The key is derived from the failure's identity, not from a run:

```
agent_error:session:<sessionID>:<agentExecutionID>
agent_error:session:<sessionID>                      // when AgentExecutionID is empty
```

Both components are needed: a session can host several executions in sequence (rotation, resume,
dynamic re-route) and each can fail, so a session-only key would suppress every failure after the
first for the life of the process.

**Why the fallback exists.** Not because the synthetic events omit the execution id — all four
`watcher.AgentEventData` sites set it. It exists because the field can still arrive **empty**:
`CancelTransientRetry` discards `GetExecutionIDForSession`'s error, `retryTransientPrompt` guards
teardown with `if execID != ""`, and a start failure can be raised before an execution row exists.
Rare but reachable; the collapsed key's cost is [AC-C8](#c-idempotency)'s.

**When the marker is written, and what that costs.** `handleTrigger` calls `markOperationApplied`
**only after `processActions` returns without error**, and `applyEngineTransition` runs **after**
`HandleTrigger` has returned and marked. So an action list that errors part-way leaves the operation
**unmarked** and a redelivery re-runs it from the start ([AC-C6](#c-idempotency)), while a transition
the engine resolved but `applyEngineTransition` then declines leaves it **marked** and unretried
([AC-C7](#c-idempotency)).

**The durability limit is real.** `workflowStore.IsOperationApplied` / `MarkOperationApplied`
(`workflow_store.go:886`) are backed by an in-memory `sync.Map` (`appliedOps`), no persistence, no
eviction — *exactly-once within one backend process lifetime*, not across a restart. Acceptable
because `agent.failed` is a live bus event with no replay: a restart loses the event rather than
redelivering it. A builder must not read "idempotent" as "durable".

**Transitions go through the orchestrator, not the raw engine.** `Engine.applyTransition` delegates
to `TransitionStore.ApplyTransition`, which moves the task but **does not dispatch the destination
step's `on_enter`**: `workflowStore.applyTransition` only builds a `stepentry.PendingAllocation` when
ctx is wrapped with a `stepentry.ResultHolder`, which only `applyEngineTransition` does
(`workflow_store.go:312-325`). A bare `HandleTrigger` would move a card into a step whose
`on_enter: auto_start_agent` never runs — this card's defect, one trigger over. The dispatch calls
`applyEngineTransition(..., triggerOnEnter: true)`, which validates the target step, runs the
credential preflight, and fires `on_enter`.

### Guard evaluation order

The guards in group F, plus AC-A5, AC-A8, AC-B1 and AC-B2, are a **first-match-wins sequence**, not
an unordered set. Each guard is either **silent** or **recording**, and that split is what makes the
order enforceable at all:

- **Silent:** AC-F1, AC-A5, AC-B1, AC-F2, AC-F3, AC-F4.
- **Recording:** AC-A8, AC-A7 and AC-F5 at DEBUG; AC-B2 and AC-F6 at WARNING — AC-F6 except its
  `otherWorkingSessionID` sub-case, which records nothing of its own.

From the event alone, before any repository read:

1. **AC-F1** engine snapshot absent or its engine nil → skip.
2. **AC-A5** no session id → skip.
3. **AC-A8** user-initiated (route R6) → skip, DEBUG.

Then the single task + session read described above. It is **session first, then task**, and it
short-circuits: the task is not read when the session step already skipped. That order is
contractual — it is what decides the record when both reads would fail — and it puts the commonest
vanish case (AC-A7) before any task read. **A `nil` row returned with a `nil` error is the
not-found case, not a success**, for both reads: the orchestrator treats `err != nil || row == nil`
as one condition at a dozen sites, and `assembleMachineState` dereferences `task.WorkflowStepID`
while `buildMachineState` dereferences `session.ID`, so admitting a nil row here is a panic in the
terminal-failure handler. The full table, first match wins:

4. **Session**, read by `data.SessionID`: `errors.Is(err, models.ErrTaskSessionNotFound)`, or a nil
   session with a nil error → **AC-A7**, DEBUG no-op. Any other error → **AC-F6**, WARNING.
5. **Task**, read by `data.TaskID`, only if step 4 did not skip: any error, or a nil task with a nil
   error → **AC-B2**, WARNING. AC-B2 owns every task-read failure; AC-F6 owns none of them.

Then, against that one loaded pair:

6. **AC-B1** Office-owned → skip. It precedes the shape guards deliberately: Office ownership
   decides whether this mechanism acts at all.
7. **AC-F2** ephemeral, **AC-F4** archived, **AC-F3** empty `WorkflowStepID` → skip.
8. **AC-F5** another session on the task is working → skip, DEBUG.

Then dispatch. A failure satisfying two guards at once takes the **earlier** one.

**What is contractual here, and what deliberately is not.** The order between two co-occurring
guards is contractual only where it is **observable** — where at least one of them records. Where
both are silent the relative order is **explicitly not enforced**: step 7's three guards may be
evaluated in any order or folded into one condition, and the same goes for AC-F1 against AC-A5 and
for AC-B1 against step 7. Nothing distinguishes those outcomes — same skip, no dispatch, no record —
so a criterion demanding one would be untestable. Every *recording* guard's position **is**
contractual, because a wrong order shows up as the wrong record, or as one that should not have been
emitted at all.

### Action vocabulary

**The set of actions a kanban step may declare under `on_agent_error` is exactly the set
`engine.compileGenericActions` already compiles.** No new allow-list, no new action kinds, no
kanban-specific filter at the dispatch site. That set is seven kinds (`engine/types.go:437`):

| Kind | Meaningful on a kanban task? | Executor |
|---|---|---|
| `move_to_next` / `move_to_previous` / `move_to_step` | yes | engine transition (structural — **no callback**) |
| `auto_start_agent` | yes, and it is a restart loop — see [Failure modes](#failure-modes) | `autoStartAgentCallback` |
| `queue_run` | yes for `target: primary`; needs an Office workspace CEO for `workspace.ceo_agent` | `QueueRunCallback` |
| `clear_decisions` | degenerate — a kanban step has no decisions | `ClearDecisionsCallback` |
| `queue_run_for_each_participant` | degenerate — a kanban step has no participants | `QueueRunForEachParticipantCallback` |

The three `move_to_*` kinds have **no registered callback** (`workflow_callbacks.go:32-60`); they are
handled structurally inside `evaluateActions`. That is load-bearing for AC-E4 and AC-E7, which is why
both carry an explicit transition carve-out.

Two facts a builder will otherwise assume wrongly. The embedded-YAML allow-list `validGenericAction`
(`config/workflows/loader.go:200`) accepts only the last three, all Office-shaped, so the four
kanban-meaningful kinds are **not** declarable through a shipped template. And that allow-list is the
*only* validation of generic action types anywhere — import, git-sync and step-write validate none —
so all seven are already reachable. This spec does not widen it; it makes the compiled set execute.

**A recognised-and-skipped no-op is what hid this defect, so it does not stay silent.**
`Engine.executeCallback` returns `nil` for a kind with no registered callback (`engine.go:391-394`),
the exact shape of the failure this card is about. This spec adopts
`workflow-on-enter-action-dispatch`'s AC-A6 answer: an action that reaches no executor **warns**.

**Where that warning lives is a decision, not a detail.** It is emitted at the orchestrator dispatch
call site, not inside `executeCallback`, which `evaluateActions` calls identically for all eleven
triggers: a warning there would change behaviour for every trigger the moment this ships, and would
collide with the sibling `on_enter` spec, which needs the same warning at the same line for its own
AC-A6. The call site instead walks the compiled `on_agent_error` actions against the callback
registry before invoking the engine. That registry is a local in `Service.initWorkflowEngine`
(`service.go:1797`) that neither `Service` nor `Engine` exposes, so this card retains it — an
additive field, no engine-package change.

**Retaining it is not snapshotting it.** `buildWorkflowCallbacks(svc)` rebuilds the registry on
**every** `initWorkflowEngine` call and registers `queue_run`, `queue_run_for_each_participant` and
`clear_decisions` only when `svc.engineRunQueue` / `svc.engineParticipants` / `svc.engineDecisions`
are non-nil — and the eleven `reinitWorkflowEngine()` sites re-enter it as the `Set*` methods wire
those in after construction. A value captured at construction reports those three kinds unregistered
**forever**, so AC-E4 would warn on every dispatch of an action the engine executes correctly: a
false alarm shipped by the criterion written to prevent one. Nor are two fields one atomic
replacement. `initWorkflowEngine` rebuilds **three** collaborators together — the store
(`service.go:1798`), the registry, and the engine built from both (`:1805`) — and the dispatch reads
all three: the engine to call, the registry to walk, and the store to read the actions being walked.
Held separately, each reinit is three unsynchronized writes and each dispatch three unsynchronized
reads, so one racing a `Set*` call can pair a new engine with an old registry, or walk one version of
a step while the engine executes another. One value carrying all three, published once and read once,
closes it by construction. See [AC-E12 and AC-E13](#e-action-vocabulary-and-transitions).

### Acceptance criteria

#### A. Dispatch

- **AC-A1** WHEN an agent session on a non-Office task reaches a terminal agent failure, the
  system shall dispatch `engine.TriggerOnAgentError` for that task against the session that failed.
- **AC-A2** The dispatch shall occur on every route in the [Scope](#scope) table marked "yes" — R1
  through R5. A dispatch covering only the bus-driven route R1, or only R1–R3, does not satisfy this
  criterion.
- **AC-A3** The dispatch shall be the last action of terminal-failure handling: after the session
  state write, REVIEW and signal reconcile, and the `go s.cleanupAgentExecution(...)` launch.
  Release `cancelInFlight` before dispatch; callbacks can reacquire. Do not wait on cleanup.
- **AC-A4** The step whose `on_agent_error` actions are evaluated shall be the task's current step
  **at dispatch time**, read from a task reloaded after the step-completion reconciliation and
  supplied to the engine as `PreloadedState`. WHEN a pending step-completion signal reconciled into a
  transition earlier in the same handler, the destination step's configuration is consulted, not the
  step the agent failed on.
- **AC-A5** IF the failure carries no session id, THEN the system shall not dispatch and shall not
  warn — a sessionless failure is an ordinary outcome, not an anomaly.
- **AC-A6** A trigger is **dispatched** when the dispatch called `Engine.HandleTrigger` and the call
  returned no error and `HandleResult.Idempotent` was false. A call the engine short-circuited as
  idempotent is **not** a dispatch for the purpose of this criterion, whatever
  [AC-C3](#c-idempotency)'s prose calls it. WHEN a trigger is dispatched **and**
  `HandleResult.ActionCount > 0`, the system shall emit exactly one INFO record naming task id,
  session id, step id, and the operation id. `ActionCount` is `len(actions)` from the engine's own
  result, so this is decided after the call, not guessed before it — the same field Office's
  dispatcher already reads as its "something happened" predicate. **At most one** record is emitted
  per invocation that reaches the engine — exactly one in three of the four cases and none in the
  fourth — and the four cases are total and disjoint: dispatched with
  actions → this INFO; dispatched with none → [AC-E2](#e-action-vocabulary-and-transitions)'s DEBUG;
  short-circuited as idempotent → **nothing** ([AC-C3](#c-idempotency), [AC-C8](#c-idempotency));
  engine error → [AC-E5](#e-action-vocabulary-and-transitions)'s ERROR. "At most one" is the
  invariant and the enumeration is authoritative: a builder shall **not** read this criterion as
  requiring a record on the idempotent path, which AC-C3 and AC-C8 both forbid.

  **"Record" here, and everywhere this spec counts records, means a *dispatch record*: one of the
  four outcomes enumerated above, or a recording guard's line; this criterion is where that set is
  defined, and [AC-F9](#f-guards-nil-empty-and-boundaries) states the identity requirement it places
  on the records it names.
  [AC-E4](#e-action-vocabulary-and-transitions)'s walk WARNING is deliberately **not** a dispatch
  record and does not count against "at most one".** It reports how the step is *configured*, is
  decided by the pre-engine walk rather than by the call's outcome, and is therefore orthogonal to
  the four cases: a step declaring an action with no registered callback emits it **in addition to**
  whichever case applies — including the idempotent case, where the dispatch-record count is still
  zero. Without this carve-out the criterion would be unsatisfiable for such a step, since AC-E4's
  WARNING plus this INFO is two, and AC-C3's and AC-C8's "no dispatch record" would be false whenever the
  redelivered step happens to declare an unregistered action. This is the same exemption shape
  [AC-F8](#f-guards-nil-empty-and-boundaries) already applies to its two non-dispatch log classes.
- **AC-A7** IF the failed session row no longer exists when the dispatch runs — deleted between the
  failure and this point — THEN the system shall treat it as a no-op logged at DEBUG, not as an
  error. Detection shall use `errors.Is(err, models.ErrTaskSessionNotFound)`, which the repository
  wraps with `%w`. This criterion **takes precedence over AC-F6** for the session reload that builds
  `PreloadedState`: a reload failing with `ErrTaskSessionNotFound` is this DEBUG no-op; a reload
  failing for any other reason is AC-F6's WARNING. A **missing current step** is deliberately not
  covered here — it surfaces as an unwrapped error with no sentinel and takes AC-E5's ERROR path. See
  [Out of scope](#out-of-scope).
- **AC-A8** IF the failure is user-initiated — route R6, the user cancelling the transient retry
  loop — THEN the system shall not dispatch, and shall log at DEBUG. The signal shall be an
  explicit field on `watcher.AgentEventData` set only at `CancelTransientRetry`'s call site,
  defaulting to false so every other route is unaffected. It shall not be inferred from the error
  message, the failure code, or a missing execution id.
- **AC-A9** WHEN the transient retry loop reaches a terminal exit that is not user-initiated —
  routes R4 and R5 — the system shall dispatch. Exhausting or failing to start automatic retries is
  an agent failure, and suppressing recovery there would defeat the trigger for exactly the failures
  it exists to catch.

- **AC-A10** AC-A8's suppression is scoped to the R6 delivery itself and shall not leak to a
  concurrent retry timer's delivery. WHEN a `runTransientRetry` timer has already claimed its retry
  entry (`transientRetries.Load` returned the live entry **and** `entry.claim()` succeeded) and that
  timer **does** reach R4 or R5, THEN it shall dispatch, with the AC-A8 marker at its default
  `false` — the cancel's marker rides on the cancel's own event and never on the timer's.
  **Whether the timer reaches R4 or R5 at all is deliberately NOT this criterion's, and is not
  guaranteed:** the cancel also cancels the `retryCtx` that timer is running on, and
  `retryTransientPrompt` can return at one of its `ctx.Err()` checks first — the mechanism is in
  [Concurrency](#concurrency-and-ordering). So the pair yields **one when the timer gets through,
  zero when the cancel outruns it**; both are correct and [AC-B5](#b-office-isolation) names the
  zero. A test shall pin the non-leak — a timer that reaches R4/R5 dispatches despite a concurrent
  cancel — and shall assert **at most one**, never exactly one. **Accepted residual**, and it shall
  **not** be closed by detaching the post-claim continuation from `retryCtx`: that re-drives the
  prompt after the user pressed Cancel ([Out of scope](#out-of-scope)).

#### B. Office isolation

- **AC-B1** IF the failing session's task is Office-owned by the authoritative projection
  (`models.Task.IsFromOffice`), THEN the system shall not dispatch. Office's Path A remains the only
  fire site for those tasks.
- **AC-B2** IF the task cannot be loaded — the read errored, **or** returned a nil task with a nil
  error — THEN the system shall not dispatch and shall log at WARNING. This is the sole record for
  every task-read failure; AC-F6 emits none. The check fails **closed**: an unavailable ownership answer must never be read as "not
  Office", because that is the state that double-fires with Path A. This mirrors
  `writeTaskReviewState`'s fail-closed task load, and deliberately does not reuse `isOfficeSession`,
  which returns `false` on a lookup error (fails open).
- **AC-B3** Office behaviour shall be unchanged. `dispatchAgentErrorTrigger`, its
  `agent_error:<run.ID>` key, `HandleRunFailure` and `escalateFailure` keep their current semantics,
  and the Office dispatcher's `resolveSession` / `resolveSessionByID` branches for
  `TriggerOnAgentError` shall be untouched.
- **AC-B4** For one agent failure, **at most** one `TriggerOnAgentError` shall be dispatched across
  both mechanisms. "At most" is exact; the ways the count is legitimately zero are enumerated in
  [AC-B5](#b-office-isolation), which is the sole register of them — this criterion deliberately
  carries **no count of its own**, because a number here goes stale the moment AC-B5 gains an entry
  and has already done so once. The
  one case in which two may fire is the ownership-flip race in
  [Failure modes](#failure-modes), an accepted residual; a test for this criterion shall assert "not
  more than one", never "exactly one" unconditionally.
- **AC-B5** The dispatch count for one failure shall be zero, not one, in each of these cases, none
  of them a defect: an Office-owned task whose Office run is not claimed (Path A's
  `resolveLifecycleRun` returns `sql.ErrNoRows` and AC-B1 blocks the orchestrator, so neither fires);
  any AC-A5, AC-A8 or AC-B2 skip; any AC-F1 through AC-F6 guard skip; and a user cancel that
  cancels `retryCtx` inside an already-claimed timer's window, where the timer returns at one of
  `retryTransientPrompt`'s `ctx.Err()` checks before reaching R4 or R5 and the cancel's own R6
  delivery is AC-A8-suppressed, so neither delivery dispatches ([AC-A10](#a-dispatch)).

#### C. Idempotency

- **AC-C1** The dispatch shall pass `OperationID` = `agent_error:session:<sessionID>:<agentExecutionID>`.
- **AC-C2** IF `AgentExecutionID` is empty, THEN the `OperationID` shall be
  `agent_error:session:<sessionID>`.
- **AC-C3** WHEN the same failure is handled twice within one backend process lifetime **and the
  first handling completed action evaluation without error**, the second delivery shall execute no
  action and the engine shall report `HandleResult{Idempotent: true}`. That delivery is not a
  "dispatched trigger" under [AC-A6](#a-dispatch) and emits no **dispatch record**.
  [AC-E4](#e-action-vocabulary-and-transitions)'s walk WARNING is not a dispatch record
  ([AC-A6](#a-dispatch)) and this criterion does not suppress it: the walk runs before the engine
  call and cannot know the operation is already applied.
- **AC-C4** WHEN two distinct agent executions on one session fail in sequence, both shall dispatch.
  Suppressing the second is a defect, not idempotency.
- **AC-C5** The exactly-once guarantee is scoped to one backend process lifetime, because
  `workflowStore.appliedOps` is an in-memory `sync.Map`. This is a stated limit of the contract, not
  a gap to be closed by adding durable marker storage in this card.
- **AC-C6** IF an action in the list returns an error, THEN the operation shall be left **unmarked**
  (the engine's existing behaviour: `markOperationApplied` runs only after `processActions` succeeds)
  and a subsequent delivery of the same failure shall re-run the list from the start, including
  actions that already succeeded. Actions under this trigger are not assumed idempotent and this card
  does not make them so.
- **AC-C7** IF the engine resolved a transition and marked the operation, but `applyEngineTransition`
  then declines it — target step missing, or credential preflight failure — THEN the transition
  shall not be retried. It is **not** an AC-E5 case: the engine returned success. The outcome is
  already recorded by `applyEngineTransition`'s own logs (WARNING for a missing target step, ERROR
  for a failed commit)
  and the dispatch shall not add a duplicate record. The marker precedes the transition
  commit and this card does not reorder that.

- **AC-C8** IF two failures on the same session both carry an **empty** `AgentExecutionID`, THEN
  they collapse to the one AC-C2 key and only the first shall dispatch; the second is reported
  `Idempotent: true` and emits no **dispatch record**, carrying the same AC-E4 carve-out AC-C3
  states. That suppression holds only where the first handling
  completed action evaluation without error: IF the first left the operation **unmarked** because an
  action errored, THEN the second re-runs the list as AC-C6 requires and this criterion does not
  suppress it. AC-C4 is therefore satisfied by *distinguishable* executions — both ids non-empty and
  different — since an empty id is not distinguishable from a prior empty one. The cost, a genuinely
  second empty-id failure on one session being skipped, is an accepted residual bounded by AC-C5's
  process lifetime; a fresh key per delivery would defeat the operation id for every redelivery to
  rescue a rarer case. It shall **not** be closed by adding a counter, a timestamp or any other
  discriminator to the key.

#### D. Payload

- **AC-D1** `OnAgentErrorPayload.FailedSessionID` shall be the id of the session that failed, never
  empty on a dispatched trigger (AC-A5 excludes the only case where it could be).
- **AC-D2** `OnAgentErrorPayload.FailedAgentID` shall be the failure event's `AgentProfileID`; IF
  that is empty, THEN the failed session's `AgentProfileID`; IF both are empty, THEN the empty
  string. It shall not be populated with an Office run id, an agent execution id, or an agent type.
  The synthetic events built by `handleAgentStartFailed` (R2, R3) and by the transient-loop exits
  (R4, R5) carry no `AgentProfileID`, so the session fallback is the normal path for four of the five
  dispatching routes, not an edge case.
- **AC-D3** `OnAgentErrorPayload.ErrorMessage` shall be the failure event's `ErrorMessage`; IF that
  is empty, THEN the literal `agent failed`, matching the default `persistLastAgentError` writes for
  the same failure.

#### E. Action vocabulary and transitions

- **AC-E1** The dispatch shall not filter by action kind. Every action `compileGenericActions`
  produces for `on_agent_error` is handed to the engine, and the engine's per-kind semantics apply
  unchanged.
- **AC-E2** WHEN the current step declares no `on_agent_error` actions — the trigger was dispatched
  and `HandleResult.ActionCount == 0` — the dispatch shall be a successful no-op logged at DEBUG, and
  shall mark the operation applied (the engine already does so on its empty-actions path). This is
  the overwhelmingly common case: it must not warn, and per AC-A6 it emits this DEBUG **instead of**,
  never in addition to, the AC-A6 INFO.
- **AC-E3** WHEN an `on_agent_error` action resolves a transition target, the system shall apply that
  transition through `applyEngineTransition` with `triggerOnEnter: true` — the same call an
  `on_turn_complete` transition receives, including target-step validation and credential preflight.
  The engine's bare `ApplyTransition`, which skips the destination's `on_enter`, does not satisfy
  this criterion. The one case where `on_enter` legitimately does not run despite a committed
  transition is AC-E10's WIP-queued destination.

  **The `taskDescription` argument shall be the `Description` of the task already reloaded for
  `PreloadedState`** ([Execution model](#execution-model)) — not the empty string, and not a third
  read of the task. That argument is not inert: `applyEngineTransitionWithCommit` hands it to
  `processOnEnter`, which passes it to `buildWorkflowPrompt`, so it becomes the prompt given to an
  agent the destination step auto-starts — the path [Failure modes](#failure-modes) already names.
  The repository supplies this argument two different ways at its three existing call sites (the
  task's description at the `on_turn_complete` and `on_children_completed` sites, the empty string at
  the `on_turn_start` site), so it is named here rather than left to precedent. A recovery move
  carries the task's description for the same reason `on_turn_complete` does: the relaunched agent
  needs to know what the task is. **Substituting** the empty string does not satisfy this criterion;
  a task whose own `Description` is empty passes that empty value through unchanged, which is the
  field's value and not a substitution. No default, no placeholder, and no second read of the task.
- **AC-E4** IF a declared `on_agent_error` action reaches no executor, THEN the system shall log at
  WARNING naming workflow id, step id, step name, action type, task id and session id, and the
  remaining actions shall be unaffected. "Reaches no executor" means **no callback is registered for
  its kind in the running configuration**. The three `move_to_*` kinds are explicitly **excluded**:
  they have no callback by design and execute structurally, so warning on them would be a false
  positive — including for the second and subsequent transition actions AC-E7 sends down the callback
  path. The warning is emitted at the orchestrator dispatch call site by walking the step's compiled
  `on_agent_error` actions against the callback registry, before the engine call; it shall not be
  added inside `engine.executeCallback`, which is shared by all eleven triggers. This WARNING is
  **not** a dispatch record ([AC-A6](#a-dispatch)): it does not count against AC-A6's or AC-F8's
  "at most one", and because the walk precedes the engine call it is emitted even when the engine
  then short-circuits the operation as idempotent ([AC-C3](#c-idempotency)).
- **AC-E5** IF the engine returns an error, THEN the system shall log at ERROR and not propagate it.
  Failure bookkeeping already committed by the surrounding handler must not be masked by a
  recovery-dispatch failure — the contract `dispatchAgentErrorTrigger` states for Office. This is
  also the path a missing current step takes (AC-A7).
- **AC-E6** `validGenericAction` in `config/workflows/loader.go` shall be unchanged and no shipped
  workflow template shall gain an `on_agent_error` declaration. This card makes the trigger work; it
  configures no board.
- **AC-E7** WHEN a step declares the same **non-transition** action type more than once under
  `on_agent_error`, each declaration shall execute, in declared order. There is no deduplication and
  none shall be added. Transition actions are **excluded**: `evaluateActions` gates
  transition handling on `targetStepID == ""`, so only the first eligible transition resolves a
  target and any later transition action is not executed as a transition. This is
  first-eligible-transition-wins, the engine's existing behaviour for every trigger, unchanged here.
- **AC-E8** IF a transition action resolves to the step the task already occupies, THEN no transition
  shall be applied and no `on_enter` dispatched. The trigger is still marked applied under AC-C1.
- **AC-E9** Transition guards and `requires_approval` shall behave exactly as they do under every
  other trigger, and the dispatch shall add no special case. A transition action whose guard is not
  satisfied is skipped, recorded through the engine's existing `recordGuardNotFired`, and resolves no
  target; an action carrying `requires_approval` is not treated as a transition at all
  (`evaluateActions` tests `!action.RequiresApproval`) and takes the callback path, where AC-E4's
  transition carve-out keeps it silent. Neither case shall produce an AC-E4 warning.
- **AC-E10** IF a transition commits but the destination step is WIP-limited and the task is queued
  rather than admitted (`QueuedForStepID == ToStepID && !WIPAdmitted`), THEN `on_enter` is
  **deferred to the queue-promotion event**, not dispatched by this handler, and that is not an
  AC-E3 failure. `applyEngineTransitionWithCommit` already takes this branch and returns true. An
  AC-E3 test shall therefore use a destination step with no WIP limit, or assert the deferral.
- **AC-E11** A transition committed under this trigger shall be recorded in the ADR-0015 audit
  ledger — the **`session_step_history`** table `recordAutoStepTransition` writes — under **its own**
  trigger label. `applyEngineTransitionWithCommit`
  picks that label with a switch casing only `TriggerOnTurnStart` and `TriggerOnChildrenCompleted`,
  everything else falling through to `wfmodels.StepTransitionTriggerAutoComplete` — exhaustive for
  its three existing callers, and this card is the first to route a fourth trigger through it. The
  system shall add `wfmodels.StepTransitionTriggerAgentError = "on_agent_error"` and the matching
  `case engine.TriggerOnAgentError`, so a recovery move is distinguishable from a routine move in
  `session_step_history.trigger`. Recording a recovery move as `auto_complete` does **not** satisfy
  this criterion. The new value needs no migration (that column is `TEXT NOT NULL`, no CHECK) and no
  DTO or frontend change; `queue_promotion` and `plugin_move` were added to this enum the same way.

  **The ledger named here is exact, and the other one is deliberately excluded.** There are two
  step-transition ledgers with two separate enums, and the switch this criterion changes reaches only
  the first:
  - `session_step_history.trigger` ← `wfmodels.StepTransitionTrigger`, written by
    `recordAutoStepTransition` → `workflow/service.CreateStepTransition`. **This criterion's target.**
  - `task_step_transitions.trigger`, and the `telemetry_step_transitions_inserted_total` expvar
    counter keyed off it, ← the separate, explicitly **closed** `steptelemetry.Trigger` enum,
    written from `steptelemetry.FromContext(ctx).Trigger` and set by `engineTransitionAttribution`,
    which hardcodes `TriggerEngineTransition` and, by an outermost-caller-wins rule
    (`steptelemetry.HasTrigger`), will not overwrite an attribution a caller already placed on the
    context.

  A `wfmodels` addition therefore does **not** reach the task ledger or that counter, and this
  criterion does not claim it does: both continue to read `engine_transition` for an
  `on_agent_error` move, exactly as they do for every other engine transition. Distinguishing the
  recovery move *there* is [out of scope](#out-of-scope). A test for this criterion shall assert the
  `session_step_history` row and shall **not** assert against `task_step_transitions.trigger`, whose
  value under this trigger is unchanged by this card.

  This criterion constrains the **label only**: `recordAutoStepTransition`'s pre-existing
  best-effort semantics (nil recorder, swallowed write error, async enqueue in production) are shared
  by every trigger and are not changed here. The pending-signal clear stays scoped to
  `TriggerOnTurnComplete`.

- **AC-E12** The walk of AC-E4 shall read the action list as `StepSpec.Events[TriggerOnAgentError]`
  from `TransitionStore.LoadStep(ctx, task.WorkflowID, task.WorkflowStepID)` — the **same store the
  engine will use for its own `LoadStep`**. Re-reading the step row and calling `engine.CompileStep`
  at the call site does **not** satisfy this: it bypasses the store's `stepCache` and can compile a
  newer row than the engine executes, which is precisely the false alarm AC-E4 exists to prevent.
  **The guarantee this buys is bounded, and the bound is contractual.** The walk and the engine make
  **two** `LoadStep` calls, not one, so they see one compiled version **only absent an interleaved
  cache change on that key** — `stepSpecCache` entries carry a TTL and are dropped by
  `stepCache.invalidate`, which `handleWorkflowStepCacheInvalidation` fires on any step edit. A
  concurrent edit between the two reads may have the walk inspect one version while the engine
  executes another. **Accepted residual:** the cost is one spurious or missed AC-E4 WARNING on a step
  being edited at the instant it fails, the dispatch is unaffected, and pinning one `StepSpec` across
  both would mean handing a pre-compiled step to the engine, which `PreloadedState` does not carry
  and no engine API accepts. A test shall assert the shared-store read path, **not** a version
  equality that only holds when nothing invalidates.
- **AC-E13** The engine, the registry walked by AC-E4 and the store read by AC-E12 shall be retained
  as **one value** — a single struct holding all three — built inside `initWorkflowEngine` where the
  store and `callbacks` are already built, published in a **single atomic write** (an
  `atomic.Pointer` or equivalent), and read **exactly once** per dispatch, that one read supplying
  all three. Separate fields do **not** satisfy this criterion: each of the eleven
  `reinitWorkflowEngine()` calls is then three unsynchronized writes, and a dispatch racing one can
  pair a new engine with an old registry, or walk one version of a step while the engine executes
  another. A value snapshotted at construction does not satisfy it either — it would report
  `queue_run`, `queue_run_for_each_participant` and `clear_decisions` as unregistered even after the
  `Set*` wiring registers them. Synchronizing `s.workflowEngine`'s and `s.workflowStore`'s own
  pre-existing readers is [out of scope](#out-of-scope).
- **AC-E14** IF AC-E12's `LoadStep` fails, THEN the walk shall be skipped, no AC-E4 warning shall be
  emitted, **the walk shall contribute no dispatch record of its own, and the dispatch shall still
  proceed to the engine**. This is not a guard and it does not fail closed: the walk is an advisory diagnostic,
  and a dispatch that cannot be pre-inspected is still a dispatch that must happen.
  **The dispatch's record is then whatever the engine's own outcome dictates, and all three of the
  following are reachable** — `stepSpecCache.getOrLoadStep` propagates a fetch error to its waiters
  and **never caches it**, so the engine's `LoadStep` on the same key is an independent retry, not a
  replay of the walk's failure:
  - IF the engine's `LoadStep` also fails, THEN the outcome is
    [AC-E5](#e-action-vocabulary-and-transitions)'s single ERROR, and it is the only record.
  - IF the engine's `LoadStep` **succeeds** — the walk hit a transient error the retry did not —
    THEN the dispatch proceeds normally and records exactly what
    [AC-A6](#a-dispatch)'s four-case table says for the result it gets: the INFO when actions ran,
    [AC-E2](#e-action-vocabulary-and-transitions)'s DEBUG when there were none. This criterion does
    **not** suppress that record, and a builder shall not read "no warning" as covering the dispatch.
  - IF the operation id was already applied, THEN the engine **never calls `LoadStep` at all** and
    neither branch above is taken: `handleTrigger` tests `isOperationAlreadyApplied` **before**
    `loadExecutionContext`, so it short-circuits to `HandleResult{Idempotent: true}` above the step
    load. The outcome is [AC-C3](#c-idempotency)'s — no action executed, no dispatch record — and this
    criterion neither adds one nor overrides it. A builder shall not treat the two `LoadStep`
    branches as the complete matrix.
  The "no warning" here is scoped to the **walk**, so one failure never becomes two dispatch records
  and AC-F8 is not contradicted; the idempotent case contributes zero dispatch records, which "at
  most one" admits.
  Skipping the dispatch on a failed walk does not satisfy this criterion.
#### F. Guards, nil, empty, and boundaries

Each is a skip with no dispatch; whether it records is fixed by
[Guard evaluation order](#guard-evaluation-order). They mirror guards the orchestrator's other engine
call sites apply, and are enumerated because omitting one produces a nil-dereference or a nonsensical
dispatch.

- **AC-F1** IF the retained value ([AC-E13](#e-action-vocabulary-and-transitions)) is absent or its
  engine nil, THEN skip.
- **AC-F2** IF the task is ephemeral (`task.IsEphemeral`), THEN skip: it has no workflow. Mirrors
  `processOnTurnStartViaEngine`.
- **AC-F3** IF `task.WorkflowStepID` is empty, THEN skip.
- **AC-F4** IF the task is archived, THEN skip. Mirrors `writeTaskReviewState`.
- **AC-F5** IF another session on the same task is in a working state, THEN skip and log at DEBUG
  naming the blocking session. A sibling still doing work must not have its card moved out from
  under it by a peer's failure. The predicate is `otherWorkingSessionID`
  (`event_handlers_streaming.go:2127`), the one `writeTaskReviewState` applies a few lines earlier.
  It is **re-evaluated**, not inherited: a second, later read that may legitimately disagree with the
  first if a sibling started or settled in between. Accepted.
- **AC-F6** IF a repository read the dispatch needs fails — the **session** reload that builds
  `PreloadedState` (see [Execution model](#execution-model)), or `otherWorkingSessionID` returning
  its `ok == false` error result — THEN skip. A read "fails" when it returns an error **or** a nil
  row with a nil error; the two are one condition. Every read on this path fails closed, matching
  AC-B2: acting on a partial view of a task's state is worse than not acting, because the action is
  moving the operator's card. **Which record it emits depends on which read failed.** A failed
  session reload logs the dispatch's own WARNING, **except** one failing with
  `ErrTaskSessionNotFound` or returning a nil session, which is AC-A7's DEBUG no-op. A failed
  `otherWorkingSessionID` logs **nothing of the dispatch's own**: that helper already writes its own
  WARNING (`"failed to list task sessions before REVIEW state reconcile"`) before returning
  `ok == false`, and a second line saying the same is the duplicate AC-F8 forbids. **The task reload
  is deliberately NOT this criterion's**: every task-read failure, nil task included, is
  [AC-B2](#b-office-isolation)'s WARNING and only AC-B2's, so one skip never produces two WARNINGs.
  Each skip stays observable as the absence of a dispatch.
- **AC-F7** These guards shall be evaluated before the `OperationID` is marked applied, so a skip
  caused by a transient condition does not suppress a later valid dispatch of the same failure within
  the process lifetime.
- **AC-F8** The guards shall be evaluated first-match-wins in the sequence given in
  [Guard evaluation order](#guard-evaluation-order), and one skip shall produce **at most one** record
  of the dispatch's own: that of the guard that fired when it is a recording guard, and none at all
  when it is silent. No later guard's record shall be emitted. **The relative order of two
  silent guards is not part of this criterion** — it is unobservable and therefore not contractual;
  only a recording guard's position is. Two classes of log line do not count against the "at most
  one", neither being this card's: those the surrounding handler writes before the dispatch runs
  (`handleRecoverableFailure`'s own session-reload WARNING), and those a shared helper the dispatch
  calls writes for itself (`otherWorkingSessionID`'s list-failure WARNING — see AC-F6).
  [AC-E4](#e-action-vocabulary-and-transitions)'s walk WARNING **is** this card's and is likewise
  not counted, because it is not a dispatch record ([AC-A6](#a-dispatch)); it cannot in any case
  co-occur with a guard skip, since the walk runs at the dispatch, after every guard in
  [Guard evaluation order](#guard-evaluation-order) has passed.
- **AC-F9** Every record the dispatch emits — the AC-A6 INFO, the AC-E2 DEBUG, and each recording
  guard's line — shall carry `task_id`, `session_id` and a message string stable and unique to this
  dispatch within the orchestrator package. Without that identity AC-F8's "no later guard's record"
  is unassertable: the log stream already carries the two classes AC-F8 exempts, and a test cannot
  tell them from the dispatch's own. [AC-E4](#e-action-vocabulary-and-transitions)'s walk WARNING
  is **not** a dispatch record ([AC-A6](#a-dispatch) defines that set) and is outside this
  criterion's enumeration; it carries the six fields AC-E4 requires, which include `task_id` and
  `session_id`, so it stays identifiable in a test without being counted.

## Concurrency and ordering

- **Two failures, same session, in sequence.** `cancelInFlight` serializes bookkeeping, then is
  released before dispatch so callbacks can reacquire it. Distinct executions produce distinct
  operation ids (AC-C1) and both dispatch; a repeat is absorbed by AC-C3.
- **Two failures, two sessions, same task, concurrent.** Not serialized — the guard is per-session.
  AC-F5 resolves the common shape: whichever session settles while the other is still working skips.
  IF both reach a terminal state such that neither observes the other as working, THEN both may
  dispatch, and `ApplyTransition` is unguarded, so **the last transition to commit wins**, with no
  ordering required. Accepted; a compare-and-swap here is out of scope.
- **A user cancel racing an already-claimed retry timer.** `runTransientRetry` claims its entry before
  calling `retryTransientPrompt`, while `CancelTransientRetry` reads `active` and only then clears
  state — but clearing it calls `entry.cancel()` unconditionally, with no `claimed` check, and that
  cancels the `retryCtx` the claimed timer is still running on. `retryTransientPrompt` then has four
  `ctx.Err()` early returns before its R4 / R5 calls. So a won claim does **not** guarantee arrival:
  the timer dispatches if it gets past those checks first and returns silently if the cancel does.
  Both deliveries funnel through `handleRecoverableFailure` and serialize on `cancelInFlight`, so
  this is an ordering outcome, not a data race. The total is therefore **one or zero, never two** —
  [AC-A10](#a-dispatch) fixes which part is guaranteed (the non-leak of AC-A8's marker) and
  [AC-B5](#b-office-isolation) names the zero as legitimate.
- **Ordering against the surrounding handler** is fixed by AC-A3 and is total: session state → REVIEW
  → step-completion reconciliation → `cleanupAgentExecution` launch → dispatch.
- **Ordering of actions within one dispatch** is the stored order of the step's
  `events.on_agent_error` JSON array — `compileGenericActions` preserves it, `evaluateActions`
  iterates it — with first-eligible-transition-wins as tiebreak (AC-E7). Unchanged by this card.

## Failure modes

- **`auto_start_agent` under `on_agent_error` is an unbounded restart loop, and this card does not
  bound it.** `autoStartAgentCallback` calls `LaunchSession` with the failed session's own id, so it
  relaunches the agent that just failed; the repeat failure has a new execution id, mints a new
  operation id (AC-C1) and must dispatch again (AC-C4) — indefinitely, no backoff, no cap, and it is
  the most obvious thing an operator would configure here. **Accepted residual:** the loop is a
  property of that configuration rather than of the dispatch, bounding it needs per-session attempt
  state that does not exist, and AC-A6's one INFO per dispatch makes it visible from the first
  iteration. A `move_to_step` whose destination declares `on_enter: auto_start_agent` produces the
  identical loop one step over, via AC-E3.
- **Ownership flips mid-failure.** A task whose `project_id` is cleared, or whose workspace
  `office_workflow_id` changes, between Office's `resolveLifecycleRun` and this site's task load while
  an Office run is claimed, satisfies both predicates and double-fires — the one case AC-B4 exempts.
  Accepted; the outcome is one duplicate CEO run. The same window has a **second symptom**: the
  handler gates its step-completion reconciliation on the *early* `isOfficeSession` read while AC-B1
  re-reads ownership *later*, so a flip between them skips the reconciliation and the dispatch
  evaluates the **stale failed-at step** rather than AC-A4's destination step. Named so a builder does
  not read AC-A4 as unconditional. A future card making Office ownership a stored column closes both.
- **Current step deleted between the failure and the dispatch.** `LoadStep` surfaces it as an unwrapped
  `fmt.Errorf("step %s not found")` (or `"load step %s: %w"` wrapping the getter's error) with no
  sentinel, so it takes AC-E5's ERROR path — louder than the DEBUG a vanished row ideally gets ([Out of scope](#out-of-scope)).
- **Engine error mid-action-list.** `evaluateActions` aborts on the first action error, so later
  actions do not run and the operation is left unmarked (AC-C6); AC-E5 makes it visible at ERROR.
  Abort-on-first-failure is the engine's contract for every trigger.
- **`queue_run` with `target: workspace.ceo_agent` on a kanban workspace with no CEO.** Resolution
  fails, the engine errors, AC-E5 logs it, nothing else in the list runs. A misconfiguration, loud
  after this card and silent before it.
## Out of scope

Each entry is a named exclusion and part of the contract.

- **Office's Path A and Path B.** No change to `dispatchAgentErrorTrigger`, `HandleRunFailure`,
  `escalateFailure` or the Office dispatcher's session resolution. AC-B3 freezes them.
- **New action kinds, and compile-time validation generally.** ADR-0004 §7's `pause_agent` and
  `create_inbox_item` are not implemented and are not added. `compileGenericActions` drops an
  unrecognised type — and a `move_to_step` whose `step_id` is missing or unreadable — *before* the
  engine sees an action, so AC-E4's warning cannot reach either: an operator's typo'd `step_id` still
  reads as configured and still does nothing. That is this card's defect shape one layer up; the fix
  is compile-time validation shared by all seven Phase 2 triggers. Different site, different card.
  Validating action types on the import / git-sync / step-write paths — none do today, which is how
  an unrecognised type reaches the compiler at all — belongs to that same card.
- **Widening `validGenericAction`.** The embedded-YAML allow-list stays at its three Office-shaped
  kinds; the four kanban-meaningful kinds remain declarable only through import, git-sync and the
  step API. A template-authoring change, not a dispatch change.
- **Distinguishing an `on_agent_error` move in `task_step_transitions` or its expvar counter.**
  [AC-E11](#e-action-vocabulary-and-transitions) labels the `session_step_history` ledger only, and
  names why the other one is unreachable from that switch. Retargeting it means adding a value to the
  closed repo-wide `steptelemetry.Trigger` enum and either branching the shared
  `engineTransitionAttribution` on the engine trigger or wrapping the attribution further out — blast
  radius every transition writer, not just this dispatch. An `on_agent_error` move therefore reads as
  `engine_transition` there, indistinguishable from a routine engine move; a follow-up wanting them
  separable in task-level telemetry must decide the attribution site for **all** engine triggers at
  once.
- **A `models.ErrWorkflowStepNotFound` sentinel.** Distinguishing a vanished step from a genuine
  engine error means a sentinel threaded through `workflow/repository/sqlite.go`,
  `workflow/service` and `orchestrator/workflow_store.go` — shared plumbing every trigger depends on.
  AC-A7 is narrowed to the session-vanished case, which already has `models.ErrTaskSessionNotFound`.
- **Making a user cancel and an already-claimed retry timer agree.** AC-A10 accepts that the outcome
  of that race is decided by which side reaches `retryTransientPrompt`'s `ctx.Err()` checks first: the
  timer dispatches if it gets through, and nothing dispatches if the cancel's `entry.cancel()` lands
  first. Making it deterministic in *either* direction needs per-session cancel state outliving the
  single event delivery AC-A8's field rides on — its own mechanism, with its own lifetime and
  eviction rules. Note the seemingly obvious fix is the wrong one: detaching the post-claim
  continuation from `retryCtx` would make the timer always arrive, but at the cost of re-driving the
  prompt after the user pressed Cancel.
- **Bounding the `auto_start_agent` restart loop.** An accepted residual in
  [Failure modes](#failure-modes); a bound needs per-session or per-step attempt accounting that does
  not exist and would change what AC-C4 means.
- **Reconciling an event whose `task_id` disagrees with its session row.** [Execution
  model](#execution-model) makes this unrepresentable inside the dispatch by reading only `data.TaskID`
  and never `session.TaskID`. Detecting and reporting a genuine disagreement is a repository-integrity
  concern shared by every consumer of `watcher.AgentEventData`, not a dispatch fix.
- **The no-session failure branch.** It cannot dispatch: engine state is keyed on
  `(taskID, sessionID)` and there is no session to key on. Giving it one is not a dispatch fix.
- **Durable operation markers.** AC-C5 states the process-scoped limit; making `appliedOps` durable
  is `workflow-on-enter-action-dispatch`'s step-entry-record territory.
- **Synchronizing `s.workflowEngine`'s existing readers.** AC-E4 makes *this dispatch's* engine and
  registry one atomically-published value, read once. The field's other readers stay as they are: they
  were already unsynchronized before this card, and fixing them is a separate change.
- **A compare-and-swap on `on_agent_error` transitions.** See
  [Concurrency](#concurrency-and-ordering); last-write-wins is accepted.
- **Data-bag semantics.** No kind in this vocabulary produces a `DataPatch` today and
  `EvaluateOnly: true` skips `PersistData`. `applyEngineTransition` *does* persist `result.DataPatch`
  when a transition commits, so the path is empty only because no compilable kind emits one. IF a
  future kind under this trigger produces a `DataPatch`, THEN this spec must be re-opened.
## Verification

Every criterion below is observable without reading logs, except where a log record *is* the
contract (AC-A6, AC-A8, AC-B2, AC-E2, AC-E4, AC-E5, AC-F5, AC-F6, AC-F8, AC-F9).

- **Fire-site invariant.** A test asserting `TriggerOnAgentError`'s production fire sites are exactly
  two — the regression guard for the defect itself, failing if a third partial dispatcher is added or
  either is removed. Copy the **design** of
  `task/repository/sqlite/step_transition_writers_pin_test.go`: a `go/ast` walk keying on a
  *statement shape*, comparing enclosing function identities against a registered list.

  **A fire site is defined syntactically**, because a bare identifier scan also matches the constant
  declaration, the `compileGenericActions` trigger-map entry, a payload doc comment and four
  session-resolution references in `internal/office/engine_dispatcher/dispatcher.go`. A fire site is
  an occurrence in exactly one of two positions: (1) the value of the `Trigger:` key in an
  `engine.HandleInput` composite literal — the orchestrator shape; or (2) a direct call argument to
  `dispatchEngineTrigger` — the Office shape, which passes the trigger positionally. A
  `*ast.BinaryExpr` operand, a `case` clause, a `const` declaration, a map-literal key and a comment
  are **not** fire sites. That predicate, not a directory allowlist, discriminates, so the **scan root
  is the whole backend tree** (`apps/backend`, `_test.go` excluded). Assert the set of enclosing
  function identities, package-qualified as the prior art qualifies them, equals exactly
  `{office/service/Service.dispatchAgentErrorTrigger,
  orchestrator/Service.dispatchKanbanAgentErrorTrigger}`. A builder who names the second identity
  differently shall update the registered set in the same commit.
- **AC-A1/A2/A9** — one test per dispatching route R1–R5, each asserting exactly one dispatch with
  the expected trigger and payload. Five tests, not three.
- **AC-A8** — the `CancelTransientRetry` route produces zero dispatches, asserted on the explicit
  marker, not the cancel message text. **AC-A10** — a claimed retry entry that **does** reach R4/R5
  dispatches while the cancel's own delivery does not, proving AC-A8's marker did not leak to the
  timer's event. Assert **at most one**, never exactly one, and do not assert the timer arrives: a
  cancel that cancels `retryCtx` first legitimately yields zero (AC-B5), so a test demanding one
  would be asserting a race it cannot win. Drive the marker, not the interleaving.
- **AC-A4** — a task with a pending step-completion signal that reconciles into a transition,
  asserting the trigger is evaluated against the destination step. Because `PreloadedState` bypasses
  `LoadState`, this fails if the handler builds state before the reconciliation rather than after.
- **AC-B1/B4/B5** — an Office-owned task's failure produces one dispatch and it is not the
  orchestrator's; an Office-owned task with no claimed run produces zero; a task-load error (AC-B2)
  produces zero.
- **AC-C1/C2/C3/C4** — key shape, empty-execution fallback, replay suppression, two executions on one
  session each firing. **AC-C6** — an action that errors leaves the operation unmarked and a
  redelivery re-runs the list. **AC-C7** — a declined transition is not retried. **AC-C8** — two
  failures on one session both carrying an empty `AgentExecutionID` produce one dispatch and the
  second reports `Idempotent: true`; AC-C4's test must not be written to contradict this.
- **AC-A6/E2** — the four-case record table is total and disjoint: a step with one declared action
  emits the INFO and no DEBUG; a step with none emits the DEBUG and no INFO; the idempotent replay of
  either emits neither; an engine error emits only AC-E5's ERROR. Asserting one case without
  asserting the absence of the other three does not satisfy this. All four are assertions about
  **dispatch records**, so run them on a step whose declared actions are all registered and no
  AC-E4 WARNING is in play. Then add one mixed case — a step declaring an **unregistered** action —
  asserting that the AC-E4 WARNING and the applicable dispatch record are **both** emitted and that
  the pair does not violate "at most one", because only the second is counted. A test that counts
  every log line the dispatch emits, rather than every dispatch record, fails on that case.
- **AC-E3** — a step declaring `move_to_step` under `on_agent_error` moves the task **and** dispatches
  the destination step's `on_enter`, using a destination with no WIP limit; asserting the move alone
  passes with the defect this criterion exists to prevent. Assert the `taskDescription` argument
  separately and observably: give the task a **non-empty, distinctive** description and a destination
  step declaring `on_enter: auto_start_agent`, then assert that description reaches the prompt
  `buildWorkflowPrompt` produces. A test that only asserts the move and the `on_enter` fires passes
  with the empty string this criterion forbids. **AC-E10** — the WIP-queued destination
  defers `on_enter` instead. **AC-E11** — that same move writes a **`session_step_history`** row whose
  `trigger` is `on_agent_error`, not `auto_complete`, and the existing labels are unchanged for their
  own triggers. Do **not** assert against `task_step_transitions.trigger`: that column is fed by the
  separate `steptelemetry.Trigger` enum, still reads `engine_transition` under this trigger, and is
  named [out of scope](#out-of-scope) — a test asserting `on_agent_error` there cannot pass.
- **AC-E4** — a kind with no registered callback produces the WARNING with all six named fields; a
  step declaring **two** `move_to_step` actions produces **no** warning, the false positive the
  transition carve-out exists to prevent; and an **idempotent redelivery** for a step with an
  unregistered action still produces that WARNING while producing no dispatch record, which is the
  carve-out AC-A6 states and which fails if the builder gates the walk on the operation id.
  **AC-E12/E13** — a `Set*` call between construction and
  dispatch does not make `queue_run` warn, which fails if the value was snapshotted at construction;
  and the walk reads through the engine's own store. Do not assert walk-vs-engine version equality:
  two `LoadStep` calls with an invalidation between them may legitimately differ (AC-E12's accepted
  residual). **AC-E14** — **all three** outcomes after a failed walk, since the cache never caches
  the walk's error: when the engine's `LoadStep` also fails, exactly one dispatch record and it is
  AC-E5's ERROR; when it succeeds, the dispatch still happens and records AC-A6's INFO or AC-E2's DEBUG per
  the result; and when the operation id was already applied, the engine short-circuits above the step
  load so no `LoadStep` runs and no dispatch record is written (AC-C3). Asserting only the first case leaves
  the contradiction this criterion was rewritten to remove untested, asserting only the first two
  treats the `LoadStep` branches as the whole matrix, and a failing walk is never a skip.
  **AC-E7/E9** — the second
  transition does not transition; a guard-unsatisfied transition and a `requires_approval` transition
  neither transition nor warn.
- **AC-F1..F7** — one skip case each, including a failing state-reload (AC-F6) and the
  `otherWorkingSessionID` failure asserting the dispatch adds **no** record of its own.
  **AC-F8** — at least two failures satisfying two guards at once, each pair chosen so it is
  **observable** (at least one of the two records) or the test proves nothing. A sessionless event on
  an unloadable task — AC-A5 silent, then AC-B2 WARNING — asserts the **absence** of the WARNING. A
  cancelled retry on an archived task — AC-A8 DEBUG, then AC-F4 silent — asserts the **presence** of
  AC-A8's DEBUG, the discriminator being the earlier guard's record since AC-F4 has none. Do **not**
  write an ordering test for a silent/silent pair such as AC-F2 against AC-F3. Add a **session-read
  failure on a task that is also unloadable**, asserting exactly one WARNING and that it is AC-F6's,
  which pins the session-before-task order and AC-F6-vs-AC-B2 ownership together. **AC-F9** — every
  dispatch record carries `task_id`, `session_id` and its stable message, which is what lets these
  assertions key on the dispatch's own lines rather than the pre-existing ones sharing the stream. A
  per-guard suite that never overlaps two guards cannot detect a wrong order.
- **Office regression** — the existing `internal/office/service` and
  `internal/office/engine_dispatcher` `on_agent_error` tests pass with no edits to their assertions.

Commands:

```
cd apps/backend
go test ./internal/orchestrator/... -run 'AgentError|AgentFailed|RecoverableFailure'
go test ./internal/office/... -run 'AgentError'
go test ./internal/workflow/engine/...
```
