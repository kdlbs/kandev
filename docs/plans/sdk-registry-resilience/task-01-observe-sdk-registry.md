---
id: "01-observe-sdk-registry"
title: "Make SDK registry reads resilient"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-RELEASE-NPM-PUBLICATION-001
  - REQ-RELEASE-NPM-PUBLICATION-002
acceptance_criteria:
  - AC-RELEASE-NPM-PUBLICATION-001.1
  - AC-RELEASE-NPM-PUBLICATION-001.2
  - AC-RELEASE-NPM-PUBLICATION-001.3
  - AC-RELEASE-NPM-PUBLICATION-001.4
  - AC-RELEASE-NPM-PUBLICATION-001.6
  - AC-RELEASE-NPM-PUBLICATION-002.1
  - AC-RELEASE-NPM-PUBLICATION-002.2
system_design:
  - ../../specs/release/system-design/npm-publication-reliability.md
---

# Task 01. Make SDK registry reads resilient

## Summary

Give delayed npm SDK visibility and `latest` updates a ten-minute window.
Retry transient reads and check once at the deadline.
Give the SDK publication job 30 minutes.

## In scope

- Registry error classification and observation using the existing clock and delay.
- Terminal read diagnostics and the protected SDK job timeout.
- Timing and retry wording in the release procedure.

## Out of scope

- Retained upload attempt evidence. Task 02 owns this work.
- Changes to application package publication or credential handling.

## Acceptance

- A version that becomes visible after 309 seconds completes with one upload. A matching version or dist-tag can complete on the final probe.
- Network timeouts, HTTP 429, and HTTP 5xx responses can recover within the window. Exhausted reads, confirmed absence, and definitive errors have distinct results.
- The workflow job has a 30-minute timeout. Tests cover the deadline and preserve the single-write and verified identity checks.

## Verification

Run from the repository root.

```bash
node --test scripts/release/publish-plugin-sdk.test.mjs scripts/release/sdk-registry.test.mjs scripts/release/sdk-diagnostics.test.mjs
python3 .github/scripts/release-workflow-contract_test.py
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `scripts/release/sdk-publication-state.mjs`
- `scripts/release/sdk-registry.mjs`
- `scripts/release/sdk-diagnostics.mjs`
- `scripts/release/publish-plugin-sdk.test.mjs`
- `scripts/release/sdk-registry.test.mjs`
- `scripts/release/sdk-diagnostics.test.mjs`
- `.github/workflows/release.yml`
- `.github/scripts/release-workflow-contract_test.py`
- `docs/public/release-process.md`

## Dependencies

None.

## Risks

- The final request retains its own deadline. Test that a request already in progress can serve as the final probe.
- A transient HTTP response can contain an HTML body. Classify its status before parsing metadata.

## Parallelism

`sequential`

## Inputs

- [Registry observation design](../../specs/release/system-design/npm-publication-reliability.md#registry-observation).
- The fake clock in `scripts/release/publish-plugin-sdk.test.mjs`.

## Results

Passed:

- `node --test scripts/release/publish-plugin-sdk.test.mjs scripts/release/sdk-registry.test.mjs scripts/release/sdk-diagnostics.test.mjs` (53 tests)
- `python3 .github/scripts/release-workflow-contract_test.py` (78 tests)
- `node --test scripts/validate-public-docs.test.mjs` (63 tests)
- `node scripts/validate-public-docs.mjs` (56 published pages)
- `git diff --check`
