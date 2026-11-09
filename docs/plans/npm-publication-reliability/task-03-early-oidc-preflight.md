---
id: "03-early-oidc-preflight"
title: "Gate normal Stable releases on npm authentication"
status: completed
wave: 3
depends_on:
  - "02-publication-diagnostics"
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

# Task 03: Gate Normal Stable Releases on npm Authentication

## Summary

Authenticate existing npm publishers before normal Stable preparation.
Use a separate protected job with read-only GitHub access and no signing operation.

## In scope

- Add `npm-auth-preflight.mjs` and a transport-injected fixture suite.
- Reuse the runtime package inventory from `npm-packages.sh`.
- Check GitHub OIDC context, audience, and each package-scoped npm exchange.
- Defer only confirmed SDK absence to its existing first-publication procedure.
- Add the protected preflight job and explicit `prepare` dependency handling.
- Cover every workflow mode and update the release operating guide.

## Out of scope

- Publisher renewal, permanent npm credentials, token fallback, and live API mutations.
- Early authentication for Nightly, dry runs, desktop validation, or backfill.
- Proving direct-publish/dist-tag permissions from a successful exchange.
- Changes to SDK evidence, release signing, or notification policy.

## Acceptance

1. A rejected package among healthy publishers blocks normal release mutations and builds.
2. Mode and SDK-absence fixtures preserve the design's complete workflow matrix.
3. The helper performs only registry reads and token exchanges, keeps tokens in memory, and reports safe package-specific results.

## TDD cases

Start with `one expired publisher blocks preparation before the release branch or tag`.
Use mixed healthy and rejected packages to prove that every required check must pass.
Verify the audience and encoded package route for each exchange.
Cover malformed, expired, missing-token, rejected, redirected, and timed-out responses.
Verify that a runtime exchange 404 fails, while a confirmed whole-package SDK 404 defers only the SDK check.
A failed SDK registry read must not become bootstrap deferral.

The raw workflow suite must cover normal success/failure, SDK absence, dry run, desktop validation, backfill, both Nightly triggers, and cancellation.
Assert permissions, trusted revision, pinned actions, and `persist-credentials: false`.
Trace every build/publication condition reachable through `prepare`.
Fixtures must record zero package, channel, publisher-configuration, and signing writes.

## Verification

Run from the repository root.
Use the task-defined scanner baseline and report existing findings separately.

```bash
node --test scripts/release/npm-auth-preflight.test.mjs scripts/release/sdk-runtime.test.mjs scripts/release/sdk-registry.test.mjs
python3 .github/scripts/release-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
zizmor .github/workflows
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `scripts/release/npm-auth-preflight.mjs` (new)
- `scripts/release/npm-auth-preflight.test.mjs` (new)
- `scripts/release/sdk-runtime.mjs`
- `scripts/release/sdk-runtime.test.mjs`
- `.github/workflows/release.yml`
- `.github/scripts/release-workflow-contract_test.py`
- `docs/public/release-process.md`

## Dependencies

Task 02 establishes the diagnostics and operating-guide vocabulary.
Read `.github/AGENTS.md` before workflow changes.

## Risks

- An upstream skipped job can silently suppress downstream work without explicit status functions.
- A token exchange proves authentication but cannot establish every publisher action permission.
- Early authentication is a snapshot.
  The final publisher still needs valid trust when it runs.
- Existing runs retain their original workflow definition.
  Test with fixtures rather than dispatching a release.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/release/requirements/npm-publication-reliability.md), `REQ-RELEASE-NPM-PUBLICATION-003`.
- [Design](../../specs/release/system-design/npm-publication-reliability.md), Early authentication gate and Workflow modes.
- Existing `assertReleaseContext` and `npm-packages.sh`.
- [npm OIDC API](https://api-docs.npmjs.com/#tag/oidc).
- [npm trusted publishers](https://docs.npmjs.com/trusted-publishers/).

## Results

Completed on 2026-11-09.

- Added `scripts/release/npm-auth-preflight.mjs`. It validates the protected release context, requests a fresh GitHub OIDC token with audience `npm:registry.npmjs.org` for each package, exchanges it at the public package route, requires HTTP 201 with a valid unexpired response, and discards both tokens.
- Reused `npm-packages.sh` for the application and five runtime packages and added the SDK name explicitly. Confirmed whole-package SDK absence defers only the SDK check; an unreadable SDK registry or missing runtime package fails.
- Added the protected read-only `npm-auth-preflight` job with `contents: read` and `id-token: write`, a trusted workflow-revision checkout, pinned setup actions, and no persisted credentials.
- Added an explicit `prepare` dependency. Normal Stable requires preflight success. Dry run, desktop validation, and backfill may proceed only through the intentional skipped branch. Nightly and scheduled runs keep their existing paths.
- Recorded workflow and checkout revisions separately in the result. The result names each authenticated or deferred package and provides publisher-configuration guidance without response bodies or credentials.
- Documented that a successful exchange proves authentication only, not direct-publish or dist-tag permission, and that the publishing jobs still authenticate when they run.
- Verification:
  - `node --test scripts/release/npm-auth-preflight.test.mjs scripts/release/sdk-runtime.test.mjs scripts/release/sdk-registry.test.mjs`: 15 tests passed.
  - `python3 .github/scripts/release-workflow-contract_test.py`: 78 tests passed.
  - `python3 .github/scripts/lint-action-pinning_test.py`: 9 tests passed.
  - `python3 .github/scripts/lint-action-pinning.py`: all 30 workflow files use SHA-pinned action refs.
  - `zizmor .github/workflows`: existing baseline only; no finding in the added job.
  - `python3 scripts/list-docs.py validate`: 439 decisions and 1948 specifications passed.
  - `python3 scripts/lint-spec-files.py --all`: all specification files passed.
  - `git diff --check`: passed.
