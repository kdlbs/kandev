---
id: "01-exchange-acceptance"
title: "Match the npm exchange acceptance contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-RELEASE-NPM-PUBLICATION-003
acceptance_criteria:
  - AC-RELEASE-NPM-PUBLICATION-003.1
  - AC-RELEASE-NPM-PUBLICATION-003.2
  - AC-RELEASE-NPM-PUBLICATION-003.3
  - AC-RELEASE-NPM-PUBLICATION-003.4
  - AC-RELEASE-NPM-PUBLICATION-003.5
  - AC-RELEASE-NPM-PUBLICATION-003.6
  - AC-RELEASE-NPM-PUBLICATION-003.7
system_design:
  - ../../specs/release/system-design/npm-publication-reliability.md
---

# Task 01: Match the npm Exchange Acceptance Contract

## Summary

Accept a successful package exchange with a nonempty token string without requiring descriptive metadata.
Keep identity-token validation, publisher rejection, transport safety, and credential privacy effective.

## In scope

- Remove `token_type`, creation-time, and expiry metadata conditions from `exchangePackageToken`.
- Remove exchange-specific time parsing and unused clock plumbing.
- Omit `expires_at` and all response metadata from successful package summaries.
- Replace metadata-rejection fixtures with accepted token-only and arbitrary-metadata responses across the full inventory.
- Move CLI failure/redaction fixtures to an invalid token and add successful CLI summary coverage.
- Update public release instructions and delivery results.

## Out of scope

- Removing GitHub identity-token expiry or audience checks.
- Accepting HTTP rejection, invalid JSON/body shape, or missing, empty, or non-string tokens.
- Changing registry, redirect, timeout, protected context, SDK absence, or workflow gates.
- Publication writes, live exchanges, new workflow modes, and new dependencies.

## Acceptance

1. Every required package accepts token-only HTTP 201 responses and ignores response metadata, including unexpected metadata values.
2. Expired or invalid GitHub identity tokens, rejected publishers, unsafe transport, malformed bodies, and invalid token values still block preparation.
3. CLI summaries identify package/status or safe failure context without credentials, metadata, or a verified-lifetime claim.

## TDD cases

Start with `token-only package exchanges pass the full preflight`.
Run it before production edits and observe the current `invalid_token_type` rejection.

Use serialized HTTP JSON with a fixed clock for GitHub identity-token validation.
Cover all application/runtime packages and the SDK when present.
Include absent, null, malformed, numeric, object, and past-date metadata across `token_type`, `created`, and `expires`.
Keep the documented full-metadata success response.

Retain a mixed inventory with healthy token-only responses and one HTTP rejection.
Retain unreadable JSON, null/array body, missing/empty/non-string token, missing runtime package, and unreadable SDK registry cases.
Preserve the separate expired GitHub identity-token case and prove no package exchange occurs after that failure.
Preserve audience, context, redirect, timeout, and SDK-deferral tests.

Replace `invalid_expires` failure-summary triggers with a missing or invalid token.
Add a successful CLI token-only response and inspect actual emitted JSON.
Assert that neither success nor failure output contains tokens, authorization credentials, ignored metadata, or `expires_at`.
Do not add test-only production exports or introduce a new runner abstraction.

## Verification

Run from the repository root.

```bash
node --test scripts/release/npm-auth-preflight.test.mjs scripts/release/sdk-runtime.test.mjs scripts/release/sdk-registry.test.mjs
python3 .github/scripts/release-workflow-contract_test.py
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run Prettier on modified scripts and the documentation coverage preflight with all referenced artifacts.

## Files likely touched

- `scripts/release/npm-auth-preflight.mjs`
- `scripts/release/npm-auth-preflight.test.mjs`
- `docs/public/release-process.md`
- This work order and its plan.

## Dependencies

None. The merged creation-only repair is present at the incident revision.

## Risks

- Removing obsolete metadata fixtures without replacing their failure trigger can erase redaction coverage.
- The ignored exchange expiry must not be confused with the mandatory GitHub identity-token expiry check.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/release/requirements/npm-publication-reliability.md), including `AC-RELEASE-NPM-PUBLICATION-003.7`.
- [Design](../../specs/release/system-design/npm-publication-reliability.md#early-authentication-gate).
- [Incident and provider evidence](plan.md#evidence-and-root-cause).
- Existing HTTP transport and CLI subprocess fixtures in `npm-auth-preflight.test.mjs`.

## Results

Implemented by the user-selected Luna subagent with maximum reasoning.
The full-inventory token-only regression failed with `invalid_token_type` before the parser change.
It passed after removing metadata conditions and exchange-specific clock plumbing.

- Release Node tests: 25 passed.
- Release workflow contract tests: 78 passed.
- Public documentation tests: 63 passed; 55 pages validated.
- Documentation catalog: 449 decisions and 1984 specifications validated.
- Full specification lint, script formatting, documentation coverage, and `git diff --check`: passed.

Coverage includes the full package inventory, ignored metadata, malformed tokens, identity-token validation, publisher rejection, and CLI credential exclusion.
No live exchange or publication was run.
