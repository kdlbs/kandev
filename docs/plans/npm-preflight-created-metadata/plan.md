---
created: 2026-11-11
status: completed
requirements:
  - REQ-RELEASE-NPM-PUBLICATION-003
system_design:
  - ../../specs/release/system-design/npm-publication-reliability.md
legacy_specs: []
---

# Implementation Plan: npm Creation Metadata Compatibility

## Overview

Stop unused creation metadata from blocking an otherwise valid npm authentication check.
One work order removes the creation-time requirement and preserves token, expiry, context, and workflow guards.
The release system owns this repair under the existing early-authentication requirement.

## Confirmed cause and evidence

- [Release run 43068047253](https://github.com/kdlbs/kandev/actions/runs/43068047253) used revision `cfbce14a1cdbf780a4771b810677b896f8a41adb`.
- [Job 130688170086](https://github.com/kdlbs/kandev/actions/runs/43068047253/job/130688170086) failed for `kandev` with HTTP 201, category `npm_exchange_response`, and reason `invalid_created`.
- The helper already accepted the response object, OIDC token type, and nonempty token before this condition failed.
- `Date.parse(state.created)` rejected the creation metadata. The helper does not use that timestamp afterward.
- A temporary in-memory reproduction accepts the normal fixture and rejects missing or unparseable creation metadata despite a valid future expiry.
- [npm CLI 11.21.0](https://github.com/npm/cli/blob/v11.21.0/lib/utils/oidc.js) consumes the returned token without a creation-time check.
- The release PR, tag, builds, and publications were skipped. No backfill is necessary for this attempt.

The exact creation value remains private and unknown. The trace identifies the failed condition without requiring that value.
Expiry validation was not reached in the live run. Its format and the remaining package checks remain unverified.
The earlier diagnostic package remains complete and supplies this evidence.

## Scope

### In scope

- Ignore `created` during package token exchange validation.
- Accept absent, null, malformed, or differently formatted creation metadata when token and expiry checks pass.
- Replace creation-rejection tests with acceptance coverage for every required package.
- Move failure-summary and redaction fixtures to an active validation error.
- Explain the creation-metadata boundary in the release guide.

### Out of scope

- Changes to HTTP status, registry identity, token type, token presence, or expiry requirements.
- Changes to context checks, SDK absence handling, workflow modes, or release dependencies.
- Token fallbacks, publisher changes, live token exchanges, or release dispatch.
- SDK packaging changes, new dependencies, and product UI.

## Technical approach

Remove the unused creation-date parsing and `invalid_created` rejection from `exchangePackageToken`.
Keep expiry parsing and its expired-token rejection intact.
Keep creation metadata out of all summaries and messages.

Update `npm-auth-preflight.test.mjs` at the helper, full inventory, and CLI summary boundaries.
Use a fixed clock and a valid future expiry for creation-metadata acceptance fixtures.
Keep a mixed inventory with healthy packages and one HTTP rejection or expired token.
The universal gate still requires every non-deferred package to pass.

## Tests

| Criteria | Evidence |
| --- | --- |
| `AC-RELEASE-NPM-PUBLICATION-003.1`, `AC-RELEASE-NPM-PUBLICATION-003.2` | Every required package authenticates without creation metadata. One rejected package still blocks the gate. |
| `AC-RELEASE-NPM-PUBLICATION-003.3`, `AC-RELEASE-NPM-PUBLICATION-003.4` | Existing workflow-mode and SDK-absence regressions remain effective. |
| `AC-RELEASE-NPM-PUBLICATION-003.5`, `AC-RELEASE-NPM-PUBLICATION-003.6` | Active failure summaries preserve safe context and exclude credentials and ignored metadata. |

## Work orders

- [x] [Task 01: Ignore creation metadata in npm authentication](task-01-ignore-creation-metadata.md).

The work order is sequential. The primary session owns integration and delivery records.
The user authorized delegated implementation, PR publication, a 15-minute hold, and PR fixup on 2026-11-11.

## Verification results

The temporary reproduction made no network requests or permanent test edits.
Its missing and malformed `created` cases reproduced `invalid_created` with HTTP 201.
Implementation and task-defined checks are complete.

Design validation passed on 2026-11-11:

- `python3 scripts/list-docs.py validate`: 449 decisions and 1984 specifications passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: covered with zero errors for this package and a simulated script change.
  No production path changed during planning.
- `git diff --check`: passed.
- Authoring checks found no sentence above 25 words in the new delivery documents.
- The design handoff left the delivery documents unstaged and uncommitted on `fix/npm-preflight-created-metadata`, based on the merged diagnostic revision.

Implementation verification passed on 2026-11-11:

- The full-inventory regression failed with `invalid_created` before production edits.
- The production repair removes only creation-time parsing and rejection.
- All seven package exchanges accept absent, null, malformed, numeric, and object creation metadata when token and expiry checks pass.
- Mixed healthy and rejected publishers still block preparation at the rejected package.
- The CLI redaction fixture now rejects `invalid_expires` and retains exact safe summary assertions.
- `node --test scripts/release/npm-auth-preflight.test.mjs scripts/release/sdk-runtime.test.mjs scripts/release/sdk-registry.test.mjs`: 22 passed.
- `python3 .github/scripts/release-workflow-contract_test.py`: 78 passed.
- `node --test scripts/validate-public-docs.test.mjs`: 63 passed.
- `node scripts/validate-public-docs.mjs`: 55 published pages validated.
- `python3 scripts/list-docs.py validate`: 449 decisions and 1984 specifications passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Documentation coverage preflight for the actual changed paths: covered with zero errors.
- Prettier and `git diff --check`: passed.

The user-selected GPT-6 Luna subagent at max reasoning implemented the owned script, tests, and public guide.
The primary session coordinated integration and delivery records.
PR publication, the requested 15-minute hold, and fixup use live GitHub delivery evidence.
No live token exchange, release dispatch, or publication occurred during implementation.

## Risks and recovery

- Expiry or another package can expose a separate problem after this condition passes.
- Ignored metadata must never enter diagnostics through fixture or summary changes.
- Expiry, token, and context failures must remain explicit failures.
- A new Stable run from merged `main` uses the correction. Reruns retain their original helper revision.
- A successful preflight can proceed into publication. Live dispatch remains outside this package.

## Related records

- [Requirements](../../specs/release/requirements/npm-publication-reliability.md).
- [System design](../../specs/release/system-design/npm-publication-reliability.md#early-authentication-gate).
- [Completed diagnostic repair](../npm-preflight-response-diagnostics/plan.md).
