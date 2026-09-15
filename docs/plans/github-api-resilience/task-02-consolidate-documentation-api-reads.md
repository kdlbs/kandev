---
id: "02-consolidate-documentation-api-reads"
title: "Consolidate documentation API reads"
status: done
wave: 2
depends_on:
  - "01-rate-aware-api-recovery"
plan: "plan.md"
requirements:
  - REQ-CI-PR-DOCS-002
  - REQ-CI-PR-DOCS-003
  - REQ-CI-PR-DOCS-004
acceptance_criteria:
  - AC-CI-PR-DOCS-002.2
  - AC-CI-PR-DOCS-003.1
  - AC-CI-PR-DOCS-003.2
  - AC-CI-PR-DOCS-004.1
  - AC-CI-PR-DOCS-004.2
  - AC-CI-PR-DOCS-004.3
  - AC-CI-PR-DOCS-004.4
system_design:
  - ../../specs/ci/system-design/pull-request-documentation-coverage.md
---

# Task 02: Consolidate documentation API reads

## Summary

Reuse exact-head and base data, remove a duplicate pull request read, and avoid
full evaluations for events that cannot change documentation coverage.

## In scope

- `.github/scripts/pr-docs.cjs`
- `.github/scripts/pr-docs.test.cjs`
- `.github/workflows/pr-docs.yml`
- `.github/scripts/pr-docs-workflow-contract_test.py`
- This work order's implementation results

## Out of scope

Coverage-policy exemptions, artifact frontmatter, merge-group membership,
rulesets, tokens, and other GitHub workflows.

## Acceptance

- Reuse the pending-status pull request snapshot. Perform one later consistency
  read for a stable pull request. Read each `(revision, path)` once.
- Resolve a verified existing requirement from changed head/base documents with
  zero code-search calls. Keep bounded fallback and duplicate detection for new,
  moved, missing, or ambiguous IDs.
- Route opened, reopened, synchronize, exact `no-docs-allow` label transitions,
  dispatch, and merge-group checks. Skip description, readiness, and unrelated
  label events before checkout or API use.

## Verification

Run from the repository root:

```bash
node --test .github/scripts/pr-docs.test.cjs
python3 .github/scripts/pr-docs-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
python3 .github/scripts/lint-action-pinning.py
zizmor .github/workflows/pr-docs.yml
git diff --check
```

## Files likely touched

- `.github/scripts/pr-docs.cjs`
- `.github/scripts/pr-docs.test.cjs`
- `.github/workflows/pr-docs.yml`
- `.github/scripts/pr-docs-workflow-contract_test.py`
- `docs/plans/github-api-resilience/task-02-consolidate-documentation-api-reads.md`

## Dependencies

Task 01.

## Risks

Start a fresh cache after metadata instability and for every merge-group member.
Scan all changed requirement documents in a referenced directory before relying
on trusted-base uniqueness.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ci/requirements/pull-request-documentation-coverage.md)
- [System design](../../specs/ci/system-design/pull-request-documentation-coverage.md)
- [Plan](plan.md)
- `.github/AGENTS.md`

## Results

Implemented the exact-revision read strategy in `.github/scripts/pr-docs.cjs`
and the relevant event gates in `.github/workflows/pr-docs.yml`.

- The initial pull request snapshot used for the pending status is passed into
  evaluation, leaving one later consistency read for a stable run.
- Content reads use a `(revision, path)` cache. Changed requirement documents
  are loaded at the exact head before search, and their corresponding base
  paths are loaded once for identity comparison.
- Existing or renamed requirement definitions with the same requirement
  heading/ID in their trusted base path skip code search, including body or
  acceptance-text changes. New, moved without a trusted base identity, missing,
  and ambiguous identities keep the bounded search and directory fallback.
- The workflow admits only opened, reopened, synchronize, exact override-label
  transitions, manual dispatch, and merge-group checks. Description edits,
  readiness changes, and unrelated label events are rejected at the job gate.

Verification:

- `node --test .github/scripts/pr-docs.test.cjs` (72 passed)
- `python3 .github/scripts/pr-docs-workflow-contract_test.py` (6 passed)
- `python3 .github/scripts/lint-action-pinning_test.py` (9 passed)
- `python3 .github/scripts/lint-action-pinning.py` (24 workflows passed)
- `zizmor .github/workflows/pr-docs.yml` (no findings)
- `git diff --check` (passed)
