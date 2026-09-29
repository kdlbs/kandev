---
id: "01-main-checkout-validation"
title: "Admit and reuse healthy main checkouts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-001
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-002
acceptance_criteria:
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-001.3
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-002.1
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-002.3
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-002.6
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-002.7
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-002.8
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-002.9
system_design:
  - ../../specs/tasks/system-design/worktree-metadata-recovery.md
---

# Task 01: Admit and reuse healthy main checkouts

## Summary

Validate main checkouts separately from linked-worktree metadata. Apply the result
to both admission paths and every reuse path without expanding mutation authority.

## In scope

- Add `checkout_inspection.go` and `checkout_inspection_test.go` in the worktree package.
- Keep the linked inspector strict. Add a distinct healthy-main classification
  and bounded, read-only validation of the working-tree and metadata roots.
- Use explicit Git inputs. Reject parent discovery, ambient Git overrides,
  symlink metadata, malformed directories, bare repositories, and path changes.
- Preserve `validateWorktreePathSafe`, `validateExistingWorktreePathOwner`, and
  handle verification before accepting a checkout. Propagate context to new checks.
- Adapt `openReusableWorktreePath` to accept a validated main checkout as a
  reuse-only alternative to the handle's linked-worktree predicate.
- Replace the additional strict `IsValid` checks in `tryReuseExisting` with the
  validated reuse result. Cover session/repository lookup and explicit worktree ID.
- Cover `reuseRequiredWorktree` through `Create` and preserve its no-mutation contract.
- Keep main checkouts outside snapshot recovery, relocation, and replacement paths.

## Out of scope

- Changing `Manager.IsValid`, the storage directory-handle interface, or cleanup authority.
- Changing non-Worktree executor behavior, schema, frontend controls, or error retry policy.
- Fetch, checkout, reset, contribution setup, or repository scripts during attach-only reuse.

## Acceptance

1. Write `TestMainCheckoutAdmissionAndReuse` first. It fails on the current code
   with the pointer-file rejection. It passes for initialized and unborn main
   repositories after the correction. It covers both admission methods, ordinary
   reuse by session and ID, and additional-session `Create` with `ReuseRequired`.
2. `TestMainCheckoutRejectsInvalidMetadata` and `TestMainCheckoutMixedInventoryRefusesBeforeRecovery`
   cover the plan's negative matrix. A healthy main slot never masks an invalid
   sibling. Proven missing linked metadata retains its existing guarded recovery.
3. Compare path, branch, HEAD, index bytes, tracked edits, staged edits, untracked
   files, and ignored files before and after successful admission and reuse.
   No main-checkout recovery record, claim, snapshot, sibling, or relocation is
   created. Existing linked integrity, ownership, and symlink regressions pass.

## Verification

From the repository root:

```bash
(cd apps/backend && go test ./internal/worktree -run 'TestMainCheckout' -count=1 -v)
(cd apps/backend && go test ./internal/worktree ./internal/system/storage/workspaces -count=1)
git diff --check
```

Run the first command before production changes and retain its expected failure.
Then run every command after implementation. Use permission-error injection where
root-run tests cannot create an unreadable path reliably. Do not count a skipped
permission test as evidence. Record native macOS/Windows coverage or its absence.

## Files likely touched

- `apps/backend/internal/worktree/checkout_inspection.go` (new)
- `apps/backend/internal/worktree/checkout_inspection_test.go` (new)
- `apps/backend/internal/worktree/manager.go`
- `apps/backend/internal/worktree/recovery_admission.go`
- `apps/backend/internal/worktree/manager_lifecycle.go`
- `apps/backend/internal/worktree/manager_reuse_required_test.go`

## Dependencies

None.

## Risks

An admission-only patch still fails at reuse. A shared-predicate change can expand
archive or recovery authority. Keep the fix at the admission and reuse boundaries.
An unborn branch has no commit, so requiring `HEAD` to resolve incorrectly rejects it.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/worktree-metadata-recovery.md), requirements 001 and 002.
- [Design](../../specs/tasks/system-design/worktree-metadata-recovery.md), Main-repository checkout compatibility.
- [Plan evidence](plan.md#evidence-and-root-cause).
- Existing `recovery_test.go`, `recovery_review_test.go`, and `manager_reuse_required_test.go` fixtures.

## Results

- Red: `(cd apps/backend && go test ./internal/worktree -run '^TestMainCheckoutAdmissionAndReuse$' -count=1 -v)` failed before the implementation because the healthy main checkout received the linked-pointer rejection.
- Green: `(cd apps/backend && go test ./internal/worktree -run 'TestMainCheckout' -count=1 -v)` passed, including initialized and unborn repositories, invalid metadata, mixed inventory, and linked-sibling recovery.
- `(cd apps/backend && go test ./internal/worktree ./internal/system/storage/workspaces -count=1)` passed.
- `git diff --check` passed. The Git-inspection denial was injected because this root-run environment cannot rely on filesystem permission errors.
- Native macOS and Windows tests were not available in this Linux environment.
