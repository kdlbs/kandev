---
id: "01-fence-superseded-candidate-failure"
title: "Fence a dynamic failure to the candidate that produced it"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.10
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-02.md
---

# Task 01: Fence a dynamic failure to the candidate that produced it

## Summary

A failure of an execution that the route already left must not be charged to
the candidate the route holds now.

## Changes

- Move `dynamicFailureSession` into `dynamic_failure_attribution.go` and make
  it decline a failure whose event names a concrete execution profile other
  than the session's current execution profile. The existing execution ID
  check stays.
- Add `requireCurrentFailureRoute` to the engine and call it at the top of
  `ApplyFailureContext`, before any circuit is opened. A failure whose
  generation differs from the known route state, or whose candidate differs
  from the route's current candidate, returns `ErrStaleGeneration`. A session
  without known route state keeps the existing behavior.
- Capture `ExecutionProfileID` from the execution in lifecycle stream events.
  Carry that value through `handleAgentErrorEvent` to the existing guard.
  Keep logical-profile attribution and Office identity separate.

## Acceptance

1. After a route moved from candidate A to candidate B, a failure that names
   the old generation, the old candidate, or both returns
   `ErrStaleGeneration`; candidate B's circuit stays closed and the route
   state keeps B's generation and candidate.
2. When the session already names candidate B but candidate A's execution
   still serves it, a failure of that execution leaves candidate B's circuit
   closed and the persisted route state unchanged.
3. Stream events retain the concrete profile through publication and decoding.
   A predecessor stream error leaves the successor's circuit and route unchanged.
   Current-candidate errors still apply the configured policy.
   Legacy events without a concrete profile retain their existing behavior.

## Verification

- `go test -tags fts5 ./internal/agent/runtime/dynamic/ -run TestStaleFailureDoesNotOpenTheCurrentCandidateCircuit`
- `go test -tags fts5 ./internal/orchestrator/ -run TestDynamicFailureOfSupersededCandidateDoesNotChargeCurrentCandidate`
- `golangci-lint run ./internal/agent/runtime/dynamic/... ./internal/orchestrator/...`
- `make -C apps/backend build`
- `go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle/ -run TestAgentStreamEventCarriesConcreteExecutionProfile`
- `go test -trimpath -tags fts5 ./internal/orchestrator/ -run TestStreamErrorFailureAttributionAfterRouteChange`

## Results

The original contributor supplied verification results in PR #4328.
The review follow-up adds stream attribution and regression coverage.
The review did not run validation commands, as requested.
Verification of the added coverage remains pending.
