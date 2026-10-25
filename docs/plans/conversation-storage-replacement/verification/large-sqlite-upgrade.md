# Large legacy SQLite verification

Date: 2026-09-16. Scope: the uncommitted replacement on
`88c6c0fe0a6ae5d25332070d603b99f7d9241645`. No commit was created.

## Fixture and method

The disposable fixture contained 50,000 agent messages with 8,190 bytes of
synthetic content each. One insert and one update per message produced 100,000
message versions and 100,000 events through the actual historical copy triggers.
The five legacy tables and seven SQLite triggers came from the historical Go
SQL builders at the scope base. The turn-version table was empty; this run
measures message-heavy storage, not a large turn-history workload.

The source database measured 2,953,932,800 bytes (2.75 GiB). The verified backup
measured 2,949,378,048 bytes. Kandev used SQLite 3.51.1 on Linux amd64 and Go 1.26.
These are single-run, warm-cache measurements on a shared development VM.
An initial fixture attempt with an invalid workspace was discarded and corrected.
No developer database or live instance was used.

The opt-in test opens the real single-writer/read-pool pair through
`persistence.Provide`, takes its pre-migration snapshot, runs the production
cleanup transaction, and reopens the populated task repository.
It then calls the real required-store health checker while eight workers update
messages through `UpdateMessageWithConversationReceipt`.

A separate isolated backend uses that upgraded database. After `/ready` returns
200, eight clients PATCH `/api/v1/_test/messages/large-<0..7>` at up to 100 Hz
per client for 60 seconds. That route uses the production receipt transaction
and publishes on the event bus. `/ready` is sampled every 100 ms. The normal
15-second, full-catalog persistence probe remains enabled.
This does not simulate agent adapters or browser subscribers.

## Separate timings

| Measurement | Seconds | Boundary |
| --- | ---: | --- |
| Pre-migration backup | 10.811 | Snapshot creation, validation, and publication, from the production log |
| Open plus backup | 10.815 | Entire `persistence.Provide` call |
| Journal cleanup | 0.625 | Production cleanup transaction only |
| Populated task repository reopen | 0.033 | Task schema initialization after cleanup |
| Backend process to HTTP readiness | 2.227 | Separate launch against the already-cleaned database |
| Snapshot inside that backend launch | 1.402 | Separate post-cleanup snapshot, included in 2.227 seconds |
| Remaining backend startup | 0.825 | Process-to-ready minus that launch's snapshot duration |

These staged measurements must not be added and presented as a single cold
upgrade. Full backend startup includes owners beyond the task repository.
An initial backend launch lacked the agentctl executable; that environment was
corrected before the measured ready/load run.

The former source benchmark incorrectly reopened an empty path for its startup
measurement. It now reopens the populated file and seeds a valid workspace.
The corrected 10,000-message benchmark passed: populated reopen took 14.8 ms.

## Post-readiness writer results

| Run | Updates | Write failures | Probe samples | Probe failures | Worst probe |
| --- | ---: | ---: | ---: | ---: | ---: |
| Repository, 8 workers, 60 seconds | 44,204 | 0 | 599 persistence checks | 0 | 115.891 ms |
| Full backend, 8 clients, 60 seconds | 45,115 | 0 | 596 HTTP readiness checks | 0 | 14.307 ms |

The repository checks use the production two-second deadline and the shared
writer connection, with the task-store table set. The full backend logs show
four successful full-catalog periodic probes after readiness, at 15-second
intervals. There were no `writer ping failed`, `table probe failed`, or
`required persistence probe failed` messages. These results establish a bounded
load pass, not an unrestricted concurrency or long-duration guarantee.

## Retirement audit

The verified pre-upgrade backup contains all five legacy tables and all seven
copy triggers. After cleanup and reopening, the active conversation schema has
only `conversation_session_revisions` and these revision-only triggers:

- `conversation_source_message_insert`, `conversation_source_message_update`,
  `conversation_source_message_delete`
- `conversation_source_turn_insert`, `conversation_source_turn_update`,
  `conversation_source_turn_delete`

No active trigger inserts into the retired event or version tables.
Production source has no `SessionEventLog`, `SessionDeliveryDispatcher`,
`StartSessionEventMaintenanceWorker`, `maintainSessionEvents`,
`syncAllCommittedSessionEvents`, `SyncCommittedSessionEvents`, or ordered-journal
broadcast implementation. Remaining legacy names belong to removal code,
historical documentation, or explicit test fixtures.
A runtime goroutine capture contains one new `runConversationChecks` worker and
none of the retired worker frames.

Seeded `.host/session-events.sqlite`, `-wal`, and `-shm` files were removed at
backend startup and did not reappear during load. PostgreSQL cleanup statements
cover its three historical copy-trigger names, but PostgreSQL execution remains
unverified. This audit does not remove unrelated workflow or logging journals.

## Explicit compaction

Cleanup left the live file at 2,953,932,800 bytes, with 605,351 free pages out of
721,175. `auto_vacuum` was zero. No startup compaction was added.

After stopping the isolated backend, an explicit offline `VACUUM` took 2.961
seconds and reduced the file to 477,069,312 bytes. Free pages fell to zero.
`PRAGMA integrity_check` returned `ok`; all 50,000 source messages remained.
The compacted database then reached HTTP readiness in 0.413 seconds.

The [operations guide](../../../public/operations.md#compact-sqlite-after-the-conversation-upgrade)
now separates compaction from startup and from backup `VACUUM INTO`, explains
maintenance disk requirements, and gives explicit UI and offline procedures.
Keep rollback snapshots outside automatic retention: repeated pre-ready starts
can produce further snapshots and prune older ones.

## Reproduction and retained evidence

Run the opt-in test from `apps/backend`, with at least 10 GiB of free disk:

```bash
KANDEV_LARGE_UPGRADE_DIR=/absolute/disposable/empty-directory \
  go test ./internal/task/repository/sqlite \
  -run '^TestConversationLargeLegacyUpgrade$' -count=1 -v -timeout=15m
```

It retains `<directory>/data/kandev.db`, backups, and `persistence.log`.
Only use a disposable fixture for the backend HTTP workload described above.
The measured backend was built from the current checkout and launched with
`__backend`, isolated home/database/ports, `KANDEV_E2E_MOCK=true`, mock providers,
and the agentctl binary on PATH. Temporary instances were stopped after each
run. Temporary databases and binaries are removed after evidence collection.

For a complete disposable reproduction, run from `apps/backend`:

```bash
scratch=$(mktemp -d)
KANDEV_LARGE_UPGRADE_DIR="$scratch/data" go test ./internal/task/repository/sqlite \
  -run '^TestConversationLargeLegacyUpgrade$' -count=1 -v -timeout=15m
go build -o "$scratch/kandev" -ldflags '-X main.Version=large-upgrade-verification' ./cmd/kandev
go build -o "$scratch/agentctl" ./cmd/agentctl
python3 ../../docs/plans/conversation-storage-replacement/verification/runtime_verify.py "$scratch"
```

Inspect `runtime-results.json` and the backend logs for failures. The diagnostic
script stops its backend on exit and retains its scratch directory for inspection.
Remove that invocation-owned directory when finished. The driver is for the
synthetic fixture only, not for a running instance or a production database.

Retained evidence: [metrics](large-sqlite-results.json),
[test output](large-sqlite-test.log), [legacy inventory](legacy-backup-inventory.txt),
and [runtime persistence logs](runtime-health-evidence.log).
PostgreSQL, browser E2E, and the unrelated full-backend failures remain separate
plan gates; this verification does not mark them complete.

## Validation status

The large opt-in test, corrected benchmark, cleanup regression test, public-doc
validator, specification validators, and whitespace checks passed. The normal
opt-in test invocation skips the large workload unless its environment variable
is set.

Package-wide `golangci-lint run ./internal/task/repository/sqlite/...` still
reports six issues in the pre-existing replacement implementation: function
length in `initSQLiteConversationSourceTriggers`, four constant-reuse findings
in `conversation_source.go` and `conversation_receipts.go`, and nested complexity
in `UpdateMessageWithConversationReceipt`. It reports none in the added large
fixture test or corrected benchmark. These findings remain an implementation
gate; this workload verification is not a full lint or merge-readiness pass.
