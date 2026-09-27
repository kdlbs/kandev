---
created: 2026-09-26
status: done
requirements:
  - REQ-OFFICE-RUN-CAUSATION-001
  - REQ-OFFICE-BACKPRESSURE-001
  - REQ-OFFICE-LAUNCH-SAFETY-001
  - REQ-OFFICE-SCHEDULER-001
system_design:
  - ../../specs/office/system-design/unattended-launch-safety-01.md
  - ../../specs/office/system-design/scheduler-01.md
legacy_specs: []
---

# Implementation Plan: Shared Run Contract Ownership

## Overview

Move reusable Go run-row and run-event contracts out of `internal/office/models`
and into `internal/runs/models`, where the shared queue owns their definitions.
This is an ownership change only. The accepted
[run contract ownership ADR](../../decisions/2026-09-26-run-contract-ownership.md)
keeps Office causation, launch, priority, and routing policy in Office. It does
not move physical database schema ownership or change persisted or wire values.

## Scope

The extraction covers `Run`, `RunStatus`, `RunEvent`, `RunEventType`,
`RunEventLevel`, and the nested persisted field types `ActorKind`,
`PriorityClass`, and `RoutingBlockedStatus`, including their existing constants
and pure methods. Direct runs consumers use the runs-owned types. Deprecated
Office aliases preserve compatibility for the many existing Office callers and
are registered individually in the compatibility ledger for removal after
Office consumers migrate.

Office continues to own causation derivation, launch decisions and safety gates,
priority classification, routing policy, and metrics. Office-specific safety
records and continuation-scope policy remain in Office. SQL tables, migration
locations, scan layouts, transactions, queue behavior, event protocol, and
scheduling behavior are unchanged.

## Work order

- [x] [Task 01: Extract shared run contracts](task-01-extract-shared-run-contracts.md)

## Verification results

- `go test -p 1 ./internal/runs/... ./internal/office/... ./internal/backendapp -count=1`
  passed.
- `golangci-lint run ./internal/runs/... ./internal/office/models --new-from-rev=origin/main --timeout=5m`
  reported zero issues.
- `python3 scripts/lint-architecture.test.py` passed 62 tests;
  `make lint-architecture` and the explicit `origin/main` baseline lint passed.
- In a disposable combined tree using #3975 head
  `c6acc8f8afea6c41973199b4b825bd79fa58f453`, the #3974 extraction commit
  `26b1a6070745aa2d4a583e1c5577e8a23d0bde41`, and current main
  `dfce4dac05809c0fcec156166f5479f14b0cdb76`, the exact declaration scanner
  matched all 31 Office aliases to 31 registrations with zero ledger diagnostics;
  the full architecture CLI passed. #3975's changes after `a2cf91d` are
  documentation-only.
- `python3 scripts/lint-spec-files.py --all` passed; the documentation catalog
  validated 312 decisions and 1,187 specifications.
- The current-main PR documentation validator accepted the linked work order and
  its referenced designs with no errors. Its source is unchanged from
  `origin/main`; the hosted trusted-base status is checked after the PR push.
- PostgreSQL-specific run repository cases reported explicit skips because
  `KANDEV_TEST_POSTGRES_DSN` is not configured in this environment.

## Risks

The Office aliases retain temporary compatibility surface. Their individual
ledger entries identify the production-consumer migration condition and must be
removed with the aliases after Office consumers migrate to `internal/runs/models`.
