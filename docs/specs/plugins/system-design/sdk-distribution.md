---
status: current
system: plugins
requirements:
  - REQ-PLUGINS-SDK-DISTRIBUTION-001
  - REQ-PLUGINS-SDK-DISTRIBUTION-002
  - REQ-PLUGINS-SDK-DISTRIBUTION-003
  - REQ-PLUGINS-SDK-DISTRIBUTION-004
---

# Plugin SDK Distribution System Design

## Purpose and boundaries

The plugin system owns the public dependency boundary and compatibility record.
The [release system](../../release/README.md) provides Stable preparation,
signed tags, npm publication, and recovery. This design introduces no plugin
runtime behavior, Host authority, or UI composition changes.

The version policy is recorded in the
[SDK ADR](../../../decisions/2026-10-09-published-plugin-sdk.md).

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-PLUGINS-SDK-DISTRIBUTION-001` | Go module boundary; Frontend package; Clean consumers |
| `REQ-PLUGINS-SDK-DISTRIBUTION-002` | Release record; Consumer compatibility command |
| `REQ-PLUGINS-SDK-DISTRIBUTION-003` | Stable release integration; Failure and recovery |
| `REQ-PLUGINS-SDK-DISTRIBUTION-004` | Host integration and migration; Author guidance |

## Current evidence

Rechecked on 2026-10-16 against PR head `8548962279d8d0700ac44d7a4013ce1a71353329`
and upstream `main` at `76954ff53ea20b0263dd15c01c7149a531668341`.
Upstream changed CI allocation, caching, and E2E selection since the original
review. Its backend/E2E filters still omit `pluginsdk/**`. Release preparation
still excludes backfill from signing-key import. These observations support
the corrections below. They do not establish registry ownership or publication.

At planning time, `apps/backend/go.mod` declares `github.com/kandev/kandev`.
The Go SDK's production imports outside the standard library are go-plugin,
gRPC, protobuf, and the generated plugin protocol. Tests also use testify.
There are 161 host/fixture Go files importing the SDK outside its own package.

`apps/packages/plugin-sdk` is version `0.1.0`, exposes `src/index.ts`, has no
runtime or peer dependencies, and has standalone contract tests. The web host
binds its types in `apps/web/lib/plugins/types.ts`. `release.yml` currently
publishes only the CLI and runtime npm packages. The canonical external template
uses both a backend `replace` and a frontend `file:` dependency. These are source
observations, not the target public installation contract.

## Go module boundary

Move the existing SDK package and tests to root `pluginsdk/`, retaining package
name `pluginsdk`. Create a minimal `go.mod`/`go.sum`; begin with the current
supported Go toolchain rather than adding a separate toolchain downgrade.
Derive dependency versions from the extracted imports and `go mod tidy`, not
from a copy of the backend dependency list. Keep production SDK imports free of
backend packages. Test dependencies remain test-only.

Move the sole canonical `plugin.proto`, generated `.pb.go` files, and Buf
configuration to `pluginsdk/proto/`. Set `go_package` to
`github.com/kdlbs/kandev/pluginsdk/proto/kandev/plugin/v1;pluginv1` and regenerate
with the existing pinned tools. Keep `package kandev.plugin.v1`, service/method
names, message field numbers, defaults, and handshake unchanged. Go package
identity changes; the wire contract does not.

Give the SDK its own `Makefile` proto target, and preserve
`make -C apps/backend proto` as a delegation to it. Move existing generation
tests with the source. No old generated package or SDK alias facade remains.
The Go release tag is `pluginsdk/v0.1.0`, matching the module subdirectory.
[Go's module rules](https://go.dev/ref/mod#vcs-version) require that tag prefix.

## Host integration and migration

The backend keeps its existing module identity. Require the new SDK module and
use `replace github.com/kdlbs/kandev/pluginsdk => ../../pluginsdk` for same-tree
development. This replacement belongs to the backend main module only; the
SDK's published `go.mod` must contain no replacement. Do not introduce `go.work`
as a prerequisite for existing host commands.

Migrate all host, fixture, and SDK test imports atomically with extraction.
Root builds, targeted tests, and proto generation must work before that work
order completes. The local replacement deliberately follows current source so
an unpublished contract change can be tested before its first tag exists.

Build paths that currently copy only `apps/backend/` must retain the relative
`apps/backend` to `pluginsdk` layout. In particular, `e2e.Dockerfile` currently
flattens the backend into `/build`; change its Go stage to `/build/apps/backend`
and copy the SDK to `/build/pluginsdk` before module download. Audit build
contexts, sparse checkouts, scripts, and module enumeration: `go test ./...`
inside the backend no longer includes SDK tests. Add an explicit SDK CI step.
Preserve deprecation history when updating the moved SDK source path in
`config/architecture-lint/deprecation_ledger.json`.

Update the change-selection patterns in `.github/workflows/backend-tests.yml`
and `.github/workflows/e2e-tests.yml` for `pluginsdk/**` and
`scripts/pluginsdk/**`. Preserve each workflow's existing Markdown exclusions
and required-check reporting. An SDK-only source, proto, module, or tooling
change must schedule separate SDK race/vet tests, host compile/runtime checks,
and the E2E build/fixture path. A README-only SDK change follows the existing
documentation skip policy. Do not prove this with an extraction diff that also
changes backend files.

Add `.github/scripts/plugin-sdk-ci-contract_test.py` to exercise actual workflow
patterns through `changed-paths.py` and assert the selected jobs and commands.
Include both modules' `go.mod`/`go.sum` in dependency/cache identities. On an
updated base, also inspect `.github/actions/cache-go/`, `go-cache-identity.*`,
and `ci-test-only-changes.py`. Test that SDK production changes cannot select
an E2E test-only shortcut or reuse a stale host/fixture build.

A legacy binary fixture, built from the pre-extraction public SDK without new
imports, proves handshake and a Host state round trip against the new host.
Keep its legacy SDK snapshot bounded and test-only; it is not another
production protocol source. Record the snapshot revision. Existing
`TestServe_EndToEnd` is a same-process gRPC harness, so it alone is insufficient
evidence for this binary compatibility outcome.

## Frontend package

Keep the current runtime-free TypeScript source exports and npm package name.
Publish `src`, the release record, README, and license; add repository metadata
with the package directory so provenance identifies the monorepo source.
Expose `./sdk-release.json` for tooling without introducing a runtime dependency.
The current host receives types through its workspace dependency.

Generate the package's release-record copy from the canonical Go module record,
with a consistency check; do not maintain two editable policies. Add an isolated
tarball consumer test using an independent tsconfig and the packed artifact.
Retain the no-dependencies/no-bare-import tests, and verify both NodeNext and
bundler TypeScript resolution. No runtime code is promised by the type exports.

Define the publish file allowlist and exact Node/npm packaging versions in
`scripts/pluginsdk/pack-config.json`. Check the actual installed versions before
packing. The allowlist includes `package.json`, `src/**`, `sdk-release.json`,
README, and license. Exclude tests, workspace files, and application sources.
Add `pack-candidate.mjs` to pack once and return the archive path, file inventory,
and SHA-512 integrity. Packed-consumer tests accept that archive path and verify
its hash before and after installation. Release publication consumes those same
bytes. Selecting a floating Node major or npm range does not satisfy this pin.

## Release record

The proposed canonical file is `pluginsdk/sdk-release.json`. Versioned fields:

| Field | Meaning |
| --- | --- |
| `schema_version` | Record schema, initially `1` |
| `sdk_version` | Shared SDK version, initially `0.1.0`, without Go's `v` prefix |
| `host_release` | Exact originating Stable version for this SDK publication |
| `min_kandev_version` | SDK-wide minimum host, at most `host_release` |
| `api_versions` | Supported manifest integers; initially the actual `[1, 2]` |
| `change_kind` | `initial`, `patch`, `additive`, or `breaking` for this release |

For an unpublished SDK change, `host_release` may be null. A null host minimum
means the next actual Stable host release is required. These nulls are draft
state, never valid published metadata. Stable preparation fills `host_release`
from its computed release and fills a null minimum with that version in the
release PR. Compatible SDK fixes can retain an earlier non-null minimum.
Backfill reads the record from the existing signed tag and never restamps it.

New Go and npm artifacts for one SDK version contain identical records. They
originate from the same root Stable tag commit. A later host release reusing
that SDK references its original record; it does not change the record's
`host_release`. Publish the mapping in host release notes and artifact metadata.

PR/release checks compare the public contract inputs against the previous SDK
tag. Inputs include SDK production source and module declarations, protobuf
source, frontend SDK declarations, frontend Host public bindings, and manifest
API/capability version policy. Declare that inventory once in release tooling.
Changed inputs require an intentional shared SDK version and change kind;
tests, comments-only changes, and implementation-only fixes may conservatively
cause a compatible patch release. Do not infer a semantic change kind from a
text diff. Check the declared kind and version progression; semantic reviewers
judge whether the declaration matches the change.

API constants in `internal/plugins/manifest/manifest.go` remain the runtime
authority; consistency tests compare them with the release record. SDK SemVer,
manifest `api_version`, protobuf package `v1`, and go-plugin ProtocolVersion `1`
are separate axes. Extraction increments none of the last three.

## Consumer compatibility command

Ship a Go command at `pluginsdk/cmd/kandev-sdk-check`, usable as:

```bash
go run github.com/kdlbs/kandev/pluginsdk/cmd/kandev-sdk-check@v0.1.0 \
  --manifest manifest.yaml \
  --frontend-sdk node_modules/@kdlbs/plugin-sdk
```

The frontend flag is omitted for Go-only plugins. Support `--frontend-only` for
projects without a backend SDK dependency. Resolve the Go SDK via `go list -m
-json` in the consumer's directory with `GOWORK=off`; inspect the selected
module version and its record, not just the verifier's own version. For the
frontend, inspect the installed package manifest and its release record. In an
npm workspace, resolve the nearest installed SDK from the consumer directory
up to the selected lockfile root. Bind that exact location to its lock entry
and the consumer's dependency declaration; an explicit path cannot bypass a
closer installed package. The author installs and freezes dependencies before
running the check.

Reject missing SDKs, draft/null records, metadata/package version disagreement,
SDK pseudo-versions, Go replacements, and frontend packages resolved through
local workspace/file dependencies. Inspect npm `package-lock.json` or pnpm
`pnpm-lock.yaml` to establish registry resolution. Normal pnpm symlinks into
its managed dependency store are valid; a symlink alone is not evidence of a
local SDK substitution. Resolve the consumer directory before ancestor lockfile
discovery, including aliases directly to workspace members. Resolve checkout
aliases before comparing requested, installed package, and pnpm store paths. Keep the selected package's lexical
lock entry binding and reject links that substitute the installed SDK tree.
Allow lockfiles up to 32 MiB because they describe the entire workspace;
retain the 1 MiB limit for package manifests, plugin manifests, and SDK records.
Larger files fail with a diagnostic that names the limit and path.
Missing or unsupported lock evidence fails with a
specific diagnostic. Require a complete Stable host minimum without prerelease
or build suffixes, matching the runtime's Stable minimum policy. An optional
lowercase `v` prefix remains accepted. Require the minimum to be at least the
maximum of the consumed SDK floors and `api_version` to be in each relevant
supported set. Plugins may intentionally pin different releases from the shared
SDK train; both are checked. The checker is read-only and exits nonzero with a
specific mismatch; it does not rewrite a manifest or bump dependencies.

Use existing `gopkg.in/yaml.v3` for bounded manifest parsing in this command,
and a maintained SemVer parser already used by the repo for comparison. These
small checker dependencies belong to the SDK module, with no backend import;
the author runtime package still only imports its current transport dependencies.
Do not create a second full manifest validator. Runtime install validation and
capability/version gates remain authoritative in Kandev.

## SDK artifact identity

The SDK tag identifies the original SDK source, even when a later host reuses
its version. The canonical record identifies that source's originating host
release. Never pack a later host checkout under an existing SDK version.

For a new SDK, build one candidate from the verified signed host commit with
the exact packaging versions in `pack-config.json`. Test that archive. Retain
`plugin-sdk-<S>.tgz` and `plugin-sdk-<S>.receipt.json` as assets on the originating
host's GitHub Release before creating the SDK tag or uploading to npm. CI
artifacts alone are insufficient because their retention can expire.

The receipt records schema version, SDK version/tag, full source commit,
originating host tag, canonical record digest, archive SHA-512 integrity, file
inventory, exact Node/npm versions, and workflow/run identity. Sign the receipt
with the approved release signing identity and retain its detached signature.
Consumers of the receipt verify that signature, its source, and the archive
hash before reuse. A receipt records candidate identity, not publication success.

Reuse downloads the original archive and receipt from their originating host
release. It verifies the SDK tag, canonical record, and npm `dist.integrity`
against that identity. A README or tooling change in a later host tree may
remain unpublished while the original SDK is reused. To publish changed package
bytes, declare a new shared SDK version, even for a packaging-only fix.

Recovery uses the retained candidate. If the archive is lost, rebuild only from
its original signed source with the receipt's exact tools. Accept the rebuild
only when its integrity equals the authenticated receipt. Missing or conflicting
receipt evidence after any SDK publication stops recovery. Before any SDK
publication, an interrupted first candidate creation may restart from the
verified host source. Never overwrite a conflicting retained asset.

## Stable release integration

Extend `.github/workflows/release.yml` under its existing non-cancelling release
concurrency group. Keep the SDK separate from the runtime package array and
Nightly state machine. Normal Stable and latest-tag `backfill_tag` use the same
SDK preparation and publication operations.

1. Host `prepare` checks versions, contract inputs, registry reads, and confirmed
   scope ownership. It stamps pending SDK records in the release PR. Package
   absence does not require OIDC setup before a candidate can exist.
2. Host preparation signs only the root Stable tag. After `publish-release`, a
   protected `prepare-plugin-sdk` job verifies the root tag signature, approved
   fingerprint, and exact commit. Backfill reads that commit without restamping.
   SDK tag pushes use command-scoped Git credential helpers with `GH_TOKEN`;
   preparation does not persist a credential helper. Signing and OIDC
   publication retain separate protected jobs.
3. For a new SDK, `prepare-plugin-sdk` tests and retains the candidate and signed
   receipt. It checks all existing SDK identities before any SDK mutation.
   Then it creates the missing signed `pluginsdk/v<S>` at the verified host
   commit, or verifies and reuses a matching signed tag. Reused SDKs instead
   resolve their original host/tag/receipt and never retarget to the current host.
4. `publish-plugin-sdk` requires successful SDK preparation. It verifies the
   retained receipt and publishes that exact archive with public access and
   OIDC provenance, or verifies an existing npm version. Package absence with
   unavailable first-publication setup produces `bootstrap_required`, as below.
5. Public consumer checks verify both SDKs, their compatibility records, and
   artifact identities. Completion and contributor notices require this job's
   success plus the existing channels. Unchanged SDKs also run verification.

Keep signing keys and Git write permission in `prepare-plugin-sdk`, in the
protected `release` environment. The npm job uses the configured protected
environment with `contents: read` and
`id-token: write`, uses GitHub-hosted capacity, and receives no signing key.
Both jobs record workflow identity separately from checked-out source identity.
Use the approved public key/fingerprint, not a key supplied by an SDK artifact.
PR and dry-run validation receive neither signing nor publishing capabilities.

Dry-run performs metadata and read-only registry preflight. Desktop validation
and Nightly skip both SDK mutation jobs. Conditions must include status
functions and explicit successful dependency results, including backfill.
SDK failures leave sibling publication channels independent but block Stable
completion and contributor notices. Root-tag recovery remains unchanged.

### First npm publication

Record confirmed `@kdlbs` ownership before publication. Do not treat a
404 as proof of ownership. Distinguish a confirmed absent package from an absent
version of an existing package. Authentication errors, malformed responses,
timeouts, and other registry failures stop the job with a read/setup error.

If the package is absent and first-publication OIDC is unavailable, return
`bootstrap_required` only after the exact candidate and signed receipt are
retained. Report the asset links, SDK version, integrity, and required maintainer
action. This is an incomplete SDK result, not a successful or skipped release.
The maintainer publishes that exact archive with an authenticated account,
configures the trusted publisher, then reruns the SDK verification path.
The retry accepts the upload only if its bytes and metadata match the receipt.
Never publish a placeholder, rebuild a different local archive, or add a CI
token fallback. An existing package with a broken publisher configuration is a
setup failure, not a first-publication exception.

Document bootstrap provenance separately. A manual upload does not acquire
workflow provenance merely because a later job verifies it. Subsequent automated
uploads require OIDC provenance. Verified bootstrap artifacts can satisfy the
initial availability gate with their bootstrap origin explicitly recorded.

[Official npm guidance](https://docs.npmjs.com/trusted-publishers/) describes
setup in package settings and separate permissions for direct publication and
dist-tag operations. `scripts/pluginsdk/pack-config.json` pins Node 24.19.0
and npm 11.21.0. Release jobs load and enforce the original SDK's pins and
verify both required capabilities. Do not use `npm whoami` as OIDC proof. Verify
current CLI requirements during implementation and before the first release.

## Failure and recovery

There is no atomic transaction across Git tags, the Go proxy, and npm. Read both
SDK states and verify any existing identities before creating a missing one.
Before SDK mutation, require a verified host source and authenticated candidate receipt.
An absent SDK tag can be restored only at its original host commit.

| SDK tag | npm version | Recovery action |
| --- | --- | --- |
| Absent | Absent | Retain candidate, create signed tag, publish or report `bootstrap_required`, verify consumers |
| Matching | Absent | Keep tag, publish retained candidate or report `bootstrap_required`, verify consumers |
| Absent | Matching | Verify npm bytes/record first, create missing signed tag, verify consumers |
| Matching | Matching | Verify original artifacts and public consumers without republishing |
| Conflicting or unreadable | Any | Stop before SDK mutations and report the conflict/read failure |
| Any | Conflicting or unreadable | Stop before SDK mutations and report the conflict/read failure |

A matching tag has the expected peeled commit and approved signature. A matching
npm version has the receipt's integrity and canonical record. Recheck remote
identity after an uncertain write response before retrying. Never delete or
move tags, unpublish versions, or replace mismatched assets. Use bounded retries
for public proxy/registry visibility and report exhaustion as incomplete.

Repair missing/stale `latest` only after identity verification and only when it
does not downgrade a newer valid SDK. Require the configured OIDC dist-tag
permission. Failure to repair remains explicit. Backfill retains the existing
latest-host-tag restriction and never restamps compatibility records.

## Clean consumers

Both modes use `GOWORK=off`, `GOENV=off`, disposable module/build/npm caches,
and temporary projects outside the checkout. Do not alter global Go or npm
configuration. Reject conflicting trust inputs in public mode before network
access. Tests assert the effective child-process environment.

In local mode, serve the SDK archive through one test Go proxy with no client
fallback. The proxy serves SDK requests only from the fixture and forwards
non-SDK dependencies to the public proxy. Set
`GONOSUMDB=github.com/kdlbs/kandev/pluginsdk`, clear private/proxy exemptions,
and keep checksum verification for other modules. A missing SDK archive fails,
even if a public copy exists. This narrow exception follows
[Go's checksum and proxy rules](https://go.dev/ref/mod#authenticating).

Install the exact packed npm archive for typechecks. For the combined checker
flow, serve those same bytes through a disposable registry fixture so npm/pnpm
produce ordinary registry lock evidence. Do not add a checker bypass for `file:`
dependencies. Local mode proves package contents and compatibility only.

Public mode uses `GOPROXY=https://proxy.golang.org` and `GOSUMDB=sum.golang.org`.
Clear `GOPRIVATE`, `GONOPROXY`, `GONOSUMDB`, and `GOINSECURE`. Require the public
npm registry and reject inherited local registry or checksum-bypass settings.
Check effective configuration, including npm configuration files. Public mode
installs from public registries, never from a local archive or workspace
replacement. The retained archive supplies the authenticated comparison hash.

Both modes build/vet/test a plugin using `UnimplementedPlugin`, typecheck Host
types, and run the same compatibility assertions. Include too-low host minima,
unsupported API versions, missing local archives, and public trust contamination.
The public CLI uses `--mode public --host-tag vH`; it resolves the signed
source and receipt rather than trusting caller-provided metadata.
Release verification additionally compares installed records and npm integrity
with the original signed receipt. Record local packaging and public availability
as separate results. No browser scenario is added.

## Author guidance

Update `docs/public/plugins-authoring.md` with a short installation/migration
section and link detailed policy rather than duplicating the entire contract.
Update the release how-to for setup, partial recovery, and SDK completion gates.
Show exact pinning and replace the old Go import in live examples. Preserve
historical completed plan results as snapshots; update unresolved work-order
paths and live authority references that would otherwise point to removed code.

The canonical external template requires a subsequent repository change to
remove its sibling dependencies and SDK-ref checkout logic. Record exact target
files and validation commands as a handoff; changes to external plugin
repositories are excluded from this monorepo package. Host fixtures supply the
clean-consumer acceptance evidence independently of that follow-up.

## Related contracts and implementation plan

- [Plugin boundary](plugins-01.md).
- [Managed coordination compatibility](managed-coordination.md#independent-consumers-and-compatibility).
- [SDK publishing plan](../../../plans/plugin-sdk-publishing/plan.md).
