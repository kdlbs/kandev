---
id: "03-prove-integration"
title: "Prove recovery integration"
status: in_progress
wave: 3
depends_on:
  - "02-claim-authority"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-001
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-002
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-003
acceptance_criteria:
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-001.1
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-001.2
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-001.3
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-001.4
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-001.5
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-002.5
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-002.6
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-002.7
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-003.1
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-003.5
system_design:
  - ../../specs/tasks/system-design/worktree-metadata-recovery.md
---

# Task 03: Prove recovery integration

## Summary

Prove that real launch and resume requests use the selected and recovered inventory.
Complete public guidance only after the compatibility contract has executable evidence.

## In scope

- A real Git fixture and production executor-to-manager connection.
- Launch, resume, inherited environments, repo-free tasks, executor transitions,
  remote-executor exclusion, and multi-repository partial failure.
- A recording lifecycle boundary that proves the exact workspace paths supplied
  to startup, or zero startup calls after refusal.
- Final public documentation for automatic recovery eligibility, retained files,
  branch naming, staging limits, busy refusal, and per-slot partial success.
- Accurate requirement, design, ADR, and work-order statuses.

## Out of scope

- New UI layouts or controls.
- Broad unrelated verification or cleanup.

## Acceptance

1. `TestWorktreeRecoveryLaunchIntegration` and `TestWorktreeRecoveryResumeIntegration`
   prove production wiring with real Git and durable inventory. They include
   shared-runtime refusal and refreshed workspace paths after replacement.
2. Typed refusal reaches the existing response classification without agent startup
   or unsafe retry actions. No excluded executor triggers host filesystem access.
3. Public documentation describes only implemented behavior. All work-order commands
   have recorded results, and the design matches the final implementation.

## Verification

From the repository root:

```bash
(cd apps/backend && go test ./internal/orchestrator/... ./internal/worktree ./internal/agent/runtime/lifecycle -count=1)
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
git status --short -- docs/plans/worktree-metadata-recovery
```

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_worktree_recovery_test.go`
- `apps/backend/internal/orchestrator/` focused production-wiring test
- `apps/backend/internal/orchestrator/executor/launch_failure.go`
- `apps/backend/internal/orchestrator/executor/executor_launch_failure_classification_test.go`
- `docs/public/git-operations.md`
- This plan, its work orders, and the linked requirement, design, and ADR.

## Dependencies

Tasks 01 and 02, including non-skipped PostgreSQL evidence.

## Risks

A callback mock that always returns success cannot prove real selection or
recovery. The test must inspect durable slot identities and lifecycle arguments.
Do not promote a draft design solely because its document linter passes.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/worktree-metadata-recovery.md)
- [Design](../../specs/tasks/system-design/worktree-metadata-recovery.md)
- [Proposed ADR](../../decisions/2026-09-10-worktree-metadata-recovery-boundary.md)
- Existing launch-failure and resume tests.

## Results

The production executor launch and resume paths now call selected-environment
admission after executor selection. `TestWorktreeRecoveryLaunchIntegration` and
`TestWorktreeRecoveryResumeIntegration` pass and assert the selected task,
environment, owner generation, executor, and repository slot. The full
orchestrator/worktree and lifecycle packages pass, and existing real-Git
worktree recovery tests pass. The integration tests use a recording admission
boundary rather than combining real SQLite recovery publication with lifecycle
runtime startup, so this work order remains in progress.
