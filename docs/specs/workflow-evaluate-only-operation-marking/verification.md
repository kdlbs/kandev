# Verification plan — `EvaluateOnly` operation marking

Companion to [`spec.md`](spec.md). Provenance and test mapping, not contract. Where the two
disagree, `spec.md` wins.

## Prior art receipts

The unabridged receipts; `spec.md` § Prior art carries the same three legs in short form and points
here.

- **Leg 1 (wiki) — DID NOT RUN.** Resolved `OBSIDIAN_VAULT_PATH=/Users/henry/Documents/henry/wiki`
  via `~/.obsidian-wiki/config` → `config.henry`, `QMD_WIKI_COLLECTION="wiki"`. Neither
  `obsidian-wiki` nor `qmd` is on `PATH`; the documented grep fallback returns
  `Operation not permitted` on the vault and a direct read of `index.md` returns `EPERM`. Sandbox
  restriction, not an empty vault.
- **Leg 2 (`saas-kb` / `search_fsm_docs`) — DID NOT RUN.** MCP server absent from the session tool
  list; no query issued with or without `category: "ai_sdlc"`.
- **Leg 3 (in-repo) — RAN.** `grep -rn "markOperationApplied\|IsOperationApplied"` and
  `grep -rn "OperationID:"` over `apps/backend` (non-test) at `ddcc4fc6a`. It produced the two
  precedents `spec.md` § Why dissects.

**What we do differently** (displaced from `spec.md` § Prior art, which is at its size ceiling):
neither precedent generalizes — `reevaluateGuardedTransitions` holds the invariant only because it
commits in-call, `processOnChildrenCompleted` only by declining to use the engine's marker. This spec
makes it a property of `HandleTrigger` itself and hands the caller an explicit signal, so the next
`EvaluateOnly` caller inherits it.

## Measurement receipts

Displaced from `spec.md`, which is at its size ceiling. Contract statements stay there; the raw
measurements that back them live here.

### Defect receipt

The tail of `handleTrigger` at `ddcc4fc6a`, `apps/backend/internal/workflow/engine/engine.go`:

```go
result, err := e.processActions(ctx, in, state, step, actions, filter)
if err != nil {
    return HandleResult{}, err
}

return result, e.markOperationApplied(ctx, in.OperationID)   // unconditional
```

The commit this mark claims sits inside `processActions`, behind `if !in.EvaluateOnly`.

### Blast radius receipts

`grep -rn "applied_operations\|operation_id" --include=*.sql apps/backend` returns nothing — there is
no `applied_operations` table and no operation-id column anywhere in the schema, so the marker lives
only in `appliedOps sync.Map` (`internal/orchestrator/workflow_store.go:104`, read/written at
`:886-899`) and suppression is process-scoped. `agentErrorOperationID`
(`internal/orchestrator/event_handlers_agent_error.go:58-63`) is where the
`agent_error:session:<sessionID>` fallback is built when `AgentEventData.AgentExecutionID` is empty.

### The fifth return path

Named for the path AC-EO-11's four-case table excludes, but the accounting here is over **every**
`return false` in `applyEngineTransitionWithCommitMode`. There are **six**, not five, and the sixth
is not a non-committing return at all. Measured at `ddcc4fc6a`,
`internal/orchestrator/event_handlers_workflow.go`:

| line | path | disposition |
|---|---|---|
| 5411 | target step lookup error | AC-EO-11 case 1 |
| 5419 | credential preflight failure | AC-EO-11 case 2 |
| 5433 | source-step load failure | AC-EO-11 case 3 |
| 5458 | commit error | AC-EO-11 case 4 |
| 5461 | `applied == false` (CAS closure) | never committed; unreachable, see below |
| 5524 | `maybySwitchSessionForProfile` failure | **post-commit**; unreachable, see below |

**The fifth, `applied == false`, is unreachable from `applyEngineTransition`.** The commit closure
`applyEngineTransitionWithMode` supplies returns `(true, nil)` or `(false, err)` and never
`(false, nil)`. `(false, nil)` originates only from the guarded-decision CAS closure over
`applyTransitionIfAtStepRaw`, which AC-EO-9 carves out and which never runs in `EvaluateOnly` mode.

**The sixth is a different kind of return, and is the one to read carefully.** It sits *after* the
commit at `:5458` has already succeeded, inside the `mode == transitionLifecycleOnTurnStart` branch
opened at `:5517`, so it is a `return false` on a **committed** transition — not a non-committing
path, and therefore outside AC-EO-11's framing rather than a fifth-and-sixth pair with the CAS
closure. It is unreachable from this caller by construction, on the same "mode is fixed at the sole
live call site" argument: `applyEngineTransition` (`:5343-5353`) sets
`mode = transitionLifecycleWithOnEnter` whenever `triggerOnEnter` is true, and
`dispatchKanbanAgentErrorTrigger` passes `true` (`event_handlers_agent_error.go:253`), so the
`transitionLifecycleOnTurnStart` branch cannot execute for it.

Why this matters beyond the count: a `return false` after a successful commit is a **second
structural opener** of AC-EO-17's commit-succeeded-but-unmarked window, alongside the panic that AC
names and the mark failure AC-EO-12 permits. Under AC-EO-10's `if and only if` it leaves the marker
absent on a committed transition. A future `EvaluateOnly` caller whose commit helper reaches
`applyEngineTransitionWithCommitMode` in `transitionLifecycleOnTurnStart` mode would hit it on every
profile-switch failure — the same stranding this spec exists to close, re-entered through the caller
obligations the spec hands out. That caller therefore owes a **third** obligation beyond AC-EO-10 and
AC-EO-13: confirm which lifecycle mode its commit helper selects, because the mode is what closes
this today and nothing in the engine does.

## Why the contract changed rather than the call site

Three options were on the table. The originating comment named two.

**(b) Each `EvaluateOnly` caller re-opens the marker when its own commit fails.** Rejected on
mechanism, not on taste. `TransitionStore` has no clearing verb, so this means adding one to an
interface with production and test implementations, and the clear itself can fail. Worse, it leaves
a crash window between the premature mark and the clear that reproduces the original bug exactly —
the failure it is meant to fix is the failure it reintroduces. It also has to be re-applied at every
future call site, which is the property AC-EO-15 exists to prevent.

**(a-full) A caller-driven two-phase commit protocol on `HandleInput`/`HandleResult`.** Rejected as
more machinery than the problem needs. The engine does not need to learn the caller's commit
outcome; it only needs to stop asserting one it never had.

**(a-narrow) — adopted.** The engine declines to mark exactly the work it deferred, and reports that
it did so. Two conditions in the engine, one caller change, no interface growth, no new failure
window. It also generalizes: the reason `on_children_completed` is already correct is that it does
this by hand.

The invariant is not novel, and `spec.md` § Why states it in one sentence because the detail lives
here. `Engine.reevaluateGuardedTransitions` (`internal/workflow/engine/quorum.go:667`) already states
the rule in its own doc comment — the operation id is marked applied "only once this function has
fully determined the outcome (transitioned, abandoned, or nothing satisfied) — never before, so a
crash mid-evaluation does not leave a decision's re-evaluation permanently skipped" — and it commits
through the CAS *inside* the call before marking. `EvaluateOnly` was the one mode where the outcome
is determined outside the call, and the one mode that marked anyway. This spec closes that hole; it
does not invent a new rule.

## Test mapping

TDD: every test below is written failing first. `TestHandleTrigger_EvaluateOnlySkipsPersistence`
(`engine_test.go:298`) passes no `OperationID` and so does not cover any of this — do not mistake it
for existing coverage.

### `apps/backend/internal/workflow/engine`

Extend `fakeStore` to record `MarkOperationApplied` calls (it already has an `applied` map; the
tests need call-order or a call log, not just the final set).

| AC | Test |
|---|---|
| AC-EO-1 | `EvaluateOnly: true` + non-empty `OperationID` + a step whose trigger fires `move_to_next` → store records **no** `MarkOperationApplied`, and `Transitioned` is true |
| AC-EO-2 | same, but the step declares only a non-transition action → `MarkOperationApplied` **is** called |
| AC-EO-2 | same, non-transition action returning a `DataPatch` → `MarkOperationApplied` is called. Assert **only** that a patch does not change marker ownership. Do NOT assert or imply the patch was persisted — on this path it is dropped, which `spec.md` § The contract carves out and § Out of scope tracks. Name the test for marker ownership, not for patch handling, so the deferred fix does not have to reinterpret it |
| AC-EO-3 | `EvaluateOnly: false` + transition → `MarkOperationApplied` called, `ApplyTransition` called, mark after commit |
| AC-EO-4 | `HandleResult.OperationMarkDeferred` is `true` for the AC-EO-1 case, and `false` for each of: idempotent short-circuit, no-actions step, `EvaluateOnly: false`, deferred transition with empty `OperationID`, and a `processActions` error |
| AC-EO-5 | callback returns an error → no mark, zero `HandleResult` |
| AC-EO-6 | step declares no actions for the trigger, `EvaluateOnly: true` → mark called |
| AC-EO-7 | empty `OperationID` → neither `IsOperationApplied` nor `MarkOperationApplied` called |
| AC-EO-8 | AC-EO-2, AC-EO-4-`false` and AC-EO-7 re-run through `HandleTriggerSessionShapedOnly` with the same expectations, **plus** a structural assertion that `sessionShapedActionKinds` and the kinds `isTransitionAction` admits are disjoint sets. There is deliberately **no** AC-EO-1 case here: the filter admits no transition kind and `evaluateActions` applies it before the transition branch, so a deferred transition is unreachable through this entry point and AC-EO-1 holds vacuously (`spec.md` AC-EO-8). The disjointness assertion is the thing that fails if a later change widens the filter. Do **not** make an AC-EO-1 case pass here by adding a transition kind to the session-shaped set — that double-dispatches step entry against AC-OFFICE-STEP-ENTRY-001 |
| AC-EO-9 | existing quorum re-evaluation coverage still marks `decision:<task>:<step>:<decision>`; add an explicit assertion if none pins the ordering today |

### `apps/backend/internal/orchestrator`

New file — `event_handlers_agent_error*_test.go` is already at six test files and the
800-effective-line revive limit applies to test files.

| AC | Test |
|---|---|
| AC-EO-10 | `OperationMarkDeferred` result + `applyEngineTransition` succeeds → operation id present in the store afterwards |
| AC-EO-11 | one case per **reachable** non-committing path: target step missing, credential preflight failure, source-step load failure, commit error. **Four cases, not five** — `applied == false` is unreachable from `applyEngineTransition` by construction (its commit closure never returns `(false, nil)`), so there is no fifth test and its absence is deliberate. Each case asserts the operation id is **absent** afterwards, then re-dispatches the same `AgentEventData` and asserts the engine re-evaluated rather than returning `Idempotent` |
| AC-EO-12 | **No test.** The branch is unreachable: `agentErrorDispatchDeps.store` is the concrete `*workflowStore`, whose `MarkOperationApplied` is `return nil` unconditionally, and no fake substitutes without widening a production field (deferred — `spec.md` § Out of scope). Enforcement is `errcheck` — active via golangci-lint v2's **default** linter set, not via `apps/backend/.golangci.yml`'s `enable:` list (that file sets no `linters.default`); confirm with `golangci-lint linters` rather than by grepping it — plus code review of the warn-and-continue handler. Do not manufacture a seam, and do not drop the handler |
| AC-EO-13 | two concurrent `dispatchKanbanAgentErrorTrigger` calls with the same `(SessionID, AgentExecutionID)` → exactly one commit. Run under `-race`. Assert against the **new** per-operation-id helper, not `lockChildCompletionOperation` — AC-EO-13 requires a separate map, so a test that reaches into `childCompletionLocks` is testing the wrong thing |
| AC-EO-13 | **lock SPAN through the commit, the case "exactly one commit" cannot see.** `TestDispatchKanbanAgentErrorTrigger_ConcurrentSameOperationLockSpansThroughCommit` (`event_handlers_agent_error_evaluate_only_test.go`) blocks the first dispatch **inside** its `UpdateTaskWithWorkflowStepAdmission` commit call (via `agentErrorBlockingCommitRepo`) and asserts the second, concurrent dispatch has not yet re-read `GetTaskSession` or observed the operation as applied while the first is still inside its own commit — pinning that the lock is held from before state load through the commit→mark window, not just up to the engine call. Mutation-verified: releasing the lock right after `HandleTrigger` returns (before `applyEngineTransition`/mark) made the *old* one-commit-only assertion fail just 1-in-20 runs at `-count=20`; this test fails deterministically (5/5) under the same mutation, closing the gap a scheduler-dependent assertion could not |
| AC-EO-14 | after a failed commit (task still on the **source** step), re-delivery re-executes that step's callbacks — assert the callback ran twice. Pins at-least-once as intended, so a later reader does not "fix" it |
| AC-EO-16 | `TestProcessOnChildrenCompleted_StillIdempotentOnRedelivery` (`event_handlers_children_completed_test.go`): a 3-step chain where the target step **also** declares an `OnChildrenCompleted` action, so the marker check is load-bearing — without it, redelivery would re-fire that action and advance a second time. Asserts `IsOperationApplied` is true after the first delivery and that redelivery neither transitions again nor moves the parent past its post-first-delivery step. Mutation-verified: deleting `childCompletionAlreadyApplied`'s short-circuit makes this test fail. `on_turn_start` / `on_turn_complete` callers pass no `OperationID` at all, so they never match the `HandleInput{EvaluateOnly: true, OperationID: <non-empty>}` shape the AC-EO-15 pin test (`evaluate_only_operation_marking_pin_test.go`) scans for — that pin test would flag either caller the moment it started pairing the two fields, which is the structural guarantee this row relies on rather than a dedicated behavioral test per caller |
| AC-EO-17 | commit succeeds, marker left absent → re-delivery evaluates the **target** step's actions, not the source step's. Assert the step id the second evaluation loaded, and that a mark lands by the end of it (via AC-EO-10 or AC-EO-2) so a third delivery short-circuits. Drive the missing mark from the test rather than by panicking, and do not assert a specific second transition — the AC permits one, it does not require one |

The AC-EO-11 rows are the ones that would have caught the original defect. Prefer the credential
preflight case as the primary regression test: it is the most ordinary of the four and the least
likely to be dismissed as a synthetic DB fault.

### Regression guard (AC-EO-15)

`spec.md` AC-EO-15 fixes the mechanism, the detection predicate and the remediation; this section only
says how to build it. Model on `agent_error_fire_site_pin_test.go`, which is the same kind of artifact:
a `go/parser` walk rooted at `apps/backend`, skipping `_test.go`, keyed
`"pkgRelDir/ReceiverType.FuncName"`, failing on any key absent from a registered set.

- **It is a closed-set allowlist, not a data-flow check.** Do not try to verify that
  `OperationMarkDeferred` is read — that is undecidable and false-fires on a caller that delegates the
  check into a helper.
- **Detection predicate:** a `HandleInput` composite literal with both an `EvaluateOnly:` key whose
  value is the literal `true` and an `OperationID:` key whose value is not the empty-string literal.
  Nothing more; runtime non-emptiness is not inferable and must not be attempted.
- **Seed the set with one entry:** `internal/orchestrator/Service.dispatchKanbanAgentErrorTrigger`.
- **Put it in a new file** rather than extending `agent_error_fire_site_pin_test.go`. Either satisfies
  every AC-EO-15 observable, so this is a steer, not a requirement: the two guards pin unrelated
  predicates, and a shared walk helper couples their lifetimes for no gain.
- **Editing the set IS the remediation**, not a workaround — it is the review gate, exactly as in the
  fire-site pin. Point the failure message at AC-EO-10, AC-EO-13 and AC-EO-15 together, so the next
  author reads all three caller obligations before adding their entry rather than after: honour the
  flag, serialize the window, and check which lifecycle mode their commit helper selects (§ The fifth
  return path — in `transitionLifecycleOnTurnStart` mode the helper can return `false` on a committed
  transition, which AC-EO-10's `if and only if` would then leave unmarked).

## Commands

```bash
# focused, during TDD
go test ./internal/workflow/engine/... -run 'EvaluateOnly|OperationApplied|MarkOperation' -race -count=1
go test ./internal/orchestrator/... -run 'AgentError' -race -count=1

# before commit — internal/orchestrator carries goleak.VerifyTestMain; the engine package does not
go test ./internal/workflow/engine/... ./internal/orchestrator/... -race -count=1

# changed-file lint, per apps/backend/CLAUDE.md
golangci-lint run ./... --new-from-rev="$(git merge-base HEAD origin/main)" --timeout=5m
```

All from `apps/backend`. `internal/orchestrator` carries `goleak.VerifyTestMain`, so the AC-EO-13
concurrency test must join its goroutines rather than leaving them to the race detector.

Specification lint, from the repo root:

```bash
python3 scripts/lint-spec-files.py --all
```

`docs/specs/<slug>/` classifies as `legacy` for the linter, whose only check on it is the 32768-byte
ceiling plus the size ratchet. **`spec.md` is at 32689 / 32768 bytes — 79 bytes of headroom, which is
effectively full.** A future edit must DISPLACE prose into this file, which has no ceiling, rather than
append to `spec.md`; § Measurement receipts above exists because three rounds of edits already had to.
Re-measure with `wc -c` after every edit and re-run the lint. Do not add a
`docs/specs/spec-lint-exceptions.tsv` entry: exceptions only ratchet downward and a new file has no
business needing one.

## Risks

- **The AC-EO-14 / AC-EO-17 trade is the one a reviewer will push back on.** Deferring the mark means
  callbacks can run twice when a commit fails (AC-EO-14), and — in the narrower window where the
  commit landed but the mark did not — that the re-delivery evaluates the *target* step and may
  transition again (AC-EO-17). The alternative is the current behavior, where neither happens and the
  task is stranded. Both are stated as ACs rather than left implicit precisely so the push-back lands
  on the decision instead of on the implementation.
- **AC-EO-13's lock must span load → engine → commit → mark, not engine → commit → mark.** This is
  the trap of the whole spec. The engine consumes `HandleInput.PreloadedState` verbatim and never
  re-reads the store, and the live caller builds that state *before* it even computes the operation
  id — so a lock that starts at the engine call lets a blocked racer wake holding pre-commit state
  and re-evaluate the **source** step, which AC-EO-17 forbids. Because `ApplyTransition` is a plain
  write with no compare-and-set, that second commit re-runs `processOnExit` for a step the task has
  already left and re-enters the target: a step-entry double-dispatch. A naive concurrency test that
  only counts commits or marks passes anyway, which is why the second AC-EO-13 row above asserts the
  step id instead. The lock must also be a *separate* map from `childCompletionLocks`, per AC-EO-13.
- **AC-EO-12 has no test, on purpose.** `sync.Map` makes `MarkOperationApplied` infallible and the
  caller's dependency is a concrete `*workflowStore`, so the error branch is neither reachable nor
  injectable without a production change this spec defers. `errcheck` keeps the error from being
  dropped; the handler ships untested and becomes testable when the marker is persisted (`spec.md`
  § Out of scope). Reviewers should not read the missing test as an omission — but should reject any
  attempt to delete the handler because it lacks one.
