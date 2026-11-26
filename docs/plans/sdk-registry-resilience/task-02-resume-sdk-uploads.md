---
id: "02-resume-sdk-uploads"
title: "Resume recorded SDK uploads"
status: done
wave: 2
depends_on:
  - "01-observe-sdk-registry"
plan: "plan.md"
requirements:
  - REQ-RELEASE-NPM-PUBLICATION-001
  - REQ-RELEASE-NPM-PUBLICATION-002
  - REQ-RELEASE-NPM-PUBLICATION-004
acceptance_criteria:
  - AC-RELEASE-NPM-PUBLICATION-001.3
  - AC-RELEASE-NPM-PUBLICATION-001.4
  - AC-RELEASE-NPM-PUBLICATION-001.5
  - AC-RELEASE-NPM-PUBLICATION-002.2
  - AC-RELEASE-NPM-PUBLICATION-002.3
  - AC-RELEASE-NPM-PUBLICATION-002.4
  - AC-RELEASE-NPM-PUBLICATION-002.5
  - AC-RELEASE-NPM-PUBLICATION-004.1
  - AC-RELEASE-NPM-PUBLICATION-004.2
  - AC-RELEASE-NPM-PUBLICATION-004.3
  - AC-RELEASE-NPM-PUBLICATION-004.4
system_design:
  - ../../specs/release/system-design/npm-publication-reliability.md
---

# Task 02. Resume recorded SDK uploads

## Summary

Retain the exact upload intent before npm runs.
Use the result to recover a delayed upload in a rerun or backfill.
Complete the existing promotion and public consumer checks when the version appears.

## In scope

- Validate Actions artifacts against the signed candidate and trusted workflow metadata.
- Reconcile authenticated workflow-run/job-attempt metadata when retained artifacts are missing.
- Wire the intent, upload, result, and completion steps into the protected SDK job.
- Document reruns, confirmed rejection, legacy evidence, and expired history in the release procedure and repository release skill.

## Out of scope

- General workflow state storage, automated dispatch, and npm publisher administration.
- Changes to the signed candidate or unsupported recovery paths outside the evidence window.

## Acceptance

- The protected entry point requires a retained intent before the first upload. It records the result and preserves an uncertain state when the runner stops first.
- An accepted, uncertain, or interrupted attempt resumes with the same signed archive and one total upload. Unknown history blocks a new upload. A confirmed rejection permits one new upload in a new backfill.
- Authenticated matching public bytes permit recovery for new and legacy candidates. Workflow checks cover permissions, quiet skips, and the required Stable completion gate.

## Verification

Run from the repository root.

```bash
node --test scripts/release/publish-plugin-sdk.test.mjs scripts/release/sdk-upload-attempts.test.mjs scripts/release/sdk-registry.test.mjs scripts/release/sdk-diagnostics.test.mjs scripts/release/sdk-release-io.test.mjs
python3 .github/scripts/release-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
zizmor .github/workflows
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `scripts/release/publish-plugin-sdk.mjs`
- `scripts/release/sdk-publication-state.mjs`
- `scripts/release/sdk-release-io.mjs`
- `scripts/release/sdk-diagnostics.mjs`
- `scripts/release/sdk-upload-attempts.mjs`
- `scripts/release/sdk-upload-attempts.test.mjs`
- `scripts/release/publish-plugin-sdk.test.mjs`
- `scripts/release/sdk-diagnostics.test.mjs`
- `scripts/release/sdk-release-io.test.mjs`
- `.github/workflows/release.yml`
- `.github/scripts/release-workflow-contract_test.py`
- `docs/public/release-process.md`
- `.agents/skills/release/SKILL.md`

## Dependencies

[Task 01](task-01-observe-sdk-registry.md) puts the longer, bounded observation on the recovery path.

## Risks

- An intent artifact can survive a cancellation before any outcome. Allow a new authorization only with positive evidence that the command did not write.
- The workflow controls one release-wide concurrency group. Tests must prove that a backfill uses the same guard.
- Missing history for an absent legacy SDK requires maintainer reconciliation. An authenticated public version can finish through the regular verification path.

## Parallelism

`sequential`

## Inputs

- [Recovery design](../../specs/release/system-design/npm-publication-reliability.md#recovery-and-verification).
- [Upload attempt ADR](../../decisions/2026-11-26-sdk-npm-upload-attempts.md).
- `scripts/release/sdk-artifact.mjs` and the pinned Actions artifact upload steps.

## Results

Passed:

- The release SDK suite passed 99 tests with Node 24.19.0 and npm 11.21.0. It includes a real archive pack and public consumer verification.
- Static review regressions cover the canonical workflow API path, missing retained artifacts after successful intent retention, incomplete job metadata, skipped publisher jobs, E403 uncertainty, transient completion reads, first-package policy rechecks, pre-publish interruption, `already_public` observation, and disabled npm CLI retries for writes.
- PR review follow-up indexes attempts through authenticated workflow-run and job-attempt metadata, carries history into visible-version completion, and blocks when that history is unreadable.
- `python3 .github/scripts/release-workflow-contract_test.py` passed 79 tests.
- `python3 .github/scripts/lint-action-pinning_test.py` passed 9 tests.
- Public documentation tests passed 63 tests; the validator checked 56 pages.
- The docs catalog validated 467 decisions and 2,118 specifications. All specification files passed lint.
- Prettier checks passed for the changed formatted JavaScript files. `git diff --check` passed.

`zizmor .github/workflows` returned nonzero on existing findings. The findings
cover `pull_request_target` triggers and `shell: cmd` in other workflows, plus
the existing informational `softprops/action-gh-release` finding in
`release.yml`. The scan reported no finding on the new SDK upload steps.
