---
spec: docs/specs/release/requirements/release-pr-queue-bypass.md
created: 2026-08-17
status: complete
---

# Implementation Plan: Stable Release PR Queue Bypass

## Overview

The release workflow still requests an immediate branch deletion during a queued merge. GitHub rejects that option before the generated release pull request can merge.

The repair uses an organization-administrator personal access token for one privileged pull-request merge. It then binds tag creation to the merge commit reported by GitHub.

## Release workflow

### Regression contract

- Update `.github/scripts/release-workflow-contract_test.py` before the workflow change.
- Require the protected `RELEASE_PR_BYPASS_TOKEN` secret only in the privileged merge step.
- Require an explicit missing-token failure before the merge command.
- Require `GITHUB_TOKEN` for branch push and pull-request creation.
- Require the administrator token for `gh pr merge --admin` only.
- Require `GITHUB_TOKEN` for merged-state inspection.
- Reject `--delete-branch`, queue enrollment, polling, and a merge without `--match-head-commit`.
- Require the reported merge commit to exist on `origin/main` before tag creation.
- Require a valid lowercase pull-request title subject.

### Immediate privileged merge

- Update `.github/workflows/release.yml`.
- Read `RELEASE_PR_BYPASS_TOKEN` only in the normal-release merge step.
- Fail before the merge command when the secret is empty.
- Capture the release pull-request URL and head commit.
- Merge with `--admin`, `--squash`, and `--match-head-commit`.
- Remove `--delete-branch` and rely on repository branch cleanup.
- Read the merged state and merge commit through `GITHUB_TOKEN`.
- Fetch `main`, make sure that it contains the merge commit, and select that commit for signing.
- Keep the existing public-key revalidation after the merge.

## Release contract records

- Update `docs/public/release-process.md` as a how-to guide.
- Update `docs/ci-merge-queue.md` with the administrator-token exception.
- Update `.agents/skills/release/SKILL.md` with the bypass and configuration contract.
- Update `AGENTS.md` because its release guidance must name the bypass requirement.
- Set the repair spec and its index entry to `shipped` after the implementation checks pass.
- Keep ADR `2026-08-17-release-pr-ruleset-bypass` aligned with the final implementation.

## Tests

- **What:** Normal Stable releases expose the administrator token only to the privileged merge step.
  **File:** `.github/scripts/release-workflow-contract_test.py`.
  **How:** Assert the secret name, normal-release condition, missing-secret guard, and token isolation.
- **What:** Release pull requests bypass CI and the merge queue for one exact head.
  **File:** `.github/scripts/release-workflow-contract_test.py`.
  **How:** Assert `--admin`, `--squash`, and `--match-head-commit`. Reject `--delete-branch` and `--auto`.
- **What:** Tag signing uses the merge commit that GitHub reports.
  **File:** `.github/scripts/release-workflow-contract_test.py`.
  **How:** Assert merged-state parsing, ancestry validation against `origin/main`, and local selection of the reported commit.
- **What:** Excluded release modes cannot request the bypass token.
  **File:** `.github/scripts/release-workflow-contract_test.py`.
  **How:** Assert the normal-release condition on token creation, identity validation, and privileged merge steps.
- **What:** Maintainer documentation names the token scope, owner, expiration, rotation, ruleset, and protected environment contract.
  **Files:** `docs/public/release-process.md`, `.agents/skills/release/SKILL.md`, `AGENTS.md`, and `docs/ci-merge-queue.md`.
  **How:** Use the public-doc and harness-file validators.

## Verification results

- `python3 .github/scripts/release-workflow-contract_test.py` passed 31 tests.
- `python3 .github/scripts/lint-action-pinning_test.py` passed 9 tests.
- `node --test scripts/validate-public-docs.test.mjs` passed 61 tests.
- `node scripts/validate-public-docs.mjs` validated 41 pages.
- `python3 scripts/lint-harness-files.test.py` passed 19 tests.
- `python3 .github/scripts/lint-harness-files.py --all` passed 118 files.
- `pre-commit run harness-lint --files AGENTS.md .agents/skills/release/SKILL.md` passed.
- `git diff --check` passed.
- `actionlint` was not available in the local environment. The repository contract tests and commit hooks remain required.

## Implementation waves and parallel candidates

Wave 1 (sequential):

- [x] [task-01-release-workflow](task-01-release-workflow.md)

Wave 2 (sequential, depends on Wave 1):

- [x] [task-02-release-documentation](task-02-release-documentation.md)

Parallel delegation is not authorized. The tasks share the release contract and run sequentially in the primary conversation.

## Post-merge rollout

1. Create a fine-grained personal access token from an organization administrator account.
2. Select only `kdlbs/kandev` and grant `contents: write` permission.
3. Add the token as the `RELEASE_PR_BYPASS_TOKEN` secret in the `release` environment.
4. Record its owner and expiration date in the maintainers' credential inventory.
5. Dispatch a new normal Stable release for `v0.89.0`.
6. Make sure that the release PR merges immediately and the signed tag targets its merge commit.

The implementation delivery closes stale release PR #2751. The next release run removes and recreates its existing release branch.

## Open questions

None.
