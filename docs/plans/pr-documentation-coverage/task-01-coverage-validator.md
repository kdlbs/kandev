---
id: "01-coverage-validator"
title: "Validate documentation coverage"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CI-PR-DOCS-001
acceptance_criteria:
  - AC-CI-PR-DOCS-001.2
  - AC-CI-PR-DOCS-001.3
  - AC-CI-PR-DOCS-001.4
  - AC-CI-PR-DOCS-001.5
  - AC-CI-PR-DOCS-001.6
system_design:
  - ../../specs/ci/system-design/pull-request-documentation-coverage.md
---

# Task 01: Validate documentation coverage

## Summary

Implement pure path classification and artifact graph validation with TDD. Cover existing artifacts, new packages, deletions, renames, unknown paths, malformed metadata, ambiguous IDs, and incomplete references.

## In scope

- `.github/scripts/pr-docs.cjs`
- `.github/scripts/pr-docs.test.cjs`

## Out of scope

Application code, unrelated workflow cleanup, live label changes, and live ruleset changes.

## Acceptance

- Only explicitly exempt changes pass without a work order; the #3137 runtime-change fixture fails.
- A changed work order and valid referenced package pass without cosmetic edits to existing specifications.
- Missing, empty, deleted, escaping, or mismatched references produce precise failures.

## Verification

Run from the repository root:

```bash
node --test .github/scripts/pr-docs.test.cjs
git diff --check
```

## Files likely touched

- `.github/scripts/pr-docs.cjs`
- `.github/scripts/pr-docs.test.cjs`

## Dependencies

None.

## Risks

Do not confuse the size-label file filter with this policy; their exclusions differ.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ci/requirements/pull-request-documentation-coverage.md)
- [System design](../../specs/ci/system-design/pull-request-documentation-coverage.md)
- [Plan](plan.md)
- [Decision](../../decisions/2026-09-10-pr-documentation-coverage.md)
- `.github/AGENTS.md` and existing PR size workflow contract tests.

## Results

Implemented `.github/scripts/pr-docs.cjs` path classification, bounded
frontmatter parsing, and linked artifact validation. The validator handles
renames, deleted references, malformed metadata, multi-requirement acceptance
criteria, ambiguous requirement IDs, bounded requirement search, disjoint
multi-design ownership, document limits, and the representative runtime-change
fixture.

Verification:

- `node --test .github/scripts/pr-docs.test.cjs`: 35 tests passed.
- `git diff --check`: passed.
