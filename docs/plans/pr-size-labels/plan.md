---
created: 2026-09-09
status: done
requirements:
  - REQ-CI-PR-SIZE-001
system_design:
  - ../../specs/ci/system-design/pull-request-size-labels.md
legacy_specs: []
---

# Implementation Plan: Pull Request Size Labels

## Overview

Add one trusted workflow that maintains pull request size labels. One work order keeps the workflow, contract test, and CI registration in one reviewable change.

## Scope

### In scope

- Recalculate size labels for opened, reopened, and synchronized pull requests.
- Count the confirmed application and test file classes.
- Create missing `small`, `medium`, and `big` label definitions.
- Preserve unrelated labels and converge on one size label.
- Add focused workflow contract coverage and action-pinning coverage.

### Out of scope

- Changed-line or language-weighted scores.
- A manual override or settings surface.
- A deployment backfill for existing open pull requests.
- Changes to application code or existing review workflows.

## Technical approach

Create `.github/workflows/pr-size-label.yml` with the trusted `pull_request_target` event and per-pull-request serialization.

Use pinned `actions/github-script` for paginated changed-file reads and label mutations. Keep the classifier and limits explicit in the trusted inline script.

Create `.github/scripts/pr-size-label-workflow-contract_test.py`. Execute the classifier against representative paths and the seven confirmed pull request file-count totals.

Register the contract test in `.github/workflows/lint-action-pinning.yml`. Preserve its existing workflow checks and pinned action policy.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-CI-PR-SIZE-001.1`, `AC-CI-PR-SIZE-001.9` | Contract assertions for events, current API reads, and serialized concurrency. |
| `AC-CI-PR-SIZE-001.2` | Boundary cases for 0, 10, 11, 50, and 51 files. |
| `AC-CI-PR-SIZE-001.3`, `AC-CI-PR-SIZE-001.4` | Representative source, test, translation, documentation, workflow, script, generated, dependency, linter, settings, and asset paths. |
| `AC-CI-PR-SIZE-001.5`, `AC-CI-PR-SIZE-001.6` | Contract assertions for label creation, add-before-remove order, exact size names, and unrelated-label preservation. |
| `AC-CI-PR-SIZE-001.7` | Contract assertions for the 3,000-file limit and pre-mutation error path. |
| `AC-CI-PR-SIZE-001.8` | Contract assertions for base-controlled execution, least privilege, no checkout, and a pinned action. |

## E2E tests

The workflow contract test runs the complete classifier and mocked label flow. Browser Playwright tests do not apply to GitHub-owned pull request labels.

A post-merge pull request event supplies live GitHub evidence. This observation is not a local implementation gate.

## Work orders

- [x] [Task 01: Add pull request size workflow](task-01-add-pr-size-workflow.md)

## Verification results

- RED: the new contract test failed because the pull request size workflow did
  not exist yet.
- GREEN: `python3
  .github/scripts/pr-size-label-workflow-contract_test.py` passed, 8 tests.
- `python3 .github/scripts/lint-action-pinning_test.py` passed, 9 tests, and
  `python3 .github/scripts/lint-action-pinning.py` passed for 23 workflows.
- `zizmor .github/workflows/pr-size-label.yml` passed with no findings after
  the intentional `pull_request_target` audit was documented inline.
- `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check -- .github` passed.
- Review remediation narrowed script and tool-setting exclusions to explicit
  paths, added asset exclusions, fixed the step-summary row, and handled the
  duplicate-label creation race.
- Review remediation updated the concurrency and asset contracts across the
  system design, requirements, and ADR.
- Fixup verification: `python3
  .github/scripts/pr-size-label-workflow-contract_test.py` passed, 9 tests;
  action-pinning tests and linter passed; specification lint passed; targeted
  `zizmor` passed; and `git diff --check` passed.

## Risks

- GitHub limits the files endpoint to 3,000 entries. The workflow fails without mutations at that limit.
- The explicit source filter needs updates when the repository adds application languages or tooling directories.
- Existing open pull requests need a later qualifying event before they receive a size label.
