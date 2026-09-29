---
id: "02-prove-integration"
title: "Prove launch recovery and document limits"
status: complete
wave: 2
depends_on: ["01-restore-checkout"]
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-001
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-002
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-003
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-004
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
acceptance_criteria:
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-001.1
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-001.2
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-001.3
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-001.4
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-001.5
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-002.6
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-003.1
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-003.5
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-004.1
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-004.2
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-004.4
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-004.6
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-004.7
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-004.8
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-004.9
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.2
system_design:
  - ../../specs/tasks/system-design/worktree-metadata-recovery.md
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
---

# Task 02: Prove launch recovery and document limits

## Summary

Prove that existing launch and recovery entry points use guarded restoration
before attachment. Document eligibility and unrecoverable content in the public
Git guide after the behavior is implemented.

## In scope

- Real manager, Git, and SQLite integration through selected-environment executor admission.
- Additional-session preparation, prepared launch, resume, fresh-start preflight, and workspace restoration.
- Startup ordering, current path projection, typed refusal, executor exclusions, and multi-slot cases.
- Public recovery limits and synchronized package results.

## Out of scope

- New rendered UI, translations, controls, provider conversation identities, and migrations.
- Closing historical verification gaps unrelated to issue #4052.

## Acceptance

1. `TestMissingCheckoutRecoveryLaunchAndResume` reaches the real recovery
   manager and SQLite claim. Its route subtests prove restored path and branch
   identity before a runtime fake observes startup. Add lifecycle and
   orchestrator tests for routes outside the executor package. No recording-only
   admission stub can stand in for the restoration proof.
2. A busy shared environment or invalid slot prevents startup and preserves
   provider recovery state. Repo-free and non-Worktree requests perform no host
   recovery. Multiple selected slots preserve healthy content and expose one
   complete canonical inventory after success.
3. Update the how-to recovery section in `docs/public/git-operations.md`. Explain
   missing checkout versus missing metadata, same-branch restoration, live-consumer
   refusal, and lost uncommitted content. Reconcile links in operations or CLI
   guidance only where their wording conflicts. Record every required result.
4. The explicit `resume_new_branch` route reaches preflight with its authorization
   intact. Pair the service-boundary assertion with a real manager preflight and
   prove that no checkout is created until the established replacement path runs.
   Probe errors remain inspection errors, and confirmed branch loss keeps the
   existing typed recovery action.

## Verification

```bash
(cd apps/backend && go test -race ./internal/orchestrator/executor -run 'TestMissingCheckoutRecovery|TestWorktreeRecovery' -count=1)
(cd apps/backend && go test -race ./internal/orchestrator -run 'TestMissingCheckoutRecovery|Test.*Recovery' -count=1)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'TestMissingCheckoutRecovery|Test.*Recovery' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Name new route tests with the `TestMissingCheckoutRecovery` prefix so these
commands execute them. Confirm that each new test runs. Re-run task 01 checks
only if its files change or integration reveals a new concern.

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_missing_checkout_recovery_integration_test.go` (new)
- `apps/backend/internal/orchestrator/executor/executor_worktree_recovery.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go` and `executor_resume.go`, only for proven wiring gaps
- `apps/backend/internal/orchestrator/missing_checkout_recovery_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/missing_checkout_recovery_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`, only for proven admission gaps
- `apps/backend/internal/orchestrator/session_launch.go`, only for proven fresh-start preflight gaps
- `docs/public/git-operations.md`
- This plan and both work-order result sections

## Dependencies

Task 01. Its exact restoration and exclusive authority must pass before route coverage.

## Risks

A mocked admission callback can hide the original defect. A stale runtime must
not bypass the claim or become evidence of a successful recovered launch.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/worktree-metadata-recovery.md), requirements 001-004.
- [Design](../../specs/tasks/system-design/worktree-metadata-recovery.md), integration and compatibility.
- [Attach design](../../specs/tasks/system-design/additional-session-workspace-reuse.md).
- `executor_worktree_recovery_integration_test.go` and task 01 real-store fixtures.
- `.agents/skills/docs-maintainer/SKILL.md` and `docs/public/README.md`.

## Results

Completed on 2026-09-29.

- Extended `TestMissingCheckoutRecoveryLaunchAndResume` to exercise new-session
  preparation against an existing environment, prepared launch, and resume. All
  three routes use real Git, SQLite inventory and claims, and the real recovery
  manager. Launch and resume verify the canonical path, branch, and HEAD while
  the claim is held at the runtime startup boundary.
- Added `TestMissingCheckoutRecoveryLifecycleRestoresAndProjectsSelectedWorkspace`.
  It verifies lifecycle admission projects the restored checkout path and branch,
  then attaches that canonical row under the recovery claim.
- Added
  `TestMissingCheckoutRecoveryFreshStartPreflightPreservesProviderState`.
  A typed missing-checkout refusal prevents agent startup and preserves the
  provider resume token, session state, environment state, and recovery metadata.
- Extended multi-slot recovery coverage to verify that successful restoration
  retains both the unchanged healthy slot and restored slot in the canonical
  environment inventory. Existing scope tests confirm repo-free and excluded
  executor requests do not inspect host checkouts. Busy-consumer, invalid-slot,
  and no-mutation refusal cases are covered by real-store worktree tests.
- Added `TestMissingCheckoutRecoveryPreflightCarriesExplicitBranchReplacement`.
  The executor passes explicit authorization through real selected-environment
  preflight to the real Git recovery manager, preserves the absent checkout
  before replacement, and creates the authorized new branch through the existing
  worktree path. Orchestrator `RecoverSession` coverage separately proves that
  only `resume_new_branch` sets the preflight and launch authorization while
  preserving the session and provider conversation identity.
- Updated `docs/public/git-operations.md` to distinguish missing Git metadata
  from a missing checkout and explain branch identity, exclusive ownership,
  live-consumer refusal, attach-only behavior, and unrecoverable deleted files.
- Passed the required race tests for executor, orchestrator, and lifecycle
  recovery routes; the complete worktree race suite; and SQLite recovery-claim
  race tests.
- `make -C apps/backend build`, specification catalog/lint, public-doc tests and
  validation, PR-documentation reference coverage, and `git diff --check` passed.
- Backend-wide lint and Windows worktree test cross-compilation passed.
- PostgreSQL recovery-claim tests were not run because
  `KANDEV_TEST_POSTGRES_DSN` is not configured.
