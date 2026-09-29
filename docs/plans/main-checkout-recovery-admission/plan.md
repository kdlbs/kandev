---
created: 2026-09-29
status: done
requirements:
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-001
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-002
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-003
system_design:
  - ../../specs/tasks/system-design/worktree-metadata-recovery.md
legacy_specs: []
---

# Implementation plan: Main-checkout recovery admission

## Overview

Fix [issue #4049](https://github.com/kdlbs/kandev/issues/4049) without modifying
healthy checkout contents. First correct admission and reuse together. Then prove
launch integration, retained-error retirement, and executor compatibility.

The task system owns this repair because it owns selected-environment admission
and canonical checkout lifetime. The existing preservation criteria apply.
Criteria 002.8 and 002.9 make the previously missing main-checkout case explicit.
No new ownership rule, persistence schema, or architectural decision is required.

## Evidence and root cause

Investigation used commit `cb9a530004f7d624629c9d9dd6cebdff4a32e7de`.
The issue and its follow-up comment contain no image attachments.

`inspectLinkedWorktree` requires a regular `.git` file. A healthy main repository
has a `.git` directory, so it receives the `ambiguous` classification.
Both `admitPersistedWorktreeRecovery` and `inspectRecoverySlot` reject that result.
`WorktreeRecoveryError` supplies the misleading corruption message.

There are two later blockers. `tryReuseExisting` calls the strict `Manager.IsValid`.
`openReusableWorktreePath` calls `DirectoryHandle.IsValidWorktree`, which reads
`.git` as a pointer file on both Unix and Windows. Changing only admission is insufficient.

A temporary `TestIssue4049HealthyMainCheckout` used a real initialized repository,
a persisted active `Worktree`, and the existing mock store. Git resolved `.git`
and the correct top-level. These subtests all failed with the reported pointer error:

- `legacy_admission`: `Manager.AdmitTaskRecovery`.
- `selected_admission`: `Manager.AdmitRecovery` with the Worktree executor.
- `reuse`: `tryReuseExisting` with the persisted worktree ID.

Command: `(cd apps/backend && go test ./internal/worktree -run '^TestIssue4049HealthyMainCheckout$' -count=1 -v)`.
Result: three expected regression failures. The temporary file was removed.
No live instance or user checkout was modified. The macOS report was not reproduced
in a browser. The Linux reproduction proves the shared backend cause.

## Scope

### In scope

- Validated main-checkout acceptance in selected and legacy admission.
- Additional-session and ordinary reuse without checkout mutation.
- Real-Git safety tests, mixed inventories, and launch-path integration.
- Successful ordinary relaunch and matching stale-error retirement.
- A short clarification in the public Git recovery guide after implementation.

### Out of scope

- A new general-purpose task-error dismissal control or API.
- Changing `noRetry` for genuinely unsafe recovery errors.
- Cleanup, clone relocation, or snapshotting of main repositories.
- Remote filesystem inspection, migrations, feature flags, or provider changes.
- Automatic data repair on the reporter's installation.

## Technical approach

Add a read-only checkout classifier in `internal/worktree/checkout_inspection.go`.
Keep `inspectLinkedWorktree` unchanged for linked metadata. Validate a main checkout
with bounded, explicit Git metadata inspection and no-follow identity checks.
Reject an empty `.git`, symlink metadata, bare repositories, parent discovery,
metadata redirection, and a mismatched working-tree root. Allow valid unborn branches.

Use the classifier in `manager.go`, `recovery_admission.go`, and the reuse-only
paths of `manager_lifecycle.go`. Propagate context for bounded inspection.
Keep `Manager.IsValid` and the storage handle's linked-worktree predicate strict.
A healthy main result skips repair and linked-worktree relocation. Preserve owner,
canonical slot, and pinned path validation. Do not infer cleanup authority from health.

All selected slots must pass. Main-plus-invalid inventories refuse before any
replacement. Main-plus-recoverable-linked inventories preserve the main checkout
while existing recovery policy controls the damaged linked slot.

| Executor or checkout | Intended behavior | Evidence |
| --- | --- | --- |
| Worktree, healthy main checkout | Admit and reuse without repair | Real-Git manager and launch integration tests |
| Worktree, healthy linked checkout | Preserve pointer and backlink validation | Existing recovery and reuse suites |
| Worktree, missing linked admin | Existing guarded recovery only | Existing recovery suites plus mixed inventory test |
| Local | Return before host recovery inspection | Excluded-executor regression |
| Docker, SSH, Sprites, Kubernetes, mock remote | Preserve provider ownership | Excluded-executor regression |
| Malformed, symlinked, redirected, or unreadable metadata | Refuse without mutation | Negative classifier and ownership tests |

The comment's banner and workspace-panel symptoms are reported impact. They are
not evidence of an independent frontend defect. Normal launch is the recovery
route for this misclassification after the repair. Existing successful launch
calls `clearTaskLaunchErrorIfStamp`; Task 02 proves its behavior for a retained error.
If ordinary launch cannot be reached with that retained error, report that specific
blocker before introducing another recovery contract.

## Tests

- `TestMainCheckoutAdmissionAndReuse` in `checkout_inspection_test.go`: criteria
  002.1, 002.8. Cover legacy admission, selected admission, ordinary reuse by ID
  and session, and `Create` with `ReuseRequired`.
- `TestMainCheckoutRejectsInvalidMetadata`: 002.3, 002.9. Include no metadata,
  empty metadata, symlinks, bare metadata, redirection, and ambient Git overrides.
- `TestMainCheckoutMixedInventoryRefusesBeforeRecovery`: 002.6, 002.7. Preserve the healthy main slot
  with a damaged linked slot. Refuse all replacement for an ambiguous sibling.
- `TestMainCheckoutLaunchIntegration` in the executor integration file: 001.2,
  001.3, 002.8, 003.5. Use real manager admission, not an always-success callback.
- `TestMainCheckoutRelaunchRetiresMatchingError` in the orchestrator: 002.8.
  Prove matching-error retirement and preservation of a successor error.

## End-to-end evidence

Task 02 must connect public executor launch/resume entry points to a real Git
checkout and manager admission. Cover an added session and workspace restoration
with the same canonical path. A recording runtime can replace external agent
startup. Assert the existing primary session remains intact and dirty content
survives. This backend integration is the end-to-end evidence for this package.

There is no rendered UI change. No ASCII layout preview or mobile UI work is
required. Desktop and phone use the same repaired launch contract.

## Work orders

- [x] [Task 01: Admit and reuse healthy main checkouts](task-01-main-checkout-validation.md)
- [x] [Task 02: Prove launch continuity and error retirement](task-02-launch-continuity.md)

## Verification results

Implementation and verification completed on 2026-09-29. The temporary
reproduction and the first permanent regression both failed before the fix for
the expected linked-pointer rejection.

- `(cd apps/backend && go test ./internal/worktree ./internal/system/storage/workspaces -count=1)`: passed.
- `(cd apps/backend && go test ./internal/orchestrator/executor -count=1)`: passed.
- `(cd apps/backend && go test ./internal/orchestrator -run 'TestMainCheckout|TestClearTaskLaunch|TestTaskLaunch' -count=1 -v)`: passed.
- `(cd apps/backend && go test ./internal/agent/runtime/lifecycle -count=1)`: passed on rerun after a transient temp-directory cleanup failure in an existing test.
- `(cd apps/backend && go build ./...)`: passed.
- `node scripts/validate-public-docs.mjs`: passed, 47 published pages.
- `python3 scripts/list-docs.py validate`: passed, 327 decisions and 1243 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- Native macOS and Windows path behavior was not tested in this Linux environment.

The scoped production, test, specification, plan, and public-documentation
changes are ready for review. The temporary reproduction was removed.

The earlier [metadata recovery package](../worktree-metadata-recovery/plan.md)
retains its historical implementation status and outstanding integration evidence.
This package covers the main-checkout regression only. It does not mark those
older work orders complete or claim their PostgreSQL checks passed.

## Risks

- A `.git` directory does not prove a valid repository. Git can discover a parent
  or obey environment overrides unless the validation pins its inputs.
- Broadening shared health predicates can authorize recovery or archive behavior
  outside this repair. Keep main acceptance local to admission and reuse.
- Healthy main checkouts can be busy. Read-only validation must not acquire a
  replacement claim or interfere with an existing primary session.
- Native macOS and Windows path behavior remains untested. Linux evidence alone
  does not prove native no-follow behavior on those systems.
