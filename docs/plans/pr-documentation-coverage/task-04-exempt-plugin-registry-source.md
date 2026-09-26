---
id: "04-exempt-plugin-registry-source"
title: "Exempt the canonical plugin registry source"
status: done
wave: 4
depends_on:
  - "03-queue-coverage"
plan: "plan.md"
requirements:
  - REQ-CI-PR-DOCS-001
acceptance_criteria:
  - AC-CI-PR-DOCS-001.7
system_design:
  - ../../specs/ci/system-design/pull-request-documentation-coverage.md
---

# Task 04: Exempt the canonical plugin registry source

## Summary

Allow a pull request that updates only the curated `plugin-registry/plugins.yaml`
source to pass documentation coverage without a linked delivery package. Keep
all other registry paths and mixed registry-plus-runtime changes covered.

## In scope

- Add the exact `plugin-registry/plugins.yaml` path to the classifier exemption list.
- Add regression coverage for the registry-only and mixed-path cases.
- Reconcile the CI requirement, system design, decision, and plan records.

## Out of scope

- Exempting the full `plugin-registry/` directory.
- Changing the registry index workflow, registry schema, package validation, labels, or rulesets.
- Updating public product documentation.

## Acceptance

- A registry-only change is classified as exempt and does not require delivery artifacts.
- A registry change combined with any non-exempt path still requires coverage.
- Existing path exemptions and generic YAML classification remain unchanged.

## Verification

Run from the repository root:

```bash
node --test .github/scripts/pr-docs.test.cjs
python3 .github/scripts/pr-docs-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
python3 .github/scripts/lint-action-pinning.py
zizmor .github/workflows/pr-docs.yml
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

Task 03. The existing coverage validator and workflow must remain the implementation boundary.

## Risks

- A prefix or glob exemption could accidentally cover registry schema, generator, or generated-catalog changes. Use exact-path matching and a negative mixed-path test.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ci/requirements/pull-request-documentation-coverage.md)
- [System design](../../specs/ci/system-design/pull-request-documentation-coverage.md)
- [Plan](plan.md)
- [Decision](../../decisions/2026-09-10-pr-documentation-coverage.md)
- `.github/AGENTS.md` and the existing validator tests.

## Results

Implemented the exact `plugin-registry/plugins.yaml` exemption in the path
classifier and added positive and mixed-path regression coverage. Other registry
files still trigger coverage, and mixed changes retain their non-exempt paths.

Verification:

- `node --test .github/scripts/pr-docs.test.cjs`: 77 tests passed.
- `python3 .github/scripts/pr-docs-workflow-contract_test.py`: 7 tests passed.
- `python3 .github/scripts/lint-action-pinning_test.py`: 9 tests passed.
- `python3 .github/scripts/lint-action-pinning.py`: 24 workflows passed.
- `zizmor .github/workflows/pr-docs.yml`: no findings.
- `python3 scripts/list-docs.py validate`: 279 decisions and 959 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
