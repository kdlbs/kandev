---
id: "01-attachment-identity"
title: "Repair PR attachment identity validation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
  - REQ-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001
acceptance_criteria:
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.11
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.15
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.16
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.17
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.18
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.19
  - AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.1
  - AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.2
  - AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.3
  - AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.4
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
  - ../../specs/integrations/system-design/github-fork-review-start.md
---

# Task 01: Repair PR attachment identity validation

This work order preserves the original implementation sequence. The Results
section records the delivered implementation and verification.

## Summary

Accept a valid fork PR when the ordinary task attachment identifies its target
repository. Preserve exact identity checks for explicit bindings and fork-attached
checkouts. Resolve from existing metadata so unprepared failed tasks can retry.
Keep unattended GitHub review-watch execution disabled for fork or
identity-incomplete PRs until a user starts the linked task.

## In scope

- Add `TestResolveTaskRepoInfo_TargetAttachedForkPRWithoutContribution` in
  `executor_pr_base_identity_test.go` before production edits. Use an upstream
  attachment, a distinct fork head, matching checkout branch, and only PR metadata.
  The initial run must fail with the reported repository-binding error.
- Update `resolvePRBaseForLaunch` and `validPRBaseIdentity` with the three
  attachment forms from the design. Keep provider source identity transient.
- Cover target mismatch, wrong PR number/branch, missing head identity, and empty
  checkout branch. Explicit contribution and comparison mismatches cannot enter
  the unbound target-attached path. Keep current explicit-binding fallback rules.
- Cover resolver lookup with no linked row, one exact linked row, duplicate
  matches, same-number foreign rows, cancellation, and provider errors.
- Add `TestTargetAttachedForkPRBasePreparationEndToEnd` in
  `executor_pr_base_materialization_test.go`, beside existing real Git
  materialization coverage. Use separate fork/upstream repositories with different
  same-named base commits. Publish the fork head under upstream `refs/pull/N/head`.
  Prove target base, PR head, and unchanged origin/push configuration.
- Repeat resolution for an unprepared task to represent retry. Include a mixed
  two-repository preparation where one invalid binding blocks the whole launch.
- Keep same-repository GitHub review watches eligible for configured auto-start.
  Persist a manual-start marker for fork or identity-incomplete review watches,
  enforce it at automated launch boundaries, and prove a direct manual start
  remains available. Leave ordinary browser PR-link tasks unaffected.

## Out of scope

New database schema or migrations, browser payload changes, contribution push
authorization, automatic repair of live tasks, and PR-association lifecycle
changes.

## Acceptance

1. The ordinary upstream-attached fixture fails before the fix and succeeds
   after it, with or without a completed linked association.
2. Explicit binding mismatches and ambiguous identities keep their existing
   rejection or stored-qualified-target behavior. No unrelated base is applied.
3. Real Git preparation uses the correct head and target commits. Retry and
   mixed-repository checks preserve cancellation, fallback, and push invariants.

## Planned verification sequence

This is the original RED/GREEN sequence. The Results section records the
commands and outcomes from implementation.

```bash
(cd apps/backend && go test ./internal/orchestrator/executor -run '^TestResolveTaskRepoInfo_TargetAttachedForkPRWithoutContribution$' -count=1 -v)
(cd apps/backend && go test ./internal/orchestrator/executor -run '^TestTargetAttachedForkPRBasePreparationEndToEnd$' -count=1 -v)
(cd apps/backend && go test ./internal/orchestrator/executor ./internal/backendapp ./internal/worktree -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The first test records the expected pre-fix RED; the second proves real Git
materialization. Both tests reside in `internal/orchestrator/executor`. The
final package run also covers existing same-repository, explicit contribution,
resolver, and qualified-base regressions.

## Files touched

- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_pr_base_identity_test.go`
- `apps/backend/internal/orchestrator/executor/executor_pr_base_materialization_test.go`
- `apps/backend/internal/backendapp/pr_base_resolver_test.go`
- `apps/backend/internal/orchestrator/event_handlers_github.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_github_review_test.go`
- `apps/backend/internal/task/models/models.go`
- `docs/specs/integrations/requirements/github-fork-review-start.md`
- `docs/specs/integrations/system-design/github-fork-review-start.md`
- `docs/decisions/2026-09-23-fork-pr-review-watch-manual-start.md`

## Dependencies

None.

## Risks

A loose target match can bypass an explicit source binding. A linked-row
requirement can reintroduce the browser association race. Tests must exercise
production resolution and preparation, not only a copied predicate.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/workspaces/requirements/worktree-base-refresh.md), .11 and .15-.19.
- [Design clarification](../../specs/workspaces/system-design/worktree-base-refresh.md#ordinary-pr-link-launch-compatibility).
- [Fork review-start requirement](../../specs/integrations/requirements/github-fork-review-start.md).
- [Fork review-start design](../../specs/integrations/system-design/github-fork-review-start.md) and [decision](../../decisions/2026-09-23-fork-pr-review-watch-manual-start.md).
- `executor_pr_base_identity_test.go` and
  `executor_pr_base_materialization_test.go`.
- [Comparison target ADR](../../decisions/2026-08-19-repository-qualified-comparison-targets.md).

## Results

Completed. The ordinary target-attached fork regression failed before the fix
with the reported repository-binding error. It passes after the change, along
with wrong target/number/branch, missing head repository, empty checkout,
explicit comparison/source mismatches, cancellation, retry, mixed-repository,
resolver association and real Git preparation coverage.

Commands and results:

- `go test ./internal/orchestrator/executor -run '^TestResolveTaskRepoInfo_TargetAttachedForkPRWithoutContribution$' -count=1 -v`: expected RED before production change.
- `go test ./internal/orchestrator/executor -run '^TestTargetAttachedForkPRBasePreparationEndToEnd$' -count=1 -v`: expected RED before production change; passed after the test exercised normal PR-head fetch.
- Focused new executor and resolver tests: passed.
- `go test ./internal/orchestrator/executor ./internal/backendapp ./internal/worktree -count=1`: passed.
- Specification catalog, spec linter tests, all-spec lint and `git diff --check`: passed.

Security follow-up results:

- `go test ./internal/orchestrator -run 'TestBuildReviewTaskRequest_ForkPRRequiresManualStart|TestAutoStart_ForkReviewWaitsForManualStart' -count=1`: passed.
- `go test ./internal/orchestrator ./internal/orchestrator/executor ./internal/backendapp ./internal/worktree -count=1`: passed.
- `make -C apps/backend build`: passed.
- `(cd apps/web && pnpm e2e:run --project chromium tests/task/create-task-github-url.spec.ts)`: 10 passed.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-create-task-remote-repo.spec.ts)`: 7 passed.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check`: passed.
- Same-repository review watches retain automatic start. Fork and identity-incomplete review watches remain available without unattended execution; workflow and central automatic-start gates enforce the marker, and direct manual start launches.
- Requirement, system design, and decision links above record the delivered security boundary. Browser PR-link launch remains unchanged.

The test fixture uses separate fork/upstream bare repositories with different
same-named `main` commits. It publishes the fork head under the upstream PR
ref and preserves the upstream origin and push URL during target-attached
preparation.
