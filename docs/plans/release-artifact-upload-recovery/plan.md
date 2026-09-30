---
created: 2026-09-19
status: implemented
requirements:
  - REQ-RELEASE-ARTIFACT-UPLOAD-RECOVERY-001
system_design:
  - ../../specs/release/system-design/artifact-upload-recovery.md
legacy_specs: []
---

# Implementation Plan: Resilient Release Artifact Uploads

## Overview

Harden the release workflow against transient GitHub Actions artifact-service
failures. The implementation adds bounded retries to required build artifact
uploads, makes missing outputs fail explicitly, and lets all desktop matrix
targets finish before the release gate evaluates the result. Publication keeps
its existing all-target success requirement.

The work is sequential: update the workflow and its contract tests first, then
bring the public and internal release guidance in line with the behavior.

## Scope

### In scope

- Required web, runtime bundle, and desktop artifact uploads.
- Three total upload attempts with 30-second and 60-second waits.
- Explicit missing-file failures for required uploads.
- `build-desktop` matrix isolation with `fail-fast: false`.
- Contract tests, action security checks, and maintainer recovery guidance.

### Out of scope

- Artifact download retries.
- GHCR, npm, Homebrew, Scoop, or GitHub Release API retry policies.
- Automatic backfill dispatch or tag/version behavior.
- Changes to artifact contents or publication ordering.

## Technical approach

### Workflow and test contract

Update `.github/workflows/release.yml` with a repeated, explicit upload pattern
for the three required producer paths. Each pattern records the action outcome,
waits only after failure, retries with `overwrite: true`, and fails through a
final gate when all attempts fail. Set `if-no-files-found: error` for required
uploads. Set `strategy.fail-fast: false` for the desktop matrix while retaining
the existing downstream `needs.*.result == 'success'` conditions.

Extend `.github/scripts/release-workflow-contract_test.py` to assert the retry
budget, wait durations, overwrite behavior, required-file checks, desktop
matrix setting, and fail-closed publication gates.

### Documentation

Update `docs/public/release-process.md`, `AGENTS.md`, and
`.agents/skills/release/SKILL.md` with the retry and matrix behavior. Keep the
existing backfill instructions and state that retries do not make an incomplete
release publishable.

## Tests

- **Retry contract:** `.github/scripts/release-workflow-contract_test.py`
  asserts all required upload sites use the bounded retry and missing-file
  contract.
- **Action pinning:** `.github/scripts/lint-action-pinning_test.py` and
  `.github/scripts/lint-action-pinning.py` verify every workflow action remains
  pinned.
- **Workflow security:** `zizmor .github/workflows` checks the changed workflow
  for unsafe GitHub Actions patterns.
- **Public documentation:** `scripts/validate-public-docs.test.mjs` and
  `scripts/validate-public-docs.mjs` validate the release guide.
- **Harness documentation:** `scripts/lint-harness-files.test.py` and
  `.github/scripts/lint-harness-files.py --all` validate the updated guidance.

## Work orders

- [x] [Task 01: Add resilient artifact uploads](task-01-resilient-artifact-uploads.md)
- [x] [Task 02: Update release recovery guidance](task-02-release-recovery-guidance.md)

## Verification results

Task 01 and Task 02 are implemented and verified.

- `python3 .github/scripts/release-workflow-contract_test.py`: 33 tests passed.
- `python3 .github/scripts/lint-action-pinning_test.py`: 9 tests passed.
- `python3 .github/scripts/lint-action-pinning.py`: all 24 workflow files passed.
- `zizmor .github/workflows`: the command completed with the repository's
  existing 233 findings and exit code 14. The changed `release.yml` had 27
  findings before and after this change, so the retry blocks added no new
  findings.
- Public documentation validators: 62 tests passed and 47 pages validated.
- Harness validators: 19 tests passed and 199 files passed.
- All scoped `git diff --check` commands passed.

## Risks

- Each failed upload can add up to 90 seconds before the job fails.
- Repeated YAML patterns can drift if a future required artifact upload is added
  without extending the contract test.
- `fail-fast: false` can consume more runner time after a real desktop build
  failure, but preserves diagnostics and independent target results.
