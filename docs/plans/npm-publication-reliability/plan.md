---
created: 2026-11-09
status: completed
requirements:
  - REQ-RELEASE-NPM-PUBLICATION-001
  - REQ-RELEASE-NPM-PUBLICATION-002
  - REQ-RELEASE-NPM-PUBLICATION-003
system_design:
  - ../../specs/release/system-design/npm-publication-reliability.md
legacy_specs: []
---

# Implementation Plan: npm Publication Reliability

## Overview

Improve SDK registry observation and diagnostics first.
Then add early authentication for normal Stable releases.
Execute three work orders sequentially because they share the release adapters, workflow, and operating guide.

The release system owns this package because it owns publication infrastructure and maintainer recovery.
The existing plugin specification remains authoritative for SDK identity and compatibility.

## Evidence and assumption check

The user requested all four proposed improvements: longer polling, clear diagnostics, regression coverage, and early authentication.
This package includes regression coverage within each implementation work order.
It adds no separate generic verification phase.

- Baseline: `1529e72ed5b8ccd3bc6fd79498a12d57492ec120`.
- [Release attempt 1](https://github.com/kdlbs/kandev/actions/runs/42540137466/attempts/1) failed with `ENEEDAUTH`.
  The npm package page showed an expired SDK publisher.
- [Attempt 2](https://github.com/kdlbs/kandev/actions/runs/42540137466/attempts/2) accepted the upload.
  The visibility check stopped at 10:56:34 UTC.
  npm records publication at 10:57:19 UTC.
- [Attempt 3](https://github.com/kdlbs/kandev/actions/runs/42540137466/attempts/3) passed public consumers and Stable completion.
  The public archive matched the retained receipt and carried attempt-2 provenance.
- `observe` currently makes three reads with 10-second and 20-second waits.
  The same helper also observes signed GitHub tags.
- `ReleaseIO.npm` captures command output, but its callers discard successful output.
  `runRelease` writes its normal summary only after success or explicit bootstrap.
- Existing tests cover immediate visibility, uncertain writes, immutable conflicts, bootstrap, and no downgrade.
  They do not reproduce visibility beyond the current three-read limit.

The five-minute observation window is settled by the prior recommendation.
The early gate applies only to normal Stable bumps.
This scope preserves validation capabilities and the established backfill recovery path.
No material question blocks the package.

## Scope

### In scope

- Separate SDK registry observation from GitHub tag recovery.
- Observe SDK version and dist-tag propagation for five minutes.
- Preserve one write per operation, immutable identity, and public consumer gates.
- Write bounded, sanitized command diagnostics and phase summaries on failure.
- Authenticate every existing normal Stable npm package before release preparation.
- Add deterministic delay, error, redaction, and workflow-mode regressions.
- Update the release operating guide in the same implementation package.

### Out of scope

- Live release dispatch, npm publication, package settings, or credential rotation.
- New release versions, SDK bytes, receipt schemas, or signing policy.
- New retry policies for runtime/Nightly publishers or GitHub tag creation.
- Early OIDC checks for dry runs, desktop validation, Nightly, or backfill.
- Changes to Discord and contributor delivery policy.
- Product UI and browser tests.

## Technical approach

Task 01 updates `sdk-publication-state.mjs` and `ReleaseIO` with registry-only deadline observation.
Keep the GitHub tag and consumer retry policies intact.
Inject time and sleep for deterministic tests.

Task 02 adds `sdk-diagnostics.mjs` and connects it to command, observation, and `runRelease` boundaries.
Write failure summaries before cleanup.
Classify accepted, uncertain, and rejected writes without inventing publication success.

Task 03 adds `npm-auth-preflight.mjs` and the isolated `npm-auth-preflight` workflow job.
Read the canonical runtime inventory through `npm-packages.sh`.
Use the package-scoped OIDC exchange rather than a token-dependent identity command.
Make `prepare` explicitly handle successful and intentionally skipped dependencies.

### Compatibility matrix

| Boundary | Intended behavior | Evidence | Unsupported result |
| --- | --- | --- | --- |
| npm accepted upload, delayed version | Poll within five minutes | Virtual-time state-machine regression | Timeout remains incomplete |
| npm uncertain write | Reconcile reads after one write | Recorded-write fixture | Identity conflict stops |
| npm stale `latest` | Observe the channel separately | Delayed-channel regression | No downgrade |
| GitHub SDK tag recovery | Existing bounds | Existing publication tests | Existing failure |
| Normal Stable OIDC | Exchange for every existing required package | HTTP fixture and job guards | Package-specific setup failure |
| SDK package absent | Defer early SDK check | Whole-package-absence fixture | Existing bootstrap gate |
| Validation, Nightly, backfill | Existing mode paths | Raw workflow matrix | No new credential requirement |

## Tests

| Acceptance criteria | Regression evidence |
| --- | --- |
| `001.1`, `001.2` | `publish-plugin-sdk.test.mjs`: delayed version and delayed `latest` at 90 seconds |
| `001.3`, `001.5` | Same suite: uncertain delayed response and rerun without another upload |
| `001.4` | Existing conflict/provenance/consumer tests plus errors during observation |
| `002.1`, `002.2` | `sdk-diagnostics.test.mjs`: attempt events and distinct failure categories |
| `002.3`, `002.5` | Runner fixture: phase summary before cleanup on each failure |
| `002.4` | Reporter and command fixtures: secret redaction, output limits, summary failure |
| `003.1`, `003.2` | `npm-auth-preflight.test.mjs`: package-specific exchanges and mixed healthy/rejected inventory |
| `003.3` | `release-workflow-contract_test.py`: all workflow modes and cancellation |
| `003.4` | Authentication fixtures: SDK absence, runtime absence, unreadable registry |
| `003.5`, `003.6` | Authentication fixtures: context, redirects, token lifecycle, safe summaries |

All abbreviated criterion numbers refer to `AC-RELEASE-NPM-PUBLICATION-<number>`.
Test names in work orders are intended new cases, rather than claims of existing coverage.
Use the original SDK Node/npm pins for suites that build real candidates.

## Work orders

- [x] [Task 01: Observe delayed SDK registry state](task-01-registry-visibility.md).
- [x] [Task 02: Report safe publication diagnostics](task-02-publication-diagnostics.md).
- [x] [Task 03: Gate normal Stable releases on npm authentication](task-03-early-oidc-preflight.md).

Run Task 01, then Task 02, then Task 03.
No subagents are authorized.

## Verification results

Design validation passed on 2026-11-09:

- `python3 scripts/list-docs.py validate`: passed, with 439 decisions and 1948 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: passed for all three work orders and six proposed production script paths.
  The proposed workflow path retains its existing CI-infrastructure exemption.
- `git diff --check -- docs/specs docs/plans/npm-publication-reliability`: passed.
- Document size and authoring checks: passed, with no invisible characters or oversized specifications.

Implementation verification passed on 2026-11-09 with the SDK's pinned Node 24.19.0 and npm 11.21.0:

- `node --test scripts/release/*.test.mjs`: 156 tests passed.
- Task 01 suite: `node --test scripts/release/publish-plugin-sdk.test.mjs scripts/release/sdk-release-io.test.mjs scripts/release/sdk-registry.test.mjs`: 36 tests passed.
- Task 02 suite: `node --test scripts/release/sdk-diagnostics.test.mjs scripts/release/publish-plugin-sdk.test.mjs scripts/release/sdk-release-io.test.mjs scripts/release/sdk-runtime.test.mjs`: 49 tests passed.
- Task 03 suite: `node --test scripts/release/npm-auth-preflight.test.mjs scripts/release/sdk-runtime.test.mjs scripts/release/sdk-registry.test.mjs`: 17 tests passed.
- `python3 .github/scripts/release-workflow-contract_test.py`: 78 tests passed.
- `python3 .github/scripts/lint-action-pinning_test.py`: 9 tests passed.
- `python3 .github/scripts/plugin-sdk-ci-contract_test.py`: 10 tests passed.
- `python3 .github/scripts/backend-tests-workflow-contract_test.py`: 26 tests passed.
- `python3 .github/scripts/lint-action-pinning.py`: all 30 workflow files use SHA-pinned action refs.
- `node --test scripts/validate-public-docs.test.mjs`: 63 tests passed.
- `node scripts/validate-public-docs.mjs`: 55 published docs pages validated.
- `python3 scripts/list-docs.py validate`: passed, with 439 decisions and 1948 specifications.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check`: passed.

`zizmor .github/workflows` reported the existing repository baseline, including the pre-existing `superfluous-actions` finding for `softprops/action-gh-release` in `release.yml`. The new `npm-auth-preflight` job introduces no finding.

Production and permanent test implementation are complete.
Each work order records its exact implementation checks and results.

## Risks

- An observation deadline must not repeat a write or accept mismatched bytes.
- Diagnostic output can expose credentials unless filtering precedes retention and error propagation.
- Adding a skipped dependency can suppress downstream jobs through GitHub's implicit status condition.
- An OIDC exchange proves authentication, rather than allowed direct-publish or dist-tag actions.
- Publisher trust can expire after a successful early check.
  The real publication job remains responsible for its own authentication.
- Existing workflow runs use their original workflow revision.
  Deployment of this fix requires a future run from the merged revision.

## Related packages

- [Original SDK publication](../plugin-sdk-publishing/task-04-stable-publication.md).
- [Immutable SDK recovery](../immutable-release-sdk-recovery/plan.md).

Their completed results remain historical evidence.
This package does not reopen their work orders or change their contracts.

The [preflight response diagnostics repair](../npm-preflight-response-diagnostics/plan.md) addresses the later failure in run `43038508616`.
This package's completed verification remains historical evidence.
