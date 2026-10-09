---
id: "03-remove-size-label-definition-preflights"
title: "Remove size-label definition preflights"
status: done
wave: 3
depends_on:
  - "02-consolidate-documentation-api-reads"
plan: "plan.md"
requirements:
  - REQ-CI-PR-SIZE-001
acceptance_criteria:
  - AC-CI-PR-SIZE-001.5
  - AC-CI-PR-SIZE-001.6
  - AC-CI-PR-SIZE-001.7
  - AC-CI-PR-SIZE-001.9
  - AC-CI-PR-SIZE-001.10
system_design:
  - ../../specs/ci/system-design/pull-request-size-labels.md
---

# Task 03: Remove size-label definition preflights

## Summary

Remove three unconditional repository label reads from each pull request size
calculation while preserving missing-label recovery and convergence.

## In scope

- `.github/workflows/pr-size-label.yml`
- `.github/scripts/pr-size-label-workflow-contract_test.py`
- This work order's implementation results

## Out of scope

Size thresholds, file classification, event routing, label colors and text,
other workflow labels, and documentation coverage.

## Acceptance

- Read current pull request labels once and perform no repository label-definition
  reads on a steady run whose target label is already present.
- Apply an absent target directly. After a narrowly identified missing-definition
  error, create only that definition and retry the apply.
- Preserve add-before-remove ordering, concurrent `already_exists` recovery,
  unrelated labels, least privilege, and trusted execution.

## Verification

Run from the repository root:

```bash
python3 .github/scripts/pr-size-label-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
python3 .github/scripts/lint-action-pinning.py
zizmor .github/workflows/pr-size-label.yml
git diff --check
```

## Files likely touched

- `.github/workflows/pr-size-label.yml`
- `.github/scripts/pr-size-label-workflow-contract_test.py`
- `docs/plans/github-api-resilience/task-03-remove-size-label-definition-preflights.md`

## Dependencies

Task 02. The source files are disjoint, but sequential execution keeps delivery
and live request-count evidence in one ordered package.

## Risks

Match missing-definition responses narrowly. A permission or validation error
must fail without attempting label creation.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ci/requirements/pull-request-size-labels.md)
- [System design](../../specs/ci/system-design/pull-request-size-labels.md)
- [Plan](plan.md)
- `.github/AGENTS.md`

## Results

Implemented lazy size-label provisioning in `.github/workflows/pr-size-label.yml`
and updated its contract tests.

- The workflow reads current pull request labels once and skips all repository
  label-definition reads when the target is already present.
- An absent target is applied directly. Only a narrowly identified missing
  definition response (`Label`/`name`/`invalid` for the exact target) creates
  the selected target, then retries the same apply.
- Concurrent `already_exists` creation is accepted before the retry. The
  existing add-before-remove order, unrelated labels, permissions, and trusted
  execution remain intact.

Verification:

- `python3 .github/scripts/pr-size-label-workflow-contract_test.py` (9 passed)
- `python3 .github/scripts/lint-action-pinning_test.py` (9 passed)
- `python3 .github/scripts/lint-action-pinning.py` (24 workflows passed)
- `zizmor .github/workflows/pr-size-label.yml` (no findings)
- `git diff --check` (passed)
