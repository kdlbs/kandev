---
created: 2026-11-11
status: completed
requirements:
  - REQ-RELEASE-NPM-PUBLICATION-003
system_design:
  - ../../specs/release/system-design/npm-publication-reliability.md
legacy_specs: []
---

# Implementation Plan: npm Preflight Response Diagnostics

## Overview

Make a successful npm exchange that fails local validation distinguishable from an authentication rejection.
Implement one work order before another live attempt.
Keep every existing authentication gate effective.
This package repairs the diagnostic defect. It does not claim to resolve the unknown response incompatibility.

## Evidence and assumption check

- [Release run 43038508616](https://github.com/kdlbs/kandev/actions/runs/43038508616) used workflow and checkout revision `b2376186ed19770aa4e69b1e9689c2f5c4ac2241`.
- [Preflight job 130583698573](https://github.com/kdlbs/kandev/actions/runs/43038508616/job/130583698573) failed at 14:26:59 UTC on 2026-11-11.
- The log named `kandev` and reported an invalid or expired exchange token.
- In `exchangePackageToken`, that message follows HTTP 201 and successful JSON parsing.
  It combines token-type, token-presence, creation-date, expiry-format, and expiry-time failures.
- The response body was not retained. The exact rejected field remains unknown.
  The log does not establish that the returned token was present or that the publisher configuration was invalid.
- All 11 existing preflight tests passed locally.
  A temporary transport fixture reproduced the same message for each missing field and an expired token.
- Release preparation, builds, tagging, and all publication jobs were skipped.
  The final Stable completion failure was a downstream consequence.
- [npm CLI 11.21.0](https://github.com/npm/cli/blob/v11.21.0/lib/utils/oidc.js) consumes the returned token without these metadata checks.
  The [npm API example](https://api-docs.npmjs.com/#tag/OIDC) includes the metadata.
  Neither source proves the actual response shape in this run.

The existing requirement defines the intended gate and credential boundary.
No new product requirement or ADR is necessary.
Only a diagnostic repair is ready for implementation.
A change to accepted response shapes requires additional evidence.

## Scope

### In scope

- A fixed diagnostic reason for each exchange-response validation failure.
- A distinction between local response validation and npm authentication rejection.
- Safe error summaries with package, HTTP status, reason, and both source revisions.
- Regression coverage for the real error and summary boundaries.
- A release-guide explanation of the distinct recovery paths.

### Out of scope

- Relaxation of token-type, token-presence, date, or expiry checks.
- A bypass, token fallback, or change to release modes and job dependencies.
- Live workflow dispatch, npm publication, credential rotation, or publisher changes.
- New workflow inputs, signing changes, SDK evidence changes, or notifications.
- Product UI and browser tests.

## Technical approach

Update `NpmAuthPreflightError` and `exchangePackageToken` in `scripts/release/npm-auth-preflight.mjs`.
Preserve the current acceptance predicate while each failed condition receives a fixed reason.
Reject non-object bodies explicitly before field access.
Keep request bounds, public-registry validation, and redirect rejection intact.

Include the structured reason and HTTP status in the existing failure summary.
Include the workflow and checkout revisions on failure as well as success.
Use fixed recovery messages instead of response values or underlying exception text.
No authentication response enters stdout, stderr, or the step summary.

Use transport-injected tests in `scripts/release/npm-auth-preflight.test.mjs`.
Exercise the emitted summary as well as the error properties.
Use a fixed clock for expiry boundaries.
Retain mixed-package rejection, SDK absence, and unsupported-context coverage.

Update the Early npm authentication section of `docs/public/release-process.md` during implementation.
The page remains a how-to guide.
It must distinguish HTTP rejection, malformed successful response, and expired token.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-RELEASE-NPM-PUBLICATION-003.6` | Separate reason for every validation failure, visible in the serialized failure summary |
| `AC-RELEASE-NPM-PUBLICATION-003.5` | Sentinel credentials and malicious metadata absent from messages and summaries |
| `AC-RELEASE-NPM-PUBLICATION-003.1`, `AC-RELEASE-NPM-PUBLICATION-003.2` | Valid inventory passes and one rejected package blocks the preflight |
| `AC-RELEASE-NPM-PUBLICATION-003.3`, `AC-RELEASE-NPM-PUBLICATION-003.4` | Existing workflow modes and SDK-absence regressions remain effective |

## Work orders

- [x] [Task 01: Identify npm exchange-response failures safely](task-01-response-diagnostics.md).

The user authorized a Luna implementation subagent on 2026-11-11.
The primary session owns coordination, delivery records, and PR publication.
One subagent owns the script, its tests, and the public release guide.

## Operational recovery

After implementation and merge, a maintainer can dispatch a new normal Stable release from the corrected `main` revision.
A rerun of this failed run retains its original workflow and helper revision.
Dry runs skip this gate and cannot prove its repair.
This run created no release tag, so its recovery does not need `backfill_tag`.

If validation fails again, the fixed reason identifies the response condition to investigate.
Only safe structural evidence can inform a later acceptance-rule correction.
If npm rejects authentication, recovery concerns that package's trusted publisher.
A passing preflight can proceed into a real release. Live dispatch is outside this implementation package.

## Verification results

Diagnostic baseline: all 11 existing preflight tests passed on 2026-11-11.
The temporary in-memory reproduction made no network request or repository edit.
Implementation and task-defined checks are complete.

Design validation passed on 2026-11-11:

- `python3 scripts/list-docs.py validate`: 448 decisions and 1982 specifications passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: covered with zero errors for the proposed script change and linked work order.
  This check simulated the future production path. No production file changed.
- `git diff --check`: passed.
- `git status --short -- docs/plans/npm-preflight-response-diagnostics`: both new delivery files are present and untracked.
- Authoring checks found no sentence above 25 words in the new plan or work order.

Implementation verification passed on 2026-11-11:

- `node --test scripts/release/npm-auth-preflight.test.mjs scripts/release/sdk-runtime.test.mjs scripts/release/sdk-registry.test.mjs`: 22 tests passed after review remediation.
- `node --test scripts/release/publish-plugin-sdk.test.mjs scripts/release/sdk-release-io.test.mjs`: 32 tests passed with the SDK's pinned Node and npm versions.
- `python3 .github/scripts/release-workflow-contract_test.py`: 78 tests passed.
- `node --test scripts/validate-public-docs.test.mjs`: 63 tests passed.
- `node scripts/validate-public-docs.mjs`: 55 published pages validated.
- `python3 scripts/list-docs.py validate`: 448 decisions and 1982 specifications passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: covered with zero errors for the actual changed paths and linked delivery artifacts.
- Prettier passed for both modified release scripts.
- `git diff --check`: passed.

The user-selected Luna subagent implemented the script and regression tests.
The primary session coordinated integration and the delivery records.
The release guide now explains each recovery path.
No live token exchange or release dispatch occurred.

The initial regression failed before production edits because the local HTTP 201 validation error lost its HTTP status.
Integration regressions also failed before the publisher and expired-token recovery messages were restored.
The final implementation retains known safe errors and sanitizes unexpected exceptions.

PR review identified lost setup detail in the fixed context-error wrapper.
The primary session reproduced that defect with failing CLI and shared-validator tests.
The shared validator now carries only the failed setting and its required value.
All seven protected-context checks preserve specific reasons without exposing supplied values.
SDK consumer validation initially refused the default tools. It passed with Node 24.19.0 and npm 11.21.0.
The original 48 passing remote checks describe the initial implementation head.
Remote verification of the review correction remains pending in the PR.

The implementation work order is complete. Remote CI and review state belong to the PR delivery evidence.

## Risks

- The actual rejected field is unknown. This repair improves evidence without claiming to fix an unobserved payload.
- Error construction can expose credentials if it includes arbitrary response values.
- Changing the acceptance predicate without evidence can accept invalid tokens or repeat this failure.
- Tests based only on the published API example do not prove compatibility with a live exchange.
- Release dispatch after merge can publish if every check succeeds.

## Related records

- [Active requirements](../../specs/release/requirements/npm-publication-reliability.md).
- [System design](../../specs/release/system-design/npm-publication-reliability.md#exchange-response-diagnostics).
- [Original implementation](../npm-publication-reliability/task-03-early-oidc-preflight.md).

The original package remains complete. Its test results describe its historical revision.

The [creation metadata compatibility repair](../npm-preflight-created-metadata/plan.md) addresses the `invalid_created` failure exposed by these diagnostics.
