---
id: "03-queue-coverage"
title: "Support merge queue coverage"
status: done
wave: 3
depends_on:
  - "02-pr-workflow"
plan: "plan.md"
requirements:
  - REQ-CI-PR-DOCS-001
  - REQ-CI-PR-DOCS-002
  - REQ-CI-PR-DOCS-003
acceptance_criteria:
  - AC-CI-PR-DOCS-001.6
  - AC-CI-PR-DOCS-002.2
  - AC-CI-PR-DOCS-003.1
  - AC-CI-PR-DOCS-003.4
  - AC-CI-PR-DOCS-003.5
system_design:
  - ../../specs/ci/system-design/pull-request-documentation-coverage.md
---

# Task 03: Support merge queue coverage

## Summary

Add merge-group membership resolution and per-member evaluation. Reconcile live group status after label changes. Document contribution examples, exact exemptions, label persistence, structural limits, dispatch retries, and administrator rollout.

## In scope

- `.github/workflows/pr-docs.yml`
- `.github/scripts/pr-docs.cjs`
- `.github/scripts/pr-docs.test.cjs`
- `.github/scripts/pr-docs-workflow-contract_test.py`
- `.github/pull_request_template.md`
- `.github/AGENTS.md`
- `CONTRIBUTING.md`
- `docs/ci-merge-queue.md`

## Out of scope

Application code, unrelated workflow cleanup, live label changes, and live ruleset changes.

## Acceptance

- Queue groups evaluate every member independently and publish the same context on the group head; missing or ambiguous membership fails.
- Removing an override reevaluates affected current groups without reusing an obsolete success.
- Contributor instructions and rollout checks distinguish deployed failures from ruleset-enforced blocking, with no live ruleset edits.

## Verification

Run from the repository root:

```bash
node --test .github/scripts/pr-docs.test.cjs
python3 .github/scripts/pr-docs-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
python3 .github/scripts/lint-action-pinning.py
zizmor .github/workflows
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `.github/workflows/pr-docs.yml`
- `.github/scripts/pr-docs.cjs`
- `.github/scripts/pr-docs.test.cjs`
- `.github/scripts/pr-docs-workflow-contract_test.py`
- `.github/pull_request_template.md`
- `.github/AGENTS.md`
- `CONTRIBUTING.md`
- `docs/ci-merge-queue.md`

## Dependencies

Task 02.

## Risks

Queue head/base mapping is a GitHub integration assumption. Validate with captured real payloads before adding the required context; do not pass unknown groups.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ci/requirements/pull-request-documentation-coverage.md)
- [System design](../../specs/ci/system-design/pull-request-documentation-coverage.md)
- [Plan](plan.md)
- [Decision](../../decisions/2026-09-10-pr-documentation-coverage.md)
- `.github/AGENTS.md` and existing PR size workflow contract tests.

## Results

Implemented exact merge-group boundary resolution, independent member
evaluation, all active prefix reevaluation after label removal, target-branch
serialization with a bounded pending queue, contributor guidance, and the
administrator rollout notes. The workflow publishes the shared status on the
synthetic group head without changing labels, queue membership, or rulesets.

Verification:

- `node --test .github/scripts/pr-docs.test.cjs`: 35 tests passed.
- `python3 .github/scripts/pr-docs-workflow-contract_test.py`: 5 tests passed.
- `python3 .github/scripts/lint-action-pinning_test.py`: 9 tests passed.
- `python3 .github/scripts/lint-action-pinning.py`: 24 workflows passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `zizmor .github/workflows/pr-docs.yml`: no findings.
- `git diff --check`: passed.

The repository-wide `zizmor .github/workflows` command still reports existing
findings in unrelated workflows. Live merge-group payload validation remains a
deployment step before the status becomes required.
