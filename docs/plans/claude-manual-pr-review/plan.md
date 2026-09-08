---
spec: ../../specs/integrations/requirements/claude-fork-review-allowlist.md
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

The workflow keeps the default branch at the trusted workspace root and does
not check out pull request content. For an explicit `@claude review` comment, a
constrained review action reads the exact current diff through GitHub's
read-only PR commands, explores the trusted default branch for surrounding
context, and reads a complete current-head PR file through a bound GET-only
helper when the diff is insufficient. Lightweight workflow and helper tests
pin the trust boundary.

## Scope

- Update `.github/workflows/claude.yml` so the default branch remains at the
  workspace root and no untrusted pull request checkout occurs.
- Disable credential persistence for the trusted checkout and constrain manual
  reviews to read and comment tools.
- Preserve generic Claude mention behavior on the trusted checkout.
- Extend `.github/scripts/claude-code-review-workflow-contract_test.py` with
  the regression contract for manual PR mentions and ordinary issue mentions.
- Add `.github/scripts/claude-read-pr-file.py` and its behavior tests for
  constrained complete-file access at the event PR's current head.
- Record the repaired behavior in the existing fork-review allowlist spec.

## Non-goals

- Change the open-only automatic review policy or the fork allowlist policy.
- Change Claude action pinning, OAuth/OIDC, job permissions, or existing generic
  Claude prompts. The manual review path adds a dedicated constrained prompt.
- Run untrusted pull-request code as part of the checkout; the workflow still
  only provides Claude's existing read-oriented review context.

## Approach

### Trusted checkout and PR access

Use a trusted root plus constrained GitHub access:

- Always check out the trusted default branch at the workspace root.
- Do not check out the pull request head in the privileged workflow.
- For a top-level pull request comment containing `@claude review`, use an
  explicit review-only prompt with trusted local read tools, `gh pr diff`,
  `gh pr view`, the bound PR-file helper, and comment tools as its complete
  allowlist.
- Bind the helper to the event PR number, resolve its current head SHA with a
  GET, and permit one normalized, regular UTF-8 file of at most 256 KiB per
  invocation. Do not expose generic GitHub API or arbitrary interpreter access.
- Keep other Claude mentions on the generic trusted-root path.

The current diff is resolved by GitHub and includes newly added files. No
workflow step checks out or executes pull request code. See
ADR-2026-07-31-isolate-manual-pr-review-content.

### Regression contract

Add raw-workflow contract assertions before changing the workflow. The tests
must prove that the constrained `gh pr diff` and `gh pr view` path targets the
issue's PR number, no PR content is checked out, and other mentions still use
the default checkout. Existing tests continue to pin the open-only automatic
review and allowlist behavior.

## Tasks

- [x] [task-01-pr-head-checkout](task-01-pr-head-checkout.md)

## Verification

```bash
rtk python3 .github/scripts/claude-code-review-workflow-contract_test.py
rtk python3 .github/scripts/claude-read-pr-file_test.py
rtk python3 .github/scripts/lint-action-pinning_test.py
rtk python3 .github/scripts/lint-action-pinning.py
rtk zizmor .github/workflows/claude.yml
rtk python3 -m py_compile .github/scripts/claude-read-pr-file.py \
  .github/scripts/claude-read-pr-file_test.py
rtk git diff --check
```

Both remediation commits ran the active pre-commit and commit-msg hooks:
Harness file lint and Conventional Commits validation passed; Go, frontend, and
format hooks skipped because no applicable files changed. Public documentation
under `docs/public/**` is unaffected because this is an internal CI workflow
repair.

## Risks and rollback

The failure mode is specific to PR-only files, while the security boundary is
specific to privileged comment-triggered workflows. Reading the diff through
constrained GitHub commands keeps ordinary mentions and trusted project
configuration independent of untrusted content. If the action's upstream
event handling changes, rollback is a single workflow revert; automatic
opened-only reviews remain independent in `claude-code-review.yml`.
