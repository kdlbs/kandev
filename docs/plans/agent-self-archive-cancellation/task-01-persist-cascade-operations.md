---
id: "01-persist-cascade-operations"
title: "Persist cascade operations and membership admission"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
  - REQ-TASKS-EXTERNAL-ID-001
  - REQ-TASKS-EXTERNAL-ID-SCENARIOS-001
  - REQ-TASKS-EXTERNAL-ID-BOUNDARIES-001
  - REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-001
  - REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-002
acceptance_criteria:
  - AC-TASKS-RUNTIME-CLEANUP-001.6
  - AC-TASKS-RUNTIME-CLEANUP-001.11
  - AC-TASKS-RUNTIME-CLEANUP-001.13
  - AC-TASKS-RUNTIME-CLEANUP-001.18
  - AC-TASKS-RUNTIME-CLEANUP-001.19
  - AC-TASKS-RUNTIME-CLEANUP-001.20
  - AC-TASKS-RUNTIME-CLEANUP-001.22
  - AC-TASKS-EXTERNAL-ID-001.5
  - AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.1
  - AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.2
  - AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-002.1
  - AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-002.2
  - AC-TASKS-RUNTIME-CLEANUP-001.35
  - AC-TASKS-RUNTIME-CLEANUP-001.38
  - AC-TASKS-RUNTIME-CLEANUP-001.51
  - AC-TASKS-RUNTIME-CLEANUP-001.54
  - AC-TASKS-RUNTIME-CLEANUP-001.57
system_design:
  - ../../specs/tasks/system-design/detached-workspace-continuity.md
  - ../../specs/tasks/system-design/runtime-cleanup.md
  - ../../specs/tasks/system-design/durable-archive-cascades.md
  - ../../specs/tasks/system-design/archive-cascade-execution-contracts.md
  - ../../specs/tasks/system-design/archive-cascade-boundary-contracts.md
  - ../../specs/tasks/system-design/external-id-idempotency.md
  - ../../specs/tasks/system-design/task-creation-protocol.md
  - ../../specs/tasks/system-design/archive-cleanup-evidence.md
  - ../../specs/tasks/system-design/archive-cascade-registries.md
  - ../../specs/tasks/system-design/workspace-config-fence.md
  - ../../specs/tasks/system-design/workspace-deletion-table-registry.md
---

# Task 01: Persist Cascade Operations and Membership Admission

## Summary

Add the fenced archive-operation ledger, immutable task/group membership,
transactional event outbox, and complete membership-writer admission.

## In scope

- Add creation operations/typed step ledger, release replay, cascade operations;
  task/group jobs; resource/config claims; retained revisions/gaps/watermarks;
  `task_resource_cleanup_snapshots`, `task_launch_dispatch_claims`, and
  append-only `task_resource_cleanup_snapshot_evidence`; outboxes/steps; and
  diagnostic/audit schemas without cascading evidence FKs.
- Add CreationPlan/handle Begin, step claim/success/failure/unknown/
  compensation APIs, coordinator, `completing`, Complete/Abort, and persist
  recovery APIs. Runtime recovery/reconciliation/compensation/workspace-delete
  worker is Task 03. Migrate every insert/helper/finalizer/settler and
  compensation; request errors invoke Abort only when ledger-eligible.
- Add REST/MCP identity lookup before non-identity validation. Implement required
  stable release operation IDs/results; unfinished has no event, completed
  advances revision, and replay never reads a later holder.
- Route Task/MCP, expiry/profile, exact provider/reset, automation, plugin, and
  e2e callers through typed delete commands and trusted canonical-provenance
  constructors. Persist exact reason bytes in delayed jobs; remove all fallbacks.
- Implement every exact row in `workspace-deletion-table-registry.md`, including
  `workspace_deletion_steps`, executable owner joins, nullable-owner rules,
  retention, and order. Own only the writer, typed-table, provenance, and
  lifecycle-publisher registry entries; Task 02 owns config-surface entries,
  Task 03 owns startup/compensation/reconciliation entries, and Task 04 owns the
  declared export/verifier.
- Execute the migration-only catalog for `task_session_worktrees`, deprecated
  flat `task_environments` worktree columns, and invalid preview
  `session_delete` cleanup rows; preserve source precedence, hashes, rollback,
  and commit-unknown replay before dropping legacy schema.
- Persist sorted task tombstones before workspace event and durable external
  aftermath steps; no provider/config filesystem I/O occurs in SQL.
- Persist full versioned group/member snapshots and retained-key metadata for
  authenticated legacy binding.
- Implement concrete sealed actors and transaction checks for user, task session,
  maintenance, scheduled, recovery, cleanup, outbox, operator, and installation
  actions. Implement all 17 lock tiers, absent-row restart, sorted bulk locks,
  bounded traversal, and generation-fenced claims.
- Migrate knowable identities into explicit historical gap/watermark state
  without marking retained events delivered.

## Acceptance

- Same-DB failpoints prove handle/plan/token/actor binding, every durable step
  transition/evidence/lease/unknown/compensation, Complete ambiguity/recovery,
  Abort eligibility/prohibition, revision-zero Begin, one Created event, and
  workspace delete ordering against both unsettled states.
- Reservation-versus-creation and task-delete races
  linearize through the reserved membership-generation/admission barrier;
  changed fences restart or return typed conflicts, with cross-workspace
  identity locks acquired first. A concurrent external-baseline reparent
  (`Store.SetTaskParent`) hits the same typed conflict, cross-checked here as a
  non-owning fence dependency and never re-verified as this package's
  behavior.
- Expired creation work is recovered through a privileged
  `OperationRecoveryActor` and `RecoverTaskCreation(operationID, actor, action)`;
  no caller `CreationHandle` or completion token is reconstructed from hashes.
- Workspace deletion partitions creation states: preparing and durable
  `preparing-release` have retained `workspace_deleted` disposition with no
  task revision/tombstone; completing atomically emits revision-one
  `task.created` then revision-two `task.deleted`; settled tasks use one ordered
  delete tombstone. Release CAS is `preparing -> preparing-release -> preparing`
  with cleared external-ID claim and no event; replay/recovery cannot create a
  completed holder.
- REST/MCP Found settled/unsettled wins over missing/invalid/stale non-identity
  fields with no side effects; transport/auth/workspace/identity errors precede.
- Release versus Complete/Abort/Delete/reuse yields specified outcome/order.
  Same stable release operation replay returns stored result without inspecting
  a later holder on both dialects.
- Registry rejects every legacy insert/finalizer/settler/release/rollback/direct
  publisher, unmapped or noncanonical deletion provenance, missing exact table
  row/predicate, and name/raw-path config API.
- Task/Office signatures preserve confirmation/actor; locked rename/auth changes
  write nothing. Incident blockers/comments only.
- Independent inventory proves every production table is exactly classified;
  GitHub registrations/deliveries survive and provider children obey joins/order.
- Every config replacement method uses Resolver/Fence/VerifiedRoot lifecycle;
  same-name/state/generation/marker/legacy ambiguity performs no I/O.
- Full group/member snapshot round-trips every field; schema/hash/key metadata
  survives live-row deletion.
- Every concrete actor succeeds only with current authorization mode and complete
  persisted binding; missing/foreign/spoofed/stale identity or lease inserts
  nothing.
- Every command follows published tier mapping. PostgreSQL advisory-key and
  SQLite immediate-write races, digest collision, candidate change, and whole-
  transaction uniqueness/deadlock restart are deterministic.
- Detachment integration cross-checks the external baseline's
  `Store.DetachTask` and narrow `SetTaskParent` ledgers; this package owns only
  the shared-ledger replay, lock-order composition, and route cutover around
  them, never their shipped mode/owner branching, actor and generation fencing,
  closed admission, or lifecycle outbox atomicity, which stay covered by the
  completed baseline suite. Task 04 verifies the in-scope detach ledger replay,
  crash, cleanup-race, sibling-lock, and adjacent-inversion cases on both
  dialects.
- Migration advances only across explicit absent historical gaps, delivers any
  real retained row first, handles first post-epoch creation, and diagnoses
  conflicts without inventing history.
- Discovery uses bounded depth and a `max_cascade_members+1` sentinel before
  sorting or digesting; member-count, depth, and discovery-overflow tests own
  this allocation boundary.

## Verification

```bash
go test ./internal/task/archivecascade -run '^TestArchiveCascadeAggregate_(CreationHandle|CreationStepLedger|CreationCompleting|CreationRecovery|CreationManifest|CreationStepKind|CreationEvidence|CreationCompensation|WorkspaceDeleteUnsettledCreation|ExternalIDKeyCollision|ExternalIDCollation|ExternalIDSchemaAttestation|ExternalIDMigrationReplay|ExternalIDRelease|ExternalIDReleaseReplayAfterReuse|ExternalIDLookupPreflight|WriterRegistry|DeletionProvenance|WorkspaceConfirmation|WorkspaceTableInventory|GitHubRegistrationRetention|BlockerCommentOwnership|ConfigReplacementAPI|LockOrder|RevisionHistoricalGaps|ActorBindings|GroupSnapshot|Failpoint)$' -count=1
KANDEV_TEST_POSTGRES_DSN="$KANDEV_TEST_POSTGRES_DSN" go test ./internal/task/archivecascade -run '^TestArchiveCascadeAggregate_Postgres(CreationHandle|CreationStepLedger|CreationStepKind|CreationEvidence|CreationCompensation|CreationCompleting|WorkspaceDeleteUnsettledCreation|ExternalIDKeyCollision|ExternalIDCollation|ExternalIDSchemaAttestation|ExternalIDMigrationReplay|ExternalIDRelease|ExternalIDReleaseReplayAfterReuse|ExternalIDLookupPreflight|Retention|DeletionProvenance|WorkspaceConfirmation|WorkspaceTableInventory|GitHubRegistrationRetention|BlockerCommentOwnership|ConfigReplacementAPI|LockOrder|HistoricalGaps|ActorBindings|Failpoint)$' -count=1
go test ./internal/task/repository/sqlite ./internal/office/repository/sqlite ./internal/task/service ./internal/office/service ./internal/mcp/handlers ./internal/backendapp -run '^(Test.*CreationHandleDelegate|Test.*CreationStepLedger|Test.*WorkspaceDeleteUnsettledCreation|Test.*ExternalIDPreflight|Test.*ExternalIDReleaseReplayAfterReuse|Test.*DeletionProvenance|Test.*WorkspaceConfirmPropagation|Test.*WorkspaceTableInventory|Test.*PublisherCutover|Test.*CascadeMembershipAdmission|Test.*CrossWorkspaceDescendant|Test.*CascadeDiscoveryBounds|Test.*CascadeDiscoveryOverflow|Test.*AggregateTransactionRollback|Test.*CleanupWorkerLeaseRace|Test.*GroupWorkspaceAuthorization|Test.*BlockedCandidateBarrier|Test.*DetachTaskModeBranch|Test.*DetachTaskCrashRollback|Test.*CreationRouteAttachment|Test.*CreationAttachmentFailure)$' -count=1
```

Use channel or `testing/synctest` coordination. For every writer, begin before reservation and release after the competing transaction reaches its lock; do not use sleeps.

## Files likely touched

- new `apps/backend/internal/task/archivecascade/`
- `apps/backend/internal/task/repository/sqlite/`
- `apps/backend/internal/office/repository/sqlite/`
- task and Office repository/service interfaces and callers
- dedicated aggregate integration and focused delegation tests
- `apps/backend/internal/task/archivecascade/registry/writers.go`
- `apps/backend/internal/task/archivecascade/registry/tables.go`
- `apps/backend/internal/task/archivecascade/provenance_registry.go`

## Dependencies

None.

## Risks

- Any unlisted direct writer bypasses admission.
- A two-repository test is not proof unless both use the same DB and aggregate Tx.
- Aggregate package dependencies must avoid task/Office import cycles.
- PostgreSQL lock behavior still needs the dedicated integration suite.

## Parallelism

`sequential`

## Results

Pending.
