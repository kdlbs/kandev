---
id: "06-exempt-architecture-lint-tooling"
title: "Exempt architecture-lint tooling"
status: done
wave: 6
depends_on:
  - "05-exempt-ci-paths"
plan: "plan.md"
requirements:
  - REQ-CI-PR-DOCS-001
  - REQ-CI-PR-DOCS-003
acceptance_criteria:
  - AC-CI-PR-DOCS-001.9
  - AC-CI-PR-DOCS-003.4
system_design:
  - ../../specs/ci/system-design/pull-request-documentation-coverage.md
---

# Task 06: Exempt architecture-lint tooling

## Summary

Exempt the architecture-lint implementation, focused tests, reviewed
configuration, and confirmed entrypoints from PR delivery-package coverage.
Keep unrelated and mixed changes subject to the existing policy.

## In scope

- Exempt `scripts/architecture_lint/**`, `scripts/architecture_lint_tests/**`,
  and `config/architecture-lint/**` with directory-boundary-safe matching.
- Exempt the exact files `scripts/lint-architecture.py` and
  `scripts/lint-architecture.test.py`.
- Add classifier and evaluator fixtures for allowed paths, mixed changes,
  lookalikes, renames, and merge-group member independence.
- Reconcile the CI requirement, system design, policy decision, and plan.

## Out of scope

- Exempting all of `scripts/`, `config/`, or `.github/`.
- Changing work-order validation, the `no-docs-allow` label, trusted-base
  execution, status publishing, merge-group boundaries, or repository rulesets.
- Changing architecture rules, baselines, or product behavior.

## Acceptance

- The named directories and exact entrypoints, alone or with already exempt
  paths, pass without loading a delivery package; lookalikes stay covered.
- An unrelated or application path in the same pull request still requires
  coverage, including when a non-exempt file is renamed into an exempt path.
- An architecture-lint-only merge-group member does not exempt another
  member's application changes.

## Verification

Run from the repository root:

```bash
node --test .github/scripts/pr-docs.test.cjs
python3 .github/scripts/pr-docs-workflow-contract_test.py
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `.github/scripts/pr-docs.cjs`
- `.github/scripts/pr-docs.test.cjs`
- `docs/specs/ci/requirements/pull-request-documentation-coverage.md`
- `docs/specs/ci/system-design/pull-request-documentation-coverage.md`
- `docs/decisions/2026-09-10-pr-documentation-coverage.md`
- `docs/plans/pr-documentation-coverage/plan.md`

## Dependencies

Task 05. The existing classifier and workflow remain the implementation
boundary.

## Risks

- A broad prefix could exempt unrelated tools or configuration. Directory
  boundaries and lookalike fixtures constrain the list.
- A mixed pull request or rename could hide product code. The classifier must
  keep every old and new path, and merge-group evaluation must remain per member.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ci/requirements/pull-request-documentation-coverage.md)
- [System design](../../specs/ci/system-design/pull-request-documentation-coverage.md)
- [Plan](plan.md)
- [CI policy decision](../../decisions/2026-09-10-pr-documentation-coverage.md)
- [Architecture lint decision](../../decisions/2026-08-01-architecture-lint-budgets.md)
- `.github/AGENTS.md` and the existing validator tests.

## Results

The classifier exempts only the three named architecture-lint directories and
two confirmed entrypoints. Mixed paths, unrelated scripts and configuration,
lookalikes, renames, and merge-group members retain the normal coverage rules.

Verification:

- `node --test .github/scripts/pr-docs.test.cjs`: 87 tests passed.
- `python3 .github/scripts/pr-docs-workflow-contract_test.py`: 7 tests passed.
- `python3 scripts/list-docs.py validate`: 309 decisions and 1183
  specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- A read-only fixture from PR #2234 head
  `ada835c937ed6e57822ba5b8a7fa7239be0834fa` classified all 21 changed paths
  as exempt. GitHub reported the PR as merged during this check. This local
  reproduction does not publish a GitHub status.
