---
status: current
system: release
requirements:
  - REQ-RELEASE-WINDOWS-SIGNING-001
---

# Windows Runtime Signing System Design

## Purpose and boundaries

The release bundle producer owns optional signing of its two Windows host
executables before existing packaging and publication. Desktop installers,
release-tag signing, and package-manager publication keep their own contracts.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-RELEASE-WINDOWS-SIGNING-001` | [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery), [Security](#security) |

## Components and responsibilities

- `signpath-signing-ready.sh` classifies completeness and run purpose.
- `build-bundles` uploads the two Windows host binaries to a run-scoped artifact,
  invokes the pinned SignPath action, and adopts complete signed output.
- Existing package steps consume the selected host binaries and canonical
  helpers, then validate and checksum the current Stable or Nightly layout.
- Guard tests and release-workflow contracts protect eligibility, ordering,
  fallback integrity, modes, and downstream archive contracts.

## Data and contracts

Repository secret `SIGNPATH_API_TOKEN` and repository variables
`SIGNPATH_ORGANIZATION_ID`, `SIGNPATH_PROJECT_SLUG`, and
`SIGNPATH_SIGNING_POLICY_SLUG` configure the bundle job, which has no environment.
The uploaded artifact contains `kandev.exe` and `agentctl.exe` at its ZIP root.
The SignPath artifact configuration must match that shape. Remote helpers are
Linux/macOS binaries and are not submitted for Authenticode signing.

## Control flow

1. Build the unsigned host binaries using the existing platform matrix.
2. Classify the purpose as Nightly, desktop validation, or Stable publication.
3. With complete eligible inputs, upload the two binaries and submit a signing
   request. Both network steps use best-effort outcomes; approval waits one hour.
4. Extract returned output into `runner.temp/signpath-signed`, separately from
   the unsigned build. Only a successful signing step reaches adoption.
5. Check that both output files are regular and non-empty, then install both
   with mode `0755` into `apps/backend/bin`. Incomplete output emits a warning.
6. Run existing bundle validation, archive creation, checksumming, required
   artifact retries, and publication gates without changing their contracts.

## Failure and recovery

The pinned action extracts ZIP entries sequentially and does not roll back a
partial extraction. Separate staging prevents a failed action from corrupting
unsigned input. A missing or empty signed pair is rejected before either
original is replaced. A local installation or storage error still fails the
producer instead of claiming the bundle is complete.

The guard reports incomplete configuration with status 1, a publishing test
policy with status 2, unsupported purpose with status 3, and Nightly with status
4. Workflow notices or warnings describe the unsigned fallback. Legacy backfill
source revisions lacking the readiness helper remain unsigned.

## Persistence

Signing artifacts and temporary output are run-scoped. No application state,
release versions, tag identities, helper manifests, or channel settings change.

## Security

The workflow keeps SHA-pinned actions and existing read-only bundle permissions.
Only eligible Windows jobs submit binaries. SignPath enforces its build-origin
and signing policy; Foundation release signing requires manual approval.
Readiness checks never print the API token. The slug guard restricts known test
policy names and does not independently inspect certificate trust.

Release-tag keys stay in protected release environments. This feature does not
move those secrets, change release administrator controls, or dispatch releases.

## Observability

Notices distinguish unconfigured or Nightly runs. Warnings distinguish refused
test policies, failed upload/signing, and incomplete returned output. The job's
step outcomes remain the evidence of whether adoption was reached.

## Related decisions

- [Release backfill and desktop diagnostics](../../../decisions/0029-release-backfill-and-desktop-diagnostics.md)
