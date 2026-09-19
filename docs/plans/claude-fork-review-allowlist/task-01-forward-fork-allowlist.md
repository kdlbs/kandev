---
id: "01-forward-fork-allowlist"
title: "Forward the approved fork allowlist"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/claude-fork-review-allowlist.md"
---

# Task 01: Forward the approved fork allowlist

## Acceptance

- The fork Claude review action receives the job-approved pull request author through `allowed_non_write_users`.
- The job-authorized pull request author is passed to the pinned Claude action without re-evaluating the optional JSON repository variable.
- A focused contract test fails without the input, passes with it, and is run by the workflow-lint CI job.

## Verification

- `python3 .github/scripts/claude-code-review-workflow-contract_test.py`
- `python3 .github/scripts/lint-action-pinning.py`

## Files likely touched

- `.github/workflows/claude-code-review.yml`
- `.github/workflows/lint-action-pinning.yml`
- `.github/scripts/claude-code-review-workflow-contract_test.py`

## Dependencies

None.

## Parallelism

`sequential`; the test and both workflow changes form one contract.

## Inputs

- Spec scenarios in `docs/specs/integrations/requirements/claude-fork-review-allowlist.md`.
- Failed `claude-review-fork` job 90922436522 from workflow run 30557734099.
- Pinned action input contract: `allowed_non_write_users` accepts comma-separated usernames only when `github_token` is provided.

## Output contract

Report the workflow expression added, files changed, RED and GREEN test results, action-pinning lint result, residual need for a live fork workflow run, and task/plan status updates.
