# Plugin SDK publishing: the complete proposal

Read time: about seven minutes. This explains the proposed implementation in
order. No SDK has been published by this task.

## How this relates to issue #4358

The core architecture follows [the issue](https://github.com/kdlbs/kandev/issues/4358).
It asks for a standalone Go module, a public npm package, and explicit version
compatibility. The proposal makes the repository layout, compatibility policy,
release process, and migration concrete.

| Issue proposal | This plan's concrete choice |
| --- | --- |
| A Go SDK module independent of the backend | Keep it in this monorepo at `pluginsdk/`, with public import `github.com/kdlbs/kandev/pluginsdk` |
| Include SDK code and generated plugin protocol | Move the SDK, canonical `.proto`, and generated Go types together; host and plugins use one definition |
| Publish `@kandev/plugin-sdk` (original issue name) | Publish the frontend types as `@kdlbs/plugin-sdk`, selected after scope ownership checks |
| Document SDK, host, and manifest API compatibility | Give both SDKs a shared independent release version, publish a compatibility record, and check the manifest in author CI |
| Release SDKs when the plugin contract changes | Extend Stable release automation with immutable artifacts, explicit recovery, and clean public installation checks |

The shared SDK version and SDK-wide host minimum are the choices made during
our planning discussion. The issue leaves those policy details open. The later
[design review](design-review.md) identified release and test corrections.
Those required corrections now appear in the canonical design and work orders.
The module architecture remains unchanged.

## Step 1: Extract the Go SDK and keep Kandev working

**Result:** Go plugin authors can depend on the SDK without depending on the
whole backend module.

Move `apps/backend/pkg/pluginsdk` into root `pluginsdk/`. Give that directory
its own `go.mod` with only the dependencies the SDK and its tools need. Move the
canonical protobuf and generated Go types into that module too.

For a plugin author, the source migration is:

```go
// Current import
import "github.com/kandev/kandev/pkg/pluginsdk"

// Proposed public import
import "github.com/kdlbs/kandev/pluginsdk"
```

The author then pins a published module version and removes the replacement
that points at a sibling Kandev checkout. This is the module split requested
in the issue. The plan additionally chooses the exact package layout.

Kandev itself imports the new package and uses a repository-local replacement
to compile the SDK source beside it. That lets host and SDK changes develop
together before publication. External plugin builds do not need that replacement.
The SDK remains in the Kandev repository; there is no separate SDK repository
or second production copy of the protocol.

This changes source import paths. It does not intentionally change the
messages exchanged between the host and an already-built plugin. A legacy
plugin subprocess test must prove that compatibility. Update Docker paths and
CI selection so later SDK-only changes also test the host.

**Work order:** [01: Go module](task-01-go-module.md).

## Step 2: Make the frontend package installable by itself

**Result:** A plugin UI can typecheck with the npm SDK and its own project files.

Use the public name `@kdlbs/plugin-sdk`, selected on 2026-10-26 because the
maintainer controls that npm organization. Replace the former local name
`@kandev/plugin-sdk` in dependencies, imports, and lockfiles. Existing installed
frontend bundles do not resolve this name at runtime. Its types live separately
from the web application. Prepare its package contents, license, repository
metadata, and compatibility-record export for publication.

Pack it into an npm archive and install that archive in a temporary project
outside the repository. Typecheck there using an independent TypeScript
configuration. This catches files or configuration that a workspace link could
silently supply but a real npm install would lack.

The package describes the frontend Host interfaces. It does not bundle another
React runtime or copy the Kandev application's state implementation.

This step prepares and tests the artifact. Step 4 makes it publicly available.

**Work order:** [02: Frontend package](task-02-frontend-package.md).

## Step 3: Define versions and catch incompatible manifests in CI

**Result:** Authors can tell what host version their selected SDK requires
before releasing a plugin.

Both SDKs start at independent version `0.1.0`. That number does not mean
Kandev `0.1.0`, and it does not replace manifest `api_version`.

Publish one compatibility record with the SDK version, originating Kandev
release, minimum Kandev version, and supported manifest API versions. Maintain
one source record and generate its npm copy. Resolve the first host version
from the actual release rather than guessing it now.

Add an author CI command that reads the SDK versions actually selected by the
plugin. It checks the manifest's `min_kandev_version` and `api_version` against
those records. If Go and npm dependencies use different SDK releases, the
manifest minimum must meet both host baselines.

The selected policy is conservative: installing a newer SDK can require a
newer host even if a particular plugin only uses older APIs. Runtime capability
checks still apply. A newer SDK grants no additional permissions.

The issue asks for version mapping and CI enforcement. The shared version,
single record, and checker are our concrete implementation of that request.
The review's optional lockfile-policy simplification is still a proposal; it
does not change the selected host-minimum policy.

**Work order:** [03: Compatibility check](task-03-compatibility-check.md).

## Step 4: Publish and verify SDKs during Stable releases

**Result:** The packages become publicly installable and a failed publication
can resume without changing an existing version.

The revised release proposal is:

1. Prepare the host release and resolve pending SDK compatibility metadata.
2. Once the signed host release exists, build and test the exact SDK candidate.
   Retain the npm archive and a record of its source, tooling, and integrity.
3. A protected SDK job creates or verifies `pluginsdk/v<S>` at the correct
   source commit and publishes the matching npm version.
4. If first-publication setup is needed, retain the candidate and report the
   precise maintainer action as `bootstrap_required`. Publication stays incomplete.
   A retry verifies any existing artifacts and
   creates only the missing ones. Conflicting artifacts fail verification.
5. Install both SDKs from public infrastructure in clean projects. Count the
   release as complete only when these checks and the existing channels pass.

When the SDK has not changed, verify and reuse its original release artifacts.
Do not repack a later host checkout under the old SDK version. Nightly and
dry-run flows do not publish Stable SDK versions.

The issue requires tagged and published SDK releases. The retained archive,
bootstrap state, and recovery rules make that requirement operational. These
are the corrections from the design review, now reconciled into the canonical
design and WO04. Signing and npm publication use separate protected jobs.
The release workflow is implemented; public availability still requires an
authorized Stable release and its public consumer checks.

**Work order:** [04: Stable publication](task-04-stable-publication.md).

## Step 5: Give authors a usable migration path

**Result:** An author knows exactly what to change in an existing plugin.

The migration guide will explain how to change Go imports, remove the backend
replacement and npm `file:` dependency, install exact SDK versions, update the
manifest minimum, and add the compatibility check to CI.

Update live source references and maintainer release instructions. Explain
SDK upgrades, the one-time npm setup, and recovery after a partial publication.
Document the canonical external template's required migration as a separate
repository handoff. Until that migration lands, the template still carries the
old sibling-checkout setup; docs alone do not fix it.

The finished author workflow is: clone the plugin repository, install its
pinned dependencies, build/typecheck/test, and validate compatibility. A Kandev
checkout is no longer a prerequisite.

Implementation can establish package and automation readiness. The first
authorized release must establish actual public availability. Keep those two
completion claims separate.

**Work order:** [05: Author migration](task-05-author-migration.md).


## Existing installations: runtime support

**The intended outcome is that supported, already-built plugins keep working
after extraction.** Operators should not need to reinstall or rebuild a plugin
solely because its Go source package moved.

A compiled Go plugin does not resolve its source imports when Kandev launches
it. The proposal preserves the gRPC service names, message fields, handshake,
and Host contracts. Existing manifest, capability, platform, and installation
checks still apply. Publication adds no requirement for an installed plugin to
carry the new SDK release record or run the author checker.

The frontend package continues to provide types, and publication changes no
frontend Host API by itself. Existing UI bundles are therefore intended to
remain usable. The SDK move does not establish new support for private Host
APIs or previously unsupported plugin behavior.

Local tests prove the v0.97.0 fixture handshake and Host state call. The
[host continuity requirement](../../specs/plugins/requirements/sdk-distribution.md#req-plugins-sdk-distribution-004-host-continuity-and-author-migration)
still awaits Docker validation. This does not certify every community plugin.

## Existing source builds: migration boundary

**Yes, the extraction can break an existing source build that follows the new
Kandev source while retaining the old Go import.** The old
`github.com/kandev/kandev/pkg/pluginsdk` package will move. The accepted design
does not retain a source alias at that path.

A plugin that updates its sibling checkout to a post-extraction revision must
migrate its Go imports and module dependency. Builds pinned to a pre-extraction
revision retain that source layout. Extraction does not rewrite their pin or
automatically upgrade their dependencies. This is not a guarantee about future
toolchain availability or unrelated dependency failures.

After public SDK verification, authors can replace the sibling dependency with
an exact published Go version. Authors using the frontend SDK also replace
its local `file:` dependency with an exact npm version. Its npm name and source
location remain unchanged, so the Go move alone does not remove that local npm
path. The new checker intentionally rejects local replacements and unsupported
lock evidence when an author adopts it in CI. It does not disable installed
plugins or add itself to existing plugin CI automatically.

The [author inventory](author-notification-inventory.md#packaging-tool-follow-through)
also identifies backend-owned `plugin-pack` dependencies. Migration must inspect
packaging commands and CI before claiming the entire build needs no checkout.
Those tools are not automatically published by extracting the SDK.

There is a transition window between merging extraction and verifying the first
public SDK release. Authors tracking `main` need advance notice before the move.
During that window, a pre-extraction pin retains the old build inputs. Do not
instruct authors to install a public SDK version before it is available.

## Older hosts and newer plugin releases

**The SDK-wide host minimum applies when an author adopts a new SDK release.**
It does not retroactively raise the minimum in an already-published plugin's
manifest. Existing plugin releases keep their declared host/API requirements.

A plugin rebuilt with the published SDK must declare a minimum host that meets
that SDK's baseline. This can exclude older hosts even if the plugin only uses
older APIs. That conservative behavior follows the accepted SDK-wide policy.
The first SDK's host baseline remains unresolved until the actual host release.

For example, an existing plugin release can remain usable on host A. If a new
plugin release adopts an SDK requiring newer host B, its manifest must require B.
Users staying on A need a compatible plugin release. This proposal does not add
marketplace version selection, automatic downgrade, or an old-host support period.
Maintaining an older compatible plugin release remains an author decision.

SDK publication does not change manifest API or transport protocol versions by
itself. Supported API sets and runtime permissions remain separate checks.

## Support evidence and rollout

The plan defines the checks below. Advance author notice is a rollout
recommendation, and delivery still needs explicit authorization.

1. Build a legacy plugin from the pre-extraction `v0.97.0` source. Run it as a
   real subprocess against the extracted host. Prove its handshake and Host
   state call in [work order 01](task-01-go-module.md).
2. Run host/fixture checks and isolated Go/npm consumer tests. Preserve the
   frontend contract and verify SDK-only CI selection. New-source typechecks
   alone do not prove every previously shipped UI bundle works.
3. Prepare an advance notice for affected authors before removing the old source
   path. Provide the
   public migration guide and verified version mapping when publication succeeds.
   The [notification inventory](author-notification-inventory.md) identifies
   candidates. No notifications were sent by this task.
4. Separate test results from support claims. A legacy fixture proves the tested
   handshake and Host call, not all behavior in every community plugin. Broader
   ecosystem claims need representative plugin smoke tests and author feedback.

The 29-repository dependency inventory is not a compatibility certification.
All five work orders are complete. Local checks, implementation CI, and review
cleanup passed. The legacy subprocess and Docker builder checks passed. The
[template handoff](template-migration.md) remains separate, as does public publication.

A subsequent [adversarial implementation review](adversarial-review.md) found
three bugs in publication and author validation. The corrections preserve the
packed-record digest, reject host minimum suffixes that Kandev rejects, and
support registry SDKs hoisted by npm workspaces with exact lock evidence.
Existing plugin runtime protocols remain unchanged. Each correction has a
regression test; the remediation is validated separately from the earlier CI.
