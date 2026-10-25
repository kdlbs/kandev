---
created: 2026-09-12
status: done
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
legacy_specs: []
---

# Implementation Plan: Workflow reset failure containment

## Overview

[Issue 3358](https://github.com/kdlbs/kandev/issues/3358) reports an escalated cancellation followed by a context reset that holds the session guard.
The residual cancellation defect is reproducible at `49df3794b`.
GitHub's current `main` also resolved to that commit during investigation.
The fix stops unsafe reset admission, bounds unanswered reset requests, and makes failed workflow entry visible.

The issue is assigned to `carlosflorencio`.
Tasks 01-04 are implemented and verified.

## Evidence and confidence

| Finding | Evidence | Assessment |
| --- | --- | --- |
| Escalation permits provider reset | Temporary `TestIssue3358ReproEscalationMustStopReset` calls the real orchestrator with an active durable turn and `ErrCancelEscalated`. It fails because reset returns success. | Confirmed residual defect |
| Ordinary cancellation failure blocks reset | Existing `TestResetAgentContext_CancelFailureStopsProviderReset` passes. | Control passes |
| Confirmed cancellation permits reset | Existing `TestResetAgentContext_QuiescesActiveTurnBeforeProviderReset` passes. | Control passes |
| Reset uses an unbounded response wait | `ResetSession -> createSessionRequest -> sendStreamRequest` supplies no deadline. The WebSocket wait ends only on reply, disconnect, or caller cancellation. | Confirmed source path |
| Caller cancellation releases the response wait | Temporary `TestIssue3358ReproResetWaitsForCallerCancellation` passes against a silent WebSocket peer. It receives the request, waits 100 ms, then cancels and observes release. | Transport characterization, not a hours-long hang reproduction |
| Startup ordering is already repaired | `Service.Start` starts watcher and scheduler before `startLifecycleSweepAsync`. Recovery no longer blocks readiness. | Source confirmed |
| Startup ordering is covered by the existing regression | `TestStartDoesNotBlockOnLifecycleSweep` passes with the current startup implementation. | Verified separately; no startup change was needed |

The escalation reproduction failed again when run alone.
Confidence is high for the residual reset correction.
The reporter's precise provider-side hang has no goroutine dump and was not reproduced with Claude ACP in this investigation.

The original report and all three comments were read.
The reported startup outage is already addressed by the current startup implementation, including the change in PR 2944.
The optional step re-entry behavior is not required to contain the reset defect.

Review remediation evidence is complete. The ACP adapter regression proves that
`session/new` returns before a blocked best-effort `session/close`, while a
delayed provider response after the manager deadline remains fenced and its
late setup event is discarded. The workflow barrier regression proves that a
successor admission and session deletion wait until reset-failure persistence
and publication finish. Backend and browser coverage also verifies a distinct
step prompt, prompt-call and user-message counts, and the persisted reset cause
in the expanded desktop and phone notices.

## Requirement conformance

The current implementation violates `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.1`: local reconciliation is not provider quiescence.
Retain `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.4`, which forbids automatic prompting after reset failure.
The issue's suggestion to continue prompting after failure conflicts with that contract.
This package uses the issue's alternative of a visible error.

The owning [requirements](../../specs/tasks/requirements/workflow-step-agent-start-ownership.md) add criteria `.7`, `.8`, and `.9`.
They cover escalation, unanswered reset requests, and persistent recovery feedback.
The [system design](../../specs/tasks/system-design/workflow-step-agent-start-ownership.md#reset-failure-containment) records the implementation boundary.

The existing [quiescence ADR](../../decisions/2026-08-30-context-reset-quiesces-active-turn.md) remains applicable.
No new ADR is needed: the design records the local error-policy choice without creating a new architectural owner.

## Scope

### In scope

- Retain provider cancellation outcome separately from successful local reconciliation.
- Reject escalated cancellation only at the workflow reset admission boundary.
- Give the provider reset request a 10-second child deadline.
- Return reset timeout or caller cancellation without automatic restart fallback.
- Preserve ordinary unsupported-reset fallback and runtime configuration restoration.
- Persist reset failure through the existing error notice and preserve deletion access after cleanup.

### Out of scope

- Startup recovery scheduling changes or readiness changes.
- Repair of the unrelated startup test fixture.
- Automatic prompting after a failed reset.
- Automatic restart after escalated cancellation.
- Changing manual step re-entry eligibility for settled sessions.
- A new recovery button, API, runtime flag, or database migration.
- General cancellation of waits on shared mutexes or all provider RPCs.
- Retrospective repair of a reset goroutine already stuck in an older backend.

## Technical approach

### 1. Provider cancellation outcome

Change the coordinator in `task_operations.go` and `event_handlers_clarification.go`.
Capture the raw result before tolerated sentinels become reconciliation success.
Keep it on the operation that owns the captured execution and turn.
Read it through the exclusive reset helper after completion.
Do not change the common operation error for explicit cancellation or joined callers.

`quiesceActiveResetTurn` rejects escalation.
The existing guards, reset marker, silent reconciliation, and missing-execution cleanup remain authoritative.

### 2. Reset request timeout

Change `Manager.ResetAgentContext` in `manager_interaction.go`.
Use `context.WithTimeout` around `client.ResetSession`, with a 10-second internal constant.
Classify deadline expiry and caller cancellation before the ordinary fallback branch.
Release the client lease and return an error through normal deferred cleanup.

A timeout does not prove that the remote operation stopped.
Do not reuse the same stream for a speculative restart or treat a late reply as successful reset.
Preserve the current unsupported-reset fallback.
Test actual request dispatch before measuring timeout; independent mutex contention is outside this request-response bound.

### 3. Visible workflow failure

Add an error-bearing reset helper without forcing unrelated callers to change.
Use `persistLastAgentError` directly from workflow failure handling.
Do not route the failure through `handleAgentFailed`, which can dispatch workflow actions.

Persist safe reset-specific text, publish it, and reload session metadata before the waiting-state event.
Retain the automatic-prompt early return and omit the successful reset divider.
The existing desktop and phone error notice provides the presentation.

## ASCII UI preview

UI-01: Conversation after a workflow reset failure.
The desktop chat and phone conversation share this notice composition.
The phone uses its existing focused conversation and scroll owner.

Before:

```text
Destination step
Conversation history
[Composer]
```

After:

```text
Destination step
Conversation history
+--------------------------------------+
| Previous agent error          [Hide] |
| Context reset failed.                |
| The workflow step prompt did not     |
| start.                               |
+--------------------------------------+
[Composer]
```

The hierarchy is required; wording is illustrative and must follow localization rules.
Reuse `LastAgentErrorNotice`, its wrapping, its alert semantics, and its existing dismissal control.
Increase only the coarse-pointer dismissal target from 32 pixels to at least 44 pixels.
No hover is needed to read the phone error.
Tests must prove visibility after reload and no horizontal document overflow.

## Tests

| Criteria | Test file and test |
| --- | --- |
| `.1`, `.2`, `.4`, `.7` | `event_handlers_workflow_reset_quiescence_test.go::TestResetAgentContext_EscalatedCancelStopsProviderReset` |
| `.2`, `.6`, `.7` | Same file: escalation with a late completion, a joined explicit caller, and missing execution cases |
| `.8` | `manager_interaction_reset_timeout_test.go::TestManager_ResetAgentContext_ResetRequestTimeout` and `TestManager_ResetAgentContext_CallerDeadlinePrecedesRequestTimeout` |
| `.3`, `.6`, `.8` | Same file and agentctl tests: caller cancellation, abandoned reply, terminal cleanup, normal fast reset, unsupported fallback |
| `.4`, `.9` | `event_handlers_workflow_reset_failure_test.go::TestProcessOnEnter_ResetFailurePersistsNotice` |
| `.4`, `.7`, `.9` | `internal/integration/workflow_context_reset_failure_test.go::TestWorkflowResetEscalationLeavesSessionDeletable` |
| `.3`, `.6`, `.8` | `adapter_session_test.go::TestResetSessionSerializesConcurrentLoadDuringClose`; `manager_interaction_reset_timeout_test.go::TestManager_ResetAgentContext_DelayedProviderSessionIsFenced` |
| `.4`, `.7`, `.9` | `event_handlers_workflow_reset_failure_test.go::TestProcessOnEnter_ResetFailureSettlementOwnsSuccessorAdmission` and `TestProcessOnEnter_SuccessfulResetDispatchesAutoStartPrompt` |

The integration test follows `cancel_after_crash_test.go`.
Drive workflow entry against an agent simulator that returns escalation.
Assert no provider reset, no automatic step prompt or user message, a persisted
cause, and successful `session.delete`. The unit fixture also covers provider
timeout and a successful reset that dispatches the distinct step prompt.
This is backend end-to-end evidence; it does not substitute for notice rendering.

## E2E tests

- Desktop: extend `apps/web/e2e/tests/session/agent-error-indicator.spec.ts` with `workflow reset failure remains visible after reload`.
- Phone: add `apps/web/e2e/tests/session/mobile-workflow-reset-error.spec.ts` for the same notice, reload, dismissal, and viewport containment.
- Reuse the existing error-metadata fixture. Backend integration proves production emission; seeded rendering tests prove the existing UI contract.
- Map both scenarios to `AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.9`.
- Projects: `chromium` and `mobile-chrome`.

## Companion packages

The quiescence, idle-dispatch-gate, and runtime-configuration packages are already implemented.
Their historical test results remain historical evidence, not results of this package.
Their completed work orders do not need reopening.
The new work orders retain targeted compatibility checks for their contracts.

- [Workflow context reset quiescence](../workflow-context-reset-quiescence/plan.md)
- [Idle reset dispatch gate](../idle-context-reset-dispatch-gate/plan.md)
- [Reset runtime configuration](../acp-context-reset-runtime-configuration/plan.md)

## Work orders

Execute sequentially in this session after an explicit implementation request.

- [x] [Task 01: Preserve reset cancellation outcome](task-01-reset-cancellation-outcome.md)
- [x] [Task 02: Bound provider reset requests](task-02-provider-reset-timeout.md)
- [x] [Task 03: Surface workflow reset failures](task-03-workflow-reset-feedback.md)
- [x] [Task 04: Verify reset recovery surfaces](task-04-reset-recovery-surfaces.md)

## Verification results

Investigation commands:

```bash
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run '^TestIssue3358ReproEscalationMustStopReset$' -count=1 -v)
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/agentctl -run '^TestIssue3358ReproResetWaitsForCallerCancellation$' -count=1 -v)
```

The first command failed at the expected reset-admission assertion.
The second command passed.
Both diagnostic files are temporary and are removed before handoff.
The first combined diagnostic run also passed both control tests. The existing
startup regression was rerun after implementation and passed.

Permanent implementation verification is recorded in the completed work
orders. The final checks include backend package tests and build, frontend
unit tests, typecheck, desktop and phone E2E, i18n validation, public-docs
validation, specification validation, formatting, and diff checks.

Backend verification passed:

```text
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/orchestrator ./internal/integration)
(cd apps/backend && make lint)
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agentctl/server/adapter/transport/acp ./internal/orchestrator ./internal/integration -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle ./internal/agentctl/server/adapter/transport/acp -run 'TestManager_ResetAgentContext_|TestResetSession' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run 'TestProcessOnEnter_(ResetFailure|SuccessfulReset)' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/integration -run 'TestWorkflowResetEscalationLeavesSessionDeletable' -count=1)
(cd apps/backend && go test -race -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestManager_ResetAgentContext_|TestManager_RestartAgentProcess_|TestWaitForPendingDispatchedPrompt_' -count=1)
(cd apps/backend && go test -race -tags fts5 ./internal/agent/runtime/agentctl -run 'TestResetSession_|TestSendStreamRequest' -count=1)
(cd apps/backend && go test -race -tags fts5 ./internal/orchestrator -run 'TestProcessOnEnterResetAgentContext|TestProcessOnEnter_ResetFailure|TestResetAgentContext_' -count=1)
(cd apps/backend && go test -race -tags fts5 ./internal/integration -run 'TestWorkflowResetEscalationLeavesSessionDeletable|TestOrchestratorCancelAfterAgentCrash_UnstiksSession|TestOrchestratorCancelWhenAgentHangs_UnsticksSession' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run '^TestStartDoesNotBlockOnLifecycleSweep$' -count=1 -v)
make -C apps/backend build
```

Frontend and documentation verification passed:

```text
(cd apps/web && pnpm exec vitest run components/task/chat/message-list-shared.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web build:vite)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/session/agent-error-indicator.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-workflow-reset-error.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

The focused ACP, lifecycle, orchestrator, and integration suites passed after
the review fixes. Desktop and phone E2E both passed with the technical-details
disclosure expanded; the phone test measured zero document overflow while the
details were visible and a 44-pixel dismiss target. Backend lint, typecheck,
Vite build, i18n checks, documentation validation, and specification lint also
passed.
Documentation validation passed:

- `python3 scripts/list-docs.py validate`: 264 decisions and 818 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check`: passed for tracked changes.
- Package reference and whitespace checks: passed for all five plan files, including untracked work orders.

Production code, permanent tests, frontend E2E coverage, and public guidance
changed. No PR was opened.

## Risks

- Changing the shared cancellation error can break explicit or joined cancellation. Preserve a separate provider outcome.
- An expired request does not cancel the remote provider operation. Block automatic prompting and ignore abandoned responses.
- Ordinary unsupported-reset fallback retains its current initialization behavior. This package does not bound every restart phase.
- A deadline cannot cancel mutex acquisition. The request-response guarantee starts after the peer receives the request.
- Stale waiting-state metadata can erase a newly published error. Reload after persistence.
- Startup recovery remains outside this change; the existing startup regression passes without a production startup modification.
