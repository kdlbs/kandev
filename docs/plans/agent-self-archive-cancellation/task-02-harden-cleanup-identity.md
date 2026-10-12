---
id: "02-harden-cleanup-identity"
title: "Harden cleanup identity and cancellation"
status: pending
wave: 2
depends_on:
  - "01-persist-cascade-operations"
plan: "plan.md"
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
  - REQ-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001
acceptance_criteria:
  - AC-TASKS-RUNTIME-CLEANUP-001.10
  - AC-TASKS-RUNTIME-CLEANUP-001.42
  - AC-TASKS-RUNTIME-CLEANUP-001.43
  - AC-TASKS-RUNTIME-CLEANUP-001.45
  - AC-TASKS-RUNTIME-CLEANUP-001.50
system_design:
  - ../../specs/tasks/system-design/runtime-cleanup.md
  - ../../specs/tasks/system-design/durable-archive-cascades.md
  - ../../specs/tasks/system-design/archive-cascade-execution-contracts.md
  - ../../specs/tasks/system-design/archive-cascade-boundary-contracts.md
  - ../../specs/tasks/system-design/archive-cleanup-evidence.md
  - ../../specs/tasks/system-design/archive-cascade-registries.md
  - ../../specs/tasks/system-design/workspace-config-fence.md
  - ../../specs/tasks/system-design/session-delete-resource-cleanup.md
  - ../../specs/tasks/system-design/environment-owned-git-status.md
---

# Task 02: Harden Cleanup Identity and Cancellation

## Summary

Fix self-archive cancellation and bind every task/group cleanup transition and
destructive stage to the durable fenced operation.

## In scope

- Compute one `TaskArchiveTimeout` absolute deadline. After durable preparation,
  detach cancellation but cap every rollback, mutation recovery, finalization,
  vacancy, activation, and nested compensation context at
  `min(original_deadline, now+step_budget)`. On expiry persist wakeup and stop.
- Make archive/delete stop every `executors_running` owner before destructive
  cleanup. Session deletion removes references only. Shared or borrowed
  environments/worktrees survive until no active holder. Agent/agentctl grace
  expiry kills and joins the complete process group before shutdown succeeds.
- Extend task cleanup and add group jobs with complete versioned group/member
  snapshots, direction/generation, retry policy, leases, markers, HMAC key
  identity, and unknown evidence. Implement every cleanup/restore state.
- Use `cascade_retry`: after normal backoff retain exhausted diagnostics and retry
  every 12 hours. Only non-restorable delete/shutdown uses bounded terminal
  failure.
- Change cleaner/restorer interfaces to `CleanupFence`. Implement byte-exact DB
  namespace, canonical UUID resource keys, SHA-256 path keys, bounds, aliases,
  and exact marker/operation/member/generation/snapshot binding.
- Issue and verify versioned HMAC-SHA256 evidence over the full boundary envelope;
  retain verification keys through rows/markers/outboxes and reject UUID/hash-
  only or tampered legacy proof.
- For local/worktree proof bind registration, filesystem object, canonical
  containment, Git common/worktree dirs, branch, HEAD, and base; protect external
  markers under runtime data with authenticated content. Use no-follow handles
  and identity rechecks. Keep provider-specific Docker/Kubernetes/SSH/Sprites
  proof.
- Treat a missing captured DB row as metadata-idempotent, but never infer
  physical absence. A missing worktree/path is successful only with matching
  authenticated marker and locked Git registration; generic not-found is
  retained `unknown` and retried.
- Implement `internal/workspaceconfig` resolver, unconstructable fence, access-
  mode/state checks, VerifiedConfigRoot guard/recheck, marker verification,
  generation cache invalidation, delete/quarantine transitions, and path-event
  resolution. Own `apps/backend/internal/workspaceconfig/registry.go` config
  entries. Replace every exact loader/writer/memory/Git/config-sync/default/
  settings method in the boundary table; remove name/raw-path APIs.
- Commit claim before sorted guards; use short marker/completion transactions
  around I/O, release reverse, and never wait on a guard inside SQL.
- Wire cleanup due work to mandatory Runtime startup sweep/wakeup; optional
  auto-archive scheduling must be disabled in recovery tests.

## Acceptance

- Force expiry independently inside rollback, recovery, finalization, vacancy,
  activation, and nested compensation; no request-owned context exceeds the
  original deadline, and later work requires a Runtime claim.
- Archive/delete stops all owned runtime rows before cleanup; session deletion
  preserves task resources; shared/borrowed resources defer; process-group
  timeout kills and joins descendants. Tests cover each invariant independently.
- Every task/group cleanup and restore state, unknown proof, takeover, and stale
  claim is generation/lease/marker CAS-tested. Cascade jobs resume after restart
  beyond eight failures; only bounded-policy delete/shutdown retains `failed`.
- Full group snapshots reproduce byte-equivalent rows/membership order; missing,
  extra, reordered, version-conflicting, or digest-conflicting proof blocks.
- Key rotation verifies retained old-key HMAC but new rows use only the active
  key; every envelope-field tamper and UUID/hash-only legacy row blocks.
- Local proof rejects foreign registration, path alias/escape, symlink swap,
  inode/device replacement, wrong Git directory/branch/HEAD/base, and forged or
  unprotected external markers. Other providers require exact ownership proof.
- Missing captured metadata rows are idempotent; missing physical paths without
  marker/registration proof remain unknown. With exact proof they succeed.
- Every method in replacement table acquires and reloads the correct access mode;
  deleting/blocked/released/generation/marker/path-event failures perform no I/O.
  Cache invalidation, same-name create/rename, quarantine handoff, and legacy
  ambiguity are deterministic; no exported name/raw-path entry remains.
- Pause after claim/before guard, take over or reverse, and prove serialized I/O
  plus stale rejection after acquisition. No SQL transaction waits on a guard;
  cancellation releases prefixes and crash closes OS guards.
- Bounded inputs reject before allocation or durable write at
  `max_cascade_members=10000`, `max_cascade_depth=256`,
  `max_group_snapshot_bytes=4194304`, `max_evidence_envelope_bytes=262144`,
  and `max_retained_payload_bytes=524288`; inclusive boundaries pass and excess
  cases return `archive_size_exceeded` with zero side effects.

## Verification

```bash
go test ./internal/task/service ./internal/agent/runtime/... -run '^(TestArchiveTaskTree_(SelfArchive|AbsoluteDeadlineAllSteps|PreparationRollback|ExpiredWakeup)|TestArchiveDeleteStopsAllRuntimes|TestDeleteSessionPreservesTaskResources|TestSharedEnvironmentDefersCleanup|TestAgentctlProcessGroupTermination|TestPhysicalAbsenceProof|TestCleanupPhysicalFence_|TestArchiveCleanupWorker_)' -count=1
go test ./internal/task/archivecascade ./internal/task/repository/sqlite ./internal/office/repository/sqlite ./internal/office/configloader ./internal/office/config ./internal/backendapp -run '^Test.*(CleanupStateTable|RestoreStateTable|CascadeRetryPolicy|RestartAfterExhaustion|GroupSnapshotRoundTrip|LegacyHMAC|CleanupEvidenceEnvelope|KeyRotation|CleanupUnknown|CleanupTakeover|CleanupResourceKey|CleanupMarkerState|GitProviderProof|PhysicalAbsenceProof|WorkspaceConfigResolver|ConfigReplacementAPI|ConfigPathEvent|ConfigNameReuse|ConfigDefaultAdapter|TOCTOU|PhysicalLifecycleFence|CascadeSizeBounds|CascadeDepthBounds|EvidenceSizeBounds|RetainedPayloadBounds)' -count=1
```

Blocking fakes return only on their received context or explicit release and have an independent watchdog. Race tests pause between read and transition to force activation or claim to win.

## Files likely touched

- `apps/backend/internal/task/service/handoff_cleaner.go`
- task/group cleanup services and interfaces
- `apps/backend/internal/task/archivecascade/`
- task and Office SQL repositories
- focused lease/takeover/resource-guard tests

## Dependencies

- `01-persist-cascade-operations`

## Risks

- Claim generation without the held physical guard leaves a last-check race.
- Guard acquisition must be cancellation-aware and always released.
- Detached work must retain the absolute deadline.

## Parallelism

`sequential`

## Results

Pending.
