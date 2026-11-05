---
status: active
system: plugins
created: 2026-10-09
owners:
  - kandev
---

# Plugin SDK Distribution Requirements

## Overview

Runtime plugin authors shall install pinned Go and frontend SDKs without a
Kandev checkout. The plugin system owns the public author contract and its
compatibility policy; the release system supplies publication infrastructure.
This capability addresses [issue #4358](https://github.com/kdlbs/kandev/issues/4358).

## Terminology

- **SDK release:** One independently versioned release of the Go and npm SDKs.
- **Host baseline:** The minimum Stable Kandev version supported by that SDK.
- **Manifest API version:** The manifest's `api_version`, independently selected
  from the API versions supported by the SDK and host.

## Requirements

### REQ-PLUGINS-SDK-DISTRIBUTION-001: Installable public SDKs

**Intent:** A plugin repository shall contain everything needed to resolve its
SDK dependencies through public package infrastructure.

#### Acceptance criteria

- **AC-PLUGINS-SDK-DISTRIBUTION-001.1:** From an empty module and module cache,
  `go get github.com/kdlbs/kandev/pluginsdk@<released-version>` shall succeed.
  A plugin using that module shall build, vet, and test without an SDK
  replacement directive, a Kandev checkout, or a Go workspace override.
- **AC-PLUGINS-SDK-DISTRIBUTION-001.2:** From an empty project,
  `npm install --save-dev --save-exact @kdlbs/plugin-sdk@<released-version>`
  shall succeed through the public npm registry. Its exported types shall
  typecheck without Kandev, React, or Zustand application dependencies.
- **AC-PLUGINS-SDK-DISTRIBUTION-001.3:** The resolved Go SDK shall not depend on
  Kandev's backend module or private application packages. Both SDK artifacts
  shall contain the contract and compatibility information needed by consumers.

### REQ-PLUGINS-SDK-DISTRIBUTION-002: Explicit compatibility

**Intent:** SDK upgrades shall expose their host requirement before a plugin is
packaged or released.

#### Acceptance criteria

- **AC-PLUGINS-SDK-DISTRIBUTION-002.1:** Go and npm SDK releases shall share an
  independent SDK version, initially `0.1.0`, with a published mapping to the
  originating Stable Kandev release, host baseline, and supported manifest API
  versions. The first host release number shall be resolved from the actual
  release rather than guessed during planning.
- **AC-PLUGINS-SDK-DISTRIBUTION-002.2:** A plugin author shall have a CI command
  that checks the resolved SDK dependencies against `min_kandev_version` and
  `api_version`. It shall reject a missing, malformed, or too-low host minimum,
  an unsupported API version, unresolved compatibility evidence, and local SDK
  substitutions. The minimum shall identify a complete Stable version, without
  prerelease or build suffixes. Registry SDKs installed in npm workspaces shall
  be supported when their resolved location matches the workspace lock evidence.
  A checkout-directory symlink shall not invalidate registry dependency evidence.
  Lockfile parsing shall use a finite limit sized for workspace dependencies,
  separate from the smaller limit for SDK metadata.
  When both SDKs are used, the minimum shall meet both baselines.
- **AC-PLUGINS-SDK-DISTRIBUTION-002.3:** Every SDK release shall declare its
  compatibility change. Before `1.0.0`, a breaking SDK change shall increase
  the minor version and provide migration guidance. From `1.0.0`, breaking
  changes shall increase the major version, additive changes the minor version,
  and compatible fixes the patch version. SDK numbering alone shall not change
  manifest or transport protocol versions.
- **AC-PLUGINS-SDK-DISTRIBUTION-002.4:** The published host baseline shall apply
  to the whole SDK surface. Runtime feature detection and capability checks
  shall still apply above that baseline; building against a newer SDK shall
  not grant additional host authority.

### REQ-PLUGINS-SDK-DISTRIBUTION-003: Reproducible SDK publication

**Intent:** A contract-changing Stable Kandev release shall make its matching
SDKs available with verifiable provenance and recoverable publication.

#### Acceptance criteria

- **AC-PLUGINS-SDK-DISTRIBUTION-003.1:** Each Stable release that changes the
  plugin contract shall publish the matching versioned Go and npm SDKs. A
  release with an unchanged contract may reuse the existing SDK release and
  shall identify that SDK in its release compatibility information. Reuse shall
  verify the original SDK artifacts. Changed package bytes shall require a new
  SDK version.
- **AC-PLUGINS-SDK-DISTRIBUTION-003.2:** Public SDK versions shall be immutable.
  Retrying partial publication shall verify existing artifacts, resume missing
  publication, and reject a conflicting tag or package rather than overwrite it.
  A retry after host publication shall recover a missing SDK tag at its original
  signed source. It shall reject an invalid signature or unreadable identity.
- **AC-PLUGINS-SDK-DISTRIBUTION-003.3:** A Stable release requiring a new SDK
  shall not be reported complete until both public SDKs can be installed and
  the compatibility check succeeds in a clean consumer. Dry-run, desktop-only
  validation, and Nightly runs shall not publish Stable SDK artifacts or move
  their npm `latest` tag.
- **AC-PLUGINS-SDK-DISTRIBUTION-003.4:** Automated npm SDK publication shall use
  the repository's trusted publishing workflow and provenance. Registry
  ownership and first-publish setup shall be documented as explicit maintainer
  prerequisites, with any bootstrap publication distinguished from automated
  provenance-bearing releases. If initial publisher setup requires an existing
  package, the release shall retain the exact tested candidate and report the
  required maintainer action. SDK publication shall remain incomplete until
  that candidate passes public verification. A registry read or authentication
  failure shall not count as package absence.

### REQ-PLUGINS-SDK-DISTRIBUTION-004: Host continuity and author migration

**Intent:** Extracting the SDK shall preserve plugin execution and provide a
bounded migration for existing authors.

#### Acceptance criteria

- **AC-PLUGINS-SDK-DISTRIBUTION-004.1:** The host, in-tree plugin fixtures, and
  SDK tests shall continue to build and execute through the same gRPC service,
  handshake, and Host contracts. A previously built supported plugin binary
  shall continue to handshake and make a Host call after extraction.
- **AC-PLUGINS-SDK-DISTRIBUTION-004.2:** Published author guidance shall show
  the old and new Go imports, removing sibling-checkout dependencies, exact
  Go/npm pinning, compatibility validation in CI, and deliberate SDK upgrades.
  The canonical template shall have a documented migration handoff.

## Out of scope

- Renaming or publishing the full backend module.
- New Host RPCs, manifest fields, capabilities, or rendered UI.
- SDK Nightly packages, per-feature host minimums, and promotion to SDK `1.0.0`.
- Updating third-party plugin repositories or automatically bumping plugins.
- A compatibility shim for the old unfetchable Go import path.

## Implementation plans

- [SDK publishing plan](../../../plans/plugin-sdk-publishing/plan.md).
