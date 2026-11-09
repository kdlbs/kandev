---
id: "02-publication-diagnostics"
title: "Report safe publication diagnostics"
status: completed
wave: 2
depends_on:
  - "01-registry-visibility"
plan: "plan.md"
requirements:
  - REQ-RELEASE-NPM-PUBLICATION-002
acceptance_criteria:
  - AC-RELEASE-NPM-PUBLICATION-002.1
  - AC-RELEASE-NPM-PUBLICATION-002.2
  - AC-RELEASE-NPM-PUBLICATION-002.3
  - AC-RELEASE-NPM-PUBLICATION-002.4
  - AC-RELEASE-NPM-PUBLICATION-002.5
system_design:
  - ../../specs/release/system-design/npm-publication-reliability.md
---

# Task 02: Report Safe Publication Diagnostics

## Summary

Expose the failed SDK phase and safe recovery action.
Report accepted uploads separately from confirmed public versions.

## In scope

- Add the small `sdk-diagnostics.mjs` reporter and its tests.
- Connect observation events and both successful and failed npm command results.
- Classify authentication, registry, visibility, identity, and consumer failures.
- Write success and failure summaries before cleanup.
- Document propagation, expired trust, and same-version recovery in the release operating guide.

## Out of scope

- Raw npm debug-log retention, new artifact uploads, and metrics services.
- Authentication preflight and notification policy changes.

## Acceptance

1. Each failure reports its category, last confirmed phase, and recovery action without falsely declaring availability.
2. Logs and summaries contain bounded safe output and exclude every secret in the negative fixtures.
3. A runner failure writes its summary before cleanup and preserves its original nonzero result if summary writing also fails.

## TDD cases

Start with `accepted upload visibility timeout reports upload success and pending registry state`.
Add `consumer failure preserves confirmed publication and channel promotion`.
Use a fixture npm command to exercise the real command adapter's stdout, stderr, and rejection path.
Cover known credentials, bearer headers, npmrc tokens, JWTs, signed URLs, output truncation, and summary-write failure.
Add an uncertain upload case whose summary does not claim that publication failed or succeeded.

## Verification

Use the SDK's pinned Node/npm versions for real candidate tests.
Run from the repository root.

```bash
node --test scripts/release/sdk-diagnostics.test.mjs scripts/release/publish-plugin-sdk.test.mjs scripts/release/sdk-release-io.test.mjs scripts/release/sdk-runtime.test.mjs
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `scripts/release/sdk-diagnostics.mjs` (new)
- `scripts/release/sdk-diagnostics.test.mjs` (new)
- `scripts/release/sdk-publication-state.mjs`
- `scripts/release/sdk-release-io.mjs`
- `scripts/release/publish-plugin-sdk.mjs`
- `scripts/release/publish-plugin-sdk.test.mjs`
- `scripts/release/sdk-release-io.test.mjs`
- `docs/public/release-process.md`

## Dependencies

Task 01 supplies the registry observer and its timing events.

## Risks

- Filtering only the final printed error leaves captured subprocess data exposed to other consumers.
- Summary handling must not replace the original publication error.
- Recovery guidance must retain immutable SDK evidence and the distinction between authentication and registry delay.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/release/requirements/npm-publication-reliability.md), `REQ-RELEASE-NPM-PUBLICATION-002`.
- [Design](../../specs/release/system-design/npm-publication-reliability.md), Diagnostics and summaries and Credential handling.
- `runRelease`, `ReleaseIO.npm`, and the existing state-machine fixtures.

## Results

Completed on 2026-11-09.

- Added `scripts/release/sdk-diagnostics.mjs` with phase tracking, accepted/uncertain/rejected write outcomes, bounded observation events, failure classification, and recovery guidance.
- Connected the state machine, `ReleaseIO.npm`, consumer verification, and `runRelease` success and failure summaries. Summaries are written before temporary-directory cleanup, and a summary-write failure cannot replace the original release error.
- Sanitized successful and failed npm output before retention. Known environment secrets, bearer and basic headers, npmrc authentication lines, JWTs, npm and GitHub tokens, and credential-bearing URLs are removed. Each command keeps at most 64 KiB of sanitized tail with explicit truncation.
- Distinguished authentication, accepted-upload visibility, uncertain upload, registry read, identity or provenance, and consumer-verification failures. A consumer failure preserves confirmed publication and channel promotion.
- Review follow-up: redacted complete Basic and Proxy-Authorization values, preferred typed npm authentication codes over command-line text such as `--provenance`, and kept confirmed immutable publication when only the `latest` dist-tag fails. Channel write outcomes and channel-specific recovery guidance are reported separately.
- Documented the five-minute propagation window, diagnostics, expired publisher recovery, and same-version recovery in `docs/public/release-process.md`.
- Verification with Node 24.19.0 and npm 11.21.0:
  - `node --test scripts/release/sdk-diagnostics.test.mjs scripts/release/publish-plugin-sdk.test.mjs scripts/release/sdk-release-io.test.mjs scripts/release/sdk-runtime.test.mjs`: 44 tests passed.
  - `node --test scripts/validate-public-docs.test.mjs`: 63 tests passed.
  - `node scripts/validate-public-docs.mjs`: 55 published docs pages validated.
  - `git diff --check`: passed.
