---
created: 2026-09-10
status: done
requirements:
  - REQ-CI-PR-DOCS-001
  - REQ-CI-PR-DOCS-002
  - REQ-CI-PR-DOCS-003
system_design:
  - ../../specs/ci/system-design/pull-request-documentation-coverage.md
legacy_specs: []
---

# Implementation plan: PR documentation coverage

## Overview

Build a deterministic coverage validator, integrate PR status reporting, then add merge-queue evaluation and contributor guidance.
The CI system owns this package because it controls repository checks and label exceptions.

## Scope

In scope: narrow exemptions, linked artifact validation, the exact `no-docs-allow` override, current-revision statuses, queue compatibility, and actionable summaries.

Out of scope: AI classification, application UI, automatic label management, publishing a PR, and live ruleset mutations.
Public user documentation remains governed by the existing contribution checklist.

## Technical approach

Use the proposed `.github/scripts/pr-docs.cjs` module for pure policy functions and bounded GitHub reads.
Use `.github/workflows/pr-docs.yml` for base-controlled execution and the `PR documentation coverage` commit status.
Reuse the existing Node runtime from GitHub-hosted runners and the built-in test runner; no pnpm dependency installation is needed.
Register Node and workflow contract tests in `.github/workflows/lint-action-pinning.yml`.

Existing patterns: `.github/workflows/pr-size-label.yml` for metadata pagination,
`.github/scripts/pr-size-label-workflow-contract_test.py` for workflow checks, and
`docs/ci-merge-queue.md` for required-check rollout constraints.

## Tests

All named test suites below are proposed files.

| Criteria | Evidence |
| --- | --- |
| AC-CI-PR-DOCS-001.2 through .6 | `pr-docs.test.cjs`: classification and artifact graph fixtures |
| AC-CI-PR-DOCS-001.1; AC-CI-PR-DOCS-002.1 through .4 | Fake GitHub adapter exercises draft/fork events and label transitions |
| AC-CI-PR-DOCS-003.1 through .3 | API failures, pagination caps, stale head/label reads, path and content restrictions |
| AC-CI-PR-DOCS-003.4 through .5 | Per-member queue evaluation, mismatched queue boundaries, override removal, and rollout checklist |

Use a representative #3137 fixture: `fix(worktree)` with runtime recovery paths and no artifacts fails.
Do not fetch the live PR in unit tests.

## End-to-end evidence

No application browser flow changes. The workflow contract and mocked API tests run locally.
After deployment, record real GitHub run URLs for an uncovered PR, a covered PR, an exempt PR, a fork, override addition/removal without pushes, and a merge group.
Verify one member's documents or override cannot satisfy another member.
Live smoke tests and required-check activation are deployment steps, not completed local evidence.

## Work orders

- [x] [Task 01: Validate documentation coverage](task-01-coverage-validator.md)
- [x] [Task 02: Report pull request coverage](task-02-pr-workflow.md)
- [x] [Task 03: Support merge queue coverage](task-03-queue-coverage.md)

Execute sequentially. No subagents are authorized.

## Verification results

Design validation on 2026-09-10:

- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/decisions docs/plans/pr-documentation-coverage`: passed.
- `git status --short -- docs/plans/pr-documentation-coverage`: confirmed the new package is present and untracked.

Implementation is complete. Live GitHub smoke-test evidence and required-check
activation remain deployment steps and are not claimed by this change.

Implementation verification on 2026-09-10:

- `node --test .github/scripts/pr-docs.test.cjs`: 25 tests passed.
- `python3 .github/scripts/pr-docs-workflow-contract_test.py`: 5 tests passed.
- `python3 .github/scripts/lint-action-pinning_test.py`: 9 tests passed.
- `python3 .github/scripts/lint-action-pinning.py`: 24 workflows passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `zizmor .github/workflows/pr-docs.yml`: no findings.
- `git diff --check`: passed.

Repository-wide `zizmor .github/workflows` still reports existing findings in
unrelated workflows; the new workflow has no reported findings.

## Risks

- Conservative defaults require labels for some small fixes, refactors, and dependency manifest changes.
- Structural validation cannot establish semantic relevance or planning chronology.
- Merge queue boundary resolution needs real payload validation before mandatory rollout.
- GitHub label/status publication is eventually consistent.
- Adding a required status is an administrator operation separate from merging workflow files.

## References

- [Requirements](../../specs/ci/requirements/pull-request-documentation-coverage.md)
- [System design](../../specs/ci/system-design/pull-request-documentation-coverage.md)
- [Decision](../../decisions/2026-09-10-pr-documentation-coverage.md)
