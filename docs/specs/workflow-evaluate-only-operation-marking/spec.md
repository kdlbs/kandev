---
status: draft
created: 2026-09-05
updated: 2026-09-05
owner: Kandev
---

# `EvaluateOnly` operation marking in the workflow engine

Decisions:

- [ADR-0004 task-model-unification](../../decisions/0004-task-model-unification.md) — establishes
  `internal/workflow/engine` as the shared coordinator every trigger routes through, which is why this
  is a contract change, not a call-site patch.

Origin:

- Review comment 3921830599 on [PR #3315](https://github.com/kdlbs/kandev/pull/3315)
  (thread `PRRT_kwDOQ2-eWs6ezhf9`). The defect predates that PR; #3315 added the fourth
  `EvaluateOnly` call site and the reviewer noticed the shared gap from it.

Related spec (same subsystem, adjacent defect class):

- [`docs/specs/workflow-on-agent-error-dispatch/spec.md`](../workflow-on-agent-error-dispatch/spec.md)
  — the six `on_agent_error` dispatch routes (R1-R6) and the fire site this spec's caller half sits
  in. Its route table is load-bearing here and is not restated.

## Why

`Engine.handleTrigger` records an operation as applied even when it has deliberately not committed
that operation's effect. In `EvaluateOnly` mode the engine skips `ApplyTransition` and leaves the
commit to its caller, then marks the operation applied anyway before returning. If the caller's
commit does not land, the idempotency marker still says it did, and every later delivery of the same
operation short-circuits on `Idempotent: true`. The task stays on its old step with no further
recovery attempt.

Verified 2026-09-05 against `ddcc4fc6a`: `handleTrigger`'s tail
(`apps/backend/internal/workflow/engine/engine.go`) marks unconditionally, while the commit that mark
claims sits inside `processActions` behind `if !in.EvaluateOnly` — so in `EvaluateOnly` mode
`applyTransition` never runs, yet the mark still does. Source quoted in
[`verification.md`](verification.md#defect-receipt).

This is not a new design position. `Engine.reevaluateGuardedTransitions` already states the same rule
in its own doc comment and commits in-call before marking, so `EvaluateOnly` is the one mode that
marks an outcome it did not determine. Quoted and dissected in
[`verification.md`](verification.md#why-the-contract-changed-rather-than-the-call-site).

### Corrections to the reported scope

The originating comment lists four affected call sites. Measured, only one is live. `markOperationApplied`
returns `nil` immediately when `OperationID` is empty, so a caller that passes no operation id is
unaffected by construction. Verified by `grep -rn "OperationID:" --include=*.go apps/backend`
(non-test) cross-referenced against every `EvaluateOnly: true` site:

| Call site | `EvaluateOnly` | `OperationID` passed | Affected today |
|---|---|---|---|
| `event_handlers_agent_error.go:227` (`dispatchKanbanAgentErrorTrigger`) | true | **yes** (`agentErrorOperationID`) | **yes** |
| `event_handlers_workflow.go:5039` (`on_turn_complete`) | true | no | no — mark is a no-op |
| `event_handlers_workflow.go:5620` (`on_turn_start`) | true | no | no — mark is a no-op |
| `event_handlers_children_completed.go:228` (`on_children_completed`) | true | no | no — see below |
| `office/engine_dispatcher/dispatcher.go:166` | false | yes | no — engine commits, then marks |
| `orchestrator/workflow_callbacks.go:142` (`switchWorkflowDispatcher`) | false | yes | no — engine commits, then marks |

`on_children_completed` is the informative case: it passes **no** operation id and runs the two-phase
protocol by hand against the same store — `childCompletionAlreadyApplied` before the engine call,
`markChildCompletionApplied` only after the commit returns true. Its author opted out of the engine's
idempotency entirely to get the ordering this spec makes the default, which is the evidence that the
engine's contract is wrong.

So this is a **latent contract defect with one live instance**. The ACs are written against the engine
contract, not the single caller, because `HandleInput` today permits `EvaluateOnly: true` with a
non-empty `OperationID` and gives no signal that the resulting marker is a lie.

### Blast radius, measured rather than assumed

`workflowStore.IsOperationApplied` / `MarkOperationApplied`
(`internal/orchestrator/workflow_store.go:886-899`) are backed by `appliedOps sync.Map` (`:104`), and
there is no `applied_operations` table and no operation-id column anywhere in the schema (measurement
receipts in [`verification.md`](verification.md#blast-radius-receipts)). Two consequences the fix must
be specified against:

1. Suppression is **process-scoped, not permanent** — a restart clears every marker. The defect strands
   a task until then, which on a long-lived install is hours to days, but "permanently" overstates it.
2. There is no marker-clearing verb, which rules out the "each caller re-opens the marker on commit
   failure" option: adding one grows `TransitionStore` for every implementation, and the clear leaves
   a crash window that reproduces the original bug. Not marking has no such window.

The suppression window is also wider than the "transient DB failure" the report imagines. AC-EO-11
enumerates the four reachable non-committing paths, most of them ordinary: a credential preflight
failure is a routine operator condition, and today it burns the operation id for the process's
lifetime. Worse, where `AgentExecutionID` is empty the key falls back to one per *session*, so a
suppression there disables `on_agent_error` recovery for that whole session until restart, not just
for one failure (receipts in [`verification.md`](verification.md#blast-radius-receipts)).

## Prior art

Three legs; receipts, findings, and what we do differently in
[`verification.md`](verification.md#prior-art-receipts). The wiki and `saas-kb` legs **DID NOT RUN**
(tools absent from this sandbox — redo them from an unsandboxed checkout before treating this
contract as unprecedented); the in-repo leg **RAN** and produced the two precedents dissected above.

## What

### The contract

The engine records an operation as applied when, and only when, its own call has fully determined
that operation's **transition** outcome. Work the engine defers to its caller is not the engine's to
claim.

In `EvaluateOnly` mode a transition is deferred work. Nothing else is: callbacks have already
executed by the time `processActions` returns, and a run that produces no transition has no
transition outstanding.

Ownership of the marker therefore transfers to the caller exactly when a transition is deferred, and
the engine says so in its result — `HandleResult.OperationMarkDeferred` — rather than leaving each
caller to re-derive the rule from `EvaluateOnly`.

**The invariant is scoped to transitions, deliberately and narrowly.** It is not "the call fully
determined every effect". One effect is knowingly excluded: in `EvaluateOnly` mode an evaluation that
produces a data patch and no transition drops that patch, because the engine skips `PersistData` and
the caller only persists `result.DataPatch` inside its transition branch. That is a real, separate
defect, tracked in [Out of scope](#out-of-scope). AC-EO-2 still marks on that path, and this is a
decision rather than an oversight: marking there is the pre-existing behavior, withholding the mark
would strand the operation forever without persisting the patch, and no re-delivery can recover a
patch that no code path writes. Widening the invariant to cover patches would mean fixing the drop — a
different change with a different owner. Naming the exclusion here keeps AC-EO-2 and the invariant
consistent rather than quietly contradictory.

### Terms

- **Deferred transition** — a `HandleTrigger` call where `HandleInput.EvaluateOnly` is `true` and
  evaluation selected a target step different from the current step (the condition that sets
  `HandleResult.Transitioned`).
- **Marker** — the `(operation id → applied)` entry read by `TransitionStore.IsOperationApplied` and
  written by `TransitionStore.MarkOperationApplied`.
- **Commit** — the caller's persistence of a deferred transition. For the live caller this is
  `Service.applyEngineTransition` returning `true`.
- **Deferred-mark flag** — `HandleResult.OperationMarkDeferred`, the new exported boolean defined by
  AC-EO-4. The identifier is fixed here: AC-EO-10 reads it and AC-EO-15 scans for it by name.

### Engine acceptance criteria

- **AC-EO-1:** WHERE `HandleInput.EvaluateOnly` is `true` AND the call produces a deferred
  transition, `Engine.HandleTrigger` SHALL NOT invoke `TransitionStore.MarkOperationApplied` for
  `HandleInput.OperationID`.

- **AC-EO-2:** WHERE `HandleInput.EvaluateOnly` is `true` AND the call produces no deferred
  transition, `Engine.HandleTrigger` SHALL invoke `TransitionStore.MarkOperationApplied` for a
  non-empty `HandleInput.OperationID`, unchanged from current behavior. This includes the case where
  callbacks executed and returned a data patch but no transition action fired. A data patch SHALL NOT
  change marker ownership. This is not a claim that the patch was persisted — on this path it is
  dropped, per the carve-out in [The contract](#the-contract) and the entry in
  [Out of scope](#out-of-scope). Marking is still correct here because withholding it fixes nothing
  and strands the operation instead.

- **AC-EO-3:** WHERE `HandleInput.EvaluateOnly` is `false`, `Engine.HandleTrigger` SHALL invoke
  `TransitionStore.MarkOperationApplied` after `processActions` returns a nil error, unchanged from
  current behavior, whether or not a transition occurred.

- **AC-EO-4:** `HandleResult` SHALL expose an exported boolean field named exactly
  `OperationMarkDeferred`, reporting that the engine skipped the mark and the caller now owns it. It
  SHALL be `true` if and only if AC-EO-1 applied **and** `HandleInput.OperationID` is non-empty. It SHALL be `false` on every other return from
  `HandleTrigger`, including: the idempotent short-circuit, a step declaring no actions for the
  trigger, `EvaluateOnly: false`, a deferred transition with an empty `OperationID`, and every error
  return.

- **AC-EO-5:** IF `processActions` returns a non-nil error, THEN `Engine.HandleTrigger` SHALL NOT
  invoke `MarkOperationApplied` and SHALL return the zero `HandleResult`. (Current behavior; pinned
  so the fix cannot regress it.)

- **AC-EO-6:** WHERE the step declares no actions for the trigger, `Engine.HandleTrigger` SHALL
  invoke `MarkOperationApplied` regardless of `EvaluateOnly`, because nothing was deferred.

- **AC-EO-7:** WHERE `HandleInput.OperationID` is the empty string, `Engine.HandleTrigger` SHALL NOT
  invoke `IsOperationApplied` or `MarkOperationApplied`, unchanged from current behavior.

- **AC-EO-8:** `HandleTriggerSessionShapedOnly` SHALL follow AC-EO-1 through AC-EO-7 identically to
  `HandleTrigger`; the action-kind filter SHALL NOT change marker ownership. Both route through the
  same `handleTrigger` body.

  AC-EO-1 holds **vacuously** on this entry point, by construction rather than by omission:
  `sessionShapedActionKinds` and the kinds `isTransitionAction` admits are disjoint sets, and
  `evaluateActions` applies the filter *before* the transition branch, so no deferred transition is
  reachable here. Observable coverage is therefore AC-EO-2, AC-EO-4's `false`, and AC-EO-7 through this
  entry point, plus a structural assertion that `sessionShapedActionKinds` admits no transition kind —
  that assertion is what fails if a later change widens the filter and makes AC-EO-1 reachable, which
  is the "a fix applied to only one entry point fails" property this AC exists for. Satisfying this AC
  by admitting a transition kind into the session-shaped set is FORBIDDEN: it would double-dispatch
  step entry against AC-OFFICE-STEP-ENTRY-001.

- **AC-EO-9:** `Engine.reevaluateGuardedTransitions` SHALL continue to mark its own
  `decision:<task>:<step>:<decision>` operation id after `applyFirstSatisfiedGuardedTransition`
  returns, unchanged. Its commit is in-call, so it is not deferred work and is outside AC-EO-1.

### Caller acceptance criteria

- **AC-EO-10:** WHEN `dispatchKanbanAgentErrorTrigger` receives a `HandleResult` whose
  `OperationMarkDeferred` is `true`, it SHALL invoke `MarkOperationApplied` for that operation id if
  and only if `applyEngineTransition` returned `true`. The live call site discards that return value
  today; capturing it is part of this AC.

- **AC-EO-11:** IF `applyEngineTransition` returns `false` for a deferred transition, THEN the
  operation SHALL remain unmarked, so a later delivery carrying the same operation id re-evaluates
  instead of short-circuiting on `Idempotent: true`. This SHALL hold for **all four non-committing
  return paths reachable from this caller**: target step not found, credential preflight failure,
  source-step load failure, and commit error — not only the commit-error path.

  `applyEngineTransitionWithCommitMode` has a fifth non-committing return, `applied == false`, and it
  is **unreachable from `applyEngineTransition` by construction** — the closure trace is in
  [`verification.md`](verification.md#the-fifth-return-path). No test is required for it and its
  absence from the test table is deliberate, not an omission. If a future change makes that return
  reachable from this caller, this AC extends to it unchanged.

- **AC-EO-12:** IF the caller's own `MarkOperationApplied` returns an error, THEN the caller SHALL
  log at warn level with the task id, session id and operation id, and SHALL NOT treat the
  transition as failed. The transition is already committed; the consequence is a possible
  re-delivery, whose behavior AC-EO-17 defines.

  **This is a code-shape requirement, not a runtime-observable one, and no test is required for it.**
  The branch is unreachable and not injectable — see the `agentErrorDispatchDeps.store` entry under
  [Out of scope](#out-of-scope) for why. Enforcement is static instead: `errcheck` is active for this
  package via golangci-lint v2's **default** set — absent from `apps/backend/.golangci.yml`'s
  `enable:` list, which sets no `linters.default`, so confirm it with `golangci-lint linters` rather
  than by grepping that file — so the error cannot be silently discarded, and this AC fixes what the
  handler does with it. Do not manufacture a seam to test it, and do not drop the handler because it
  cannot be tested — it becomes reachable the moment the marker is persisted.

- **AC-EO-13:** `dispatchKanbanAgentErrorTrigger` SHALL serialize concurrent invocations that carry
  the same operation id. The lock SHALL be acquired **before the task, session and `MachineState`
  used for evaluation are loaded**, and held across the engine call, the commit, and the caller's
  mark. Locking only the engine-call-to-mark span breaks AC-EO-17: a racer that blocked on the lock
  wakes holding a `PreloadedState` built before the winner's commit, which the engine consumes
  verbatim, so it re-evaluates the **source** step. The operation id derives only from the failure
  event, so it is computable at entry, before any load.
  See [Concurrency](#concurrency) for why this becomes required rather than merely advisable.

  The lock SHALL be a **new, separate** ref-counted per-operation-id helper on `Service` — the same
  shape as `lockChildCompletionOperation`, not the same instance. `Service.childCompletionLocks` is
  named for child-completion ids; admitting `agent_error:*` keys couples two unrelated triggers'
  contention and lets a change to either one's lock lifetime silently affect the other. Naming is the
  builder's; the contract is not: keyed by operation id, created on first acquire, ref-counted, and
  deleted when the last holder releases, so the map does not grow without bound.

- **AC-EO-14:** On the deferred path, action callbacks SHALL be understood as at-least-once. When a
  commit fails and the operation is re-delivered, the **source** step's callbacks run again alongside
  the re-evaluation (AC-EO-17 covers the case where the commit instead succeeded). This is the intended trade: on the failure path, re-running recovery actions is
  preferable to a task stranded with no recovery at all. Callers that cannot tolerate a callback
  replay must not defer.

- **AC-EO-15:** A regression guard SHALL fail when a non-test `engine.HandleInput` composite literal
  pairs `EvaluateOnly: true` with a non-empty `OperationID` and its enclosing function is not in a
  registered set. The predicate below is this AC's exact and intended reach, not an approximation of a
  broader rule: the blind spot it names is part of the requirement, not a shortfall against it. This is
  the AC that makes the contract survive the next caller. Its mechanism and predicate are fixed here
  because the obvious reading — verify the caller *consults* `OperationMarkDeferred` — is a data-flow
  property no source scan can decide, and
  attempting it false-fires on any caller that delegates the check into a helper, the way
  `dispatchKanbanAgentErrorTrigger` already delegates its commit into `s.applyEngineTransition`.

  - **Mechanism: a closed-set allowlist pin**, modelled on `agent_error_fire_site_pin_test.go` — a
    `go/parser` walk rooted at `apps/backend`, skipping `_test.go` files, collecting offending call
    sites keyed `"pkgRelDir/ReceiverType.FuncName"` and failing on any key not in a registered set.
    The guard SHALL NOT attempt to verify that `OperationMarkDeferred` is read. It does not check
    correctness; it forces a human to review any new caller.
  - **Detection predicate, purely syntactic and therefore decidable:** a composite literal of type
    `engine.HandleInput` (or `HandleInput` inside the engine package) having **both** an
    `EvaluateOnly:` key whose value is the literal `true`, **and** an `OperationID:` key whose value
    is anything other than the empty-string literal. Runtime non-emptiness is undecidable and SHALL
    NOT be attempted — every live site passes an identifier (`OperationID: operationID`), never a
    literal, so "key present and not literally empty" is the working definition of non-empty. A caller
    that omits the `OperationID:` key entirely is not detected, which is correct: AC-EO-7 makes it
    unaffected. **Known blind spot:** a caller that does not build a `HandleInput` composite literal
    in place — assigning fields on a variable, or receiving one from a helper — is not detected
    either. Accepted: no live site does it, and widening the walk to chase assignments reintroduces
    the data-flow analysis this AC exists to avoid.
  - **Registered set seeded with exactly one entry**,
    `internal/orchestrator/Service.dispatchKanbanAgentErrorTrigger`, per the scope table in
    [Corrections to the reported scope](#corrections-to-the-reported-scope).
  - **Remediation when it fires:** add the new call site to the registered set, in the same commit
    that makes that caller honour `OperationMarkDeferred` per AC-EO-10 and serialize per AC-EO-13.
    Editing the set is the intended remediation, not a workaround — it is the review gate. The failure
    message SHALL cite AC-EO-10, AC-EO-13 and AC-EO-15 so the next author reads the obligations before
    editing the set rather than after.

- **AC-EO-16:** The three `EvaluateOnly` callers that pass no operation id — `on_turn_complete`,
  `on_turn_start`, `on_children_completed` — SHALL be unchanged by this work. In particular
  `processOnChildrenCompleted` SHALL keep its own `IsOperationApplied` / `MarkOperationApplied`
  bracket; it is correct, and rewriting it to pass an operation id through the engine is a
  behavior-neutral refactor this spec does not authorize.

- **AC-EO-17:** WHERE a deferred transition's commit succeeded but the operation was left unmarked,
  a re-delivery of that operation id SHALL evaluate the task's **current** step — which is now the
  transition's target step — and that step's own `on_agent_error` actions. It SHALL NOT re-evaluate
  the source step. A further transition produced by that evaluation is permitted. An evaluation that
  completes **without a failed commit** invokes the mark — via AC-EO-10 once the caller commits, or
  via AC-EO-2 when no transition fires — and once that invocation succeeds the next delivery
  short-circuits on `Idempotent: true`. An evaluation whose commit fails leaves it unmarked, which is
  the ordinary AC-EO-11 retry path, now anchored at whatever step the task occupies. Re-delivery is
  therefore bounded by the event source's
  own retry policy, not by this marker; the marker only guarantees a *successful* outcome is not
  re-run.

  This window is narrow but real. **Three** things can open it, and only one is reachable for the live
  caller today: a panic between commit and mark, swallowed by
  `dispatchKanbanAgentErrorTriggerRecovered`'s `recover()`. The other two are shut by facts about this
  caller rather than by the engine, so a future caller inherits the duty to re-check both: a failing
  caller mark, which AC-EO-12 permits and which cannot occur while the store's `MarkOperationApplied`
  is infallible; and `applyEngineTransition` returning `false` *after* it has committed, which happens
  only in a lifecycle mode this caller never selects. Receipts for all three in
  [`verification.md`](verification.md#the-fifth-return-path). The reachable one is accepted rather
  than closed — one extra recovery evaluation against the step the task actually occupies is the same
  work
  `on_agent_error` would do if the agent failed again there, and strictly preferable to the defect
  being fixed here, where the task is stranded with no recovery at all. A caller that cannot tolerate
  it must not defer.

## Forced-to-invent pass

The axes below are settled here so a builder does not have to guess. Each states a decision, not a
description.

### Ordering

There is exactly one ordering constraint and it is total: **commit, then mark.** For a deferred
transition the sequence is state load → `IsOperationApplied` → evaluate → caller commit →
`MarkOperationApplied`, all inside AC-EO-13's lock. No step may be reordered and none may be skipped
on the success path.

The marker write is unordered with respect to the transition's other post-commit work (step history
row, `processOnEnter`, session state flip) because it shares no state with them. It SHALL be issued
after `applyEngineTransition` returns, from the same goroutine, and SHALL NOT be moved into
`launchProcessOnEnter`'s goroutine, where a panic or early return would drop it silently.

### Idempotency and retry

Marking an already-marked operation is a no-op: `sync.Map.Store` overwrites. A double mark is harmless
and needs no guard.

The engine's `Idempotent: true` short-circuit keeps its current meaning — "this operation's outcome was
already fully determined" — and after this change that statement is true, which it was not before.

A re-delivery of an unmarked operation re-runs the whole call against **the task's current step**,
whatever that now is: `assembleMachineState` derives `CurrentStepID` from `task.WorkflowStepID`, and
`loadExecutionContext` loads that step's spec. Which step that is depends on whether the first
attempt's commit landed, and the two cases are not symmetric:

- **Commit did not land** (the four AC-EO-11 paths). The task is still on the source step, so the
  re-delivery re-evaluates the *same* actions. Callbacks replay — at-least-once, per AC-EO-14 — and the
  transition is retried. This is the recovery the whole change exists to restore.
- **Commit landed but the mark did not.** The task is now on the *target* step, so the re-delivery
  evaluates that step's actions instead. AC-EO-17 governs this case and permits it.

It does **not** reduce to "the second evaluation finds `targetStepID == state.CurrentStepID` and
produces no transition" — that assumes the source step is re-read, and it is not. AC-EO-17 governs
what the second evaluation does and when it terminates; only a repeatedly failing commit keeps the
operation re-evaluating, which is the retry this change exists to restore.

### Concurrency

Two callers holding the same operation id concurrently is not newly possible, but it is newly
consequential.

Today `IsOperationApplied` and `MarkOperationApplied` are separate `sync.Map` operations with no
compare-and-set between them, so two callers can both pass the check and both proceed. The window
between check and mark is currently the engine call. After this change, for a deferred transition,
it widens at both ends: forward to include the caller's commit — `processOnExit`, a credential
preflight, a DB write and a step-history write — and back over the caller's own state load, because
the engine reads `PreloadedState` verbatim and never re-reads the store. AC-EO-13 closes the whole
widened window for the live caller with a per-operation-id mutex, which is the same instrument
`processOnChildrenCompleted` already uses and which is sufficient because both
racers are in-process by construction (the marker itself is process-local, so a cross-process racer
could never have been deduplicated by it anyway).

The engine SHALL NOT serialize on the caller's behalf. It does not today, its store interface has no
claim/release verb, and adding one would make every `TransitionStore` implementation responsible for
lock lifetime to fix a race only one caller can currently experience.

That decision pushes an obligation onto callers, so it is stated as a general rule rather than only
for the one that exists: **any caller that pairs `EvaluateOnly: true` with a non-empty `OperationID`
inherits two obligations, not one** — honour `OperationMarkDeferred` (AC-EO-10) *and* serialize the
load → check → evaluate → commit → mark window against its own operation id (AC-EO-13). AC-EO-15's
guard cannot verify either; it fires so a human checks both before the caller ships. A future caller that
takes the flag but skips the lock reintroduces exactly the race this section describes, widened.

Two callers with *different* operation ids on the same task remain unserialized by this spec. That
race is real and predates it — it is the race `ApplyTransitionIfAtStep` (AC-46/48) exists for — and
it is untouched here.

### Nil, empty and error

Each case names the AC that governs it; the ACs are authoritative.

- Empty `OperationID`: no store calls at all (AC-EO-7), `OperationMarkDeferred` is `false` (AC-EO-4).
  This is the current behavior of three of the four `EvaluateOnly` callers and must keep working.
- `processActions` error: no mark, zero `HandleResult` (AC-EO-5). Every caller already discards the
  result on a non-nil error; this spec introduces no partial result.
- `MarkOperationApplied` error in the engine (AC-EO-2/3 paths): returned as `HandleTrigger`'s error
  alongside a populated `HandleResult`, unchanged. Pre-existing and deliberately left alone — see
  [Out of scope](#out-of-scope).
- `MarkOperationApplied` error in the caller: warn and continue (AC-EO-12); unreachable today.
- Nil `TransitionStore`: out of contract. The engine already dereferences `e.store` unconditionally and
  `New` accepts no nil store from any production wiring.

### Defaults and boundaries

- `OperationMarkDeferred` defaults to `false` — the zero value is the safe one, so a caller that ignores
  it behaves exactly as it does today, and a mistakenly zero-valued `HandleResult` never claims
  ownership the caller has not accepted.
- No new configuration, no runtime flag, no profile entry. The correct behavior is unconditional;
  a flag would mean shipping the defect as a supported mode.
- No schema change. The marker stays in `sync.Map` (see [Out of scope](#out-of-scope)).
- `HandleInput` gains no field. `EvaluateOnly` plus the evaluated outcome already determines
  ownership; a second input flag would let a caller ask for the broken ordering.

## Out of scope

Named exclusions, each with what a follow-up would need to know.

- **Persisting the operation marker.** `appliedOps` has no eviction and no durability (see
  [Blast radius](#blast-radius-measured-rather-than-assumed)). Fixing it means an
  `applied_operations` table with a retention policy, a migration, and a decision about what
  `IsOperationApplied` returns while that table is written under load — a separate durability
  requirement that changes behavior for callers this spec does not touch. This spec is correct under
  both the current in-memory store and a future persisted one, because it constrains only *when* the
  mark is issued.

- **Data patches dropped on the no-transition `EvaluateOnly` path.** In `EvaluateOnly` mode the
  engine skips `PersistData` (`engine.go:292`) and the caller persists it — but
  `applyEngineTransitionWithCommitMode` only persists `result.DataPatch` inside its transition path
  and only when `sessionLifecycle` is set (`event_handlers_workflow.go:5492`), and it is only called
  at all when `result.Transitioned` is true. So an `EvaluateOnly` evaluation that produces a data
  patch and no transition drops the patch entirely. This is a real defect in the same mode and the
  same function, and it was found while confirming AC-EO-2's boundary. It is excluded because it is
  a different failure (silent data loss, not retry suppression), it has a different fix (deciding
  whether the engine or the caller owns patch persistence in evaluate-only mode), and no
  `EvaluateOnly` caller demonstrably hits it today — `set_workflow_data` is the only patch-producing
  callback and the shipped steps that declare it also declare a transition. A follow-up needs:
  those two file:line anchors, the fact that `on_children_completed` never persists patches at all,
  and a survey of whether any workflow YAML declares `set_workflow_data` without a sibling
  transition action. This exclusion is why [The contract](#the-contract) scopes its invariant to
  transitions and why AC-EO-2 marks on this path anyway; a follow-up that fixes the drop should
  revisit that carve-out, not just the persistence call.

- **`HandleTrigger` returning a populated `HandleResult` alongside a non-nil error.** On the
  AC-EO-2/3 paths a mark failure yields a result describing a committed transition together with an
  error, and every caller discards the result and returns — so in non-`EvaluateOnly` mode the
  transition committed but the caller believes the call failed. Excluded: a wart on the *non*-deferred
  path, fixing it changes error semantics for the Office dispatcher and `switchWorkflowDispatcher`,
  and it is unreachable while `MarkOperationApplied` cannot fail. A follow-up should take it with
  the persistence work above, which is what makes it reachable.

- **Widening `agentErrorDispatchDeps.store` to an interface.** The field, and `Service.workflowStore`
  behind it, are the concrete `*workflowStore`, whose `MarkOperationApplied` is `return nil`
  unconditionally. That is why AC-EO-12's handler cannot be driven from a test and is enforced
  statically by `errcheck` instead. Widening the field to a `TransitionStore`-shaped interface would
  make it testable, but it is a production dependency change for a branch that is unreachable until
  the marker is persisted, and it touches wiring this spec otherwise leaves alone. A follow-up should
  do it together with the persistence work above, which is what makes the branch reachable and the
  test worth having.

- **Migrating `on_children_completed` onto the engine's operation id.** AC-EO-16 pins it unchanged. A
  behavior-neutral simplification once the engine's contract is correct, but a refactor of a working
  path — it belongs to whoever next has reason to touch it.

- **Frontend, API and documentation surfaces.** Not observable from any user-facing surface. See
  [E2E decision](#e2e-decision).

## E2E decision

**No Playwright coverage.** No user-visible surface: no API response shape, no WS event, no DTO field,
no rendered copy. The entire observable footprint is the *absence* of a suppressed retry inside the
backend, which no browser-level assertion can distinguish from an ordinary successful dispatch.

Surfaces touched: `internal/workflow/engine` (contract) and `internal/orchestrator` (one caller).
Nothing under `apps/web`, `docs/public`, or `pkg/api/v1`. Verification is Go unit tests only —
commands, failing-first list and per-AC coverage mapping are in
[`verification.md`](verification.md).
