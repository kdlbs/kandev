---
id: "02-complete-weekly-pr"
title: "Complete weekly PR records and validation"
status: completed
wave: 2
depends_on:
  - "01-decouple-runtime-tests"
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-001
  - REQ-CI-PR-DOCS-001
  - REQ-CI-PR-DOCS-003
acceptance_criteria:
  - AC-AGENTS-RUNTIME-UPDATES-001.9
  - AC-AGENTS-RUNTIME-UPDATES-001.10
  - AC-CI-PR-DOCS-001.3
  - AC-CI-PR-DOCS-001.4
  - AC-CI-PR-DOCS-001.5
  - AC-CI-PR-DOCS-003.1
  - AC-CI-PR-DOCS-003.3
system_design:
  - ../../specs/agents/system-design/runtime-updates-01.md
  - ../../specs/ci/system-design/pull-request-documentation-coverage.md
---

# Task 02: Complete weekly PR records and validation

## Summary

Make the bot's grouped PR satisfy the current documentation policy. Generate
the delivery record after validation and explicitly request the trusted
documentation evaluation for the created or refreshed PR.

## In scope

- Add a generator and offline fixture tests under `.github/scripts/`.
- Save the trusted baseline catalogue before the updater runs. Generate the
  fixed plan and work order after the six-package validation succeeds.
- Include before/after versions, active requirement/design references, and
  actual validation results. An unchanged catalogue produces no changed record.
- Stage only the catalogue and the two generated Markdown files.
- Export the PR number after both create and refresh paths. Keep one grouped
  PR, the existing token permissions, and all six validation dispatches.
- Use the repository PR template and link the generated work order in the body.
- Dispatch `pr-docs.yml` from `main` with that PR number. Extend raw workflow
  contract tests and register generator tests in the existing lint workflow.

## Out of scope

Documentation policy changes, automatic exception labels, new credentials,
auto-merge, schedule changes, and broader PR infrastructure refactors.

## Acceptance

1. Generated catalogue-plus-record fixtures pass `validateCoverage`. Catalogue-only,
   missing-reference, and stale-record fixtures fail their intended assertions.
2. Workflow contracts prove validation precedes completed records, commit, and
   push. Both PR paths expose the exact PR number for the trusted-main dispatch.
3. No-change runs create no PR or record changes. Repeated changed runs refresh
   the same record paths and the single existing PR.

## Verification

Run from the repository root.

```bash
set -euo pipefail
node --test scripts/update-agent-runtime-pins.test.mjs .github/scripts/record-agent-runtime-pin-refresh.test.mjs
python3 .github/scripts/update-agent-runtime-pins-workflow-contract_test.py
python3 .github/scripts/pr-docs-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
python3 .github/scripts/lint-action-pinning.py
zizmor .github/workflows
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Local tests must cover creation and refresh dispatches with stubbed `gh`
arguments. Remote workflow execution remains a separate delivery check after
publication authorization. Local success does not prove remote PR creation.

## Files likely touched

- `.github/scripts/record-agent-runtime-pin-refresh.mjs` (new)
- `.github/scripts/record-agent-runtime-pin-refresh.test.mjs` (new)
- `.github/scripts/update-agent-runtime-pins-workflow-contract_test.py`
- `.github/workflows/update-agent-runtime-pins.yml`
- `.github/workflows/lint-action-pinning.yml`
- `docs/plans/managed-runtime-pin-automation/plan.md` and
  `task-01-refresh-pins.md`, generated only for a real changed catalogue.

## Dependencies

Task 01 restores validation with changed pin values before the complete weekly
path can pass.

## Risks

The documentation workflow rejects a non-default dispatch ref. The six existing
test workflows use the bot branch, while documentation evaluation uses `main`.
Generated delivery records must not claim that a planned or skipped check passed.

## Parallelism

`sequential`

## Inputs

- Runtime design: scheduled workflow and delivery records.
- CI design: artifact contract, events/overrides, and security.
- `.github/AGENTS.md` and existing workflow contract tests.
- `validateCoverage` in `.github/scripts/pr-docs.cjs`.

## Results

Completed on 2026-11-05.

- Added the delivery-record generator and nine offline generator tests. Changed
  catalogues produce the fixed plan and work-order paths; an unchanged catalogue
  writes no completed record and leaves existing files untouched. Generated
  records pass the actual `validateCoverage` function, while catalogue-only and
  broken-reference fixtures fail with the intended statuses.
- The workflow saves the trusted baseline catalogue before updating, generates
  records only after the six-package Go validation with the `fts5` build tag,
  stages only the catalogue and two record files, and exports the PR number from
  both create and refresh paths.
- The six required validation dispatches remain. A separate `pr-docs.yml`
  dispatch uses `main` and the exported PR number. The PR body follows the
  repository template and links the generated work order.
- The workflow contract test now executes both PR paths and the dispatch step
  with a stubbed `gh`, proving the create, refresh, six validation dispatches,
  and trusted-main documentation dispatch arguments. The generator tests are
  registered in `Makefile`'s `test-scripts` target as well as the lint workflow.
- `node --test scripts/update-agent-runtime-pins.test.mjs .github/scripts/record-agent-runtime-pin-refresh.test.mjs`
  passed 18/18.
- The pin workflow contract passed 13/13, the PR documentation workflow contract
  passed 7/7, and action-pinning tests passed 9/9.
- `zizmor` reported no findings for the changed pin workflow. The repository-wide
  workflow directory retains its pre-existing 238 findings, unchanged from the
  HEAD baseline.
- `python3 scripts/list-docs.py validate` and
  `python3 scripts/lint-spec-files.py --all` passed. `git diff --check` passed.
