---
id: "02-guarded-snapshot-retry"
title: "Retry proven blocked snapshots"
status: done
wave: 2
depends_on:
  - "01-preserve-permissions"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-003
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-002
acceptance_criteria:
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-003.1
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-003.2
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-003.3
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-003.4
  - AC-TASKS-MANAGED-CLONE-RELOCATION-002.4
  - AC-TASKS-MANAGED-CLONE-RELOCATION-002.5
system_design:
  - ../../specs/tasks/system-design/worktree-metadata-recovery.md
  - ../../specs/tasks/system-design/managed-clone-relocation.md
---

# Task 02: Retry proven blocked snapshots

## Summary

Continue one historical permission-only blocked snapshot through a new explicit relocation request. Retain its failed snapshot and operation identity.

## In scope

- Write the four named `TestPermissionOnlyBlockedRelocation*` regressions with real Git, using the existing managed-clone store fixture.
- Seed matching materialized relocation and blocked snapshot records. Require empty manifest, exact historical reason, and proven permission-only differences.
- Allow provisional operation discovery only for an explicitly authorized dirty action. Do not mutate during candidate discovery.
- Under the same durable claim and both OS locks, reread complete identity, failure stamp, source and replacement state.
- Compare full entry sets, file bytes, link targets, permission loss shape, and stable source attributes through no-follow handles.
- Cover old-copier set-ID ownership loss, retained-bit owner mismatch, and ownership-only source/snapshot drift without changing the v1 manifest.
- Persist one `mode_retry` object and transition to the new deterministic snapshot before copying. Preserve the old snapshot unchanged.
- Keep generic blocked adoption refused. Resume the same replacement after interruption with the existing guarded publication.
- Cover content/link/type/membership drift, foreign or symlinked paths, conflicting IDs, live or borrowed runtime, competing claim, cancellation, changed HEAD, edited replacement, and repeated blocked retry.

## Out of scope

- Automatic retry during ordinary resume, restore, fresh start, or upgrade.
- A generic blocked-record reset or new recovery action.
- SQL schema changes and changes to claim expiry or original retention.

## Acceptance

1. A new explicit action succeeds for the exact legacy permission-only fixture. Its old snapshot, branch, provider identity, and operation ID survive.
2. All unsafe predicates leave records and original content unchanged, without publication or agent startup. Mixed inventories cannot hide a failed sibling.
3. Crash checkpoints before/after transition, after snapshot completion, and after publication resume or refuse under the same operation. A second blocked failure grants no new rebuild.

## Verification

Run this block from the repository root after Red, Green, and Refactor.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/worktree -run '^TestPermissionOnlyBlockedRelocation' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/worktree -run '^TestPermissionOnlyBlockedRelocation' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/worktree -run '^(TestPrepareRecoverySnapshotRejectsRequiredIdentityDriftAfterCopy|TestRebuildRecoverySnapshotRejectsRequiredIdentityDriftAfterEntryCopy|TestInterruptedPermissionRetryRejectsOriginalIdentityDriftAfterSnapshot)$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/worktree -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/worktree/recovery.go`
- `apps/backend/internal/worktree/recovery_admission.go`
- `apps/backend/internal/worktree/managed_clone_relocation.go`
- `apps/backend/internal/worktree/recovery_mode_retry.go (new)`
- `apps/backend/internal/worktree/managed_clone_permission_retry_test.go (new)`
- `apps/backend/internal/worktree/managed_clone_permission_identity_test.go (new)`
- `apps/backend/internal/worktree/recovery_identity_unix_test.go (new)`

## Dependencies

01-preserve-permissions.

## Risks

- The old error is a candidate hint, not authority.
- Do not release and reacquire the snapshot lock between proof and record transition.
- Unknown JSON fields permit old reads but do not make older writers implement the new retry contract.

## Parallelism

`sequential`

## Inputs

- Relocation design section "Permission-only blocked snapshot continuation" and proposed ADR.
- `managed_clone_relocation_recovery_test.go`: interrupted and published relocation fixtures.
- `recovery_admission.go`: operation identity merge and claim-aware admission.
- `workspaces.DirectoryHandle`: pinned traversal and no-follow file reads.

## Results

Implemented read-only retry candidate discovery for explicit dirty relocation,
complete-inventory preflight, pinned no-follow snapshot comparison, and one
same-operation transition to a fresh deterministic snapshot. The previous
snapshot and its error/update stamp remain in `mode_retry` provenance. Resume
uses the already-materialized replacement, and generic blocked adoption remains
refused. Historical evidence compares required UID/GID only when the corresponding
set-ID bit remains in the old snapshot. When that bit was lost, the old copied
owner neither authorizes nor vetoes source ownership; pinned checks take the
required identity from the current original and verify it stays stable. Fresh
snapshots and replacements must preserve that identity. The v1 manifest remains
unchanged, with separate identity checks at snapshot adoption and publication.

- `go test -trimpath -tags fts5 ./internal/worktree -run '^TestPermissionOnlyBlockedRelocation' -count=1 -v`: passed.
- `go test -trimpath -tags fts5 -race ./internal/worktree -run '^TestPermissionOnlyBlockedRelocation' -count=1`: passed.
- `go test -trimpath -tags fts5 ./internal/worktree -count=1`: passed.
- The named regressions pass for historical retry, provenance integrity,
  restart from snapshotting/rematerializing, unsafe source and snapshot drift,
  setuid owner drift, replacement edits and HEAD changes, repeated retry,
  symlinked evidence, and mixed-inventory refusal.
- A genuine old-copier fixture with source UID/GID 65534 and backend-owned
  historical files/directories succeeds after the old set-ID bits are stripped;
  the old snapshot remains unchanged, and the fresh snapshot and replacement
  retain the original UID/GID. Retained-bit owner mismatch remains a refusal.
- Rematerializing restart refuses UID-only setuid and GID-only setgid snapshot
  drift without canonical publication. Tests also refuse original UID/GID drift
  after copy, both at snapshot adoption and during snapshot creation, while the
  v1 bytes/mode manifest stays unchanged. An interrupted retry also refuses
  matching source and completed-snapshot identity drift against its stored
  pinned proof. Successful restart verifies IDs.
- `go test -trimpath -tags fts5 ./internal/worktree -run '^(TestPrepareRecoverySnapshotRejectsRequiredIdentityDriftAfterCopy|TestRebuildRecoverySnapshotRejectsRequiredIdentityDriftAfterEntryCopy|TestInterruptedPermissionRetryRejectsOriginalIdentityDriftAfterSnapshot)$' -count=1 -v`: passed.
- All four review-specific historical, restart, adoption, and copy-boundary identity regressions passed under `go test -trimpath -race`.
- `git diff --check`: passed.
