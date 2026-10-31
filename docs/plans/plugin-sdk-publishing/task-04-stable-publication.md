---
id: "04-stable-publication"
title: "Publish SDKs through Stable releases"
status: done
wave: 4
depends_on:
  - 03-compatibility-check
plan: "plan.md"
requirements:
  - REQ-PLUGINS-SDK-DISTRIBUTION-001
  - REQ-PLUGINS-SDK-DISTRIBUTION-002
  - REQ-PLUGINS-SDK-DISTRIBUTION-003
acceptance_criteria:
  - AC-PLUGINS-SDK-DISTRIBUTION-001.1
  - AC-PLUGINS-SDK-DISTRIBUTION-001.2
  - AC-PLUGINS-SDK-DISTRIBUTION-002.1
  - AC-PLUGINS-SDK-DISTRIBUTION-002.3
  - AC-PLUGINS-SDK-DISTRIBUTION-003.1
  - AC-PLUGINS-SDK-DISTRIBUTION-003.2
  - AC-PLUGINS-SDK-DISTRIBUTION-003.3
  - AC-PLUGINS-SDK-DISTRIBUTION-003.4
system_design:
  - ../../specs/plugins/system-design/sdk-distribution.md
---

# Task 04: Publish SDKs through Stable releases

## Summary

Integrate the shared SDK version into Stable preparation, signed tagging, npm
trusted publishing, and completion gates. Local tests prove mutation/recovery
rules; an authorized later release supplies public installation evidence.

## In scope

- Add `scripts/release/plugin-sdk.mjs` for metadata/version/contract preflight,
  the contract-input inventory, and release-PR stamping. Add tests first.
- Add `scripts/release/publish-plugin-sdk.sh` and mocked publication tests.
  Reuse registry helpers without expanding the Nightly/runtime package inventory.
- Extend `release.yml` outputs/staging, backfill validation, protected
  `prepare-plugin-sdk`, `publish-plugin-sdk`, completion, and notice dependencies.
  Root preparation signs only the host tag. SDK preparation creates or verifies
  the SDK tag after `publish-release`, including on backfill.
- Verify immutable tag commit, npm integrity/record, uncertain response recovery,
  retry bounds, registry visibility, OIDC/provenance, and no `latest` downgrade.
- Retain the original archive and signed identity receipt. Implement the explicit
  `bootstrap_required` state and recovery rules in the contract below.
- Implement public mode of `consumer-smoke.mjs` and include it in the SDK job.
  Reject null metadata at publication; verify reused SDK versions too.

## Candidate, bootstrap, and recovery contract

Implement the design's [artifact identity](../../specs/plugins/system-design/sdk-distribution.md#sdk-artifact-identity)
and [recovery table](../../specs/plugins/system-design/sdk-distribution.md#failure-and-recovery).
`prepare-plugin-sdk` uses the protected signing identity and Git write access.
`publish-plugin-sdk` uses the configured OIDC identity without signing secrets.
Both normal Stable and backfill traverse these jobs after host publication.

Retain the exact tested archive, receipt, and detached receipt signature on the
originating host release before SDK tag/npm publication. The receipt records
original source/tag, metadata digest, integrity, file inventory, exact packaging
tools, and workflow identity. Verify retained assets before reuse. Rebuild a
lost archive only from the original source and tools, with an exact hash match.
Missing/conflicting receipt evidence after publication stops recovery.

A confirmed absent package with unavailable first-publication OIDC yields
`bootstrap_required` after retaining the candidate. Report its links, version,
integrity, and maintainer instructions. Keep SDK completion unsuccessful.
The maintainer publishes those exact bytes and configures OIDC. A retry verifies
the upload and records its bootstrap provenance separately. Existing-package
setup errors and registry read/authentication failures are not bootstrap states.

Pin packaging tools through WO02's config and verify current OIDC publish and
dist-tag support. Validate the actual workflow/environment permissions. Never
inject a token fallback. For unchanged SDKs, verify the original artifact and
receipt instead of packing the new host tree. Publishing any changed package
bytes requires a new version.

## Workflow-mode matrix

| Mode | SDK behavior | Required result |
| --- | --- | --- |
| Stable, changed contract | Stamp release PR, retain candidate after host publication, sign SDK tag, publish/verify exact archive | Both public consumers pass before completion/notices |
| Stable, unchanged contract | Verify previous SDK identity/mapping; no new upload/tag | Successful verification result for completion dependencies |
| Dry-run | Read/validate metadata and registry preflight only | Zero tag/npm mutations |
| Desktop validation or Nightly | Skip Stable SDK mutations | Existing channel behavior remains valid |
| Latest Stable backfill | Read signed host record, create or verify SDK tag, resume from retained candidate | Identity conflict fails; no mutable-channel downgrade |

## Required failure cases

- Existing signed host plus absent SDK tag: create one correctly signed tag at
  the host commit. Reuse a matching tag. Reject wrong targets or signatures
  before mutation. Exercise normal and backfill paths.
- Confirmed absent npm package: retain candidate and report `bootstrap_required`.
  Retry accepts matching bootstrap bytes. Authentication, transport, malformed
  responses, and an existing package with a missing version cannot masquerade
  as package absence.
- Exercise every tag/npm combination in the design's recovery table. Re-query
  uncertain writes. Reject mismatched archives, records, receipts, or signatures.
  A different npm version cannot silently generate replacement bytes.
- Later host with SDK README-only changes: verify the original artifact.
  Changed runtime/type contracts cannot reuse its version. Test missing retained
  archives, receipt-preserving rebuilds, and immutable asset conflicts.
- Public mode rejects local proxy/registry and checksum-bypass settings, including
  inherited configuration. Both modes run identical compatibility assertions.
  Completion/notices must fail on bootstrap or SDK failure. Test OIDC dist-tag
  permission failures and prevention of `latest` downgrades.

Place version/reuse tests in `plugin-sdk.test.mjs`, state/receipt/bootstrap tests
in `publish-plugin-sdk.test.mjs`, environment tests in `consumer-smoke.test.mjs`,
and job/permission/mode guards in `release-workflow-contract_test.py`.

## Out of scope

Actual registry account configuration, first publish, release dispatch, or
external repository changes during implementation. Do not modify the unrelated
Stable runtime package version policy.

## Acceptance

1. Changed/reused SDK paths and every workflow mode meet the matrix; SDK failure
   prevents Stable completion and contributor notices, without blocking sibling
   channel publication unnecessarily.
2. Existing-version retry verifies immutable identities and resumes missing work;
   tag/integrity conflicts, auth/read errors, and unresolved records fail closed.
3. A release job installs both public packages, runs the compatibility check,
   and reports source/mapping evidence; npm bootstrap and OIDC setup have explicit
   maintainer prerequisites and no silent token fallback.

## Verification

```bash
node --test scripts/release/plugin-sdk.test.mjs scripts/release/publish-plugin-sdk.test.mjs
node --test scripts/release/nightly-version.test.mjs scripts/release/nightly-release.test.mjs scripts/release/publish-npm.test.mjs
python3 .github/scripts/release-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
bash -n scripts/release/publish-plugin-sdk.sh scripts/release/publish-npm.sh
zizmor .github/workflows
node --test scripts/pluginsdk/consumer-smoke.test.mjs scripts/pluginsdk/pack-candidate.test.mjs
node scripts/pluginsdk/consumer-smoke.mjs --mode local
git diff --check
```

Future public release verification, run by the authorized Stable SDK job after
tagging/publication (not by this work order's local test block):

```bash
RELEASE_GPG_FINGERPRINT="$APPROVED_FINGERPRINT" node scripts/pluginsdk/consumer-smoke.mjs --mode public --host-tag "$HOST_TAG"
```

The job resolves versions and the original receipt from the signed host/SDK
tags and retained host-release assets. It supplies the design's public trust environment,
fresh caches, and public npm registry. Unverified caller claims are not evidence.
Record public success separately from mocked/local
tests. An unavailable workflow tool is a reported verification gap, not a pass.

## Files likely touched

- `.github/workflows/release.yml`, `.github/scripts/release-workflow-contract_test.py`.
- `scripts/release/{plugin-sdk.mjs,plugin-sdk.test.mjs,publish-plugin-sdk.sh,publish-plugin-sdk.test.mjs}` (new).
- Applicable existing registry lookup/release helper tests.
- `scripts/pluginsdk/consumer-smoke{,.test}.mjs`.
- SDK record/package/version/lock files as release-prepare staging inputs.

## Dependencies

[Task 03](task-03-compatibility-check.md): consumer validation and metadata rules.

## Risks

Signed tags become publicly visible before the npm artifact; the release is
partial until verified. npm package ownership, bootstrap, trusted-publisher
identity, direct-publish permission, and dist-tag permission require maintainer
access. Scope ownership precedes release. First-package publisher setup may
follow the retained candidate through the explicit bootstrap state.
Backfill uses the existing
latest-tag restriction and must not restamp metadata.

## Parallelism

`sequential`

## Inputs

- [Design: Stable integration and recovery](../../specs/plugins/system-design/sdk-distribution.md#stable-release-integration).
- `.github/AGENTS.md`, release skill, current prepare/publish/backfill/notices
  jobs, registry helper semantics, and official npm/Go documentation.

## Results

Implemented on 2026-10-19. The external adapter verifies real signed tags and
receipt signatures, retains immutable GitHub assets before SDK tagging, packs
from the original signed source with original tools, publishes exact bytes via
OIDC, checks all four tag/npm states, and verifies public consumers. Stable and
backfill use separate signing and OIDC jobs. Current workflow helpers control
recovery; archived SDK source and packaging pins remain tied to the original
release. Dry-run is read-only; Nightly and desktop validation skip SDK mutation.

Local evidence:

- 49 SDK release/consumer tests passed, including real disposable GPG keys,
  lost upload responses, partial retention, exact rebuild hashes, inherited
  config rejection, bounded visibility retries, and no-downgrade checks.
- 68 release workflow contracts passed. SDK CI selection: nine tests passed.
  Existing runtime/Nightly publication tests: 30 passed. Action pinning: nine
  tests passed. Notification workflow contracts: four passed.
- Shell syntax, Node syntax, documentation/specification/harness validation,
  and whitespace checks passed.
- `zizmor .github/workflows` remains nonzero on existing repository findings
  (18 informational, 3 low, 8 medium, 11 high; 228 suppressed and 2 ignored).
  No finding points to the new SDK release jobs or changed SDK consumer steps.

Implementation CI passes. No release was dispatched, no SDK tag was pushed,
and no registry package was published. Public availability and protected npm
ownership/publisher setup remain release-time prerequisites.

## Completion

Completed on 2026-10-23 after local acceptance checks and implementation CI
passed. The [plan's cleanup record](plan.md#validation-and-review-cleanup)
contains the validation evidence and review dispositions. Public publication,
registry configuration, external repository migration, and author notices
remain separate follow-up work; completion here does not certify those actions.

## Adversarial review correction

On 2026-10-24, the [implementation review](adversarial-review.md) found that the
packer dropped the record digest required by `ReleaseIO.build`. The digest now
crosses that boundary. The new `sdk-release-io.test.mjs` regression failed before
the fix and passed after it, using real packing and isolated Go/npm consumers.
Conflicting source digests remain rejected. This closes a gap that the earlier
mocked release-state and separate packer tests did not detect.


### Main integration: command-scoped SDK tag authentication

The security workflow update removes persisted GitHub credential helpers.
SDK tag pushes now clear inherited helpers and select `gh auth git-credential`
for that command only. Preparation configures bot identity without running
`gh auth setup-git`; signing and OIDC publication remain separate.
The focused push-argument regression failed before the correction. Verification
also covers the real signed-tag repository fixture and release workflow gates.
No package publication or protected-setting change occurs during this check.
