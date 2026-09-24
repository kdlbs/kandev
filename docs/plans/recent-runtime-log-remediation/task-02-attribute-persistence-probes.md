---
id: "02-attribute-persistence-probes"
title: "Attribute persistence probe failures"
status: done
wave: 2
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007
acceptance_criteria:
  - AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.1
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.3
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.4
system_design:
  - ../../specs/platform/system-design/runtime-failure-attribution.md
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
---

# Task 02: Attribute persistence probe failures

## Summary

The September 24 probe timed out at the writer and made stateful calls
temporarily unavailable. Add one safe failure diagnostic that distinguishes a
pool wait from other probe stages without changing the health policy.

## In scope

- Measure writer ping, reader ping, and table probe stages in `Health`.
- Capture bounded writer and reader pool statistics at a failed probe.
- Test timeout and subsequent recovery with unchanged readiness behavior.

## Out of scope

- Changing probe deadline, pool sizes, maintenance coordination, or 503 policy.

## Acceptance

- One failed sweep emits one stage, elapsed time, class, and pool snapshot.
- No SQL, DSN, or raw database error containing sensitive data enters new fields.
- Recovery continues on the next successful probe without restart.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/persistence/requiredstores -run 'Test(RuntimeHealth|StartupHealth|HealthCheck|ProbeTables)' -count=1)
```

## Files likely touched

- `apps/backend/internal/persistence/requiredstores/health.go`
- `apps/backend/internal/persistence/requiredstores/health_test.go`

## Dependencies

None.

## Risks

- Pool statistics are a snapshot, not a causal trace; do not infer a fix from
  one event.

## Parallelism

`sequential`

## Inputs

- Runtime-failure-attribution and required-store health designs.
- Required-internal-persistence and maintenance-health-probe ADRs.

## Results

The periodic health warning now records the failed stage, elapsed time, a
fixed error class, and writer/reader pool snapshots without the raw database
error. Writer and reader timeouts emit one warning for a sweep, and the next
successful check restores health. Table-probe failures carry their store ID.
Verification passed:

```bash
(cd apps/backend && go test -tags fts5 ./internal/persistence/requiredstores -run 'Test(RuntimeHealth|StartupHealth|HealthCheck|ProbeTables)' -count=1)
```
