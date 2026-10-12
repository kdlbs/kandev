---
created: 2026-10-02
status: done
requirements:
  - REQ-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001
system_design:
  - ../../specs/platform/system-design/prompt-completion-ownership.md
legacy_specs: []
---

# Implementation plan: Dispatch completion and wakeup

## Overview

Repair [issue #4150](https://github.com/kdlbs/kandev/issues/4150) in one sequential work order.
The issue is assigned to `carlosflorencio`. Task 01 implemented the completion ownership fix and its lifecycle regressions.

## Evidence and root cause

Investigation used commit `d22fbaab1fcdf35add3272c0a6fcafebb4ed8e2b`.
The canonical issue has no comments or image attachments.
The reporter describes pi-acp notifications after a dispatch-only prompt, followed by a 30-second readiness wait and forced restart.
Those provider timing observations are reporter evidence, not a local replay.

Current source confirms the core sequence:

1. `finishAcceptedPrompt` sets `dispatchedPromptPending` after dispatch acknowledgement.
2. A numbered completion sets `promptCompletionGeneration` and Ready status but leaves the pending flag set.
3. A later message chunk changes Ready to Running through `recordActivity`.
4. `claimPromptCompletion` rejects the generation-zero completion because the historical pending flag remains set.

The temporary `TestIssue4150_DispatchCompletionThenWakeup` drove real lifecycle handlers and dispatch bookkeeping.
Both cases failed the expected Ready assertion:

| Event order | After numbered completion and acknowledgement | After autonomous completion |
| --- | --- | --- |
| Acknowledgement, then completion | `READY`, pending true | Completion rejected, `RUNNING`, pending true |
| Completion, then acknowledgement | `READY`, pending true | Completion rejected, `RUNNING`, pending true |

The second case proves that clearing the flag only on completion is insufficient.
The acknowledgement must not restore stale pending state.
The temporary test was removed after execution. No live provider or user instance was changed.

The ACP adapter's `isAsyncTurnContentEvent` accepts message chunks and schedules completion after a five-second idle window.
The backend's `recordActivity` treats ordinary message chunks as turn content.
This confirms the normalized trigger path without proving pi's raw metadata behavior locally.
The current predecessor wait is ten seconds; the reported 30 seconds belongs to the separate readiness recovery path.

## Requirement conformance

Platform owns shared foreground completion and runtime lifecycle safety.
The existing workflow-reset requirements cover cancellation and reset, not ordinary completion followed by autonomous work.
The new [requirement](../../specs/platform/requirements/prompt-completion-ownership.md) gives that missing outcome explicit acceptance criteria.
ADR 0035 already establishes generation identity and generation-zero wakeup compatibility; no new architectural decision is needed.

The assumption check found no material product choice requiring clarification.
The requested result is normal completion and subsequent input without a stale barrier or forced restart.

## Scope

### In scope

- Clear the completed generation's dispatch barrier after transcript processing and, for errors, after applying terminal state and delivering the completion signal.
- Prevent delayed dispatch acknowledgement from restoring that barrier.
- Apply numbered error or shutdown-stop state before waking successors; publish the captured terminal event after releasing the lifecycle lock.
- Preserve successor ownership, transcript ordering, cancellation, reset, and ordinary blocking prompts.
- Prove same-execution delivery after a dispatch-only prompt and autonomous completion.

### Out of scope

- pi `_meta.piAcp.notify` filtering; this is an optional trigger mitigation, not the shared root-cause repair.
- Issue #4149's recursive startup-lock deadlock or a general event-lock audit.
- Frontend rendering, state-projection changes, new timeouts, retries, or persistence.
- Persistent Kandev subtasks, commits, or a PR.

## Technical approach

Follow the [system design](../../specs/platform/system-design/prompt-completion-ownership.md).
Change `manager_events.go` completion finalization and `session.go` dispatch acknowledgement.
Use existing generation state and mutexes; capture terminal status, event identity, and evidence before unlocking.
Successful completion signals before ready publication. Error completion applies terminal state, signals, releases pending, unlocks, and only then publishes failure or shutdown-stop handling.
Do not clear pending at claim entry or acquire `promptMu` in the event handler.

### Compatibility matrix

| Provider/path | Transport and identity | Expected behavior | Evidence or limit |
| --- | --- | --- | --- |
| pi-acp, reported trigger | ACP through normalized agentctl events | Late autonomous completion can settle after a numbered dispatch | Handler reproduction; no live pi claim |
| Other structured providers | Numbered agentctl completion | Matching completion releases only its generation | Lifecycle and WebSocket mock tests |
| Autonomous ACP work | Generation zero | Reject during pending foreground work; accept after settlement | Wakeup and rejection tests |
| Blocking prompts and steering | Existing generation and completion consumer | Preserve completion ownership | Existing blocking and steering regressions |
| Missing or stale completion | Absent or mismatched identity | Keep current bounded wait and recovery | Pending-wait and cancel/reset tests |
| Passthrough | Separate PTY path | Unchanged | Outside the modified path |

## Tests

Criteria below belong to `AC-PLATFORM-PROMPT-COMPLETION-OWNERSHIP-001`.
The new test file is `apps/backend/internal/agent/runtime/lifecycle/manager_events_dispatch_completion_test.go`.

| Criteria | Regression |
| --- | --- |
| `.1`, `.3` | `TestDispatchCompletion_WakeupAfterNumberedCompletion`, both acknowledgement orders |
| `.3`, `.4` | `TestDispatchCompletion_AcknowledgementOwnership`, completed, successor, and replacement cases |
| `.4` | `TestDispatchCompletion_StaleAndDuplicateEvents`, including an older completed generation plus a live successor |
| `.5` | `TestDispatchCompletion_FinalizationBarrier`, delayed transcript flush and an already waiting successor |
| `.6` | `TestDispatchCompletion_ErrorReleasesOnlyMatchingGeneration`, `TestDispatchCompletion_UninitializedStartupFailureIsTerminal` |
| `.7` | `TestDispatchCompletion_TerminalPublicationUsesCapturedGeneration`, captured generation, turn, attempt, and failure evidence |
| `.3`, `.8` | `TestDispatchCompletion_ErrorBeforeAcknowledgementReleasesDispatchGuard`, synchronous failure subscriber and dispatch callback guard |
| `.4`, `.5`, `.9` | `TestDispatchCompletion_ErrorFinalizationRejectsWaitingSuccessor`, terminal-state barrier before successor admission |
| `.10` | `TestWaitForPendingDispatchedPrompt_TimesOutWithoutClearingGate` |
| `.11` | `TestManager_CancelAgent_DispatchCompletionTimeoutPreservesTransportFailure` |
| `.2`, `.5` | `TestDispatchCompletion_NextPromptUsesSameExecution` through real `Manager.PromptAgent` and the WebSocket mock |

Existing compatibility suites include `session_pending_prompt_test.go`, `manager_interaction_dispatch_cancel_test.go`,
`session_steer_test.go`, `wakeup_simulated_test.go`, and reset tests in `manager_interaction_test.go`.
The cancel fixture's completion-before-ack expectation was updated without removing its cancellation checks.

## E2E tests

Backend protocol integration is the appropriate end-to-end boundary for this internal correction.
The same-execution test observes two actual prompt requests at the mock server.
Between them, it delivers a numbered completion, late content, and generation-zero completion through lifecycle handlers.
It asserts unchanged execution/client identity, exactly one successor dispatch, no stop/restart, and no readiness delay.
Use synchronization barriers and bounded contexts, not arbitrary sleeps.
No Playwright test is needed because rendered UI and browser interactions do not change.

## Companion packages

Preserve the completed delivery records and behavior from:

- [Dispatch-only cancellation](../dispatch-only-cancel-completion/plan.md).
- [Idle context reset](../idle-context-reset-dispatch-gate/plan.md).
- [Lost completion cancellation recovery](../lost-turn-completion-cancel-recovery/plan.md).

Their historical results remain unchanged. This package changes normal completion bookkeeping and revalidates the affected compatibility tests.

## Work orders

- [x] [Task 01: Settle dispatch completion ownership](task-01-settle-dispatch-completion.md)

## Verification results

The temporary reproducer failed in both acknowledgement orders before implementation and was removed. The permanent regression suite covers the original completion ownership cases plus the two review interleavings.

- `go test ./internal/agent/runtime/lifecycle -run '^TestDispatchCompletion_' -count=1 -v`: passed.
- The work-order race command: passed for dispatch completion, cancellation, reset, steering, and wakeup compatibility.
- `make -C apps/backend build`: passed for the backend runtime and helper binaries. macOS helper targets reported that no signing tool is installed.
- `python3 scripts/list-docs.py validate`: passed; validated 340 decisions and 1,297 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed; all specification files passed.
- Documentation-coverage preflight: passed with status `covered` and no errors.
- `git diff --check`: passed.

Review remediation reproduced both findings before production changes. A synchronous failure subscriber now runs after the completion releases `promptLifecycleMu`, so the dispatch callback can release its orchestrator guard. Error finalization applies FAILED or shutdown STOPPED before signaling or clearing the pending barrier; terminal identity and failure evidence are captured before unlock and published afterward. A waiting successor therefore cannot reopen the failed execution.

The reproduction establishes the lifecycle defect, not the complete provider-specific 30-second restart sequence. Issue #4149's separate startup-lock race and pi notification filtering remain outside this correction; coordinate release with #4149.

After review remediation, `TestDispatchCompletion_ErrorBeforeAcknowledgementReleasesDispatchGuard` and `TestDispatchCompletion_ErrorFinalizationRejectsWaitingSuccessor` both passed alongside the full `TestDispatchCompletion_` group. The specified lifecycle compatibility suite passed with `-race`; `make -C apps/backend build`, specification validation, specification lint, documentation-coverage preflight, and `git diff --check` also passed. The build reported only the expected missing macOS signing-tool warnings.

PR review added a numbered error during uninitialized startup and a terminal-publication test that blocks mutable generation reads while checking captured identity and failure evidence. The startup regression failed before removing startup-owner deferral from numbered prompt errors. The requirement now separates terminal status ordering, immutable event identity, synchronous subscriber progress, failed-execution admission, bounded missing-completion waiting, and cancellation escalation into independently testable criteria. Generation-zero process-exit deferral and the #4149 release caveat remain intact.

Backend CI exposed a race in `TestHandleAgentEvent_ErrorClaimCannotRaceReplacementPrompt`: post-unlock error logs read `execution.Status` after a replacement prompt could set it to RUNNING. The terminal publication now captures the applied status with its payload, and the regression asserts that the error outcome remains FAILED after a newer prompt begins. The deterministic publication test and 20 race-enabled repetitions of the CI-failing test pass.

The complete lifecycle package also passes under `go test -race ./internal/agent/runtime/lifecycle -count=1` (130.320s) after the snapshot change.

## Risks

- Successor admission must stay behind transcript flushing and terminal failure application.
- Late acknowledgement can restore stale pending state unless it shares the completion ownership check.
- Publishing a failure while holding the lifecycle lease can deadlock against the orchestrator dispatch guard; terminal publication stays outside that lease.
- Cancellation fixtures encode the old flag lifetime; assertion changes must retain meaningful ownership coverage.
- Issue #4149 still affects valid generation-zero completions when a startup writer contends. Coordinate its repair before claiming complete recovery for that race.
- Mock evidence does not establish notification semantics for every provider.

## Public documentation

The docs-maintainer assessment requires no public-doc change for this design package.
The planned correction restores completion behavior without changing commands, APIs, configuration, or user procedures.
