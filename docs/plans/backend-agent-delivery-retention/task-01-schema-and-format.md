---
id: "01-schema-and-format"
title: "Add retention schema and shared transaction fencing"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DELIVERY-RETENTION-006
  - REQ-PLATFORM-DELIVERY-RETENTION-003
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-RETENTION-006.1
  - AC-PLATFORM-DELIVERY-RETENTION-006.4
  - AC-PLATFORM-DELIVERY-RETENTION-003.2
system_design:
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
---

# Task 01: Add retention schema and shared transaction fencing

## Summary

Add task-owned compact evidence columns, cleanup/progress tables and format readiness. Introduce shared delivery transaction serialization without activating pruning.

## In scope

- Implement additive ordered migrations, readiness assertions, default-zero checkpoint constraints and registered task-store conformance.
- Apply shared session/stream ordering after existing hierarchy/managed-deletion guards. Reuse factory `_txlock=immediate` SQLite transactions and deferred reader snapshots; add only missing delivery/PostgreSQL fencing. No custom BEGIN, per-feature pool or row-touch reservation.
- Include current structural metadata CAS in `session.go` and managed state/environment admission; never call a pool-opening helper from inside an owned transaction. Preserve historical-generation policy.
- Write fresh/reopen/pre-retention/previous-stable upgrade tests and interrupted DDL tests on both engines. Include unknown/newer format rejection and compatibility-marker state before first prune.

## Out of scope

Implementation was authorized by the user on 2026-10-09.
Agentctl journal policy, canonical-history expiry, new UI and feature flags are excluded.
This order does not authorize commit, push, merge or delegation.

## Acceptance

- Both engines replay schema initialization safely and preserve active/uncertain/prepared submissions, projected-but-unsettled terminals and independent blocks.
- Required columns/indexes/format failure blocks readiness; no destructive backfill or startup payload scan occurs.
- Real independent pools serialize delivery/deletion/replacement while WAL readers progress. Cancelled BEGIN performs no business predicate/mutation and settles before pool reuse. Native SQLite wait may reach its existing 5-second busy timeout; no database lock crosses network work.

## TDD evidence

Start each changed-logic scenario with a failing test against the pre-change behavior. Record Red, Green and refactor results.
Use existing backend test helpers, fake clocks and deterministic barriers. Do not use sleeps to manufacture race order.
Required new test groups: `TestDeliveryRetentionSchema`, `TestDeliveryRetentionFormat`, `TestDeliveryRetentionSerialization`.
Names are planned deliverables, not existing passing tests. Before using a -run selector, list tests and confirm every required group exists.
The real repository and retained journal must prove state transitions where named; fakes do not prove database crash semantics.
Read `.agents/skills/tdd/SKILL.md` and its `references/backend-tests.md` at implementation time.

## Verification

Run from repository root. Commands are implementation-time checks, not run during planning.

```bash
(cd apps/backend && go test -trimpath -race ./internal/task/repository/sqlite -run 'TestDeliveryRetention(Schema|Format|Serialization)' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/db -run 'TestSQLiteWriterTransaction|TestSQLiteReaderProgressDuringWriterTransaction' -count=1)
```

Repeat the first two commands with KANDEV_TEST_POSTGRES_DSN set to an isolated real database. The schema tests must use OpenIsolatedPostgres and real startup ordering. Add at least TestDeliveryRetentionSchema/Postgres and TestDeliveryRetentionSerialization/Postgres; a skip is incomplete.
For every environment-gated test, record executed and skipped subtests. A skip cannot complete the corresponding acceptance.
The package helper checks document links/IDs; it does not prove runtime behavior.

## Files likely touched

- `apps/backend/internal/task/models/agent_delivery.go`
- `apps/backend/internal/task/repository/`
- `apps/backend/internal/task/repository/sqlite/agent_delivery.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_recovery_state.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/repository/sqlite/managed_deletion_session.go`
- `apps/backend/internal/persistence/requiredstores/`
- `apps/backend/internal/persistence/storeconformance/`

Add focused `_test.go` files beside changed logic; split oversized tests rather than extending files past lint limits.
Paths marked new are intended deliverables. Other directory references identify the existing integration boundary.

## Dependencies

None. Execute sequentially in the primary session. This and Task 02 form release N; publish neither pruning nor a destructive maintenance entry point. Schema and writer fencing share one safety boundary; record independent multi-connection serialization evidence before consumer work.

## Inputs

- [Requirements](../../specs/platform/requirements/backend-agent-delivery-retention.md).
- [Design](../../specs/platform/system-design/backend-agent-delivery-retention.md); sections matching this scope.
- [Plan](plan.md), including baseline, storage thresholds and engine gates.
- Scoped `apps/backend/AGENTS.md`, existing delivery tests and nearby maintenance patterns.

## Risks

SQLite writer entry is already immediate; injected deferred or aliased test pools can hide real reader/independent-writer behavior. Native busy waits can outlast context cancellation. PostgreSQL lock inversion remains a risk; keep explicit migration readiness.

## Parallelism

`sequential`. No delegation is authorized.

## Results

Implemented additive format-1 retention schema and readiness checks for SQLite and PostgreSQL, including unknown/missing marker rejection, required-index checks, and replay-safe initialization. Added upgrade and interrupted-migration coverage that preserves active, uncertain and projected-but-unsettled evidence with zeroed retention metadata. Added shared transaction advisory fencing for PostgreSQL delivery writes and admission paths; SQLite continues to use immediate writer transactions. Registered retention tables with task-store conformance.

TDD: schema/readiness, format rejection and serialization tests failed before implementation. Focused tests then passed for SQLite and PostgreSQL, including independent PostgreSQL pools. The interrupted migration and legacy evidence fixtures passed on both engines.

Validation passed:

- `go test -trimpath -race ./internal/task/repository/sqlite -run 'TestDeliveryRetention(Schema|Format|Serialization)' -count=1` (SQLite subtests; PostgreSQL-specific subtests are environment-gated and were run in the next command).
- `KANDEV_TEST_POSTGRES_DSN=... go test -trimpath -race ./internal/task/repository/sqlite -run 'TestDeliveryRetention(Schema|Format|Serialization)' -count=1 -v` (SQLite and PostgreSQL schema, format, upgrade and interrupted-migration subtests; independent PostgreSQL serialization test).
- `env -u KANDEV_TEST_POSTGRES_DSN go test -trimpath -race ./internal/persistence/storeconformance -count=1` passed.
- `KANDEV_TEST_POSTGRES_DSN=... go test -trimpath -race ./internal/persistence/storeconformance -count=1` passed against an isolated PostgreSQL 16 database.
- `go run ./cmd/sqlguard ./internal` passed after adding a shared, tested dialect-aware index probe.
- `go test -trimpath -race ./internal/db -run 'TestSQLiteWriterTransaction|TestSQLiteReaderProgressDuringWriterTransaction' -count=1` passed.
- `go test -trimpath ./internal/db -run '^TestTableColumnsAndExists(SQLite|Postgres)$' -count=1`, repeated with `KANDEV_TEST_POSTGRES_DSN`, passed.
- `git diff --check` passed.

The PostgreSQL DSN used a disposable local PostgreSQL 16 container. Task 01 introduces no collector or destructive pruning path.
