---
id: "01-pr-head-checkout"
title: "Check out the PR head for manual Claude reviews"
status: completed
spec: "../../specs/claude-fork-review-allowlist/spec.md"
plan: "plan.md"
depends_on: []
---

# Task 01: Check out the PR head for manual Claude reviews

## Objective

Ensure an explicit `@claude review` can inspect files that exist only in the
current pull request, without changing the checkout used for normal issues.

## Files

- `.github/workflows/claude.yml`
- `.github/scripts/claude-code-review-workflow-contract_test.py`

## TDD cases

1. The trusted default branch remains at the workspace root.
2. A PR `issue_comment` containing `@claude review` checks out
   `refs/pull/${{ github.event.issue.number }}/head` under `pr-head/` without
   persisted credentials.
3. The manual review receives `pr-head/` through `--add-dir` and cannot edit,
   push, or fetch arbitrary network content.
4. Other Claude mentions retain the generic trusted-root behavior.
5. Existing tests still prove automatic review runs only on opening and the
   fork allowlist remains fail-closed.

## Implementation steps

1. Add the workflow contract assertions and run them to demonstrate the
   current workflow fails the new PR-head requirement.
2. Keep the default checkout at the trusted root, isolate the PR head under
   `pr-head/`, and add a constrained manual-review action.
3. Re-run the workflow contract and action-pinning checks.
4. Mark this task complete with the exact command results.

## Acceptance criteria

- A manual PR review can read files added only on the pull request head.
- Untrusted PR content does not replace the trusted workspace root, checkout
  credentials are not persisted, and review tools are read-only.
- Other Claude mentions retain their existing behavior.
- The workflow remains action-pinning compliant and has no whitespace errors.

## Results

- RED: `rtk python3 .github/scripts/claude-code-review-workflow-contract_test.py`
  failed because the generic Claude workflow had no pull-request-head checkout.
- GREEN: the initial contract suite passed 6 tests after conditional pull
  request and default-branch checkouts were added.
- Review remediation RED: the expanded contract suite failed 3 tests because
  the PR head replaced the trusted root, credentials were persisted, and the
  manual review used unrestricted mention mode.
- Review remediation GREEN: the expanded contract suite passed 7 tests after
  isolating `pr-head/`, disabling checkout credential persistence, and adding
  an explicit review-only prompt and tool policy.
- `rtk python3 .github/scripts/lint-action-pinning_test.py` passed 9 tests.
- `rtk python3 .github/scripts/lint-action-pinning.py` confirmed all 17
  workflow files use SHA-pinned action refs.
- `zizmor .github/workflows/claude.yml` reported no findings.
- `rtk git diff --check` passed.
