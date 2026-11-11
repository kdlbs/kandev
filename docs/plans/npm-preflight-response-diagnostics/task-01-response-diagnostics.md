---
id: "01-response-diagnostics"
title: "Identify npm exchange-response failures safely"
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

# Task 01: Identify npm Exchange-Response Failures Safely

## Summary

Report the exact local validation condition that rejects a successful npm token exchange.
Keep credential values private and preserve the current authentication gate.

## In scope

- Fixed reasons for unreadable JSON, invalid body shape, invalid token type, missing token, invalid creation date, invalid expiry, and expired token.
- Structured HTTP status and reason on `NpmAuthPreflightError`.
- `npm_exchange_response` for malformed successful responses.
- Existing authentication category for HTTP rejection and expired tokens.
- Failure summaries with the safe reason, package, HTTP status, workflow SHA, and checkout SHA.
- TDD at the transport and emitted-summary boundaries.
- Release-guide recovery instructions for the distinct errors.

## Out of scope

- Relaxing any existing successful-response requirement.
- Editing `.github/workflows/release.yml` or adding an authentication bypass.
- Live token exchange, publication, release dispatch, signing, or publisher configuration changes.
- Recording raw response bodies, token fragments, or arbitrary response field values.

## Acceptance

1. Each response condition produces its own fixed diagnostic reason in the error and emitted failure summary.
2. Existing valid responses pass, every existing invalid response fails, and one failed package still blocks preparation.
3. Sentinel credentials and malicious metadata never appear in diagnostics, and the guide names the appropriate recovery path.

## TDD cases

Start with `HTTP 201 with a missing creation date reports invalid_created instead of publisher rejection`.
The current code must fail because it reports the generic authentication category and lacks a specific reason.

Cover each field separately, including missing, null, wrong-type, malformed, and expired values.
Use a fixed clock and include expiry equal to the current time.
Include an invalid body, unreadable JSON, HTTP rejection, and the documented valid response.
Assert HTTP status 201 for local validation errors.
Assert both source revisions on failure.

Exercise the actual summary serialization boundary.
Put credential sentinels in unexpected metadata and transport exception messages.
Assert their absence from all retained messages and summaries.
Retain the existing mixed-package, SDK-deferral, redirect, request-bound, and context cases.

## Verification

Run from the repository root after implementation.

```bash
node --test scripts/release/npm-auth-preflight.test.mjs scripts/release/sdk-runtime.test.mjs scripts/release/sdk-registry.test.mjs
python3 .github/scripts/release-workflow-contract_test.py
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the repository documentation coverage preflight with the actual changed production paths and these linked artifacts.

## Files likely touched

- `scripts/release/npm-auth-preflight.mjs`
- `scripts/release/npm-auth-preflight.test.mjs`
- `docs/public/release-process.md`
- `docs/plans/npm-preflight-response-diagnostics/plan.md`
- `docs/plans/npm-preflight-response-diagnostics/task-01-response-diagnostics.md`

## Dependencies

None. The original preflight implementation is present at the incident revision.

## Risks

- A generic error cannot identify the actual rejected response field in the original run.
- A diagnostic that includes response values can expose credentials.
- A changed validation predicate can weaken the release gate.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/release/requirements/npm-publication-reliability.md), `REQ-RELEASE-NPM-PUBLICATION-003`.
- [System design](../../specs/release/system-design/npm-publication-reliability.md#exchange-response-diagnostics).
- [Incident evidence and recovery](plan.md#evidence-and-assumption-check).
- Existing transport fixtures in `scripts/release/npm-auth-preflight.test.mjs`.
- `/tdd`, `/docs-maintainer`, and the public documentation guide.

## Results

Completed on 2026-11-11 by the user-selected Luna subagent under primary-session coordination.

- The first regression failed before production edits because a local HTTP 201 validation error had no HTTP status.
- Added fixed reasons and `npm_exchange_response` for malformed successful responses.
  HTTP rejection and expired tokens retain `npm_authentication`.
- Preserved the token acceptance predicate, public registry checks, redirect rejection, request bounds, and context gates.
- Added subprocess coverage for the real CLI summary, safe source revisions, recovery guidance, and credential redaction.
- Publisher and expired-token guidance regressions failed before their fixed recovery messages were restored.
  Known preflight errors retain safe messages. Unexpected exceptions use a fixed fallback.
- Updated the release guide with authentication, response-validation, and rerun recovery instructions.
- `node --test scripts/release/npm-auth-preflight.test.mjs scripts/release/sdk-runtime.test.mjs scripts/release/sdk-registry.test.mjs`: 21 passed after the final production edit.
- `python3 .github/scripts/release-workflow-contract_test.py`: 78 passed.
- `node --test scripts/validate-public-docs.test.mjs`: 63 passed.
- `node scripts/validate-public-docs.mjs`: 55 published pages validated.
- `python3 scripts/list-docs.py validate`: 448 decisions and 1982 specifications passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Documentation coverage preflight: covered with zero errors for the actual changed paths.
- Prettier and `git diff --check`: passed.

The exact response incompatibility in the original release remains unknown.
No live authentication, publication, or release dispatch occurred.
