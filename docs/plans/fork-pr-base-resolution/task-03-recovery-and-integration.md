---
id: "03-recovery-and-integration"
title: "Guard recovery and prove integration"
status: pending
wave: 3
depends_on: 
  - "02-qualified-materialization"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
acceptance_criteria:
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.15
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.16
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.17
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.18
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.19
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
---

# Task 03: Guard recovery and prove integration

## Summary

Prevent default recovery from erasing an explicit upstream target. Prove provider,
preparation, and Git behavior together, then document the delivered behavior.

## In scope

- Reject `retry_default` for explicit cross-repository bindings before writes.
  Preserve current error stamps and the task's comparison metadata.
- Preserve ordinary default recovery and explicit manual base selection.
- Add end-to-end Go coverage from provider response through production wiring
  to real Git fixtures. Do not manually seed the final resolved ref.
- Include first creation, recreation, remote materialization, non-default
  upstream targets, and a mixed task with one unresolved fork repository.
- Update `docs/public/git-operations.md` during implementation. Explain PR
  target identity, required-fetch errors, retry, and unchanged push routing.
- Record each work-order command result and synchronize plan status.

## Out of scope

New UI, issue #3856, automatic history repair, PR publication, and broad audits.

## Acceptance

- `TestRecoverTaskLaunch_ForkPRDefaultPreservesTarget` proves zero default/base
  writes and zero relaunch on rejected recovery. Ordinary recovery still works.
- `TestForkPRBasePreparationEndToEnd` proves the target OID and preserved PR
  head, origin, push configuration, and metadata through real request wiring.
- One valid sibling cannot hide an unresolved required target. Cancellation
  leaves the task unlaunched without claiming base resolution succeeded.

## Verification

```bash
(cd apps/backend && go test ./internal/orchestrator ./internal/backendapp -run 'Test(RecoverTaskLaunch|ForkPRBasePreparationEndToEnd)' -count=1)
(cd apps/backend && go test ./internal/orchestrator ./internal/backendapp -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Write the recovery regression before the guard. End-to-end fixtures must use
local temporary repositories and fake provider responses, with no real push.

## Files likely touched

- `apps/backend/internal/orchestrator/task_launch_recovery.go`
- `apps/backend/internal/orchestrator/task_launch_recovery_pr_base_test.go` (new)
- `apps/backend/internal/backendapp/pr_base_integration_test.go` (new)
- `docs/public/git-operations.md`
- `docs/plans/fork-pr-base-resolution/plan.md`
- This package's three work-order result sections.

## Dependencies

Tasks 01 and 02. Use their test fixtures and the current recovery test harness.

## Risks

The manual branch-update service intentionally clears comparison targets.
Automatic recovery must not reuse that write path for a qualified target.
A stale error stamp must still reject the request before all mutations.

## Parallelism

`sequential`

## Inputs

- Worktree base refresh design: Recovery and persistence.
- Task launch recovery design and existing stamp/race tests.
- `service_branch_update_test.go`: same-name manual selection clears the target.
- Public Git operations guide, primarily a how-to/reference page.

## Results

Pending.
