---
id: "01-pr-head-checkout"
title: "Review the current PR diff without checkout"
status: completed
spec: "../../specs/integrations/requirements/claude-fork-review-allowlist.md"
plan: "plan.md"
depends_on: []
---

# Task 01: Review the current PR diff without checkout

## Objective

Ensure an explicit `@claude review` can inspect the exact current pull request
diff, including newly added files, without checking out untrusted PR content.

## Files

- `.github/workflows/claude.yml`
- `.github/scripts/claude-code-review-workflow-contract_test.py`
- `.github/scripts/claude-read-pr-file.py`
- `.github/scripts/claude-read-pr-file_test.py`

## TDD cases

1. The trusted default branch remains at the workspace root.
2. A PR `issue_comment` containing `@claude review` does not check out untrusted
   pull request content.
3. The manual review reads the current diff with constrained `gh pr` commands
   and cannot edit, push, or fetch arbitrary network content.
4. Other Claude mentions retain the generic trusted-root behavior.
5. Existing tests still prove automatic review runs only on opening and the
   fork allowlist remains fail-closed.
6. Claude can explore trusted default-branch files and fetch a complete file
   only from the workflow-bound PR head through a GET-only helper.
7. The helper rejects traversal, non-files, oversized content, and non-UTF-8
   content before returning PR data to the reviewer.

## Implementation steps

1. Add the workflow contract assertions and run them to demonstrate the
   current workflow fails the new PR-head requirement.
2. Keep the default checkout at the trusted root and add a constrained manual
   review action that reads the current diff through GitHub.
3. Re-run the workflow contract and action-pinning checks.
4. Add behavior tests and the constrained PR-file helper for semantic review.
5. Mark this task complete with the exact command results.

## Acceptance criteria

- A manual PR review can read files added only on the pull request head through
  the current GitHub diff and can retrieve a complete changed text file when
  semantic review requires more than a diff hunk.
- Pull request inspection commands are read-only. Posting review comments is
  the only allowed write; repository edits, pushes, PR checkout, and arbitrary
  network access remain prohibited.
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
- CodeQL remediation RED: CodeQL still rejected the isolated `pr-head/`
  checkout because untrusted content flowed into a privileged action.
- CodeQL remediation GREEN: the contract suite passed 7 tests and `zizmor`
  reported no findings after removing the untrusted checkout and using the
  constrained current-diff commands.
- Permission-mode RED: the workflow contract failed because the manual review
  did not explicitly select headless `dontAsk` mode.
- Permission-mode GREEN: the contract suite passed 7 tests after unapproved
  tool requests were configured to fail closed.
- Semantic-access RED: the workflow contract failed because the event PR was
  not bound for complete-file reads, and all 3 initial helper behavior tests
  failed because no constrained reader existed.
- Semantic-access GREEN: the workflow contract passed 7 tests and the helper
  suite passed 6 tests after enabling trusted local reads and the bound,
  GET-only current-head file reader.
- `rtk python3 .github/scripts/lint-action-pinning_test.py` passed 9 tests.
- `rtk python3 .github/scripts/lint-action-pinning.py` confirmed all 17
  workflow files use SHA-pinned action refs.
- `zizmor .github/workflows/claude.yml` reported no findings.
- Python bytecode compilation passed for the helper and its tests.
- `rtk git diff --check` passed.
- Active pre-commit hooks passed Harness file lint on commits `b03601a8e` and
  `07102d615`; inapplicable Go, frontend, and formatting hooks skipped. The
  active commit-msg hook passed Conventional Commits validation.
- Reviewed public documentation impact: no `docs/public/**` change is needed
  for this internal CI workflow repair.
