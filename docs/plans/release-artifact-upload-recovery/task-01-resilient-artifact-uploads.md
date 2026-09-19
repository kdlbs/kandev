---
id: "01-resilient-artifact-uploads"
title: "Add resilient artifact uploads"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-RELEASE-ARTIFACT-UPLOAD-RECOVERY-001
acceptance_criteria:
  - AC-RELEASE-ARTIFACT-UPLOAD-RECOVERY-001.1
  - AC-RELEASE-ARTIFACT-UPLOAD-RECOVERY-001.2
  - AC-RELEASE-ARTIFACT-UPLOAD-RECOVERY-001.3
  - AC-RELEASE-ARTIFACT-UPLOAD-RECOVERY-001.4
  - AC-RELEASE-ARTIFACT-UPLOAD-RECOVERY-001.5
system_design:
  - ../../specs/release/system-design/artifact-upload-recovery.md
---

# Task 01: Add resilient artifact uploads

## Summary

Update the Stable and shared runtime release workflow so transient required
artifact upload failures receive two bounded retries. Keep missing outputs and
persistent upload failures fail-closed, and let all desktop matrix targets
finish before the publication gate evaluates the result.

## In scope

- Add the three-attempt upload pattern to `build-web`, `build-bundles`, and
  `build-desktop` required artifact uploads.
- Set `if-no-files-found: error` on those required uploads.
- Set `build-desktop.strategy.fail-fast: false`.
- Add static contract coverage for the retry, matrix, and publication behavior.

## Out of scope

- Artifact download retries.
- External publication retry policies.
- Automatic backfill dispatch or release tag changes.

## Acceptance

- Each required upload logs and performs at most three attempts with 30-second
  and 60-second waits, using `overwrite: true` on retries.
- Missing required files and three failed attempts make the producer job fail.
- A failed desktop target does not cancel sibling targets, and publication
  remains gated on the complete desktop matrix.

## Verification

```bash
python3 .github/scripts/release-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
python3 .github/scripts/lint-action-pinning.py
zizmor .github/workflows
git diff --check -- .github/workflows/release.yml .github/scripts/release-workflow-contract_test.py
```

## Files likely touched

- `.github/workflows/release.yml`
- `.github/scripts/release-workflow-contract_test.py`

## Dependencies

None.

## Risks

- A persistent upload failure can add up to 90 seconds to a producer job.
- The repeated workflow pattern must remain synchronized across required upload
  sites.

## Parallelism

`sequential`

## Inputs

- `docs/specs/release/requirements/artifact-upload-recovery.md`
- `docs/specs/release/system-design/artifact-upload-recovery.md`
- The existing release workflow and its contract tests.

## Results

Implemented the three-attempt upload pattern for web, runtime, and desktop
artifacts, with explicit missing-file failures, bounded waits, retry logging,
and fail-closed final gates. The desktop matrix now uses `fail-fast: false`
while `publish-release` still requires a successful desktop matrix.

Verification passed:

- `python3 .github/scripts/release-workflow-contract_test.py`: 33 tests.
- `python3 .github/scripts/lint-action-pinning_test.py`: 9 tests.
- `python3 .github/scripts/lint-action-pinning.py`: 24 workflows.
- `git diff --check -- .github/workflows/release.yml .github/scripts/release-workflow-contract_test.py`.

`zizmor .github/workflows` completed with its existing repository findings.
The changed `release.yml` had 27 findings before and after this work.
