---
id: "01-retire-unused-event-type-aliases"
title: "Retire unused run-event-type aliases"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-ARCHITECTURE-LINT-DEPRECATION-001
acceptance_criteria:
  - AC-ARCHITECTURE-LINT-DEPRECATION-001.4
system_design:
  - ../../specs/architecture-lint/system-design/deprecation-ledger.md
---

# Task 01: Retire Unused Run-Event-Type Aliases

## Summary

Remove five run-event-type compatibility aliases from the Office models
package after confirming they have no direct consumers. Remove the matching
ledger entries in the same change, while preserving the canonical run model
constants and their stored values.

## In scope

- Remove `RunEventTypeInit`, `RunEventTypeAdapterInvoke`,
  `RunEventTypeComplete`, `RunEventTypeError`, and
  `RunEventTypeRuntimeAction` from `run_compat.go`.
- Remove only the five matching declaration registrations from the compatibility
  ledger.
- Run the affected Office and Runs tests and architecture checks.

## Out of scope

- Removing aliases that still have consumers or unrelated unused aliases.
- Changing `internal/runs/models` declarations, run-event values, persistence,
  event serialization, or Office policy.

## Acceptance

- The five aliases no longer exist in the Office models package.
- The five matching ledger registrations are absent, and architecture lint
  reports no stale or invalid entries.
- The canonical run-event-type constants and values remain unchanged.

## Verification

```bash
(cd apps/backend && TMPDIR=/var/tmp go test -p 1 ./internal/office/... ./internal/runs/...)
python3 scripts/lint-architecture.test.py
make lint-architecture
node --test .github/scripts/pr-docs.test.cjs
git diff --check
```

## Files likely touched

- `apps/backend/internal/office/models/run_compat.go`
- `config/architecture-lint/compatibility-ledger.json`

## Dependencies

None. The completed shared-run contract extraction owns the canonical types
and constants.

## Risks

None beyond an undiscovered caller; the scoped consumer inventory and Go tests
check this boundary.

## Parallelism

`sequential`

## Inputs

- REQ-ARCHITECTURE-LINT-DEPRECATION-001 and its acceptance criterion 001.4.
- The explicit deprecation tracking system design.
- The completed shared-run contract ownership plan and current Go consumers.

## Results

- Removed the five aliases and exact ledger entries.
- `TMPDIR=/var/tmp go test -p 1 ./internal/office/... ./internal/runs/...`
  passed.
- `python3 scripts/lint-architecture.test.py` passed (99 tests).
- `make lint-architecture` passed.
- The local PR documentation preflight accepted this work order; its test suite
  passed (101 tests).
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all`
  passed.
- The race-enabled backend check first failed once in the unrelated task-service
  test `TestArchiveUnarchiveResumeReactivatesLocalOnlyBranch`. The failure did
  not reproduce in 20 local repetitions or in the failed-only CI rerun.
