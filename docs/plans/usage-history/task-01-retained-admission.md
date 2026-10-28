---
id: "01-retained-admission"
title: "Capture retained usage and ownership"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-HISTORY-002
  - REQ-COSTS-USAGE-HISTORY-003
  - REQ-COSTS-USAGE-HISTORY-007
acceptance_criteria:
  - AC-COSTS-USAGE-HISTORY-002.1
  - AC-COSTS-USAGE-HISTORY-002.2
  - AC-COSTS-USAGE-HISTORY-002.3
  - AC-COSTS-USAGE-HISTORY-002.4
  - AC-COSTS-USAGE-HISTORY-003.1
  - AC-COSTS-USAGE-HISTORY-003.2
  - AC-COSTS-USAGE-HISTORY-003.3
  - AC-COSTS-USAGE-HISTORY-003.4
  - AC-COSTS-USAGE-HISTORY-003.5
  - AC-COSTS-USAGE-HISTORY-003.6
  - AC-COSTS-USAGE-HISTORY-003.7
  - AC-COSTS-USAGE-HISTORY-007.3
system_design:
  - ../../specs/costs/system-design/usage-history.md
---

# Task 01: Capture retained usage and ownership

## Summary

Add retained subjects and scope ownership without changing existing read contracts.
Commit a retained native observation inside the existing accepted-event transaction.

## In scope

- Add portable scope/subject schema and additive retained bindings to the current measurement tables.
- Capture org/workspace and actor evidence at execution/turn admission. Preserve unknown/mixed evidence and single-user identity.
- Persist immutable turn/execution binding IDs through admission, usage publication, and writer lookup. Cover shared sessions, replay, and child attribution.
- Normalize native and existing external records through one internal accounting contract with checked integer and category semantics.
- Add a narrow transactional retention capability without a task/analytics import cycle or another native writer.
- Preserve native insert, session-rollup, duplicate, FK-retry, transient-retry, and shutdown behavior.
- Register durable dirty-partition work in the observation transaction for later projection.

## Out of scope

- Legacy backfill/deletion cutover, public Host extensions, statistics query cutover, and rendered UI.

## Acceptance

- An accepted native transaction contains its retained observation and correct trusted ownership, or rolls back atomically.
- Old callers remain compatible. Counts, units, nullable fields, provenance, and integer transport never change meaning silently.
- Scope/actor tests cover Auth off, Auth on, MultiTenancy, unknown/mixed actors, and forged collector identities.

## Verification

Run from the Kandev root. New `TestUsageHistory*` cases must exist before these commands count as evidence.

```bash
(cd apps/backend && go test -race -trimpath ./internal/analytics/... ./internal/task/usage/... -run 'TestUsageHistory|TestSessionUsage' -count=1)
(cd apps/backend && go test -race -trimpath ./internal/task/repository/sqlite ./internal/backendapp -run 'TestUsageHistory|TestCreateTaskUsageEvent|TestTaskUsage' -count=1)
(cd apps/backend && : "${KANDEV_TEST_POSTGRES_DSN:?Set an isolated PostgreSQL test DSN}" && go test -trimpath ./internal/analytics/... ./internal/task/repository/sqlite -run 'TestUsageHistoryPostgres' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/analytics/{models/usage.go,repository/interface.go,service/service.go}`
- `apps/backend/internal/analytics/repository/sqlite/{usage_schema.go,usage.go}` and new `usage_history_*` files/tests
- `apps/backend/internal/task/{usage,models}` and `repository/sqlite/usage_events.go`
- `apps/backend/internal/backendapp/task_usage_wiring.go`
- Existing execution/turn admission paths that carry `ExecutorActorUserID` and authenticated identity

## Dependencies

None. Use the accepted requirements and current source baseline from the plan.

## Risks

The native writer already owns transaction retries and rollup atomicity. A post-commit callback cannot be its only retained-data path.
Do not treat a session's current actor as proof for every earlier turn.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/costs/requirements/usage-history.md), REQ-002/003/007 and listed ACs.
- [Design](../../specs/costs/system-design/usage-history.md), Retained identity, Ownership, Measurement contract, Native admission.
- [Plan](plan.md), compatibility matrix and existing writer/ledger references.

## Results

Pending.
