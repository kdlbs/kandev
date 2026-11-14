---
id: "02-lifecycle-migration"
title: "Preserve history through deletion and migration"
status: in_progress
wave: 2
depends_on: ["01-retained-admission"]
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-HISTORY-001
  - REQ-COSTS-USAGE-HISTORY-002
  - REQ-COSTS-USAGE-HISTORY-007
acceptance_criteria:
  - AC-COSTS-USAGE-HISTORY-001.1
  - AC-COSTS-USAGE-HISTORY-001.2
  - AC-COSTS-USAGE-HISTORY-001.3
  - AC-COSTS-USAGE-HISTORY-001.4
  - AC-COSTS-USAGE-HISTORY-001.5
  - AC-COSTS-USAGE-HISTORY-001.6
  - AC-COSTS-USAGE-HISTORY-002.5
  - AC-COSTS-USAGE-HISTORY-002.6
  - AC-COSTS-USAGE-HISTORY-002.7
  - AC-COSTS-USAGE-HISTORY-007.1
  - AC-COSTS-USAGE-HISTORY-007.2
  - AC-COSTS-USAGE-HISTORY-007.3
  - AC-COSTS-USAGE-HISTORY-007.6
  - AC-COSTS-USAGE-HISTORY-007.7
system_design:
  - ../../specs/costs/system-design/usage-history.md
---

# Task 02: Preserve history through deletion and migration

## Summary

Backfill existing native and external usage through restart-safe batches.
Integrate retained accounting with archive, delete, pruning, erasure, and concurrent admission.

## In scope

- Add durable high-water marks and immutable keyset cursors. Resume after every migration crash point without duplication.
- Preserve old measurements before task cascade, using durable deletion admission and bounded preservation batches.
- Fence session pruning before it clears usage links or cascades the turn evidence required for historical attribution.
- Serialize preservation completion/final deletion with native and external writes in SQLite and PostgreSQL.
- Detach live references while preserving original subject incarnation, scope, dates, models, and amounts.
- Keep task ledger cascade and task-session APIs intact. Late retained observations require previously validated execution bindings.
- Register every new table/checkpoint with existing workspace/org erasure, suspension policy, and E2E reset.
- Add explicit retained-scope erasure by captured org ID, independent of live workspaces. Fence concurrent writers and every rebuild generation.
- Preserve historical ownership across reassignment and flag changes. Leave unsupported historical identities unknown.
- Document safe read rollback and unsafe old-binary deletion paths. Do not rebuild all history on each startup.

## Out of scope

- New erasure controls, generic orphan admission, exact recovery of absent data, and statistics UI changes.

## Acceptance

- Archive, task deletion, session pruning, ID reuse, and uninstall leave retained amounts intact.
  Explicit erasure removes captured org history, including transferred-workspace scopes, without worker resurrection.
- A concurrent delete cannot erase a source row before its retained copy commits. Failures leave source data and retryable cleanup.
- Backfill interruption, repeated startup, flag changes, and non-UTC PostgreSQL sessions do not duplicate or reassign usage.

## Verification

```bash
(cd apps/backend && go test -race -trimpath ./internal/analytics/... ./internal/task/repository/sqlite ./internal/task/service -run 'TestUsageHistory|TestManagedDeletion|TestArchiveTask' -count=1)
(cd apps/backend && go test -race -trimpath ./internal/org/... ./internal/backendapp -run 'TestUsageHistory|TestOrgDelete|TestOrgSuspend' -count=1)
(cd apps/backend && : "${KANDEV_TEST_POSTGRES_DSN:?Set an isolated PostgreSQL test DSN}" && go test -trimpath ./internal/analytics/... ./internal/task/repository/sqlite ./internal/org/... -run 'TestUsageHistoryPostgres' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/analytics/repository/sqlite/usage_history_migration*.go` and lifecycle tests (new)
- `apps/backend/internal/task/repository/sqlite/{task.go,resource_cleanup.go,usage_events.go}`
- `apps/backend/internal/task/service/{managed_deletion.go,resource_cleanup_preparation.go}`
- `apps/backend/internal/org`, `internal/backendapp/orgs.go`, and `task/service/service_resources.go` (`DeleteOrganizationWorkspaces`)
- `apps/backend/internal/backendapp` startup/reset wiring and database migration metadata

## Dependencies

Task 01. Retained schema and native admission must exist before destructive lifecycle cutover.

## Risks

A process-local lock cannot protect cross-process PostgreSQL deletion races.
Preservation must not require an unavailable transcript tool or keep a volatile-only migration cursor.
Restore and binary downgrade need separate treatment from reverting the read path.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/costs/requirements/usage-history.md), REQ-001/002/007 and listed ACs.
- [Design](../../specs/costs/system-design/usage-history.md), Retained identity, Native admission, Migration and recovery.
- Existing task deletion admission, org/workspace erasure services, and [native ledger contract](../../specs/task-cost-ledger/spec.md).

## Results

### Current integration checkpoint

Current-main integration passed 390 normal analytics results and 77 focused lifecycle/history-cleanup results normally and under race.
Persistence conformance passed 644 race results across 42 adapters on SQLite and PostgreSQL 18.6.
These counts overlap and are not a combined count of distinct tests.

Browser logs then exposed a stamped-turn transaction defect with retention enabled.
`CreateTurnWithStepStamp` now uses `BeginTxx`, preserving workflow stamps and atomic turn/binding admission.
`TestUsageHistoryStampedTurnTransaction` and its PostgreSQL counterpart reproduce the original failure and pass after the correction.
They verify active and completed turns, trusted actor/org attribution, native ledger admission, and rollback followed by retry.
Focused normal and race runs pass on both engines. Go formatting, scoped vet, and changed-line lint pass. Fresh browser validation remains pending.
The full analytics/persistence receipts above predate this narrow correction.

Earlier implementation and review receipts follow as historical evidence.

Implemented durable 200-row high-water/keyset backfill for native and external sources, unknown-identity migration gaps, preservation fences for task deletion and session pruning, captured-scope erasure/reset, and durable live-link detachment. Source copies and cursor advancement commit together; task/session writes are fenced during preservation, and final detachment shares the destructive transaction. Organization erasure uses captured organization ownership after workspace transfer. Native ledger cascade and task rollup behavior remain intact.

Verification completed (resource-capped with `GOMAXPROCS=2` and `-p 1`):

- `(cd apps/backend && GOMAXPROCS=2 go test -p 1 -race -trimpath ./internal/analytics/... ./internal/task/repository/sqlite ./internal/task/service -run 'TestUsageHistory|TestManagedDeletion|TestArchiveTask' -count=1)` — passed; includes the execution-binding immutability regression and task/native/external admission race checks.
- `(cd apps/backend && GOMAXPROCS=2 go test -p 1 -race -trimpath ./internal/org/... ./internal/backendapp -run 'TestUsageHistory|TestOrgDelete|TestOrgSuspend' -count=1)` — command succeeded, but these package test names matched no tests. The actual organization lifecycle regression passed separately: `(cd apps/backend && GOMAXPROCS=2 go test -p 1 -race -trimpath ./internal/backendapp -run '^TestOrganizationDeletionUsesWorkspaceLifecycle$' -count=1)`.
- With PostgreSQL 18 credentials loaded silently from `/tmp/usage-history-pr3660-postgres.env`: `(cd apps/backend && set -a; . /tmp/usage-history-pr3660-postgres.env; set +a; GOMAXPROCS=2 go test -p 1 -trimpath ./internal/analytics/... ./internal/task/repository/sqlite ./internal/org/... -run '^TestUsageHistoryPostgres' -count=1)` — passed, including `TestUsageHistoryPostgresTaskDeletePreservesLegacySources`, `TestUsageHistoryPostgresCapturedOrgErasureAfterWorkspaceTransfer`, and the Task 01 native admission/retention test.
- `python3 scripts/list-docs.py validate` — passed (417 decisions, 1828 specifications).
- `python3 scripts/lint-spec-files.py --all` — passed.
- `git diff --check` — passed.
- Final-tree follow-up after formatting the task-ID reuse regression: `(cd apps/backend && GOMAXPROCS=2 go test -p 1 -race -trimpath ./internal/task/repository/sqlite -run '^TestUsageHistoryTaskIDReuseKeepsPriorIncarnation$' -count=1)` — passed. This follow-up changed test formatting only; no production code changed after the broader checks above.

Logs: `/tmp/usage-history-agent2-race-core.log`, `/tmp/usage-history-agent2-race-org.log`, `/tmp/usage-history-agent2-org-lifecycle-race.log`, `/tmp/usage-history-agent2-postgres.log`. Task 03 can use `usagehistory.Retainer.AdmitBindingTx` for transaction-bound immutable ownership admission; `analytics.Repository.UpsertSessionUsage` calls `AdmitUsageWriteTx` in its mutation transaction and queues the old and new `(scope_id, subject_id, coverage_key-or-lifetime)` partitions through `enqueueUsagePartitionTx`. Its correction path should reuse those same-transaction checks and invalidations when it adds retained-binding corrections and target discovery. Task 04 owns projection sequencing, replay, and generations.

Bounded lifecycle lint cleanup handoff: split preservation, legacy preservation, and erasure into `usage_history_preservation.go`, `usage_history_legacy_preservation.go`, and `usage_history_erasure.go`. Extracted cursor handling, legacy scope/subject/actor resolution, live-session validation, and organization erasure batches; removed the empty workspace-erasure branch. Task 03 target refresh, source indexing, selection backfill, and contract-table erasure calls remain in their original transaction/order. Reused `usageSourceNative`, `usageForUpdate`, `usageLifetime`, and `usageUnknown`. Applied `gofmt` only to the five owned Go files. Prior pass receipts above are preserved; verification of this refactor remains pending for Worker 3's combined changed-file lint, targeted race, and real PostgreSQL run. Shared `task`/`session` and cost-source string constants still need cross-file coordination after all workers freeze. No lint, test suite, or race run was launched for this cleanup.

R1-F data-loss correction: preservation now returns the last source key actually retained by each bounded page. The page copies and that key still commit in one transaction. Removed the queries that selected the end of the entire remaining range. Native ID gaps, external byte ordering, existing captured-scope resolution, fences, batch limits, and completion conditions remain intact.

Added `usage_history_preservation_pages_test.go` and `usage_history_preservation_retry_test.go`. Each fixture seeds 451 native events, 451 legacy measurements, and 451 legacy buckets. Actual task deletion and session pruning verify every retained source identity, token amount, cost, scope, and unknown legacy actor. Unrelated task IDs are interleaved; pruning also preserves a separate live session. Second-page faults in native, measurement, and bucket retention verify rollback to exactly the first 200 committed records, unchanged durable cursors and high-water marks, rejected writes while fenced, live source preservation, repository reconstruction, and successful retry before the live cascade.

Focused R1-F verification:

- RED before the production fix: `(cd apps/backend && GOMAXPROCS=1 go test -p 1 -race -trimpath ./internal/task/repository/sqlite -run '^TestUsageHistory(TaskDelete|SessionPrune)PreservationPages$/all_pages$' -count=1 -v)` — failed in both actual lifecycle cases with only 200/451 native records retained and an unscoped external tail. Receipt: `/tmp/usage-history-agent2-r1f-red-sqlite.log`.
- Final SQLite race: `(cd apps/backend && GOMAXPROCS=1 go test -p 1 -race -trimpath ./internal/task/repository/sqlite -run '^TestUsageHistory((TaskDelete|SessionPrune)PreservationPages|TaskDeleteBackfillsLegacyAndDetachesLinks|SessionPrunePreservesSubjectBeforeDetachment)$' -count=1 -v)` — passed, 48.364s. Receipt: `/tmp/usage-history-agent2-r1f-green-sqlite.log`.
- Final PostgreSQL race: after silently sourcing `/tmp/usage-history-pr3660-postgres.env`, `(cd apps/backend && GOMAXPROCS=1 go test -p 1 -race -trimpath ./internal/task/repository/sqlite -run '^TestUsageHistoryPostgres(TaskDelete|SessionPrune)PreservationPages$' -count=1 -v)` — passed, 61.269s, on PostgreSQL 18.6 with session timezone `Asia/Kathmandu`. Receipt: `/tmp/usage-history-agent2-r1f-green-postgres.log`.
- `gofmt -l` on the three R1-F Go files — no output.

The first PostgreSQL run exposed a separate bare-task-store pruning defect: `DeleteSessionInTransaction` inserts into absent `queue_session_locks`, while the task session cleanup caller swallows the missing-table error and PostgreSQL retains an aborted transaction. That receipt remains at `/tmp/usage-history-agent2-r1f-postgres-missing-queue.log`. Final parity fixtures initialize the real production messagequeue repository on both databases; assertions remain unchanged. This separate missing-queue lifecycle defect was reported to the coordinator and is outside the reserved preservation-file fix.

Frozen R1-F source hashes are recorded in `/tmp/usage-history-agent2-r1f-hashes.sha256`:

- `usage_history_preservation.go`: `7a82a17bf7de0989a503e88fd94d905d490fad8b1f80f781aa5e8cf800c5ba90`
- `usage_history_preservation_pages_test.go`: `db55a42c408b005d2c4286517d0593300785f7ea7f9b11105bc15c9da309dbd1`
- `usage_history_preservation_retry_test.go`: `2d65d391ef49f4f467f944cc8ad3da60de3a62fa2386413a0ec5bcaa5e3f387c`

R1-C current-target/source-safety handoff (additional coordinator assignment): implemented exact current task/session/transcript lookup and issued retained-binding revalidation in the existing target API. Successful point responses acknowledge `CurrentTarget` and contain at most one target, without snapshot/preparation state. Canonical live metadata and resume-token lookups use exact indexes. PostgreSQL point reads lock current workspace ownership. Actor identity includes actor kind; current actor/source evidence never rewrites issued snapshots. Unknown, mixed, revoked and cross-scope contexts remain unavailable. Frozen import snapshots/keysets and the R1-F preservation source remain intact.

Added hash-only source guards and workspace memberships. Production org/workspace erasure preserves these privacy controls. Legacy binding guards seed in durable 200-row org-erasure pages before source purge; failure/restart preserves the actual cursor and source evidence. The gated E2E reset clears only its workspace's guard membership, preserves other workspace guards, advances fences and revokes old grants, allowing new fixture incarnations to reuse transcript IDs. Session-creation capture now observes durable scope-erasure fences and cannot recreate erased metadata. Captured migration gaps now allow erasure of withheld native ledger rows after workspace transfer; workspace reset also clears these source rows. The rollback regression compares complete pre/post table snapshots against legitimate creation-capture state and still requires zero ledger/rollup effects.

Worker4 supplied the additive SDK/protobuf, four-table schema/catalog registration and live-source indexes. Worker2 did not edit those files. The source identity contract is unnamespaced: exact transcript collisions across scopes fail closed; no provider/plugin/actor identity is guessed. Unproven legacy context stays unknown.

Final R1-C verification, all with one process and no skipped cases:

- After silently sourcing `/tmp/usage-history-pr3660-postgres.env`: `(cd apps/backend && GOMAXPROCS=1 go test -p 1 -race -trimpath ./internal/analytics/repository/sqlite ./internal/task/repository/sqlite -run '^TestUsageHistory((Postgres)?(CurrentTarget[^/]*|FreshSessionCannotReuseErasedTranscript|ProductionWorkspaceEraseKeepsSourceGuards|ErasureIncludesWithheldNativeSource|SourceGuard(ConcurrentFirstOwners|ErasureResumesBoundedPages)|SessionCreationCannotReopenErasingScope)|NativeRollbackAtomic|OrganizationErasureUsesCapturedOrganizationAfterWorkspaceTransfer)$' -count=1 -v)` — passed: 27 analytics entry tests (10 actual PostgreSQL), 85.007s; both original task regressions, 5.562s. PostgreSQL 18.6 used isolated schemas and `Asia/Kathmandu`. Receipt: `/tmp/usage-history-agent2-current-final-race.log`.
- `(cd apps/backend && CGO_ENABLED=1 CGO_CFLAGS='-O2 -g -DSQLITE_ENABLE_STMT_SCANSTATUS' GOMAXPROCS=1 go test -p 1 -race -trimpath -tags 'sqlite_trace usage_history_scanstatus' ./internal/analytics/repository/sqlite -run '^TestUsageHistoryCurrentTargetConstantScanVisits$' -count=1 -v)` — passed, 6.284s. Actual total NVISIT before/after 902 unrelated legacy/retained sessions: task 121/121, session 121/121, transcript 154/154, retained binding 122/122. Plans use `idx_usage_live_transcript` and `idx_usage_live_resume`; each live transcript seek visits one matching row. Receipt/plans: `/tmp/usage-history-agent2-current-scan-visits-green.log`.
- `gofmt -l` on the 22 owned R1-C Go files — no output.

Meaningful RED receipts are preserved: erased transcript reuse (`current-source-red.log`), missing point resolution (`current-point-red.log`), actor-kind collision (`current-actor-red.log`), current metadata/fixture corrections (`current-sqlite-race.log`), erasing-scope creation (`current-birth-fence-red.log`) and missing live index (NVISIT 121 to 1925, `current-scan-visits.log`), all under `/tmp/usage-history-agent2-`. None are counted as final passes.

Frozen handoff, exact changed files/commands and hashes: `/tmp/usage-history-agent2-current-handoff.md`, `/tmp/usage-history-agent2-current-receipt.json`, `/tmp/usage-history-agent2-current-hashes.sha256`. Integrated lint/vet/full integration remain coordinator-owned. Work-order status stays `in_progress` for handoff.

Coordinator review found that an ownership gap could let ordinary deletion cascade away uncopied native ledger amounts.
Preservation now verifies an accepted copy before advancing each page and again before the destructive transaction completes.
Failures preserve the live task/session, every raw ledger value, and the last committed cursor.
Regressions cover 2/451 rows, restart, stale completed checkpoints, recovery, captured-org erasure, and different task births.
Verified copies keep their original workspace and organization after the task moves through the real transfer API.
The transfer registry explicitly approves captured accounting relations while retaining its unknown-owner guard.
Its actual-API ownership, replay, live-routing, and unknown-table controls passed 100 normal and 100 race results.
The final transfer-then-delete/prune cases passed eight normal and eight race results on SQLite and PostgreSQL 18.6.
Final affected-package lint, vet, SQL guard, and formatting passed.
These runs overlap; their counts are not a combined count of distinct tests.
The positive migration fixture also passed seven normal and seven race cases without weakening its existing assertions.
Integration on current main, final browser evidence, pushes, and current-head CI remain coordinator-owned and pending.
