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

1. A PR `issue_comment` checkout references
   `refs/pull/${{ github.event.issue.number }}/head`.
2. PR review-comment and submitted-review events use their pull request number
   to check out the reviewed head.
3. A non-PR issue mention retains the default-branch checkout.
4. Existing tests still prove automatic review runs only on opening and the
   fork allowlist remains fail-closed.

## Implementation steps

1. Add the workflow contract assertions and run them to demonstrate the
   current workflow fails the new PR-head requirement.
2. Split the unconditional checkout into conditional PR-head and default-branch
   checkout steps, using GitHub-provided pull request refs.
3. Re-run the workflow contract and action-pinning checks.
4. Mark this task complete with the exact command results.

## Acceptance criteria

- A manual PR review no longer hashes a PR-added file from a checkout of
  `main`.
- Non-PR issue mentions retain their existing behavior.
- The workflow remains action-pinning compliant and has no whitespace errors.

## Results

- RED: `rtk python3 .github/scripts/claude-code-review-workflow-contract_test.py`
  failed because the generic Claude workflow had no pull-request-head checkout.
- GREEN: the same contract suite passed 6 tests after conditional pull request
  and default-branch checkouts were added.
- `rtk python3 .github/scripts/lint-action-pinning_test.py` passed 9 tests.
- `rtk python3 .github/scripts/lint-action-pinning.py` confirmed all 17
  workflow files use SHA-pinned action refs.
- `rtk git diff --check` passed.
