---
id: "01-add-pr-size-workflow"
title: "Add pull request size workflow"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CI-PR-SIZE-001
acceptance_criteria:
  - AC-CI-PR-SIZE-001.1
  - AC-CI-PR-SIZE-001.2
  - AC-CI-PR-SIZE-001.3
  - AC-CI-PR-SIZE-001.4
  - AC-CI-PR-SIZE-001.5
  - AC-CI-PR-SIZE-001.6
  - AC-CI-PR-SIZE-001.7
  - AC-CI-PR-SIZE-001.8
  - AC-CI-PR-SIZE-001.9
system_design:
  - ../../specs/ci/system-design/pull-request-size-labels.md
---

# Task 01: Add Pull Request Size Workflow

## Summary

Add the trusted workflow that calculates and maintains one pull request size label. Add focused tests for classification, API behavior, and workflow security.

## In scope

- Add the `pull_request_target` workflow and per-pull-request concurrency.
- Implement the application-file filter and the 10/50 size limits.
- Create missing label definitions and converge on one size label.
- Add and register the workflow contract test.

## Out of scope

- Application source changes.
- Existing review workflow changes.
- A backfill operation for current open pull requests.
- Live mutation of repository labels during local development.

## Acceptance

- The classifier assigns all boundary cases and seven confirmed examples to the required labels.
- A successful run preserves unrelated labels and leaves exactly one size label.
- The workflow fails before mutations for incomplete file data and uses only trusted, pinned, least-privilege execution.

## Verification

```bash
python3 .github/scripts/pr-size-label-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
python3 .github/scripts/lint-action-pinning.py
zizmor .github/workflows/pr-size-label.yml
git diff --check -- .github
```

## Files likely touched

- `.github/workflows/pr-size-label.yml`
- `.github/scripts/pr-size-label-workflow-contract_test.py`
- `.github/workflows/lint-action-pinning.yml`

## Dependencies

None.

## Risks

- GitHub can return an incomplete changed-file list at its 3,000-file limit.
- Label mutations can partially complete before an API error. The next event or rerun converges on the current target.
- A new source language requires an explicit classifier update.

## Parallelism

`sequential`

## Inputs

- `REQ-CI-PR-SIZE-001` and its acceptance criteria.
- `docs/specs/ci/system-design/pull-request-size-labels.md`.
- `docs/decisions/2026-09-09-count-application-files-for-pr-size.md`.
- `.github/AGENTS.md` and the existing label workflow contract pattern.
- GitHub files and labels REST API contracts.

## Results

- Added the base-controlled `pull_request_target` workflow for opened,
  reopened, and synchronized pull requests with per-pull-request serialization.
- Added the explicit application-file classifier, fixed size boundaries, 3,000
  file completeness guard, repository label creation, and convergent label
  mutations that preserve unrelated labels.
- Added 9 workflow contract tests covering the classifier, examples, API
  limits, label lifecycle, concurrency, security, and lint registration.
- RED failed because the workflow was missing; GREEN passed after the workflow
  and lint registration were added.
- `python3 .github/scripts/pr-size-label-workflow-contract_test.py` passed.
- `python3 .github/scripts/lint-action-pinning_test.py` passed.
- `python3 .github/scripts/lint-action-pinning.py` passed for 23 workflows.
- `zizmor .github/workflows/pr-size-label.yml` passed with no findings.
- `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check -- .github` passed.
- Review remediation narrowed script and tool-setting exclusions to explicit
  paths, added asset exclusions, fixed the step-summary row, handled the
  duplicate-label creation race, and corrected the concurrency contract.
- Fixup verification passed: the contract test passed with 9 tests, action
  pinning tests and linter passed, specification lint passed, targeted
  `zizmor` passed, and `git diff --check` passed.
