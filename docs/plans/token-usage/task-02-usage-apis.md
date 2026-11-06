---
id: "02-usage-apis"
title: "Host usage and query APIs"
status: done
wave: 2
depends_on: ["01-core-usage"]
plan: "plan.md"
requirements:
  - REQ-COSTS-TOKEN-USAGE-001
  - REQ-COSTS-TOKEN-USAGE-002
  - REQ-COSTS-TOKEN-USAGE-004
  - REQ-COSTS-TOKEN-USAGE-005
acceptance_criteria:
  - AC-COSTS-TOKEN-USAGE-001.1
  - AC-COSTS-TOKEN-USAGE-001.6
  - AC-COSTS-TOKEN-USAGE-001.7
  - AC-COSTS-TOKEN-USAGE-002.5
  - AC-COSTS-TOKEN-USAGE-004.4
  - AC-COSTS-TOKEN-USAGE-005.5
  - AC-COSTS-TOKEN-USAGE-004.8
  - AC-COSTS-TOKEN-USAGE-004.9
  - AC-COSTS-TOKEN-USAGE-004.10
  - AC-COSTS-TOKEN-USAGE-004.11
  - AC-COSTS-TOKEN-USAGE-004.12
system_design:
  - ../../specs/costs/system-design/token-usage.md
---

# Task 02: Host usage and query APIs

## Summary

Expose typed Host writes and saved reads through the accounting service. Add authorized browser queries and efficient session discovery.

## In scope

- Expose model/provider, daily and monthly groupings, raw numeric sorting, pagination, rate availability and filtered aggregate totals.
- Preserve provider identity through Host usage writes and prohibit collector/provider conflation.
- Add Usage accessors, proto DTOs, capability gates, source binding and batch limits.
- Push session filters and pagination into SQL while preserving existing filter intersections.
- Add authenticated native usage aggregates, existence metadata and collection health.
- Update GRPC-CONTRACT.md and SDK contract documentation for the implemented API.

## Out of scope

- Plugin command execution and page markup.

## Acceptance

- An undeclared capability or foreign session cannot read or write usage.
- Repeated writes return authoritative results and never bypass the service.
- Session-list tests verify bounded queries, pagination and workspace filters.

## Verification

Validation was run from the Kandev repository root; the executed commands and results are recorded below.

```bash
env -u KANDEV_INTERNAL_CONFIG_FILE -u KANDEV_INTERNAL_CONFIG_HOME_FILE -u KANDEV_HOME_DIR -u KANDEV_SERVER_PORT \
  go test ./pkg/pluginsdk/... ./internal/plugins/... ./internal/analytics/... ./internal/task/repository/sqlite/... ./internal/task/service/... -count=1
go build ./cmd/kandev
```

## Files likely touched

- `apps/backend/proto/kandev/plugin/v1/plugin.proto`
- `apps/backend/pkg/pluginsdk/host.go`
- `apps/backend/pkg/pluginsdk/data_types.go`
- `apps/backend/internal/plugins/host_data_queries.go`
- `apps/backend/internal/plugins/host_usage.go (new)`
- `apps/backend/internal/analytics/handlers/usage_handlers.go (new)`
- `apps/backend/internal/backendapp`
- `docs/plans/plugins/GRPC-CONTRACT.md`

## Dependencies

01-core-usage.

## Risks

Additive v1 DTOs must preserve old callers. The proposed exact Host protocol is not part of this work.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/costs/requirements/token-usage.md) and the acceptance IDs in frontmatter.
- [System design](../../specs/costs/system-design/token-usage.md).
- [Core projection ADR](../../decisions/2026-09-13-core-session-usage-projections.md).
- Existing source paths and test patterns listed in the [plan](plan.md#tests).

## Results

Implemented typed Host usage read/write APIs, capability and source scoping, authenticated token usage queries, generated protobuf bindings, workspace-aware session filtering and SQL pagination with a compatibility fallback for lightweight adapters.

Validation: `go test ./internal/plugins ./internal/task/repository/sqlite ./internal/task/service ./pkg/pluginsdk ./internal/analytics/handlers -count=1` passed, and `go build ./cmd/kandev` passed. The full backend run also passed all changed packages.
