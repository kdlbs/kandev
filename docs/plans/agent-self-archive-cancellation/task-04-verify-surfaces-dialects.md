---
id: "04-verify-surfaces-dialects"
title: "Verify archive surfaces and database dialects"
status: pending
wave: 4
depends_on:
  - "03-resume-archive-unarchive"
plan: "plan.md"
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
  - REQ-TASKS-EXTERNAL-ID-001
  - REQ-TASKS-EXTERNAL-ID-SCENARIOS-001
  - REQ-TASKS-EXTERNAL-ID-BOUNDARIES-001
  - REQ-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001
  - REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-001
  - REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-002
acceptance_criteria:
  - AC-TASKS-RUNTIME-CLEANUP-001.1
  - AC-TASKS-RUNTIME-CLEANUP-001.2
  - AC-TASKS-RUNTIME-CLEANUP-001.3
  - AC-TASKS-RUNTIME-CLEANUP-001.4
  - AC-TASKS-RUNTIME-CLEANUP-001.5
  - AC-TASKS-RUNTIME-CLEANUP-001.8
  - AC-TASKS-RUNTIME-CLEANUP-001.9
  - AC-TASKS-RUNTIME-CLEANUP-001.23
  - AC-TASKS-RUNTIME-CLEANUP-001.25
  - AC-TASKS-RUNTIME-CLEANUP-001.27
  - AC-TASKS-EXTERNAL-ID-BOUNDARIES-001.1
  - AC-TASKS-EXTERNAL-ID-BOUNDARIES-001.2
  - AC-TASKS-EXTERNAL-ID-BOUNDARIES-001.3
  - AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.3
  - AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.4
  - AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.5
  - AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.6
  - AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-002.3
  - AC-TASKS-RUNTIME-CLEANUP-001.28
  - AC-TASKS-RUNTIME-CLEANUP-001.31
  - AC-TASKS-RUNTIME-CLEANUP-001.33
  - AC-TASKS-RUNTIME-CLEANUP-001.48
  - AC-TASKS-RUNTIME-CLEANUP-001.53
  - AC-TASKS-RUNTIME-CLEANUP-001.55
  - AC-TASKS-RUNTIME-CLEANUP-001.56
  - AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.1
  - AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.2
  - AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.3
  - AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.4
  - AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.5
  - AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.6
  - AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.7
  - AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.8
  - AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.9
system_design:
  - ../../specs/tasks/system-design/detached-workspace-continuity.md
  - ../../specs/tasks/system-design/runtime-cleanup.md
  - ../../specs/tasks/system-design/durable-archive-cascades.md
  - ../../specs/tasks/system-design/archive-cascade-execution-contracts.md
  - ../../specs/tasks/system-design/archive-cascade-boundary-contracts.md
  - ../../specs/tasks/system-design/archive-cleanup-evidence.md
  - ../../specs/tasks/system-design/runtime-state-publication-order.md
  - ../../specs/tasks/system-design/external-id-idempotency.md
  - ../../specs/tasks/system-design/task-creation-protocol.md
  - ../../specs/tasks/system-design/workspace-config-fence.md
  - ../../specs/tasks/system-design/workspace-deletion-table-registry.md
  - ../../specs/tasks/system-design/runtime-startup-registry.md
  - ../../specs/tasks/system-design/archive-cascade-registries.md
  - ../../specs/tasks/system-design/session-delete-resource-cleanup.md
  - ../../specs/tasks/system-design/environment-owned-git-status.md
---

# Task 04: Verify Archive Surfaces and Database Dialects

## Summary

Prove the completed state machines through explicit and scheduled production
wiring and through SQLite/PostgreSQL task and Office repository behavior.

## In scope

- Compose REST/MCP lookup-first creation, handle/CreationPlan/step coordinator,
  aggregate release, every canonical provenance adapter, Task/Office workspace
  deletion, workers, and operator routes against one Store.
- Prove the sole production seam: construct without starts; bind bootstrap;
  initial setup plus agentctl prerequisite; Runtime; then hostname/gateway/
  host-utility/orchestrator/all pollers/plugins/delivery/routes/readiness. Cover
  every failure cleanup and stop-once reset/restore/shutdown.
- Independently scan every startup-owned package for starts, goroutines,
  subscriptions, processes, listeners, sweeps, and publication; exact-match each
  discovered symbol/stage/effect/cleanup/readiness field to the B/Q/R/P/U
  catalog and inject every partial-start boundary.
- Cover self-cancellation, every phase/repeat/task/workspace outcome, original
  confirmation/actor, workspace deletion of both unsettled creation states,
  every config replacement API, stable release replay after reuse, exact group
  restore, canonical deletion provenance, and perpetual retry.
- Run `scripts/verify_archive_cascade_registries.py` as an independent
  production scan. It must derive observed DDL/SQL/AST/SSA/startup entries
  without importing implementation registry data, then compare names, columns,
  collations, constraints, aliases, config access, provenance, publishers, and
  B/Q/R/P/U effects.
- The Task 03-owned startup catalog at
  `apps/backend/internal/backendapp/startup_registry.go` is scanned
  independently. The verifier owned here is
`scripts/verify_archive_cascade_registries.py`. Its production CLI is
`--production --compare <declared-json> [--repo-root <path>]`; its separate
dialect-attestation mode is
`--attest --sqlite <dsn> --postgres <dsn> --compare <declared-json>`.
Attestation introspects both catalogs into canonical `ObservedEntry` values and compares them to the typed export without importing registry packages. SQLite DSN comes from `KANDEV_TEST_SQLITE_DSN` (a temp-file path created by the harness; default `$(mktemp -u)/attest-sqlite.db` is not accepted as proof of persistence). Both DSNs are required; missing/unreachable either dialect leaves verification incomplete, never a pass.
  Both modes write deterministic JSON with sorted `artifact`, `key`, and
  `fields` values and no diagnostics to stdout. Exit codes are `0` equal,
  `2` usage/input, `3` independent discovery/attestation failure,
  `4` declared/observed mismatch, and `5` unsupported toolchain/dialect.
  Script existence, `--help`, empty-production rejection, missing-dialect
  rejection, and each exit code are acceptance-tested.
- Verify `AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.9` through the existing
  environment-keyed session-runtime store and boot-hydration tests. This package
  changes no layout or touch interaction; the environment-owned Git-status
  design's existing mobile parity note remains authoritative, so no new mobile
  Playwright scenario is required.
- Independently prove runtime AC `.1`-`.5` and `.7`-`.9`: all owned runtimes stop
  before cleanup; session delete retains task resources; shared/borrowed teardown
  defers; process groups fully terminate; stale-row outcomes are closed; removed
  worktrees recreate; newer CAS survives silently; exact Git/path/branch/commit
  proof and physical-absence evidence are mandatory.
- Both dialect suites exercise every command row in the lock map, including
  creation-step and compensation claims, creation recovery, workspace-deletion
  aftermath, and the delivery transaction's tier-6 watermark/gap lock before
  tier-16 outbox lock. Candidate changes, adjacent inversions, dependency
  release, and workspace-event races restart deterministically.
- Compare exact writer/config-surface/provenance/publisher registries and every
  compensation Abort; reject legacy name/raw-path and reasonless APIs.
- Run one shared DB per dialect for creation-step/release failpoints, advisory-
  key collisions/candidate changes, all lock tiers, races, deadlines, proof,
  revisions/outboxes, Runtime startup/restart/quiescence, and deadlock freedom.
- Verify the Task 03-owned exact public CLI docs command/scope/auth/replay/
  redaction/JSON/exit strings and validate concrete tokens.
- Run project, spec, and public-document gates and record results.

## Acceptance

- Every source uses a concrete sealed actor and one Store/Runtime; spoofed/stale
  bindings, public wrapper bypasses, and direct lifecycle publication have zero
  effects. MCP self-archive survives cancellation.
- Focused tests map every runtime AC `.1`-`.9` to an observable assertion,
  including DB-row versus physical-path absence and no-warning newer-generation
  CAS. No criterion is satisfied only by aggregate archive tests.
- Both dialects prove handle/plan binding and every step state including
  `launch_intent` `cancelled/workspace_deleted`, lease/evidence transitions,
  unknown reconciliation, compensation, `complete_preparing` recovery from
  armed or all-required-succeeded `preparing`, and Complete/Abort decision.
  They prove one created event and workspace deletion against both unsettled
  states.
- REST/MCP Found states ignore malformed non-identity payload; envelope errors
  retain precedence with zero side effects.
- Release races all creation/delete states. Same-operation replay returns stored
  result and never reads a later holder.
- The independent verifier exists at the declared path, exposes the exact CLI
  and deterministic stdout/exit contract above, rejects empty production
  discovery, and passes only when observed and declared registries are equal.
- Session deletion leaves the task environment and environment-keyed Git status
  visible to both desktop and mobile Changes consumers without reload or a
  replacement session; focused store/hydration tests prove the same message
  ordering and no stale-file restoration.
- Independent DDL inventory equals exact registry rows; every predicate/order and
  retained invariant executes, including GitHub registration/delivery survival.
- Operation uniqueness covers only open phases and releases its root barrier for
  terminal `completed`/`aborted` rows while preserving operation-ID replay.
  Dialect tests prove retained runtime/resource/Git evidence survives live-row
  deletion, resume-claim fencing, launch-intent cancellation, and unique
  workspace-aftermath step replay.
- Crash after launch arming or the final required step recovers `preparing` via
  `complete_preparing`; overlapping recovery predicates select one action.
- Crash after creation phase A and a definitively failed `preparing` recovery
  selects `complete_completing` or the exact `abort_preparing` predicate,
  respectively, with generation/lease fencing.
- Archive operation retry_wait and marked-membership recovery resume under
  operation/scope generation and lease fencing.
- A crash after cleanup preparation but before the archive activation commit
  cannot claim cleanup or delete active workspace data; only committed activation
  permits `resume_pending_cleanup`.
- Cleanup/group worker claims race workspace deletion and unarchive while taking
  workspace tier 1 then operation/scope tier 4 before late-tier rows; no
  inversion or absent-workspace resurrection is accepted.
- DeleteTask Git registration/branch cleanup remains `cascade_retry` after
  attempt eight and is recoverable after restart; only other bounded delete jobs
  may terminalize.
- Root-only archive rejects a foreign-workspace descendant before reserving
  operation/scope or membership rows and before any stop, cleanup, archive, or
  restore effect. Workspace deletion preserves foreign members/resources,
  rehomes deleted-workspace group stewardship before resource selectors run,
  retains live foreign FKs, and fences stale cleanup claims.
- Unarchive remains `restore_pending` until every required physical
  materialization claim is ready; a `confirmed_missing` member blocks exact
  group reconstruction and keeps that group not-ready.
- Repeated archive against `restore_pending` returns the stored pending
  unarchive result without a new archive claim; foreign-scope workspace deletion
  replays `foreign_scope_blocked` without releasing or touching foreign members.
- Archive activation recovery resumes every `prepared` cleanup job to `pending`
  across its failpoint; `complete_archive` is impossible while any prepared job
  remains, and replay cannot strand physical cleanup.
- Workspace deletion transfers both `running` and expired `unknown`
  materialization claims with persisted `workspace_deleted` disposition and
  delete ownership; stale I/O and replay are fenced by generation.
- Launch-dispatch deletion transfer has precedence over provider/no-dispatch
  reconciliation; deletion-fenced claims cannot become succeeded or retryable.
- Release-operation tests cover closed claim/terminal states, deletion-generation
  fencing, holder-independent replay, and the shared workspace lock prefix.
- Migration from a database with neither external-ID column exercises both
  dialects' nullable add, canonical index, attestation, failpoint rollback,
  readiness gate, and the following archive/creation races.
- Archive completion refuses `retry_wait`/`unknown` nonmissing cleanup jobs and
  requires `succeeded`; only confirmed-missing jobs with durable `delete_owned`
  transfer are an exception.
- A permanent step failure with an armed launch intent cannot select
  `abort_preparing`; the locked coordinator rejects that impossible transition,
  while workspace-delete Abort alone can disarm with `absent_proven` evidence.
- Mixed external-ID schemas with populated IDs and a missing settledness column
  retain a blocking diagnostic unless authenticated settlement evidence exists;
  neither dialect silently settles or unsettles those rows.
- Response loss after exact missing-group insertion replays by operation/snapshot
  hash, ownership generation, and member set; only mismatches block.
- Creation recovery with a permanent failure plus another planned step cannot
  select `resume_preparing`; only the exact abort predicate is legal, and the
  mixed planned-plus-failed case is covered.
- Delivery races prove the retained per-epoch cursor serializes task/session
  `(revision, queue_sequence)` successors across crash and retry; the retained
  materialization-claim TableEntry is present in deletion inventory and stale
  claims are fenced.
- Materialization `unknown|blocked|retry_wait` recovery selects the closed
  reconciliation/retry action, while workspace deletion cancels or transfers
  claims and stale I/O is fenced.
- Launch-dispatch unknown recovery proves provider success, safe no-dispatch
  retry, or deletion transfer; no orphan launch or replay bypass is accepted.
- Workspace deletion racing `preparing-release` locks and terminalizes the
  retained release operation before clearing the task identity; same-operation
  replay remains stable.
- An `archived` operation racing workspace deletion transitions atomically to
  terminal `workspace_deleted`, preserves delete-owned evidence/dispositions,
  releases its barrier, and rejects restore/unarchive replay.
- Release cannot enter `preparing-release` with unknown/running steps or pending
  compensation, and release of `completing` returns a typed conflict while
  phase-B ownership/replay remains unchanged.
- Launch arming requires all non-launch steps succeeded; an armed launch cannot
  coexist with a planned required step.
- Detachment uses one `Store.DetachTask` transaction with barrier 4 -> task 5 ->
  revision 6 -> effects/claims 7-10 -> environment/repository 11 ->
  group/member 12 -> cleanup/markers 13-14 -> task outbox 16; tiers 2, 3, 15,
  and 17 are intentionally empty. Crash/replay preserves one canonical
  lifecycle envelope, and delayed delivery/tombstone races remain ordered and
  deadlock-free.
- Detachment transfers stewardship only for `inherit_parent` with exact
  parent-owned group/environment and matching actor, linkage, membership, and
  generations; `shared_group` and `new_workspace` preserve mode, membership,
  owner, and generations. The closed admission matrix covers every active,
  uncertain, failed-with-claim, cancelled/restored, and transferred operation,
  claim, step, job, and marker state, with no wait or transfer.
- PostgreSQL verification uses the named `PostgresCascadeFixture` helper. It
  requires `KANDEV_TEST_POSTGRES_DSN` to be non-empty, opens a pool with
  `MaxOpenConns >= 2`, creates a unique schema per test run, and drops that
  schema during cleanup. The fixture asserts pool capacity before race cases;
  session-level advisory-lock guards hold a dedicated connection for the
  entire guarded operation and never borrow it for another transaction. The
  named gate has no `t.Skip`: an unset environment is reported pending, while
  a non-empty malformed or unreachable DSN fails.
- Group/resource rehome is a canonical `group_rehome` workspace-deletion step
  with snapshot, idempotency key, lease/generation/due state, and
  `retry_wait(rehome_pending)` recovery after restart. It acquires the same
  physical guard before superseding `cleanup_claimed`; unknown recovery proves
  untouched, already-cleaned, or ambiguous outcomes, never treating unknown as
  success. Rehome-vs-I/O-start tests prove no old worker deletes a rehomed
  resource.
- Absent-root authenticated `reconcile_foreign_scope` resolves a blocked legacy
  scope without touching foreign members; unauthenticated or mismatched proof
  remains blocked and replay is deterministic.
- The foreign-scope operation persists as canonical
  `foreign_scope_blocked` within root uniqueness; its reconciliation is reachable
  in auth-enabled multi-workspace scope and releases only after preserving
  foreign members/resources.
- Schema verification compares the complete typed `TableEntry` fields
  (columns/types/nullability/defaults/collations, indexes/predicates/uniques/
  collations, checks, foreign-key actions/update actions/deferrability, lock
  tier, ordinal, retention/lifecycle, invariants, aliases, and dialect DDL hash)
  against live SQLite and PostgreSQL catalogs; the generated Markdown inventory
  must match it and cannot be a second schema authority.
- Transport tests delay task/session events and workflow snapshots across
  terminal tombstones and lifecycle epochs; stale revisions are discarded and
  accepted envelopes remain monotonic.
- Post-delete Phase B and delivery retries lock the present/absent workspace
  identity key, and workspace-deletion-step/outbox/dependency races are
  deterministic after the live workspace row is gone. Parent/session/runtime/
  environment writers prove barrier-first admission against archive, creation,
  and cleanup operations.
- Every deletion caller accepts only its trusted canonical SourceID constructor;
  missing/malformed provenance and every old fallback have zero writes.
- Workspace deletion preserves unrelated blocker/comment rows and rejects stale
  Task/Office confirmation or actor.
- Every replacement config API denies stale path/state/generation/marker before
  filesystem/subprocess I/O; no name/raw-path API remains.
- Startup assertions cover each enumerated pre-bind and post-gate starter:
  bootstrap liveness survives initial setup, agentctl health, and Runtime retry;
  readiness stays false; fatal/downstream failures reverse-clean every acquired
  resource; stop-once quiesces reset/restore/shutdown.
- Operator scope/auth/replay/audit/redaction/DTO/status/JSON/exit contracts
  survive both dialects. CLI documentation asserts exact tokens; every gate
  passes.

Traceability verification reads
`docs/specs/tasks/system-design/archive-cascade-traceability.yaml` without
importing implementation registries. It derives each AC prefix from the source
requirement ID, expands the declared ranges, and rejects any missing, duplicate,
or extra requirement/AC mapping, unresolved section anchor, missing work-order
owner, or
verification name absent from the named command groups. The canonical vocabulary
YAML is validated by the same independent gate.

## Verification

```bash
cd apps/backend
go run ./internal/task/archivecascade/registry/export --output internal/task/archivecascade/registry/declared.json
go run ./internal/task/archivecascade/registry/export-markdown --output ../../docs/specs/tasks/system-design/workspace-deletion-table-registry.md
python3 ../../scripts/verify_archive_cascade_registries.py --production --compare internal/task/archivecascade/registry/declared.json
python3 ../../scripts/verify_archive_cascade_registries.py --check-markdown --compare internal/task/archivecascade/registry/declared.json
python3 ../../scripts/verify_archive_cascade_registries.py --check-traceability --vocabulary ../../docs/specs/tasks/system-design/archive-cascade-vocabulary.yaml --traceability ../../docs/specs/tasks/system-design/archive-cascade-traceability.yaml --compare internal/task/archivecascade/registry/declared.json
python3 ../../scripts/verify_archive_cascade_registries.py --attest --sqlite "$KANDEV_TEST_SQLITE_DSN" --postgres "$KANDEV_TEST_POSTGRES_DSN" --compare internal/task/archivecascade/registry/declared.json
go test ./internal/backendapp -run '^Test(HTTP|MCP|ExternalIDPreflight|ExternalIDReleaseReplayAfterReuse|BootstrapStarterInventory|BootstrapLiveness|AgentctlPrerequisite|RuntimeTransientReadiness|RuntimeFatalListenerClose|PostGateFailureCleanup|RuntimeStopOnce|Scheduled|TaskWorkspaceDelete|OfficeWorkspaceDelete|WorkspaceDeleteUnsettledCreation|CleanupWorker|TaskOutboxWorker|WorkspaceOutboxWorker)ArchiveComposition_' -count=1
go test ./internal/task/service ./internal/agent/runtime/... ./internal/task/archivecascade -run '^Test.*(StopsAllOwnedRuntimes|SessionDeletePreservesTaskResources|SessionDeletePreservesEnvironment|SessionDeleteNoPhysicalCleanup|SessionDeleteSharedEnvironment|SessionDeleteLastSession|SessionReuseRetainedWorkspace|SessionCleanupOwnerBoundary|SessionlessTaskCleanupDiscovery|SessionDeleteLiveViews|BorrowedResourcePreservation|SharedEnvironmentDefersCleanup|ProcessGroupTermination|StartupStaleRuntimeRow|DeadRowRepairCASNewerGeneration|UnarchiveRecreatesOwnedWorktree|GitWorktreeExactProof|PhysicalAbsenceProof|CleanupAdmissionMatrix|CanonicalResourceKeys|ExternalIDBoundaryAuthorization|CreationHandleVisibility|DeferredSurfaceBoundary)' -count=1
go test ./internal/task/repository/sqlite ./internal/office/repository/sqlite ./internal/office/configloader ./internal/office/config ./internal/task/service ./internal/office/service ./internal/mcp/handlers ./internal/automation -run '^Test.*(CreationHandle|CreationStepLedger|CreationCompleting|WorkspaceDeleteUnsettledCreation|ExternalIDPreflight|ExternalIDBoundaryAuthorization|ExternalIDKeyCollision|ExternalIDReleaseReplayAfterReuse|CanonicalDeletionProvenance|WorkspaceConfirmActorRace|OfficeWorkspaceDeleteActorRace|WorkspaceTableInventory|GitHubRegistrationRetention|ConfigReplacementAPI|BlockerCommentOwnership|WorkspaceDeleteAggregate|WriterRegistry|PublisherCutover)' -count=1
go test ./internal/launcher ./internal/backendapp -run '^TestArchiveCascadeReconcile(CLI|Loopback|Scope|Pagination|Replay|SecurityAudit|Auth|Output|ExitCode|DocsContract)' -count=1
go test ./internal/task/archivecascade -count=1
KANDEV_TEST_POSTGRES_DSN="$KANDEV_TEST_POSTGRES_DSN" go test ./internal/task/archivecascade -run '^TestPostgres(DetachmentDSNGate|DetachTaskModeBranch|DetachTaskCrashRollback|DetachTaskPublicationReplay|DetachTaskCleanupRace|DetachTaskRace|DetachTaskSiblingLockOrder|DetachTaskAdjacentLockInversion)$' -count=1
KANDEV_TEST_POSTGRES_DSN="$KANDEV_TEST_POSTGRES_DSN" go test ./internal/task/archivecascade -run '^TestPostgres' -count=1
cd ../..
make fmt
make typecheck test lint
node --test --test-name-pattern='archive cascade reconciliation CLI contract' scripts/validate-public-docs.test.mjs
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.py --all
git diff --check
```

If `KANDEV_TEST_SQLITE_DSN` or `KANDEV_TEST_POSTGRES_DSN` is unavailable, do not claim dialect verification complete; report the missing prerequisite and keep this work order pending. Production-only success plus Go integration tests never substitute for independent attestation.

## Files likely touched
- `apps/backend/internal/backendapp/*archive_composition_test.go`
- `apps/backend/internal/task/archivecascade/`
- native launcher CLI/parser and dedicated-loopback route tests
- task/Office aggregate-delegation and scheduled composition tests
- task specifications and plan results
- `scripts/verify_archive_cascade_registries.py`
- `apps/backend/internal/task/archivecascade/registry/export`
- `apps/backend/internal/task/archivecascade/registry/declared.json` (generated export)
- frontend environment-keyed store/hydration tests covering desktop/mobile Changes

## Dependencies

- `03-resume-archive-unarchive`

## Risks

- Handler-unit injection is not production composition evidence.
- Separate-package tests do not prove one cross-schema transaction.
- Skipped PostgreSQL tests are not a passing result.

## Parallelism

`sequential`

## Results

Pending.
