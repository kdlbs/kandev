---
id: "01-restore-helper-permissions"
title: "Restore downloaded helper permissions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-RELEASE-COMPACT-RUNTIME-003
acceptance_criteria:
  - AC-RELEASE-COMPACT-RUNTIME-003.1
  - AC-RELEASE-COMPACT-RUNTIME-003.2
  - AC-RELEASE-COMPACT-RUNTIME-003.4
system_design:
  - ../../specs/release/system-design/compact-runtime-distribution.md
---

# Task 01: Restore Downloaded Helper Permissions

## Summary

Restore executable permissions at both canonical artifact download boundaries.
Use regression tests that execute the workflow commands against real artifact
fixtures with stripped modes. Preserve all existing integrity checks.

## In scope

- Add failing regression tests before changing the workflow.
- Add a restoration step in `build-bundles` and `verify-release-assets`.
- Apply `chmod 0755` to the four explicit helper paths immediately after download.
- Execute the extracted restoration scripts against temporary fixtures.
- Record the exact validation results in this work order and the plan.

## Out of scope

- Changes to artifact schemas, the verifier, channel gates, or signing.
- Release dispatch, publication, commits, pushes, or pull requests.
- Backend, frontend, runtime resolver, and public documentation changes.

## Acceptance

1. Both consumers restore all four modes before verification or copying.
   Stable and Nightly fixtures pass the real verifier without byte changes.
2. A missing helper fails restoration. A wrong release identity still fails
   verification after restoration. Existing payload checks remain intact.
3. Workflow regression tests fail before the correction and pass afterward.
   All commands in the verification block pass.

## Verification

Run these commands from the repository root:

```bash
python3 .github/scripts/release-workflow-contract_test.py
node --test scripts/release/remote-helper-assets.test.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

The first command includes the three regression tests named in the plan.
Node and Bash must be available for those executable workflow tests.

## Files likely touched

- `.github/workflows/release.yml`
- `.github/scripts/release-workflow-contract_test.py`
- `docs/specs/release/system-design/compact-runtime-distribution.md`
- `docs/plans/release-helper-permissions/plan.md`
- `docs/plans/release-helper-permissions/task-01-restore-helper-permissions.md`

## Dependencies

None. The existing producer and verifier define the fixture contract.

## Risks

- A test that only searches for `chmod` does not prove mode restoration.
- A glob can omit a missing helper without failing.
- Global step extraction can accidentally cover only the first consumer.
  Extract restoration scripts within each job.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/release/requirements/compact-runtime-distribution.md), requirement `003`.
- [System design](../../specs/release/system-design/compact-runtime-distribution.md#artifact-download-boundary).
- [Plan](plan.md), root cause and tests.
- `.github/AGENTS.md` and the release skill.
- `scripts/release/remote-helper-assets.mjs` and its existing fixture tests.

## Results

Both `build-bundles` and `verify-release-assets` now run an explicit
`chmod 0755` for all four helpers immediately after downloading the canonical
artifact. The verifier, payload checks, and artifact identity checks remain in
place. The regression tests extract and execute each workflow step. They use
Stable and Nightly fixtures created by the real helper artifact producer.

The new regressions failed before the workflow correction because the
restoration steps were absent. They pass with the correction. Verification:

- `python3 .github/scripts/release-workflow-contract_test.py`: 48 tests passed.
- `node --test scripts/release/remote-helper-assets.test.mjs`: 8 tests passed.
- `make -C apps/backend build-agentctl-remote`: all four remote helpers built.
  Darwin signing tools were unavailable; the target left those binaries as-is.
- `python3 scripts/list-docs.py validate`: 348 decisions and 1336
  specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 .github/scripts/lint-action-pinning_test.py`: 9 tests passed.
- `git diff --check`: passed.

`zizmor .github/workflows` exited nonzero on existing findings unrelated to
the added steps. It reported no finding on either restoration step.
`actionlint` was unavailable in the environment.
