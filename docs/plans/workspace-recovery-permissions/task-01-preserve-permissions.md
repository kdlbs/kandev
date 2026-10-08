---
id: "01-preserve-permissions"
title: "Preserve snapshot permissions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-002
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-002
acceptance_criteria:
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-002.2
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-002.3
  - AC-TASKS-MANAGED-CLONE-RELOCATION-002.2
system_design:
  - ../../specs/tasks/system-design/worktree-metadata-recovery.md
  - ../../specs/tasks/system-design/managed-clone-relocation.md
---

# Task 01: Preserve snapshot permissions

## Summary

Correct snapshot and restore attributes without weakening the integrity check. Preserve the owner identity required by set-ID bits.

## In scope

- Write failing mode regressions in new `recovery_permissions_test.go` and Unix-specific test files.
- Cover 0775 regular files under umask 0022, setuid files, setgid files/directories, sticky directories, and read-only parents.
- Use a child process for umask mutation. Preserve complete file modes after writing and directory modes after children.
- Preserve required UID/GID through pinned attribute operations before applying set-ID bits. Include an ownership change allowed case and a denied case.
- Verify source, snapshot, and restored modes and required identities. Keep no-follow links and unchanged full-mode manifests.
- Exercise real-Git metadata recovery and explicit dirty relocation with an ignored miniature rootfs. Never execute its setuid file.

## Out of scope

- Blocked-record adoption and compatibility retry.
- General ownership, ACL, timestamp, and xattr preservation.
- Live task data and rendered UI changes.

## Acceptance

1. The named snapshot, restore, and umask tests fail before the fix and pass afterward with exact mode equality.
2. Set-ID tests preserve their required UID/GID or refuse before publication. A source-owned executable never gains the backend UID.
3. Existing snapshot, manifest, symlink, metadata, and relocation package tests pass. Platform-specific skips remain explicit.

## Verification

Run this block from the repository root after Red, Green, and Refactor.

```bash
(cd apps/backend && go test -trimpath ./internal/worktree -run '^TestRecovery(Snapshot|Restore|SetID)' -count=1 -v)
(cd apps/backend && go test -trimpath ./internal/worktree -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/worktree/recovery_files.go`
- `apps/backend/internal/worktree/recovery_identity.go (new)`
- `apps/backend/internal/worktree/recovery_permissions_test.go (new)`
- `apps/backend/internal/worktree/recovery_permissions_unix_test.go (new)`
- `apps/backend/internal/worktree/recovery_permissions_unix.go (new, if required)`
- `apps/backend/internal/worktree/recovery_permissions_windows.go (new, if required)`

## Dependencies

None.

## Risks

- Chown can clear special bits. Apply it before final chmod and verify afterward.
- Process-global umask tests can contaminate parallel packages. Use a child process.
- If new pinned attribute capability is required, keep it local to recovery and preserve the existing storage-handle contract.

## Parallelism

`sequential`

## Inputs

- Existing preservation criteria and the metadata design section "Snapshot permission preservation".
- `recovery_review_test.go`: read-only directory and manifest validation examples.
- `manager_managed_clone_relocation_test.go`: explicit dirty relocation fixture.
- Proposed ADR and permission evidence in `plan.md`.

## Results

Implemented descriptor-based permission and required set-ID identity preservation.
New snapshots keep their root private, copy file content before applying mode,
sync file content before applying privileged set-ID attributes, and apply
directory attributes after their children. Source identity is checked before
and after copying, and ownership or mode readback failures stop recovery.
The v1 manifest remains unchanged; pinned identity checks separately compare the
authoritative original with completed snapshots and replacements at adoption
and before canonical publication.

- Targeted permission tests: passed, including the umask child-process case,
  set-ID UID/GID and sticky mode preservation, and an injected ownership denial.
- `go test -trimpath -tags fts5 ./internal/worktree -run '^TestSyncRecoveryContentBeforeAttributes' -count=1 -v`: passed.
- Full worktree package: `go test -trimpath -tags fts5 ./internal/worktree -count=1` passed.
- `git diff --check`: passed.
- Windows cross-build was attempted. It stops in the existing SQLite repository
  package because `sqlite3.Error` and `sqlite3.ErrConstraintUnique` are undefined
  for that target. Native Windows execution is not claimed.
