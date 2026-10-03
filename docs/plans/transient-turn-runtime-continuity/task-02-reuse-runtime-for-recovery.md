---
id: "02-reuse-runtime-for-recovery"
title: "Reuse runtimes for recovery"
status: done
wave: 2
depends_on:
  - "01-retain-failed-turn-runtime"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-TURN-CONTINUITY-001
  - REQ-PLATFORM-TURN-CONTINUITY-002
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-002
acceptance_criteria:
  - AC-PLATFORM-TURN-CONTINUITY-001.2
  - AC-PLATFORM-TURN-CONTINUITY-001.4
  - AC-PLATFORM-TURN-CONTINUITY-001.5
  - AC-PLATFORM-TURN-CONTINUITY-001.6
  - AC-PLATFORM-TURN-CONTINUITY-001.7
  - AC-PLATFORM-TURN-CONTINUITY-001.8
  - AC-PLATFORM-TURN-CONTINUITY-002.1
  - AC-PLATFORM-TURN-CONTINUITY-002.3
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.11
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.2
system_design:
  - ../../specs/platform/system-design/transient-turn-runtime-continuity.md
  - ../../specs/platform/system-design/provider-error-recovery.md
  - ../../specs/platform/system-design/provider-interruption-continuation.md
---

# Task 02: Reuse runtimes for recovery

## Summary

Use the retained runtime for eligible automatic replay and enabled safe Cursor continuation.
Preserve the existing recovery owner, budget, evidence fence, and queue priority.
Cancellation and exhaustion return to usable Chat when the runtime remains alive.

## In scope

- Extend the existing transient retry owner with immutable execution and retained-runtime evidence.
- Recheck configuration, native identity, generation, foreground admission, cancellation, and current runtime usability at timer dispatch.
- Dispatch an eligible replay through ordinary prompt admission without predecessor cleanup or session restoration.
- Keep existing five-attempt timing and acceptance accounting; never repeat an ambiguous accepted attempt.
- Add the live-runtime branch to `retryInterruptedContinuation` before its current stop/restore path.
- Preserve Cursor-only continuation support, the experimental toggle, existing instruction, effect ledger, and permission settings.
- Keep authoritative disconnect and process-loss restoration, plus restart-interrupted notice retirement.
- Cancel idle automatic work without stopping the usable process; dispatched cancellation retains existing escalation policy.
- On refusal, exhaustion, or idle cancellation, settle the turn and persist truthful attempt metadata through the work-order-01 path.
- Retain explicit teardown authority, Auto-run semantics, queued-user-input priority, and duplicate/stale owner checks.

## Out of scope

- A second retry loop, larger budgets, automatic model changes, Codex continuation after output, or replay after writes.
- New feature flags or promotion of existing continuation defaults.
- Historical UI models, localization, and browser fixtures, owned by work order 03.
- Policy changes for Office, dynamic, utility, passthrough, or automation-owned execution contexts.

## Acceptance

1. Eligible replay and enabled safe Cursor continuation use the exact retained runtime.
   Tests prove no stop, initialize, new, load, or resume call while current usability remains established.
2. Existing unsafe-work refusals, budgets, queue priority, cancellation ownership, and stale-generation fences still pass.
   A real runtime loss follows existing terminal recovery or native restoration without duplicating accepted work.
3. Idle cancellation or exhausted attempts preserve ACP and allow manual follow-up after settlement.
   Retry messages reflect actual started attempts; refusal before dispatch cannot become exhaustion.

## Verification

Add failing service tests for retained replay and retained Cursor continuation before changing dispatch.
Run the following block from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run 'Transient|Continuation|Interruption|TurnFailure|CapacityAfterTools' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle ./internal/agentctl/server/adapter/transport/acp -run 'Continuation|TurnFailure|TransientTurnFailure|CodexCapacity' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Create `event_handlers_transient_retained_runtime_test.go` and `provider_interruption_retained_runtime_test.go` with the manifest's planned names.
Use barrier-controlled races for cancel versus timer, process exit versus dispatch, duplicate outcomes, successor prompts, and queued user work.
Assert execution/native identity, physical process reuse, ordinary prompt admission, and zero restoration calls.
Run existing replay-safety, ordering, transport-loss, interruption budget, ownership, managed-input, refusal, and restart cases.
Cover both runtime-retained and runtime-lost branches instead of replacing restoration expectations wholesale.
Keep tests with the continuation toggle disabled and enabled; do not change registry/profile defaults.
The full browser sequence belongs to work order 03.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_transient.go` and new retained-runtime tests.
- The dedicated turn-failure handler introduced by work order 01.
- `apps/backend/internal/orchestrator/provider_interruption_dispatch.go`, `provider_interruption_preparation.go`, and shared admission helpers.
- `provider_interruption_continuation.go`, `provider_interruption_failure.go`, `provider_interruption_cancel.go`, and `provider_interruption_notice.go`.
- Existing transient resolution, replay-safety, ordering, and transport-loss tests.
- Existing interruption continuation, budget, ownership, phase, queue, refusal, managed-input, and restart tests.
- Runtime foreground-admission helpers only where needed to reuse the existing prompt seam.

## Dependencies

[Task 01](task-01-retain-failed-turn-runtime.md): retention evidence, failure owner, caller deduplication, and durable settlement must already pass.

## Risks

- Timer-time liveness may differ from failure-time evidence. Recheck the current runtime and stop stale owners before dispatch.
- Shared retry metadata already distinguishes replay and continuation; avoid a competing owner or reset budget.
- A continuation can reach ACP before an acknowledgment is lost. Dispatch acceptance must remain conservative.
- Cancelling recovery differs from explicit session stop; preserve the existing cancellation escalation boundary.
- Main's completed continuation plan records restoration behavior. Preserve its real-loss cases while adding live-runtime coverage in this package.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/transient-turn-runtime-continuity.md): runtime reuse, existing safety, cancellation, and truthful attempt feedback.
- [Design](../../specs/platform/system-design/transient-turn-runtime-continuity.md): recovery admission and ownership.
- [Decision](../../decisions/2026-10-03-transient-turn-runtime-lifetime.md).
- Existing [provider error recovery design](../../specs/platform/system-design/provider-error-recovery.md).
- Existing [interruption continuation design](../../specs/platform/system-design/provider-interruption-continuation.md) and completed `docs/plans/provider-interruption-continuation/` package.

## Results

Completed in the primary session. The retained-runtime replay and Cursor continuation paths passed the targeted race-instrumented checks:

- `(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run 'Transient|Continuation|Interruption|TurnFailure|CapacityAfterTools' -count=1)`: passed.
- `(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle ./internal/agentctl/server/adapter/transport/acp -run 'Continuation|TurnFailure|TransientTurnFailure|CodexCapacity' -count=1)`: both packages passed.
- `(cd apps/backend && go test -trimpath -race ./internal/orchestrator/watcher -count=1)`: passed.
- Task 01 lifecycle, watcher, and orchestrator regression block passed after integration; the agentctl/ACP, process, and instance race block also passed.
- `(cd apps/backend && go test -trimpath -race ./cmd/mock-agent -count=1)`: passed with the integrated fixture checks.

Review follow-up added exact-owner finalization and an error-aware liveness decision. A stale finalizer now retires only its own retry entry under notice serialization; the barrier regression covers both cancellation and refusal while preserving a successor entry, timer, cached prompt, and notice. Replay and continuation treat probe errors as inconclusive and retain the runtime; confirmed absence still takes the existing loss fallback.

Additional verification passed:

- `go test -trimpath -race ./internal/orchestrator -run 'Test(RetainedRetryFinalizerCannotRetireSuccessorOwner|RetainedRuntime(StatusProbeErrorBlocksReplayAndContinuation|ConfirmedAbsenceUsesExistingReplayFallback)|AgentTurnFailedRoutesAutomationOriginsToTerminalFailureOwner|AgentTurnFailedSettlesDurableErrorWithoutStoppingRuntime)' -count=1`: passed.
- `go test -trimpath -race ./internal/orchestrator -run 'Transient|Continuation|Interruption|TurnFailure|CapacityAfterTools' -count=1`: passed.
- `go test -trimpath -race ./internal/agent/runtime/lifecycle ./internal/agentctl/server/adapter/transport/acp -run 'Continuation|TurnFailure|TransientTurnFailure|CodexCapacity' -count=1`: both packages passed.
- `go test -trimpath -race ./cmd/mock-agent ./internal/orchestrator/executor -count=1`: both packages passed.
- `make -C apps/backend build`: passed for host binaries and configured cross-build targets.

Final catalog, specification, documentation-coverage, and whitespace validation results are recorded in the manifest after work order 03.
