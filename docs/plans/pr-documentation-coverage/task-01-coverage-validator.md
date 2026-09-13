---
id: "01-coverage-validator"
title: "Validate documentation coverage"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CI-PR-DOCS-001
  - REQ-CI-PR-DOCS-003
acceptance_criteria:
  - AC-CI-PR-DOCS-001.2
  - AC-CI-PR-DOCS-001.3
  - AC-CI-PR-DOCS-001.4
  - AC-CI-PR-DOCS-001.5
  - AC-CI-PR-DOCS-001.6
  - AC-CI-PR-DOCS-003.1
  - AC-CI-PR-DOCS-003.2
  - AC-CI-PR-DOCS-003.3
  - AC-CI-PR-DOCS-003.4
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
- Repeated requirement lookups reuse successful results only within one PR
  snapshot and requirement directory. Empty search results still load changed
  requirements at the exact head; failed lookups remain evaluation errors.

## Verification

Run from the repository root:

```bash
node --test .github/scripts/pr-docs.test.cjs
python3 .github/scripts/pr-docs-workflow-contract_test.py
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

### Requirement lookup regression, 2026-09-13

Successful code-search results (including empty results) now reuse a cache
keyed by requirement directory and ID within one exact-head snapshot.
Directory fallback reuses the complete listing for that snapshot. Existing
bounded file reads still load PR-only requirements and search candidates at
the exact head. Each distinct ID is searched even when a previously loaded
document defines it, so other candidate definitions remain visible to the
ambiguity check. Lookup failures remain errors; retries and merge-group
members get fresh caches.

TDD evidence:

- Red: six work orders sharing three IDs exhausted a deterministic ten-search
  budget on request 11 and returned `error` instead of `covered`.
- Green: the same fixture returns `covered` with three searches. A second red
  regression exceeded a one-listing budget for PR-only requirements; it now
  passes with one listing and three empty search results.
- `node --test .github/scripts/pr-docs.test.cjs`: passed.
- `node --test-isolation=none --test --test-reporter=dot .github/scripts/pr-docs.test.cjs`:
  50 tests passed, including system/design boundaries, PR-only and missing
  requirements, ambiguous definitions, failure recovery, head retries, and
  independent merge-group members. Disabling process isolation exposes the
  individual tests in this executor, whose default runner reports only the file.
- `python3 .github/scripts/pr-docs-workflow-contract_test.py`: 5 tests passed.
- `python3 scripts/list-docs.py validate`: 265 decisions and 824 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.

A GET-only diagnostic using this workspace's fixed evaluator on exact PR
#3626 head `93abff34efe8c9ab2bec68acb8e6aba1eb2a23d4` returned `covered`: six
work orders, nine accepted references, three searches, 18 total requests, and
no failed requests. It published no status. The earlier CI run's caught
exception was not observed, and distinct IDs or a depleted shared quota can
still fail closed. The fix must reach trusted `main` before the privileged
workflow consumes it; rerunning an older run does not substitute this branch's
evaluator. Workflow permissions, bounds, policy, and status rules are unchanged.

Internal docs updated only. This CI implementation change has no public product
documentation or screenshot impact.
