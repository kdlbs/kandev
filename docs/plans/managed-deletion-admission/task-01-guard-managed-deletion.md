---
id: "01-guard-managed-deletion"
title: "Guard managed deletion against stale revisions"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-MANAGED-COORDINATION-013
acceptance_criteria:
  - AC-PLUGINS-MANAGED-COORDINATION-013.1
  - AC-PLUGINS-MANAGED-COORDINATION-013.2
  - AC-PLUGINS-MANAGED-COORDINATION-013.3
  - AC-PLUGINS-MANAGED-COORDINATION-013.4
  - AC-PLUGINS-MANAGED-COORDINATION-013.5
  - AC-PLUGINS-MANAGED-COORDINATION-013.6
  - AC-PLUGINS-MANAGED-COORDINATION-013.7
  - AC-PLUGINS-MANAGED-COORDINATION-013.8
system_design:
  - ../../specs/plugins/system-design/managed-deletion-admission.md
---

# Task 01: Guard managed deletion against stale revisions

## Summary

Deliver one coherent typed lifecycle/native admission correction with real
independent-service evidence and registered Host receipts. ROOT selected native
exclusive admission with admitted partial-effects compatibility; physical
removal is a separate commit. ROOT reviewed all four artifacts and issued the
later explicit implementation release in this same primary. Implementation and
local task-defined validation pass; delivery is incomplete. ROOT also reviewed the shared SQLite writer-factory extension and issued its
later implementation release. Local acceptance checks pass; hosted review and delivery remain pending.

## In scope

- At implementation release, correct design-only phase wording and mark this
  order in progress. Establish meaningful existing-API RED before
  correcting production behavior, using real task Service deletion and
  production conversation services/state persistence.
- Implement the reviewed typed lifecycle and native admission/commit boundary,
  participating current identity/revision/detach/cleanup writers, owner-only
  reservation handling, and necessary typed native adapters only. Required
  planned types are `managed.DeleteRequest`/`DeleteClaim`; native methods are
  `AdmitManagedDeletion`, `InspectManagedDeletion`, `FinalizeManagedDeletion`,
  `ReleaseManagedDeletion`; typed real lifecycle is
  `Service.DeleteManagedConversationTask`. Snapshot owner/phase fencing extends
  existing cleanup storage without schema changes. Keep canvas-failure abort.
- Preserve actual resource cleanup, hierarchy, queues, transcript identity,
  receipts, error classifications, rollback/cancellation, and postcommit truth.
- Prove independent SQLite and PG16 physical admission and final-delete
  behavior plus real registered Host receipts. Resolve any causal required
  review/CI finding and deliver through verified merge under plan constraints.

## Out of scope

Schema/wire/auth/UI/feature-flag changes, generic callback/version framework,
bare SQL lifecycle replacement, unsafe optional fallback, general cleanup-engine
rewrite, local full browser/E2E/build, delegation, new tasks/tabs/sessions,
model switching, immutable proof replay, unrelated hosted failure remediation.

## Acceptance

1. Current admission and final deletion implement the ROOT-reviewed design;
   competing accepted update/detach wins are retained, and deletion wins reject
   competing authority without deadlocks or stale resurrection (.1-.3, .7).
2. Real failure/cancellation/rollback and replacement/absence observations
   establish no effects before admission, retained task/primary/transcript on
   own failed final deletion, admitted partial-effects compatibility, and honest
   accepted/postcommit outcomes with lifecycle/hierarchy preserved
   (.2, .4, .5, .8).
3. Actual registered Host/SQL receipts and exact compatibility commands pass;
   joined verification and authorized delivery meet the plan's actual merge
   and owned cleanup completion gate (.6-.8).

## Existing-API RED and new permanent evidence

The lifetime test catalog was reviewed and released. Its DB factory extension
below is also reviewed and released. Use the serial resource limits.
Use the real `createTestService` pattern or equivalent native fixture, two
independent production conversation services, independent SQLite connection
pools against the same file, and real `state.Store`. Initialize owners before
opening independent handles. Add a retained message and verify it with the
task and primary session, rather than checking only task absence.

| Permanent test and file | Required cases and AC coverage |
| --- | --- |
| `TestManagedDeletionAdmissionInterleavings`, `internal/task/service/managed_deletion_admission_test.go` | Existing API RED update-after-advisory-read and detach-after-read; current-delete and already-stale controls; revision and detach ordering both directions, replacement/missing and concurrent ordinary removal (.1-.3, .5, .7, .8) |
| `TestManagedDeletionAdmissionFailureEffects`, same file | Real SQL admission rollback and cancellation: zero rejected effects. After admitted borrower transfer/canvas success, final purge failure or canvas error: task/primary/transcript retained, partial effects honestly possible, no task.deleted/runnable destruction; actual current-delete cleanup/job/event controls (.2, .4, .5, .7) |
| `TestManagedDeletionAdmissionOwnershipRecovery`, same file | Owner-only cancellation/release; foreign same-operation retry refused; cancelled reuse revalidates new revision/new owner; stale owner cannot update/start/cancel restored claim; commit marker survives acknowledgement loss; restart before/after final commit and independent startup with original owner live; reserved/incomplete snapshot never runs; ownership/commit read failure retains safe barrier (.3-.5, .7, .8) |
| `TestManagedDeletionHostReceipts`, `internal/plugins/host_managed_deletion_admission_test.go` | Registered `Service.hostForPlugin` with real CommandStore/services/lifecycle: stale conflict, detach/missing not-found, current applied, completed replay, payload mismatch, native support unavailable; receipt ack failure and proven-marker incomplete replay; replacement never deleted (.6) |
| `TestManagedDeletionHostAdmittedFailures`, same Host file | Real lifecycle canvas/SQL failure after admission yields Unavailable/admitted_failed with incomplete receipt; owner/commit uncertainty yields outcome_uncertain; prior admitted record followed by missing/detach/revision change never becomes clean conflict or already-applied; plain replay absence is NotFound without commit marker; transport-only mocks for SDK boundary (.4, .6) |
| `TestManagedDeletionAdmissionSQLiteWaits`, `internal/task/repository/sqlite/managed_deletion_admission_test.go` | Independent real SQLite pools: native writer reservation waiting/current-after-wait for revision/detach and owner fence; cancellation, admission/final-delete rollback, both winner directions (.1-.4, .7) |
| `TestManagedDeletionAdmissionNativeConformance`, same native file | Actual SQL/typed store admission identity/revision/cleanup/hierarchy guard, owner-aware environment transfer, snapshot update/release/restore/start fencing, all required managed settings/state and runtime/primary/hierarchy participants, ordinary retained-task owner exclusion, unchanged legacy full-snapshot semantics outside claim (.3-.5, .7, .8) |
| `TestManagedDeletionAdmissionPostgresWaits`, `internal/task/repository/sqlite/managed_deletion_admission_postgres_test.go` | Independent service/physical pools and observer: actual PID/pg_locks waiting on newly selected admission/final deletion; holder commits revision/detach/identity/missing and waiter sees current typed result; cancellation and isolation; both winner orders (.1-.4, .7) |
| `TestManagedDeletionAdmissionPostgresRollback`, same file | Actual SQL rollback of admission and separately final delete/atomic marker; final-purge failure keeps original rows and no deleted marker; admitted error, owner release/uncertainty and current resource/transcript observations (.4, .5, .7) |

Gate construction may forward transport/native calls but must never fabricate
business predicate results. First RED must fail behaviorally against existing
APIs, not fixture types or missing new methods. Once production admission
changes, place deterministic gates relative to the actual chosen boundary.
For delete-first ordering, hold the service AFTER native admission commit and
before effects; the competing writer should finish with typed conflict, not
wait for external I/O or be accepted then deleted. Separately assert real native
transaction waits at admission/finalization. Never await a winning mutation
behind an owned admission. Every goroutine is bounded/cancelled/released/joined before fixture DB
cleanup. Observe actual locks, not elapsed-time sleeps as lock evidence.

PG must use `testutil.PostgresDSNFromEnv`/`OpenIsolatedPostgres` with private
schema and independent pools, following the existing hierarchy/managed tests.
Record `SHOW transaction_isolation`, physical holder/waiter/observer PIDs,
`pg_locks` non-granted waits and current-after-wait results at the newly owned
boundary. SQLite needs equivalent native writer reservation/current-after-wait,
cancellation and SQL rollback observations. Add focused SQL-guard/store
conformance for the new owner/predicate/state CAS at the named native test;
these observations cannot be replaced by mocks or a generic settings suite.

## Exact compatibility evidence from reviewed source

The following names exist at proofbase; retain them rather than substituting
generic Delete/Ensure wildcard runs:

- Managed settings/state: `TestManagedConversationAdmissionInterleavings`,
  `TestManagedConversationAdmissionRollback`, `TestManagedConversationAdmissionControls`,
  `TestManagedConversationAdmissionLifecycle`, `TestManagedConversationAdmissionPublication`,
  `TestManagedConversationEnsureOperationReplay`,
  `TestManagedConversationEnsureRepairsAcceptedOperation`,
  `TestManagedConversationConfigurationUsesRevisionAndIdleBoundary`,
  `TestManagedConversationSurvivesLegacyPluginCleanup`,
  `TestManagedConversationUninstallDetachesAndHidesRetainedTranscript`.
- Real task lifecycle: `TestService_DeleteTask`,
  `TestService_DeleteTaskWithReason_PublishesReason`,
  `TestService_DeleteTask_OmitsReasonWhenUnset`,
  `TestService_DeleteTaskPreservesExecutorRunningWhenStopFails`,
  `TestService_DeleteTaskFailsClosedWhenRuntimeInventoryFails`,
  `TestDeleteTaskRejectsDirtyWorktreeBeforeMutation`,
  `TestDeleteTaskWithDiscardConsentPersistsAndCleansDirtyWorktree`,
  `TestDeleteTask_TransfersBorrowedEnvironmentBeforeDeletingOwner`,
  `TestDeleteInheritedSubtaskRestoresSharedEnvironmentOwnershipOnEarlyAbort`,
  `TestPrepareTaskResourceCleanupCancelsBarrierAfterSnapshotFailure`,
  `TestPreparedCleanupIsNotRunnableUntilStarted`,
  `TestCancelPreparedTaskResourceCleanupIgnoresCallerCancellation`,
  `TestTaskMutationCommitThenErrorKeepsCleanupRunnable`,
  `TestDeleteTaskCleanupSnapshotSurvivesSessionCascade`.
- Legacy conversation: `TestDeleteAllForPluginReportsNonNotFoundErrors`,
  `TestDeleteReportsNonNotFoundErrors`, `TestDeletePathsMatchSentinelNotErrorText`,
  `TestDeletePathsAbsorbWrappedNotFoundSentinel`,
  `TestDeleteUsesLifecycleAwareTaskDeleterWhenAvailable`.
- Native: `TestTaskCleanupBarrier_RejectsSessionCreation`,
  `TestTaskCleanupBarrier_RejectsEnvironmentCreation`,
  `TestTaskCleanupBarrier_CommittedCreationIsIncludedInInventory`,
  `TestTaskCleanupBarrier_ReleasedBarrierAllowsCreation`,
  `TestTransferTaskEnvironmentAdvancesGenerationAndHonorsCleanupBarrier`,
  `TestDeleteTaskNotifiesQueuePurgeAfterCommit`,
  `TestDeleteTaskClearsPromptSequenceBeforeSessionIDReuse`,
  `TestPostgresRepository_DeleteTask_MissingTaskReturnsErrTaskNotFound`,
  `TestPostgresRepository_DeleteTask_SerializesWithSessionCreation`,
  `TestManagedConversationAdmissionPostgresWaits`,
  `TestManagedConversationAdmissionPostgresRegistrationFence`,
  `TestManagedConversationAdmissionPostgresRollback`.
- Registered Host/SDK/canvas: `TestManagedConversationHostAdmissionReceipts`,
  `TestManagedConversationLifetime`,
  `TestManagedConversationUninstallDetachesAndReinstallCannotAdopt`,
  `TestManagedConversationInactiveInstallationCannotUseOldHost`,
  `TestManagedAgentConversationHostMethodsRoundTrip`,
  `TestCleanupTaskClearsPromotedCanvasOrigin`,
  `TestRemoveDeletesCreationAuthority`. SDK round-trip mocks cover transport
  only; they do not replace the real Host deletion proof.

## Verification during released implementation

Each command is independently rooted; run serially, join every handle. These
are now authorized by the later reviewed DB-boundary implementation release. The exact
compatibility names above are intentionally anchored in the commands below.
Record targeted RED first and GREEN after correction. ROOT's chosen boundary
may require adding a narrowly affected named test, with a documented cause.

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/service -run '^(TestManagedDeletionAdmissionInterleavings|TestManagedDeletionAdmissionFailureEffects|TestManagedDeletionAdmissionOwnershipRecovery)$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/service -run '^(TestManagedConversationAdmissionInterleavings|TestManagedConversationAdmissionRollback|TestManagedConversationAdmissionControls|TestManagedConversationAdmissionLifecycle|TestManagedConversationAdmissionPublication|TestManagedConversationEnsureOperationReplay|TestManagedConversationEnsureRepairsAcceptedOperation|TestManagedConversationConfigurationUsesRevisionAndIdleBoundary|TestManagedConversationSurvivesLegacyPluginCleanup|TestManagedConversationUninstallDetachesAndHidesRetainedTranscript|TestService_DeleteTask|TestService_DeleteTaskWithReason_PublishesReason|TestService_DeleteTask_OmitsReasonWhenUnset|TestService_DeleteTaskPreservesExecutorRunningWhenStopFails|TestService_DeleteTaskFailsClosedWhenRuntimeInventoryFails|TestDeleteTaskRejectsDirtyWorktreeBeforeMutation|TestDeleteTaskWithDiscardConsentPersistsAndCleansDirtyWorktree|TestDeleteTask_TransfersBorrowedEnvironmentBeforeDeletingOwner|TestDeleteInheritedSubtaskRestoresSharedEnvironmentOwnershipOnEarlyAbort|TestPrepareTaskResourceCleanupCancelsBarrierAfterSnapshotFailure|TestPreparedCleanupIsNotRunnableUntilStarted|TestCancelPreparedTaskResourceCleanupIgnoresCallerCancellation|TestTaskMutationCommitThenErrorKeepsCleanupRunnable|TestDeleteTaskCleanupSnapshotSurvivesSessionCascade|TestDeleteAllForPluginReportsNonNotFoundErrors|TestDeleteReportsNonNotFoundErrors|TestDeletePathsMatchSentinelNotErrorText|TestDeletePathsAbsorbWrappedNotFoundSentinel|TestDeleteUsesLifecycleAwareTaskDeleterWhenAvailable)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/plugins ./pkg/pluginsdk ./internal/canvas -run '^(TestManagedDeletionHostReceipts|TestManagedDeletionHostAdmittedFailures|TestManagedConversationHostAdmissionReceipts|TestManagedConversationLifetime|TestManagedConversationUninstallDetachesAndReinstallCannotAdopt|TestManagedConversationInactiveInstallationCannotUseOldHost|TestManagedAgentConversationHostMethodsRoundTrip|TestCleanupTaskClearsPromotedCanvasOrigin|TestRemoveDeletesCreationAuthority)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/repository/sqlite -run '^(TestManagedDeletionAdmissionSQLiteWaits|TestManagedDeletionAdmissionNativeConformance|TestManagedDeletionAdmissionPostgresWaits|TestManagedDeletionAdmissionPostgresRollback|TestTaskCleanupBarrier_RejectsSessionCreation|TestTaskCleanupBarrier_RejectsEnvironmentCreation|TestTaskCleanupBarrier_CommittedCreationIsIncludedInInventory|TestTaskCleanupBarrier_ReleasedBarrierAllowsCreation|TestTransferTaskEnvironmentAdvancesGenerationAndHonorsCleanupBarrier|TestDeleteTaskNotifiesQueuePurgeAfterCommit|TestDeleteTaskClearsPromptSequenceBeforeSessionIDReuse|TestPostgresRepository_DeleteTask_MissingTaskReturnsErrTaskNotFound|TestPostgresRepository_DeleteTask_SerializesWithSessionCreation|TestManagedConversationAdmissionPostgresWaits|TestManagedConversationAdmissionPostgresRegistrationFence|TestManagedConversationAdmissionPostgresRollback)$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout -k 10s 11m golangci-lint run ./... --new-from-rev=d803d6f209030789d14751db90c5e9eaf5ba74a6 --concurrency 2 --allow-serial-runners --timeout 10m)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

PG commands require upfront verified owned PG16 receipts and the private owned
DSN exported as `KANDEV_TEST_POSTGRES_DSN`; a skipped PG suite is not
evidence. Capture original installed linter path/hash/PID/elapsed/exit separately.
At PR creation, verify immutable base matches the command; if actual PR base
differs, checkpoint the exact revision rather than silently substituting main.
Apply all install/resource/CI/review/merge/cleanup limits in the manifest.

## Files likely touched after boundary review

- `apps/backend/internal/task/service/managed_conversation_settings.go`
- `apps/backend/internal/task/service/agent_conversations.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- `apps/backend/internal/task/service/managed_deletion_admission_test.go` (new)
- `apps/backend/internal/task/repository/managedconversation/admission.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/managed_conversation_admission.go`
- `apps/backend/internal/task/repository/sqlite/managed_conversation_state.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/repository/sqlite/task_cleanup_barrier.go`
- `apps/backend/internal/task/repository/sqlite/resource_cleanup.go`
- `apps/backend/internal/task/repository/sqlite/task_environment.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/task/repository/sqlite/executor.go`
- `apps/backend/internal/task/repository/sqlite/task_hierarchy_admission.go`
- `apps/backend/internal/task/repository/sqlite/managed_deletion_admission_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/managed_deletion_admission_postgres_test.go` (new)
- `apps/backend/internal/plugins/host_managed_conversations.go`
- `apps/backend/internal/plugins/host_managed_deletion_admission_test.go` (new)
- `apps/backend/internal/backendapp/services.go` (only necessary typed wiring)
- `apps/backend/internal/task/service/managed_deletion.go` (new typed lifecycle
  helper/phase-aware error if useful for size limits, not a callback framework)
- `apps/backend/internal/task/repository/sqlite/managed_deletion.go` (new native
  admission/inspection/finalization/owner CAS helpers if useful for size limits)
- Existing resource provider implementations and canvas service stay unchanged;
  native owner-aware environment/helper variants are the necessary adapters.
- `docs/public/plugins-authoring.md` (focused reference paragraph after release,
  covering admitted partial effects/Unavailable, replay evidence, uncertainty).
- These four owning documents, the accepted dated SQLite writer-admission ADR,
  and the reconciled existing Platform persistence design; one work order.

## Dependencies, inputs, and risks

No predecessor work order. ROOT's boundary decision and later reviewed
implementation INTERRUPT are recorded. Read the paired requirement/design,
existing managed-coordination/atomic-settings package, scoped backend guidance,
and immutable proof receipt. Exclusive admission must precede effects without
hidden fallback or external SQL lock duration. Admitted failure is potentially
partial, not clean rejection; a persisted deleted marker proves physical commit.
An old PREPARED row/startup cutoff cannot prove foreign owner death. Retain
safe barrier and Unavailable when uncertain; no new lease/force-recovery promise.

## Parallelism

`sequential`. No delegation, agents, workers, tasks, tabs, or sessions.

## Results

Permanent existing-API lifetime RED and factory RED preceded their corrections.
Four factory tests, native conformance and actual SQLite admission/final-delete
physical waits pass on the current correction, with joined handles. Current service, registered Host, independent PG16, affected compatibility,
SQL guard and full SQLite/PG store conformance checks pass. Documentation checks
pass. Original full immutable-base lint passes. Reviewed delivery remains pending. Exact debug
receipts live in the external plan; order stays in progress.

The original full lint reported seven source diagnostics and timed out. Under
ROOT's bounded correction release, small local helpers now express existing
native validation, snapshot and lifecycle phases; two native test branches use
equivalent switches. Attachment locking and deferred release remain in their
original function, with transaction ownership and external-effects order
preserved. Affected SQLite native, lifecycle service, registered Host and real
PG16 wait/rollback/participant selectors passed after correction. All handles
and the exact owned PG fixture were joined and cleaned. The corrected-source
original full lint completed with one additional lifecycle complexity diagnostic.
A private attachment-repository getter and postcommit attachment-cleanup helper
now preserve the caller's deferred release and existing cleanup order; an
unreachable duplicate error check was removed. The affected lifecycle service
and registered Host selectors passed again. No native SQL or lock path changed
after the joined SQLite/PG checks. The original full immutable-base lint passed
after this correction. Hosted review and verified delivery remain pending.

## SQLite factory extension

Reviewed and released: the shared `internal/db.OpenSQLite` writer uses
supported `_txlock=immediate`; actual sqlx/database/sql Tx owns writer entry.
This is ROOT's explicit exception to the earlier no-global-writer-policy scope.
See the [accepted ADR](../../decisions/2026-10-05-sqlite-writer-transaction-admission.md)
and [Platform design](../../specs/platform/system-design/postgres-domain-store-parity.md#sqlite-writer-transaction-admission).
No per-feature pool/manual transaction facade/driver mutation/row-touch retry.
Retain OpenSQLiteReader mode/pool, PG READ COMMITTED, native hierarchy/owner
CAS/deleted marker and all lifetime safety observations. An injected deferred
pool may fail BUSY safely; do not assert that all arbitrary pools now wait.

Extend this SAME sequential order, under the later release, in this sequence:

1. Add meaningful factory admission RED through actual `db.OpenSQLite` handles
   before changing its DSN. Archive/join the anchored failure. The earlier
   existing-API lifetime RED is already recorded; never replay ROOT's archive.
2. Change only writer `_txlock`, necessary factory/pool comments and a concise
   backend AGENTS persistence note describing writer BEGIN admission, dedicated
   deferred readers and bounded busy cancellation. Keep paths/timeouts/WAL/cache,
   dependencies and pool counts unchanged. Keep the guidance concise and scoped.
3. Replace native permanent fixture's duplicated deferred DSN/trace connector
   with independent actual `db.OpenSQLite` handles, plus actual reader factory
   for snapshot proof. Schema template closes/checkpoints before copied fixtures;
   initialize/ping/read back busy_timeout BEFORE holder entry. Instrumentation
   may observe genuine driver BEGIN stacks/entry and return, never substitute SQL
   or predicates or override configured mode. Use fresh waiter connections so
   pool checkout is not the alleged physical wait.
4. Prove same blocked SQLite driver work inside actual BEGIN across held writer
   SQL mutation and a separate real BUSY writer probe; then holder commit makes
   the waiter read current canonical revision/detach/identity and actual owner
   state. Repeat admission and final-deletion ordering both directions. Keep
   typed rollback/owner release/marker/effect assertions unchanged. No transient
   C-step sample or sleep alone is accepted as waiting evidence.
5. Join factory/native safety then the exact focused compatibility below, before
   remaining service/Host/owned PG and original full immutable-base lint. A real
   causal failure gets minimal correction; resource/lost/material seam failure
   gets exact durable WAITING, no retry or assertion weakening. Preserve all
   standing one-heavy/receipt/normal hooks/review/merge/owned cleanup gates.

| Proposed factory test in `internal/db/sqlite_test.go` | Required real evidence |
| --- | --- |
| `TestSQLiteWriterTransactionAdmission` | Two independent factory writers; genuine blocked BEGIN, holder SQL mutation/probe and current-after-wait; BeginTxx and Beginx remain real Tx; no title/FTS trigger from transaction entry; writer read-only transaction also reserves. |
| `TestSQLiteWriterTransactionCancellation` | Already-cancelled caller; cancellation during busy while holder CONTINUES locked until waiter settles; default 5-second busy bound with honest observed latency; uncancelled busy expiry; error/extended code; no native accepted owner/effects. No nonexistent Tx rollback after failed BEGIN. Returned Tx near cancellation gets context check and joined normal rollback. Release holder only after waiter result for the latency case. |
| `TestSQLiteWriterTransactionRollbackReuse` | SQL failure, explicit rollback and cancellation after successful entry; no orphan Tx/connection. Actual pooled reuse after failed-BEGIN/busy expiry/cancel and successful commit; all returned Tx/rows/connections settled. |
| `TestSQLiteReaderProgressDuringWriterTransaction` | Dedicated actual OpenSQLiteReader progresses while one writer holds and another waits; stable old committed snapshot across holder commit, new read sees new data; read-only main rejects writes. No alias fixture as reader-concurrency evidence. |

Bound contexts and an outer timeout cover the native 5-second busy wait plus
cleanup. Do not claim instantaneous cancellation. Factory cancellation and
native cancellation tests assert that cancellation before committed admission
cannot create owner rows or effects. Commit uncertainty remains unavailable and
owner-fenced; no task-absence inference. Every Tx, goroutine, rowset and handle
settles before the next heavy command/fixture cleanup. Arbitrary legacy deferred
injection has a focused fail-safe control in native conformance, not a fallback.

Ownership added: `apps/backend/internal/db/sqlite.go`, `pool.go` comments,
`sqlite_test.go` (new), and `apps/backend/AGENTS.md` scoped guidance. The existing
native admission test fixture is corrected in its owned file. No general store
rewrite or read-only transaction migration. Existing Platform context owns the
operational rule; Plugins still owns REQ-013/.1-.8 outcomes.

Exact additional compatibility selectors, verified against reviewed source,
are in the commands below. Run serially under the reviewed release;
new factory names above are implemented, and all other names exist. Existing named
lifetime/Host/PG commands remain required, not replaced by this expansion.

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout -k 10s 2m go test -trimpath -tags fts5 -race -p=1 ./internal/db -run '^(TestSQLiteWriterTransactionAdmission|TestSQLiteWriterTransactionCancellation|TestSQLiteWriterTransactionRollbackReuse|TestSQLiteReaderProgressDuringWriterTransaction)$' -count=1 -timeout=90s -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/db ./internal/testutil -run '^(TestTableColumnsAndExistsSQLite|TestLockTaskRowInTx_SQLiteNoOpSucceedsRegardlessOfExistence|TestLockTaskRowInTx_SQLiteSerializesViaSingleWriterConnection|TestLockTaskRowInTx_PostgresBranchQueriesForUpdate|TestRegisterWriterPoolStatsExposesDBStatsAtDebugVars|TestMigrateLogger_Apply_Idempotent|TestRequiredMigrateLogger_Apply_ReturnsUnexpectedFailure|TestRequiredMigrateLogger_Apply_AllowsIdempotentFailure|TestRequiredMigrateLoggerContextCancelsActiveStatement|TestSQLiteTemplateClonesIndependentDurableDatabases)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/persistence -run '^(TestProvide_TakesSnapshotOnUpgrade|TestProvide_NoSecondSnapshotSameVersion|TestProvide_BackupFailureReturnsError|TestProvideAdoptsCommittedWALDataAndRetainsLegacySidecars|TestProvideExplicitDatabasePathBypassesLegacyDiscovery|TestSnapshotSQLite_CreatesReadableCopy|TestSnapshotSQLite_CanceledRemovesDestination)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/maintenance -run '^(TestRunDryRunMissingDatabaseCreatesNothing|TestRunDryRunLegacySchemaDoesNotMigrateOrHeal|TestRunDryRunReportsCandidatesWithoutMutating|TestRunExecuteRefusesWhenAnotherProcessHoldsOwnership|TestRunExecuteDeletesCandidatesBackupAndIsIdempotent|TestRunExecuteWithCompactReplacesDatabaseAndPreservesRollback)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/agent/settings/store -run '^(TestSQLiteRepositoryDynamicProfileCRUDUsesOptimisticVersions|TestUpdateAgentProfileMcpConfigPatchPreservesUnchangedFields|TestDuplicateAgentProfile_RoundTrip|TestDuplicateAgentProfile_RollsBackMcpFailure|TestGetAgentProfileTx_LegacyBackfillReadsThroughTransaction)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/office/repository/sqlite -run '^(TestInitSchema_Idempotent|TestRecreateBudgetClaims_OldShapeDatabase_GuardedAcrossBoots|TestClassifyCoordinatorInstallWaitErr_CallerCancellationWins|TestClassifyCoordinatorInstallWaitErr_DeadlineExceededIsContention|TestClassifyCoordinatorInstallWaitErr_SQLiteBusyTextIsContention|TestWithCoordinatorInstallLock_FnContentionIsClassified|TestListTaskCommentsWindow_SnapshotAllowsConcurrentWrite)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/plugins/state ./internal/plugins/instances -run '^(TestCommandStoreAdmitReplaysAndRejectsPayloadMismatch|TestCommandStoreCompletesReceiptAcrossReload|TestInstanceStoreUsesRevisionedConditionalWrites|TestInstanceStoreDeleteRetainsTombstoneRevision|TestStoreInitSchemaIsIdempotent|TestUserStoreInitSchemaIsIdempotent|TestCreateInstanceAdmissionIsAtomicAtTaskAndWorkspaceLimits|TestRemoveArtifactIfUnreferencedRechecksReleaseOwnership|TestCleanupJobsCanBeClaimedRetriedAndCompleted)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/repository/sqlite ./internal/automation -run '^(TestQuerySidebarTaskPageUsesReaderPoolForSnapshot|TestSidebarQueryScratchKeepsReadSnapshotAndReadOnlyMain|TestListTaskPlanCommentsReturnsOneDatabaseSnapshot|TestConversationSourceMessagePageReturnsConsistentRevision|TestExportAutomationsDocument_AC30_ConcurrentMutationNeitherBlocksNorIsReflected|TestResolveDescriptors_AC29_SameTransactionHandleReachesStoreAndEveryLookup)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/github ./internal/secrets -run '^(TestStore_ResetPRWatch_DropsDiscoveredSourceOnSearchingCollision|TestResetPRWatch_ConcurrentSessionsCoalesceDestination|TestTransferStore_ConcurrentSameTargetSQLite|TestTransferStore_RollbackSurvivesCanceledContext)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/persistence/storeconformance -run '^(TestStoreCatalogCompleteness|TestOwnerBehaviorCoverage|TestStoreConformance|TestPreviousStableUpgrade|TestUpgradeFixtureManifest)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go run -trimpath -tags fts5 ./cmd/sqlguard ./internal)
```

The central conformance command uses the already receipted owned PG16 only when
its later allocation is authorized and all ownership evidence exists; SQLite
runs remain useful but PG skips do not prove PG acceptance. Allocate only the owned PG16 fixture with upfront receipts. Mandatory original full lint remains its separate receipt.

Public docs scope: `docs/public/plugins-authoring.md` remains the reference for
admitted partial effects/Unavailable and operation-proven replay.
`docs/public/backend-development.md` remains an explanation: concise persistence
wording covers immediate writer transactions, separate deferred WAL read
snapshots and busy-handler cancellation latency. No new page/meta/setting,
README or screenshot update. Backend pure data/lifecycle only: no mobile/UI or
localization change. Run public-doc validators after the focused wording change.

Current result: factory, native SQLite/PG, lifecycle service, registered Host,
affected compatibility, SQL guard and full store conformance checks pass.
Public reference and backend guidance are updated and documentation validation
passes. Original full immutable-base lint passes; actual reviewed delivery remains pending.
Keep order in progress. Exact receipts/diagnostic history are in the external
plan, with all immutable proof archives retained read-only.
