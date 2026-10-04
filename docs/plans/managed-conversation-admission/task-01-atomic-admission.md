---
id: "01-atomic-admission"
title: "Admit current managed settings atomically"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-MANAGED-COORDINATION-002
acceptance_criteria:
  - AC-PLUGINS-MANAGED-COORDINATION-002.1
  - AC-PLUGINS-MANAGED-COORDINATION-002.2
  - AC-PLUGINS-MANAGED-COORDINATION-002.3
  - AC-PLUGINS-MANAGED-COORDINATION-002.4
  - AC-PLUGINS-MANAGED-COORDINATION-002.5
  - AC-PLUGINS-MANAGED-COORDINATION-002.6
  - AC-PLUGINS-MANAGED-COORDINATION-002.7
  - AC-PLUGINS-MANAGED-COORDINATION-002.8
system_design:
  - ../../specs/plugins/system-design/managed-coordination.md
---

# Task 01: Admit current managed settings atomically

## Summary

Make EnsureManaged current revision/idle validation and coherent settings persistence
one native admission. Migrate participating managed writers and prove actual service,
provider, runtime-row and Host receipt behavior across independent SQLite/PG16 clients.
Implementation released by ROOT after review of the completed design turn.
Execution remains in the same primary session, without delegation.

## In scope

1. ROOT release received; phase wording updated and status in_progress. Read scoped backend rules
   and TDD backend factory guidance. Preserve the primary session/profile/executor.
2. Add permanent existing-API RED before production correction. Adapt the immutable
   proof into TestManagedConversationAdmissionInterleavings, using real independent
   SQLite services/connections and actual runtime state/attempt/executor persistence.
   Construction-time forwarding gates pause AFTER a real pre-admission read, let the
   competitor commit and independently read its accepted result, then release and
   join before assertions/DB close. Preserve ROOT's proof source; never replay it.
3. Implement the mandatory typed repository seam and backendapp/provider forwarding.
   Use current native locks, predicates, narrow column/metadata writes, atomic
   create/update/repair, typed outcomes and committed projections from the design.
   Test failure between the session/task writes using real SQL failure injection,
   not a service mock. Cancellation before commit leaves both unchanged.
4. Move exact pause, installation pause, invalidation and detach to native admission.
   Preserve revision/change/replay and per-conversation iteration behavior, stop and
   wake only after commit. Obey existing cleanup barriers and primary replacement.
   Do not redesign DeleteManaged or its lifecycle deletion preflight.
5. Extend meaningful real registered Host v2 tests (hostForPlugin -> ExactHost manager,
   SetManagedAgentConversations with real service, real approval/command stores).
   Verify concurrent loser CONFLICT/receipt, winner APPLIED, NO_CHANGE, replay and
   lost Complete acknowledgement with committed descriptor and recoverable stamp.
   Profile resolution/external transport only may be mocked where required.
6. Independent PG16 physical row wait/cancellation/READ COMMITTED proofs with current
   state/identity readback, actual runtime writers and atomic rollback. Use real
   factories, join every client/goroutine before any schema/container cleanup.
7. Targeted Go, SQLguard/store conformance, mandatory original full installed lint,
   doc/coverage/diff gates. Record each real receipt before delivery.

## Out of scope

Schema/wire/SDK/UI/auth/profile-registry/execution redesign, general callbacks,
version engine, optional unsafe fallback, unrelated legacy snapshot protection,
legacy v1 lifecycle migration, generic task/delete cleanup changes, permanent
harness/lockfile/cache edits, runtime instance/browser work, feature flags,
additional work orders, agents/tasks/tabs/sessions or automatic retries.

## Acceptance

- Both interleavings reject through existing APIs with expected typed statuses and
  preserve accepted runtime/config winner. All canonical managed config/revision
  writers use required admission and preserve unrelated fields.
- Current/empty/nochange/busy/replay/repair/create/isolation controls pass; injected
  update/create/repair failure and cancellation have no partial own configuration,
  event/wake/stop. Host receipts/descriptors report actual pre/post-commit outcomes.
- Independent SQLite and PG16 physical-lock evidence, appropriate conformance and
  original full lint pass with joined handles. Delivery records distinguish local
  validation, published head, actual merge and owned cleanup.

## Files likely touched

- apps/backend/internal/task/service/agent_conversations.go, managed_conversation_settings.go and managed_conversations_test.go
- apps/backend/internal/task/service/managed_conversation_admission_test.go and managed_conversation_admission_controls_test.go (new)
- apps/backend/internal/task/handlers/process_handlers_test.go (required shared fail-closed test wiring only)
- apps/backend/internal/task/repository/managedconversation/admission.go (new typed domain seam)
- apps/backend/internal/task/repository/interface.go
- apps/backend/internal/task/repository/sqlite/managed_conversation_admission.go and managed_conversation_state.go (new)
- apps/backend/internal/task/repository/sqlite/managed_conversation_admission_test.go (new)
- apps/backend/internal/task/repository/sqlite/managed_conversation_admission_postgres_test.go (new)
- apps/backend/internal/backendapp/adapters_agent_conversations.go and focused wiring tests
- apps/backend/internal/plugins/host_managed_conversation_admission_test.go (new)
- apps/backend/internal/plugins/host_managed_conversations.go (only causal receipt/status correction)
- The four linked design/package documents for phase/results accuracy.

Read task_metadata_merge.go, session.go, executor.go, task_cleanup_barrier.go,
task_hierarchy_admission.go and db/taskhierarchy.go for tx primitives; change an
existing primitive only if the narrow seam cannot reuse it safely. Native store
must not depend on plugin transport. Do not add SQLguard exemptions/catalog edits.

## Verification

Run commands independently from repository root, sequentially. Test names below
are implementation targets. Never accept SKIP/compile-only as PG evidence.

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/service -run '^TestManagedConversationAdmissionInterleavings$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/service -run '^(TestAgentConversationServiceExposesManagedInputOperations|TestAgentConversationTaskIsCreatedAsEphemeral|TestConcurrentEnsureIsIdempotent|TestDeleteAllForPluginConcurrentCallsOverRealRepository|TestDeleteAllForPluginIgnoresMalformedProvenance|TestDeleteAllForPluginLeavesOrdinaryUserTasksUntouched|TestDeleteAllForPluginLeavesOtherPluginsUntouched|TestDeleteAllForPluginNothingOwnedIsNoop|TestDeleteAllForPluginOverRealRepository|TestDeleteAllForPluginOverRealRepositoryLeavesEverythingElse|TestDeleteAllForPluginRejectsEmptyPluginID|TestDeleteAllForPluginRejectsEmptyPluginIDOverRealRepository|TestDeleteAllForPluginReportsNonNotFoundErrors|TestDeleteAllForPluginSpansWorkspaces|TestDeleteConcurrentCallsOverRealRepository|TestDeleteFindsConversationsBeyondFirstPage|TestDeleteIdempotent|TestDeleteOnlyOwnedConversations|TestDeletePathsAbsorbWrappedNotFoundSentinel|TestDeletePathsMatchSentinelNotErrorText|TestDeleteRejectsEmptyOwnershipIdentifiers|TestDeleteRemovesConversation|TestDeleteReportsNonNotFoundErrors|TestDeleteUsesLifecycleAwareTaskDeleterWhenAvailable|TestDispatchBeforeEnsure|TestDispatchBusySessionDoesNotConsumeOccurrence|TestDispatchCoalescesDistinctConcurrentOccurrencesForAnIdleSession|TestDispatchConcurrentOccurrenceClaimIsAtomic|TestDispatchDelimiterCoordinatesUseDistinctMessageIDs|TestDispatchDeliversThroughRuntime|TestDispatchFindsConversationBeyondFirstPage|TestDispatchOccurrenceKeysAreScopedPerConversation|TestDispatchPropagatesRuntimeDeliveryFailure|TestDispatchRecoversPendingOccurrenceAfterRestart|TestDispatchRejectsEmptyArguments|TestDispatchRetriesOccurrenceAfterCancelledDelivery|TestDispatchRetriesOccurrenceAfterDeliveryFailure|TestDispatchWithBusySession|TestDispatchWithIdleSessionSendsRatherThanStarts|TestDispatchWithOccurrenceKeyDeduplicates|TestDispatchWithStartingSessionIsAlsoBusy|TestDispatchWithoutDispatcherReturnsUnavailable|TestEnsureCreatesConversation|TestEnsureCrossPluginSeparation|TestEnsureCrossWorkspaceSeparation|TestEnsureFindsConversationBeyondFirstPage|TestEnsureProfileInDescriptorFromExistingTask|TestEnsureProfileValidatorReceivesConfiguredID|TestEnsureReconcilesExistingConversationProfileAndBasePrompt|TestEnsureRejectsDisabledProfileWithZeroPartialRows|TestEnsureRejectsDisabledWorkspaceDefaultProfileWithoutPartialRows|TestEnsureRejectsEmptyArguments|TestEnsureRejectsMissingProfileWithZeroPartialRows|TestEnsureRejectsMissingWorkspaceDefaultProfileWithoutPartialRows|TestEnsureRejectsProfileChangeForLiveConversationSession|TestEnsureRepairRefusedWhenBoundProfileBecomesInvalid|TestEnsureRepairsMissingPrimarySession|TestEnsureRepairsWrappedMissingPrimarySessionOverRealRepository|TestEnsureReturnsExistsOnSecondCall|TestEnsureStoresProfileIDInMetadata|TestEnsureUsesOneConversationAcrossServiceInstances|TestEnsureUsesWorkspaceDefaultProfileWhenPluginOmitsProfile|TestEnsureWithoutProfileConfiguredUsesWorkspaceDefault|TestIsManagedConversationTask|TestIsManagedConversationTaskHonorsEphemeralMetadata|TestManagedConversationAdmissionControls|TestManagedConversationAdmissionInterleavings|TestManagedConversationAdmissionLifecycle|TestManagedConversationAdmissionRollback|TestManagedConversationApprovalChangeInvalidatesAndStopsPolicy|TestManagedConversationConcurrentRevisionConflict|TestManagedConversationConfigurationUsesRevisionAndIdleBoundary|TestManagedConversationEnsureOperationReplay|TestManagedConversationEnsureRepairsAcceptedOperation|TestManagedConversationIdentity|TestManagedConversationPauseStopsActiveExecution|TestManagedConversationSurvivesLegacyPluginCleanup|TestManagedConversationUninstallDetachesAndHidesRetainedTranscript|TestManagedInputAdmissionEnforcesConfiguredQueueLimit|TestManagedInputCancelAcceptedAndRequiresExactStopConfirmation|TestManagedInputEnqueueReplayConflictAndBoundedList|TestManagedInputEnqueueReturnsReceiptBeforeQueueWakeCompletes|TestManagedInputImmediateDispatchNeverQueuesAndReplaysStatus|TestManagedInputImmediateDispatchReturnsBusyForSessionAndQueuedInput|TestManagedInputPauseRetainsAcceptedWorkAndUnpauseWakesQueue|TestManagedInputRejectsInvalidCoalescingAndStaleAuthority|TestManagedPeriodicInputCoalescesOnlyPendingPeriodicWork|TestStoresBasePromptInMetadata)$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/repository/sqlite -run '^TestManagedConversationAdmission.*$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/plugins -run '^TestManagedConversation.*$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/backendapp -run '^TestManagedConversationAdmissionWiring$' -count=1 -v)
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/task/repository/sqlite -run '^TestManagedConversationAdmissionPostgres(Waits|RegistrationFence|Rollback)$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 ./internal/db/sqlguard ./internal/persistence/storeconformance -run '^Test.*$' -count=1 -v)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go run -trimpath -tags fts5 ./cmd/sqlguard internal)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

First command is the permanent RED gate before correction, then GREEN on the same
existing-API outcomes. Real factory fixtures: createTestService and
seedConversationWorkspace; file-backed independent SQLite pools; PG
newHierarchyPostgresRepoPair/OpenIsolatedPostgres/NewWithInitializedDB. Tests must
register cancel/idempotent release/join cleanup immediately and join on failure.
Do not assert while a worker is still alive. A gate inside an already locked tx
cannot require the competitor to commit. PG observes actual lock wait through
pg_locks/pg_stat_activity and blocking PID, not elapsed sleep alone. Exercise task,
primary state/attempt, existing execution identity, absent canonical registration,
primary replacement and disposal current results. Record actual READ COMMITTED.

Mandatory full installed lint after final changed revision, from apps/backend:

```bash
GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 11m /home/jcfs/.local/bin/golangci-lint run ./... --new-from-rev=99f509743b29d3d021d76167f920304790485d42 --concurrency=2 --allow-serial-runners --timeout=10m
```

The specified SHA is the independently read current authoritative main snapshot at implementation; at delivery verify the actual
immutable PR base and substitute only if it differs. Record the original installed
binary realpath/hash, PID, timestamps/duration, exact command/base, actual exit and
joined handle. Hooks/scoped lint never substitute. Resource timeout/lost handle
requires durable exact ROOT checkpoint + WAITING, not automatic retry/cache wipe/
foreign kill. Concrete diagnostics authorize minimal causal correction/check.

One heavy at a time. Retain every returned session_id and ACTUALLY JOIN before any
next heavy/dependent mutation. No discarded handles or inferred duplicate results.
Use existing Node PATH with bash login=false. If apps/node_modules absent AFTER
release, perform once with pnpm9.15.9 install --frozen-lockfile from apps; no install
in the design turn, lockfile/harness/cache edits or repeated resource retries.

PG upfront receipt must identify exact owned container ID/labels, immutable PG16
image, ALL mounts/volumes and credential ownership; private schema/tmpfs and no
anonymous volume. Record actual physical waits/results, join clients before cleanup
of ONLY proven owned resources. Forbidden unproved foreign volume
2c48e791f0a8b8e64e6ecd30db0ede17388b572d4a303d39e2e0ee3fa7573ea7 stays untouched.
Missing PG access or resources becomes an honest durable WAITING checkpoint.

Run the actual .github/scripts/pr-docs.cjs validateCoverage for the four actual
artifacts and all planned source triggers, loading this plan/order and owning pair.
At design actual docs are exempt but planned triggers must be covered; after
implementation actual source must be covered. Confirm frontmatter/REQ/AC/order/plan
links, status, and exactly four unstaged docs at the design handoff.

## Delivery under ROOT release

Use normal active Conventional hooks, no bypass/amend. Ready PR, live body readback,
remote/local/upstream/PR exact head. Freeze published head except necessary semantic
corrections; no main-only rebase/synthetic merged replay. One retained90m scripts/pr-await
all-terminal JSON, join before replacing it; no duplicate/manual timer/hosted cancel.
Inspect actual substantive FULL CodeRabbit App347564 coverage of every current file
and all inline/grouped findings during CI. Explicit skip/gap permits at most one
necessary new-head full request; no optional assurance churn. Disposition every
actual finding; grounded optional style/docstring defer needs no change.

Unrelated hosted failure: exact job/log/artifact/source and durable ROOT WAITING
checkpoint before expansion/rerun. Normal expected-head squash only after actual
terminal policy checks and complete current review without hidden actionable errors;
no admin/bypass. Independently verify MERGED/time/SHA/parent/tree/all owned blobs and
authoritative remote inclusion. Published SHA is distinct from merge SHA. Completion
requires joined handles, only owned cleanup and clean managed worktree; ROOT retains
proof/archive and owns next child. Local done/publication is not persistent completion.

## Dependencies and parallelism

None. `sequential`; one primary session, no delegation.

## Risks

Physical lock order, missing-row fencing, overlooked lifecycle writers, honest
post-commit effect errors and legacy bounds are specified in the owning design.
No new architecture or unrelated CI scope without a ROOT checkpoint.

## Results

Local implementation and required full lint passed. The checkpoints below preserve
actual diagnostic history; publication, CI/review and verified merge are tracked
externally until complete.
Design receipts remain in plan.md. All returned handles below were actually joined.

- Permanent existing-API interleavings RED50828: three real races failed, two controls passed;
  GREEN61859 passed five cases plus atomic rollback. ROOT proof never replayed.
- Atomic create/update/repair rollback RED81083 and GREEN61859 used actual SQL failures.
- Empty-settings descriptor RED57945 found reused decoded-map keys; fresh projection fixed it.
- Missing-primary exact pause RED69404 found erroneous success; native rejection now occurs
  before writes, retaining the existing missing-primary error and accepted-replay repair.
- Precise compatibility84038: all 88 service tests passed (84 inventory plus four new names),
  pkg8.666s. Host concurrent-loser receipts and production adapter passed3.236s/3.810s.
- Final native PG55111: three suites passed16.034s, including seven physical-wait variants,
  registration fence, cancellation/current READ COMMITTED and three rollback variants.
  Plugin six-suite lifetime/authorization/disable/detach/Host compatibility passed1.377s.
- Transcript fixture required a real turn FK; corrected only fixture. Final affected
  Controls/Lifecycle47094 passed1.474s with real message/turn retention, raw metadata/scalars,
  replay/repair/nochange/execution-only/empty/cancellation and positive/negative effects.
- SQLguard/store conformance26688 passed, including actual SQLite/PG owner contracts and
  previous-stable upgrade103.220s; no SKIP. Native SQLguard60730 passed; no exemption added.
- All PG clients joined before exact owned tmpfs container removal19:08:13Z. No anonymous
  volume or foreign resource mutation. Ownership/cleanup receipt remains in the external plan.
- Catalog348/1337, all36 specification tests, full spec lint, actual coverage evaluator
  (actual/planned covered, one work order, zero errors) and whitespace passed.
- Original full installed lint19895 joined exit1 after559.836s on a concrete handler-test
  typecheck failure: shared mockRepository lacked the newly required native methods.
  Minimal common fixture forwarding now fails closed as unavailable; affected handler
  six explicit tests passed1.101s (joined3417); later full-lint receipts below
  supersede this diagnostic checkpoint. No production fallback.
- Second distinct original full lint92844 joined exit1 after237.807s: nine concrete
  cyclop/constant/test-switch/unused-assignment/file-size findings. The managed settings
  block moved intact into its focused service file, pure closed intent helpers reduced
  complexity, existing SQL constant reused, and only added fixture methods relocated.
  Final regression86783 passed:88conversation9.797s, realHost3.513s, adapter4.123s,
  sixaffectedhandlers3.190s; positive and rejected active-stop control included.
  Final independent PG10990 passed16.830s with seven physical-wait cases, registration
  fence/cancellation and three rollback cases. Both owned tmpfs containers were removed
  only after all clients joined. A final QF1008 redundant embedded selector was simplified
  without behavior change; anchored actual admission interleavings/rollback/controls
  passed1.562s (joined60497). Original full installed lint4 passed exit0/240.893s,
  joined17930, originalPID1259727, GNUtimeoutPID1259726, finished20:02:21.359635Z.
  Binary SHA25667342f8c4ce658ed63692675a906a43293597ab526f97e1c2b60a2900355f06b;
  receipt /tmp/kandev-managed-admission-full-lint4-receipt.json. Earlier diagnostic
  receipts are retained. Local implementation is done; normal delivery remains pending.
- Normal active hooks passed without bypass (joined51991,40.252s), then the clean
  commit was published as ready PR4207. CI/review and actual verified merge remain
  external delivery gates; this work order does not claim them complete.
  Authoritative current main snapshot for lint is99f509743b29d3d021d76167f920304790485d42;
  proof/design base remains1a2c60cf81e9da95d3572aa689c1b5c9e3a2271b. No main-only rebase.

### Compatibility selector inventory

Mechanically enumerated 84 existing tests only from 11 reviewed conversation files, plus four explicit planned admission tests. No wildcard suffix matching generic task tests.

```text
TestAgentConversationServiceExposesManagedInputOperations
TestAgentConversationTaskIsCreatedAsEphemeral
TestConcurrentEnsureIsIdempotent
TestDeleteAllForPluginConcurrentCallsOverRealRepository
TestDeleteAllForPluginIgnoresMalformedProvenance
TestDeleteAllForPluginLeavesOrdinaryUserTasksUntouched
TestDeleteAllForPluginLeavesOtherPluginsUntouched
TestDeleteAllForPluginNothingOwnedIsNoop
TestDeleteAllForPluginOverRealRepository
TestDeleteAllForPluginOverRealRepositoryLeavesEverythingElse
TestDeleteAllForPluginRejectsEmptyPluginID
TestDeleteAllForPluginRejectsEmptyPluginIDOverRealRepository
TestDeleteAllForPluginReportsNonNotFoundErrors
TestDeleteAllForPluginSpansWorkspaces
TestDeleteConcurrentCallsOverRealRepository
TestDeleteFindsConversationsBeyondFirstPage
TestDeleteIdempotent
TestDeleteOnlyOwnedConversations
TestDeletePathsAbsorbWrappedNotFoundSentinel
TestDeletePathsMatchSentinelNotErrorText
TestDeleteRejectsEmptyOwnershipIdentifiers
TestDeleteRemovesConversation
TestDeleteReportsNonNotFoundErrors
TestDeleteUsesLifecycleAwareTaskDeleterWhenAvailable
TestDispatchBeforeEnsure
TestDispatchBusySessionDoesNotConsumeOccurrence
TestDispatchCoalescesDistinctConcurrentOccurrencesForAnIdleSession
TestDispatchConcurrentOccurrenceClaimIsAtomic
TestDispatchDelimiterCoordinatesUseDistinctMessageIDs
TestDispatchDeliversThroughRuntime
TestDispatchFindsConversationBeyondFirstPage
TestDispatchOccurrenceKeysAreScopedPerConversation
TestDispatchPropagatesRuntimeDeliveryFailure
TestDispatchRecoversPendingOccurrenceAfterRestart
TestDispatchRejectsEmptyArguments
TestDispatchRetriesOccurrenceAfterCancelledDelivery
TestDispatchRetriesOccurrenceAfterDeliveryFailure
TestDispatchWithBusySession
TestDispatchWithIdleSessionSendsRatherThanStarts
TestDispatchWithOccurrenceKeyDeduplicates
TestDispatchWithStartingSessionIsAlsoBusy
TestDispatchWithoutDispatcherReturnsUnavailable
TestEnsureCreatesConversation
TestEnsureCrossPluginSeparation
TestEnsureCrossWorkspaceSeparation
TestEnsureFindsConversationBeyondFirstPage
TestEnsureProfileInDescriptorFromExistingTask
TestEnsureProfileValidatorReceivesConfiguredID
TestEnsureReconcilesExistingConversationProfileAndBasePrompt
TestEnsureRejectsDisabledProfileWithZeroPartialRows
TestEnsureRejectsDisabledWorkspaceDefaultProfileWithoutPartialRows
TestEnsureRejectsEmptyArguments
TestEnsureRejectsMissingProfileWithZeroPartialRows
TestEnsureRejectsMissingWorkspaceDefaultProfileWithoutPartialRows
TestEnsureRejectsProfileChangeForLiveConversationSession
TestEnsureRepairRefusedWhenBoundProfileBecomesInvalid
TestEnsureRepairsMissingPrimarySession
TestEnsureRepairsWrappedMissingPrimarySessionOverRealRepository
TestEnsureReturnsExistsOnSecondCall
TestEnsureStoresProfileIDInMetadata
TestEnsureUsesOneConversationAcrossServiceInstances
TestEnsureUsesWorkspaceDefaultProfileWhenPluginOmitsProfile
TestEnsureWithoutProfileConfiguredUsesWorkspaceDefault
TestIsManagedConversationTask
TestIsManagedConversationTaskHonorsEphemeralMetadata
TestManagedConversationApprovalChangeInvalidatesAndStopsPolicy
TestManagedConversationConcurrentRevisionConflict
TestManagedConversationConfigurationUsesRevisionAndIdleBoundary
TestManagedConversationEnsureOperationReplay
TestManagedConversationEnsureRepairsAcceptedOperation
TestManagedConversationIdentity
TestManagedConversationPauseStopsActiveExecution
TestManagedConversationSurvivesLegacyPluginCleanup
TestManagedConversationUninstallDetachesAndHidesRetainedTranscript
TestManagedInputAdmissionEnforcesConfiguredQueueLimit
TestManagedInputCancelAcceptedAndRequiresExactStopConfirmation
TestManagedInputEnqueueReplayConflictAndBoundedList
TestManagedInputEnqueueReturnsReceiptBeforeQueueWakeCompletes
TestManagedInputImmediateDispatchNeverQueuesAndReplaysStatus
TestManagedInputImmediateDispatchReturnsBusyForSessionAndQueuedInput
TestManagedInputPauseRetainsAcceptedWorkAndUnpauseWakesQueue
TestManagedInputRejectsInvalidCoalescingAndStaleAuthority
TestManagedPeriodicInputCoalescesOnlyPendingPeriodicWork
TestStoresBasePromptInMetadata
```
