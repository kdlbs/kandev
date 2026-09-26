---
spec: docs/specs/integrations/requirements/claude-fork-review-allowlist.md
created: 2026-07-30
status: complete
---

# Implementation Plan: Claude Fork Review Allowlist

## Overview

The fork review job already authorizes trusted contributors at its job-level condition and forwards that authorization into `anthropics/claude-code-action`. Extend the workflow contract so the same allowlist also labels newly opened fork PRs with `safe-to-review` and `safe-to-test`, while the preview workflow recognizes the allowlist directly because label events emitted by `GITHUB_TOKEN` do not start another workflow run. Keep the change base-controlled and limited to existing fork trust gates.

## Confirmed root cause

PR #2072 is a cross-repository pull request authored and synchronized by `ClemDNL`. The repository variable is valid JSON and contains that login, which is why `claude-review-fork` started. Its log then showed an empty `ALLOWED_NON_WRITE_USERS`, resolved `ClemDNL` to repository permission `read`, and failed with `Actor does not have write permissions to the repository`.

## Workflow

- Add `.github/scripts/claude-code-review-workflow-contract_test.py` to assert that the fork job forwards its approved pull request author to the action's `allowed_non_write_users` input.
- Update `.github/workflows/claude-code-review.yml` to pass `github.event.pull_request.user.login` to the fork action.
- Update `.github/workflows/claude-code-review.yml` with a base-controlled, open-only labeling job that adds both approval labels for allowlisted fork authors.
- Update `.github/workflows/preview-env.yml` so `CLAUDE_REVIEW_ALLOWLIST` is a direct trusted fork-preview gate for all existing non-closed pull-request-target events.
- Add focused contract coverage for the labeling job and preview gate, and run both contract tests from `.github/workflows/lint-action-pinning.yml`.

## Tests

- **What:** An approved non-write fork author reaches Claude's internal permission bypass without evaluating the optional JSON allowlist again in the label-only path.
- **File:** `.github/scripts/claude-code-review-workflow-contract_test.py`
- **How:** A focused Python contract test reads the fork job and requires the exact `allowed_non_write_users` expression. It must fail before the workflow change and pass afterward.
- **What:** An allowlisted fork PR receives both trust labels from a base-controlled job only when it opens and is external to the repository.
- **File:** `.github/scripts/claude-code-review-workflow-contract_test.py`
- **How:** Assert the job gate, `issues: write` permission, and the `issues.addLabels` call with exactly `safe-to-review` and `safe-to-test`.
- **What:** The preview workflow authorizes allowlisted fork PRs without depending on a label event emitted by `GITHUB_TOKEN`.
- **File:** `.github/scripts/preview-env-workflow-contract_test.py`
- **How:** Assert the `deploy-fork` condition retains the existing `safe-to-test` and `PREVIEW_ENV_ALLOWLIST` paths and adds the `CLAUDE_REVIEW_ALLOWLIST` path.
- **Commands:**
  - `python3 .github/scripts/claude-code-review-workflow-contract_test.py`
  - `python3 .github/scripts/preview-env-workflow-contract_test.py`
  - `python3 .github/scripts/lint-action-pinning.py`

## Implementation

- [x] [Task 01: Forward the approved fork allowlist](task-01-forward-fork-allowlist.md)
- [x] [Task 02: Label allowlisted fork pull requests](task-02-label-allowlisted-fork-prs.md)

## Risks

- GitHub expression syntax is runtime-specific. The contract test pins the expected expression, while final proof still requires a new fork workflow run after the change reaches the base branch.
- Re-evaluating the JSON variable in the action input could break the independent label-only path when that optional variable is empty. Passing the actor already approved by the job gate avoids that dependency.
- `CLAUDE_REVIEW_ALLOWLIST` becomes a trusted preview-execution boundary as well as a review boundary; maintainers must treat its entries as authorized to run fork preview code with deployment credentials.
- The label job uses the repository `GITHUB_TOKEN`, so labels are status markers and must not be the only trigger for privileged downstream workflows.

## Verification Results

Task 01 results are retained in `task-01-forward-fork-allowlist.md`. Task 02 results are recorded in `task-02-label-allowlisted-fork-prs.md`: both workflow contract suites passed, all 18 workflow action references are SHA-pinned, and `git diff --check` passed. Live verification against a real allowlisted fork PR remains post-merge work.
