---
created: 2026-10-09
status: implemented
requirements:
  - REQ-PLUGINS-SDK-DISTRIBUTION-001
  - REQ-PLUGINS-SDK-DISTRIBUTION-002
  - REQ-PLUGINS-SDK-DISTRIBUTION-003
  - REQ-PLUGINS-SDK-DISTRIBUTION-004
system_design:
  - ../../specs/plugins/system-design/sdk-distribution.md
legacy_specs: []
---

# Implementation Plan: Versioned Plugin SDK Publishing

Start with the [complete proposal walkthrough](proposal-walkthrough.md) for
the five implementation steps and their relationship to issue #4358.
The dated [plugin author inventory](author-notification-inventory.md) identifies
existing consumers and notification candidates for work order 05.

## Reassessment checkpoint

The required amendments from the [design review](design-review.md) are now
reconciled into the canonical requirement/design and affected work orders.
The review remains evidence of the original gaps, not a second contract.

- R1: SDK signing runs after host publication on normal and backfill paths.
- R2: `bootstrap_required` retains the exact candidate and leaves publication incomplete.
- R3: Reuse checks the original archive, signed receipt, and source identity.
- R4: Local/public consumers have distinct, tested trust environments.
- R5: SDK-only fixtures prove backend/E2E selection and cache/build coverage.

The optional npm lockfile-policy simplification is not accepted. WO03 retains
strict npm/pnpm lock evidence. Shared version `0.1.0` and the SDK-wide host floor
remain accepted. The first host numbers and registry ownership/setup remain
release-time dependencies. External template adoption remains a separate handoff.
The design reconciliation changed documentation only. Implementation was
authorized on 2026-10-19. All five work orders now have implementation and
documentation changes. Local acceptance and implementation CI
passed. All six design-review threads have verified replies and resolutions.
Public publication and external template migration have not run.

## Overview

Deliver public Go and npm SDK packages for
[issue #4358](https://github.com/kdlbs/kandev/issues/4358), with pinned clean
consumer builds and an enforceable host baseline. Extract the Go module and
prove host continuity first, make the frontend artifact independently
installable second, add compatibility validation third, wire Stable publication
fourth, and finish author/release migration guidance fifth.

The user selected a shared independent SDK version, initially `0.1.0`, and an
SDK-wide minimum Kandev version. Requirements are active and the design is
current. Execution was sequential in the primary conversation, authorized on
2026-10-19 and completed on 2026-10-23.

## Scope

### In scope

- A root `pluginsdk` Go module, canonical protobuf, migrated host imports,
  matching build layout, and an explicit SDK CI test boundary.
- A public npm tarball with standalone types and compatibility metadata.
- One compatibility record, intentional shared versions, and an author CI
  command checking actual installed Go/npm SDKs against the manifest.
- Stable release preparation, signed SDK tags, npm OIDC publication, public
  consumer verification, and partial-publication recovery.
- Live contract references, public author/release documentation, and a concrete
  external template migration handoff.

### Out of scope

- Backend module rename, new RPCs or manifest fields, and source alias shims.
- SDK Nightlies, feature-specific minimums, or SDK `1.0.0` promotion.
- Rendered UI, new Playwright flows, or changed plugin permissions.
- Mutating external plugin repositories or registry configuration during this
  monorepo implementation package.
- Running a Stable release as part of implementation. Public availability is a
  later release completion gate, not a claim made from local packaging tests.

## Technical approach

### Go extraction and host continuity

Move `apps/backend/pkg/pluginsdk` to `pluginsdk`, and move
`apps/backend/proto` to `pluginsdk/proto`. Migrate imports atomically, regenerate
the `go_package` identity, and retain the actual gRPC wire and go-plugin
handshake. Introduce SDK module/Make targets; the backend proto target delegates.
Use a backend-only `../../pluginsdk` replacement so unreleased host development
works without a workspace file. SDK consumers and SDK module metadata have no
replacement.

Audit `e2e.Dockerfile`, the backend Makefile, CI paths, test fixture build
commands, `.dockerignore`, module traversal, and the deprecation ledger. Docker
must preserve the module-relative layout before `go mod download`. The root
SDK gets explicit Go unit/race/vet CI coverage. Update backend/E2E workflow
selection for SDK source/tooling, test single-path fixtures, and preserve
Markdown skips. Include both modules in cache identities and inspect upstream
E2E test-only selection so SDK changes cannot reuse stale host artifacts.
Build a minimal legacy binary
from the verified pre-extraction `v0.97.0` source in a disposable directory;
test it against the new Host in a real subprocess. Do not commit a binary or
turn an old source snapshot into a second production SDK.

### Packaging and compatibility

Keep npm source exports and its runtime-free dependency boundary. Add package
repository/license metadata and the exported release-record file. The canonical
record lives in `pluginsdk/sdk-release.json`; the npm copy is generated and
checked. Initial `host_release` and minimum remain pending until Stable
preparation resolves the actual first shipping release. Do not write `0.97.0`
as the initial SDK floor: current source includes changes beyond that release.

Add `pluginsdk/cmd/kandev-sdk-check` and an internal compatibility package.
Use the existing YAML and SemVer libraries rather than custom parsers. The
command verifies selected Go module metadata and installed npm metadata,
rejects unpublished/local evidence, compares the maximum host baseline, and
checks each API-version set. It is read-only and does not import backend
manifest validation. The host's manifest constants remain runtime authority.

### Release boundary

Add dedicated scripts under `scripts/release/` for SDK metadata/version
preflight and publication. Keep the existing runtime package arrays and Nightly
state machine separate. Declare one contract-input inventory; version updates
are intentional and their change kind is reviewed, not inferred from text.

Extend host preparation to stamp pending SDK records into the release PR and
sign only the root Stable tag. After `publish-release`, `prepare-plugin-sdk`
verifies the host signature/source, retains the exact tested candidate and signed
receipt, and creates or verifies the SDK tag at its original host commit.
`publish-plugin-sdk` then uses OIDC to publish those bytes or verify an existing
version. Both jobs run during normal Stable and latest-tag backfill. Signing
keys remain separate from the npm publishing identity.

Use WO02's exact packaging-tool pins and archive interface. Unchanged SDKs
verify original release assets instead of repacking the current host tree.
Recovery compares source, signatures, canonical records, and archive integrity.
Conflicts or unreadable state stop mutation. Confirm scope ownership before
release. If initial OIDC setup needs an existing package, retain the candidate
and return `bootstrap_required` with maintainer instructions. No placeholder or
CI token fallback is allowed. Contributor notices and completion require public
consumer success, including for reused SDKs.

## Compatibility matrix

| Consumer or mode | Target behavior | Evidence |
| --- | --- | --- |
| Backend and in-tree fixtures | One current SDK source via host-local replacement | Host compile, targeted plugin/automation tests, real subprocess test |
| External Go-only plugin | Exact public module, no replacement/workspace | Clean Go build, vet, tests, checker |
| External frontend-only plugin | Exact npm tarball, no application graph | Independent NodeNext/bundler typecheck, checker frontend-only mode |
| Combined plugin | Check both resolved baselines and API sets | Clean consumer and mixed-SDK mismatch cases |
| Supported legacy binary | Existing wire/Host behavior | Pre-extraction binary against extracted host |

Dry-run, desktop-only validation, Nightly, and backfill are covered in the
release work order's workflow-mode matrix. Unsupported/draft/ambiguous evidence
fails the author check rather than defaulting to the current SDK version.

## Tests

The test paths below are implemented outputs of the named work orders. Existing
SDK tests moved with their source. Use TDD for new logic and meaningful failure
cases; import-only migration relies on compile/runtime evidence.

| Acceptance criteria | Test boundary and evidence |
| --- | --- |
| `001.1`, `001.3` | WO01: `scripts/pluginsdk/consumer-smoke.test.mjs`, fresh Go archive/proxy consumer; narrow checksum exception; missing archive cannot fall back; graph excludes backend |
| `001.2`, `001.3` | WO02: `apps/packages/plugin-sdk/test/packed-consumer.node.mjs`, install the same hashed archive outside checkout, validate file allowlist/tool pins, typecheck both resolution modes |
| `002.1`, `002.3`, `002.4` | WO03/04: `pluginsdk/internal/compatibility/release_test.go` and `scripts/release/plugin-sdk.test.mjs`, version/floor/API consistency and prepare stamping |
| `002.2` | WO03: `pluginsdk/internal/compatibility/release_test.go` and `pluginsdk/cmd/kandev-sdk-check/main_test.go`, resolved versions, missing/low minima, unsupported API, malformed/null metadata, local replacements/links, mixed SDK floors |
| `003.1`, `003.2` | WO04: `scripts/release/publish-plugin-sdk.test.mjs`, original-artifact reuse, signed receipt/tag identities, missing-tag backfill, bootstrap, unreadable states, retry after uncertain response and partial completion |
| `003.3`, `003.4` | WO04: `.github/scripts/release-workflow-contract_test.py`, mode guards, downstream completion, contributor notices, provenance/OIDC/bootstrap boundaries |
| `004.1` | WO01: moved `serve_test.go`/`protocol_test.go`, existing runtime tests, new `sdk_binary_compatibility_test.go` with an actual legacy subprocess |
| `004.1` | WO01: `.github/scripts/plugin-sdk-ci-contract_test.py`, SDK-only source/proto/module/tooling selection, README skips, both-module cache identities, host/E2E build coverage |
| `004.2` | WO05: public/harness/spec validators, canonical example/import audit, template handoff and clean consumer commands |

Criterion IDs in this table abbreviate the common prefix
`AC-PLUGINS-SDK-DISTRIBUTION-`.

## End-to-end author workflow

Create `scripts/pluginsdk/consumer-smoke.mjs` to orchestrate disposable Go and
TypeScript consumers. `--mode local` installs an exact local Go module archive
through a test proxy and a packed npm tarball, with caches outside the checkout.
`--mode public` fetches the exact Go/npm SDK version from public infrastructure.
Both modes follow the design's explicit [trust environments](../../specs/plugins/system-design/sdk-distribution.md#clean-consumers).
Local Go has no SDK fallback and only an exact-module checksum exception.
Public mode rejects private/checksum exemptions and local registry/proxy inputs.
The local npm checker fixture serves the exact archive with registry lock
metadata. It does not weaken WO03's rejection of local dependency substitutions.
Both modes build/vet/test a plugin, typecheck representative frontend Host
types, and run the compatibility check once WO03 exists. Before WO03, pass
`--skip-compatibility-check` explicitly. Negative cases prove the local
replacement cannot accidentally hide incomplete artifacts.

Release verification runs public mode. Record fresh-cache paths, selected
versions, signed source revision, registry integrity, and mapping in job output.
No browser interaction or mobile composition changes; no ASCII UI preview or
new Playwright suite is applicable.

## Work orders

- [x] [Task 01: Extract Go SDK and preserve host execution](task-01-go-module.md)
- [x] [Task 02: Package standalone frontend SDK](task-02-frontend-package.md)
- [x] [Task 03: Enforce consumer compatibility](task-03-compatibility-check.md)
- [x] [Task 04: Publish SDKs through Stable releases](task-04-stable-publication.md)
- [x] [Task 05: Document author and maintainer migration](task-05-author-migration.md)

Dependency order: **01 -> 02 -> 03 -> 04 -> 05**. All work orders are sequential
because the module, canonical record, package metadata, and release contract are
shared. This sequence does not authorize subagents.

## Verification commands

Each work order contains its exact command block rooted at the repository root.
The principal checks are SDK Go race/vet, focused Host package tests and compile,
isolated packed npm typechecks, checker tests, mocked release tests, workflow
security contracts, and spec/public/harness validators. The scripts named in
those blocks are implemented by their owning work orders.

Use the repo's PR-documentation `validateCoverage` function against this package
to validate work-order frontmatter, requirement/AC ownership, plan backlinks,
and design coverage. Do not stage files merely to make a diff-based check see
new files; include untracked package files in the local adapter.

The paired requirement is `active`, the design is `current`, and this plan is
`implemented` after all five work orders passed their implementation gates.
Public package availability is recorded separately after authorized release.

## Verification results

Design-package validation on 2026-10-09:

- `python3 scripts/list-docs.py validate`: passed; 369 decisions and 1495
  specifications validated.
- `python3 scripts/lint-spec-files.test.py`: passed; 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Repository `validateCoverage` preflight: passed for all five work orders,
  using their actual files and a representative future `pluginsdk/serve.go`
  change to exercise coverage. No product file was created for this check.
- `git diff --check` and local artifact link/whitespace checks: passed. Files
  were unstaged at that checkpoint and later published in draft PR #4534.

Reconciliation validation on 2026-10-16:

- Catalog validation passed: 369 decisions and 1495 specifications.
- Specification linter tests passed: 36 tests. Full specification lint passed.
- `validateCoverage` passed for all five work orders using the actual documents
  and a synthetic `pluginsdk/serve.go` change. No product file was created.
- Local link validation passed: 43 links across 11 documents, including anchors.
- `git diff --check` passed. All changed files are documentation.


Evidence recheck: PR source `8548962279d8d0700ac44d7a4013ce1a71353329` and
upstream `main` at `76954ff53ea20b0263dd15c01c7149a531668341`. Upstream changed
CI/cache/test-selection helpers. WO01 includes those integration boundaries.
Official npm trusted-publisher and Go module guidance were rechecked. Registry
ownership and first-release availability were not established.

Product and release tests were not run in that planning turn, so all work-order
results were pending at that checkpoint. Later implementation validation is
recorded below. Public Go/npm availability has not been established.

## Risks and delivery dependencies

- npm `@kandev` ownership and bootstrap/trusted-publisher configuration require
  maintainer access; they cannot be inferred from `publishConfig.access`.
- Moving 161 host imports and the proto root requires one atomic migration.
  SDK tests fall outside backend `./...`; Docker copy layout and CI discovery
  must be updated with the move.
- Go tags and npm uploads can expose a partial release. Completion is gated
  on verified installs; recovery must not move immutable source identities.
- The initial SDK host baseline is unresolved by design until the actual Stable
  release. Package and checker tests use resolved temporary test records; public
  publication rejects pending values.
- External template/Forgejo migrations are separate repository work. This
  package provides the public dependency contract and a handoff, not changes to
  those repositories.

## Validation and review cleanup

The final implementation CI exposed four SDK validation issues: the shared
lint configuration still matched the old module path; an npm configuration
test inherited CI's Go flags; runtime shards lacked the legacy release source;
and a historical work-order path note selected unsupported legacy frontmatter.
The fixes preserve the narrow adapter lint exemption and API v1 labels mapping,
isolate only the npm test's Go inputs, fetch release history for runtime tests,
and leave the historical result unchanged. No publication contract is relaxed.
Two upstream housekeeping tests now import the extracted SDK module.

The branch incorporates current main, including its fix for fake process IDs
colliding with the test process in the orphan-reap test. Six review threads
refer to amendments reconciled before implementation. Their current contract
and test evidence was verified, and every thread received a reply and resolution.

Local remediation checks and exact-head CI are tracked separately. All five
work orders passed their implementation and review gates. Registry
configuration, first publication, external template changes, and author notices
remain separate release or repository handoffs.

Focused cleanup validation:

- SDK race tests, vet, and lint passed. The imported housekeeping tests passed
  with the race detector. The real legacy subprocess and the
  upstream orphan-reap regression each passed three runs with the race detector.
- Fresh-cache Go/npm consumer passed, including Go-only, frontend-only, combined,
  and insufficient-host-floor compatibility checks, plus both TypeScript modes.
- All 43 release/packaging/trust tests passed with the pinned npm tool and GPG
  available. The signature case was rerun after correcting the local tool PATH.
- Consumer/proto helper tests: 11 passed. SDK CI contracts: 10 passed. Backend
  workflow contracts: 25 passed. Release workflow contracts: 68 passed.
- Documentation coverage passed for exactly the five SDK work orders after the
  historical-only edit was removed. Catalog: 401 decisions and 1724 specs.
  Specification and harness checks passed; 204 harness files validated.

Implementation CI passed at `f4fd54c00be5fc1d1d915f16555fe8134952f8e9` against
main `144e9e503f82001bb4d1440b603f7c33b932dd0a`: 92 successful checks, 16 skipped,
zero failures, and zero pending checks. The waiter returned success after two
matching terminal snapshots. All six review threads were resolved; a fresh
thread list was empty. The worktree was clean and local/remote heads matched.
The backend workflow, including static checks, all test shards, packed SDK
consumer, and Docker builder, passed in run `39991576328`.

Public-doc validation passed 63 tests and all 54 pages. Normal pre-commit and
commit-message hooks passed without bypass. A shared lint-lock conflict was
resolved by serializing the retry, and lint used the verified current base.
The requirement/design/work-order lifecycle is now complete for implementation.
Subsequent documentation-only commits must pass their own exact-head checks.
No release was dispatched and no SDK was published.
