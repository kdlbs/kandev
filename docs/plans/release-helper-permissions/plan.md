---
created: 2026-10-04
status: done
requirements:
  - REQ-RELEASE-COMPACT-RUNTIME-003
system_design:
  - ../../specs/release/system-design/compact-runtime-distribution.md
legacy_specs: []
---

# Implementation Plan: Canonical Helper Artifact Permissions

## Overview

Restore executable permissions after each canonical helper download.
One work order covers both consumers and their regression tests.
The release system owns this boundary because it owns artifact transfer and
the contents of every release channel.

The existing [requirements](../../specs/release/requirements/compact-runtime-distribution.md)
already define complete helper sets and publication integrity.
This repair uses `AC-RELEASE-COMPACT-RUNTIME-003.1`,
`AC-RELEASE-COMPACT-RUNTIME-003.2`, and
`AC-RELEASE-COMPACT-RUNTIME-003.4`. It adds no product requirement.

## Scope

### In scope

- Restore all four canonical helper modes in platform bundle builds and
  Stable asset verification.
- Preserve executable, identity, and payload validation.
- Cover the actual workflow commands with temporary artifacts that model
  permission loss during download.

### Out of scope

- Release dispatch, publication, or edits to existing tags.
- Changes to archive formats, channel contents, signing, or job conditions.
- Runtime resolver, backend, frontend, and public documentation changes.

## Confirmed root cause

Scheduled Nightly runs #194 through #199 fail before Go compilation with
`canonical helper agentctl-linux-amd64 is not executable`.
[Run #199](https://github.com/kdlbs/kandev/actions/runs/37134223734/job/111235995184)
contains this error after successful helper and web artifact downloads.
[PR #3934](https://github.com/kdlbs/kandev/pull/3934) introduced this path
after successful run #193.

The inspected ZIP records executable helper modes. The pinned download action
uses an extractor that writes files without restoring those modes.
A temporary reproduction failed at `0644` and passed at `0755` for both
Stable and Nightly validation. The reproduction files were removed.

## Technical approach

Add a restoration step immediately after the canonical helper download in
`build-bundles` and `verify-release-assets` in `.github/workflows/release.yml`.
Use `chmod 0755` with four explicit operands under
`dist/remote-helper-artifact/bin`: `agentctl-linux-amd64`,
`agentctl-linux-arm64`, `agentctl-darwin-amd64`, and `agentctl-darwin-arm64`.
Keep this operation before `verify-artifact` and before bundle copying.
The explicit operands make a missing helper fail instead of silently passing.

Keep the current artifact directory layout and upload retries.
Permission restoration needs no new script, dependency, or transport format.
The real verifier continues to reject non-regular helpers, wrong identities,
and mismatched Stable payloads. The producer retains its executable checks.

## Tests

Extend `.github/scripts/release-workflow-contract_test.py` with executable
coverage of each job's restoration step. Use existing job extraction patterns.
Create temporary fixtures through the real helper artifact producer.
Set all four modes to `0644` to model downloaded artifacts.

- `test_downloaded_canonical_helpers_restore_executable_modes`: execute each
  restoration script, then use the real verifier for Stable and Nightly fixtures.
  Assert that all helpers have executable modes and unchanged bytes.
  This test must fail before the workflow correction.
- `test_missing_downloaded_canonical_helper_blocks_restoration`: remove one
  helper and assert a nonzero restoration result for each consumer.
- `test_helper_restoration_preserves_artifact_identity_validation`: restore
  permissions and assert that verification still rejects the wrong commit.
- Require restoration after download and before the first helper verification
  or package operation in each job.

`AC-RELEASE-COMPACT-RUNTIME-003.1` and `.2` map to complete, executable helper sets.
`AC-RELEASE-COMPACT-RUNTIME-003.4` maps to missing-helper and wrong-identity failures.
The existing helper tests retain payload-integrity coverage.

## Work orders

- [x] [Task 01: Restore downloaded helper permissions](task-01-restore-helper-permissions.md)

## Verification results

Design validation passed on 2026-10-04:

- `python3 scripts/list-docs.py validate`: 348 decisions and 1336 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specifications passed.
- PR documentation coverage preflight: workflow and documentation paths are exempt.
- Independent reference validation: requirement, acceptance, and design links passed.
- `git diff --check`: passed.

Implementation completed on 2026-10-04. Both canonical helper consumers restore
the four explicit helper modes after download and before verification or
packaging. The regression tests execute each extracted workflow command against
Stable and Nightly artifacts from the real helper producer, compare helper
bytes, and retain missing-helper and identity rejection coverage.

Implementation checks passed:

- The four new workflow contract tests failed before the workflow correction
  because the restoration steps were absent, then passed after the correction.
- `python3 .github/scripts/release-workflow-contract_test.py`: 48 tests passed.
- `node --test scripts/release/remote-helper-assets.test.mjs`: 8 tests passed.
- `make -C apps/backend build-agentctl-remote`: all four helpers built. Darwin
  signing tools were unavailable, so the build target left those binaries as-is.
- `python3 scripts/list-docs.py validate`: 348 decisions and 1336
  specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 .github/scripts/lint-action-pinning_test.py`: 9 tests passed.
- `git diff --check`: passed.

`zizmor .github/workflows` parsed the workflow files and exited nonzero on
existing findings outside the new restoration steps, including unsafe-trigger
and template-injection findings in other workflow logic. `actionlint` was not
installed in the environment.

## Risks

- Fixing only the bundle consumer leaves Stable asset verification broken.
- Removing executable validation hides invalid packaged helpers.
- An old workflow run still uses its original definition after a rerun.
  After merge, use a new Nightly run from the corrected `main` revision.
- Release execution remains outside this work order. Local fixtures do not
  prove that a future publication succeeds.

## Related delivery record

The completed [compact-runtime package](../compact-runtime-distribution/plan.md)
records the original implementation. Its results remain historical evidence.
This package records the separate repair and its validation.
