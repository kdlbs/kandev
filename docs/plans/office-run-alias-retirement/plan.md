---
created: 2026-09-27
status: complete
requirements:
  - REQ-ARCHITECTURE-LINT-DEPRECATION-001
system_design:
  - ../../specs/architecture-lint/system-design/deprecation-ledger.md
legacy_specs: []
---

# Implementation Plan: Office Run Alias Retirement

## Overview

Retire Office compatibility aliases in small groups after a symbol-level
consumer inventory confirms they are unused. The first work order removes five
unused run-event-type aliases while the runs-owned constants and Office policy
remain unchanged.

## Scope

### In scope

- Inventory all 31 Office run aliases and their direct Go consumers.
- Remove the five unused run-event-type aliases and their exact compatibility
  ledger entries.
- Verify the canonical run model constants and event values remain unchanged.

### Out of scope

- Migrating consumers of the remaining 26 Office aliases.
- Changing run persistence, event serialization, Office policy, or the canonical
  declarations in `internal/runs/models`.

## Technical approach

The completed [shared run contract plan](../shared-run-contract-ownership/plan.md)
places generic run types and constants in `internal/runs/models` and keeps
Office policy in Office. The [consumer inventory](consumer-inventory.md) records
the direct production and test files for all 31 registered aliases. Delete only
the five unused aliases from
`apps/backend/internal/office/models/run_compat.go` and their exact declaration
registrations from `config/architecture-lint/compatibility-ledger.json`. Keep
the corresponding canonical constants and their persisted string values in
`apps/backend/internal/runs/models/run.go`.

## Tests

| Acceptance criterion | Verification |
| --- | --- |
| AC-ARCHITECTURE-LINT-DEPRECATION-001.4 | `scripts/lint-architecture.test.py` and `make lint-architecture` reject stale declaration registrations and validate the reduced ledger. |
| Alias removal compiles without Office or Runs caller changes | `go test -p 1 ./internal/office/... ./internal/runs/...` from `apps/backend`. |

## Work orders

- [x] [Task 01: Retire unused run-event-type aliases](task-01-retire-unused-event-type-aliases.md)

## Verification results

Local Office and Runs tests passed with `TMPDIR=/var/tmp`. Architecture lint
tests passed (99 tests), `make lint-architecture` passed, and the local PR
documentation preflight accepted this work order. The race-enabled backend
check first failed once in the unrelated task-service test
`TestArchiveUnarchiveResumeReactivatesLocalOnlyBranch`. The failure did not
reproduce in 20 local repetitions or in the failed-only CI rerun.

## Risks

- A missed Office-package selector could break a downstream consumer. The
  symbol-level import search found no consumer, and Go package tests compile the
  affected Office and Runs packages after deletion.
