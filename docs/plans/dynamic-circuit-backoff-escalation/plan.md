---
created: 2026-10-20
status: done
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-01.md
legacy_specs: []
---

# Implementation Plan: Escalate circuit backoff for repeated failures

This completed package records the earlier two-flag implementation. Its flag-specific
choices and sibling work order are historical. The single-flag replacement is
implemented in [dynamic circuit defaults](../dynamic-circuit-defaults/plan.md),
which records the replacement policy and its verification separately.

## Overview

A dynamic candidate circuit always opens for one fixed minute, so a candidate
that keeps failing is re-probed every minute. One sequential work order adds an
escalating backoff ladder behind a new runtime flag that is off in every
shipped profile, and persists the consecutive failure count with the circuit.

## Scope

In scope: the dynamic `CircuitRegistry` failure count and ladder, the engine's
failure open, the `routingerr.Error.ResetHintInferred` marker, the circuit store
column and migration, and the `features.dynamicCircuitBackoffEscalation` flag
across config, registry, profiles, frontend defaults, and public docs.

Excluded: new UI, a per-candidate ladder setting, changes to which failures
qualify, and changes to the probe lease.

## Technical approach

`CircuitRegistry.OpenFailure` replaces the engine's direct `Open` call. Disabled,
it computes exactly the previous deadline. Enabled, it advances the count unless
the circuit is still open within its window and applies the ladder rung, a
later reset hint, and a 24-hour cap on inferred hints. `ReleaseProbe` advances
the count on failure and keeps it on success. A turn that completes without a
provider error resets it through `ResetCircuitFailures`, called from the
orchestrator's completion handler. The count is part of `CircuitSnapshot` and
the `dynamic_resource_circuits.failure_count` column.

## Work orders

- [x] [Task 01: Escalate circuit backoff for repeated failures](task-01-circuit-backoff-escalation.md)

## Changed-path coverage

The work order owns the dynamic runtime, routingerr, task store, runtime flag,
config, profile, frontend default, and public configuration changes, their
tests, the new acceptance criteria in
`docs/specs/agents/requirements/dynamic-agent-routing.md`, and the
`docs/specs/agents/system-design/dynamic-agent-routing-01.md` update. This
manifest and its sole sibling order complete the coverage package.

## Verification

Permanent tests: `circuit_backoff_test.go` and `engine_backoff_test.go` beside
the registry and engine, `reset_hint_inferred_test.go` beside the classifier,
`dynamic_circuit_failures_test.go` beside the SQLite store,
`dynamic_circuit_completion_reset_test.go` beside the orchestrator, and
`upgrade_circuit_failures_test.go` in the store conformance suite.

## Risks

- A long open can keep a recovered candidate skipped for up to four hours; the
  flag is off by default and a clean turn resets the count.
- Concurrent failures of one incident could climb the ladder at once; a
  failure inside the open window does not advance the count.
- Resetting the count on a successful launch would keep repeated mid-turn
  failures on the first rung; only a clean turn completion resets it.
