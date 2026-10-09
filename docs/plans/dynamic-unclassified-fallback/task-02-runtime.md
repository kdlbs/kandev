---
id: "02-runtime"
title: "Route repeated safe unknown failures"
status: done
wave: 2
depends_on:
  - 01-configuration
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-002
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.3
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.4
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.5
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.6
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.7
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.8
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.9
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.11
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-002.12
system_design:
  - ../../specs/agents/system-design/dynamic-unclassified-fallback.md
---
# Task 02: Route repeated safe unknown failures

## Summary

Implement the exception through trusted failure evidence and durable route transitions. Preserve all existing classified-error and effect-safety behavior.

## In scope

- Write `TestUnclassifiedFallbackThreshold` first with three distinct current attempts and two candidates.
- Add a typed, default-deny context through the resolver, conductor, and engine. Keep global classifier flags unchanged.
- Capture startup attempt identity before launch. Admit only agent startup failures from a trusted producer, not generic task launch errors.
- Carry complete diagnostic identity before sanitizer loss. Reject missing attestation, redaction, truncation, empty text, and oversized text.
- Persist bounded streak state. Preserve it for explicit same-candidate retries and reset it at all design boundaries.
- Feed successful completion, output, and tool effects into current-attempt reset handling. Do not reset a prompt streak merely because process launch succeeded.
- Apply workflow veto and positive task-scope checks to synchronous and asynchronous failure paths.
- Test duplicate delivery, concurrent handlers, restart, currentness, cancellation, step changes, manual retry repair, continuation persistence failures, and exhausted candidate order.
- Use existing `dynamic_evidence_test.go`, `dynamic_policy_recovery_race_test.go`, and `dynamic_route_concurrency_test.go` harnesses. Add the test names listed in the plan.

## Out of scope

Automatic unknown-error retry loops, shared health changes, Office/utility enablement, and new visual settings controls.

## Acceptance

- The third matching safe failure selects one successor. Earlier failures stop without timers, and unsafe/mixed failures never accumulate toward fallback.
- Persisted counts survive restart and same-candidate manual retry. New profile/step/candidate, success, effects, stop, or veto clear them.
- Real producer-to-consumer tests prove startup and prompt admission. Office, utility, unknown scope, and raw launch wrappers remain excluded.

## Verification

Run from the repository root. New tests must fail for the intended missing behavior before production edits.

```bash
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/... ./internal/orchestrator ./internal/backendapp ./internal/task/repository/sqlite)
(cd apps/backend && go test -tags fts5 -race ./internal/agent/runtime/dynamic ./internal/orchestrator ./internal/backendapp ./internal/task/repository/sqlite -run 'Unclassified|Dynamic.*(Race|Conflict)|DynamicRoute.*Concurrent')
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/dynamic/types.go`
- `apps/backend/internal/agent/runtime/dynamic/engine.go`
- `apps/backend/internal/agent/runtime/dynamic/conductor.go`
- `apps/backend/internal/agent/runtime/dynamic/unclassified_fallback_test.go`
- `apps/backend/internal/agent/runtime/routingpolicy/policy.go`
- `apps/backend/internal/agent/runtime/dynamic_resolver.go`
- `apps/backend/internal/agent/runtime/lifecycle/`
- `apps/backend/internal/agent/runtime/routingerr/startup_error.go`
- `apps/backend/internal/orchestrator/dynamic_evidence.go`
- `apps/backend/internal/orchestrator/dynamic_launch.go`
- `apps/backend/internal/orchestrator/dynamic_policy_recovery.go`
- `apps/backend/internal/orchestrator/dynamic_unclassified_fallback_test.go`
- `apps/backend/internal/backendapp/dynamic_routing.go`
- `apps/backend/internal/backendapp/dynamic_routing_test.go`
- `apps/backend/internal/task/repository/sqlite/dynamic_route.go`
- `apps/backend/internal/task/repository/sqlite/dynamic_route_concurrency_test.go`

## Dependencies

Task 01 must pass first.

## Risks

The existing eligibility flag is not independent effect evidence. Generation checks alone do not deduplicate failures within one generation.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/dynamic-agent-routing.md), `REQ-AGENTS-DYNAMIC-AGENT-ROUTING-002`.
- [System design](../../specs/agents/system-design/dynamic-unclassified-fallback.md), all sections.
- [Package evidence and test matrix](plan.md).
- [Decision](../../decisions/2026-09-28-repeated-unclassified-fallback.md).

## Results

Passed:

- `(cd apps/backend && go test ./internal/agent/runtime/dynamic ./internal/agent/runtime/routingerr ./internal/task/repository/sqlite ./internal/orchestrator -run 'TestAgentStartupFailure|TestUnclassified|TestStopSessionClearsUnclassifiedStreak' -count=1)`
- `(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/... ./internal/orchestrator ./internal/backendapp ./internal/task/repository/sqlite)`
- `(cd apps/backend && go test -tags fts5 -race ./internal/agent/runtime/dynamic ./internal/orchestrator ./internal/backendapp ./internal/task/repository/sqlite -run 'Unclassified|Dynamic.*(Race|Conflict)|DynamicRoute.*Concurrent')`
- `git diff --check`

The runtime path now admits only attested terminal diagnostics and typed lifecycle startup failures. Persisted streaks survive same-candidate manual retries and restart, while duplicate events, changed identities, unsafe effects, workflow vetoes, successful activity, and explicit stop clear or refuse the count. The global classifier flags remain unchanged.

Review remediation passed:

- `(cd apps/backend && go test ./internal/agent/runtime/dynamic ./internal/task/repository/sqlite ./internal/orchestrator -run '^(TestRecordRouteDecisionClaimsInitialGenerationAndWritesAttemptAtomically|TestUnclassifiedWorkflowContextFencedAtRouteClaim|TestUnclassifiedWorkflowStepRevisionFencedAtRouteClaim|TestUnclassifiedWorkflowContextFencedBeforeDetachedLaunch|TestDynamicUnclassifiedStreakResetsAfterClassifiedNonFallbackFailure|TestHandleAgentFailedManualDynamicRecoveryRunsTerminalCleanup|TestPersistPendingDynamicRecoveryLeavesTurnForTerminalFailureCleanup)$' -count=1)`.
- `(cd apps/backend && go test -race ./internal/orchestrator -run '^TestUnclassifiedWorkflow(StepRevisionFencedAtRouteClaim|ContextFencedAtRouteClaim|ContextFencedBeforeDetachedLaunch)$' -count=1)`.
- `(cd apps/backend && go test -tags fts5 -race ./internal/agent/runtime/dynamic ./internal/orchestrator ./internal/backendapp ./internal/task/repository/sqlite -run 'Unclassified|Dynamic.*(Race|Conflict)|DynamicRoute.*Concurrent' -count=1)`.

The route-claim transaction now verifies the captured workflow ID, step ID, step revision, veto, and task/session ownership while holding the workflow-step and task locks used by workflow mutations. The detached worker repeats that check while verifying its starting route generation before launch. Barrier tests move the task into a vetoed step, edit a step without changing its ID, and enable the veto before detached launch; each stale context leaves a cleared streak and manual recovery without starting a successor. A current classified non-fallback failure now resets the unknown-error streak before the ordinary fallback gates.
