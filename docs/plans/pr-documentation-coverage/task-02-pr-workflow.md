---
id: "02-pr-workflow"
title: "Report pull request coverage"
status: done
wave: 2
depends_on:
  - "01-coverage-validator"
plan: "plan.md"
requirements:
  - REQ-CI-PR-DOCS-001
  - REQ-CI-PR-DOCS-002
  - REQ-CI-PR-DOCS-003
acceptance_criteria:
  - AC-CI-PR-DOCS-001.1
  - AC-CI-PR-DOCS-002.1
  - AC-CI-PR-DOCS-002.2
  - AC-CI-PR-DOCS-002.3
  - AC-CI-PR-DOCS-002.4
  - AC-CI-PR-DOCS-003.1
  - AC-CI-PR-DOCS-003.2
  - AC-CI-PR-DOCS-003.3
system_design:
  - ../../specs/ci/system-design/pull-request-documentation-coverage.md
---

# Task 02: Report pull request coverage

## Summary

Integrate the validator with bounded, revision-bound API reads and a trusted PR workflow. Add status summaries, label reevaluation, retry dispatch, and tests registered in the existing lint workflow.

## In scope

- `.github/workflows/pr-docs.yml`
- `.github/scripts/pr-docs.cjs`
- `.github/scripts/pr-docs.test.cjs`
- `.github/scripts/pr-docs-workflow-contract_test.py`
- `.github/workflows/lint-action-pinning.yml`
- `Makefile`

## Out of scope

Application code, unrelated workflow cleanup, live label changes, and live ruleset changes.

## Acceptance

- Open, draft, fork, push, edited, and label events publish the correct status on the evaluated head.
- Override addition passes and removal restores validation without a push; concurrent state changes cannot be silently accepted.
- Trusted execution, bounded reads, errors, parser limits, and test registration have passing contract coverage.

## Verification

Run from the repository root:

```bash
node --test .github/scripts/pr-docs.test.cjs
python3 .github/scripts/pr-docs-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
python3 .github/scripts/lint-action-pinning.py
zizmor .github/workflows
git diff --check
```

## Files likely touched

- `.github/workflows/pr-docs.yml`
- `.github/scripts/pr-docs.cjs`
- `.github/scripts/pr-docs.test.cjs`
- `.github/scripts/pr-docs-workflow-contract_test.py`
- `.github/workflows/lint-action-pinning.yml`

## Dependencies

Task 01.

## Risks

A pull_request_target native job check is not sufficient evidence on the PR head. Do not activate required checks at this intermediate task.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ci/requirements/pull-request-documentation-coverage.md)
- [System design](../../specs/ci/system-design/pull-request-documentation-coverage.md)
- [Plan](plan.md)
- [Decision](../../decisions/2026-09-10-pr-documentation-coverage.md)
- `.github/AGENTS.md` and existing PR size workflow contract tests.

## Results

Implemented the bounded GitHub API adapter, revision and label consistency
checks, status reporting, manual retry handling, trusted workflow, bounded
pending event queue, and workflow contract coverage. The validator suite is
registered in the lint workflow and the repository script test target. The
workflow uses only read access to pull requests and contents plus commit-status
writes.

Verification:

- `node --test .github/scripts/pr-docs.test.cjs`: 35 tests passed.
- `python3 .github/scripts/pr-docs-workflow-contract_test.py`: 5 tests passed.
- `python3 .github/scripts/lint-action-pinning_test.py`: 9 tests passed.
- `python3 .github/scripts/lint-action-pinning.py`: 24 workflows passed.
- `zizmor .github/workflows/pr-docs.yml`: no findings.
- `git diff --check`: passed.

The repository-wide `zizmor .github/workflows` command still reports existing
findings in unrelated workflows. No new finding was reported for `pr-docs.yml`.
