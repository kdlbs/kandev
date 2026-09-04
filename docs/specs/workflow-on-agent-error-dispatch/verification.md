# Verification plan and decision record — `on_agent_error` dispatch for non-Office tasks

## Prior art

**Leg 1 (wiki) and Leg 2 (`saas-kb`) DID NOT RUN** — tooling-unavailable, not empty results; receipts
are in the task plan under `## Prior art — receipts`.

**Leg 3 — in-repo prior reasoning (ran, and it outranks fresh design here).** ADR-0004 already
specified this trigger and governs three decisions:

- §7 "fire on the failing task **at its current step**" — adopted literally; see [AC-A4](spec.md#a-dispatch).
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
`orchestrator.Service.handleRecoverableFailureLockedState`, gated on a fail-closed Office-ownership
check and on the failure not being user-initiated.**

Why that function and not `handleAgentFailedLocked`, the more obvious reading of "the orchestrator's
agent-failure path": it is **the single terminal funnel**. All six routes reaching a terminal
session-backed kanban failure converge here ([Scope](spec.md#scope)), so a site placed after the
`handleRecoverableFailureLockedState(...)` call inside `handleAgentFailedLocked` would miss five. It
is also where the card said to look — "alongside where it already reconciles task state for runtime
failures": it completes the turn, persists the last agent error, sets session state, writes task
REVIEW, and reconciles a pending step-completion signal.

**A retry in flight never reaches it, but a retry that has given up does.** The first delivery of a
transient provider error returns at `handleTransientFailure`. The retry loop's own **terminal exits**
reach it through `handleRecoverableFailure`; two of those three are genuine failures and the third is
a user pressing Cancel. See [AC-A8](spec.md#a-dispatch), [AC-A9](spec.md#a-dispatch), [AC-A10](spec.md#a-dispatch).

Why this cannot become a third path overlapping Office's two. Path A (`dispatchAgentErrorTrigger`)
requires a claimed Office `*models.Run`; Path B (`HandleRunFailure` → `escalateFailure`) queues its
own CEO run and never reaches Path A. Both are Office-run-scoped by construction. But the Office
subscriber and the orchestrator watcher both subscribe to `events.AgentFailed`
(`event_subscribers.go:264`, `watcher/watcher.go:330`), so **both handlers run on every failure** and
without an explicit gate the new site fires alongside Path A on every Office failure. The two
operation-id namespaces are disjoint by construction, so a shared key can never absorb that overlap:
the gate is the only thing preventing it, which is why [AC-B1](spec.md#b-office-isolation) fails closed.

## Why the empty-execution-id fallback exists

Not because the synthetic events omit the execution id — all four `watcher.AgentEventData` sites set
it. It exists because the field can still arrive **empty**: `CancelTransientRetry` discards
`GetExecutionIDForSession`'s error, `retryTransientPrompt` guards teardown with `if execID != ""`,
and a start failure can be raised before an execution row exists. Rare but reachable; the collapsed
key's cost is [AC-C8](spec.md#c-idempotency)'s.

## Why the dispatch runs off the session guard

Rationale for [`spec.md`](spec.md#off-the-session-guard)'s off-guard rule and [AC-A11](spec.md#a-dispatch).

**Why the alternative was rejected.** Excluding `auto_start_agent` reproduces this card's own defect
one action over — an action that parses, compiles, appears in the step definition and silently never
runs, for the most obvious thing an operator configures under a recovery trigger. It is also a
per-kind allow-list needing maintenance forever: any future callback reaching a guard-acquiring path
reopens the hole, where running off the guard closes it for every kind at once. The vocabulary row
stays "yes".

**Why a synchronous release-then-dispatch was chosen over a dispatch goroutine.** Both close the
self-deadlock: the failing property is holding the guard while calling the engine, not the choice of
goroutine boundary. A goroutine adds a completion hook for tests, panic recovery of its own, and an
async lifecycle to reason about at shutdown; releasing the guard before an ordinary synchronous call
needs none of that — the call returns like any other, a test drives it directly, and there is nothing
to drain. The one thing the goroutine design bought for free — a recover above every route — is not
free here, because routes R2-R5 never reach the event bus's own `invokeHandler` wrapper. AC-A11
recovers this explicitly at the closure itself rather than by picking a mechanism that recovers it
incidentally.

**What it costs.** This dispatch is no longer serialized with the session's other decisions the way
it was while running under the guard. [Concurrency](spec.md#concurrency-and-ordering) and
[Failure modes](spec.md#failure-modes) enumerate and accept each consequence; none is a data race.

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
- **AC-A11** — two tests. **(1) The regression guard for the defect that reopened this spec.** A
  non-Office task whose current step declares exactly `on_agent_error: [auto_start_agent]`, driven
  through a **guard-holding** entry point — `handleAgentFailed` or `handleRecoverableFailure`, never
  a direct call to the dispatch function — completes within a bounded deadline. Because a dispatch
  that runs while still holding the guard **hangs** rather than returning a wrong value, the test
  shall enforce its own deadline and fail at it. **(2) A panicking dispatch does not take the process
  down.** Inject the panic through a registered callback's `Execute` — the only available seam, not a
  stub engine — drive it through a guard-holding entry point, and assert the entry point still
  returns, the ERROR record names the task and session ids, and no dispatch INFO/DEBUG record is
  also emitted for that delivery.
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
  named [out of scope](spec.md#out-of-scope) — a test asserting `on_agent_error` there cannot pass.
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
