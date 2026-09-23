---
id: "01-attachment-identity"
title: "Repair PR attachment identity validation"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
acceptance_criteria:
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.11
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.15
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.16
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.17
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.18
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.19
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
---

# Task 01: Repair PR attachment identity validation

## Summary

Accept a valid fork PR when the ordinary task attachment identifies its target
repository. Preserve exact identity checks for explicit bindings and fork-attached
checkouts. Resolve from existing metadata so unprepared failed tasks can retry.

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
- Add `TestTargetAttachedForkPRBasePreparationEndToEnd` beside existing real Git
  integration coverage. Use separate fork/upstream repositories with different
  same-named base commits. Publish the fork head under upstream `refs/pull/N/head`.
  Prove target base, PR head, and unchanged origin/push configuration.
- Repeat resolution for an unprepared task to represent retry. Include a mixed
  two-repository preparation where one invalid binding blocks the whole launch.

## Out of scope

New persistence, browser payload changes, contribution push authorization,
automatic repair of live tasks, and PR-association lifecycle changes.

## Acceptance

1. The ordinary upstream-attached fixture fails before the fix and succeeds
   after it, with or without a completed linked association.
2. Explicit binding mismatches and ambiguous identities keep their existing
   rejection or stored-qualified-target behavior. No unrelated base is applied.
3. Real Git preparation uses the correct head and target commits. Retry and
   mixed-repository checks preserve cancellation, fallback, and push invariants.

## Verification

Run from the repository root. Record RED before production changes, then GREEN.

```bash
(cd apps/backend && go test ./internal/orchestrator/executor -run '^TestResolveTaskRepoInfo_TargetAttachedForkPRWithoutContribution$' -count=1 -v)
(cd apps/backend && go test ./internal/backendapp -run '^TestTargetAttachedForkPRBasePreparationEndToEnd$' -count=1 -v)
(cd apps/backend && go test ./internal/orchestrator/executor ./internal/backendapp ./internal/worktree -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The first two test names are planned additions. The final package run covers
existing same-repository, explicit contribution, and qualified-base regressions.

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_pr_base_identity_test.go`
- `apps/backend/internal/backendapp/pr_base_resolver_test.go`
- `apps/backend/internal/backendapp/pr_base_integration_test.go`
- `apps/backend/internal/backendapp/orchestrator.go` only if resolver validation needs correction.
- A sibling integration fixture file if existing files exceed lint limits.

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
- `executor_pr_base_identity_test.go` and `backendapp/pr_base_integration_test.go`.
- [Comparison target ADR](../../decisions/2026-08-19-repository-qualified-comparison-targets.md).

## Results

Pending. Record expected RED, final commands, counts, and any fixture limitations.
