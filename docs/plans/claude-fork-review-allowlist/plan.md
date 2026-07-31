---
spec: docs/specs/claude-fork-review-allowlist/spec.md
created: 2026-07-30
status: complete
---

# Implementation Plan: Claude Fork Review Allowlist

## Overview

The fork review job already authorizes trusted contributors at its job-level condition, but it does not forward that authorization into `anthropics/claude-code-action`. Add a focused workflow contract test first, then pass the already job-authorized pull request author to the action's `allowed_non_write_users` input.

## Confirmed root cause

PR #2072 is a cross-repository pull request authored and synchronized by `ClemDNL`. The repository variable is valid JSON and contains that login, which is why `claude-review-fork` started. Its log then showed an empty `ALLOWED_NON_WRITE_USERS`, resolved `ClemDNL` to repository permission `read`, and failed with `Actor does not have write permissions to the repository`.

## Workflow

- Add `.github/scripts/claude-code-review-workflow-contract_test.py` to assert that the fork job forwards its approved pull request author to the action's `allowed_non_write_users` input.
- Update `.github/workflows/claude-code-review.yml` to pass `github.event.pull_request.user.login` to the fork action.
- Update `.github/workflows/lint-action-pinning.yml` so workflow changes run the new contract test in CI.

## Tests

- **What:** An approved non-write fork author reaches Claude's internal permission bypass without evaluating the optional JSON allowlist again in the label-only path.
- **File:** `.github/scripts/claude-code-review-workflow-contract_test.py`
- **How:** A focused Python contract test reads the fork job and requires the exact `allowed_non_write_users` expression. It must fail before the workflow change and pass afterward.
- **Commands:**
  - `python3 .github/scripts/claude-code-review-workflow-contract_test.py`
  - `python3 .github/scripts/lint-action-pinning.py`

## Implementation

- [x] [Task 01: Forward the approved fork allowlist](task-01-forward-fork-allowlist.md)

## Risks

- GitHub expression syntax is runtime-specific. The contract test pins the expected expression, while final proof still requires a new fork workflow run after the change reaches the base branch.
- Re-evaluating the JSON variable in the action input could break the independent label-only path when that optional variable is empty. Passing the actor already approved by the job gate avoids that dependency.
