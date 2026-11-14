---
created: 2026-11-11
status: completed
requirements:
  - REQ-RELEASE-NPM-PUBLICATION-003
system_design:
  - ../../specs/release/system-design/npm-publication-reliability.md
legacy_specs: []
---

# Implementation Plan: npm Exchange Response Contract

## Overview

Use the successful package-scoped exchange and returned token as authentication proof.
Remove additional response-metadata conditions that the npm client does not require.
One work order covers the parser, all package callers, summaries, and release guide.

## Evidence and root cause

- [Release 43092525816](https://github.com/kdlbs/kandev/actions/runs/43092525816) used `fa0c48b88e774936fd28d81123e16dc96e9a5be3`, the merged creation-metadata repair.
- [Preflight job 130772994528](https://github.com/kdlbs/kandev/actions/runs/43092525816/job/130772994528) failed for `kandev` with HTTP 201 and `invalid_expires`.
- The response object, token type, and nonempty token passed their checks before expiry parsing failed.
- The earlier run failed at `invalid_created`. Removing that condition exposed the next timestamp assumption.
- [npm CLI 11.21.0](https://github.com/npm/cli/blob/v11.21.0/lib/utils/oidc.js) consumes the returned token without response-metadata validation.
- The preflight never uses the issued token after the check. Publication acquires its own credential.
- `expires_at` has no checked release consumer outside the helper's human-readable summary.
- Temporary in-memory fixtures reproduce rejection of token-only responses and responses with missing or unparseable timestamps.
- Preparation, tags, builds, and publication were skipped. This attempt needs no backfill.

The actual expiry value remains unknown. The failure does not establish that the token or publisher expired.
The defect is an extra metadata contract, not a proved npm authentication rejection.
The release system owns both the requirement clarification and the parser correction.
The existing registry trust boundary remains effective. The requirement and design preserve the rationale without a separate ADR.

## Scope

### In scope

- Accept HTTP 201 JSON objects with a nonempty token string across all required packages.
- Ignore optional `token_type`, `created`, `expires`, and unknown metadata.
- Remove exchange-response lifetime parsing and the `expires_at` success field.
- Preserve GitHub identity-token audience and expiry validation.
- Preserve HTTP rejection, token shape, registry identity, redirect, timeout, context, SDK-absence, and universal inventory checks.
- Update CLI output fixtures, redaction coverage, and the release guide.

### Out of scope

- Using or persisting the returned token, bypassing publisher rejection, and token fallbacks.
- Changing publication authentication, SDK ownership, retained artifacts, or allowed-action checks.
- New workflow modes, dependencies, schedules, and release dispatch.
- Product UI, browser tests, and new packages.

## Technical approach

In `exchangePackageToken`, retain the bounded request, canonical registry validation, HTTP 201 requirement, JSON-object check, and token-string check.
Remove metadata checks, exchange expiry parsing, and the unused exchange-specific clock option.
Keep `now` injection in the GitHub identity-token path.
Return only the package and authenticated status.

Use the existing transport and subprocess seams for tests.
Positive inventory fixtures must include every application/runtime package and the SDK when present.
Keep a mixed inventory where healthy token-only responses surround a rejected publisher.
Fail before release preparation for one real authentication rejection.

Update the CLI failure fixture to use an invalid or missing token.
Add a successful token-only CLI fixture to prove the emitted summary omits metadata and credentials.
Keep safe reason codes for actual malformed bodies and missing tokens.

Update the Early npm authentication section in `docs/public/release-process.md`.
Explain that the gate proves current exchange authentication and discards the token.
Keep final publication authentication and direct-publish/dist-tag prerequisites explicit.

## Tests

| Criteria | Evidence |
| --- | --- |
| `003.1`, `003.2`, `003.7` | Full inventory accepts token-only and arbitrary-metadata responses. A rejected publisher still blocks the gate. |
| `003.3`, `003.4` | Existing workflow-mode and SDK-absence regressions remain effective. |
| `003.5`, `003.6`, `003.7` | Safe CLI success/failure summaries, credential exclusion, and expired GitHub identity-token rejection remain effective. |

All numbers refer to `AC-RELEASE-NPM-PUBLICATION-<number>`.

## Work orders

- [x] [Task 01: Match the npm exchange acceptance contract](task-01-exchange-acceptance.md).

Sequential implementation. The primary session owns integration and delivery records.
The user authorized a subagent implementation, a new PR, and PR fixup after a 10-minute post-creation hold on 2026-11-11.

## Verification results

The temporary fixtures made no network requests and changed no production or permanent test files.
A token-only response failed `invalid_token_type`.
OIDC-typed responses with missing or unparseable timestamps failed `invalid_expires`.
The user-selected Luna subagent implemented the work order with maximum reasoning.
The permanent token-only inventory regression failed with `invalid_token_type` before the production change.
After the correction, task-defined checks passed on 2026-11-11:

- Release Node tests: 25 passed.
- Release workflow contract: 78 passed.
- Public documentation tests: 63 passed.
- Public documentation validator: 55 pages validated.
- Documentation catalog: 449 decisions and 1984 specifications validated.
- Full specification lint, script formatting, and `git diff --check`: passed.
- Documentation coverage for the actual changed paths and linked artifacts: covered with zero errors.

No live authentication or release publication was run.
The primary session owns the new PR and the requested post-creation wait and fixup.

Design validation passed on 2026-11-11:

- `python3 scripts/list-docs.py validate`: 449 decisions and 1984 specifications passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: covered with zero errors for this package and a simulated script change.
  No production file changed during planning.
- `git diff --check`: passed.
- Authoring checks found no sentence above 25 words in the new delivery documents.
- The design package was handed off unstaged and uncommitted on `fix/npm-preflight-exchange-contract`, based on the merged incident revision.

## Risks and recovery

- Authentication success does not prove token lifetime, future publisher availability, or allowed publication actions.
- Metadata must not enter summaries or exception messages.
- Expired GitHub identity tokens and HTTP publisher rejection must remain failures.
- A fresh Stable run from merged `main` uses the repair. Existing attempts retain their original helper revision.
- Other packages can still reveal genuine publisher rejection. No live passing result is claimed.

## Related records

- [Requirements](../../specs/release/requirements/npm-publication-reliability.md).
- [Design](../../specs/release/system-design/npm-publication-reliability.md#early-authentication-gate).
- [Original implementation](../npm-publication-reliability/plan.md).
- [Diagnostic repair](../npm-preflight-response-diagnostics/plan.md).
- [Creation-only repair](../npm-preflight-created-metadata/plan.md).

Earlier packages remain completed historical evidence. This package supersedes their metadata acceptance rules.
