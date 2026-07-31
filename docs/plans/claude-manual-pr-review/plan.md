---
spec: ../../specs/claude-fork-review-allowlist/spec.md
status: completed
created: 2026-07-31
---

# Manual Claude PR Review Checkout

## Summary

Restore a working explicit `@claude review` path after the open-only automatic
review policy. The generic Claude mention workflow currently checks out the
default branch for every event. For a pull request comment this omits files
added only by the pull request, causing the Claude action's PR-diff fetch to
fail before it can perform a review.

The workflow will check out the current pull request head for pull-request
comment and review events, while preserving the default-branch checkout for
ordinary issue mentions. A lightweight workflow contract test will pin that
distinction.

## Scope

- Update `.github/workflows/claude.yml` so each PR-backed event type checks out
  its PR head before the Claude action runs.
- Preserve the default-branch checkout for `issues` and issue comments that
  are not pull requests.
- Extend `.github/scripts/claude-code-review-workflow-contract_test.py` with
  the regression contract for manual PR mentions and ordinary issue mentions.
- Record the repaired behavior in the existing fork-review allowlist spec.

## Non-goals

- Change the open-only automatic review policy or the fork allowlist policy.
- Change Claude action pinning, OAuth/OIDC, action permissions, or prompts.
- Run untrusted pull-request code as part of the checkout; the workflow still
  only provides Claude's existing read-oriented review context.

## Approach

### Conditional checkout

Replace the unconditional checkout step with mutually exclusive checkout
steps:

- For `issue_comment` events on a pull request, fetch
  `refs/pull/${{ github.event.issue.number }}/head`.
- For `pull_request_review_comment` and `pull_request_review` events, fetch
  `refs/pull/${{ github.event.pull_request.number }}/head`.
- For ordinary issue mentions, retain the existing default-branch checkout.

The PR refs are resolved by GitHub rather than using a contributor-controlled
branch name. The job remains triggered by an explicit `@claude` mention and
does not add any command that executes repository code.

### Regression contract

Add raw-workflow contract assertions before changing the workflow. The tests
must prove that a PR issue-comment checkout uses the issue's PR number and
that non-PR issue mentions still use the default checkout. Existing tests
continue to pin the open-only automatic review and allowlist behavior.

## Tasks

- [x] [task-01-pr-head-checkout](task-01-pr-head-checkout.md)

## Verification

```bash
rtk python3 .github/scripts/claude-code-review-workflow-contract_test.py
rtk python3 .github/scripts/lint-action-pinning_test.py
rtk python3 .github/scripts/lint-action-pinning.py
git diff --check
```

## Risks and rollback

The failure mode is specific to PR-only files. Limiting the PR-head checkout
to PR-backed events prevents an ordinary issue mention from acquiring a PR
ref. If the action's upstream event handling changes, rollback is a single
workflow revert; automatic opened-only reviews remain independent in
`claude-code-review.yml`.
