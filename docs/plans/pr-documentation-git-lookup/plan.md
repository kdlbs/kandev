---
created: 2026-09-27
status: done
requirements:
  - REQ-CI-PR-DOCS-001
  - REQ-CI-PR-DOCS-003
  - REQ-CI-PR-DOCS-004
system_design:
  - ../../specs/ci/system-design/pull-request-documentation-coverage.md
legacy_specs: []
---

# Implementation plan: Git object requirement lookup

## Overview

Replace GitHub code search in the PR documentation coverage check with bounded
`git grep` over each evaluated PR member's exact head tree. Keep the existing
coverage policy, status behavior, and trusted workflow checkout. One work order
delivers the reader and its integration together so every intermediate change
can still evaluate a complete PR.

## Scope

### In scope

- Fetch and verify the current PR head as inert Git objects for ordinary,
  fork, manual-dispatch, and merge-group evaluations. Fetch public PR refs
  anonymously and keep checkout credentials disabled.
- Locate requirement candidates in each owning system's exact-head tree and
  validate their contents through the existing bounded document path.
- Remove code-search requests and filename-derived fallback; retain missing,
  ambiguous, and infrastructure-error distinctions.
- Update tests and `.github/AGENTS.md` for the new trusted-code/data boundary.

### Out of scope

- Changing which paths need coverage, the `no-docs-allow` override, or the
  required status context.
- Private-repository support, running or checking out a contributor's code,
  adding a new token, or changing repository rulesets and merge-queue membership.
- Retrying the failed PR #3834 workflow or posting statuses during planning.

## Technical approach

The CI-owned requirement and system design define the contract. Reuse the
trusted checkout in `.github/workflows/pr-docs.yml` and the existing PR metadata
and changed-file API reads in `.github/scripts/pr-docs.cjs`. A narrow Git object
reader fetches `refs/pull/<number>/head` from the base repository, verifies its
commit ID against the current API snapshot, and keeps the checkout at the
trusted revision. Because this repository is public, fetches are anonymous and
`persist-credentials` stays disabled. The [PR walkthrough's read-only fetch pattern](../../../.github/workflows/pr-walkthrough.yml)
is the closest local example; the coverage reader does not run PR code.

Search each referenced requirement ID in its system's requirements subtree with
fixed-string `git grep` against the verified head SHA. Use argument arrays, bounded child
execution, path-safe output, and cached lookups per snapshot. Pass matching
Markdown paths to the existing exact-head `getFile` and structural validation.
The reader must distinguish a complete no-match from a fetch/search failure.
Preserve separate evaluation for each merge-group member and discard cached
paths when a head changes.

Remove `GitHubClient.searchCode` and the old changed/base identity and
filename fallback from the loader. Keep the GitHub API adapter for metadata,
changed files, file contents, and commit statuses. Update `.github/AGENTS.md`
so its documented trusted-read rule includes this bounded Git-object reader.
The [lookup decision](../../decisions/2026-09-27-pr-documentation-git-lookup.md)
records the alternatives and trust boundary.

## Tests

| Criteria | Evidence |
| --- | --- |
| AC-CI-PR-DOCS-001.4, AC-CI-PR-DOCS-004.2 | A PR #3834-shaped fixture finds seven IDs in one unchanged requirements file without a code-search request. |
| AC-CI-PR-DOCS-001.5, AC-CI-PR-DOCS-004.3 | Temporary Git trees cover new, moved, deleted, and duplicate definitions, including a duplicate outside the PR diff. |
| AC-CI-PR-DOCS-003.1, AC-CI-PR-DOCS-003.4 | Fetch/SHA mismatch and two merge-group members cannot reuse another head's candidates. |
| AC-CI-PR-DOCS-003.2, AC-CI-PR-DOCS-003.3, AC-CI-PR-DOCS-004.5 | Fetch failure, incomplete grep, timeout, output limit, unsafe path, and non-regular document fail closed; workflow contract keeps the trusted checkout. |
| AC-CI-PR-DOCS-004.1 | Repeated references in one snapshot reuse the verified head and candidate result. |

Use `.github/scripts/pr-docs.test.cjs`, `.github/scripts/pr-docs-git.test.cjs`,
and the workflow contract test. Keep the Git command tests local with temporary
repositories and injected process or fetch boundaries; no live GitHub token is
required for unit tests.

## Work orders

- [x] [Task 01: Replace requirement code search](task-01-replace-requirement-search.md)

## Verification results

- PASS: `node --test .github/scripts/pr-docs-git.test.cjs .github/scripts/pr-docs.test.cjs` (111 tests).
- PASS: `python3 .github/scripts/pr-docs-workflow-contract_test.py` (7 tests).
- PASS: `python3 .github/scripts/lint-action-pinning_test.py` (9 tests) and
  `python3 .github/scripts/lint-action-pinning.py` (24 workflow files).
- PASS: specification catalog validation (317 decisions and 1210 specifications)
  and specification lint.
- PASS: harness tests (19), all-file harness lint (200 files), and targeted
  harness pre-commit check for `.github/AGENTS.md`.
- PASS: Node syntax checks, `git diff --check`, and focused `zizmor` audit for
  `pr-docs.yml`.
- `zizmor .github/workflows` exits 14 on findings in other workflows. The
  focused `pr-docs.yml` audit reports no findings (1 ignored, 2 suppressed).
- PR fixup preserves the Git operation stage on process-launch errors, isolates
  fixture Git commands from inherited `GIT_*` variables, and documents anonymous
  fetches for the public repository with checkout credentials disabled.

## Risks

- A PR ref can move between API metadata and Git fetch. Verify the object ID
  and retry the snapshot within the existing bound or publish an error.
- `git grep` against a partially fetched commit can trigger object transfer.
  Bound child time and output and treat missing objects as infrastructure errors.
- Git output contains untrusted paths. Keep them out of shell source and require
  normalized, regular Markdown paths under the owning requirements directory.
- A large or adversarial requirements tree can exceed process or document
  bounds; it must fail closed with a useful diagnostic.

## References

- [Requirements](../../specs/ci/requirements/pull-request-documentation-coverage.md)
- [System design](../../specs/ci/system-design/pull-request-documentation-coverage.md)
- [Decision](../../decisions/2026-09-27-pr-documentation-git-lookup.md)
