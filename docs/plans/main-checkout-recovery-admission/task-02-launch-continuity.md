---
id: "02-launch-continuity"
title: "Prove launch continuity and error retirement"
status: done
wave: 2
depends_on:
  - "01-main-checkout-validation"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-001
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-002
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-003
acceptance_criteria:
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-001.2
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-001.3
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-002.8
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-003.5
system_design:
  - ../../specs/tasks/system-design/worktree-metadata-recovery.md
---

# Task 02: Prove launch continuity and error retirement

## Summary

Connect launch, resume, and workspace restoration tests to the real checkout
validator. Prove that an ordinary relaunch retires the matching previous launch
error while preserving a newer error and the existing session.

## In scope

- Add `TestMainCheckoutLaunchIntegration` using a real main repository and
  `Manager.AdmitRecovery` through the selected-admission callback.
- Extend the production-entrypoint patterns in `executor_worktree_recovery_integration_test.go`.
  Do not replace admission with an unconditional success stub.
- Cover additional-session launch, prepared launch, resume, and workspace-only
  restoration. Use a recording runtime to assert the canonical path and inventory.
- Prove that a live primary session does not trigger a replacement claim for a
  healthy main checkout. Retain its identity, dirty files, and staging state.
- Add `TestMainCheckoutRelaunchRetiresMatchingError` at the orchestrator service
  boundary. Seed the prior task-level metadata failure and launch normally.
- Seed the retained refusal with its production-shaped empty recovery-action list;
  successful `StartTask` must retire the matching stamp without invoking an action,
  while a successor-stamped error remains untouched.
- Assert successful launch clears only the captured error stamp. A newer error
  survives. Preserve existing refusal and `noRetry` behavior for unsafe metadata.
- Add a short paragraph to the public Git recovery guide after tests pass:
  healthy main checkouts do not require linked metadata recovery. Existing metadata
  errors must not prompt users to delete their checkout.

## Out of scope

- A new dismissal endpoint, recovery action, UI control, or bulk error migration.
- Independent workspace-panel redesign or generic connection retry changes.
- New executor support or changes to provider-owned filesystems.

## Acceptance

1. Public launch/resume and workspace restoration paths reach real manager
   validation and the recording runtime with the original main-checkout path.
   The same fixture fails before Task 01. No repair artifact or replacement is created.
2. An ordinary launch can proceed despite the retained misclassification error.
   Its matching error retires after success. A successor error survives. Invalid
   metadata still prevents startup and preserves all files.
3. Existing excluded-executor and linked-worktree tests pass. The public guide
   describes only verified behavior. Record remaining native-platform test gaps.

## Verification

From the repository root:

```bash
(cd apps/backend && go test ./internal/orchestrator/executor -run 'TestMainCheckout|TestWorktreeRecovery' -count=1 -v)
(cd apps/backend && go test ./internal/orchestrator -run 'TestMainCheckout|TestClearTaskLaunch|TestTaskLaunch' -count=1 -v)
(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'TestMainCheckout' -count=1 -v)
(cd apps/backend && go test ./internal/worktree -count=1)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use `workspace_main_checkout_test.go` for the lifecycle restoration scenario.
Verify each named new test runs. A command with no matching tests is not evidence.
Update plan and work-order results with exact commands and outcomes.

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_worktree_recovery_integration_test.go`
- `apps/backend/internal/orchestrator/task_launch_main_checkout_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/workspace_main_checkout_test.go` (new)
- `docs/public/git-operations.md`
- `docs/plans/main-checkout-recovery-admission/plan.md`
- Both work orders in this package, for status and results.

## Dependencies

Task 01 supplies validated admission and reuse. Run this task sequentially.

## Risks

Current executor tests use a recording admission callback. Reusing that callback
without a real manager hides the defect. A successful helper test does not prove
that a session can launch. If retained error state blocks ordinary launch, record
the exact path and reconcile the recovery contract before adding another action.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/worktree-metadata-recovery.md).
- [Design](../../specs/tasks/system-design/worktree-metadata-recovery.md), Implemented integration and Main-repository checkout compatibility.
- `executor_worktree_recovery_integration_test.go` for selected environment fixtures.
- `task_launch_pr_gate.go` for stamp-guarded retirement and `task_operations.go` for successful launch wiring.
- Existing lifecycle workspace recovery fixtures.
- [Public Git guide](../../public/git-operations.md), Automatic recovery of missing linked-worktree metadata.

## Results

- `(cd apps/backend && go test ./internal/orchestrator/executor -run 'TestMainCheckout|TestWorktreeRecovery' -count=1 -v)` passed. The launch, additional-session, and resume cases use real manager admission.
- `(cd apps/backend && go test ./internal/orchestrator -run 'TestMainCheckout|TestClearTaskLaunch|TestTaskLaunch' -count=1 -v)` passed. The matching old error clears after launch; a successor error remains.
- `(cd apps/backend && go test ./internal/orchestrator -run '^TestMainCheckoutRelaunchRetiresMatchingError$' -count=1)` passed after review remediation. The retained metadata-refusal fixture has no recovery actions, ordinary `StartTask` retires the matching error without invoking an action, and a successor-stamped error remains.
- `(cd apps/backend && go test ./internal/orchestrator/executor -run '^TestMainCheckoutInspectionTimeoutRemainsRetryable$' -count=1)` passed. An inspection timeout keeps the ordinary retry action and is not classified as a metadata refusal.
- `(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'TestMainCheckout' -count=1 -v)` passed. Workspace-only restoration kept the main checkout path and repository inventory.
- `(cd apps/backend && go test ./internal/worktree -count=1)` passed. The full executor suite and the full lifecycle suite also passed.
- `(cd apps/backend && go build ./...)` passed.
- `node scripts/validate-public-docs.mjs`, `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed.
- The first full lifecycle run had a temporary-directory cleanup error in an existing test. That test passed alone, and the full lifecycle suite passed on rerun.
- Native macOS and Windows tests were not available in this Linux environment.
