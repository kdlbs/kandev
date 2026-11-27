---
id: "09-skip-go-test-only-e2e"
title: "Avoid application E2E for independent Go test changes"
status: done
wave: 4
depends_on: ["08-reduce-scheduling-stages"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-005
  - REQ-PLATFORM-CI-PERFORMANCE-006
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-005.1
  - AC-PLATFORM-CI-PERFORMANCE-005.4
  - AC-PLATFORM-CI-PERFORMANCE-006.1
  - AC-PLATFORM-CI-PERFORMANCE-006.2
  - AC-PLATFORM-CI-PERFORMANCE-006.3
  - AC-PLATFORM-CI-PERFORMANCE-006.4
system_design:
  - ../../specs/platform/system-design/ci-performance.md
---

# Task 09: Avoid application E2E for independent Go test changes

## Summary

Add a PR-only exemption for verified application-independent Go unit-test changes. Keep backend unit/static checks and the required E2E conclusion.

## In scope

- Audit build inputs, go:embed patterns, scripts, and E2E fixtures for dependencies on `_test.go` files; exclude dependent paths.
- Classify a NUL-delimited name/status and mode-aware diff. Accept only regular `apps/backend/**/*_test.go` paths plus existing documentation exclusions.
- Keep full selection for unknown/mixed changes, either side of a production rename, missing bases, symlinks, submodules, malformed input, and non-PR events.

## Out of scope

Frontend test-only exemptions, changed-test-only unit execution, branch ruleset changes, and skipping main or merge-group verification.

## Acceptance

- The three-file PR 4349 fixture is exempt only after the dependency audit; its backend checks still run.
- An explicit classification matrix covers all file statuses, both rename directions, path modes, mixed runtime changes, workflow/shared inputs, unknown inputs, and non-PR events.
- A classifier failure runs the full E2E path; an explicit exemption reports success through the required check with a distinct skip reason.

## Verification

Run from the repository root. Install workspace dependencies once in a fresh worktree.
New test and helper paths below are implementation deliverables.

```bash
python3 .github/scripts/ci-test-only-changes_test.py
python3 .github/scripts/changed-paths_test.py
python3 .github/scripts/e2e-tests-workflow-contract_test.py
python3 .github/scripts/backend-tests-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
actionlint .github/workflows/e2e-tests.yml
zizmor .github/workflows
git diff --check
```

## Files likely touched

- `.github/scripts/ci-test-only-changes.py (new)`
- `.github/scripts/ci-test-only-changes_test.py (new)`
- `.github/workflows/e2e-tests.yml`
- `.github/scripts/e2e-tests-workflow-contract_test.py`
- `docs/ci-merge-queue.md`

## Dependencies

08-reduce-scheduling-stages.

## Risks

A test filename can still be an embedded or copied fixture. File naming alone is insufficient evidence of build independence.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/ci-performance.md), IDs in frontmatter.
- [System design](../../specs/platform/system-design/ci-performance.md), October continuation.
- [October evidence](evidence-2026-10-08.md).
- [Plan](plan.md), acceptance gates and preserved companion statuses.

## Results

Implemented the mode-aware raw-diff selector and eight regression tests.
Only potential Go-test exemptions inspect embedded inputs; ordinary application
changes return immediately after the raw diff classification.
Unknown PR bases run E2E; unknown classifier inputs retain full selection.
Audited production Go embed directives, backend build recipes, and E2E source
consumers. Existing E2E global setup already excludes ordinary Go test sources
from production-artifact freshness. The selector additionally checks embedded
paths at both revisions and rejects fixture/data paths.
The actual PR 4349 base/head range selects run=false with reason independent-go-tests.
Backend selection, merge-group, push, and manual policy remain unchanged.
Classifier, gate, path-filter, workflow, pinning, and Actionlint checks pass.
