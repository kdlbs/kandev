---
created: 2026-11-05
status: implemented
requirements:
  - REQ-RELEASE-COMPACT-RUNTIME-003
  - REQ-PLUGINS-SDK-DISTRIBUTION-001
  - REQ-PLUGINS-SDK-DISTRIBUTION-003
system_design:
  - ../../specs/release/system-design/compact-runtime-distribution.md
  - ../../specs/plugins/system-design/sdk-distribution.md
legacy_specs: []
---

# Implementation Plan: Immutable Release SDK Recovery

## Overview

Repair publication for the existing signed `v0.99.0` host release without
changing that tag or its published assets. First make host publication reuse
verified immutable assets. Then give SDK evidence its own release lifecycle.

The release system owns host artifact reuse. The plugin system owns SDK source
identity, retained evidence, bootstrap, and public consumer verification.
Each work order references only its owning system's requirements and design.

## Confirmed evidence

- [Run 42044047184](https://github.com/kdlbs/kandev/actions/runs/42044047184)
  published the GitHub Release, application npm packages, Homebrew, and Scoop.
- `prepare-plugin-sdk` failed with HTTP 422 while uploading
  `plugin-sdk-0.1.0.tgz` to the immutable host release. The SDK npm job did not run.
- The originating signed host commit is
  `f0e2e774cfe2df8a0d15e4c21dea7b48580e38fb`. The SDK version remains `0.1.0`.
- The public registry returned 404 for the whole `@kdlbs/plugin-sdk` package.
  `SDK_FIRST_PUBLISH_OIDC_READY` was unset. This does not prove npm ownership.
- The landing demo job failed because `LANDING_REPOSITORY_DISPATCH_TOKEN`
  was absent from the protected `release` environment.
- Planning started from `main` at `56af766dd9e`. Recheck live inputs before
  implementation and before another dispatch.

## Scope

### In scope

- Verify and reuse a complete, immutable host release without GitHub writes.
- Retain new SDK evidence on a draft `pluginsdk/v<S>` release, then finalize it
  after signed tag verification and before npm publication.
- Preserve the original source, receipt schema, SDK version, package bytes,
  signing authority, OIDC boundary, and explicit bootstrap state.
- Support verified legacy evidence without overwriting a host release.
- Update public recovery instructions and the landing credential procedure.

### Out of scope

- Changing the host or SDK version, package namespace, protocol, or host floor.
- Deleting or moving existing tags, replacing immutable assets, or disabling
  release immutability.
- Adding an npm token fallback, creating a placeholder SDK, or publishing a
  newly packed local archive through an ad hoc manual path.
- Changes to landing application source, UI, or third-party plugin repositories.
- Automatic contributor or Discord notices during recovery.

## Technical approach

### Host publication

Add `scripts/release/existing-release.mjs` and its focused tests. Classify the
actual selected release as absent, mutable, or published immutable. Only a
confirmed absence selects the normal create path. Verify published immutable
assets with the same inventory and validation rules as staged publication.

In `publish-release`, load the helper from the current workflow revision.
Preserve tagged application inputs. For a verified immutable release, skip
notes, updater generation/upload, and the release action. Keep the ordinary
successful job result so downstream gates remain explicit and unchanged.

### SDK evidence

Apply the [evidence ADR](../../decisions/2026-11-05-sdk-release-evidence.md).
Extend `ReleaseAssets`, `CandidateStore`, and `preparePublication` as one
verified flow. Resolve storage from the original source, bind operations to
one validated release ID, and retain all three candidate assets before signed
SDK tagging. Publish the completed SDK release with `make_latest=false`.

Keep `publish-plugin-sdk` read-only on GitHub. It accepts only finalized,
verified evidence, then publishes or verifies the exact archive. Use the
resolved evidence location in bootstrap summaries and public verification.
Do not hardcode the host tag into SDK download URLs.

### Compatibility matrix

| Remote state | Required behavior | Evidence |
| --- | --- | --- |
| Host absent or mutable | Existing publication/reconciliation behavior | Workflow and helper tests |
| Host immutable and complete | Verify existing assets. Preserve the host. | Read-only mutation recorder |
| Host immutable and incomplete | Stop before downstream publication | Missing/corrupt fixture cases |
| SDK release absent and no SDK published | Create draft, retain, sign tag, finalize | Stateful API and signed-source fixtures |
| SDK draft partly retained | Verify and resume matching evidence | Lost-response and partial-set tests |
| SDK release published | Verify only. Preserve its assets. | No-write positive and negative controls |
| Complete legacy host evidence | Verify and reuse that location | Original receipt/archive fixtures |
| Conflicting or unreadable evidence | Fail before mutation | Conflict, authentication, and pagination cases |
| npm package absent | Report exact bootstrap assets. Remain incomplete. | Bootstrap state tests |

## Tests

Task 01 owns host inventory, integrity, no-write behavior, and workflow branch
guards under `AC-RELEASE-COMPACT-RUNTIME-003.4` and `.7`.
Task 02 owns SDK retention, finalization, bootstrap, source identity, latest
selection, and public verification under `AC-PLUGINS-SDK-DISTRIBUTION-003.1`
through `.6`, plus `AC-PLUGINS-SDK-DISTRIBUTION-001.1` and `.2`.
Its signed-source fixtures must exercise the real storage adapter boundary.
No browser scenario applies to this release infrastructure change.

## Work orders

- [x] [Task 01: Reuse immutable host releases](task-01-reuse-immutable-host.md)
- [x] [Task 02: Retain and finalize SDK releases](task-02-retain-sdk-release.md)

Run Task 01 before Task 02. Both affect workflow guards and public recovery
instructions, so execution is sequential.

## Maintainer steps after implementation

1. Verify that the landing dispatch secret exists in the `release` environment.
   If the secret is absent or invalid, configure it through the procedure below.
2. Merge the repaired workflow, then dispatch `channel=stable` with
   `backfill_tag=v0.99.0` from `main`. Reconfirm that this is still the latest
   host tag. Leave notifications off.
3. If the SDK job reports `bootstrap_required`, download its exact archive,
   receipt, and signature from the reported SDK release. Verify the approved
   signing fingerprint and `archive_integrity`. Do not repack it.
4. An npm maintainer authenticates outside CI and publishes those exact bytes
   using the existing first-package bootstrap procedure. Configure the SDK's
   trusted publisher for `kdlbs/kandev`, `release.yml`, environment `release`,
   with direct publication and dist-tag permissions.
5. Retry the failed SDK job in that repaired run. Verify both public consumers,
   `stable-complete`, and the landing demo delivery. Reconfirm every sibling
   channel. An aggregate green result does not replace artifact verification.

### Landing dispatch credential

Use a fine-grained GitHub personal access token with resource owner `kdlbs`,
repository access limited to `landing`, and `Contents: Read and write`.
Set an expiration. If organization approval is required, obtain that approval.
The repository-dispatch endpoint requires that Contents permission.
Store it in the `release` environment of `kdlbs/kandev`:

```bash
gh secret set LANDING_REPOSITORY_DISPATCH_TOKEN --repo kdlbs/kandev --env release
```

Enter the value at the local prompt. Do not put it in chat, source, or logs.
This credential does not supply npm authentication or repair SDK retention.

## Verification results

Both work orders are complete. Validation passed with 24 host/helper tests,
34 SDK recovery tests, 12 packaging/consumer tests, 7 signing/registry/runtime
tests, 74 workflow contracts, 9 action-pinning tests, and 63 documentation tests.
Real packaging used the original Node 24.19.0 and npm 11.21.0 pins. The 55
published documentation pages passed validation. All 205 harness files passed
lint, and its 19 validator tests passed. The unchanged 10-finding zizmor baseline
has no new or changed findings; its nonzero scan exit remains explicit.

PR review added JSON download, safe ZIP extraction, size-report, partial-file
cleanup, and cross-phase SDK release-ID regressions. The host suite now runs in
the regular Make and CI commands. Exact byte counts remain authoritative; MiB
display checks allow the publisher's rounding at exact half steps. The linked
compact-runtime requirements and design are now active/current, consistent with
their four completed work orders from 2026-09-25 and this recovery amendment.

Design validation on 2026-11-05:

- `python3 scripts/list-docs.py validate`: passed. Validated 432 decisions and 1920 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `.github/scripts/pr-docs.cjs` coverage preflight: passed for both proposed
  production paths, both new work orders, and the linked original work order.
  All referenced requirements and designs were supplied to the evaluator.
- `git diff --check`: passed.

The first-party draft-upload documentation and the linked first-hand API
observation support retaining assets before tag publication. Implementation
must retain the explicit signed-tag and draft-finalization guards.

## Operational progress

On 2026-11-05, the landing secret became available in the `release` environment.
The scoped [landing job retry](https://github.com/kdlbs/kandev/actions/runs/42044047184/job/127366274496)
completed successfully. This proves dispatch access, not completion of a landing
deployment. No SDK retry or SDK publication occurred during this design turn.

The user then authorized implementation and PR creation. The downstream
[landing deployment](https://github.com/kdlbs/landing/actions/runs/42062120319)
failed at Wrangler authentication. Repository settings still have neither
`CLOUDFLARE_API_TOKEN` nor `CLOUDFLARE_ACCOUNT_ID`. Configure the token with
Account > Cloudflare Pages > Edit for the hosting account, and set that account's
ID as the repository variable. The successful archive download and site build
need no source repair. No release dispatch or SDK publication occurred during
implementation.

## Risks

- Draft creation must preserve the absent SDK ref until the signed-tag step.
  Verify this API behavior before any live SDK mutation. Never accept an
  automatically created unsigned tag.
- A missing or conflicting immutable host asset cannot be repaired in place.
  Stop and report it. That condition can require a new host version.
- Bootstrap requires npm maintainer access after the exact evidence is retained.
  The landing token does not satisfy this prerequisite.
- Existing SDK/host source and receipt identities must survive partial writes,
  visibility delays, legacy evidence, and concurrent or repeated attempts.
- SDK release publication must never change the application's latest release.
