---
created: 2026-09-28
status: complete
requirements:
  - REQ-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001
  - REQ-UI-CANCEL-TURN-PROGRESS-001
system_design:
  - ../../specs/tasks/system-design/workflow-cancelled-turn-completion.md
  - ../../specs/ui/system-design/cancel-turn-progress.md
legacy_specs: []
---

# Fix plan: Queued prompt blocks pause

## Overview

Remove the lock cycle between queued prompt preparation and stream publication.
Bound cancellation guard waits and verify the existing desktop and phone controls.
The three sequential work orders are complete.

## Evidence and root cause

Read-only investigation on 2026-09-28 used retained logs and live Go goroutine snapshots.
The affected task is `6f856949-3cd1-4eaa-be2f-5be3fb9a6fd2`.
Its session is `3c122950-28ea-480a-887f-8b48dbcbeb62`.
The live checkout `/root/kandev` reported `89ff7ff7131`; this package starts from `adc5d67f55d`.
The live binary's source offsets differ from this workspace; both contain the relevant locking pattern.

| Lisbon time | Evidence |
| --- | --- |
| 02:36:49 | Queued dispatch marks the session RUNNING, starts a turn, and enters prompt preparation. |
| 10:19:09 | The user's cancellation reaches the handler and publishes pending progress. |
| About 10:26 | Two goroutines have waited approximately 469 minutes; cancellation waits on their session guard. |

The stacks and source establish this cycle:

| Goroutine | Holds | Waits for |
| --- | --- | --- |
| 16856125: queued dispatch through `preparePrompt` | Session guard | `streamCoalescer.flushBoundary`, mutex `0x1f191c58fb80` |
| 543305: `streamCoalescer.add` through synchronous stream handler | Coalescer `emitMu` | Session guard `0x1f19176d6190` |
| 24040321: `runExplicitCancellationOwned` | Cancellation operation ownership | Same session guard |

The 30-second operation context cannot interrupt `sync.Mutex.Lock`.
The pending projection therefore remains active, and lifecycle cancellation is never reached.
The full `/tmp` filesystem is a separate observation, not the cause proven by these stacks.
No transcript or raw protocol capture is required for the reproduction.

## Requirement conformance and assumptions

This is an implementation violation of the active
[cancelled-turn requirement](../../specs/tasks/requirements/workflow-cancelled-turn-completion.md),
AC-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001.1 and .2.
The existing requirement already specifies bounded operation, ordered terminal frames,
single cancellation ownership, parked queues, and input-ready reconciliation.
No requirement is added or rewritten.

The independent [UI progress requirement](../../specs/ui/requirements/cancel-turn-progress.md),
AC-UI-CANCEL-TURN-PROGRESS-001.5 through .7, covers deduplication, timeout settlement, and phone parity.
The task system owns the repair because prompt admission and cancellation own the failed lifecycle boundary.
There are no unresolved product choices. No new pause semantics or forced runtime stop are proposed.

## Scope

### In scope

- Separate callback-producing preparation from guarded provider admission.
- Preserve reservation identity, generation ordering, acceptance transfer, and stale-event rejection.
- Bound cancellation-owned guard acquisitions and settle joined requests on failure.
- Add deterministic backend regressions and desktop/phone user-flow coverage.

### Out of scope

- Live-session recovery, process restart, disk cleanup, or mutation of the reported task.
- New UI copy, layout, navigation, controls, schema, public endpoints, or feature flags.
- Changes to lifecycle escalation durations, workflow completion policy, or automatic queue drain policy.
- An asynchronous event-bus rewrite, stream loss, independent publication goroutines, or removal of serialization.

## Technical approach

Follow the [task design](../../specs/tasks/system-design/workflow-cancelled-turn-completion.md).
Extend the internal executor/lifecycle callback path with final admission after stream preparation.
Retain logical reservation ownership while releasing the physical session guard.
Revalidate that ownership before generation allocation and provider dispatch.
Keep the acceptance callback's existing ownership transfer and guard release order.
For async model-switch startup, retain the accepted queue reservation through the
worker-return/initial-prompt gap. Block Send Now until provider acceptance promotes
that exact reservation to live; admission failure removes only the matching marker.

Add context-aware acquisition around cancellation-owned use of the existing session mutex.
Cover initial acquisition, reacquisition, joined explicit reconciliation, and queued source actions.
On deadline failure, clear only the matching operation and projection; perform no delayed mutation.

### Compatibility matrix

| Path | Identity and transport | Required behavior | Evidence / unsupported fallback |
| --- | --- | --- | --- |
| Queued ordinary prompt | Queue entry, incarnation, execution, generation; agentctl | Drain before admission; dispatch once | Deterministic service/lifecycle regression |
| Resumed or model-switch prompt | Resume attempt and replacement execution; agentctl | Cancelled preparation cannot dispatch; accepted execution survives pause | Existing resume tests plus new boundary cases |
| Lifecycle automation | Workflow-entry binding and reservation; agentctl | Revalidate recipient and route before dispatch | Existing lifecycle queue tests plus supersession case |
| Explicit / silent / peer / Send Now cancellation | Shared operation and captured turn | One owner; bounded wait; source policy retained | Mixed-source deadline tests |
| Structured providers | Shared lifecycle callback path | Preserve provider-neutral stream order | Real lifecycle with mock agent; no claim of live-provider certification |
| Runtime without required admission callback | Internal capability absent | Fail before unsafe dispatch | Executor capability test; no success fallback |
| Passthrough / steer | Existing distinct dispatch contracts | No unrelated semantic change | Run existing executor and orchestrator suites |

## Tests

The new test names below are implementation targets, not existing evidence.
Use channels or `testing/synctest` barriers rather than probabilistic sleeps.
Each failing test must terminate and release its fixtures after observing the defect.

| Test | Location | Criteria |
| --- | --- | --- |
| `TestPromptTask_QueuedStreamBoundaryDoesNotDeadlock` | `orchestrator/task_operations_stream_boundary_test.go` | Task 001.1: overlapping publication, dispatch, then cancel all complete |
| `TestPromptTask_PreparedDispatchRevalidatesOwnership` | Same file | Task 001.1: cancel, archive, replacement, and route change cannot admit stale work |
| `TestSessionManager_StreamBoundaryPrecedesAdmission` | `agent/runtime/lifecycle/session_stream_admission_test.go` | Task 001.1: real coalescer drain precedes callback and new generation |
| `TestQueuedDispatchAwaitingInitialAdmissionSurvivesWorkerCleanup` | `orchestrator/queued_dispatch_admission_test.go` | Task 001.1-.2: async model-switch cleanup preserves queue identity until acceptance |
| `TestPromptTask_ModelSwitchFallbackRejectsAdmissionAfterCompletedPause` | `orchestrator/task_operations_model_switch_cancellation_test.go` | A completed explicit pause invalidates delayed restart admission even after its operation is removed |
| `TestPromptTask_QueuedInPlaceModelSwitchRejectsAfterCompletedPause` | Same file | In-place continuation rejects under the session guard before it creates a successor turn |
| `TestLifecycleAdapter_RegistersInitialPromptAdmissionOnRestart` | `backendapp/adapters_acp_session_test.go` | The production adapter forwards both initial-prompt callback registrations to a real lifecycle manager |
| `TestCancelAgent_GuardDeadlineSettlesOperation` | `orchestrator/task_operations_cancellation_deadline_test.go` | Task 001.1; UI 001.5-.6: all waiters finish and pending clears |
| `TestCancelAgent_GuardDeadlineCannotMutateSuccessor` | Same file | Task 001.1-.2; UI 001.6: no delayed mutation, retry remains possible |

Wire at least one integration fixture through the real lifecycle stream coalescer and synchronous event bus.
A mock that skips `flushBoundary` cannot prove this lock cycle is removed.
The new admission test retains exact stream content, correlation, and generation isolation.
Existing prompt completion-barrier tests retain terminal ordering coverage; the end-to-end flow verifies accepted-process continuity.

## E2E tests

Add `tests/chat/queued-prompt-cancel.spec.ts` in `chromium` and
`tests/chat/mobile-queued-prompt-cancel.spec.ts` in `mobile-chrome`.
Queue a second prompt while the first produces streamed output; wait for second-turn acceptance, then pause it.
The provider emits a unique marker derived from that queued prompt before it waits for cancellation;
wait for the marker on the same persisted turn before clicking or tapping Pause. A persisted user
message and an open turn alone do not prove provider acceptance.
Assert pending clears, the captured turn closes, the session becomes input-ready, and queued work remains parked.
Send a follow-up and assert one delivery on the same healthy execution.
Map both flows to Task 001.1-.2 and UI 001.5-.7.

The deterministic Go regression proves the dangerous interleaving.
Browser tests prove the user flow; passing browser timing alone does not prove deadlock removal.
Reuse isolated mock fixtures. Extend a mock sequence only if existing commands cannot produce the necessary streamed/queued turns.
Any new mock behavior needs its own targeted Go test in the same work order.

The phone test uses the existing compact composer, taps Pause, checks a 44px target,
and verifies no document-level horizontal overflow. No rendered structure changes are planned, so no ASCII preview is required.
Existing progress navigation/reload tests remain compatibility evidence and run with the new scenarios.

## Work orders

- [x] [Task 01: Separate stream preparation from prompt admission](task-01-separate-prompt-admission.md)
- [x] [Task 02: Bound cancellation guard waits](task-02-bound-cancellation-waits.md)
- [x] [Task 03: Verify queued pause on desktop and phone](task-03-verify-queued-pause.md)

## Companion packages

[Turn cancellation responsiveness](../turn-cancellation-responsiveness/plan.md),
[resumed-turn cancellation](../resumed-turn-cancellation/plan.md), and
[ceiling replay cancellation](../ceiling-replay-cancellation-deadlock/plan.md) remain completed baselines.
Their results describe their own revisions. This package adds the missing pre-dispatch stream-boundary and guard-deadline coverage.
Do not rewrite their historical results or infer this regression was previously tested.

## Verification results

Implementation passed on 2026-09-28. The model-switch guard regression was observed failing
while the guard stayed held across provider restart, then passed after the guard release and
initial-prompt admission callback were added. The browser fixture now disables auto-run and
auto-merge until both queued messages are confirmed.

- go test ./internal/orchestrator -run '^(TestQueuedDispatchAwaitingInitialAdmissionSurvivesWorkerCleanup|TestPromptTaskHoldsQueuedDispatchGuardDuringInPlaceModelMutation|TestResumeAttempt_ModelSwitchFallbackCancellationBeforeInitialPromptAcceptance|TestResumeAttempt_ModelSwitchFallbackTransfersAcceptance|TestPromptTask_QueuedStreamBoundaryDoesNotDeadlock)$' -count=1 -timeout=180s: passed.
- go test -race ./internal/orchestrator ./internal/orchestrator/handlers -count=1 -timeout=600s: passed; orchestrator 430.509s, handlers 5.642s.
- go test -race ./internal/agent/runtime/lifecycle -run '^TestSessionManager_StreamBoundaryPrecedesAdmission$' -count=1 -timeout=120s: passed. The fixture uses the real lifecycle stream coalescer and synchronously publishes through the in-memory event bus.
- Earlier Task 01 race suite: go test -race ./internal/orchestrator ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle ./internal/backendapp -count=1 -timeout=600s: passed; orchestrator 337.177s, executor 3.506s, lifecycle 68.381s, backendapp 81.821s.
- Earlier targeted deadline/dispatch race: go test -race ./internal/orchestrator -run 'Test(Cancel|PromptTask_QueuedStreamBoundary|PromptTask_PreparedDispatch)' -count=10 -timeout=300s: passed in 77.146s.
- make -C apps/backend build: passed for the backend, mock agent, and runtime helper binaries.
- pnpm e2e:run --project chromium tests/chat/queued-prompt-cancel.spec.ts tests/chat/cancel-progress-task-switch.spec.ts: passed, 2 tests.
- pnpm e2e:run --project mobile-chrome tests/chat/mobile-queued-prompt-cancel.spec.ts tests/chat/mobile-cancel-progress-reload.spec.ts: passed, 2 tests.
- pnpm run typecheck: passed.
- Final documentation checks after implementation passed: catalog validation (321 decisions, 1221 specifications), specification-linter tests (36), all specification lint, delivery-coverage preflight (covered all three work orders), and git diff --check.

Design validation on 2026-09-28:

- `python3 scripts/list-docs.py validate`: passed, 321 decisions and 1221 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `.github/scripts/pr-docs.cjs` exported `validateCoverage`: passed for all three work orders
  against the planned `task_operations.go` change. This was a local preflight, not a published PR check.
- Relative Markdown file links in the new package and task design: passed.
- `git diff --check -- docs/specs docs/plans/queued-prompt-pause-deadlock`: passed.
- `git status --short -- docs/plans/queued-prompt-pause-deadlock`: the new package is present and untracked.

Requirements retain active status. The task design is current and the plan is complete.
All work orders are complete. The package remains unstaged and uncommitted.

## Review remediation verification (2026-09-28)

The review findings are closed. The production `lifecycleAdapter` forwards both initial-prompt
registration methods and has a compile-time assertion for the narrow interface. The adapter test
uses a real lifecycle manager. Prompt dispatch now carries the cancellation-pending revision from
the start of `promptTask`; restart and in-place model-switch admission reject a completed pause
before provider dispatch, generation advance, or successor-turn creation.

The browser fixture now proves provider acceptance on the queued turn. The mock provider emits a
unique marker with a line terminator so lifecycle buffering persists it before the cancellation
hold. The browser test enables auto-run only after the preceding streaming turn settles and asserts
that enabling it dispatched the queued head.

Final review validation passed:

- `go test ./internal/orchestrator ./internal/backendapp -run 'TestPromptTask_(ModelSwitchFallbackRejectsAdmissionAfterCompletedPause|QueuedInPlaceModelSwitchRejectsAfterCompletedPause)$|TestLifecycleAdapter_RegistersInitialPromptAdmissionOnRestart$' -count=1 -timeout=180s`.
- `go test -race ./internal/orchestrator ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle ./internal/backendapp -count=1 -timeout=600s`.
- `go test ./cmd/mock-agent/... -count=1 -timeout=180s`.
- `make -C apps/backend build` and `make -C apps/backend e2e-plugin-package`.
- `pnpm run typecheck`.
- `pnpm e2e:run --no-build --project chromium tests/chat/queued-prompt-cancel.spec.ts tests/chat/cancel-progress-task-switch.spec.ts`: 2 passed.
- `pnpm e2e:run --no-build --project mobile-chrome tests/chat/mobile-queued-prompt-cancel.spec.ts tests/chat/mobile-cancel-progress-reload.spec.ts`: 2 passed.
- Catalog validation (321 decisions, 1221 specifications), specification-linter tests (36), all specification lint, delivery-coverage preflight for all three work orders, `gofmt`, and `git diff --check`.

## Documentation impact

Internal design and delivery records change. Existing requirements remain authoritative.
Public docs need no update: this restores the existing control without changing its documented use or adding a recovery procedure.
No new ADR is needed; the local repair rationale is recorded in the task design.

## Risks

- Early admission can dispatch cancelled work; late guard release can reproduce the deadlock.
- Generation allocation before stream drain can relabel old output or reject valid terminal frames.
- Model-switch and failure callbacks can reintroduce synchronous re-entry outside the ordinary prompt path.
- Timeout cleanup must not outlive its ownership or erase a successor operation.
- The host `/tmp` is full. Use an owned writable temporary directory on a volume with capacity; never delete shared files.
- Rebase-time changes to the same admission callbacks require a fresh source and test comparison before implementation.

## PR #4034 fixup validation (2026-09-28)

The review fixes preserve the existing cancellation contract and add no new product behavior.
Admission rejection now runs cleanup separately from provider-delivery failure, and user history
records the new prompt only after provider acceptance. The cancellation-pending revision fences
both restart and in-place model-switch admission, including the interval after a pause operation
has completed and left the active registry. The concrete lifecycle adapter forwards both required
callback registrations.

Cancellation guard waits use FIFO, context-aware handoff instead of `TryLock` polling. Prompt-claim
rollback reacquires the retained guard and checks active-turn ownership before writes. Clarification
recovery stops if cancellation cannot reacquire the guard. The queue fast-path test now waits for
its mock provider and dispatch settlement; this avoids leaving its background worker outside the
test lifetime. Desktop/mobile tests wait for the provider-correlated marker on the open queued turn
before pausing. The queue-reorder helper waits for `contenteditable=true` before clicking; both
reorder cases passed with retries disabled.

Final verification passed:

- `go test -race ./internal/orchestrator ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle ./internal/backendapp -count=1 -timeout=600s`: all packages passed; orchestrator 411.827s, executor 3.667s, lifecycle 87.349s, backendapp 92.980s.
- Focused race regressions for completed-pause model-switch fencing, FIFO cancellation guard handoff, rollback successor safety, and clarification recovery.
- `go test -race ./internal/orchestrator -run '^TestQueueUserPrompt_T2DrainsWhenTaskAdmitted$' -count=1 -timeout=120s`: passed.
- `go test -race ./internal/agent/runtime/lifecycle -run '^TestDispatchInitialPromptAdmissionRejectionDoesNotFailExecutionOrPersistPrompt$' -count=1 -timeout=120s`: passed after tightening the test to traverse `dispatchInitialPrompt`.
- `go test ./internal/mcp/handlers -run '^TestMessageTaskReadiness_Delivery$' -count=1` and `go test ./cmd/mock-agent/... -count=1 -timeout=180s`: passed.
- `make -C apps/backend build` and `pnpm run typecheck`: passed.
- Managed Chromium queued-pause/task-switch and mobile-chrome queued-pause/reload flows: 2 tests passed per project. The queue-reorder Chromium spec passed 2 tests with retries disabled.
- Catalog validation (321 decisions, 1223 specifications), 36 specification-linter tests, all specification lint, delivery coverage for all three work orders, `gofmt`, Prettier, and `git diff --check`.

The PR checks that originally failed were reproduced and corrected: the MCP readiness fake now
implements the admission callback, and the queue-reorder helper waits for editor readiness before
clicking. No runtime restart or live-session mutation was performed.
