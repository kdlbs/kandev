---
spec: ../../specs/claude-fork-review-allowlist/spec.md
status: completed
created: 2026-07-31
---

# Manual Claude PR Review Checkout

## Summary

Restore a working explicit `@claude review` path after the open-only automatic
review policy. The generic Claude mention workflow currently checks out the
default branch for every event. For a pull request comment this leaves files
added only by the pull request unavailable to the reviewer and can end the run
without a useful review.

The workflow keeps the default branch at the trusted workspace root and checks
out the current pull request head under `pr-head/` only for an explicit
`@claude review` comment. A constrained review action receives that directory
as additional read-only context. A lightweight workflow contract test pins the
trust boundary.

## Scope

- Update `.github/workflows/claude.yml` so the default branch remains at the
  workspace root and explicit manual reviews isolate the PR head in a subtree.
- Disable credential persistence for both checkouts and constrain manual
  reviews to read and comment tools.
- Preserve generic Claude mention behavior on the trusted checkout.
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

Use a trusted root plus isolated review subtree:

- Always check out the trusted default branch at the workspace root.
- For a top-level pull request comment containing `@claude review`, fetch
  `refs/pull/${{ github.event.issue.number }}/head` under `pr-head/` without
  persisting checkout credentials.
- Pass `pr-head/` through `--add-dir` to an explicit review-only prompt with a
  constrained tool allowlist.
- Keep other Claude mentions on the generic trusted-root path.

The PR ref is resolved by GitHub rather than using a contributor-controlled
branch name. No workflow step executes repository code from the isolated
checkout. See ADR-2026-07-31-isolate-manual-pr-review-content.

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
zizmor .github/workflows/claude.yml
git diff --check
```

## Risks and rollback

The failure mode is specific to PR-only files, while the security boundary is
specific to privileged comment-triggered workflows. Isolating the PR checkout
keeps ordinary mentions and trusted project configuration independent of
untrusted content. If the action's upstream event handling changes, rollback
is a single workflow revert; automatic opened-only reviews remain independent
in `claude-code-review.yml`.
