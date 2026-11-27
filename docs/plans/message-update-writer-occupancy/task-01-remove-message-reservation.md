---
id: "01-remove-message-reservation"
title: "Remove redundant message reservation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001
acceptance_criteria:
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.3
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.4
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.3
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.7
system_design:
  - ../../specs/system-page/system-design/tool-payload-retention.md
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
---

# Task 01: Remove redundant message reservation

## Summary

Use existing immediate transaction admission instead of an extra message-row UPDATE.
Keep message errors, retained-payload protection, and conversation mutation receipts equivalent.

## In scope

- `updateMessageWithPayloadGuardTx` and tests of its three production callers.
- Missing-message error compatibility, rollback, retained metadata, and revision assertions.
- Update the disposable measurement overlay and record fixed-path comparisons.

## Out of scope

Other reservation statements, transaction policy changes, production instrumentation, health changes, live load, and filesystem remediation.

## Acceptance

1. Each changed-message caller performs one message-row UPDATE inside its existing transaction; the count regression fails at two before implementation.
2. Missing rows return the existing error, failed updates roll back, and stale writes preserve removal markers, metadata, payload identity, timestamps, and receipts.
3. Targeted correctness checks pass; three alternating benchmark pairs retain actual results and limitations without an arbitrary timing assertion or claim that 503s are fixed.

## Implementation sequence

1. Mark this work order in progress. Add `message_payload_writer_test.go` using a disposable file-backed database and the real writer factory.
2. Add `TestGuardedMessageUpdateWritesRowOnce` with ordinary, receipt, and changed agent-plan cases.
   Seed each case, then install a test-only audit trigger. Assert one audit entry and the stored result.
   Run it before production changes and retain the expected count-two failure.
3. Add `TestGuardedMessageUpdateMissingRow`, `TestGuardedMessageUpdateRollback`, and `TestReceiptUpdatePreservesPayloadRemovalAfterStaleRead`.
   Preserve exact missing-row text, row/receipt rollback, and stale metadata protection. Existing invariants can already pass before the fix.
4. Remove the redundant SQLite message reservation block and map SELECT `sql.ErrNoRows` on both engines.
   Keep all three callers' transaction ownership, PostgreSQL row locking, and other reservation statements intact.
5. Run the targeted checks below. Reuse the existing actual-factory admission tests; do not replace them with sleeps or mocked locks.
6. Change `measure.py` to observe the fixed path by default and restore the original reservation block only under `--restore-message-reservation`.
   Reject the obsolete removal option with a clear error. Update historical evidence's reproduction note without rewriting measured history.
7. Run three alternating benchmark pairs sequentially. Save structured results and commands in this package's `implementation-evidence.md` and `measurements.json`.
   Report entry and admitted-span distributions, elapsed time, probe outcomes, and sidebar outcomes.
8. Record every command result and any actual PostgreSQL skips. Mark done only after applicable gates pass; synchronize the plan.

## Verification

Run from the repository root. Test names below include planned new deliverables.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run 'Test(GuardedMessageUpdate|ReceiptUpdatePreserves|UpdateMessage|CreateMessage|.*Payload|Conversation(Source|Mutation)|.*AgentPlan|WriterWorkload)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/db -run '^TestSQLiteWriterTransaction' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/persistence/requiredstores ./internal/backendapp -run 'Test(PersistenceContentionFixture|RuntimeHealth|StartupHealth|HealthCheck|ProbeTables|RequiredPersistence|PersistenceMiddleware)' -count=1)
python3 docs/plans/database-writer-contention/measure.py -- go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run '^TestWriterWorkload' -count=1
for pair in 1 2 3; do
  python3 docs/plans/database-writer-contention/measure.py --restore-message-reservation -- go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run '^$' -bench '^BenchmarkWriterMessageUpdate$/^bytes=16777216$' -benchtime=128x -count=1
  python3 docs/plans/database-writer-contention/measure.py -- go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run '^$' -bench '^BenchmarkWriterMessageUpdate$/^bytes=16777216$' -benchtime=128x -count=1
done
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Capture benchmark output per run when recording results. Do not overlap these runs with correctness tests.
The generic race command can skip PostgreSQL tests when no disposable server is configured.
For engine evidence, use a disposable database provisioned through the existing test harness, then run:

```bash
(cd apps/backend && test -n "${KANDEV_TEST_POSTGRES_DSN:-}" && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run '^Test(PostgresAgentPlanUpsertSerializesAcrossConnections|ConversationPostgresSource|ConversationPostgresReceipts)$' -count=1 -v)
```

Never point that variable at the live installation. A missing test server is a recorded PostgreSQL validation gap, not a passing result.
No PostgreSQL isolation or SQL-lock expression change is intended; actual integration evidence remains required before claiming engine validation.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/message_payload_replay.go`
- `apps/backend/internal/task/repository/sqlite/message_payload_writer_test.go` (new)
- Existing `message_payload_replay_test.go`, `message_payload_test.go`, and `conversation_source_test.go` if assertions fit better there.
- `docs/plans/database-writer-contention/measure.py`
- `docs/plans/database-writer-contention/evidence.md` (historical reproduction note only)
- This package's manifest, work order, implementation evidence, and measured results.

## Dependencies

The completed [writer investigation](../database-writer-contention/task-01-attribute-writer-occupancy.md) supplies the fixture and benchmark.
The accepted factory admission change is already in the checkout.

## Risks

The removal-only prototype changes missing-row errors. Do not copy it as a complete patch.
Preserve transaction guards for every caller, and do not remove similar-looking session locks in this task.
Benchmark variability and remaining health timeouts must remain visible in the result.

## Parallelism

`sequential`

## Inputs

- [Plan and test mapping](plan.md)
- [Investigation evidence](../database-writer-contention/evidence.md)
- [Tool-payload retention requirements](../../specs/system-page/requirements/tool-payload-retention.md)
- [Tool-payload retention design](../../specs/system-page/system-design/tool-payload-retention.md#guarded-message-replacement)
- [Platform persistence design](../../specs/platform/system-design/postgres-domain-store-parity.md#sqlite-writer-transaction-admission)
- [Writer-admission ADR](../../decisions/2026-10-05-sqlite-writer-transaction-admission.md)
- `conversation_receipts.go`, `message_agent_plan.go`, and the existing source/receipt tests beside the changed helper.

## Results

Completed. The message reservation UPDATE was removed from `updateMessageWithPayloadGuardTx`; SELECT no-row errors retain the compatibility message on both engines. The audit-trigger regression failed at two writes per ordinary, receipt, and changed agent-plan caller before the code change, and passes at one afterward. Missing-row mapping, rollback with unchanged message and conversation revision, and stale receipt handling with externalized payload identity are covered.

- SQLite race-enabled message/payload/conversation test selection passed, as did the SQLite writer-admission tests and persistence/health regression selection.
- Disposable PostgreSQL 17 race-enabled tests passed for missing-row mapping, agent-plan serialization, conversation source, and receipts.
- The fixed-path writer workload control passed. Three alternating 128-update, 16 MiB pairs reduced workload elapsed time in every pair. Four health deadlines remain in the fixed-path samples; see [`implementation-evidence.md`](implementation-evidence.md) and [`measurements.json`](measurements.json).
- `measure.py` defaults to the fixed source path and restores the reservation only in a disposable build overlay. The obsolete `--omit-message-reservation` option is rejected.
- The original contention investigation's `measurements.json` remains unchanged; its reproduction note links to these new results.
