---
id: "01-restore-checkout"
title: "Restore missing checkouts under exclusive authority"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-001
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-002
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-003
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-004
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
acceptance_criteria:
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-001.3
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-002.6
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-002.7
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-003.1
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-003.2
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-003.3
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-003.4
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-004.1
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-004.2
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-004.3
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-004.4
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-004.5
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-004.6
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-004.7
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-004.8
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-004.9
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.2
system_design:
  - ../../specs/tasks/system-design/worktree-metadata-recovery.md
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
---

# Task 01: Restore missing checkouts under exclusive authority

## Summary

Extend selected-environment recovery admission to restore an absent canonical
checkout while preserving its identity and branch. Preserve read-only attachment
and all existing authority checks.

## In scope

- Missing-path preflight, exact branch resolution, safe creation, and targeted stale-registration handling.
- Existing durable claims, current-generation validation, and requester/sibling/borrowed-runtime exclusion.
- Operation records, restart refusal or continuation, partial success, and mixed inventories.
- Real Git and real SQLite claim regressions. Reuse existing fixture patterns in `store_test.go`.

## Out of scope

- Changing attach-only refusal, general recreation, or remote executors.
- New schema, UI controls, changes to the established branch-replacement behavior,
  and deleting partial checkouts.

## Acceptance

1. The first regression, `TestMissingCheckoutRecoveryPreservesCanonicalIdentity`,
   fails before implementation because admission skips the deleted checkout.
   After the fix, it restores the same path, worktree ID, branch, and commit,
   then passes attach-only reuse. Cover both retained and removed admin metadata.
2. Every unsafe or busy case fails before mutation. Cover the requester runtime,
   another same-task session, a borrowing task, a failed liveness read, and a
   concurrent startup. Cover healthy+missing and missing+invalid slots. Failed
   or deleted rows must not stand in for an active requested slot.
3. Interruption retains the operation identity and claim. Inject failures before
   Git creation, after creation, and before completion. A retry either proves
   the exact completed state or refuses while preserving all files. Verify
   different generations, concurrent admission, and a second slot failure.
4. Same-operation replay through independent managers is excluded by a real
   operating-system lock until admission release. A completed operation with a
   retained claim is reconciled only after the exact owner, generation, session,
   operation, record, and checkout are revalidated under that lock. Cover a
   completed multi-slot operation and reject mismatched claims.
5. Missing local and tracking refs trigger an exact origin probe. Cover a
   surviving origin branch, probe/authentication failure, and confirmed loss
   with the existing typed error. An explicit `resume_new_branch` authorization
   reaches the real selected-environment preflight and permits the established
   replacement path without automatic base-branch fallback.
6. Deterministic barriers replace the task-root path, replace or introduce a
   target before Git add, and introduce a checkout before stale-metadata cleanup.
   Assert the appeared or unrelated tree is unchanged and stale cleanup never
   removes a checkout.

## Verification

Run the named regression first and record its expected failure. Then run:

```bash
(cd apps/backend && go test -race ./internal/worktree -count=1)
(cd apps/backend && go test -race ./internal/task/repository/sqlite -run 'TestTaskEnvironmentRecoveryClaim' -count=1)
(cd apps/backend && : "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL DSN}" && go test -race ./internal/task/repository/sqlite -run '^TestPostgresTaskEnvironmentRecoveryClaimSerializesIndependentRepositories$' -count=1 -v)
git diff --check
```

Run the PostgreSQL command against a disposable database. Record an unavailable
DSN as an outstanding verification item, not a pass. Do not broaden the schema
or claim protocol to work around a failed test.

## Files likely touched

- `apps/backend/internal/worktree/recovery_admission.go`
- `apps/backend/internal/worktree/missing_checkout_recovery.go` (new)
- `apps/backend/internal/worktree/missing_checkout_recovery_record.go` (new)
- `apps/backend/internal/worktree/missing_checkout_recovery_test.go` (new)
- `apps/backend/internal/worktree/missing_checkout_recovery_claim_test.go` (new)
- `apps/backend/internal/worktree/store.go` and `store_test.go`, only for required claim-aware publication

## Dependencies

None. Use the existing metadata recovery claim and safe-directory facilities.

## Risks

Repository-wide pruning, general `recreate`, or force checkout would violate the
contract. A healthy checkout cannot bypass a retained operation claim.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/worktree-metadata-recovery.md), requirements 001-004.
- [Design](../../specs/tasks/system-design/worktree-metadata-recovery.md), missing canonical checkout recovery.
- [Attach contract](../../specs/tasks/requirements/additional-session-workspace-reuse.md), requirement 001.
- Existing `recovery_admission_scope_test.go`, `manager_reuse_required_test.go`, and `store_test.go`.
- `.agents/skills/tdd/SKILL.md` and its backend test reference.

## Results

Implemented in `internal/worktree/missing_checkout_recovery.go`. Recovery admission
now restores a provably absent managed checkout from its exact surviving local,
origin-tracking, or persisted compaction head under the existing environment claim.
It retains the worktree row identity, path, and branch. Attach-only reuse remains
read-only. Stale registration removal is restricted to one exact, unlocked,
prunable registration.

Real-Git/SQLite regressions cover canonical identity, stale registrations,
missing managed roots, local/remote/compaction branch identity, attach-only
refusal, requester runtime, same-task and borrowed consumers, liveness-query
failure, competing claim, failed/deleted rows, generation mismatch, interrupted
resume before and after Git materialization, healthy+missing and missing+invalid
inventories, and preservation of a successful first slot after a later slot
fails. They also verify that a mismatched stable task-root marker is refused
while a transferred environment owner can recover the original marked root.
Review regressions prove that Git receives a pinned checkout identity, stale
cleanup removes only its exact administrative entry, and a replacement or
appeared checkout remains byte-for-byte unchanged. Independent managers contend
on the real operation lock for same-operation replay. Completed claims are
settled only after exact record, checkout, owner, generation, session, and
operation revalidation, including a completed multi-slot restart. Remote branch
tests distinguish an exact surviving origin ref, authentication failure, and
confirmed branch loss; the explicit new-branch preflight is authorized without
automatic materialization or base-branch substitution.

PR #4061 review follow-up adds regressions for a remote-only branch advancing
between probe and fetch, refreshing its exact head on replay, and removing an
operation ref only when it still matches the recorded head. An interrupted
cleanup retry now removes a leftover ref before completing its record. Coverage
also checks that unrelated environment claims do not block healthy admission,
confirmed branch loss during claimed inspection creates no empty operation
record, early failures remove only an empty reserved target, and Git can add
through an inherited pinned working directory on descriptor-path platforms.

Validation passed:

- `(cd apps/backend && go test -race ./internal/worktree -count=1)`
- `(cd apps/backend && go test -race ./internal/task/repository/sqlite -run 'TestTaskEnvironmentRecoveryClaim' -count=1)`
- `(cd apps/backend && golangci-lint run ./internal/worktree --timeout=5m)`
- `(cd apps/backend && GOOS=windows GOARCH=amd64 go test -c -o /tmp/kandev-worktree.test.exe ./internal/worktree)`
- `make -C apps/backend lint`
- `make -C apps/backend build`
- `git diff --check`

The PostgreSQL claim test was not run because `KANDEV_TEST_POSTGRES_DSN` is not
configured. It remains an outstanding cross-process database verification.

PR #4061 review-fix verification on 2026-09-29:

- Focused replay, temporary-ref cleanup, unrelated-claim, no-empty-plan,
  empty-target, and pinned-working-directory regressions: passed.
- `go test -race ./internal/worktree -count=1`: passed.
- `go test -race ./internal/orchestrator/executor ./internal/orchestrator ./internal/agent/runtime/lifecycle -run 'TestMissingCheckoutRecovery|Test.*Recovery|TestRecoverSession_ResumeNewBranchPreservesSessionAndProviderIdentity' -count=1`: passed.
- `go test -race ./internal/task/repository/sqlite -run 'TestTaskEnvironmentRecoveryClaim' -count=1`: passed.
- `make -C apps/backend lint` and `make -C apps/backend build`: passed.
- Windows/amd64 and Darwin/arm64 worktree test cross-compiles: passed.
- Catalog, specification lint, public-doc validators (62 tests and 47 pages), PR-documentation reference coverage, and `git diff --check`: passed.
- PostgreSQL remains unverified because no disposable
  `KANDEV_TEST_POSTGRES_DSN` was supplied.
