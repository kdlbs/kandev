---
id: "01-scope-admission"
title: "Scope recovery admission"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-001
acceptance_criteria:
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-001.1
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-001.2
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-001.3
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-001.4
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-001.5
system_design:
  - ../../specs/tasks/system-design/worktree-metadata-recovery.md
---

# Task 01: Scope recovery admission

## Summary

Replace the task-ID-only recovery callback with selected-environment admission.
Keep the resolver's existing executor-transition and inherited-environment rules.

## In scope

- A typed internal request with session, selected executor, environment, owner,
  generation, and canonical repository slots.
- Admission after selection and before launch, resume, or workspace-only reuse.
- Negative tests that detect even one host filesystem inspection for an excluded executor.
- Tests with an unrelated broken environment and identical remote/host path strings.

## Out of scope

- Recovery claims and filesystem replacement changes, which belong to Task 02.
- Frontend controls and provider-specific workspace recovery.

## Acceptance

1. `TestManagerAdmitRecoveryExcludedExecutorsDoNotInspectHostCheckout` and
   `TestManagerAdmitRecoveryRepoFreeWorktreeInventoryIsNoop` cover every executor
   and empty-inventory case in the plan. A remote origin remains eligible for host
   Worktree recovery.
2. Explicit and inherited environment requests inspect only their selected slots.
   Executor transitions never inspect the previous environment through the new request.
3. Existing prepared-launch, resume, and workspace-only paths all use the new
   boundary. Preparation no longer performs a task-wide filesystem mutation.

## Verification

From the repository root:

```bash
(cd apps/backend && go test ./internal/orchestrator/executor ./internal/worktree -count=1)
git diff --check
```

Start with failing behavior tests through existing public executor entry points.
After the callback change, update its existing test fixtures to contain explicit
selected environments. Do not preserve task-wide behavior only to retain old tests.

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_worktree_recovery_test.go` (new)
- `apps/backend/internal/orchestrator/executor/executor_resume_test.go`
- `apps/backend/internal/orchestrator/executor/executor_launch_failure_classification_test.go`
- `apps/backend/internal/orchestrator/event_handlers_automation.go`
- `apps/backend/internal/worktree/manager.go`

## Dependencies

None. Task 02 is required before publication of the complete correction.

## Risks

Task-level lookup can hide an explicit inherited environment. A recorded runtime
can also supply executor identity that differs from a stale profile selection.
Both cases need evidence from the real selection path.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/worktree-metadata-recovery.md), requirement 001.
- [Design](../../specs/tasks/system-design/worktree-metadata-recovery.md), Selected environment admission.
- Existing executor launch and resume failure tests.

## Results

Implemented selected-environment admission in the executor, lifecycle manager,
and worktree manager. Empty inventories and excluded executors return before host
inspection. Production wiring carries the selected environment owner and
generation. Verification:

- `go test -vet=off ./internal/worktree -count=1`: passed.
- `go test -vet=off ./internal/orchestrator/executor -run 'TestWorktreeRecovery(Launch|Resume)Integration' -count=1`: passed.
- `go test -vet=off ./internal/agent/runtime/lifecycle -count=1`: passed.
- `git diff --check`: passed.
