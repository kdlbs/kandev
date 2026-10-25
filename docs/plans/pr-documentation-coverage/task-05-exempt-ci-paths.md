---
id: "05-exempt-ci-paths"
title: "Exempt CI infrastructure paths"
status: done
wave: 5
depends_on:
  - "04-exempt-plugin-registry-source"
plan: "plan.md"
requirements:
  - REQ-CI-PR-DOCS-001
acceptance_criteria:
  - AC-CI-PR-DOCS-001.8
system_design:
  - ../../specs/ci/system-design/pull-request-documentation-coverage.md
---

# Task 05: Exempt CI infrastructure paths

## Summary

Allow a pull request that changes only CI infrastructure under
`.github/workflows/**`, `.github/scripts/**`, or `.github/actions/**` to pass
documentation coverage without a linked delivery package. Keep every other
path, including workflows, scripts, and actions outside `.github/`, covered.

## In scope

- Add the `.github/workflows/**`, `.github/scripts/**`, and `.github/actions/**`
  path families to the classifier exemption list.
- Add regression coverage for the CI-only and mixed-path cases, including a
  workflow, script, or action outside `.github/` that still triggers coverage.
- Update the existing validator test that asserts `.github/workflows/release.yml`
  requires coverage.
- Reconcile the CI requirement, system design, decision, and plan records.

## Out of scope

- Exempting the full `.github/` tree, including configuration, keys, or docker
  files under `.github/`.
- Exempting workflows, scripts, or actions outside `.github/`.
- Changing the label override, rulesets, or merge enforcement.

## Acceptance

- A change containing only paths under `.github/workflows/**`,
  `.github/scripts/**`, or `.github/actions/**` is classified as exempt and does
  not require delivery artifacts.
- A CI change combined with any non-exempt path still requires coverage.
- A workflow, script, or action outside `.github/` still triggers coverage.
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

Task 04. The existing coverage validator and workflow must remain the
implementation boundary.

## Risks

- A directory-prefix exemption could accidentally cover `.github/` files outside
  the three CI path families. Use directory-scoped matching and a negative test
  for a non-CI `.github/` file.
- Broader than the registry exemption: three path families instead of one exact
  path. Keep the scope confined to `.github/` CI infrastructure.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ci/requirements/pull-request-documentation-coverage.md)
- [System design](../../specs/ci/system-design/pull-request-documentation-coverage.md)
- [Plan](plan.md)
- [Decision](../../decisions/2026-09-10-pr-documentation-coverage.md)
- `.github/AGENTS.md` and the existing validator tests.

## Results

Implemented the `.github/workflows/**`, `.github/scripts/**`, and
`.github/actions/**` exemptions in the path classifier and added positive,
mixed-path, and negative regression coverage. The previous
`.github/workflows/release.yml` coverage assertion moved to the CI exemption
test. Non-CI `.github/` files and CI paths outside `.github/` still trigger
coverage.

Verification:

- `node --test .github/scripts/pr-docs.test.cjs`: 80 tests passed.
- `python3 .github/scripts/pr-docs-workflow-contract_test.py`: 7 tests passed.
- `python3 .github/scripts/lint-action-pinning_test.py`: 9 tests passed.
- `python3 .github/scripts/lint-action-pinning.py`: 24 workflows passed.
- `zizmor .github/workflows/pr-docs.yml`: no findings.
- `python3 scripts/list-docs.py validate`: 294 decisions and 1066 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.