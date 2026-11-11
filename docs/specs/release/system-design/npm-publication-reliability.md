---
status: current
system: release
requirements:
  - REQ-RELEASE-NPM-PUBLICATION-001
  - REQ-RELEASE-NPM-PUBLICATION-002
  - REQ-RELEASE-NPM-PUBLICATION-003
---

# npm Publication Reliability System Design

## Purpose and boundaries

The release system owns registry observation, authentication checks, and delivery reports.
The [requirements](../requirements/npm-publication-reliability.md) define the maintainer outcomes.
The [SDK design](../../plugins/system-design/sdk-distribution.md) remains authoritative for candidate identity and publication state.

Apply the existing [evidence decision](../../../decisions/2026-11-05-sdk-release-evidence.md).
Signing, retained receipts, public consumers, and first-package bootstrap keep their existing authority.
No new ADR is required for these local delivery changes.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-RELEASE-NPM-PUBLICATION-001` | Registry observation; Recovery and verification |
| `REQ-RELEASE-NPM-PUBLICATION-002` | Diagnostics and summaries; Credential handling |
| `REQ-RELEASE-NPM-PUBLICATION-003` | Early authentication gate; Workflow modes; Credential handling |

## Existing components

- `scripts/release/sdk-publication-state.mjs` owns `preparePublication`, `publishPublication`, and `ensureLatest`.
  Its `observe` helper currently makes three reads for GitHub tags and npm state.
- `scripts/release/sdk-release-io.mjs` owns `ReleaseIO`, command execution, registry inspection, and consumer verification.
  Its `wait` method sleeps for 10 seconds, then 20 seconds.
- `scripts/release/sdk-registry.mjs` owns public registry reads and archive integrity checks.
- `scripts/release/publish-plugin-sdk.mjs` owns `runRelease`, final summaries, and temporary-directory cleanup.
- `scripts/release/npm-packages.sh` supplies the application and runtime package inventory.
- `.github/workflows/release.yml` owns release modes, job permissions, and publication dependencies.

## Registry observation

Add a registry-specific observer in `sdk-publication-state.mjs`.
Keep the existing GitHub tag observer and consumer retry policy separate.
Use the new observer for version visibility and `ensureLatest`.

Each registry observation starts after its one write settles.
Its deadline is 300,000 milliseconds after that point.
Probe immediately, then wait 10, 20, and at most 30 seconds between probes.
Cap each wait at the remaining time.
Start no new probe after the deadline.
An active probe retains the registry adapter's bounded request timeouts.
This deadline governs observation admission, rather than cancelling artifact validation already in progress.

Inject a monotonic clock and registry sleep through `ReleaseIO`.
Tests supply virtual time.
Keep production defaults local to the SDK adapter.
Do not add an operator setting or a general retry framework.

The observer accepts only confirmed absence or a stale channel as pending state.
Registry transport errors, malformed responses, and identity conflicts keep their existing error behavior.
Never convert these errors into a missing version.

An accepted write and an uncertain write both lead to observation.
Capture that distinction before polling.
A definite npm authentication rejection can fail after one reconciliation read.
Do not spend the visibility window retrying a known credential error.

## Recovery and verification

Retain the ordering in `publishPublication`:

1. Resolve signed source and finalized candidate evidence.
2. Read and verify an existing version.
3. Attempt one upload only when that version is absent.
4. Observe and verify its immutable identity.
5. Promote or preserve a verified newer `latest`.
6. Run both public consumer checks.

The observer never owns a write callback.
This prevents a retry from issuing a second upload or channel mutation.
An uncertain write that later appears completes the same flow.
A timeout preserves the retained candidate and leaves completion unsuccessful.

Five-minute windows apply separately to npm version visibility and dist-tag visibility.
The existing 20-minute SDK job timeout remains sufficient for both windows and consumer checks.
Use source checks and simulated-time tests to preserve that budget.

## Diagnostics and summaries

Add `scripts/release/sdk-diagnostics.mjs` as a small SDK reporter.
Its inputs are structured phase events and sanitized command results.
Do not create a persistent journal or a new metrics service.

Record these phases: source verification, candidate verification, npm upload, version visibility, channel promotion, and public consumers.
Track each phase as pending, passed, failed, or skipped.
Track write outcomes separately as accepted, uncertain, or rejected.
Track the dist-tag write outcome separately from the immutable upload.
A channel-only failure preserves a confirmed publication state and reports channel-specific recovery guidance.

For observation events, include package, SDK version, phase, attempt, elapsed milliseconds, and next wait milliseconds.
Expose the observed public version and dist-tags as bounded values.
Never report an accepted upload as confirmed public availability.

`ReleaseIO.npm` captures both successful and unsuccessful command output.
Sanitize that output before it reaches generic logs, error messages, or summaries.
Keep at most 64 KiB of sanitized output per command and mark truncation.
Preserve useful npm status lines, the npm error code, and safe registry status information.
Exclude raw debug-log files and authentication payloads.

`runRelease` writes the phase summary on success and on failure.
Write it before temporary-directory cleanup.
Preserve the original nonzero result if summary writing also fails.
Both errors must remain identifiable through safe fixed messages.

Use these failure categories:

| Category | Maintainer action |
| --- | --- |
| Authentication or authorization | Inspect this package's publisher, workflow, environment, expiration, and allowed actions |
| Accepted upload awaiting visibility | Check the public version, then rerun failed jobs in the same release |
| Uncertain upload | Check registry identity before any recovery write |
| Registry read error | Restore readable public registry access, then retry the same release |
| Identity or provenance conflict | Stop recovery and investigate the existing immutable evidence |
| Consumer verification | Inspect the named consumer check and rerun verification after correction |

Summaries include the host tag, SDK version, last confirmed phase, and recovery action.
A consumer failure can therefore show that publication and channel promotion already succeeded.
Keep Discord's existing manual recovery notice.

## Early authentication gate

Add `npm-auth-preflight` before `prepare` for normal Stable bumps.
Use a separate GitHub-hosted job with `environment: release`.
Grant only `contents: read` and `id-token: write`.
Use the existing pinned checkout and Node setup actions.
Keep `persist-credentials: false`.
Run helper code from the trusted workflow revision.
Record workflow and checkout revisions separately.

Add `scripts/release/npm-auth-preflight.mjs`.
Reuse the six application/runtime names from `npm-packages.sh` through the shell caller.
Add the independent SDK name explicitly.
Do not create another runtime package map.

Require the same repository, event, ref, workflow, hosted-runner, and scope confirmation as the current SDK release context.
Reject an unsupported context before a request.
Read SDK package existence through `sdk-registry.mjs`.
A confirmed absent SDK is reported as deferred to the current bootstrap flow.
The helper never emits `bootstrap_required` or approves the later candidate.
Runtime package exchange failures remain setup errors.

For each existing required package:

1. Request a fresh GitHub OIDC ID token with audience `npm:registry.npmjs.org`.
2. Exchange it through `POST /-/npm/v1/oidc/token/exchange/package/{package_name}` on the public npm registry.
3. Require HTTP 201 and a valid, unexpired OIDC exchange-token response.
4. Discard both tokens after the check.

Use bounded requests and reject redirects during authentication.
Fail with a package-specific category for a rejected or unreadable response.
Do not log the response body.
Do not use `npm whoami` as an OIDC check.

This is an authentication check.
The public exchange response does not prove direct-publish or dist-tag permissions.
Publication remains responsible for those checks.
The operating guide must retain both permission prerequisites.

The npm [OIDC API](https://api-docs.npmjs.com/#tag/oidc) defines the audience, package exchange route, and response fields.
The npm [publisher guide](https://docs.npmjs.com/trusted-publishers/) defines package-specific trust and allowed actions.

### Exchange response diagnostics

The helper distinguishes an HTTP rejection from a response that fails local validation.
An HTTP 201 response does not establish that every returned field is valid.
A local validation error does not establish that the trusted publisher rejected authentication.

Keep the existing token acceptance rules during the diagnostic repair.
Report one fixed reason for each failed condition.
These conditions cover unreadable JSON, invalid body shape, invalid token type, missing token, invalid creation date, invalid expiry, and expired tokens.
Use `npm_exchange_response` for malformed successful responses.
Keep `npm_authentication` for HTTP authentication rejection and an expired exchange token.
Preserve the package and HTTP status in the error.

The failure summary includes the reason, package, HTTP status, workflow SHA, and checkout SHA.
Only fixed diagnostic labels can describe response fields.
Never interpolate a response value, token fragment, unknown property name, body, or underlying exception message.
The error message identifies the failed validation condition.
Publisher guidance applies to authentication rejection.
Response-validation guidance directs maintainers to compare the named condition with the npm API and client contract.

Tests cover each validation condition separately and use a fixed clock.
The documented success fixture remains accepted.
Every invalid fixture remains rejected before release preparation.
An acceptance-rule correction requires evidence of the actual incompatible response shape.
The diagnostic repair does not infer that shape from a generic error message.

## Workflow modes

`prepare` gains the new dependency and an explicit status-function condition.
Require preflight success for a normal Stable bump.
Permit its intentional skip only for the existing validation and backfill modes.
Trace this dependency through every build and publication job.

| Mode | New authentication job | Required result |
| --- | --- | --- |
| Normal Stable | Runs | All required exchanges pass before `prepare` |
| Normal Stable, one rejected package | Fails | No release PR, tag, build, or publication |
| Normal Stable, SDK absent | Checks application/runtime packages and reports SDK deferral | Existing bootstrap flow later decides completion |
| Stable dry run | Skips | Existing read-only metadata validation |
| Desktop validation | Skips | Existing desktop validation path |
| Existing-tag backfill | Skips | Existing protected publication/recovery jobs perform authentication |
| Manual or scheduled Nightly | Skips | Existing Nightly path |
| Cancellation | Stops | No bypass through a permissive dependency condition |

Backfill deliberately retains its current repair path.
Requiring healthy credentials for channels that already need no writes can obstruct recovery.
The new early gate covers normal releases, where application/runtime versions need publication.

## Credential handling

Keep release signing secrets in their existing jobs.
The new job references no signing secret and performs no GitHub write.
It does not persist an npm credential file or an exchange token.
Do not add an npm-token fallback.

The SDK reporter excludes known credential values, npm tokens, JWTs, authorization headers, and authentication query parameters.
Filter output before retaining a bounded tail.
Negative fixtures must include bearer and Basic authorization headers, including Proxy-Authorization and case variants, npmrc authentication lines, JWTs, and URLs containing credentials.
No raw authentication error body enters the reporter.

## Verification

Use existing Node state-machine and adapter tests with virtual time.
Add focused reporter and authentication-helper suites.
Test the real orchestration boundary with fixture commands and mocked HTTP transport.
Record every attempted write in the fixtures.

Extend the raw workflow contracts for every mode in the table.
Exercise a healthy package alongside a rejected package to prove the universal gate.
Keep the SDK bootstrap, immutable evidence, no-downgrade, and public-consumer regressions.
No browser scenario applies to this infrastructure change.

## Related delivery records

- [Original SDK publication](../../../plans/plugin-sdk-publishing/task-04-stable-publication.md).
- [Immutable SDK recovery](../../../plans/immutable-release-sdk-recovery/plan.md).
- [Implementation plan](../../../plans/npm-publication-reliability/plan.md).
- [Preflight response diagnostics repair](../../../plans/npm-preflight-response-diagnostics/plan.md).
