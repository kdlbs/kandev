---
status: current
system: release
requirements:
  - REQ-RELEASE-NPM-PUBLICATION-001
  - REQ-RELEASE-NPM-PUBLICATION-002
  - REQ-RELEASE-NPM-PUBLICATION-003
  - REQ-RELEASE-NPM-PUBLICATION-004
---

# npm Publication Reliability System Design

## Purpose and boundaries

The release system owns registry observation, authentication checks, and delivery reports.
The [requirements](../requirements/npm-publication-reliability.md) define the maintainer outcomes.
The [SDK design](../../plugins/system-design/sdk-distribution.md) remains authoritative for candidate identity and publication state.

Apply the existing [evidence decision](../../../decisions/2026-11-05-sdk-release-evidence.md).
Signing, retained receipts, public consumers, and first-package bootstrap keep their existing authority.
[The upload attempt decision](../../../decisions/2026-11-26-sdk-npm-upload-attempts.md) defines the retry evidence boundary.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-RELEASE-NPM-PUBLICATION-001` | Registry observation; Recovery and verification |
| `REQ-RELEASE-NPM-PUBLICATION-002` | Diagnostics and summaries; Credential handling |
| `REQ-RELEASE-NPM-PUBLICATION-003` | Early authentication gate; Workflow modes; Credential handling |
| `REQ-RELEASE-NPM-PUBLICATION-004` | Recovery and verification |

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

Use `observeRegistry` in `sdk-publication-state.mjs` for version visibility and `ensureLatest`.

Start observation after the write settles or after a retry resolves the prior attempt.
Set a separate 600,000 millisecond deadline for each registry phase.
Probe immediately, then wait 10, 20, and at most 30 seconds between probes.
If the next wait reaches the deadline, perform one final probe when that wait returns.
A probe that finishes at or after the deadline serves as the final probe.
Keep the bounded request timeouts for metadata and archive reads in that probe.
Set `publish-plugin-sdk` to 30 minutes for both registry windows, setup, and consumer checks.

Use the monotonic clock and registry sleep on `ReleaseIO` so tests can advance virtual time.
The registry adapter identifies transient errors from transport timeouts, connection failures, HTTP 429, and HTTP 5xx before parsing the response.
The observer retries those reads on the same schedule and records the failed probe.
When the final probe fails, return the registry read error.
When the final probe confirms absence or a stale channel, return the visibility timeout.
Stop on malformed responses, identity conflicts, and definitive authentication errors.

An uncertain SDK upload or dist-tag update starts the same observation process as an accepted write.
A definite authentication rejection can still perform one immediate reconciliation read.

## Recovery and verification

### Choose the publication path

The protected `publish-plugin-sdk` job owns the decision, upload, and completion.
The release-wide concurrency group serializes the decision across normal Stable runs and backfills.
Resolve signed source and finalized candidate evidence before selecting a path.
If the version is public, verify its archive integrity, record digest, and provenance, then finish publication.
If first-package bootstrap applies, use the existing bootstrap result.
For a confirmed absent version, consult the attempt records.

| Trusted evidence for the signed candidate | Action |
| --- | --- |
| Accepted or uncertain upload, or prior confirmation that the version was public | Observe the registry for the same version |
| Intent with outcome and execution unresolved | Observe the registry as an uncertain upload |
| Complete history with no intent | Retain a new intent, then attempt the upload |
| All previous intents have proven rejection or lack of execution | Allow one new intent in a new backfill run for the same signed release |
| History is unknown, expired, or conflicting | Report the evidence problem and leave upload authorization unresolved |

The current registry read must succeed before the decision can grant an upload.
An absent version on a later retry carries the same unresolved outcome until a registry read verifies the archive or trusted evidence proves the earlier command did not write.

### Retain attempt evidence

Use two immutable GitHub Actions artifacts per SDK version and workflow run.
Use the names `sdk-npm-upload-intent-v<version>` and `sdk-npm-upload-result-v<version>`.
The intent contains a small JSON record with the schema version, package, SDK version and tag, signed source commit, archive SHA-512 integrity, workflow SHA, run ID, and run attempt.
The result repeats this identity, identifies its intent artifact ID, and records a fixed outcome.
Use accepted, uncertain, rejected, never started, or already public.
Only a proven refusal of the publish request qualifies as a rejection.
Use uncertain for ambiguous npm responses, including an error that says the version already exists.
Treat npm `E403` as ambiguous even when its diagnostic category is authentication; it does not prove that the publish request was refused.
Set `--fetch-retries=0` on npm publication and dist-tag mutations so the CLI cannot repeat a write after a transient response; the release state machine owns observation and registry-read retries.
For a definitive authentication error, end the job promptly and record uncertainty if the command might have submitted the archive.

1. Decide whether a fresh upload is eligible and write the intent file.
2. Retain it using the pinned upload action in the `retain_sdk_upload_intent` step. Require the artifact ID and minimum retention before running npm.
   Check that the repository artifact lookup returns that ID.
3. Permit an upload only in the run and attempt that retained the intent. Recheck the exact public version before the command. Record a matching existing version as already public.
4. Save the sanitized result promptly in its own artifact before observation can time out. If cancellation prevents that write, use the retained intent as uncertain. A skipped publish step in the authenticated producer attempt can prove that the command never started.
5. Call the same completion logic for the new write and for recovery. That logic observes when needed, verifies the archive, promotes or preserves a verified newer `latest`, and runs the public Go and npm consumers.

Keep the upload and observation at distinct workflow steps so GitHub can retain evidence after a stopped job.
The completion phase receives the identity and outcome through validated records.
An authorization or integrity failure stops the job while successful Stable completion requires both consumer checks.

### Trust and retention

Use the GitHub Actions repository artifact API with an exact name filter to find records across runs.
Use the authenticated workflow-run and job-attempt APIs as an independent index of SDK publisher attempts.
The publisher job name includes the exact host tag so a recovery run can distinguish attempts for this SDK from other Stable releases.
Require the canonical API workflow path `.github/workflows/release.yml`, the expected repository, `workflow_dispatch` event, `main` branch, and matching workflow SHA.
Use a small read-only helper within `scripts/release` to resolve records.
Validate the artifact ID and digest, bounded JSON payload, and its producer metadata.
Only accept records that bind to the expected repository and protected `main` release workflow.
Resolve the producer run and exact attempt and reconcile the artifact identity against the finalized SDK receipt.
Ignore artifacts whose metadata proves that they belong to other workflows or branches.
Treat malformed or contradictory evidence from the trusted workflow as unresolved history.
If authenticated job metadata shows that intent retention succeeded but the intent artifact is missing, leave upload authorization unresolved even when the registry reports absence.
A completed publisher job skipped before any steps proves that no upload attempt started.
Do not infer that history is empty from an artifact lookup alone.

Request 90-day artifact retention and confirm at least 30 days of retention on the intent before starting npm.
Keep the attempt artifacts and producer runs throughout the evidence window.
An absence check can grant authorization only when the signed receipt's producer run falls within the 30-day evidence window and its trusted workflow revision contains the intent gate.
Use the receipt's existing run ID and workflow SHA to establish that boundary.
If the origin or any prior attempt retained an intent, require its record.
Missing or expired known evidence, incomplete API responses, and legacy producer workflows require reconciliation before any fresh upload.
A matching public archive can complete an older release directly when authenticated attempt history is complete; retain discovered accepted or uncertain attempts for the provenance check.
An unreadable history response must not turn a visible first package into a manual bootstrap.
The operating guide directs maintainers to retain unresolved evidence and use the same release tag for recovery.

Use `actions: read` for artifact reads and the standard Actions upload step for writes.
The publisher also needs its existing `contents: read` and `id-token: write`.
Retain only fixed state and public identifiers in artifacts.
Keep credentials and signed download URLs in memory.

## Diagnostics and summaries

Use `scripts/release/sdk-diagnostics.mjs` for structured phase events and sanitized command results.
Report the prior upload result, its Actions run and attempt, and whether the job is observing an earlier write.
When the history is incomplete, identify the signed version and missing evidence.

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
| Accepted or uncertain upload awaiting visibility | Rerun the failed job to observe the same version |
| Upload rejected | Repair the SDK publisher and backfill the same signed release |
| Upload history unresolved | Verify the exact version and examine the recorded attempt before authorizing an upload |
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
3. Require HTTP 201 and a JSON object with a nonempty token string.
4. Discard both tokens after the check.

The public package exchange is the authority for current package authentication.
The helper ignores `token_type`, `created`, `expires`, and other descriptive response metadata.
It neither verifies nor reports the issued token's lifetime.
The success entry contains only the package and authentication status. It omits `expires_at`.
The helper discards the returned token. Publication obtains a fresh credential through its existing path.
GitHub identity-token audience and expiry validation remain mandatory before each exchange.

This contract follows the [pinned npm client](https://github.com/npm/cli/blob/v11.21.0/lib/utils/oidc.js), which consumes the returned token without metadata checks.
The registry documentation's timestamp example does not define an additional authentication condition.

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

Report one fixed reason for each failed condition.
These conditions cover unreadable JSON, invalid body shape, and a missing, empty, or non-string token.
Use `npm_exchange_response` for malformed successful responses.
Keep `npm_authentication` for HTTP authentication rejection.
Preserve the package and HTTP status in the error.

The failure summary includes the reason, package, HTTP status, workflow SHA, and checkout SHA.
`sdk-runtime.mjs` owns the protected-context checks and throws `ReleaseContextError` with the failed setting and its required value.
It never stores the supplied value in that error.
The preflight derives a fixed reason from the known setting and reports its required value.
An unexpected context exception retains a fixed fallback message.
Only fixed diagnostic labels can describe response fields.
Never interpolate a response value, token fragment, unknown property name, body, or underlying exception message.
The error message identifies the failed validation condition.
Publisher guidance applies to authentication rejection.
Response-validation guidance directs maintainers to compare the named condition with the npm API and client contract.

Tests cover each validation condition separately and use a fixed clock.
The documented success fixture remains accepted.
Invalid authentication, token, transport, and context fixtures remain rejected before release preparation.
Token-only responses and responses with missing or unexpected metadata remain accepted.
GitHub identity-token expiry tests remain separate from ignored exchange-response metadata.
The observed `invalid_created` and `invalid_expires` failures exposed the extra metadata assumptions.

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

Stable backfill enters the protected SDK recovery path after host publication.
The early gate covers normal Stable releases, where application and runtime versions need publication.

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
Use a fake clock to reproduce the 309-second visibility delay, a version visible only at the deadline, and a terminal transient read error.
Exercise the protected publication entry point through a first attempt and recovery with retained artifacts and a still-absent version.
Prove the upload count stays at one after an accepted result, an uncertain result, and an interruption after retaining the intent.

## Related delivery records

- [Original SDK publication](../../../plans/plugin-sdk-publishing/task-04-stable-publication.md).
- [Immutable SDK recovery](../../../plans/immutable-release-sdk-recovery/plan.md).
- [Implementation plan](../../../plans/npm-publication-reliability/plan.md).
- [SDK registry resilience](../../../plans/sdk-registry-resilience/plan.md).
- [Preflight response diagnostics repair](../../../plans/npm-preflight-response-diagnostics/plan.md).
- [Creation metadata compatibility repair](../../../plans/npm-preflight-created-metadata/plan.md).
- [Exchange response contract repair](../../../plans/npm-preflight-exchange-contract/plan.md).
