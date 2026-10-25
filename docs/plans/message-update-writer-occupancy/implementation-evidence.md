# Message update writer occupancy implementation evidence

## Result

The implementation removes the extra SQLite message-row reservation update. The metadata read and authoritative replacement remain inside each caller's admitted transaction. PostgreSQL continues to use `SELECT ... FOR UPDATE`. Missing rows retain the `message not found: <id>` mapping on both engines.

A disposable file-backed SQLite audit-trigger test first failed before the change with two row updates for each of the ordinary, conversation-receipt, and changed agent-plan callers. It passes with one update per caller. Additional tests preserve missing-row behavior, transaction rollback, and removed-payload metadata, payload identity, timestamp, receipt revision, and receipt contents after a stale read.

## Measurements

Three alternating pairs of the 16 MiB `BenchmarkWriterMessageUpdate` ran 128 updates on the eight-session disposable fixture. Every pair ran the restored reservation case first and the fixed path second. The full structured samples and raw Go output are in [measurements.json](measurements.json) and [runs](runs/).

| Pair | Restored seconds | Fixed seconds | Elapsed reduction | Entry p50, restored → fixed | Entry p95, restored → fixed | Admitted span p50, restored → fixed | Admitted span p95, restored → fixed | Health deadlines, restored → fixed | Sidebar errors, restored → fixed |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 15.183 | 8.323 | 45.2% | 547.0 → 255.9 ms | 2152.5 → 1306.2 ms | 105.1 → 49.2 ms | 190.3 → 175.6 ms | 5 → 2 | 0 → 0 |
| 2 | 13.462 | 6.320 | 53.0% | 494.2 → 251.7 ms | 2166.3 → 912.5 ms | 94.5 → 37.2 ms | 176.6 → 101.6 ms | 6 → 1 | 0 → 0 |
| 3 | 13.253 | 6.123 | 53.8% | 563.6 → 239.5 ms | 1727.2 → 921.7 ms | 79.1 → 39.8 ms | 189.2 → 94.3 ms | 4 → 1 | 0 → 0 |

The fixed path reduced elapsed time in all three pairs (45.2%, 53.0%, and 53.8%). It also reduced writer-wait milliseconds and the entry p50/p95 in each pair. The fixed runs still recorded four health deadlines in aggregate; this optimization improves this controlled workload but does not establish full interactive recovery. Sidebar reads succeeded in all six runs. Machine, fixture, complete distributions, writer wait, success counts, command strings, and limitations are recorded in the JSON.

## Verification

- Red: `TestGuardedMessageUpdateWritesRowOnce` failed at two updates for all three production callers before the implementation.
- Green: `go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run 'Test(GuardedMessageUpdate|ReceiptUpdatePreserves)' -count=1` passed.
- `go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run 'Test(GuardedMessageUpdate|ReceiptUpdatePreserves|UpdateMessage|CreateMessage|.*Payload|Conversation(Source|Mutation)|.*AgentPlan|WriterWorkload)' -count=1` passed.
- SQLite writer-admission race tests and persistence/health regression tests passed.
- Disposable PostgreSQL 17 tests for missing-message mapping, agent-plan locking, conversation source, and receipts passed.
- The writer-workload control passed with the fixed path as the default overlay.
