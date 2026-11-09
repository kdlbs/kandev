---
id: "02-process-cause-propagation"
title: "Carry process causes through recovery"
status: completed
wave: 2
depends_on:
  - "01-safe-process-evidence"
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.24
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.26
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.28
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.29
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.36
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.1
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.2
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.5
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.7
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-004.8
system_design:
  - ../../specs/agents/system-design/startup-process-diagnostics.md
---

# Task 02: Carry process causes through recovery

## Summary

Convert validated pre-initialize exit evidence into the existing bootstrap cause
contract. Preserve that cause through durable failure projection and matching
recovery request feedback while keeping distinct attempt history intact.

## In scope

- Capture exact client/generation evidence at failed initialize, before mutable replacement state can change it.
- Add `process_exited`, its closed reasons, and optional exit status to `BootstrapFailure` and `AgentErrorCause`.
- Validate reason/status combinations, fixed safe detail, equality, deduplication, and byte budgets.
- Carry evidence through both asynchronous failure producers, task summaries, DTOs, events, and historical messages.
- Preserve normalized causes through `SessionRecoveryFailure` and its existing response identity where readiness produces a generic wrapper.
- Keep later failed recovery operations separate from the original failure.
- Preserve authentication, cancellation, deadlines, retry budget, and stale-event fences.

## Out of scope

Frontend translation and card changes, a new outer launch category, a new error
store, journal checkpoint repair, SQL schema changes, and recovery-policy changes.

## Acceptance

1. Initial startup and resume retain status and the recognized mise reason through
   bootstrap projection, durable reads, task summaries, and owned recovery responses.
2. Only matching pre-initialize evidence qualifies. Cancellation, deadlines,
   authentication, stale generations, intentional stops, signals, and later phases
   retain existing handling. Mise refusal cannot trigger automatic replacement.
3. Same-attempt wrappers preserve the typed cause. A later failure owns current
   controls and retains the earlier dated error in history, without copying its cause.

## TDD and regression cases

Start with `TestStartupProcessBootstrapCause`, using the real typed
`SessionInitializationPhaseError` and `InitializeError` path.
The baseline reproduction proved that ordinary exit evidence currently becomes unknown.
Test matching and mismatched generations through `SessionManager`, not only a
manually populated `BootstrapFailure`.

Add `TestStartupProcessRecoveryCauseSurvivesReadiness` through the real service
readiness/recovery path. Verify the returned safe causes and stored stamp refer
to the same attempt and execution. Cover missing metadata and stale stamp cases.

Add `TestStartupProcessDistinctRecoveryHistory`: a specific resume failure followed
by a distinct generic recovery failure preserves both dated records.
Verify that a healthy sibling and a later successful attempt remain unaffected.
Inject the second failure at the normal recovery boundary.
The checkpoint algorithm from #5129 remains outside this test.

Extend normalization tests for zero, absent status, negatives, fractional wire
values, overflow, unknown reasons, wrong code/reason combinations, arbitrary
detail, and status changes affecting semantic equality.
Audit typed copies rather than relying solely on untyped JSON round trips.

Rerun managed npm startup tests. Cover complete nonempty mise diagnostics,
genuinely empty stderr, final evidence after retry exhaustion, cancellation,
and delayed exit events from a replaced process.

## Verification

Run from the repository root after implementation:

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle ./internal/task/models ./internal/task/statussummary ./internal/task/dto ./internal/orchestrator/executor ./internal/orchestrator ./internal/orchestrator/handlers -run 'Test(StartupProcess|Bootstrap|InitializeFailurePhase|ManagedRuntimeStartup|HandleAgentStartFailed|SessionRecovery|RecoveryFailure|NormalizeAgentError|AgentError)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/startup_process_diagnostics.go apps/backend/internal/agent/runtime/lifecycle/startup_process_diagnostics_test.go apps/backend/internal/agent/runtime/lifecycle/session.go apps/backend/internal/agent/runtime/lifecycle/bootstrap_failure.go apps/backend/internal/agent/runtime/lifecycle/managed_runtime_startup.go apps/backend/internal/task/models/launch_errors.go apps/backend/internal/task/models/startup_process_diagnostics_test.go apps/backend/internal/orchestrator/executor/launch_failure.go apps/backend/internal/orchestrator/executor/executor_startup_process_test.go apps/backend/internal/orchestrator/event_handlers_agent.go apps/backend/internal/orchestrator/session_recovery_feedback.go apps/backend/internal/orchestrator/startup_process_diagnostics_test.go apps/backend/internal/orchestrator/handlers/handlers.go apps/backend/internal/orchestrator/handlers/startup_process_diagnostics_test.go
git diff --check
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

Keep the lint argument list aligned with this work order's final changed Go files.
Add audited DTO, summary, or readiness files if implementation changes them.
Record the exact final argument list in Results.
If an audited DTO or summary suite uses another test prefix, add that exact suite
to the focused command before completing this work order.

## Files likely touched

- New `apps/backend/internal/agent/runtime/lifecycle/startup_process_diagnostics.go` and `startup_process_diagnostics_test.go`.
- `apps/backend/internal/agent/runtime/lifecycle/session.go`, `bootstrap_failure.go`, and `managed_runtime_startup.go` only at their existing evidence boundaries.
- `apps/backend/internal/task/models/launch_errors.go` and a focused new `startup_process_diagnostics_test.go`.
- `apps/backend/internal/orchestrator/executor/launch_failure.go` and a focused new `executor_startup_process_test.go`.
- `apps/backend/internal/orchestrator/event_handlers_agent.go` and `session_recovery_feedback.go`.
- `apps/backend/internal/orchestrator/task_operations.go` only if its readiness boundary needs captured identity wiring.
- New `apps/backend/internal/orchestrator/startup_process_diagnostics_test.go`.
- `apps/backend/internal/orchestrator/handlers/handlers.go` and a focused `startup_process_diagnostics_test.go` for safe recovery response details.
- `apps/backend/internal/task/statussummary/projector_helpers.go`, DTO/API typed copies, and their current tests only where the field audit identifies a loss.

## Dependencies

[Task 01](task-01-safe-process-evidence.md) provides the source evidence and optional wire fields.

## Risks

Mutable client identity can attribute an old exit to a replacement.
Generic readiness errors can select unrelated metadata if correlation is weak.
Normalize safe fields and require the existing session/attempt/execution/stamp
association before enrichment. Preserve all atomic failure commit guards.

## Parallelism

`sequential`

## Inputs

- [Design: classification](../../specs/agents/system-design/startup-process-diagnostics.md#classification-and-retry-compatibility),
  [projection](../../specs/agents/system-design/startup-process-diagnostics.md#safe-projection-and-persistence),
  and [attempt isolation](../../specs/agents/system-design/startup-process-diagnostics.md#distinct-recovery-attempts).
- [Recovery requirements](../../specs/agents/requirements/session-recovery-failures.md) and
  [managed startup requirements](../../specs/agents/requirements/managed-npm-runtime-recovery.md).
- Existing `session_startup_evidence_test.go`, bootstrap selection tests,
  `session_cleanup_failure_test.go`, `task_operations_resume_cancellation_test.go`,
  and managed-runtime classification tests.
- Read scoped backend guidance and `/tdd` before permanent changes.

## Results

Implemented generation-bound process-exit classification, the `process_exited` cause with optional status, fixed detail normalization, and same-attempt cause enrichment through `SessionRecoveryFailure` and the recovery response details. Added `TestStartupProcessBootstrapCause`, `TestStartupProcessBootstrapFailurePrecedence`, `TestStartupProcessRecoveryCauseSurvivesReadiness`, `TestStartupProcessDistinctRecoveryHistory`, and the models, executor, DTO, and handlers regressions.

Verification passed:

- `(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle ./internal/task/models ./internal/task/statussummary ./internal/task/dto ./internal/orchestrator/executor ./internal/orchestrator ./internal/orchestrator/handlers -run 'Test(StartupProcess|Bootstrap|InitializeFailurePhase|ManagedRuntimeStartup|HandleAgentStartFailed|SessionRecovery|RecoveryFailure|NormalizeAgentError|AgentError)' -count=1)`
- Full non-race suites for every changed backend package
- `bash scripts/lint-go-changed` for the final changed-file list
- `git diff --check`, `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`

Review remediation: `LastAgentError` now keeps explicit JSON null and missing `exit_code` nil while preserving numeric zero, with round-trip and initialize-classification regressions. Added wire-decoding tests through the real managed-runtime retry classifier for malformed npm fields.
