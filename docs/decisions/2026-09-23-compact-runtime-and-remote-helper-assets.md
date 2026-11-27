# ADR-2026-09-23-compact-runtime-and-remote-helper-assets: Publish slim Stable runtimes with verified remote helpers

**Status:** accepted
**Date:** 2026-09-23
**Area:** infra, backend

## Context

Each Stable runtime archive contains a host `kandev`, a host `agentctl`, and
four cross-platform remote `agentctl` helpers. In v0.95.1 the Linux x64
archive is 149.1 MiB compressed; repacking only the host executables yields
70.4 MiB before the planned `agentctl` dependency reduction. Most local-only
installations never use the remote helpers.

[ADR-2026-08-05-homebrew-remote-helper-audit](2026-08-05-homebrew-remote-helper-audit.md)
chose a complete Homebrew bundle to preserve offline remote execution and
avoid runtime downloads. That trade-off makes every standard distribution pay
for all remote targets. The new requirement retains an explicit full offline
option while changing the default Stable distribution.

## Decision

After an atomic release cutover, Stable command-line archives, the npm,
Homebrew-tap, Scoop, winget, and Chocolatey packages derived from them, and
the normal Desktop installer and updater contain host executables and a
release-bound remote-helper manifest, but no remote-helper executables. The
Stable GitHub Release also publishes one canonical asset for each supported
remote platform and a full offline command-line archive for every host
platform. Kandev retrieves a missing helper
only when a remote executor selects that platform, verifies it against the
installed manifest, and caches it outside the installed bundle. The host
native `agentctl` is not a substitute for a missing remote build.

Container images and npm Nightlies retain complete helpers in this rollout.
Nightlies have no GitHub Release from which to retrieve a missing
version-specific asset. Existing complete bundles, including older Desktop
installations, continue to use local helpers. The existing signed Desktop
updater track replaces a complete runtime with a standard runtime; no separate
full offline Desktop installer or updater is published.

The launcher and resolver must accept compact bundles before any existing
default archive name changes. During development, the default names stay
complete; compact archives use staging names. The final cutover changes the
default archive contents, switches Desktop preparation to the standard
archive, and switches container inputs to the full archives together. Rust
Desktop runtime validation must accept manifest-bearing standard resources
before that cutover. Cached helpers retain the current and two previous
versions, plus any version still referenced by an active or reconnectable
container. A first download is visible in the existing prepare progress.

This decision supersedes the custom-tap complete-bundle and mismatched-binary
audit rule in ADR-2026-08-05-homebrew-remote-helper-audit at the Stable
distribution cutover. It does not change the separate Homebrew Core
source-build proposal.

## Consequences

- Local-only Stable command-line and Desktop downloads and installed runtimes
  become smaller.
- A first remote launch from a standard install requires GitHub Release
  access unless the matching helper is already cached or an operator supplies
  an existing explicit helper path.
- A failed or unverified fetch blocks only that remote operation. The full
  offline command-line archive remains the supported no-fetch choice; the
  Desktop installer and updater do not have a separate full offline variant.
- Release publication must build and validate canonical helpers, the manifest,
  standard archives, full archives, and all consuming channels as one versioned
  set. The Homebrew tap's foreign-binary audit exception must be retired when
  its standard archive switches to the slim form.
- Cache data remains separate from immutable npm, Homebrew, Scoop, Desktop,
  and archive installation directories.
- A release helper asset is immutable: a retry or backfill with the same
  asset name must match its already-published digest.

## Alternatives Considered

- **Keep all helpers in every archive:** Preserves simple offline behavior but
  leaves the measured download overhead in every default installation.
- **Prune helpers to the host platform:** Remote SSH, Docker, Sprites, and
  Kubernetes targets can differ from the host, so this breaks supported work.
- **Keep Desktop complete:** Preserves its offline remote behavior but leaves
  every Desktop installer and updater carrying all four remote platforms.
- **Make containers and Nightly slim:** Changes container startup and offline
  behavior, and gives Nightly no immutable GitHub Release asset source. Defer
  those channel changes.
- **Split helpers into separate required package-manager dependencies:** Keeps
  every install large and complicates synchronized upgrades.
- **Use stronger archive compression such as xz or zstd:** This could reduce
  transfer size, but would not remove unused platform helpers from each
  install and would change the existing archive consumers. Measure it
  separately if further size reduction is needed.
