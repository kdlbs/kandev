---
id: "01-completed-admission"
title: "Admit current work after completed relocation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-001
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-002
acceptance_criteria:
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.3
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.4
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.6
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.7
  - AC-TASKS-MANAGED-CLONE-RELOCATION-002.4
system_design:
  - ../../specs/tasks/system-design/managed-clone-relocation.md
---

# Task 01: Admit current work after completed relocation

## Summary

Separate completed relocation history from unfinished transfer proofs.
Reuse the current valid checkout without altering its work or historical HEAD.
Preserve current identity, inventory, claim, and unfinished-operation guards.

## In scope

- Write `TestCompletedRelocationAdmitsCurrentWork` before production edits.
- Parameterize existing GitHub/GitLab managed clone identities.
- Cover commit, amend, non-descendant rebase, and staged/untracked/ignored edits.
- Reconstruct the manager and reuse unchanged persisted complete journal JSON.
- Cover completed siblings together and completed plus invalid/unfinished slots.
- Cover valid current HEAD without dependence on the old commit object.
- Preserve strict HEAD validation for published `materialized` records.
- Cover matching leftover claims and incomplete dirty-recovery companion records.
- Cover unrelated claims, owner drift, wrong clone/branch, unreadable records,
  cancellation, and Git errors without unsafe effects.
- Keep source-removal, excluded-executor, and existing restart cases passing.

## Out of scope

- Executor/service changes, UI, schema, flags, deployment, and live repair.
- New relocation authority, snapshot replay, or historical record replacement.

## Acceptance

1. Current complete checkouts pass without historical equality or ancestry.
   Current work, index, tokens, and historical evidence remain unchanged.
2. Every selected slot passes current canonical identity and ownership proof.
   Unverified unfinished transfers and invalid siblings still refuse startup.
3. Completed reuse causes no journal writes when bookkeeping already agrees.
   Any remaining bookkeeping uses existing locks and matching operation fences.

## Verification

Run from the repository root. Record the first expected failure before the fix.

```bash
(cd apps/backend && go test -trimpath ./internal/worktree -run '^TestCompletedRelocationAdmitsCurrentWork$' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/worktree -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/worktree/managed_clone_relocation_archive.go`
- `apps/backend/internal/worktree/recovery_admission.go` only for required ordering or authority.
- New `apps/backend/internal/worktree/managed_clone_completed_reuse_test.go`
- New `apps/backend/internal/worktree/managed_clone_completed_claim_test.go`
- `apps/backend/internal/worktree/managed_clone_relocation_recovery_test.go` for shared fixtures only.

## Dependencies

None.

## Risks

- A complete record is historical evidence, not claim-release authority.
- Relaxing unfinished HEAD checks can publish an edited replacement.
- A crash can leave companion bookkeeping incomplete after the complete write.
- Test file size limits require focused files rather than large-suite appends.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/managed-clone-relocation.md), criteria above.
- [Design](../../specs/tasks/system-design/managed-clone-relocation.md#completed-relocation-continuity).
- [Plan evidence and compatibility matrix](plan.md).
- Existing `TestAdmitRecoveryReconcilesPublishedRelocationClaimAfterRestart`.
- Existing prepared restart, permission retry, and inventory refusal fixtures.

## Results

Implemented state-aware completed-journal verification in
`managed_clone_relocation_archive.go`. Completed records validate the current
managed destination, provider origin, registered worktree, branch, and current
commit without comparing historical HEADs. Ordinary reuse leaves completed
journals and local work unchanged. Matching claims and interrupted companion
records reconcile under the existing recovery locks. Unfinished materialized
records retain exact-HEAD verification.

Verification:

- New real-Git regression failed before the production change for later commits,
  amended commits, rewritten history, and a missing historical object.
- `(cd apps/backend && go test -trimpath ./internal/worktree -run '^TestCompletedRelocation' -count=1)`: passed.
- `(cd apps/backend && go test -trimpath -race ./internal/worktree -count=1)`: passed.
- `git diff --check`: passed.
