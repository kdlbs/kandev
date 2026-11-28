---
created: 2026-11-26
status: done
requirements:
  - REQ-RELEASE-NPM-PUBLICATION-001
  - REQ-RELEASE-NPM-PUBLICATION-002
  - REQ-RELEASE-NPM-PUBLICATION-004
system_design:
  - ../../specs/release/system-design/npm-publication-reliability.md
legacy_specs: []
---

# SDK registry resilience implementation plan

## Overview

Give npm SDK uploads time to become public and let a retry finish an earlier upload.
Implement bounded registry reads first.
Then use those reads to resume a retained attempt.

The Release system owns this work because it owns publication and recovery.
The plugin system owns the [signed SDK identity and consumer contract](../../specs/plugins/requirements/sdk-distribution.md).

## Incident evidence and assumptions

- npm accepted SDK 0.5.0 at 13:14:02 UTC. The last registry read took place 271 seconds later.
  The SDK job failed at 13:19:02 UTC. npm recorded 0.5.0 at 13:19:11 UTC. [Original job](https://github.com/kdlbs/kandev/actions/runs/45816922667/job/139933281757)
- The retry found the matching public archive, advanced `latest`, and passed both public consumer checks. [Successful retry](https://github.com/kdlbs/kandev/actions/runs/45816922667/attempts/2)
- The protected `release` environment accepts the `main` branch. The repository retains Actions artifacts for 90 days.
- Use existing Actions storage for recovery. The [upload attempt ADR](../../decisions/2026-11-26-sdk-npm-upload-attempts.md) records the retention and trust choices.

## Scope

### In scope

- Ten-minute version and dist-tag observation windows with a final registry check.
- Bounded retry of transient public registry read errors.
- Stable SDK upload recovery using immutable attempt artifacts and the signed archive.
- Maintainer guidance in [the release procedure](../../public/release-process.md#plugin-sdk-publication).

### Out of scope

- Application and runtime npm publication, Nightly, and npm authentication preflight.
- SDK versions, signed receipts, package contents, provenance, and first-package policy.
- Automatic workflow dispatch, release announcements, and product UI.

## Technical approach

1. Update `observeRegistry` in `scripts/release/sdk-publication-state.mjs`.
   Classify transient reads in `scripts/release/sdk-registry.mjs`.
   Set the `publish-plugin-sdk` job limit in `.github/workflows/release.yml`.
2. Add a narrow Actions artifact adapter for SDK attempts.
   Use `runRelease` and the protected job to retain intent, issue one upload, and retain its result.
   Index authenticated workflow runs and job attempts independently of artifact retention, keyed to the exact host tag.
   Reuse `publishPublication` and `ensureLatest` for registry verification and completion.
   Update the public operating procedure with the same release.

The release-wide concurrency group serializes upload authorization.
The two version-specific artifacts give a retry enough evidence to resume the exact upload.

## Tests

| Requirement | Evidence |
| --- | --- |
| AC-RELEASE-NPM-PUBLICATION-001.1 through .2 | `scripts/release/publish-plugin-sdk.test.mjs` advances the clock to 309 seconds and to the final probe. It tests the SDK version and `latest`. |
| AC-RELEASE-NPM-PUBLICATION-001.3 through .5 | `scripts/release/publish-plugin-sdk.test.mjs` counts writes and verifies archive identity, `latest`, and public consumers. |
| AC-RELEASE-NPM-PUBLICATION-001.6 | `scripts/release/sdk-registry.test.mjs` tests error classification. The state-machine tests verify retries and the final error. |
| AC-RELEASE-NPM-PUBLICATION-002.1 through .5 | `scripts/release/sdk-diagnostics.test.mjs` verifies the report for read errors and resumed uploads, with existing output sanitization fixtures. |
| AC-RELEASE-NPM-PUBLICATION-004.1 through .4 | `scripts/release/sdk-upload-attempts.test.mjs` exercises retained, missing, or conflicting evidence and its producer. `scripts/release/publish-plugin-sdk.test.mjs` drives the release entry point through an upload and retry with fake adapters. |
| Stable job constraints | `.github/scripts/release-workflow-contract_test.py` checks step order, job budget, permissions, release modes, and downstream completion. |

## Work orders

- [x] [Task 01. Make SDK registry reads resilient](task-01-observe-sdk-registry.md)
- [x] [Task 02. Resume recorded SDK uploads](task-02-resume-sdk-uploads.md)

## Verification results

Task 01 and Task 02 implementation checks passed. Task 02 used Node 24.19.0
and npm 11.21.0 for the real SDK pack and consumer verification; the SDK suite
passed 99 tests after PR review regressions were added. The full
`zizmor .github/workflows` scan ran but returned existing findings in unrelated
workflows and an informational `softprops/action-gh-release` finding in the
release workflow. It reported no finding on the new SDK upload steps.

PR review follow-up adds authenticated workflow-run/job-attempt reconciliation
for missing retained artifacts, carries visible upload history into completion,
retries transient first reads, and records a result when publication never
started. It also covers skipped jobs with no step records and keeps unreadable
history from being treated as a manual bootstrap.

### Design checks

- `python3 scripts/list-docs.py validate` passed. The catalog includes the upload attempt ADR.
- `python3 scripts/lint-spec-files.test.py` passed all 36 tests.
- `python3 scripts/lint-spec-files.py --all` passed.
- The PR documentation coverage preflight passed for both work orders and six planned production paths.
- `git diff --check` passed. The local link and whitespace checks passed for all six design files.

## Risks

- A GitHub Actions rerun uses its original workflow revision. Runs dispatched before deployment of the attempt gate use the prior procedure.
- The first upload requires complete evidence within the retention window. Keep the attempt records and producer runs until the SDK is verified.
- A long delay can reach the new outer job limit. The next execution uses the retained attempt and repeats the required verification.
- Registry and cancellation scenarios use fixtures. The first protected release using the new workflow provides live confirmation.
