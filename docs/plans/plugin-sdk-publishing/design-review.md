# SDK publishing: design reassessment and proposal

Reviewed: 2026-10-15. Evidence: local source at `d7a44e96e` and the uncommitted
design package. This review does not establish the state of upstream `main` or
registry ownership. No implementation or release had run at that checkpoint.

## Reconciliation status

On 2026-10-16, R1–R5 were incorporated into the canonical
[system design](../../specs/plugins/system-design/sdk-distribution.md), its
requirements, and work orders 01–05. The sections below preserve the original
findings and their rationale. Their proposed corrections now describe the
reconciled design. These were design gaps, not observed production failures.
Implementation followed the reconciliation after user authorization on
2026-10-19. The canonical requirements and system design define the contract;
the [plan](plan.md) and work orders record current validation. This review
retains its filename as a historical record.

Source was rechecked at PR head `8548962279d8d0700ac44d7a4013ce1a71353329` and
upstream `main` `76954ff53ea20b0263dd15c01c7149a531668341`. Upstream still omits
root SDK paths from backend/E2E selection and excludes backfill from prepare
signing. Its newer cache and test-only selection helpers are now explicit WO01
inspection targets. Official npm/Go guidance was rechecked on that date.
Registry ownership/setup remains unverified.

The optional lockfile-policy simplification remains unaccepted. WO03 retains
strict lockfile evidence. Host release numbers remain unresolved until release.
External template adoption remains outstanding.

## Recommendation

Keep the module split, shared SDK versions starting at `0.1.0`, and the
SDK-wide minimum host version. Correct the release state transitions and the
verification boundaries before implementation.

The five findings below identify missing design details, not observed production
failures. The existing specification checks prove structure and links; they do
not prove the release design works. The canonical design and work orders now
contain the required amendments. Use those files as implementation authority.

## Findings

### R1: Missing SDK tag has no complete recovery path (high)

**Trigger:** Preparation pushes the root host tag, then fails before pushing
`pluginsdk/v<S>`. The design assigns both tags to preparation and directs recovery
to `backfill_tag`.

**Evidence at the original review:** The draft assigned SDK signing to prepare.
The [reconciled release flow](../../specs/plugins/system-design/sdk-distribution.md#stable-release-integration)
now corrects that assignment. At the reviewed source, `release.yml` puts backfill preparation
in `release-validation` and excludes backfill from key import and tag creation
(lines 103, 550-603). Existing recovery therefore cannot create the missing SDK
tag through that path. The SDK work order only says to reuse the signed tag.

**Consequence:** A retry can keep failing on a missing module version even
though the host tag already exists. Changing the version would break the
promised same-version recovery rule.

**Proposed correction:** Create or verify the SDK tag in a dedicated protected
SDK publication job after the host release exists. Run that same operation on
normal release and backfill. Verify the signed host tag, approved signing
identity, target commit, and existing SDK tag before any mutation. Keep the
existing root-tag recovery policy separate.

**Required test:** Existing host tag plus absent SDK tag creates exactly one
signed SDK tag at the host commit. A matching SDK tag is reused; a conflicting
target or invalid signature fails without changing either tag.

### R2: First npm publication can encounter a circular prerequisite (high)

**Trigger:** npm requires a first package upload before its trusted publisher
can be configured, and prepare treats configured publishing as a hard prerequisite.

**Evidence:** The draft validates bootstrap prerequisites before creating the
release, but asks maintainers to bootstrap from the exact signed-release
tarball afterward. It does not specify the first-release exception or where
that tarball becomes available. npm's documented setup starts in existing
[package settings](https://docs.npmjs.com/trusted-publishers/).

**Consequence:** The prerequisite can prevent creation of the artifact needed
to satisfy it. This is a conditional design gap; package ownership and the
actual account setup have not been checked.

**Proposed correction:** Make `bootstrap_required` an explicit first-release
state. Confirm scope ownership before release. Build and retain the final
tarball from the signed host source, then report the required maintainer action
if initial OIDC publication is unavailable. Mark SDK publication incomplete.
The maintainer publishes that exact artifact, configures the publisher, and
reruns verification. Do not build a different local archive or publish a
placeholder version. Subsequent releases use OIDC without a token fallback.

**Required test:** Absent package yields a retained artifact and a precise
bootstrap result. It neither reports success nor prevents artifact creation.
The retry accepts only the matching uploaded artifact. Authentication and
registry transport errors must not be classified as package absence.

### R3: Artifact reuse needs an explicit source and byte identity (medium)

**Trigger:** A later host release reuses SDK `0.1.0`. Package README or tooling
files changed without a public contract change. Alternatively, a retry runs
under a different npm patch version.

**Evidence:** The draft compares a contract-input inventory and permits SDK
reuse, but also requires the registry tarball to match a regenerated archive.
It does not define the exact packaging-tool pin or a retained artifact record.
The existing npm workflow selects Node `24`, which is not an exact npm pin.

**Consequence:** Packing the later host checkout can produce different bytes
under the same SDK version. A valid unchanged-SDK release can fail its integrity
check. Checking only the version would instead accept an unverified artifact.

**Proposed correction:** Identify each SDK release by its original SDK tag,
source commit, canonical metadata, exact packaging-tool version, and npm
tarball integrity. Reuse verifies those original artifacts; it does not repack
the current host tree. For a new release, pack once, test that archive, and
retain the archive and receipt with the originating host release. Recovery
uses the retained archive. If it must rebuild, compare against the retained
receipt before uploading. A deliberately changed package gets a new version.

**Required test:** A new host release with an SDK README-only change can reuse
the original SDK artifact. A changed SDK runtime or public type contract cannot
reuse that version. A retry with different archive bytes fails.

### R4: Local and public consumer tests need different trust settings (medium)

**Trigger:** A PR installs an unpublished SDK version from the proposed local
Go proxy using an empty cache and default checksum settings.

**Evidence:** The draft specifies a local proxy but no checksum-database policy.
Go can still query its public checksum database for a new module version. A
local proxy alone does not bypass that check. See the
[Go module reference](https://go.dev/ref/mod#authenticating).

**Consequence:** The test can fail because its unpublished version cannot be
authenticated publicly. A broad workaround such as disabling checksum checks
globally can then leak into public-release verification and weaken the evidence.

**Proposed correction:** In local packaging tests only, set `GONOSUMDB` to the
exact SDK module path. Set `GOWORK=off`, use disposable module/build caches, and
route the SDK through the test proxy. Do not allow a missing local SDK archive
to fall back to a public copy of the same version. Public release checks use
fresh caches, public proxy resolution, and normal checksum verification with
no inherited private-module exemptions. Test the two environments explicitly.

**Required test:** Removing the local SDK archive fails even if that version
exists publicly. Public mode rejects local proxy or checksum-bypass settings.
Both modes invoke the same consumer build and compatibility assertions.

### R5: SDK-only changes can miss host CI without a tested trigger contract (high)

**Trigger:** A later PR changes only `pluginsdk/**`, after the extraction PR
has already merged.

**Evidence:** Backend and E2E workflows currently select `apps/backend/**`
and other explicit roots. Neither selection includes root `pluginsdk/**`.
The work order says to audit CI discovery, but its verification commands do
not prove that an SDK-only change schedules host checks. Extraction itself
changes backend files and therefore cannot expose this future skip.

**Consequence:** SDK unit tests could pass while changed SDK types break the
host or its runtime fixture, with host CI never scheduled.

**Proposed correction:** Name `.github/workflows/backend-tests.yml` and
`.github/workflows/e2e-tests.yml` explicitly in WO01. Include the root module
and relevant SDK tooling in change selection, and review both module manifests
in cache/dependency preparation. Add trigger-contract tests for an SDK-only
diff. SDK tests remain a separate module command; host compile and the affected
runtime checks must also run. Review E2E build selection because the host binary
consumes SDK code, even though this work adds no browser scenario.

**Required test:** An SDK-only source change selects SDK tests and host checks.
A documentation-only SDK change follows the intentional documentation policy.
Do not use an extraction PR with backend changes as the only trigger fixture.

## Proposed walkthrough

This section explains the review corrections. The task walkthrough now uses
the [complete proposal walkthrough](proposal-walkthrough.md), which follows
all five implementation work orders and compares the proposal with the issue.

### Step 1: Give authors two ordinary dependencies

Keep the Go source and canonical protobuf in root `pluginsdk/`. External plugins
import `github.com/kdlbs/kandev/pluginsdk`. Keep the existing npm package name
`@kandev/plugin-sdk` and its runtime-free type exports.

The host uses the same Go source through its repository-local replacement.
An external consumer resolves a public version with no replacement or Kandev
checkout. Move source imports atomically, while preserving gRPC service names,
field numbers, and the go-plugin handshake. This source migration does not
require an existing plugin binary to change its protocol.

### Step 2: Make compatibility explicit

Keep one SDK release record and generate its npm copy. SDK `0.1.0`, manifest
API `1` or `2`, and the minimum Kandev version describe different things.
Resolve the first host baseline from the actual shipping release.

The checker reads the resolved Go/npm SDK records. The plugin's host minimum
must meet the greater baseline when it consumes two different SDK releases.
Its API version must be supported by both records. Feature checks and capability
authorization still apply at runtime.

**Optional simplification:** The current checker also parses npm/pnpm lockfiles
and rejects all unsupported lock formats. That is an additional dependency
origin policy, beyond the selected host-baseline policy. It excludes otherwise
compatible consumers and adds package-manager maintenance. I recommend keeping
compatibility checks focused on installed records and explicit local replacements,
with registry-origin proof in isolated release tests. This would require revising
AC-PLUGINS-SDK-DISTRIBUTION-002.2 and WO03; it is a recommendation, not a silently
accepted requirement change.

### Step 3: Publish one exact candidate after the host release exists

1. Prepare the normal host release PR and resolve pending SDK metadata.
2. After the signed host release exists, check out that exact source. Pack and
   test the SDK candidate with pinned tools. Retain the npm archive and receipt.
3. In the protected SDK publication job, create or verify the signed SDK tag.
   Normal and backfill paths use the same idempotent operation.
4. Publish the retained npm archive through OIDC, or report the explicit initial
   bootstrap state. Verify an existing npm version before treating it as success.
5. Install both public SDKs from clean consumers. Report Stable completion and
   allow contributor notices only after the SDK gate and existing channels pass.

Keep signing permission and key access in the tag step/job. Give the npm step
the publishing identity it needs. Neither PR tests nor dry-run jobs get those
mutation capabilities. An unchanged SDK verifies its original release artifacts.

### Step 4: Resume from the observed publication state

| SDK tag | npm version | Required action |
| --- | --- | --- |
| Absent | Absent | Verify host source, create tag, publish candidate, verify consumers |
| Matching | Absent | Keep tag; publish the retained candidate or report bootstrap needed |
| Absent | Matching | Verify npm identity; create the missing tag; verify consumers |
| Matching | Matching | Perform public consumer verification; never republish |
| Conflicting or unreadable | Any | Stop with the exact conflict/read failure; do not mutate identities |

An unreadable npm state also stops the operation. Missing `latest` may be repaired
only after artifact verification and with a rule that prevents downgrades.
The retry always uses the original SDK source and candidate identity. Publication
remains partial until both public installs pass.

### Step 5: Prove package isolation and host continuity

Before merge, prove local Go/npm packaging, manifest-baseline failures, host
compile, a pinned legacy plugin subprocess, and SDK-only CI selection. The
local test proxy has a narrow explicit checksum exception.

During release, prove public resolution with normal trust settings and fresh
caches. Confirm both installed records and the npm integrity against the
retained receipt. Keep this evidence separate from the earlier local test.

The external template migration remains a separate repository operation under
the existing scope. The release handoff must make it visible: until that work
lands, the canonical template still teaches the old sibling-checkout workflow.

## Work-order amendments

| Work order | Required amendment |
| --- | --- |
| [01](task-01-go-module.md) | Exact backend/E2E trigger paths and tests; local proxy isolation and checksum policy |
| [02](task-02-frontend-package.md) | Test the exact packed archive; define published files and receipt inputs |
| [03](task-03-compatibility-check.md) | Preserve strict host/API and npm/pnpm lock checks; optional simplification remains unaccepted |
| [04](task-04-stable-publication.md) | Recoverable SDK signing job, bootstrap state, original-artifact reuse, and the complete state table |
| [05](task-05-author-migration.md) | Document bootstrap/recovery and keep external template adoption visibly outstanding |

## Review validation

Document checks passed on 2026-10-15: specification catalog validation,
specification lint, whitespace checks, and 22 local links across seven plan
documents. On 2026-10-16 the task walkthrough was replaced with the complete
proposal walkthrough linked above, following user feedback about its scope.
At that review checkpoint, product code and release paths had not been executed,
and the original five work orders were pending. Required amendments were
reconciled before implementation. See the plan for subsequent validation;
public publication remains a separate release gate.
