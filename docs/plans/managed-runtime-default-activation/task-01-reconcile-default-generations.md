---
id: "01-reconcile-default-generations"
title: "Reconcile managed runtime generations"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-001
  - REQ-AGENTS-RUNTIME-UPDATES-002
acceptance_criteria:
  - AC-AGENTS-RUNTIME-UPDATES-001.5
  - AC-AGENTS-RUNTIME-UPDATES-001.6
  - AC-AGENTS-RUNTIME-UPDATES-002.1
  - AC-AGENTS-RUNTIME-UPDATES-002.2
  - AC-AGENTS-RUNTIME-UPDATES-002.4
  - AC-AGENTS-RUNTIME-UPDATES-002.7
system_design:
  - ../../specs/agents/system-design/runtime-default-activation.md
---

# Task 01: Reconcile Managed Runtime Generations

## Summary

Add the default-generation model and the idempotent store operation. The operation removes an older selection before it records the current generation.

## In scope

- Add trusted package and default-version generation records.
- Reconcile missing, matching, and changed markers.
- Treat legacy unmarked selections as stale.
- Keep the current active-selection key and JSON shape.
- Return all storage errors.

## Out of scope

- Backend startup wiring.
- Runtime command construction.
- User-interface or API changes.

## Acceptance

- A matching generation preserves the active selection without a write.
- A missing or changed generation deletes the selection before marker storage.
- A failed operation returns an error and remains safe to retry.

## Verification

```bash
go test ./internal/agent/managedruntime -count=1
```

Run the command from `apps/backend`.

## Files likely touched

- `apps/backend/internal/agent/managedruntime/selection.go`
- `apps/backend/internal/agent/managedruntime/selection_test.go`

## Dependencies

None.

## Risks

- The delete and marker write are separate operations. The order must keep retries safe after interruption.
- A malformed marker must not preserve an old selection.

## Parallelism

`sequential`

## Inputs

- `REQ-AGENTS-RUNTIME-UPDATES-001`
- `REQ-AGENTS-RUNTIME-UPDATES-002`
- `docs/specs/agents/system-design/runtime-default-activation.md`
- Existing `managedruntime.Store` tests and `system/settings.Store` behavior.

## Results

Implemented `DefaultGeneration` and idempotent `Store.ReconcileDefaults`. Matching
markers preserve selections; missing, changed, or malformed markers clear the
selection before saving the current marker. Storage errors stop reconciliation
and leave it safe to retry.

Verification: `go test ./internal/agent/managedruntime -count=1` passed.
