---
id: "03-event-retention"
title: "Compact projected events and retire eligible prefixes"
status: done
wave: 3
depends_on:
  - 02-replay-after-pruning
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DELIVERY-RETENTION-001
  - REQ-PLATFORM-DELIVERY-RETENTION-005
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-RETENTION-001.1
  - AC-PLATFORM-DELIVERY-RETENTION-001.2
  - AC-PLATFORM-DELIVERY-RETENTION-005.1
  - AC-PLATFORM-DELIVERY-RETENTION-005.2
system_design:
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
---

# Task 03: Compact projected events and retire eligible prefixes

## Summary

Implement bounded event cleanup that preserves projection and settlement dependencies. Keep pruning callable for tests; the background worker comes later.

## In scope

- Add bounded candidate selection, locked hash/byte validation, retained-receipt compaction and contiguous expired-prefix retirement. Validated raw rows in the exact retiring prefix need no intermediate UPDATE; rows above its checkpoint still persist hashes before payload clearing.
- Remove only known completed generic per-event receipts with their inbox prefix; keep terminal/workflow summaries and unknown effects.
- Until Task 04 provides `fully_settled_at` and the complete predicate, every terminal remains protected and no retirement prefix may cross any terminal. Do not invent a weaker terminal predicate.
- Keep active/uncertain prompt evidence, unprojected rows, protected terminal payloads and pending effects. Permit eligible canonical nonterminal output cleanup without ending active work.
- Add all required indexes and keyset sweep progress. Recheck predicates on the primary writer after lock acquisition.
- TDD failpoints before/after hash validation/backfill, payload clearing/direct-retirement validation, generic effect deletion, retired cursor, marker and progress; reopen and repeat.

## Out of scope

Implementation was authorized by the user on 2026-10-09.
Agentctl journal policy, canonical-history expiry, new UI and feature flags are excluded.
This order does not authorize commit, push, merge or delegation.

## Acceptance

- At each failpoint, rollback preserves full evidence or commit exposes valid hashes/checkpoint. No prefix crosses a gap or pending settlement.
- Batches obey candidate/byte and admitted-work limits, including protected/oversized rows; retries are idempotent. Separate the 250 ms SQL-work limit from native BEGIN waits. Cancelled admission changes no data; join entry/rollback before reuse.
- Until Task 04 lands, terminal payloads stay intact and prefixes stop at the first terminal. Bounds tests exercise the terminal-free million-event expiry frontier plus real SQL candidate/byte caps and first-terminal pinning; Task 08 owns the end-to-end million-row storage run and complete multi-turn bounds.

## TDD evidence

Start each changed-logic scenario with a failing test against the pre-change behavior. Record Red, Green and refactor results.
Use existing backend test helpers, fake clocks and deterministic barriers. Do not use sleeps to manufacture race order.
Required new test groups: `TestDeliveryRetentionEvents`, `TestDeliveryRetentionCrash`, `TestDeliveryRetentionBounds`.
Names are planned deliverables, not existing passing tests. Before using a -run selector, list tests and confirm every required group exists.
The real repository and retained journal must prove state transitions where named; fakes do not prove database crash semantics.
Read `.agents/skills/tdd/SKILL.md` and its `references/backend-tests.md` at implementation time.

## Verification

Run from repository root. Commands are implementation-time checks, not run during planning.

```bash
(cd apps/backend && go test -trimpath -race ./internal/task/repository/sqlite -run 'TestDeliveryRetention(Events|Crash|Bounds|Replay)' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
```

Run the repository matrix again with KANDEV_TEST_POSTGRES_DSN. Assert transactional receipt eviction and monotonic checkpoints using real concurrent connections, including committed UPDATE/DELETE MVCC behavior.
For every environment-gated test, record executed and skipped subtests. A skip cannot complete the corresponding acceptance.
The package helper checks document links/IDs; it does not prove runtime behavior.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/agent_delivery_retention.go` (new)
- `apps/backend/internal/task/repository/sqlite/agent_delivery_retention_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/agent_delivery.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_metrics.go`

Add focused `_test.go` files beside changed logic; split oversized tests rather than extending files past lint limits.
Paths marked new are intended deliverables. Other directory references identify the existing integration boundary.

## Dependencies

Complete `02-replay-after-pruning` and the release N compatibility-reader checkpoint first. Execute sequentially in the primary session. This order starts release N+1 work; no cleanup activation precedes Tasks 04-08 validation.

## Inputs

- [Requirements](../../specs/platform/requirements/backend-agent-delivery-retention.md).
- [Design](../../specs/platform/system-design/backend-agent-delivery-retention.md); sections matching this scope.
- [Plan](plan.md), including baseline, storage thresholds and engine gates.
- Scoped `apps/backend/AGENTS.md`, existing delivery tests and nearby maintenance patterns.

## Risks

A completed coarse submission state does not prove terminal settlement. A protected prefix can keep later hashes longer than the nominal window; expose that growth.

## Parallelism

`sequential`. No delegation is authorized.

## Results

Implemented a bounded transactional collector in `agent_delivery_retention.go`. It selects at most 200 ordered candidates, loads at most 4 MiB of eligible payload, enters through one context capped at 250 ms, acquires shared session/stream locks, and rechecks candidate predicates before mutation. It backfills SHA-256 and original-byte metadata, compacts only valid projected nonterminal output, retires only a contiguous prefix with the exact completed generic effect, and advances the retired cursor and sweep checkpoint in the same transaction. Unknown, pending, unprojected, terminal, workflow and other non-generic evidence stays protected. Hash, payload, receipt, cursor, progress and format-marker failpoints verify rollback and retry behavior.

Bounds coverage distinguishes the pure million-event expiry frontier from database-scale storage evidence: the former advances through a synthetic terminal-free sequence and pins at the first terminal; real SQLite/PostgreSQL rows verify candidate and payload-byte caps, first-terminal protection, replay, receipt deletion and cursor advancement. The million-row database/storage benchmark and completed-terminal multi-turn cases remain assigned to Task 08.

Post-refactor verification from `apps/backend`:

| Command | Result |
| --- | --- |
| `KANDEV_TEST_POSTGRES_DSN=postgres://… go test -trimpath ./internal/task/repository/sqlite -run 'TestDeliveryRetention(Events|Crash|Bounds|Replay)' -count=1` | Passed, SQLite and PostgreSQL matrix; 52.673s. |
| `KANDEV_TEST_POSTGRES_DSN=postgres://… go test -trimpath -race ./internal/task/repository/sqlite -run 'TestDeliveryRetention(Events|Crash|Bounds|Replay)' -count=1` | Passed, SQLite and PostgreSQL matrix; 62.380s. |
| `KANDEV_TEST_POSTGRES_DSN=postgres://… go test -trimpath -race ./internal/task/repository/sqlite -run '^TestDeliveryRetentionPostgresMVCCAndMonotonicCheckpoint$' -count=1` | Passed on real PostgreSQL; separate connections covered committed UPDATE/DELETE MVCC and monotonic checkpoint behavior; 2.767s. |
| `go run ./cmd/sqlguard ./internal` | Passed. |
| `golangci-lint run ./internal/task/repository/sqlite --new-from-rev=d7a44e96ede93d50c6277773f4174de716272f02 --timeout=5m` | Passed, 0 issues. |
| `go test -run '^$' ./internal/task/repository/sqlite` | Passed compile check. |

All retention test groups were listed before selecting them. The PostgreSQL DSN was set, so PostgreSQL subtests executed rather than taking their environment-gated skip. The task-plan TDD chronology from the earlier session was not retained in this checkpoint; this turn reran the permanent event/crash/bounds/replay regressions after refactoring and recorded the green evidence above. No benchmark or UI claim is made here.


Task 08 follow-up, 2026-10-10: the original staged-write receipts above remain historical. Direct retirement is now planned only for the validated contiguous replay-expired prefix. All budget-loaded raw candidates retain independent hash/type/byte checks; a protected gap keeps later compact hashes replayable. Existing hash/payload failure hooks remain validation boundaries even when no intermediate UPDATE is needed. Follow-up trigger RED failed on four SQLite/PostgreSQL legacy/hash-v1 cases (7.224s); first race GREEN passed (6.769s) and expanded crash/reopen/barrier/replay/bounds/hash/cache GREEN passed (69.383s). Scoped lint and SQL guard passed. Task 08 retains ownership of final timing and storage gates.
