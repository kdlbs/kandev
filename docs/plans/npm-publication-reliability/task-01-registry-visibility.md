---
id: "01-registry-visibility"
title: "Observe delayed SDK registry state"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-RELEASE-NPM-PUBLICATION-001
acceptance_criteria:
  - AC-RELEASE-NPM-PUBLICATION-001.1
  - AC-RELEASE-NPM-PUBLICATION-001.2
  - AC-RELEASE-NPM-PUBLICATION-001.3
  - AC-RELEASE-NPM-PUBLICATION-001.4
  - AC-RELEASE-NPM-PUBLICATION-001.5
system_design:
  - ../../specs/release/system-design/npm-publication-reliability.md
---

# Task 01: Observe Delayed SDK Registry State

## Summary

Replace the SDK's short npm observation limit with a five-minute deadline.
Preserve GitHub recovery bounds and one write per operation.

## In scope

- Follow the design's Registry observation and Recovery and verification sections.
- Add registry-specific clock and sleep injection.
- Apply bounded observation to SDK version visibility and `latest` promotion.
- Preserve existing bootstrap, evidence, provenance, and consumer checks.
- Add fake-time regressions alongside the publication state-machine tests.

## Out of scope

- Reporter changes, early authentication, and workflow dependency changes.
- Runtime/Nightly publication and GitHub tag retry policy.

## Acceptance

1. A version or `latest` that appears after 90 simulated seconds completes successfully within its observation window.
2. Pending state exhausts the five-minute deadline with bounded waits and no repeated write.
3. Uncertain responses, later reruns, and conflicting or unreadable state preserve the existing publication safeguards.

## TDD cases

Start with `npm version visible after 90 seconds completes without a second upload`.
Confirm that it fails under the current three-read observer.
Add delayed channel visibility, permanent absence, uncertain delayed writes, and no-new-probe-after-deadline cases.
Retain the four immutable-state combinations and the newer-channel test.
A registry read error must fail distinctly from confirmed absence.

## Verification

Use the SDK's pinned Node/npm versions from `scripts/pluginsdk/pack-config.json`.
The adapter suite builds a real candidate and needs those versions.

```bash
node --test scripts/release/publish-plugin-sdk.test.mjs scripts/release/sdk-release-io.test.mjs scripts/release/sdk-registry.test.mjs
git diff --check
```

## Files likely touched

- `scripts/release/sdk-publication-state.mjs`
- `scripts/release/sdk-release-io.mjs`
- `scripts/release/publish-plugin-sdk.test.mjs`
- `scripts/release/sdk-release-io.test.mjs`
- `scripts/release/sdk-registry.test.mjs`

## Dependencies

None.

## Risks

- Fake time must cover the real observer, rather than a test-only retry loop.
- The monotonic deadline must not change the separate GitHub observer.
- No test or verification command may publish to a real registry.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/release/requirements/npm-publication-reliability.md), `REQ-RELEASE-NPM-PUBLICATION-001`.
- [Design](../../specs/release/system-design/npm-publication-reliability.md), Registry observation and Recovery and verification.
- Existing `fixture` and recorded-write checks in `publish-plugin-sdk.test.mjs`.

## Results

Completed on 2026-11-09.

- Added a registry-only observer with a 300,000 ms deadline. It probes immediately, then waits 10, 20, and at most 30 seconds between reads, caps each wait at the remaining budget, and starts no probe at or after the deadline.
- Kept the GitHub tag observer and public-consumer retries on their existing three-attempt bounds.
- Injected a monotonic clock and registry sleep through `ReleaseIO`; tests use virtual time.
- Preserved one write per operation. A definite authentication rejection stops after one reconciliation read, while transport and identity errors remain errors.
- Added delayed version, delayed `latest`, permanent absence, deadline, read-error, authentication-rejection, and uncertain-write regressions. Existing immutable-state, bootstrap, provenance, no-downgrade, and consumer tests remain green.
- Review follow-up: preserved the verified version in the adapter registry projection so visible observation events and success summaries report the actual version.
- PR review follow-up: start source verification before source loading, keep candidate verification active through finalization, mark verify-mode registry reads under version visibility, include structured npm codes in authentication checks, bound registry diagnostic values, and classify registry transport and response-body failures.
- Verification with Node 24.19.0 and npm 11.21.0:
  - `node --test scripts/release/publish-plugin-sdk.test.mjs scripts/release/sdk-release-io.test.mjs scripts/release/sdk-registry.test.mjs`: 36 tests passed.
  - `node --test scripts/release/*.test.mjs`: 156 tests passed.
  - `git diff --check`: passed.
