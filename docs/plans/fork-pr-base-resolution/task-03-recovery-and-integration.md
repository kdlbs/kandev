---
id: "03-recovery-and-integration"
title: "Guard recovery and prove integration"
status: done
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
- Keep an explicit manual base selection authoritative across provider refreshes.
- Add end-to-end Go coverage from provider response through production wiring
  to real Git fixtures. Do not manually seed the final resolved ref.
- Include a legacy PR attachment with no stored `comparison_target` and prove
  its live resolved target reaches real worktree materialization.
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

- Default recovery rejects a persisted cross-repository GitHub PR target before
  resolving or writing a repository default or task base. It leaves comparison
  metadata and the current launch-error stamp intact. Explicit manual base
  selection remains authoritative across provider refreshes, and ordinary
  retry-default still updates its branch.
- Cancellation and deadline errors from live PR-base lookup now stop task
  repository resolution. Multi-repository preparation returns failure when a
  valid sibling is followed by an unresolved required target, and cancellation
  returns no prepared workspace. Qualified-base recreation validates before
  removing the existing checkout path.
- `TestForkPRBasePreparationEndToEnd` starts with a fake provider PR response,
  passes the converted PR base through the backendapp lifecycle mapper and
  lifecycle worktree preparer, and creates a linked worktree using real local
  fork and upstream repositories. It verifies the non-default target OID, PR
  head, persisted comparison metadata, unchanged fork origin refs and push URL,
  and disabled push routing on the comparison remote. The fixture performs no
  external push.
- `go test ./internal/orchestrator ./internal/backendapp -run 'Test(RecoverTaskLaunch|ForkPRBasePreparationEndToEnd)' -count=1`: passed.
- `go test ./internal/orchestrator/executor -run '^TestResolveTaskRepoInfo_PRBaseLookupCancellationAbortsLaunchResolution$' -count=1`: passed.
- `go test ./internal/backendapp -run '^TestForkPRBasePreparationEndToEnd$' -count=1`: passed.
- `go test ./internal/agent/runtime/lifecycle -run 'TestWorktreePreparer_(MultiRepoUnresolvedQualifiedBaseFailsDespiteValidSibling|CanceledQualifiedBaseDoesNotReturnPreparedWorkspace)' -count=1`: passed.
- `go test ./internal/worktree -run '^TestRecreate_QualifiedPRBaseFailureLeavesExistingPathUntouched$' -count=1`: passed.
- Full affected Go packages passed:
  `go test ./internal/common/gitbase ./internal/worktree ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agentctl/server/api ./internal/agentctl/server/process ./internal/github ./internal/backendapp ./internal/orchestrator ./internal/orchestrator/executor -count=1`.
- That full package run includes the complete orchestrator and backendapp suites
  requested in this work order.
- `make -C apps/backend build`: passed for agentctl targets, Kandev, mock-agent,
  acpdbg, and winjob.
- `python3 scripts/list-docs.py validate`: passed (299 decisions and 1112
  specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `node --test scripts/validate-public-docs.test.mjs`: passed (62 tests).
- `node scripts/validate-public-docs.mjs`: passed (47 published docs pages).
- Changed Go files passed `gofmt -l`; `git diff --check` passed.
- Updated `docs/public/git-operations.md` with repository-qualified PR base
  identity, required-fetch failures, retry choices, and unchanged push routing.
- Review correction: the legacy PR producer-to-materializer test starts with
  only `pr_number` metadata, resolves the linked upstream PR through the
  configured resolver, and verifies a real prepared worktree at the PR head
  using the upstream target branch and OID.
- Review correction: base selection now preserves an explicit manual override
  across provider and system refreshes. The persistence marker is committed
  with the base update; an explicit comparison-target association clears it.
- Review correction: choosing the already displayed branch still persists the
  manual override marker for a legacy PR, while skipping task events and agent
  refreshes because the branch did not change. The complete task-service suite
  passes with this behavior.
