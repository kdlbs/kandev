---
id: "01-extract-shared-run-contracts"
title: "Extract shared run contracts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-RUN-CAUSATION-001
  - REQ-OFFICE-BACKPRESSURE-001
  - REQ-OFFICE-LAUNCH-SAFETY-001
  - REQ-OFFICE-SCHEDULER-001
acceptance_criteria:
  - AC-OFFICE-RUN-CAUSATION-001.6
  - AC-OFFICE-RUN-CAUSATION-001.15
  - AC-OFFICE-BACKPRESSURE-001.1
  - AC-OFFICE-BACKPRESSURE-001.7
  - AC-OFFICE-LAUNCH-SAFETY-001.6
  - AC-OFFICE-SCHEDULER-001.1
system_design:
  - ../../specs/office/system-design/unattended-launch-safety-01.md
  - ../../specs/office/system-design/scheduler-01.md
---

# Task 01: Extract Shared Run Contracts

Move only the reusable Go representation of run rows and lifecycle events into
the runs-owned models package. Keep Office policy and existing storage behavior
unchanged.

## Scope

- Define `Run`, `RunStatus`, `RunEvent`, `RunEventType`, `RunEventLevel`,
  `ActorKind`, `PriorityClass`, and `RoutingBlockedStatus` in
  `internal/runs/models` with their existing fields, constants, tags, and pure
  methods.
- Migrate direct runs repository and service consumers to the runs-owned types.
- Keep direction-safe deprecated aliases in `internal/office/models` for Office
  callers. Register each annotated alias declaration in the compatibility
  ledger. Remove the aliases after Office production and test consumers use
  `internal/runs/models`.
- Preserve persisted values, JSON and database tags, pointer/null behavior, SQL
  scan layouts, sentinels, errors, and event payload behavior.
- Reduce only the runs-to-Office import edges removed by this migration; leave
  remaining Office policy dependencies in place.

## Exclusions

Do not change schema or migration ownership, queries, scheduling, launch budgets,
retry behavior, locking, transactions, event protocol, causation semantics,
priority classification, provider-routing policy, or runtime lifecycle. Do not
move Office safety records or `ContinuationScopeForRun` into shared models.

## Acceptance

1. The extracted types retain their persisted and serialized contract, including
   exact enum values and methods; focused contract tests and Office callers
   compile against the compatibility aliases.
2. Runs repository and service production code imports `internal/runs/models`
   directly, reducing the `runs_office_import` baseline from 11 to 6 edges.
3. No scheduler, repository algorithm, database schema, event, or launch
   behavior changes.

## Verification

Run from the repository root unless the command changes directory explicitly:

```sh
(cd apps/backend && go test -p 1 ./internal/runs/... ./internal/office/... ./internal/backendapp -count=1)
(cd apps/backend && golangci-lint run ./internal/runs/... ./internal/office/models --new-from-rev=origin/main --timeout=5m)
(cd apps/backend && go test -v -p 1 ./internal/runs/repository/sqlite -run '^TestPostgres' -count=1)
python3 scripts/lint-architecture.test.py
make lint-architecture
python3 scripts/lint-architecture.py --all --baseline-base-ref origin/main --allow-missing-base-baseline
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py validate
node --test .github/scripts/pr-docs.test.cjs
git diff --check
```

## Files

- `apps/backend/internal/runs/models/` and focused model contract tests
- Direct runs repository and service type consumers
- `apps/backend/internal/office/models/run_compat.go` and its alias checks
- `config/architecture-lint/` baseline and deprecation-ledger records
- The accepted ownership ADR and the affected Office system-design passages

## Dependencies and parallelism

No work-order dependency. Implement and validate sequentially because the model
types, consumers, aliases, compatibility records, and import baseline form one
contract migration.

## Results

- Moved the approved shared run and event contracts into `internal/runs/models`
  without changing schema, stored values, serialization, or queue behavior.
- Migrated direct runs consumers and retained individually registered Office
  aliases for existing callers.
- Reduced the measured runs-to-Office import baseline from 11 edges to 6; the
  remaining edges cover Office-owned safety, continuation, priority-policy, and
  metrics dependencies.
- `go test -p 1 ./internal/runs/... ./internal/office/... ./internal/backendapp -count=1`
  passed; `golangci-lint` reported zero issues.
- Architecture lint tests passed (62 tests), `make lint-architecture` passed,
  and explicit current-main baseline lint passed.
- The exact declaration scanner at #3975 head
  `c6acc8f8afea6c41973199b4b825bd79fa58f453` was tested in a disposable
  combined tree with the extraction commit and current main. It matched all 31
  Office aliases to 31 registrations with zero ledger diagnostics, and the full
  architecture CLI passed. The #3975 changes after `a2cf91d` are docs-only.
- Documentation catalog validation and full specification lint passed. The
  trusted current-main coverage validator accepted this linked work order with
  zero errors; its source matches `origin/main`.
- PR documentation validator tests passed (83 tests), and `git diff --check`
  passed.
- PostgreSQL repository integration cases explicitly skipped because
  `KANDEV_TEST_POSTGRES_DSN` is not configured; the SQLite run repository tests
  passed as part of the Go suite.
