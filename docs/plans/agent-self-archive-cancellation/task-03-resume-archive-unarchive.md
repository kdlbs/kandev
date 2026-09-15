---
id: "03-resume-archive-unarchive"
title: "Resume partial archive and unarchive cascades"
status: pending
wave: 3
depends_on:
  - "01-persist-cascade-operations"
  - "02-harden-cleanup-identity"
plan: "plan.md"
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
  - REQ-TASKS-EXTERNAL-ID-001
  - REQ-TASKS-EXTERNAL-ID-SCENARIOS-001
  - REQ-TASKS-EXTERNAL-ID-BOUNDARIES-001
acceptance_criteria:
  - AC-TASKS-RUNTIME-CLEANUP-001.7
  - AC-TASKS-RUNTIME-CLEANUP-001.12
  - AC-TASKS-RUNTIME-CLEANUP-001.14
  - AC-TASKS-RUNTIME-CLEANUP-001.15
  - AC-TASKS-RUNTIME-CLEANUP-001.16
  - AC-TASKS-RUNTIME-CLEANUP-001.17
  - AC-TASKS-RUNTIME-CLEANUP-001.21
  - AC-TASKS-RUNTIME-CLEANUP-001.24
  - AC-TASKS-RUNTIME-CLEANUP-001.26
  - AC-TASKS-EXTERNAL-ID-001.1
  - AC-TASKS-EXTERNAL-ID-001.2
  - AC-TASKS-EXTERNAL-ID-001.3
  - AC-TASKS-EXTERNAL-ID-001.4
  - AC-TASKS-EXTERNAL-ID-001.6
  - AC-TASKS-EXTERNAL-ID-001.7
  - AC-TASKS-EXTERNAL-ID-001.8
  - AC-TASKS-EXTERNAL-ID-SCENARIOS-001.1
  - AC-TASKS-EXTERNAL-ID-SCENARIOS-001.2
  - AC-TASKS-EXTERNAL-ID-SCENARIOS-001.3
  - AC-TASKS-EXTERNAL-ID-SCENARIOS-001.4
  - AC-TASKS-RUNTIME-CLEANUP-001.29
  - AC-TASKS-RUNTIME-CLEANUP-001.30
  - AC-TASKS-RUNTIME-CLEANUP-001.32
  - AC-TASKS-RUNTIME-CLEANUP-001.34
  - AC-TASKS-RUNTIME-CLEANUP-001.36
  - AC-TASKS-RUNTIME-CLEANUP-001.37
  - AC-TASKS-RUNTIME-CLEANUP-001.39
  - AC-TASKS-RUNTIME-CLEANUP-001.40
  - AC-TASKS-RUNTIME-CLEANUP-001.41
  - AC-TASKS-RUNTIME-CLEANUP-001.44
  - AC-TASKS-RUNTIME-CLEANUP-001.46
  - AC-TASKS-RUNTIME-CLEANUP-001.47
  - AC-TASKS-RUNTIME-CLEANUP-001.49
  - AC-TASKS-RUNTIME-CLEANUP-001.52
system_design:
  - ../../specs/tasks/system-design/runtime-cleanup.md
  - ../../specs/tasks/system-design/durable-archive-cascades.md
  - ../../specs/tasks/system-design/archive-cascade-execution-contracts.md
  - ../../specs/tasks/system-design/archive-cascade-boundary-contracts.md
  - ../../specs/tasks/system-design/archive-cleanup-evidence.md
  - ../../specs/tasks/system-design/external-id-idempotency.md
  - ../../specs/tasks/system-design/task-creation-protocol.md
  - ../../specs/tasks/system-design/workspace-config-fence.md
  - ../../specs/tasks/system-design/runtime-startup-registry.md
  - ../../specs/tasks/system-design/archive-cascade-registries.md
---

# Task 03: Resume Partial Archive and Unarchive Cascades

## Summary

Make archive/unarchive durable state machines, route scheduled auto-archive
through them, convert safe legacy cascades, and deliver committed events.

## In scope

- Implement every phase/lease CAS: direct retry, archive-to-unarchive reversal,
  repeated result, task delete, and workspace delete from every active/terminal
  phase. Workspace deletion fences claims, supersedes task/group jobs, records
  confirmed-missing members, completes with `workspace_deleted`, and releases.
- Refactor composition to bind bootstrap liveness, then Start/sweep Runtime before
  every poller/producer/real route. Transient retry keeps liveness up/readiness
  false; fatal/cancel closes listeners. Register one Close across shutdown,
  restore, and composite factory-reset/delivery DB quiescence.
- Unarchive joins task/group restoration and exposes tasks only after groups are
  exact and metadata safely materializable. Recreate missing groups only from the
  full versioned canonical authenticated snapshot.
- Startup reconciliation deletes only a confirmed-dead local runtime row;
  alive/unknown/remote/generic-error rows persist with bounded diagnostics.
  A lost repair CAS observes the newer generation and exits without warning.
- Unarchive does not perform physical worktree I/O. It restores metadata and
  lifecycle ownership; the first resumed session invokes admitted materialization
  to recreate/reactivate a proven task-owned worktree before becoming runnable.
- Apply authenticated legacy proof: only live task, committed member, or retained
  key HMAC over the full cleanup envelope proves identity. UUID/hash-only or
  unbound rows corroborate but remain blocked. Reconcile local/Git evidence with
  trusted registration, filesystem/Git identity, and authenticated marker rules.
- Implement candidate, resolution-audit, separate pre-auth security-audit, and
  `0600` fsynced emergency fallback. Implement exact scoped loopback/CLI DTO,
  authorization, pagination, replay, redaction, status, and exit contracts.
- Use concrete scheduled, recovery, cleanup, event-dispatch, task-maintenance,
  operator, and installation actors; reload all applicable bindings.
- Advance migration watermarks only across explicit absent gap ranges. Dispatch
  existing real events first and preserve first post-epoch `task.created`.
- Runtime leases expired creation steps/operations, reconciles `unknown`, retries
  deterministic idempotency keys, completes compensation, resumes/Aborts eligible
  preparing work, and only Completes completing. Workspace delete transfers/
  aborts preparing or completes-then-deletes completing under shared locks.
- Aggregate outbox publishes completed creation/release/lifecycle/deletion/
  workspace events. Trusted adapters build canonical deletion provenance; all
  optional/direct/repository/reasonless paths are removed.
- Refactor production into the one bootstrap seam: construct without starts,
  bind, initial-agent/agentctl prerequisites, Runtime Start, then every other
  goroutine/subscription/producer/gateway/route/readiness. Register and reverse-
  close each resource at acquisition.
- Implement the executable B/Q/R/P/U catalog and sole runner in
  `apps/backend/internal/backendapp/startup_registry.go` as the sole Task 03
  owner. Own compensation/reconciliation registry entries as well. Split hidden
  starts from constructors/routes, make subscriptions return cleanup, and fail/
  unwind configured entry errors.
- Own runtime recovery/reconciliation/compensation, the workspace-delete
  aftermath worker, and the exact public archive-cascade CLI documentation.
  Implement `MaterializeTaskForResume` and its retained
  `task_resume_materialization_claims` protocol; physical worktree I/O follows
  the committed claim and only a succeeded claim admits a runnable session.

## Acceptance

- Every phase/lease/repeat/task-delete/workspace-delete edge is deterministic;
  stale generations fail and workspace deletion releases every barrier.
- Startup stale-row tests distinguish dead-local, alive, unknown, remote, and
  generic error; a newer-generation CAS winner is preserved without warning.
- Unarchive restores metadata safely without physical worktree I/O. The first
  resumed session's `MaterializeTaskForResume` claims one shared
  `task_resume_materialization_claims` row per task/lifecycle/environment/resource
  identity; concurrent sessions CAS their own admissions after the claim
  succeeds. Deletion fences/transfers in-flight claims, and only a succeeded
  claim admits a recreated owned worktree as runnable.
- Creation-step dispatch/recovery, TaskSession resume, and maintenance actors
  reload their closed bindings and reject spoofed/stale fields without effects.
- Task/group cleanup never terminally wedges. Missing groups reconstruct only
  byte-equivalent complete rows; incomplete/conflicting proof blocks.
- Each concrete actor reloads its complete binding; spoofed/foreign/stale actors
  have no side effects.
- Every legacy evidence combination, HMAC envelope tamper/key version, and
  local/Git path/branch/commit/marker mismatch produces the defined result.
- Workspace/installation authorization runs for zero candidates. Pagination and
  replay cannot cross actor, owner, scope, filter, limit, cursor, expiry, or
  changed ownership; audit failures preserve denial/rollback.
- Gap traversal never passes real events. Creation tests cover every step state,
  lease expiry to unknown, proof outcome, compensation transfer, handle/manifest
  mismatch, Complete ambiguity, Abort eligibility, and workspace delete against
  preparing/completing with exact event order.
- Launch intent coordination proves `planned -> armed` before normal
  Complete; `LaunchTask` admission requires completed creation, committed
  revision-one `task.created` evidence, non-deleted lifecycle, matching
  generation, and armed intent. Workspace-delete proves
  `armed -> cancelled/workspace_deleted` without an orphan runtime; dispatch
  claim replay is generation- and lease-fenced.
- Delayed task/session events and workflow snapshots prove immutable lifecycle
  epoch/revision/event ordering, tombstone suppression, and no resurrection;
  summary revisions and timestamps cannot override lifecycle freshness.
- Workspace-delete completion cancels any retained launch intent before live-row
  deletion; post-commit dispatch rejects deleted-task state and cannot create an
  orphan runtime. Runtime/environment cleanup reloads immutable resource
  snapshots, including authenticated Git-registration removal evidence.
- Task/Office rename, owner/membership/auth, or actor change after summary writes
  nothing. Incident blockers/comments only.
- External-ID release races all creation/delete states; same operation replay
  never inspects or clears a later holder.
- Workspace deletion emits each caller's canonical reason before the workspace
  event. Config Resolver/Fence/VerifiedRoot prevents stale-generation I/O.
- Workspace aftermath tests cover Phase A durable lower-tier claims, external
  I/O outside SQL, Phase B fenced completion, unknown outcomes, crash replay,
  and workspace-event dependency on completed task events.
- With optional schedulers disabled, liveness remains successful through initial
  setup, agentctl health, and transient Runtime retry. Readiness and all
  enumerated post-gate starters remain off; every fatal/downstream failure
  reverse-cleans, stop-once quiesces, and retries beyond attempt eight resume.

## Verification

```bash
cd apps/backend
go test ./internal/task/service ./internal/office/service -run '^(TestArchiveTaskTree_Partial|TestArchiveUnarchivePhaseRace_|TestStartupStaleRuntimeRow_|TestDeadRowRepairCASNewerGeneration|TestUnarchiveRecreatesOwnedWorktree|TestCreationStepRecovery_|TestExternalIDReleaseReplayAfterReuse_|TestExternalIDFoundNoSideEffectsMatrix|TestCanonicalDeletionProvenance_|TestWorkspaceConfirmActorRace_|TestWorkspaceDeleteActiveCascade_|TestWorkspaceDeleteUnsettledCreation_|TestUnarchiveTaskTree_|TestArchiveRuntime_|TestArchiveEventOutbox_)' -count=1
go test ./internal/task/archivecascade ./internal/backendapp -run '^Test(ArchiveActorBindings|ArchiveEventOutbox|ArchiveOutboxDependencies|ArchivePhaseCAS|BootstrapStarterInventory|CreationRecovery|CreationStepRecovery|ExternalIDCollation|ExternalIDFoundNoSideEffectsMatrix|ExternalIDLookupPreflight|ExternalIDPrecedence|ExternalIDPreflight|ExternalIDRelease|ExternalIDReleaseReplayAfterReuse|ExternalIDScenarioMatrix|HistoricalGaps|LegacyConversionProof|LegacyHMAC|LegacyHMACProofMatrix|MissingGroupSnapshot|RevisionHistoricalGaps|RuntimeRestartAfterExhaustion|ScheduledArchiveComposition|ArchiveTaskTree_Partial|UnarchiveRecreatesOwnedWorktree|UnknownCleanupReconciliation|WorkspaceDeleteActiveCascade)' -count=1
KANDEV_TEST_POSTGRES_DSN="$KANDEV_TEST_POSTGRES_DSN" go test ./internal/task/archivecascade -run '^TestPostgres.*(CreationStepRecovery|CreationCompletingRecovery|CreationRecovery|ExternalIDCollation|ExternalIDPreflight|ExternalIDFoundNoSideEffectsMatrix|ExternalIDKeyCollision|ExternalIDLookupPreflight|ExternalIDScenarioMatrix|ExternalIDPrecedence|ExternalIDReleaseReplayAfterReuse|LegacyHMACProofMatrix|ArchivePhaseCAS|WorkspaceDeleteActiveCascade|WorkspaceDeleteUnsettledCreation|ConfigReplacementAPI|BlockerCommentOwnership|HistoricalGaps|CanonicalDeletionProvenance|ArchiveOutboxDependencies|RuntimeRestartAfterExhaustion)' -count=1
```

Use table-driven failpoints at every durable phase boundary. Each test clears the failpoint, retries or starts reconciliation, and proves the final task, member, group, cleanup, and event state.

## Files likely touched

- `apps/backend/internal/task/archivecascade/`
- task service/reconciler/outbox lifecycle
- auto-archive coordinator wiring
- native launcher maintenance command and dedicated loopback route wiring
- `docs/public/cli.md` operator command section and contract assertion
- focused phase, actor, conversion/operator, outbox-order, and dialect tests
- `apps/backend/internal/backendapp/startup_registry.go`

## Dependencies

- `01-persist-cascade-operations`
- `02-harden-cleanup-identity`

## Risks

- Any finalizer missing phase plus generation CAS can reverse user intent.
- Candidate dismissal must remove all barriers and append audit atomically.
- Evidence categories must not expand implicitly.
- Direct publishing breaks immutable ordering.

## Parallelism

`sequential`

## Results

Pending.
