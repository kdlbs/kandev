---
status: draft
system: release
requirements:
  - REQ-RELEASE-COMPACT-RUNTIME-001
  - REQ-RELEASE-COMPACT-RUNTIME-002
  - REQ-RELEASE-COMPACT-RUNTIME-003
---

# Compact Runtime Distribution System Design

## Purpose and boundaries

The release system owns the published artifact set, channel composition, and
integrity record. This design also describes the launcher and executor seams
needed to consume those artifacts. Executor-specific upload, bind-mount, and
remote process lifecycles remain in the executor system; they continue to
receive a local path or binary bytes from `AgentctlResolver`.

The completed rollout changes Stable command-line archives, their npm,
Homebrew-tap, Scoop, winget, and Chocolatey consumers, and the normal Desktop
installer and updater track. Container images and npm Nightlies keep complete
helper sets. The existing
prepare-progress panel gains one localized helper-download row on desktop and
phone; its layout stays unchanged and the progress event adds one optional
typed field.

## Requirement mapping

| Requirement                       | Design sections                                                                                                           |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| `REQ-RELEASE-COMPACT-RUNTIME-001` | [Shared MCP contract](#shared-mcp-contract), [Artifact production](#artifact-production)                                  |
| `REQ-RELEASE-COMPACT-RUNTIME-002` | [Runtime resolution](#runtime-resolution), [Integrity and failure](#integrity-and-failure)                                |
| `REQ-RELEASE-COMPACT-RUNTIME-003` | [Artifact production](#artifact-production), [Channel consumption](#channel-consumption), [Compatibility](#compatibility) |

## Current footprint and build boundary

The v0.95.1 Linux x64 Stable TAR is 149.1 MiB. Repacking its existing host
executables alone yields 70.4 MiB. A same-source, stripped Go build experiment
reduced native Linux `agentctl` by 23,961,184 raw bytes and 6,889,892 gzip
bytes when `internal/mcp/server` stopped importing `internal/task/service`.
The agent-controller dependency graph then contained no Kubernetes packages.
The experiment changed no MCP protocol behavior and is size evidence, not an
implemented or behavior-tested change. Its measured gzip delta suggests an
initial standard Linux x64 archive in the low-to-mid 60 MiB range; the actual
release build and embedded web assets determine the final size.

## Shared MCP contract

`internal/mcp/server/server.go` and `handlers.go` currently import
`internal/task/service` only for task-title length, plan-write modes and their
parser, and plan-revision page limit. That service imports
`internal/agent/kubernetes`, which links Kubernetes into all `agentctl`
variants. Move those values and the exact mode parser into a small
`internal/task/contract` package. The task service re-exports type aliases,
constants, and a forwarding parser where existing service callers require
them; the MCP server imports the contract directly. Do not copy values into
two independently maintained packages.

Preserve `ParsePlanWriteMode`'s empty/default behavior, case sensitivity,
error wording, plan argument validation order, and task-title limits. Tests
compare the existing MCP tool schemas and invalid-plan responses before and
after extraction. A checked-in dependency-graph test must fail whenever
`go list -deps ./cmd/agentctl` contains `internal/task/service` or any
`k8s.io/` package. The native and all four CGO-disabled remote builds use the
same command package and contract.

## Artifact production

One required `build-remote-helpers` job in `.github/workflows/release.yml`
builds the four CGO-disabled Linux/macOS helpers for the exact release ref for
both Stable and Nightly. It uses the full commit SHA, the commit timestamp
instead of a matrix-job wall clock, and `-trimpath` so a retry or backfill
produces identical helper bytes. The host `kandev` and native `agentctl`
release stamps also use that full SHA so launcher validation can compare them
with the manifest. The current per-host matrix build timestamp
would otherwise produce different bytes for same-named helpers. It validates
executable formats and the Darwin arm64 Mach-O signature, emits compressed
helper assets and an integrity manifest, and uploads them through the
workflow's required-artifact retry pattern. All five `build-bundles` matrix
entries consume these canonical bytes; Nightly packages remain full and do
not publish or need an asset-fetch manifest.

The bundle-local `remote-helpers.json` at the archive root has a schema
version, Stable version and full commit SHA, bundle variant (`standard` or
`full`), and
exactly four enumerated helper records. Each record has the platform, fixed
asset basename, uncompressed SHA-256, and uncompressed size. The publisher
also provides SHA-256 sidecars for each public compressed asset. Runtime code
derives `https://github.com/kdlbs/kandev/releases/download/v<version>/<asset>`
from the manifest version and a validated asset basename; the manifest cannot
supply an arbitrary URL or filesystem path. The HTTP client follows GitHub's
asset-CDN redirect. A bounded size is checked before and during decompression.

| Output                                                     | Contents                                          | Consumer                                                       |
| ---------------------------------------------------------- | ------------------------------------------------- | -------------------------------------------------------------- |
| `kandev-<platform>.tar.gz` after cutover                   | Host binaries and `remote-helpers.json`           | Stable GitHub, npm, Homebrew tap, Scoop, Desktop build input   |
| `kandev-<platform>-full.tar.gz`                            | Same host binaries and manifest plus four helpers | Stable offline command-line download and container build input |
| `agentctl-<goos>-<goarch>.gz`                              | One canonical remote helper                       | First-use Stable resolver                                      |
| `kandev-windows-x64.zip` and `kandev-windows-x64-full.zip` | ZIP equivalents of the two Windows TAR layouts    | winget, Chocolatey, and offline download                       |

The implementation is staged so each merged state remains releasable. First
the runtime learns the manifest schema and can resolve a compact bundle from
test fixtures. The artifact producer then writes the compact layout under a
new staging name such as `kandev-<platform>-slim.tar.gz`, while the existing
`kandev-<platform>.tar.gz` and Windows ZIP remain complete and all current
consumers keep using them. Staged candidates remain workflow artifacts
outside the public release-assets directory. The final channel cutover
atomically gives the
default names to compact archives, publishes `-full` archives, changes Desktop
preparation to copy the standard manifest and host binaries, and changes both
Docker build paths to `-full` in the same workflow change.
No independently releasable merge may put a compact bundle under a default
name before the matching runtime and consumer switches are present.

`scripts/release/package-bundle.sh` gains explicit standard/full validation;
its default remains the complete layout for source builds and existing `make`
targets. The standard validator refuses bundled remote executables or missing
manifest entries. The full validator requires all four helpers, verifies their
manifest digests, and checks the Darwin arm64 signature. Build output
publication checks every required archive, checksum, and helper before a
GitHub Release is written. An existing release helper asset with the same
name must have the same digest; a retry or backfill refuses to overwrite
different bytes. The existing Stable SemVer and archive names for default
packages remain unchanged.

## Channel consumption

`publish-release` publishes standard/full archives, Windows ZIP variants, and
the four helper assets with sidecars. `publish-npm.sh` continues to package
the exact `kandev-<platform>.tar.gz` Stable archive; the npm runtime packager
(`scripts/release/package-npm-runtime.sh`) also copies the bundle-root manifest
beside `bin/`. Homebrew tap, Scoop, winget, and Chocolatey continue to use
their existing default archive or ZIP URLs and published checksums. The
Homebrew formula template in `scripts/release/kandev.rb` asserts that the
manifest exists for a compact install. Its foreign-binary audit allowlist is
retired in coordination with the first slim release.

The `build-desktop` job consumes the default standard archive. Its preparation
script copies the host `bin/kandev`, host `bin/agentctl`, and bundle-root
`remote-helpers.json` into the Tauri resource tree; it does not copy the four
remote helpers. The shell validator and Rust `validate_runtime_dir` accept a
manifest-bearing standard resource tree with both host binaries, and also
accept an older complete tree without a manifest. Neither accepts a partial
legacy tree or a standard tree missing its manifest. Rust passes the resource
root through `KANDEV_BUNDLE_DIR`, so the launcher and backend resolve the
same manifest. The release workflow signs the two host executables on macOS
and Windows and preserves the existing signed installer, updater artifact,
and `latest.json` track. There is no second full Desktop installer or updater.
The updater can replace an older complete app with the standard resource tree;
local startup remains independent of the helper download.

Both Docker build paths consume `-full` archives. Nightly continues to
publish full npm runtime archives under the existing names and without a fetch
manifest, because an npm-only Nightly has no corresponding GitHub Release
asset. Workflow contract tests protect these distinct paths and the all-target
publication gate.

## Runtime resolution

`apps/backend/internal/launcher/bundle.go` validates the selected layout at
startup. Bundle lookup uses `KANDEV_BUNDLE_DIR` first; otherwise it derives the
bundle root from the running `kandev` executable's parent `bin/` directory.
The launcher passes the validated bundle root to its backend child, which
uses that same value. A directly invoked backend uses the executable-path
fallback. Neither process searches the working directory or accepts a
manifest from an unrelated install. A valid `standard` manifest requires only
the native executables and
manifest; a valid `full` manifest additionally requires the four local
helpers. A legacy bundle with no manifest retains the current complete-bundle
validation. The installed manifest version and commit must match the running
`cmd/kandev` build information.

`AgentctlResolver.ResolveRemoteBinary` preserves existing explicit
`KANDEV_AGENTCTL_<OS>_<ARCH>_BINARY` and Linux legacy override precedence.
For a manifest-bearing Stable installation it then chooses, in order, a
matching validated bundled helper, a matching verified local cache entry,
or a download of that platform's asset. It never uses the host-native
`agentctl` as a missing remote helper: the native executable can have CGO and
different remote-host linkage. Legacy and development bundles without a
manifest keep their existing local search and same-platform fallback.

The helper cache lives under the resolved Kandev home, keyed by release
version, remote platform, and expected digest. This avoids writes into
read-only Homebrew, npm, Scoop, Desktop, or extracted release directories.
The launcher validates against its `BuildInfo`; backend construction passes
the same release identity and a request context into resolver calls. The
existing context-free resolver method and its SSH, Docker, Sprites, and
Kubernetes call sites need a bounded migration so cancellation reaches a
download. The downloader uses a maximum 90-second timeout or the remaining
launch deadline, whichever is shorter. Fetch time consumes the existing
launch-phase budget; it must not add time beyond that budget or be counted as
a remote upload/API request timeout. Tests cover the boundary for each
executor path. The downloader uses the backend's request context,
honors standard proxy settings, streams to a same-directory temporary file,
enforces size limits, verifies the digest, and publishes by atomic rename.
The executable has a mode suitable for container bind mounts. Concurrent
requests for the same version, platform, and digest share one in-process
transfer through `singleflight`; each waiting session gets its own progress
start and completion/failure events. Separate processes may still transfer
independently but must never observe a partial file. A cache entry is
revalidated before use.

Retain the current and two previous release versions' verified cache entries
for rollback. Older entries are candidates for cleanup only after executor
reconciliation positively confirms that no active or reconnectable managed
Docker container bind-mounts their path. An unavailable or incomplete
container inventory defers cleanup. Cleanup runs after successful startup
reconciliation or a verified cache insertion. It never deletes an entry while
an in-process resolver or launch is using it; immutable versioned paths avoid
replacing a mounted helper. A newer release cannot select an older entry.

The SSH, Docker, Sprites, and Kubernetes executor call sites continue to use
the resolver's path or bytes. SSH's existing remote SHA comparison still
avoids repeated remote uploads. No remote host must access GitHub Releases.

On a cache miss, the resolver emits a typed `remote_helper_download` prepare
step through each executor's existing progress callback. Add optional
`PrepareStep.Kind` and `RemotePlatform` wire fields; the step need not carry
an English `Name`. The web panel treats a typed step as visible even when
`Name` is empty, and translates running, completed, timeout, and failure copy
from `task:` keys in all six catalogs. A raw transport error may appear in
details, not as the primary label. The row uses the current inline
prepare-progress composition on desktop and phone; no new control, overlay,
navigation, or scroll owner is needed. A phone rendered check and
`mobile-chrome` Playwright scenario verify the same state as desktop.

## Integrity and failure

The trusted download identity comes from the installed archive manifest,
not a network response. Reject unknown platforms, unknown schema versions,
wrong version or commit, duplicate helper records, path traversal, digest
mismatch, truncated data, and over-limit data. Do not execute or upload a
failed candidate; remove its temporary file without replacing a valid cache
entry. The canonical producer verifies Darwin arm64 signing before publishing,
and the manifest hash binds the downloaded bytes to that validated asset.

An unavailable asset, TLS/proxy error, unwritable cache, timeout, or failed validation
returns an error for the selected remote prepare operation. The error names
the target platform and offers the full offline command-line archive or an existing explicit
path override as recovery. It must not log credentials or continue with a
stale or native-host helper. Launcher startup and local sessions stay
available. Structured logs distinguish cache hit, fetch, validation failure,
and remote selection without using task or session IDs as metric labels.

## Compatibility

Existing complete archives and source-built bundles have no manifest and
continue to resolve their bundled helpers, including older Desktop apps. The
full Stable command-line archive works offline, including on a first remote
launch. A Desktop update from a complete app to a standard app uses the same
updater track and may require a helper download on its first subsequent remote
launch; local startup does not. Nightly retains its complete local set.
Service upgrades select the new version's helper; an old cache entry never
matches it. No database migration or public API change is required. Public
installation and troubleshooting guidance must explain the standard/full
command-line choice, Desktop's first-use network dependency, and the absence
of a full offline Desktop installer. Firewall guidance
names `github.com` and GitHub's redirected Release asset CDN host
(`release-assets.githubusercontent.com` at design time); the client follows
the redirect and verifies the installed manifest digest regardless of host.

## Verification boundaries

- Unit tests cover contract parsing and exact MCP validation, launcher
  standard/full/legacy validation, resolver precedence, cache reuse,
  singleflight and cross-process writes, retention pins, checksum and size
  failures, version mismatch, deadlines, progress transitions, and
  interrupted downloads using a local HTTP server.
- Release contract tests inspect all five Stable base/full layouts, helper
  sidecars, Windows ZIPs, npm Stable/Nightly contents, Desktop standard
  resources, and container full-bundle inputs. Rust and shell tests cover
  standard/legacy Desktop validation, manifest copying, and missing-resource
  failures. Packaged Desktop smoke tests prove local launch without a fetch
  and remote helper selection from a verified cache. Release workflow checks
  cover host-only signing and the unchanged single updater track. A missing
  or mismatched asset blocks publication.
- A local HTTP-server-backed Go integration test proves a standard bundle
  acquires exactly one helper, restarts from cache offline, and reports a
  failed fetch. Selected Docker and SSH container-project E2E launches use a
  standard package with a preseeded verified cache and a full offline package
  to prove remote execution across both installed layouts. Their fixture must
  omit `KANDEV_AGENTCTL_LINUX_BINARY` and other helper-path overrides for
  these scenarios, then assert the manifest/cache path was selected; the
  Kubernetes helper override is likewise omitted in any Kubernetes variant.
  The release-size
  report records compressed standard and full archive sizes from the same
  build, rather than substituting the earlier prototype measurements.

## Related decisions and specifications

- [Slim Stable runtime decision](../../../decisions/2026-09-23-compact-runtime-and-remote-helper-assets.md)
- [Superseded Homebrew helper decision](../../../decisions/2026-08-05-homebrew-remote-helper-audit.md)
- [SSH executor design](../../executors/system-design/ssh-executor.md)
- [Artifact upload recovery](artifact-upload-recovery.md)

## Implementation plan

- [Compact runtime distribution](../../../plans/compact-runtime-distribution/plan.md)
