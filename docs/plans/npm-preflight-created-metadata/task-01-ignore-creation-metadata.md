---
id: "01-ignore-creation-metadata"
title: "Ignore creation metadata in npm authentication"
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
system_design:
  - ../../specs/release/system-design/npm-publication-reliability.md
---

# Task 01: Ignore Creation Metadata in npm Authentication

## Summary

Accept an otherwise valid npm exchange regardless of its creation metadata.
Preserve every authentication, expiry, context, and workflow guard.

## In scope

- Remove creation-date parsing and rejection from `exchangePackageToken`.
- Replace creation-rejection fixtures with positive missing and malformed metadata cases.
- Cover all required packages through `runPreflight`, including the SDK when present.
- Preserve mixed healthy and rejected package coverage.
- Move CLI summary and redaction fixtures from `invalid_created` to `invalid_expires` or another active reason.
- Update the Early npm authentication section of the release guide.

## Out of scope

- Relaxing expiry, token, HTTP, redirect, registry, or protected-context checks.
- Editing workflow definitions or shared SDK publication logic.
- Live registry exchanges, release dispatch, and publication.

## Acceptance

1. Missing or unparseable `created` metadata cannot reject a valid token with a future expiry across the required package inventory.
2. Invalid tokens, malformed or expired expiry, HTTP rejection, and context failures still reject the preflight.
3. Failure summaries remain specific and exclude token values, ignored metadata, and raw exceptions.

## TDD cases

Start with `valid package exchanges without created metadata pass the full preflight`.
The current helper must fail with `invalid_created` before production edits.

Use the producer's serialized JSON shape and a fixed clock.
Cover absent, null, malformed string, numeric, and object creation metadata with valid tokens and future expiries.
Verify that every required package reports authenticated, with no creation value in the summary.
Keep the documented ISO creation-date success case.

Retain a mixed inventory where healthy responses omit creation metadata and one package rejects authentication.
Retain invalid body, token type, token value, missing or malformed expiry, and expiry-at-now cases.
The SDK absence case must defer only the SDK check.

Update the CLI serialization fixture to fail on an active condition after creation validation is removed.
Assert the exact package, reason, HTTP status, and source revisions, plus absence of credential and metadata sentinels.

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

Run Prettier on modified scripts and the documentation coverage preflight with the actual changed paths and linked artifacts.

## Files likely touched

- `scripts/release/npm-auth-preflight.mjs`
- `scripts/release/npm-auth-preflight.test.mjs`
- `docs/public/release-process.md`
- This work order and its plan.

## Dependencies

None. The diagnostic repair is merged in the incident revision.

## Risks

- The live trace stopped before expiry and subsequent package checks.
- Removing a diagnostic fixture without replacing its failure trigger can silently remove redaction coverage.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/release/requirements/npm-publication-reliability.md), `REQ-RELEASE-NPM-PUBLICATION-003`.
- [System design](../../specs/release/system-design/npm-publication-reliability.md#early-authentication-gate).
- [Incident evidence](plan.md#confirmed-cause-and-evidence).
- Existing transport and CLI subprocess fixtures in `npm-auth-preflight.test.mjs`.

## Results

Completed on 2026-11-11 through user-authorized delegated implementation.

- The new full-inventory regression failed with `invalid_created` before production edits.
- Removed the unused creation-time parsing and rejection.
- Added serialized HTTP fixtures for absent, null, malformed, numeric, and object creation metadata across all required packages.
- Preserved mixed healthy and rejected publishers, SDK absence, expiry, token, context, and redirect coverage.
- The CLI failure fixture now rejects `invalid_expires` and excludes token credentials, creation metadata, and unknown metadata.
- Updated the release guide with the creation-metadata boundary.
- `node --test scripts/release/npm-auth-preflight.test.mjs scripts/release/sdk-runtime.test.mjs scripts/release/sdk-registry.test.mjs`: 22 passed after the final production edit.
- `python3 .github/scripts/release-workflow-contract_test.py`: 78 passed.
- `node --test scripts/validate-public-docs.test.mjs`: 63 passed.
- `node scripts/validate-public-docs.mjs`: 55 published pages validated.
- `python3 scripts/list-docs.py validate`: 449 decisions and 1984 specifications passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Documentation coverage preflight: covered with zero errors for the actual changed paths.
- Prettier and `git diff --check`: passed.

The primary session owns PR publication and the requested hold before fixup.
No live authentication, release dispatch, or publication occurred during implementation.
