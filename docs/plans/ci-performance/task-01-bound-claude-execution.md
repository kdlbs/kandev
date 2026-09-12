---
id: "01-bound-claude-execution"
title: "Bound Claude execution"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-001
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-001.1
  - AC-PLATFORM-CI-PERFORMANCE-001.2
system_design:
  - ../../specs/platform/system-design/ci-performance.md
---

# Task 01: Bound Claude execution

## Summary

Set a 30-minute execution budget on all three Claude execution jobs.
Protect the limits with the existing workflow contract suite.

## In scope

- Job timeout fields in both Claude workflows.
- Regression assertions for budgets, existing authorization, and failure behavior.
- A short internal note that approval expiration and execution timeout are different outcomes.

## Out of scope

- Approval, permissions, prompts, models, action upgrades, turn limits, and automatic retries.

## Acceptance

- Both automatic review jobs and the interactive job declare `timeout-minutes: 30`.
- Timeout cancellation cannot become a successful review through fallback logic.
- Trigger, approval, permission, checkout, and runner contracts remain unchanged.

## Verification

Run commands from the repository root. Install workspace dependencies first in a fresh worktree.

```bash
python3 .github/scripts/claude-code-review-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
actionlint .github/workflows/claude-code-review.yml .github/workflows/claude.yml
git diff --check
```

## Files likely touched

- `.github/workflows/claude-code-review.yml`
- `.github/workflows/claude.yml`
- `.github/scripts/claude-code-review-workflow-contract_test.py`
- `docs/ci-merge-queue.md`

## Dependencies

None.

## Risks

GitHub owns cancellation latency. Do not claim this change limits approval or queue waits.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/ci-performance.md), acceptance IDs in frontmatter.
- [System design](../../specs/platform/system-design/ci-performance.md), corresponding implementation boundary.
- [Plan](plan.md), baseline and companion-package status.
- Existing workflow contract tests under `.github/scripts/`.

## Results

Implemented 2026-09-12. Added `timeout-minutes: 30` to the same-repository
review, approved fork-review, and shared interactive Claude jobs. Added a
contract assertion for all three execution budgets and documented the
distinction between approval expiry, queue delay, and runner execution in
`docs/ci-merge-queue.md`. No trigger, permission, checkout, action, or success
fallback changed.

Validation:

- `python3 .github/scripts/claude-code-review-workflow-contract_test.py` passed (10 tests).
- `python3 .github/scripts/lint-action-pinning_test.py` passed (9 tests).
- `git diff --check` passed.
- `actionlint` is not installed in this workspace; the workflow syntax remains covered by the repository contract and pinning tests.
