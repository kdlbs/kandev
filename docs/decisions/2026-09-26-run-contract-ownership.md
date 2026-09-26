# ADR-2026-09-26-run-contract-ownership: Own Shared Run Data in Runs

**Status:** accepted
**Date:** 2026-09-26
**Area:** backend, workflow

## Context

ADR `2026-08-01-global-run-scheduler-ownership` makes `internal/runs/` the
backend-wide queue and prohibits generic runs code from depending on Office.
The persisted `Run` and `RunEvent` Go contracts still live in
`internal/office/models`, so repository and service code in `internal/runs`
imports the Office implementation package to read and write shared queue data.

Office requirements define the meaning and policy of causation, launch
backpressure, provider routing, and launch safety. Those responsibilities stay
with Office even though the shared queue stores some of their values.

## Decision

`internal/runs/models` owns the Go data contracts for the shared run row and its
lifecycle events: `Run`, `RunStatus`, `RunEvent`, `RunEventType`, and
`RunEventLevel`. The field types stored as part of that row — `ActorKind`,
`PriorityClass`, and `RoutingBlockedStatus` — live with the contract. Their
Office-specific interpretation and derivation remain owned by Office.

Office continues to own enqueue attribution, priority classification, launch
gates, routing decisions, and their metrics. Office-only safety records and
policies such as `GateFailureState`, `CausationRefusalEntry`, and
`ContinuationScopeForRun` remain in Office. No schema, SQL layout, serialized
field, event protocol, or scheduling behavior changes as part of this ownership
move.

`internal/office/models` may keep direction-safe aliases to `internal/runs/models`
while Office consumers migrate. Such aliases point from Office to runs, carry a
compatibility-ledger entry, and are removed after Office production and test
consumers use the owning package directly.

## Consequences

- The shared run repository and service can consume their records without
  importing Office models.
- Office can keep its launch and routing policy without making the shared queue
  depend on those policies.
- The import baseline remains for Office-owned safety records, continuation
  scope derivation, priority policy, and launch-safety metrics until those
  dependencies are independently redesigned.
- Future fields on `Run` belong to the shared runs data contract. Their policy
  meaning can remain with Office and must be documented by the owning Office
  requirement or design.

## Alternatives Considered

### Keep all run records in Office models

Rejected because the generic queue would keep importing the Office
implementation package, contrary to the global scheduler ownership boundary.

### Move Office launch and routing policy into runs

Rejected because it expands this data-contract extraction into a policy
migration and would make generic runs own Office decisions.

### Replace nested field types with primitive values

Rejected because it weakens the Go contract without reducing the persisted or
serialized data shape.
