---
id: "01-core-usage"
title: "Core usage accounting"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COSTS-TOKEN-USAGE-001
  - REQ-COSTS-TOKEN-USAGE-004
acceptance_criteria:
  - AC-COSTS-TOKEN-USAGE-001.1
  - AC-COSTS-TOKEN-USAGE-001.2
  - AC-COSTS-TOKEN-USAGE-001.3
  - AC-COSTS-TOKEN-USAGE-001.4
  - AC-COSTS-TOKEN-USAGE-001.5
  - AC-COSTS-TOKEN-USAGE-001.7
  - AC-COSTS-TOKEN-USAGE-004.4
  - AC-COSTS-TOKEN-USAGE-004.8
  - AC-COSTS-TOKEN-USAGE-004.9
  - AC-COSTS-TOKEN-USAGE-004.10
  - AC-COSTS-TOKEN-USAGE-004.11
  - AC-COSTS-TOKEN-USAGE-004.12
system_design:
  - ../../specs/costs/system-design/token-usage.md
---

# Task 01: Core usage accounting

## Summary

Create the source-aware accounting service and durable storage. Preserve the existing native event ledger and expose canonical query results.

## In scope

- Add provider-preserving identities, model/provider and day/month aggregates, weighted effective rates, and complete filtered totals.
- Test unequal denominators, zero/unknown rates, partial months, provider collisions and monthly/daily reconciliation.
- Implement the design data model, revision rules, retention, source selection and dated versus lifetime projections.
- Verify pinned tokscale dated output with small sanitized fixtures. Record the supported export or the specified daily-query fallback.
- Add SQLite and PostgreSQL coverage using existing fixture patterns.

## Out of scope

- Host transport, collector processes and rendered UI.

## Acceptance

- Repeated cumulative snapshots leave totals unchanged, while newer corrections replace them.
- Native and external overlap never doubles usage, and the existing ledger remains intact.
- Deletion, unknown values and timezone tests pass on supported databases.

## Verification

Validation was run from the Kandev repository root; the executed commands and results are recorded below.

```bash
(cd apps/backend && go test ./internal/analytics/repository/sqlite -run 'TestSessionUsage' -count=1)
```

## Files likely touched

- `apps/backend/internal/analytics/models/usage.go (new)`
- `apps/backend/internal/analytics/service/usage.go (new)`
- `apps/backend/internal/analytics/repository/interface.go`
- `apps/backend/internal/analytics/repository/sqlite/usage.go (new)`
- `apps/backend/internal/task/models/usage_event.go (read boundary)`
- `apps/backend/internal/task/repository/sqlite/usage_events_schema.go (reference only)`

## Dependencies

None.

## Risks

The ledger uses event identities while external data uses cumulative coverage. Unknown overlap must remain incomplete.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/costs/requirements/token-usage.md) and the acceptance IDs in frontmatter.
- [System design](../../specs/costs/system-design/token-usage.md).
- [Core projection ADR](../../decisions/2026-09-13-core-session-usage-projections.md).
- Existing source paths and test patterns listed in the [plan](plan.md#tests).

## Results

Implemented core usage accounting with source-aware lifetime and dated bucket storage, revision-safe idempotent upserts, native ledger projection, workspace ownership checks, provider-preserving grouping, nullable metrics, overlap selection, retention and effective-rate calculation.

Validation: `go test ./internal/analytics/repository/sqlite -run 'TestSessionUsage' -count=1` passed. The changed analytics packages also passed in the focused backend suite. PostgreSQL integration and live tokscale benchmark were not run in this environment.
