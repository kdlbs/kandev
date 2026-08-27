---
id: "01-release-workflow"
title: "Repair the Stable release merge"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/release/requirements/release-pr-queue-bypass.md"
---

# Task 01: Repair the Stable release merge

## Acceptance

- The regression contract fails before the workflow change and passes afterward.
- A normal Stable release uses the administrator token only to merge one exact pull-request head without CI or queue enrollment.
- Tag signing selects the merge commit reported by GitHub only after that commit appears on `origin/main`.

## Verification

- `python3 .github/scripts/release-workflow-contract_test.py`
- `python3 .github/scripts/lint-action-pinning_test.py`
- `git diff --check -- .github/workflows/release.yml .github/scripts/release-workflow-contract_test.py`

## Files likely touched

- `.github/workflows/release.yml`
- `.github/scripts/release-workflow-contract_test.py`

## Dependencies

None.

## Parallelism

Sequential. The workflow and its contract test form one TDD unit.

## Inputs

- `docs/specs/release/requirements/release-pr-queue-bypass.md`
- `docs/decisions/2026-08-17-release-pr-ruleset-bypass.md`
- The `Create release PR + squash-merge` step in `.github/workflows/release.yml`.
- The protected `release` environment on the `prepare` job.

## Output contract

Report the Red and Green results, changed files, token boundary, and merge-commit guard. Update this task and `plan.md` in the same conversation.

## Results

- Red: `python3 .github/scripts/release-workflow-contract_test.py ReleaseWorkflowContractTest.test_normal_release_uses_admin_token_only_for_exact_head_merge` failed because the administrator-token merge step did not exist.
- Green: the same focused test passed after the workflow change.
- `python3 .github/scripts/release-workflow-contract_test.py` passed 30 tests.
- `python3 .github/scripts/lint-action-pinning_test.py` passed 9 tests.
- `git diff --check -- .github/workflows/release.yml .github/scripts/release-workflow-contract_test.py` passed.
- `RELEASE_PR_BYPASS_TOKEN` is available only to the privileged merge step. `GITHUB_TOKEN` creates the pull request and inspects its merged state.
- The workflow validates the exact pull-request head and checks out GitHub's reported merge commit before tag signing.
