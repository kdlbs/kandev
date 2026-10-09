---
created: 2026-09-23
status: complete
requirements:
  - REQ-RELEASE-COMPACT-RUNTIME-001
  - REQ-RELEASE-COMPACT-RUNTIME-002
  - REQ-RELEASE-COMPACT-RUNTIME-003
system_design:
  - ../../specs/release/system-design/compact-runtime-distribution.md
legacy_specs: []
---

# Implementation Plan: Compact Runtime Distribution

## Overview

First remove the task-service dependency from `agentctl` and make the size
boundary a permanent CI check. Next teach the launcher and all remote
executors to accept a compact bundle, using manifest fixtures and a local
HTTP server. Then build canonical helpers and compact archives under staging
names while all existing default archives remain complete. Finally switch
the default Stable names, Desktop slim resources, and Docker full inputs together, update
channel documentation, and exercise packaged launches. Every intermediate
merge remains releasable.

## Scope

### In scope

- Preserve MCP task-plan/title behavior while excluding Kubernetes packages
  from all `agentctl` builds.
- Publish five compact Stable command-line archives, five full offline
  command-line archives, the corresponding Windows ZIP variants, four canonical
  helper assets, and integrity records. Package the standard runtime in the
  normal Desktop installer and updater track.
- Fetch and cache one exact, verified helper on first remote use from a
  standard Stable install; retain explicit overrides and complete bundles.
- Show localized first-download progress on desktop and phone, and safely
  remove old, unreferenced cache versions.
- Keep container images and npm Nightlies complete in this rollout.
- Update installation/release guidance and record measured final artifact
  sizes from one release-equivalent build.

### Out of scope

- Kubernetes clientset replacement in `kandev`.
- New remote platform support or MCP API changes.
- Slimming container images or Nightly packages; adding a second full offline
  Desktop installer or updater track.
- Adding an operator-configured helper mirror; restricted-network installs
  use the full offline command-line archive or an existing explicit helper-path
  override.

## Technical approach

### Shared MCP contract

Create `apps/backend/internal/task/contract` as the canonical home for
`TaskTitleMaxLength`, `PlanWriteMode`, `ParsePlanWriteMode`, and
`MaxPlanRevisionPageLimit`. Keep service aliases/forwarders for existing
callers; import the contract directly from `internal/mcp/server`. Preserve
schema limits, parser errors, and handler validation order. Confirm the
`agentctl` graph has no `internal/task/service` or `k8s.io/` imports through
a checked-in guard that runs in backend CI and the release workflow.

### Runtime selection and caching

Update `internal/launcher/bundle.go` to validate standard, full, and legacy
layouts against launcher `BuildInfo`. Find the manifest under the explicit
`KANDEV_BUNDLE_DIR` first, then the running executable's parent `bin/`
directory; pass the validated root into the backend child. Pass the backend's
build identity into `AgentctlResolver` and thread request context through SSH,
Docker, Sprites, and Kubernetes call sites. Update Desktop's Rust runtime
validator to accept a manifest-bearing standard resource tree with both host
executables as well as older complete trees without a manifest, before any
standard Desktop package is published. Keep Rust and Go startup validation
aligned. Extend the resolver to resolve
override, matching bundled helper, verified cache, then exact-platform fetch.
Cache under the resolved Kandev home by release/platform/digest, validate
before reuse, stream to a temporary file, verify size and SHA-256, and rename
atomically. Use `singleflight` for same-key calls and keep the current and two
prior versions plus any helper still mounted by an active or reconnectable
container. Bound fetch by 90 seconds and the remaining launch deadline.
Report a typed, localized download step through the existing preparation
panel. Keep local startup independent of the network. An invalid fetch
fails only the selected remote operation and never falls back to the native
host executable.

### Canonical remote assets and archive variants

Add one `build-remote-helpers` workflow job for Stable and Nightly. Stamp
helpers with the full commit SHA and commit time, build with `-trimpath`, and
stamp host binaries with the same full SHA. Refuse to overwrite a published
same-named helper with different bytes.
Generate the four compressed assets and a `remote-helpers.json` manifest.
Make `scripts/release/package-bundle.sh` validate standard/full layouts;
keep its source-build default full. Before cutover, keep the current complete
`kandev-<platform>.tar.gz` and Windows ZIP names. Emit compact candidates
under `-slim` staging names and full candidates under `-full`. No default
consumer changes in this work order; staged candidates stay outside the
public release-assets directory.

### Release channels and documentation

Give the existing Stable TAR and Windows ZIP names to the compact layouts in
the same workflow change that makes Desktop consume the standard archive and
both Docker architectures consume `-full` archives. Desktop preparation copies
the host executables and root manifest into its Tauri resources, signs the two
host executables, and publishes the existing single signed installer/updater
track. Stable npm, Homebrew tap, Scoop, winget, and
Chocolatey then consume the standard layout; `package-npm-runtime.sh` copies
the manifest. Keep Nightly packages full because they have no matching GitHub
Release. Update the formula-template test and coordinate tap audit-exception
removal. Flip the new ADR to accepted, mark the old ADR superseded, and update
the Homebrew criterion only when cutover actually ships. Update public
install/offline/first-use and firewall guidance, release instructions, and
same-build size evidence.

## ASCII UI preview

`UI-01: Remote helper download`, existing session preparation panel, first
remote launch on desktop and phone:

```text
Preparing environment
  [spinner] Downloading remote helper (linux/amd64)
  [pending] Starting remote agent
```

The row changes to a check or failure icon with a platform-specific recovery
message when the fetch ends. Desktop and phone use the same existing inline
prepare panel and state; the phone view does not add a drawer, control, or
scroll region. The nearest phone exemplar is the current session
`PrepareProgress` in the mobile task layout. The required structure is one
visible row within that panel; spacing and wording shown here are
illustrative. This covers `AC-RELEASE-COMPACT-RUNTIME-002.6`.

## Tests

| Acceptance criteria                | Evidence                                                                                                                                                                                                               |
| ---------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `001.3`                            | `internal/task/contract` parser tests; `internal/mcp/server` schema and invalid-argument tests; checked-in `go list -deps ./cmd/agentctl` guard in CI/release; size comparison                                         |
| `001.2`, `002.1`–`002.5`, `003.3`  | `internal/launcher/bundle_test.go` and `internal/agent/runtime/lifecycle/agentctl_resolver_test.go` cover standard/full/legacy, override, exact-target local HTTP fetch, cache, pinned retention, failure, and restart |
| `002.6`                            | `internal/agent/runtime/lifecycle` progress/deadline tests, `components/session/prepare-progress.test.tsx`, and desktop/mobile rendered checks                                                                         |
| `001.1`, `003.1`, `003.4`, `003.5` | `scripts/release/runtime-bundle.test.sh` and `.github/scripts/release-workflow-contract_test.py` inspect each staged and final archive, consumer path, and publication gate                                            |
| `003.2`                            | `scripts/release/publish-npm.test.mjs`, `scripts/release/nightly-release.test.mjs`, formula/ZIP/desktop tests, and workflow contract tests cover all channel inputs                                                    |
| `003.6`                            | Rust standard/legacy resource validation tests, `scripts/release-desktop.test.sh`, packaged Desktop local/remote smoke, and workflow checks for host signing and one updater track                                     |

## E2E tests

- `AC-RELEASE-COMPACT-RUNTIME-001.2`, `002.1`, `002.2`, `002.3`:
  local HTTP-server-backed resolver integration tests prove exact-platform
  first fetch, offline restart, and actionable fetch failure. In the
  `containers` project, `apps/web/e2e/tests/docker/docker-launch.spec.ts`
  starts from a standard packaged bundle with a preseeded verified cache to
  prove local startup and real remote execution without relying on a GitHub
  Release that does not yet exist. The fixture omits
  `KANDEV_AGENTCTL_LINUX_BINARY` and asserts cache selection.
- `AC-RELEASE-COMPACT-RUNTIME-002.1`, `003.1`:
  `apps/web/e2e/tests/ssh/launch-task.spec.ts` in the `containers` project
  launches with the matching helper from a standard bundle's verified cache
  and a full bundle; the full bundle succeeds without release-asset access.
  Cover another supported target architecture in resolver tests where the
  runner lacks that hardware. The Kubernetes fixture must also omit its
  helper override if it is used for this coverage.
- `AC-RELEASE-COMPACT-RUNTIME-002.6`:
  `apps/web/e2e/tests/session/mobile-remote-helper-progress.spec.ts` in the
  `mobile-chrome` project and
  `apps/web/e2e/tests/session/remote-helper-progress.spec.ts` in `chromium`
  render running and failed prepare steps and confirm the platform and
  recovery text remain visible.
- `AC-RELEASE-COMPACT-RUNTIME-003.6`:
  a packaged Desktop smoke launches from slim resources without a helper fetch,
  then resolves a remote helper from a preseeded verified cache without any
  helper-path override. A release contract test checks that the existing
  updater artifact and `latest.json` track remain singular and signed; Rust
  tests cover launch of an older complete resource tree without a manifest.

## Work orders

- [x] [Task 01: Extract the shared MCP contract](task-01-extract-mcp-contract.md)
- [x] [Task 02: Resolve and cache exact remote helpers](task-02-resolve-remote-helpers.md)
- [x] [Task 03: Build canonical helpers and staged archive variants](task-03-build-runtime-assets.md)
- [x] [Task 04: Wire release channels and packaged evidence](task-04-publish-and-prove.md)

## Verification results

Implementation completed on 2026-09-25. The release workflow contract suite
passed (39 tests), release Node tests passed (30 tests), runtime bundle tests
passed (8 tests), and Desktop release preparation and verification tests
passed. The mobile helper-progress E2E passed (2 tests), Docker passed (12
tests; external-container recovery remains a documented backend FIXME), and
SSH passed (9 tests). Go launcher/resolver tests, Desktop Rust tests, and the
packaged Desktop smoke passed. Public documentation validation, specification
catalog/lint, Prettier, and whitespace checks passed. Ruby syntax validation
could not run because Ruby is not installed; release workflow tests cover the
Homebrew formula contract. No release was published and no commit was created.

## Risks

- Release matrix jobs currently embed different build timestamps, so a
  same-named helper cannot be treated as canonical until the new producer is
  the only source.
- A missing manifest in npm or Desktop resources, or a channel accidentally
  consuming a standard archive before resolver and Desktop validation rollout,
  would break remote execution or Desktop startup.
- The prepublication browser suite cannot fetch assets from an unpublished
  GitHub Release, so local HTTP integration tests cover first-fetch behavior
  and packaged E2E tests cover real execution from cached and full layouts.
- macOS host-binary signing, the transition from complete to slim Desktop
  updater resources, read-only package-manager installs, proxy failures,
  cache cleanup, and launch-timeout interactions need explicit packaged checks.
- The Homebrew tap formula lives outside this repository; its obsolete audit
  exemption must be changed with the first compact Stable release.
