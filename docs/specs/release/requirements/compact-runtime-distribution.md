---
status: draft
system: release
created: 2026-09-23
owners:
  - kandev
---

# Compact Runtime Distribution Requirements

## Overview

The standard Stable runtime currently downloads four remote agent-controller
executables even when an installation runs only local work. The release system
owns the artifact contents and publication guarantees. This capability keeps
local startup self-contained, obtains a matching remote helper when needed, and
provides a separate complete command-line archive for offline installations.

## Terminology

- **Standard runtime:** The default Stable command-line archive consumed by
  GitHub download, npm, Homebrew tap, Scoop, winget, Chocolatey, and Desktop
  build input after the compact-release cutover.
- **Remote helper:** One `agentctl` executable for a supported remote Linux or
  macOS operating system and CPU architecture.
- **Full offline runtime:** A Stable command-line archive for one host platform
  that contains the host executables and every supported remote helper.

## Requirements

### REQ-RELEASE-COMPACT-RUNTIME-001: Compact standard runtime

**Intent:** Reduce the ordinary installation download without making local
operation depend on a later asset fetch.

#### Acceptance criteria

- **AC-RELEASE-COMPACT-RUNTIME-001.1:** For each supported Stable host platform,
  the standard runtime archive shall contain the host `kandev` and `agentctl`
  executables and the information needed to identify the matching remote-helper
  assets. It shall not contain the four remote-helper executables. The Windows
  standard ZIP shall have the same runtime contents as its TAR archive.
- **AC-RELEASE-COMPACT-RUNTIME-001.2:** A standard-runtime installation shall
  start, run local agent work, and install or start a service without fetching a
  remote helper.
- **AC-RELEASE-COMPACT-RUNTIME-001.3:** Native and remote `agentctl` builds
  shall preserve the existing MCP task-plan and task-title tool names,
  validation, request payloads, and error behavior.

### REQ-RELEASE-COMPACT-RUNTIME-002: Exact remote-helper resolution

**Intent:** Preserve remote execution while downloading only the helper that a
selected remote platform needs.

#### Acceptance criteria

- **AC-RELEASE-COMPACT-RUNTIME-002.1:** When a supported remote executor first
  needs a helper that is absent locally, Kandev shall obtain only the helper for
  that remote platform from the same Stable release as the running host. It
  shall not substitute the host-native `agentctl` for a missing remote helper.
- **AC-RELEASE-COMPACT-RUNTIME-002.2:** After a matching helper has been
  obtained, subsequent use of that platform shall work from the local cache,
  including after a restart and without network access. A newer host release
  shall not use a cached helper from an older release.
- **AC-RELEASE-COMPACT-RUNTIME-002.3:** When a remote-helper download is
  unavailable, incomplete, or fails integrity validation, the selected remote
  operation shall fail with an actionable error before starting or uploading
  that helper. Local operation and startup shall remain available, and a later
  retry shall remain possible.
- **AC-RELEASE-COMPACT-RUNTIME-002.4:** Existing explicit operator helper-path
  overrides shall retain their precedence and failure behavior.
- **AC-RELEASE-COMPACT-RUNTIME-002.5:** Cached helpers shall remain available
  across a rollback and for any active or reconnectable remote container that
  uses their host path. Unreferenced older entries shall be removed under a
  bounded retention rule; uncertain references shall prevent deletion.
- **AC-RELEASE-COMPACT-RUNTIME-002.6:** A first remote launch that downloads a
  helper shall show a visible preparation step on desktop and phone. The step
  shall complete or fail with the download, and a failed or timed-out fetch
  shall identify the remote platform and a recovery action.

### REQ-RELEASE-COMPACT-RUNTIME-003: Complete offline and channel variants

**Intent:** Make the network-dependent standard install a choice rather than
the only way to obtain remote execution.

#### Acceptance criteria

- **AC-RELEASE-COMPACT-RUNTIME-003.1:** Every Stable host platform shall also
  offer a full offline command-line archive containing all four supported
  remote helpers.
  Its supported remote executors shall work without downloading a helper.
  Windows shall offer a full offline ZIP as well as a TAR archive.
- **AC-RELEASE-COMPACT-RUNTIME-003.2:** Stable npm runtime packages, the
  Homebrew tap, Scoop, winget, and Chocolatey shall consume the standard
  archives or Windows ZIP. The normal Desktop installers and signed updater
  bundles shall contain the standard slim runtime. Container images and npm
  Nightlies shall retain complete local helper sets in this rollout.
- **AC-RELEASE-COMPACT-RUNTIME-003.3:** Existing complete runtime bundles that
  have no remote-helper asset metadata shall continue to launch and resolve
  their local helpers without a new network dependency.
- **AC-RELEASE-COMPACT-RUNTIME-003.4:** Stable publication shall fail before
  publishing any channel when a required standard archive, full offline
  archive, Desktop installer or updater, remote-helper asset, or integrity
  record is missing or mismatched.
- **AC-RELEASE-COMPACT-RUNTIME-003.5:** Until a release contains a launcher and
  resolver that accept the compact layout, every archive published under an
  existing default name shall remain a complete runnable bundle. The first
  default-name switch, Desktop slim-resource switch, and container
  full-archive switch shall be published together.
- **AC-RELEASE-COMPACT-RUNTIME-003.6:** A Stable Desktop installer and its
  updater shall start and run local sessions with only the host executables
  and release-bound helper manifest. Remote execution shall use the same
  verified on-demand helper resolution as other standard installs. An update
  from an older complete Desktop installation shall keep the app launchable
  and use the existing updater track without restoring bundled helpers.

## Out of scope

- Adding Windows as a remote-helper target.
- Making npm Nightlies or container images smaller in this rollout.
- Publishing a separate full offline Desktop installer or updater track; the
  full offline command-line archive remains available.
- Replacing the Kubernetes clientset in `kandev`.
- Changing the MCP tool surface or the supported remote-platform matrix.
- Adding an operator-configured asset mirror in this rollout. The full
  offline archive and explicit helper-path overrides cover restricted networks.

## Implementation plan

- [Compact runtime distribution](../../../plans/compact-runtime-distribution/plan.md)
