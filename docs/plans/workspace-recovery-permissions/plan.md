---
created: 2026-10-04
status: complete
requirements:
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-002
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-003
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-002
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-003
system_design:
  - ../../specs/tasks/system-design/worktree-metadata-recovery.md
  - ../../specs/tasks/system-design/managed-clone-relocation.md
legacy_specs: []
---

# Implementation plan: Workspace recovery permissions

## Overview

Preserve snapshot permissions, then let an explicit relocation request continue
one proven historical permission-only failure. Finally prove resume continuity
and error retirement through the backend entry points.
The task system owns the canonical environment inventory and recovery authority.
The package leaves the user's live task unchanged.

Existing preservation criteria define the copy correction. New criterion
AC-TASKS-MANAGED-CLONE-RELOCATION-002.5 defines the narrow compatibility retry.
The [proposed decision](../../decisions/2026-10-04-permission-only-snapshot-retry.md)
records that boundary. Implementation is tracked in the sequential work orders below.

## Confirmed evidence

Investigation used `5a51992f17325ee42fecb615a2f1c328871e9844`.
The live task's managed source clone changed. Ignored build outputs qualified
its Kandev checkout as dirty despite a clean normal Git status.
Its October 3 recovery record stored `blocked`, an empty manifest, and
`recovery snapshot does not match original checkout`. The relocation record
still stored `materialized` and commit `253a8dbd484e027df86bf3fcbf8c9f745e1dfd26`.
The explicit action now fails in `recoveryOperationIDsForSlot` before repair.

The original and snapshot contained 154,342 matching filesystem entries.
Comparison found 23 permission differences and no content differences across
135,038 regular files (4,627,251,862 bytes).
Browser-demo rootfs entries included a setuid executable, setgid directories,
sticky directories, and a group-writable regular file.
The setuid file belonged to UID 0. The setgid directories belonged to GID 1000.
These identities also need preservation when their special bits transfer.

A temporary standalone program copied the repository's `recovery_files.go`.
It reproduced manifest inequality for all four permission classes under umask
0022. The investigation removed that program and its temporary files.
The package artifacts were initially validated before implementation.

## Scope

### In scope

- The shared snapshot and restore mode policy for selected host Worktree recovery.
- Set-ID identity preservation and refusal when the host cannot preserve it.
- One guarded compatibility retry through existing explicit dirty relocation.
- Retained snapshot evidence, same-operation restart, and complete-inventory gates.
- Real-Git regression tests and backend resume/error integration.
- Public Git recovery guidance after the implementation passes.

### Out of scope

- Automatic dirty relocation or automatic repair on upgrade.
- Generic unblocking of failed recoveries, lost branches, or metadata-only blocked operations.
- Discarding ignored files or weakening the manifest to ignore permissions.
- Deleting the original or old snapshot, claim expiry, or cleanup policy changes.
- New UI layout, controls, actions, translations, runtime flags, or database migrations.
- General preservation of ACLs, xattrs, hard links, timestamps, or ordinary ownership.
- Mutating the user's live task, source repository, or recovery records.

## Technical approach

### Shared file preservation

Correct `snapshotCheckoutEntry`, `snapshotRegularFile`, `copySnapshotFile`, and
directory mode capture in `apps/backend/internal/worktree/recovery_files.go`.
Apply supported permission and special bits after content writes.
Apply final directory attributes after their children.
Keep the root private and the root `.git` excluded.
Use host-specific helpers for required set-ID UID/GID preservation and readback.
Use pinned entries and descriptor operations. Never follow a snapshot symlink.
Preserve the existing manifest format and full mode comparison.

### Explicit retry and persistence

Keep read-only candidate recognition in `recovery_admission.go` separate from
mutation authority. Only explicit dirty relocation can nominate the old operation.
Under the durable claim and both file locks, a focused helper compares the old
snapshot with the unchanged source and proves the exact permission-loss shape.
Use `workspaces.DirectoryHandle` traversal and `OpenFile` for no-follow reads.
Read owner identity from pinned file/directory metadata, not lexical path lookup.
Keep cancellation and memory bounded. Recheck the complete selected inventory.

Extend `recoveryRecord` in `recovery.go` with optional `mode_retry` provenance.
Transition once to a distinct snapshot path under the same operation ID.
Never delete the historical failed snapshot. Record the transition atomically
before creating the new path. Reuse the existing deterministic replacement.
Keep generic `adoptRecoveryRecord` blocked refusal intact.
The specialized transfer path in `managed_clone_relocation.go` performs proof,
transition, fresh snapshot, restore, and guarded publication under authority.

The authoritative state transitions and rejection predicates are in the
[relocation design](../../specs/tasks/system-design/managed-clone-relocation.md#permission-only-blocked-snapshot-continuation).
They include current error stamps, owner generation, branch/HEAD, replacement
cleanliness, all selected slots, and same-operation restart.
No new SQL write capability is planned.

### Compatibility matrix

| Path | Intended result | Evidence or fallback |
| --- | --- | --- |
| Host Worktree metadata recovery | Preserve supported modes in new snapshots | Snapshot/restore tests and real-Git recovery |
| GitHub/GitLab explicit dirty relocation | Same guarded mode policy and narrow retry | Provider-parameterized real-Git relocation |
| Ordinary resume, restore, or fresh start | No blocked-mode retry authority | Negative admission and service tests |
| Unix set-ID/sticky entries | Preserve supported bits and required identity | Unix mode and ownership tests |
| Ownership change denied or mode unsupported | Refuse before publication | Forced failure tests and retained original |
| Windows ordinary permissions | Retain existing host permission contract | Portable cases, no Unix-bit assumptions |
| Local, Docker, SSH, Sprites, Kubernetes | Preserve executor-owned workspace contract | Existing excluded-executor tests |
| Another blocked reason, content drift, or unsafe record | Remain blocked without record changes | Adversarial retry cases |

Native Windows and macOS execution is not implied by Linux verification.
Tests must report platform skips and unsupported preservation explicitly.

## Tests and traceability

| Criteria | Planned test and file |
| --- | --- |
| WORKTREE-METADATA-RECOVERY-002.2, .002.3; MANAGED-CLONE-RELOCATION-002.2 | `TestRecoverySnapshotPreservesSupportedModes`, `TestRecoveryRestorePreservesSupportedModes`, `TestRecoverySnapshotIgnoresProcessUmask`, `TestRecoverySetIDPreservesRequiredIdentity`, `TestRecoverySetIDOwnershipFailureRefusesPublication` in new `recovery_permissions*_test.go` files |
| WORKTREE-METADATA-RECOVERY-003.1 through .003.4; MANAGED-CLONE-RELOCATION-002.4, .002.5 | `TestPermissionOnlyBlockedRelocationRetry`, `TestPermissionOnlyBlockedRelocationRejectsUnsafeRetry`, `TestPermissionOnlyBlockedRelocationRestart`, `TestPermissionOnlyBlockedRelocationMixedInventory` in new `managed_clone_permission_retry_test.go` |
| MANAGED-CLONE-RELOCATION-002.2 through .002.5, .003.1 | `TestPermissionRecoveryResumeIntegration` in new executor integration file and `TestRecoverSessionPermissionRetryRetiresMatchingError` in new orchestrator file |

Use complete `AC-TASKS-*` IDs in the work orders.
All tests begin with failing regressions before production edits.
Existing read-only directory, symlink, rematerializing manifest, claim, and
clean/dirty relocation tests remain in their package checks.

## End-to-end evidence

Task 03 connects `RecoverSession` and `PreflightSessionWorktreeRecovery` to the
real manager, local Git clones, persisted environment inventory, and SQLite claim.
Only external provider startup is replaced with a recording runtime.
Seed the historical blocked record directly, without copying live user files.
Prove that a current explicit action resumes the same provider conversation.
Ordinary resume and stale stamps must fail without startup or record mutation.
Prove successful inventory publication and matching-error retirement.
Include an unaffected sibling and a failed sibling in separate multi-slot cases.

The package changes no rendered UI. Existing desktop and phone recovery views
use the repaired backend action. No new browser E2E or layout preview is required.
The companion recovery-surface package retains its existing browser evidence.

## Companion packages

- [Metadata recovery](../worktree-metadata-recovery/plan.md) retains its existing
  in-progress claim/integration records. This package does not mark them done.
- [Original relocation](../managed-clone-relocation/plan.md) retains completed clean/dirty and UI work.
- [Legacy source reuse](../legacy-clone-resume/plan.md) retains implemented admission behavior.
- [Recovery convergence](../managed-clone-recovery-convergence/plan.md) retains completed projection and surface work.

This repair extends the shared copier and explicit retry, without rewriting
historical verification counts or adding UI scenarios to those packages.

## Work orders

- [x] [Task 01: Preserve snapshot permissions](task-01-preserve-permissions.md)
- [x] [Task 02: Retry proven blocked snapshots](task-02-guarded-snapshot-retry.md)
- [x] [Task 03: Prove resume continuity](task-03-resume-continuity.md)

The dependency order is 01, then 02, then 03. Every work order is sequential.

## Verification results

Implementation verification on 2026-10-04 passed:

- `go test -trimpath -tags fts5 ./internal/worktree ./internal/system/storage/workspaces ./internal/orchestrator ./internal/orchestrator/executor -count=1`: passed after review fixes.
- `go test -trimpath -tags fts5 ./internal/orchestrator -run '^TestRecoverSessionPermissionRetryRetiresMatchingError$' -count=1 -v`: passed.
- `make -C apps/backend build`: passed after the final refactor.
- Targeted `golangci-lint` for worktree, workspace storage, orchestrator, and executor packages: passed with zero issues.
- Public documentation tests (62), public page validation (47 pages), specification catalog validation (349 decisions and 1,337 specifications), specification-linter tests (36), and full specification lint: passed.
- Planned-source documentation coverage, changed-Go formatting check, and `git diff --check`: passed.

Design-package validation on 2026-10-04 also passed:

- `python3 scripts/list-docs.py validate`: 349 decisions and 1,337 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check -- docs/specs docs/decisions docs/plans/workspace-recovery-permissions`: passed.
- Work-order requirement, acceptance-criterion, and design references: valid.
- Initial PR documentation coverage: documentation-only paths exempt, planned production paths covered by all three work orders.

Task 01, Task 02, and Task 03 changed production or permanent test files. Their
results are recorded in the work orders. Review follow-up distinguishes historical
copied ownership from current source identity when a set-ID bit was lost. Separate
pinned checks now cover original, completed snapshot, and replacement identity at
snapshot adoption and immediately before canonical publication, without changing
the v1 manifest. Regression tests cover old-copier UID/GID loss, retained-bit
ownership mismatch, source UID/GID drift during and after copying, and snapshot
UID/GID drift on restart. Public Git recovery guidance describes this boundary.
The original live task remains unchanged.

### Documentation coverage preflight

Run this from the repository root. During implementation, replace the planned
path list with the final changed source paths when they differ.

```bash
node <<'JS'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const root = 'docs/plans/workspace-recovery-permissions';
const artifacts = fs.readdirSync(root).filter(n => n.endsWith('.md')).map(n => `${root}/${n}`);
const refs = [
  'docs/specs/tasks/requirements/worktree-metadata-recovery.md',
  'docs/specs/tasks/requirements/managed-clone-relocation.md',
  'docs/specs/tasks/system-design/worktree-metadata-recovery.md',
  'docs/specs/tasks/system-design/managed-clone-relocation.md',
];
const planned = [
  'apps/backend/internal/worktree/recovery_files.go',
  'apps/backend/internal/worktree/recovery.go',
  'apps/backend/internal/worktree/recovery_admission.go',
  'apps/backend/internal/worktree/managed_clone_relocation.go',
  'apps/backend/internal/worktree/recovery_mode_retry.go',
  'apps/backend/internal/worktree/recovery_permissions_unix.go',
  'apps/backend/internal/worktree/recovery_permissions_windows.go',
];
const fileContents = Object.fromEntries([...artifacts, ...refs].map(f => [f, fs.readFileSync(f, 'utf8')]));
const tracked = execFileSync('git', ['diff', '--name-only', '-z', 'HEAD'], { encoding: 'utf8' }).split('\0').filter(Boolean);
const untracked = execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' }).split('\0').filter(Boolean);
const changedFiles = [...new Set([...tracked, ...untracked, ...planned])].map(filename => ({ filename, status: 'modified', additions: 1, changes: 1 }));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
JS
```


## Risks

- Applying set-ID bits under a different owner changes their privilege semantics.
- Readback can refuse hosts that cannot preserve a required owner or group.
- A blocked reason string alone cannot prove a safe retry.
- A replacement can contain edits even when the record still says materialized.
- A restart can lose retained evidence unless provenance precedes new snapshot creation.
- Large build trees require streaming reads and cancellation through the repair request.
- Source content drift between failure and retry remains a manual-repair refusal.
- Old binaries do not implement this compatibility retry. Downgrade is not a repair path.
