---
id: "01-replace-requirement-search"
title: "Replace requirement code search"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CI-PR-DOCS-001
  - REQ-CI-PR-DOCS-003
  - REQ-CI-PR-DOCS-004
acceptance_criteria:
  - AC-CI-PR-DOCS-001.4
  - AC-CI-PR-DOCS-001.5
  - AC-CI-PR-DOCS-003.1
  - AC-CI-PR-DOCS-003.2
  - AC-CI-PR-DOCS-003.3
  - AC-CI-PR-DOCS-003.4
  - AC-CI-PR-DOCS-004.1
  - AC-CI-PR-DOCS-004.2
  - AC-CI-PR-DOCS-004.3
  - AC-CI-PR-DOCS-004.5
system_design:
  - ../../specs/ci/system-design/pull-request-documentation-coverage.md
---

# Task 01: Replace requirement code search

## Summary

Locate requirement definitions with bounded `git grep` against the exact PR
head instead of GitHub code search. Preserve existing structural validation,
per-member merge-queue isolation, and trusted workflow execution.

## In scope

- Build a Git-object reader that fetches the base repository's PR ref,
  verifies the requested head SHA, and searches only referenced requirement
  directories without checking out the PR head.
- Pass candidate paths through existing exact-head document reads and
  validation; remove code search, base-identity optimization, and filename
  fallback from the loader.
- Add deterministic tests with temporary Git trees and update CI workflow
  contract and `.github/AGENTS.md` guidance where the new boundary requires it.

## Out of scope

- Coverage exemptions, label permissions, status context, ruleset changes, and
  unrelated API retry behavior.
- External credentials, execution of PR files, or automatic reruns of #3834.

## Acceptance

- An unchanged requirement document containing multiple referenced IDs, and
  valid new or moved documents, pass from the exact PR head without GitHub
  code-search calls; missing or duplicate definitions still fail coverage.
- A stale, unavailable, incomplete, unsafe, or over-limit Git lookup returns an
  infrastructure error and cannot publish a success for the wrong revision.
- Fork and merge-group fixtures use each verified member head while the
  workflow continues to execute only trusted checked-out code.

## Verification

Run from the repository root:

```bash
node --test .github/scripts/pr-docs-git.test.cjs .github/scripts/pr-docs.test.cjs
python3 .github/scripts/pr-docs-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
python3 .github/scripts/lint-action-pinning.py
zizmor .github/workflows
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `.github/scripts/pr-docs.cjs`
- `.github/scripts/pr-docs.test.cjs`
- `.github/scripts/pr-docs-git.cjs` (new bounded Git-object reader)
- `.github/scripts/pr-docs-git.test.cjs`
- `.github/scripts/pr-docs-workflow-contract_test.py`
- `.github/workflows/lint-action-pinning.yml`
- `.github/workflows/pr-docs.yml` if the trusted checkout needs explicit Git-fetch wiring
- `.github/AGENTS.md`
- `Makefile`
- `docs/specs/ci/requirements/pull-request-documentation-coverage.md`
- `docs/specs/ci/system-design/pull-request-documentation-coverage.md`
- `docs/plans/pr-documentation-git-lookup/plan.md`

## Dependencies

None. Implement as one end-to-end change; the previous documentation coverage
plan remains complete.

## Risks

The fetch ref may advance during evaluation, a partial clone may lack needed
blobs, and malformed paths can appear in Git output. Match the API head SHA,
bound all child processes, and treat incomplete data as infrastructure error.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ci/requirements/pull-request-documentation-coverage.md)
- [System design](../../specs/ci/system-design/pull-request-documentation-coverage.md)
- [Plan](plan.md) and [decision](../../decisions/2026-09-27-pr-documentation-git-lookup.md)
- `.github/AGENTS.md`, the current validator and tests, and the PR walkthrough's
  verified Git-object fetch pattern.

## Results

Implemented exact-head Git requirement lookup with bounded object fetch and
search, SHA verification, path and output validation, and snapshot-scoped
caching. The validator uses existing exact-head API reads for candidate files,
and removes GitHub code search and filename fallback. Tests cover the PR #3834
shape, new, moved, deleted, and duplicate definitions, mismatch and failure
paths, merge-group isolation, and trusted-checkout preservation.

Passed the Node validator and Git reader tests (94 tests), workflow contract,
action-pinning tests and lint, documentation catalog validation, specification
lint, Node syntax checks, and `git diff --check`. The repository-wide audit
(`zizmor .github/workflows`) remains nonzero because of findings in other
workflow files; the changed `pr-docs.yml` workflow has no findings.
