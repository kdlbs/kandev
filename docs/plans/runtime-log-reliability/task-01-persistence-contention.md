---
id: "01-persistence-contention"
title: "Attribute persistence contention"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007
  - REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001
acceptance_criteria:
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.1
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.3
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.4
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.6
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.7
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.8
  - AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.1
system_design:
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
  - ../../specs/platform/system-design/runtime-failure-attribution.md
---

# Task 01: Attribute persistence contention

## Summary

Produce a controlled explanation for the probe outages. Keep health policy unchanged and record any remaining unknown cause.

## In scope

- Reader-pool saturation, writer occupancy, long transactions, catalog sweep cost, and maintenance controls on disposable storage.
- Existing probe-stage observations, pool counter deltas, middleware 503/recovery, and bounded synthetic fixtures.
- `persistence-evidence.md` with workload, timings, operation identity, version, and a narrowly scoped repair proposal.

## Out of scope

- Raising timeouts, changing pools or health semantics, running load against the live installation, and editing an unattributed production query.

## Acceptance

- Reproduce reader and writer contention separately. Record the held operation, queue wait, probe stage, and recovery for each fixture.
- Preserve missing-table, closed-pool, maintenance-deferral, startup, and middleware behavior through existing regression controls.
- Attribute a real workload producer before proposing its production repair. Otherwise record the cause as unresolved with the next bounded experiment.

## Verification

Run this block from the repository root after the implementation result exists.
New test names below are required planned regressions, not claims of existing coverage.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/persistence/requiredstores ./internal/backendapp -run 'Test(RuntimeHealth|HealthCheck|ProbeTables|RequiredPersistence|PersistenceMiddleware)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/persistence/requiredstores -run '^TestPersistenceContentionFixture$' -count=1 -v)
```

## Files likely touched

- `apps/backend/internal/persistence/requiredstores/health_contention_test.go (new)`
- `apps/backend/internal/persistence/requiredstores/health_test.go`
- `apps/backend/internal/backendapp/persistence_middleware_test.go`
- `apps/backend/internal/db/pool.go`
- `docs/plans/runtime-log-reliability/persistence-evidence.md (new)`

## Dependencies

None. Follow the plan's sequential priority order.

## Risks

- The fixture must identify a producer, not infer query responsibility from cumulative pool counters.
- Existing analytics and immediate-writer packages can affect the implementation base. Reconcile those contracts before selecting a repair.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md), especially evidence, contract ownership, and completion rules.
- [Design](../../specs/platform/system-design/postgres-domain-store-parity.md) and its linked requirements.
- [Design](../../specs/platform/system-design/runtime-failure-attribution.md) and its linked requirements.

- Scoped backend/agentctl instructions for any touched package.
- Existing source and tests listed above. Preserve completed companion-package results.

## Results

Cause remains unresolved. The new disposable fixture reproduced writer and reader connection-checkout stalls, recorded wait-count and wait-duration deltas, verified the bounded probe stages, and confirmed recovery after releasing each transaction. Existing controls preserved maintenance deferral, startup strictness, missing-table behavior, and middleware rejection/recovery.

Verification passed:

- `go test -trimpath -tags fts5 ./internal/persistence/requiredstores -run '^TestPersistenceContentionFixture$' -count=1 -v`
- `go test -trimpath -tags fts5 -race ./internal/persistence/requiredstores ./internal/backendapp -run 'Test(RuntimeHealth|HealthCheck|ProbeTables|RequiredPersistence|PersistenceMiddleware)' -count=1`

See [persistence evidence](persistence-evidence.md) for the retained diagnostic values and the next bounded experiment. No production behavior changed.
