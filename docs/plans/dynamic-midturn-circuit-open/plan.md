---
created: 2026-10-20
status: done
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-02.md
legacy_specs: []
---

# Implementation Plan: Open the candidate circuit for mid-turn provider failures

## Overview

A classified provider failure reported after the attempt already produced
output or effects is not routable: the orchestrator keeps the failed turn on its
normal recovery surface. Today that branch also skips the engine, so the
candidate's shared circuit never opens and every new session re-selects the
same unhealthy candidate. One sequential work order charges the circuit on that
branch without changing routing.

## Scope

In scope: a new `Engine.RecordFailureCircuit` in
`apps/backend/internal/agent/runtime/dynamic/failure_circuit.go`, the matching
`ProfileExecutionResolver.RecordFailureCircuit`, and the orchestrator's
ineligible branch in `routeDynamicAgentFailureWithEvidence`.

Excluded: route advancement, successor launch, generation changes, recovery
surface changes, backoff policy changes, and a runtime flag.

## Technical approach

The engine method reuses `requireCurrentFailureRoute` and
`openCircuitForFailure`, so the same generation/candidate fence, qualifying
codes, standard backoff, and reset hint apply as on the routable path. It skips
owner-release codes because `holdBlockedAccount` already holds that profile.
The orchestrator calls it only after `dynamicFailureSession` accepted the
failure and `prepareDynamicFailureEvidence` declared it ineligible.

## Work orders

- [x] [Task 01: Open the candidate circuit for mid-turn provider failures](task-01-midturn-circuit-open.md)

## Changed-path coverage

The work order owns the engine, resolver, and orchestrator files above, their
tests, the new acceptance criterion in
`docs/specs/agents/requirements/dynamic-agent-routing.md`, and the
`docs/specs/agents/system-design/dynamic-agent-routing-02.md` update. This
manifest and its sole sibling order complete the coverage package.

## Verification

Permanent tests: `failure_circuit_test.go` beside the engine and
`dynamic_midturn_circuit_test.go` beside the orchestrator cover the open
circuit and skipped selection, stale generation and candidate fences, a
predecessor's late failure, non-qualifying codes, and an unchanged route.

## Risks

- Charging the wrong candidate would disable a healthy successor; the existing
  fence rejects superseded generations and candidates before any circuit opens.
- Opening a circuit for a non-provider error would hide healthy candidates;
  only the existing qualifying codes open one.
