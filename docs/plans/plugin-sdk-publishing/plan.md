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

## Adversarial review cleanup

The user requested a secondary-session adversarial review on 2026-10-24, with
plugin compatibility as the priority. The [review record](adversarial-review.md)
documents three verified implementation bugs and their corrections: a dropped
packed-record digest, host minimum suffixes accepted by the checker but rejected
by Kandev, and valid npm workspace hoisting rejected by the checker.

Each fix has a regression that failed before the production change. The release
test crosses the real packer/adapter/consumer boundary. The workspace test uses
an actual npm install and checks the exact resolved lock entry, including shadow
and local-substitution failures. The accepted versioning and strict lockfile
policies remain unchanged. Historical green CI above does not validate this
remediation; its pushed head requires fresh checks.

Post-fix local validation passed:

- `(cd pluginsdk && go test -trimpath -race ./... && go vet ./...)`.
- SDK `golangci-lint` with the backend configuration: zero issues.
- The required SDK packaging command from `backend-tests.yml`: 45 tests,
  including the real release adapter, npm workspace, and GPG signature tests.
  Node/npm used the exact packaging pins; GPG was supplied through Nix.
- SDK CI contracts: 10 tests. Backend workflow contracts: 25 tests. Release
  workflow contracts: 68 tests. The release contract check first lacked GPG on
  the local PATH; it passed after supplying that tool, without source changes.
- Public-doc validators: 63 tests and 54 pages. Catalog: 401 decisions and
  1724 specifications. Full specification lint and whitespace checks passed.

The secondary reviewer rechecked the working-tree fixes and found no remaining
blockers. Their independent Go checker tests, real npm workspace regression,
and whitespace check passed. Current upstream changes were limited to shared
workflow contracts and unrelated features; final merge-result validation is
recorded with the pushed head and live base in PR delivery evidence.

### Browser validation follow-up

The SDK remediation at `5e6310d4` passed backend, frontend, and SDK CI. E2E
run `40129872943` remained failed after one failed-job rerun: the artifact audit
found two retry-pass failures and 40 skipped tests. The
[review record](adversarial-review.md#browser-validation-follow-up) records the
three test defects found during investigation and the corrections.

Local browser checks use the PR merged with upstream
`cc687bdce96bc22fb8a9ca87abe571b8309d3125`, with fresh runtime/web/fixture builds,
one worker, `CI=true`, strict WebSocket assertions, `GOMAXPROCS=2`, and
`GOMEMLIMIT=3GiB`. Retries are disabled. The temporary wrong-branch stimulus
failed before the chat-fixture correction and passed afterward; it is not part
of the permanent test source. Four workflow lifecycle variants each passed
three runs (12 tests) after excluding startup-only turns. ESLint, Prettier, and
the E2E sleep ratchet passed for the edited tests.

The PR-watcher cleanup case followed by the chat case passed three runs in the
same worker per repeat (six tests), with retries disabled. This checks the real
preceding branch change as well as the cleanup-message readiness correction.
The secondary reviewer inspected all three test corrections and found no
remaining issue. Fresh CI on the pushed follow-up remains required before
reporting validation complete.

### Container validation follow-up

CI on `0e029887a1737118597d59a833bf6c62efd583b6` passed backend and frontend
checks but failed two container cases on their first attempts. The SSH host-key
rotation test must wait for a successful probe with the new fingerprint after
sshd reload. The Kubernetes foreign-UID case passed task creation and UID checks,
then its archive response failed after 30 seconds. The archive work budget is
two minutes, but the server write timeout is 30 seconds.

PR cleanup work order: first reproduce the archive response loss with the real
HTTP handler and a short server write timeout. Then align only this route's
write deadline with its existing work deadline plus five seconds. Preserve
earlier caller deadlines, identity, cancellation handling, and foreign-resource
checks. Run the handler and archive-context tests with the race detector,
changed-file lint, and specification checks. Commit the archive correction with
the SSH readiness correction, then require fresh container CI. Local Docker is
unavailable. The exact shutdown step that occupied the failed CI request remains
unknown; the response deadline mismatch is independently testable.

The real-server regression failed with EOF in both cases before the correction.
Afterward, focused archive tests passed. Race validation passed for archive
response delivery, the finite five-second write allowance, earlier caller
boundaries, request cancellation, authorization, and success/pending outcomes.
The archive-context package also passed with the race detector. The broad handler
suite hit its ten-minute total limit without an assertion failure or race report;
the SQLite fixture active at timeout passed separately in 5.31 seconds. This
broad local result remains incomplete, pending CI. SSH ESLint, formatting, and
sleep-ratchet checks passed, as did catalog and specification validation.

Scoped Go lint passed with zero issues. The secondary reviewer found no blocking
issue in the archive or SSH correction. A finite response-deadline assertion
addresses their additional coverage suggestion. Container execution remains a
CI requirement because this workspace has no Docker daemon.

### Browser fixture validation after container fixes

The published archive/SSH correction passed backend and frontend CI and all six
container shards. The browser aggregate still failed on a retry-pass storage
assertion. Five unchanged local runs passed. The single failed-job rerun passed
that assertion but exposed an MCP saved-report fixture failure at 320px.

PR cleanup work order: integrate the current base containing the failing helper,
retain its mock route across report cases, and await the intended response before
UI assertions. Preserve all desktop and mobile assertions. The unchanged local
baseline reproduced the same `deferred_restart` versus `applied` failure with
retries disabled. Validate the correction at 320px, 393px, 767px, desktop, and
tablet widths, plus helper lint, formatting, and the sleep ratchet. Check the
auto-merged workflow contracts and documentation. Commit through normal hooks,
push to the same draft PR, and require fresh CI and review evidence.

The corrected helper passed nine mobile runs (320px, 393px, and 767px, three
runs each) with retries disabled. The command was
`pnpm e2e:raw --project=mobile-chrome e2e/tests/chat/mobile-mcp-status.spec.ts --grep 'contains multiline MCP connection rows' --repeat-each=3 --retries=0 --trace=on`.
The isolated checkout used the current base's MCP implementation and the exact
edited helper. It did not include the later archive-response binary correction;
fresh current-head CI remains required for the full integrated result.

Helper ESLint, Prettier, and sleep-ratchet checks passed. Workflow contracts
passed: 10 SDK selection tests, 27 E2E workflow tests, and 68 release tests
(with GPG available). Catalog validation passed for 404 decisions and 1734
specifications; all specification files passed. The secondary reviewer found no
blocking issue in the final persistent-route and response-header correction.

Desktop and tablet checks also passed with retries disabled (two tests):
`pnpm e2e:raw --project=chromium e2e/tests/chat/mcp-status.spec.ts --grep 'explains mixed connections|tablet MCP connections' --retries=0 --trace=on`.
The final helper therefore passed 11 focused browser tests. Fresh CI and review
on the delivered merge commit remain pending.

### Static-check job completion budget

The MCP correction passed full E2E CI: 4239 first-attempt passes, zero retries
or failures, and 40 skips. Backend static validation also passed every command,
but its newly integrated 20-minute job budget cancelled post-job cache
compression. The checks consumed about 18 minutes before cleanup began.

PR cleanup work order: set a finite 30-minute total static-check budget while
keeping the 15-minute lint limit and every validation command. Run backend/SDK
workflow contracts and workflow lint, preserve the failed job evidence, and
push through normal hooks. Require fresh backend and aggregate CI on that head.
This changes CI scheduling capacity only; public behavior and SDK contracts do
not change.

Validation passed: `node --test scripts/backend-lint-workflow.test.mjs` (two
tests), `python3 .github/scripts/backend-tests-workflow-contract_test.py`
(25 tests), `python3 .github/scripts/plugin-sdk-ci-contract_test.py` (10 tests),
and `actionlint -shellcheck= .github/workflows/backend-tests.yml`. Catalog and
specification checks also passed. The secondary reviewer found no blocking
issue. The correction adds no test bypass and changes no test assertion.

### Browser synchronization follow-up

The static-check budget correction passed backend CI. Three retry-pass browser
cases kept E2E CI failed: mobile deep-tree scroll setup, Quick Chat reset
confirmation, and desktop workflow button selection. The adversarial review
record contains the exact distinction between CI and local failure evidence.

PR cleanup work order: establish the virtualized scroll precondition with the
existing bounded scrolling convention; await a real completed opening turn
before Quick Chat reset interactions; scope workflow buttons to the active chat
through `SessionPage`. Preserve all behavioral assertions and default locator
budgets. Run repeated mobile regressions, the desktop workflow scenario, existing
Quick Chat activity tests using the strengthened helper, and changed-file lint,
format, sleep-ratchet, and documentation checks. Deliver one reviewed correction
through normal hooks and require fresh CI. No production UI change is planned.

The existing session-helper unit case now includes a completed lifecycle-only
startup record before an unfinished conversational turn. It failed with the
old predicate because readiness returned at the startup record. All four helper
tests pass with the corrected predicate. Three deep-tree runs, three Quick Chat
reset runs, and two desktop workflow runs passed with retries disabled.

The two existing Quick Chat activity tests also passed with retries disabled,
covering desktop and tablet callers of the strengthened helper. The final
focused browser total is ten passes. Changed-file ESLint, Prettier, the E2E
sleep ratchet, catalog validation (404 decisions and 1734 specifications), and
specification lint passed. Fresh CI on the delivered commit remains required.

### Ready-for-review merge update

The user requested ready-for-review status and conflict resolution after
`main` advanced to `62af870aea3556e34ae3878041d2b99bf1c522bc`.
The MCP helper keeps the base branch's named route and targeted cleanup while
retaining this PR's response-provenance header and readiness wait. The
multi-file chat test adopts the base branch's isolated repository, which starts
on `main` and publishes its fixture commit before task creation. This preserves
the original branch/remote visibility correction without shared seed mutations.
The mobile deep-tree setup fix is identical on both branches.

Run the affected desktop/tablet chat and MCP tests, mobile MCP report cases,
changed-file lint/format, and documentation validators. Commit through normal
hooks, push to the same PR, and require new CI before reporting merge readiness.
The PR is ready for review, not merged. SDK publication, external template
migration, and author notices remain separate release work.

Merged-result validation passed: three desktop/tablet tests (MCP state reports
and multi-file chat links) and three mobile MCP report tests (320, 393, and
767 pixels), all with retries disabled. The managed runner rebuilt backend,
frontend, and plugin fixtures from the merged tree. Changed-file ESLint,
Prettier, sleep-ratchet, catalog validation (404 decisions, 1738 specifications),
and full specification lint passed. Fresh CI remains required on the merge
commit, which replaces the previous head's evidence for readiness.


### Workspace file and path review corrections

Automated review of the ready-for-review merge found two author checker defects:
whole-workspace locks inherited the 1 MiB metadata limit, and checkout-directory
symlinks were mistaken for local SDK substitutions. New npm and pnpm regressions
failed on both cases before correction. Lockfiles now have a separate 32 MiB
limit; manifests and SDK records retain the 1 MiB limit. Path verification
resolves the owning project and lock root before comparing installed locations,
while preserving exact lock-entry binding and SDK substitution rejection.

Validation passed: `cd pluginsdk && go test -race ./... -count=1 && go vet ./...`
and `node --test scripts/pluginsdk/npm-workspace.test.mjs` with the pinned
Node/npm tools. Regressions cover both package managers, oversized locks,
metadata limits, relative and absolute paths through checkout aliases, and
linked SDK rejection. The real npm workspace integration retains its shadow,
explicit-path bypass, lock-version, and lock-root negative cases. Public author
guidance and the canonical requirement/design describe the corrected boundaries.
SDK lint passed with zero issues. Documentation catalog and specification
validation passed. Fresh CI and review disposition are required after delivery.

The base advanced once more to `f33e6ef287d53c93b19f95c7054feb7080ec77b0`
(question-image preview cleanup). It merged without conflicts. The two affected
frontend unit files passed all 21 tests in the merged tree. SDK race tests,
vet, and the real npm workspace integration also passed after integration.

The first delivery's CI passed SDK tests but reported `goconst` for repeated
package-manager kind strings. The resolver now shares constants for both kinds;
this changes no resolution policy. SDK race tests, vet, and full SDK lint
passed with CI's `GOFLAGS=-trimpath`. The documentation preview's remote
demo download failed with a closed TLS socket and recovered on its failed-job
rerun. Fresh CI remains required for the lint correction.


The secondary review also reproduced aliases directly to workspace members
missing ancestor lockfiles. New npm/pnpm member-alias regressions failed before
the fix. Directory canonicalization now precedes lock discovery; requested-path
comparison and installed-tree verification share canonical project paths without
resolving away SDK substitutions. SDK race tests, vet, full lint with CI flags,
and the real npm workspace CLI integration pass. The integration also exercises
relative and absolute package arguments from an aliased member checkout.


### Final browser fixture corrections

CI run `40232279840` passed backend and SDK checks but failed three browser
cases that passed only on retry. Mobile runtime controls reported a rendered
height of `43.999969482421875` for a 44 px control. The test now checks the
44 px CSS constraint and rounds rendered geometry, matching the existing
same-file convention. Three corrected runs passed with retries disabled.

The Stats recovery fixture allowed success after its first failed read. A
controlled foreground refresh reproduced disappearance of its error state
before manual recovery. A second probe showed that enabling success immediately
before a Playwright action still allowed a refresh before click dispatch.
The desktop and mobile fixtures now keep the outage active until the actual
Retry click. A one-shot capturing listener marks that event, and the route
reads the document-owned marker. The same controlled probe passed. Production
retry behavior and all original UI assertions remain unchanged.

The workflow-change dialog timeout remains unproven. Its failed attempt has no
retained trace; the retry trace cannot establish the failed attempt's cause.
Three unchanged local runs passed with retries disabled and traces enabled.
No timeout, click retry, or production change is justified by this evidence.
Fresh CI must validate this unchanged case along with the corrected fixtures.

Final validation passed with retries disabled: three mobile Stats cases,
three desktop Stats cases, three mobile runtime cases, and three unchanged
workflow-dialog cases. Changed-file ESLint, Prettier, the E2E sleep ratchet,
catalog validation (404 decisions and 1738 specifications), and specification
lint passed. The secondary reviewer found no remaining actionable issue in
the corrected fixtures. The PR remains open and ready for review; merge
readiness requires new terminal CI.


### Discord integration and current CI cleanup

The current base adds automatic Discord release announcements. Preserve both
public release-guide sections and gate automatic delivery and its recovery
notice on public SDK verification and `stable-complete`, matching the accepted
Stable completion boundary. Extend both workflow contract suites; reconcile the
Discord requirement/design and release instructions. Contract tests reproduced
the missing SDK gate before the workflow correction.

Current CI run `40234952605` failed the mobile rename ownership, active-task
Go-cache cleanup, and PR-watcher plan-mode cases. Inspect first-attempt artifacts,
reproduce their failed scope with retries disabled, and preserve behavioral
assertions. Run affected workflow contracts, documentation validation, focused
browser regressions, normal hooks, and new CI after delivery. Keep the separately
requested root `NOTIFICATION_GUIDE.md` uncommitted.


The PR-watcher fixture reused one checkout for three different PR branches.
A new environment-path assertion failed with one path instead of three.
Selecting worktree execution exposed another deterministic fixture failure:
PR head refs were absent from the offline origin. The test now owns a disposable
repository and publishes both branch and `refs/pull/N/head` refs before starting
the watch. All original layout assertions remain, alongside workspace isolation.
Owned watch/tasks/workflow/repository and changed preferences are cleaned up;
independent cleanup failures are aggregated with the original test error.

The unchanged Go-cache case passed three focused runs; a temporary 1.2-second
opening-animation probe also passed and was removed. The unchanged rename case
passed three runs. Their CI first attempts have no retained traces, and neither
local result establishes the original failure cause. No speculative timeout,
click retry, or product change was made for those cases. Fresh CI must cover them.

The Discord gate regression failed before the correction. After correction,
68 release contract tests, 4 Discord workflow tests, 16 delivery tests,
8 content tests, 4 contributor workflow tests, and 10 SDK CI contract tests pass.
Harness validation passed (19 tests and 204 files); specification validation
passed (36 validator tests, 404 decisions, 1740 specifications). Changed-file
ESLint, Prettier, and the E2E sleep ratchet pass. Local actionlint 1.7.12 rejects
only inherited `concurrency.queue` keys; it cannot validate that newer grammar.
The workflow contract tests retain and verify the upstream queue contract.


Final cleanup-aware validation passed twice with retries disabled. Each run
created and exercised all three watch-owned PR tasks, verified three distinct
workspace paths, and completed owned cleanup. The secondary reviewer found no
remaining actionable issue in the fixture or release integration. Documentation
validators and the guide checksum pass. Commit/push through normal hooks and
require current-head CI; the root notification guide remains uncommitted.


### Historical changed-files CI follow-up

New-head CI failed mobile historical-diff scenarios. Actual failed traces show
an initial pending history read before session subscription, then conversation
backfill containing the completed turn with no later history read. The exact
server notification emission is not retained, so its timing is not claimed.
A controlled hook regression reproduces the stale pending snapshot when the
terminal event falls outside the subscription. A second case keeps the initial
HTTP response pending across acknowledgment to prevent a deduplication bypass.
Use the existing subscription-ready lease, refresh after acknowledgment and
reconnect, preserve the independent initial HTTP read, and cancel obsolete
refreshes. Reconcile the owning Tasks requirement/design and transcript work
order. Verify unit ownership/error boundaries, fresh desktop/mobile E2E, normal
hooks, and new CI. Keep the root notification guide uncommitted.

Validation: the pending-snapshot and overlapping-read regressions failed before
the handoff fix. Review exposed a late error from an obsolete subscribed refresh;
its controlled session-switch regression also failed before correction.
The final focused unit run passes 12 tests. Typecheck, changed-file ESLint,
Prettier, catalog/specification validation, and diff checks pass. Fresh desktop
and mobile builds pass all seven historical-diff cases with retries disabled.
The secondary reviewer found no remaining actionable issue in the final diff.
The complete previous CI artifact audit contains 118 reports and 4,294 attempts;
all five failed attempts belong to the two affected mobile changed-files cases.
A new committed head still requires full CI. Public docs need no new command or
UI composition changes for this freshness correction.

### Delayed session hydration CI correction

The next exact-head CI run exposed a stale session-state projection in the MCP
subtask case. Its failed retry trace shows the parent idle, followed by an
optional active-session timeout and a regression to STARTING, without a newer
backend start. The fallback session list was older than the live settled row.
Keep the idle assertion and correct hydration freshness through the existing
UI session-refresh contract and its progressive-navigation work order.
Canonical rows, per-task copies, and settled-turn boundaries must agree.

The same terminal CI run caught a regression in the newly added changed-files
readiness lease: hidden mounted transcripts retained rich streams. The original
changed-files cases passed; the visible-demand test failed four attempts.
Scope the lease with existing `detailActive` and group visibility, including
persistent historical diff viewers. Preserve eager HTTP history loading and
all existing browser assertions. The owning Journey data-loading design and
visible-demand work order record the correction.

Validation for both corrections: 259 focused unit cases across 15 files passed;
after lint cleanup, the 127 affected ownership/native-transcript cases and six
typed hydration fixtures passed again. Full typecheck and changed-file ESLint
with zero warnings pass. The independent reviewer found no remaining actionable
issue. The unchanged full subtask file plus desktop progressive navigation
passed 19 cases against a fresh build. Desktop historical-diff/visible-demand
validation passed five cases, then the final eight-tab ownership case passed
after rebuilding the final source. Seven phone cases passed: visible demand,
historical diffs, and progressive navigation. All browser runs disable retries.
The prior CI artifact audit contains 118 reports and 4,295 attempts: four
visible-demand failures and two subtask failures, with no other error results.
Main advanced to e5c38184cf9 without file conflicts. New-head CI and focused
synthetic-merge validation remain delivery gates; keep the guide uncommitted.

### Final test fixture and completion-boundary cleanup

The next full CI run has 118 reports and 4,304 attempts. Its four failed
attempts belong to menu preselection, policy-label geometry, SSH save
persistence, and phone Threads swiping. Previous session hydration and
visible-stream corrections no longer fail. These follow-ups change tests only.

The preselection case incorrectly equated the first command row with the
persisted branch choice. Policies intentionally precede branches. An owned
policy reproduces the original failure locally; the corrected case checks
`main` by its exact value and retains selected-surface and keyboard assertions.
The settings fixture now removes only the policies it creates, preserving
baseline IDs even when an assertion fails.

The geometry helper sampled nullable boxes immediately after a text-only check.
A controlled hidden-to-visible browser probe fails before the correction. Poll
only read-only geometry with the existing assertion budget; null boxes cannot
pass, the three-pixel alignment bound remains, and misaligned labels still
fail. The temporary probe was removed after red/green validation. The original
failed CI attempt has no trace, so exact original render timing is not claimed.

The SSH case waited for the accessible Save button name to disappear, which
also happens when its label becomes Saving. Wait for the existing explicit
saved status before the single persisted API read and reload assertion. Source
ordering and the later renamed/Saved failure screenshot establish the invalid
completion gate; no failed HTTP trace is available.

The phone Threads case assumed a native flick always stops on its immediate
neighbor. Its failed snapshot shows one chat on page 3 instead of the expected
page 2. Await native scroll completion, require motion in the intended
direction, and bind the sole chat/indicator to the geometrically settled
column. Preserve snap alignment, layout, touch controls, and tablet-to-phone
chat ownership. Do not change production snapping or add gesture retries.

Validation: 32 selector/pill unit cases pass. Seven desktop cases pass,
including five real fixture flows and the two temporary geometry probes.
The Threads case passes three stress repetitions, and all nine final phone
Threads/branch-policy cases pass with retries disabled. Changed-file lint has
zero warnings. SSH persistence passes three cases; its first fingerprint case
was skipped by the Docker availability fixture; a separate attempt was also
skipped. The affected save case is verified by its passing trace. Container CI
remains authoritative for fingerprint coverage.
Main advanced to 94ec7bfc99d and introduced one mobile runtime test conflict.
Merge both sides: retain the explicit 44px CSS invariant and rounded touch
geometry, and use the incoming settled-box helper before measuring. The MCP
helper merge preserves the persistent report route and new neutral evidence
assertions. The actual merged worktree passes checkout-identity and lifecycle
Go tests, 102 route/hydration/MCP unit cases, full typecheck, changed-file lint,
all six mobile runtime cases, three desktop/tablet MCP cases, and four phone
MCP cases. Browser checks use zero retries. Documentation validation and
specification lint pass. Independent review finds no merge concern. Require
fresh-head CI after publishing the merge. The root notification guide remains
uncommitted.


### Optional membership and causal scroll correction

Full CI on dd5ada91cdc completes with 91 passing and 11 skipped checks. Four
browser shards and their aggregate fail. All 118 reports are inspected:
4,305 attempts include five failures and five retries, with no parse errors.

The mobile bottom-freeze test fails twice because its 1.5-second delay expires
before it captures the marker count. The retained failed retry trace and
snapshot show the prompt and assistant already present when the baseline is
two. Reuse the existing mock-turn file gate, establish settled geometry while
the final response is held, require one marker, release once, and require two.
Keep the two-pixel freeze assertion and release the turn during cleanup.
Three focused repetitions and all seven mobile scroll cases pass without retries.

Source review of the non-auto-start failure exposes a separately controlled
production defect. Optional enrichment replays an earlier empty session list
and clears a session prepared after identity load. Four real-store/hydrator
regressions reproduce selection or membership loss, including the interval
between resolution and application. Omit those already applied identity slices
from the successful-empty-identity optional result. A fifth regression proves
unavailable identity recovery remains an authoritative empty read. The owning
UI requirement, design, and progressive-navigation work order are reconciled.
The failed CI attempt has no trace, so this controlled defect is not claimed
as its proven original cause.

The model-preview and restart-activity failures also lack failed-attempt traces;
each passes five unchanged local repetitions. The non-auto-start case passes
five diagnostic repetitions too. Preserve all model, activation, and restart
assertions and retain first-failure traces in their files. Temporary diagnostic
instrumentation is removed. No speculative product change is made for either
unreproduced failure.

Validation on the rebuilt app: 136 focused SSR/route/hydration/task-session
unit cases, the existing mock-turn helper test, four desktop restart/navigation
cases, and four phone navigation/override cases pass. Full typecheck and
changed-file lint pass with zero warnings. Independent review finds no
remaining issue in the fix or its coverage. Full CI on the next published head
remains the delivery gate. NOTIFICATION_GUIDE.md is uncommitted.


### Goal activation and file-transfer view correction

CI on ee67f21f366 contains two failed browser attempts across 118 reports and
4,302 total attempts; the audit has no parse errors. Prior model, restart,
prepared-session, and scroll cases pass in this run. Preserve the distinction
between that result and proof of an earlier untraced failure's exact cause.

The Quick Chat goal-popover failure exposes a controlled desktop interaction
bug. Hover or focus opens the goal details, then the primitive's default click
toggles them closed. Both component regressions fail before correction. The
controlled desktop trigger now explicitly opens and consumes that toggle,
matching the existing goal-disclosure contract. Keep Escape, outside dismissal,
focus restoration, and the native phone drawer. Fifteen focused unit cases,
all three desktop goal scenarios, and full typecheck pass. The goal design and
owning work order are reconciled before production correction.

The collision-upload failure is a test-view mismatch. Its failed screenshot
and context show Changes with the expected `taken-1.txt`, while the assertion
requires a visible Files-tree node. A controlled local run waits for the
Changes row and reproduces that original assertion failure. Follow the
existing neighboring upload cases: await the Changes row, select Files, and
retain both filename assertions. Three repetitions pass with retries disabled.
No upload or keep-both naming behavior changes. Retain first-failure traces in
both affected browser files. Fresh exact-head CI remains required.


Final local verification adds both phone goal scenarios, unchanged touch
controls, zero-warning changed-file lint, and independent read-only review.
The complete prior CI outcome is 93 passing checks, 11 skipped checks, the two
failed browser shards and their aggregate, and zero pending checks. Review
threads are refreshed separately before delivery. Full CI must pass on the
next published commit; no retry-pass is treated as a clean required run.


Main advances to b6263ea4c31 while validation runs. It already contains the
same two-line file-transfer focus correction. Integrate that main commit and
retain a single copy of the correction. Its integration-plugin sidebar changes
require merged-code navigation/plugin grouping validation before publication.


The actual merge with b6263ea4c31 is conflict-free and contains one copy of the
file-transfer correction. All 86 focused sidebar/layout/goal unit cases and
full typecheck pass. Five desktop/tablet MCP/plugin-grouping scenarios and
three phone goal/plugin-grouping scenarios pass with retries disabled. They
cover saved plugin visibility and native phone disclosure. Documentation and
specification validators pass. Publish both local commits together, then
require fresh full CI. The notification guide remains uncommitted.


### Quick Chat menu handoff and fallback diagnostics

Full CI on 1c2335447f7 finishes with 93 passing and 11 skipped checks, two
failed browser shards plus their aggregate, and no pending checks. The audit
covers 118 reports and 4,305 attempts: one failed fallback UI assertion, one
timed-out cross-device setup, two retries, and no parse errors. Prior corrected
cases pass. No review thread is left unresolved in the refreshed snapshot.

The cross-device setup reaches an agent option, then loses it while the Add
menu trigger regains focus. The failed attempt has no trace. Source review
finds the owned menu exit animation and deferred focus return can outlast the
new setup's visibility. Three unchanged local repetitions pass, so exact CI
event ordering remains unproven. A controlled keyboard probe holds that menu
exit, opens the picker, then finishes the exit: Add regains focus and the
picker closes. An earlier pointer probe is inconclusive because the retained
modal overlay blocks the click; it is not used as failure proof.

The shared New-agent setup helper now owns that transition: one Add click,
one scoped New-agent click, menu DOM removal, the same trigger's focus return,
and setup visibility. Use it in both cross-device cases and the existing stale
setup opener. Keep the picker selection, timeouts, tab/rename/delete outcomes,
and production focus behavior. The controlled corrected probe then selects an
agent with a normal mouse click. Both cross-device cases pass three times
(six tests) with retries disabled. Temporary probes are removed before commit.

The fallback failed-attempt trace shows the correct visible session with stale
Starting UI after the API has persisted completion. It has no WebSocket frame
payloads, so dropped delivery, rejected state, and overwritten state cannot be
distinguished. Three full diagnostic repetitions pass unchanged; no product
fix or assertion relaxation is justified. Reuse the existing metadata-only
gateway capture and attach it only on failure, before cleanup, then rethrow.
The full fallback scenario passes with capture enabled. This narrows future
transport evidence; it does not provide complete store/route-generation proof.

Five existing helper unit cases, full typecheck, zero-warning changed-file
lint, and independent read-only review pass. Fresh full CI remains required.
The notification guide stays uncommitted and unchanged.


The same shared handoff also passes a real phone two-chat setup probe with
normal agent selection. All temporary probe files are removed. The complete
fallback scenario passes once more with metadata capture enabled. Normal
commit hooks and fresh exact-head CI are the remaining delivery gates.


### Mobile cancellation observation and topbar evidence

Full CI on 47818c553a6 finishes with 93 passing and 11 skipped checks, two
failed browser shards plus their aggregate, and no pending checks. The audit
covers 118 reports and 4,304 attempts: two failed attempts, one retry, and no
parse errors. Prior corrected cases pass.

The mobile Quick Chat palette case expects a disabled Cancel control after
the actual cancellation has already settled. Its failed context shows the
cancellation notice, idle input, and no Cancel control. Reuse the existing
real-gateway cancellation observation helper in both mobile cancellation
cases: install before navigation, arm for the exact session immediately before
the single action, observe actual pending state and the original disabled
assertion, then release in finally. Keep settlement, touch geometry, follow-up
response, and overflow assertions. Both cases pass three repetitions each
with retries disabled. Independent read-only review finds no issue.

The mobile topbar failure leaves View options open after a Search action.
Its failed attempt has no trace. Five unchanged runs, one controlled long
transition run, and five runs with sixfold CPU throttling all pass. Captured
search clicks in the probes occur after finite animations have finished; this
does not establish an animation cause. Remove all temporary instrumentation
and speculative waits. Preserve the original behavior, assertions, and
timeouts, and retain first-failure traces in both affected browser files.
Fresh full CI and its complete artifact audit remain required. The notification
guide stays uncommitted.


### Optimistic save and review fixture isolation

Full CI on 5c4c2caa52d exposes three different browser failures. The previous
mobile cancellation and topbar cases pass. The Office title test reads the
database immediately after the optimistic render. A controlled held PATCH
reproduces its old-title failure; awaiting the exact task's successful save
response after the original immediate UI assertions makes the probe pass.
All five title/description scenarios pass, including failed-save rollback and
independent-client projection. No production mutation behavior changes.

The mobile review context contains a prior passthrough upload plus the two
owned files. The extra attachment is correctly reviewed, while the manually
cleared target remains unchecked. An injected earlier-sorting attachment
reproduces the aggregate-zero failure. Reuse resetSeedRepositoryCheckout before
task creation and require exact two-file status membership. The contaminated
probe then passes with all original aggregate and manual-scroll assertions.

The coordinator executor option detaches during selection; the failed attempt
has no trace. A controlled held agent-popover exit with normal pointer input
keeps the executor open: nonmodal outside-interaction suppresses old close
focus. This does not reproduce the CI cause. Preserve chooseIdentity actions,
assertions, and timeouts; retain first-failure traces. Remove temporary probes
before commit. The notification guide remains uncommitted.


The final browser audit covers 118 reports and 4,306 attempts: two failed
attempts, one timed-out attempt, three retries, 40 skipped tests, and no parse
errors. CI reports 92 passing checks, 11 skipped checks, three failed shards
plus their aggregate, and no pending checks. Review evidence is complete with
no unresolved threads or API errors. All four coordinator cases pass three
unchanged repetitions. The review flow passes three phone and three desktop
repetitions after removing deliberate contamination. Full typecheck, changed
file lint, documentation checks, guide checksum, and independent read-only
review pass. Fresh exact-head CI remains required.


### Review input cancellation follow-up

Full CI on c6815d866f7 reports 94 passing checks, 11 skipped checks, one failed
browser shard plus its aggregate, and no pending checks. The complete audit
covers 118 reports and 4,305 attempts: two failed attempts, two retries, 40
skipped tests, and no parse errors. Both failures are the same phone review
first-scroll assertion. Fixture membership is exactly two files; this differs
from the corrected contamination failure. First-attempt trace retention works.
Three unchanged local repetitions pass; exact browser causality remains open.

A real-hook regression independently exposes suppression ordering: cancelling
a queued anchor restoration can restore its saved true suppression after
manual input has set false. All three wheel/touch, scroll-key, and root-pointer
cases fail before the correction. Cancel restoration first, then apply explicit
manual marking permission. Preserve descendant-pointer and unrelated-key gates.
The owning review requirement, design, plan, and work order are reconciled
before production correction. All 52 focused input, anchor, auto-mark, and
persistence unit cases pass, including both negative input gates. This verifies
a source defect, not the exact cause of the recorded browser failure.


Follow-up validation: all 52 focused unit cases, full typecheck, changed-file
lint, documentation validators, and three desktop plus three phone browser
repetitions pass after rebuilding web assets. Independent read-only review
confirms the real-hook regression and narrow ordering correction. The original
CI trace does not expose saved restoration state, so its precise cause remains
unconfirmed. Fresh exact-head CI is required after publication.


## npm namespace correction (2026-10-26)

The user selected `@kdlbs/plugin-sdk` after authenticated npm checks confirmed
`zval` owns the `kdlbs` organization. The previous `@kandev` scope is not under
that account's organization membership. Update the frontend package, host type
imports, workspace dependency/lock evidence, checker installation paths,
registry/publication adapters, CI filters, author docs, and queued external
migration briefs together. Historical inventory and review evidence retain the
name inspected at the time. The accepted Go module, independent `0.1.0` SDK
version, host floor, and installed-plugin protocol remain unchanged.

Validation: first exercise the existing registry and installed-dependency tests
against the new identity and observe the old implementation fail. Then run Go
SDK tests/vet, actual npm workspace installs, exact packed consumers in both
TypeScript resolution modes, the real release build adapter, release/workflow
contracts, frontend typecheck, documentation/harness checks, and new-head CI.
No registry publication or placeholder package is part of this correction.


Local namespace validation passed: Go SDK tests/vet and checker race tests;
45 SDK packaging/release tests (the signed-source case required the installed
GPG binaries on PATH); 11 publication-state tests; 63 public-doc validator tests;
35 focused web contract/consumer tests; SDK and full web typechecks; frozen
workspace install; 10 SDK CI contracts; 68 release workflow contracts; nine
action-pinning checks; full documentation/specification/harness validation.
The registry URL assertion failed against the former scope before the change,
and real npm workspace consumers plus both packed TypeScript modes now pass.
Eleven migration task descriptions were updated and verified CREATED with
autopilot disabled. New-head CI and current-main merge validation follow delivery.


## Namespace-head CI cleanup (2026-10-26)

Run `40386346725` on `9168ef88253113814ad283dad80f1ecc08e1091f`
completed with three flaky E2E cases. The complete 118-report audit found
4,308 attempts: 4,265 passed, 40 skipped, three failed and three retries,
with no parse errors. Other checks passed and review threads were resolved.

- Tablet preview: the failed attempt measured the declared 44px button as
  `43.99993896484375`. Keep exact CSS height verification and compare geometry
  at hundredth-pixel precision, consistent with existing touch checks. A
  `43.99px` measurement still fails. Preserve the real tap and workflow result.
- Jira: a successful mock provider contradicted the manually seeded failed
  health row. A controlled real save/probe overwrote that row and reproduced
  the failed banner assertion. Configuring the provider failure before saving
  passed the same controlled check. Arm/verify the actual config-save receipt
  before seeding health. Keep both original failed-state/error assertions.
  Original CI write ordering is unproven because its failed attempt has no trace.
- Sidebar: the actual failure concerns active-row opacity, after persistence
  assertions passed. Its retained trace belongs to the successful retry. Five
  valid unchanged repeats passed; no causal fix is claimed. Preserve every
  assertion and retain first-failure traces. All three affected specs now retain
  failure traces without increasing timeouts or retrying actions.

Validation: the independent Jira producer control failed with the original
inconsistent provider and passed with aligned failure. The three complete specs
passed 17/17 with `--retries=0`; unchanged sidebar repeats passed 5/5.
Local startup required `KANDEV_SERVER_HOST=127.0.0.1`, the existing Nix Chromium
library path, IPv4-first DNS, and an isolated port offset. Earlier setup/browser
launch failures never reached the UI assertion and are not CI reproductions.
Temporary probe files were removed from the repository. New-head CI remains a
subsequent delivery gate. Current main `baa92e5dcf53eca4dac223b0deba5e26c1ee08fb`
adds a Quick Chat helper conflict; preserve the owned menu handoff/lifecycle-only
exclusion and incoming Codex profile/preferred-agent selection together.

Merged-main validation: main `baa92e5dcf53eca4dac223b0deba5e26c1ee08fb`
was integrated after cleanup commit `9530585e5fd7b3c9c42b75debea92549040bc8a6`.
The sole Quick Chat helper conflict retains both parents' behavior: owned menu
handoff and non-lifecycle opening-turn readiness, plus Codex mock preparation
and preferred-agent selection. Automatic merges retain the click-triggered
Stats recovery latch and bounded archive response grace. Independent read-only
review found no actionable issue. Fresh host/web/plugin builds succeeded;
27/27 combined browser tests passed with retries disabled, including cross-device
Quick Chat, opening drafts, Codex recovery/cancellation, and the three CI scopes.
Four focused unit files passed 31 tests; full web typecheck, 10 SDK CI contracts,
68 release contracts, full docs/spec validation, and changed helper lint passed.
The release-key checks required the installed GPG binary on PATH. No publication
or registry/protected-setting change occurred. Fresh pushed-head CI is pending.

## Integrated-head follow-up (2026-10-27)

The `b7f1e656796` full browser audit contains 118 reports and 4,321 attempts:
4,277 passed, 40 skipped, four failed, four retries, and no parse errors.
It exposes an executor-profile routing retry in a green shard as well as the
three red shards (mobile profile ordering, mobile filter drawer, and mid-turn
restart). The complete audit remains a delivery gate alongside check status.

Both mobile cases passed five unchanged local runs; their failed CI attempts
have no retained trace. Preserve their assertions/timeouts and retain the next
first-failure trace. Mid-turn restart also passed five unchanged runs, but its
actual failed trace and backend log show unresolved recovery. A separate
controlled Go test proves pinned silent restore inherits a workspace reader's
refusal because its request has no manual recovery action. The lifecycle fix
classifies the runtime-only pinned candidate for its own admission, preserving
serialization, unrelated-error and cancellation behavior. The owning design
and existing platform delivery work order record the correction and causality
limit. Focused race checks pass. Rebuilt browser, routing reproduction, normal
hooks, push, and fresh-head CI remain pending. No SDK publication occurred.


Follow-up validation: all five unchanged executor-routing repetitions passed
with retries disabled. Its original failed attempt has no trace; independent
source review found no justified synchronization/product patch. Retain a
first-failure trace while preserving all navigation, save, and discard assertions.
The exact final lifecycle selection passed with race detection, including
silent-candidate cancellation, unrelated peer errors, and own-launch failures.
Fresh host/web/plugin builds and three mid-turn restart browser repetitions
passed with retries disabled. Independent review found no actionable issue in
the coalescing correction. The full old-head artifact audit remains four retries;
fresh-head CI is required after delivery.

## Test completion and observation corrections (2026-10-27)

Head `e0415183a79` completed with 89 passing checks, four failed E2E leaves
(2, 11, 17, 21), and a failed aggregate. All 118 browser reports were audited:
4,321 attempts, 4,277 passed, 40 skipped, four failed/retried, no parse errors.
The previous four failing scenarios did not recur; the current four are below.

- Mobile PR history: a controlled real pending Git-detail update removes the
  dialog while the actual contribution request is held. The original HEAD
  assertion fails then; the matching successful response establishes completion.
  The normal fixture now correlates session, provider head, repository root,
  and request ID before retaining every existing Git postcondition. Controlled
  GREEN and six normal paired mobile cases pass. Original CI ordering is untraced.
- Silent prompt notice: the actual failed trace proves output cleared the notice
  during setup before its existence assertion. Two silent release gates replace
  both fixed delays, preserving the real watchdog and RUNNING/reload checks.
  The gate helper retains its start announcement by default. An unextended-helper
  RED independently proves that announcements contaminate this silent scenario.
  Five desktop, four phone, and one default-announcement caller pass.
- Mobile oversized message and task mention: both failed attempts lack traces.
  Each case passed five unchanged repetitions. Keep assertions, actions, and
  timeouts; retain first-failure traces. The mention snapshot lacks the whole
  chat panel, so it is not evidence of a broken mention identity parser.

Independent review found no actionable issue in the two fixture corrections.
Changed-file lint/format and full docs/spec checks pass. Temporary probes are
outside the repository. These are test-only changes; no product/release contract
or SDK bytes changed. Normal hooks, delivery, and fresh-head CI follow.

## Documentation coverage and locale diagnostics (2026-10-27)

Head `3fa49bd631a` completed with 91 passing checks, two failed E2E shards,
a failed E2E aggregate, and invalid documentation coverage. The full 118-report
audit found 4,324 attempts: 4,277 passed, 40 skipped, seven failed/retried, and
no parse errors. Six failures are backend fixture readiness timeouts in shard
11; the other is pseudo-locale activation in shard 22. No other browser
failure is hidden by a passing retry. Startup cancellation logs are cleanup
evidence, not proof of the initial delay. Resource records show no OOM kill
or CPU throttling. No startup budget or product behavior is changed.

The trusted documentation evaluator rejects the legacy `spec` frontmatter in
the changed remote-contribution work order. Its header and sibling plan now
reference the existing task requirement, exact acceptance criteria, and system
design. The trusted-base evaluator accepts the local document closure with
status `covered` and no errors. Full documentation catalog and specification
checks pass; the validator itself is unchanged.

The pseudo-locale failure has no initial-attempt trace. Five unchanged local
repetitions pass. Retain first-failure tracing and attach failure-only locale
metadata: the locale cookie pair, requested/current/reload URLs, response
status, and server/current HTML language. Do not attach other cookies or the
HTML body. A deliberate English-cookie negative control retains the original
assertion failure and produces the expected metadata. A normal diagnostic run
passes. Independent review finds no actionable issue. Original actions,
assertions, and timeouts remain unchanged; locale causality is unresolved.
Normal hooks, delivery, and fresh-head CI remain required.

## Drag and manual-recovery observation corrections (2026-10-27)

Head `d62c643512a` completed with 92 passing checks, two failed E2E shards
(7 and 14), and their failed aggregate. Documentation coverage passed. The
previous startup and pseudo-locale failures did not recur. All 118 browser
reports were audited: 4,319 attempts, 4,277 passed, 40 skipped, two timed-out
attempts/retries, and no parse errors. No other first-attempt failure is hidden.

- Directory drag: the failed command succeeded and all byte/command assertions
  passed, but Files disappeared during inspection. A controlled delay of real
  Git notifications and refresh responses proves the old switch can precede
  Changes autoactivation. Wait for both moved-file rows before selecting Files.
  Controlled GREEN and nine cases across three full-spec repetitions pass.
- Mobile navigation recovery: the failed snapshot already contains recovered
  data while the first Retry tap cannot locate its target. A controlled real
  foreground refresh recovers before the old fixture's tap. Keep reads failing
  until the actual Retry click, captured by the owning menu so child replacement
  cannot lose the latch. Controlled GREEN and all 21 cases across three full-spec
  repetitions pass. The successful read wait is armed before the single tap.

Both original failed attempts lack traces; controlled proof does not establish
their exact event ordering. First-failure tracing is retained. Existing actions,
assertions, and timeouts remain intact. Owning work orders record the evidence.
No product, SDK, or release behavior changes. Scoped lint/format, full docs/spec
checks, and trusted linked-document validation pass. Independent review found
no actionable issue. Normal hooks, push, and fresh-head CI follow.

## Restart and drawer recovery fixture correction (2026-10-27)

Published head `0bbebdcf6afa` ended with 92 passing checks, two failed browser
shards, and their aggregate. All 118 reports were audited: 4,321 attempts, 4,277
passed, 40 skipped, four failed attempts/retries, and no parse errors. Earlier
drag, navigation, startup, and locale corrections passed this run.

The drawer recovery fixture now keeps reads unavailable until its actual Retry
click. A shared stable-owner latch serves the drawer and menu fixtures. A real
foreground-refresh control reproduces premature recovery under the former
fixture; the corrected probe passes. Both full phone specifications pass three
repetitions: 27 cases with retries disabled.

Both failed restart-activity traces contain a competing exact session read
outside the held list route. Its full snapshot correctly removes the synthetic
activity. Independent gates isolate that read until list recovery proves
clearance. A controlled individual-read release reproduces the original UI
precondition failure; isolated list recovery passes. The existing list and
reconciler unit suites pass all 47 tests. All three affected ACP restart scenarios pass three repetitions: nine cases,
retries disabled.

Healthy-sibling setup failed before the slow-restore assertions, without an
initial-attempt trace. Five local attempts completed successfully across an
interrupted run and a fresh run. Preserve the unknown cause. Add failure-only
sibling/backend capture before cleanup and retain first-failure traces. No
product budget, action, assertion, or timeout changes.

Owning work orders record these boundaries. The background-liveness plan/work
order use canonical requirement/design frontmatter without changing their
existing scope or status. Full docs/spec checks and trusted local documentation
coverage pass. Independent review, remaining browser checks, normal hooks,
current-main combined validation, push, and fresh CI remain delivery gates.

## Current-main integration validation (2026-10-27)

Fixture correction `09f3741730d7` passed normal hooks without bypass. Merge
current main `d549fa694b7d5f8e1dd0d6bade40d4e2b41e71d3` without conflicts.
Preserve incoming Quick Chat startup/recovery, the drawer's existing animation
wait, and shared review fixture isolation. The bounded archive response
allowance, actual-click recovery latch, stronger file membership assertion,
and first-failure trace retention remain intact.

Rebuild web, backend helpers, durable-delivery fixture, and packaged plugin from
the combined source. Full typecheck, 90 startup/session unit tests, backend
archive/Quick Chat tests, full docs/spec checks, and trusted exact-diff
documentation coverage pass. The latter checks 461 changed paths and 27 work
orders against 86 linked documents. Five desktop and five phone browser cases
pass with retries disabled, covering restart/list recovery, worktree paths,
uncertain Quick Chat creation, context reset, drawer recovery, and review-file
ownership. Earlier repetitions add 27 phone and nine restart passes.

Normal merge hooks, push, and fresh exact-head CI remain required. The original
healthy-sibling startup cause is still unresolved, and no release publication
or external migration was performed. Keep NOTIFICATION_GUIDE.md untracked.

## Final Save observation correction (2026-10-27)

Head `3e4a5a3aff7f` completed with 92 passing checks, one failed browser shard,
and its failed aggregate. All 118 reports from 34 blob artifacts were audited:
4,322 attempts, 4,281 passed, 40 skipped, one failed attempt/retry, zero parse
errors. All reports use the same CI merge commit `0b4653c83ebb` and event base
`d549fa694b7d`. No additional failing attempt is hidden by a successful retry.

The only failure is the desktop open-editor rename test added on main. It
acknowledges rename and verifies the destination, then reads disk immediately
after the final Save click without waiting for that Save's result. ENOENT
aborts its polling callback immediately. The failed attempt has no trace; its
retry trace cannot establish the original write timing. Backend update success
is returned after Git apply and the updated-file read.

A temporary Git wrapper pauses the real unlink/recreate operation for 500 ms.
The original test reproduces ENOENT. The corrected test waits for the matching
renamed-path/session update receipt, requires success, and reads exact bytes.
Under the same pause, it passes all original disk, editor, tab, and reopen
assertions. The observation budget remains five seconds. Instrumentation is
removed and the worker launcher restored. First-failure tracing remains.
This proves the missing completion boundary, not the exact historical syscall
sequence. Normal repeated checks and fresh CI follow. No SDK or production
behavior changes. The owning editor-continuity work order records the proof.

Normal rename and save-refresh scenarios pass three repetitions: six cases,
retries disabled. Formatting, zero-warning scoped lint, full typecheck, and
documentation/specification checks pass. Independent review finds no actionable
issue. Normal hooks, delivery, and fresh CI remain required.

## Three browser-fixture follow-ups (2026-10-27)

The Save-correction head's terminal CI snapshot has 88 passing checks, three
failed browser shards and their aggregate, plus an automated walkthrough
generator deadline. All 118 blob reports were audited: 4,323 attempts,
4,280 passed, 40 skipped, three failed, two retries, zero parse errors.
Reports share CI merge 55cb2cb715a1 and event base d549fa694b7d.

- Phone Tasks entry now observes its exact opening animation before the
  single filter-gear tap. Five unchanged repetitions and three slowed
  opening probes passed. CI captured parent motion, but the historical
  event-delivery cause remains unresolved. This is fixture readiness hardening.
- Phone survivor validation holds a real mock turn through reattachment
  assertions. The failed trace proves the previous 12-second turn completed
  normally before the running assertion. Explicit release preserves the
  completion, idle and follow-up checks without a larger timeout.
- Canvas reuses the existing 0.01-pixel touch-target precision helper after
  finite owned animations. The reported value was 43.99993896484375 pixels;
  production styles and the 44-pixel requirement remain unchanged.

The owning work orders record evidence and limits. The automated walkthrough
generator returned no response in either bounded 600-second attempt. No
provider, privacy, protected setting, timeout or skip change was made. Native
proposal walkthrough and SDK publication prerequisites remain intact.
Focused checks and current-head CI remain required. NOTIFICATION_GUIDE.md
stays untracked. No SDK or external plugin publication was performed.

Three Canvas cases and three phone filter cases passed. The first gated
survivor run exposed an exact-text locator that failed when streamed chunks
combined. Assistant-scoped substring matching retains genuine completion
proof without matching the user script. All three corrected survivor cases
then passed. All runs disable retries. Full typecheck, scoped zero-warning
lint, docs catalog, spec lint and trusted local coverage pass (31 work orders,
99 linked documents). Normal hooks and main-conflict resolution follow.

## Current-main conflict reconciliation (2026-10-27)

Main d27d8e59302f advances the integrated base and overlaps five paths.
The resolution preserves the real cancellation-pending observation, both
owned drag/drop rows, and the actual Retry-click latch. Stats retains main's
clock-driven automatic-retry proof while keeping requests unavailable until
the click event. SSH combines the incoming successful PATCH receipt and
baseline-name cleanup with the existing final Saved-status check.

Changed-file reconciliation combines incoming reactive client identity with
background-only snapshot loading and post-await session ownership. Its real
store regression proves that a client installed or replaced at unchanged
connected status acquires a fresh lease and snapshot. Singleton-only lookup
fails that regression. Three automatically duplicated detailActive declarations
are removed while preserving chat and historical-viewer demand gating.
Fresh combined builds, scoped tests and review remain required. Docker is not
installed locally; the SSH container case requires real container CI.

Combined-source validation passes fresh E2E web/backend/helper/delivery/plugin
builds, 125 focused web tests, full typecheck, zero-warning scoped lint,
backend broker/plugin race checks, 27 E2E workflow contracts and 10 SDK CI
contracts. Seven phone cases pass with retries disabled, covering all three
new corrections plus cancellation, Stats recovery and historical changed files.
Desktop drag/drop, dirty-editor Save and historical-capture checks remain in
progress. Independent review finds no remaining actionable merge issue.

All four desktop integration cases pass with retries disabled: captured-turn
history, dirty-editor Save, single-file drag/drop, and directory descendants.
All seven phone cases also pass. The final merged source passes full
typecheck, 125 focused units, backend broker/plugin race tests, workflow
contracts, zero-warning scoped lint, and docs/spec checks (406 decisions,
1,755 specifications). No unresolved merge paths or whitespace errors remain.
Normal merge hooks, exact-head push and fresh CI remain required.

## Hidden retry findings and current fixes (2026-10-27)

The last head ends with 92 passing checks, one failed automated walkthrough,
and zero pending. Both the 118-report audit and official retry summary expose
two successful retries: profile-name selection and Azure phone board detail.
The 4,333 attempts comprise 4,291 passed, 40 skipped, one failed and one timed
out; two retries and zero parse errors. Reports use merge 408bc0d0d94c and
base d27d8e59302f. Passing aggregates do not establish a clean browser run.

Profile-name setup assumed the first family owned a profile. A controlled
profileless-first catalog reproduces the TypeError; exact seeded-profile
selection passes. No production change is needed for that case.

A held real Azure Move response proves an independent product race: Move,
Close, then successful completion reopens detail. The original failed attempt
has no trace, so its exact ordering remains unproven. A single opening owner
now guards detail, selection/focus, busy and error publication. The duplicate Move item state and
its late continuation are removed. Assignment keeps its existing endpoint and delivers receipts to the board
mutation owner so confirmed data still updates after Close. A separate board-context owner
rejects success, rollback and error into a later context. Existing refreshes
retain opening ownership. Deferred board success/failure regressions fail
against the old code. Shared domain-local lifecycle code serves both owners;
private board snapshot extraction keeps hook size within repository limits.

The owning requirement, design and work orders record the correction. Public
docs need no new instructions because Close and selection behavior are
restored. Browser checks, full typecheck/lint, review, hooks and new-head CI
remain required. No SDK publication or protected setting changes.

Latest validation passes 30 focused Azure tests, full typecheck, zero-warning
scoped lint, docs/spec checks and exact-diff documentation coverage (34 work
orders, 110 linked documents). Three held-Move phone cases and all six desktop
Azure/profile cases pass with retries disabled. Temporary catalog/probe
instrumentation is absent. The canonical SDK design now names the selected
@kdlbs ownership prerequisite; historical original-name records remain intact.
Main 61a96d8c9a99 has independently added a minimal Azure close guard and a
held-assignment desktop test, plus protected-workflow input hardening. The
next integration preserves those contracts and aligns SDK tag credentials
with command-scoped authentication. No release or protected setup is changed.


## Main f306 integration and release authentication (2026-10-27)

The working merge integrates f30676f2a8ac over the committed Azure/profile
corrections d5aedf613f3c. Eight conflicts preserve causal turn gates, actual
Stats click admission, exact Office save receipts, shared touch sizing,
opening-owned Azure detail and the system-resolved sleep timeout fixture.
Main's held-assignment desktop regression remains intact. Duplicate selection
mutation logic is replaced by canonical board data plus opening ownership.

SDK tag authentication now uses a command-scoped GitHub credential helper.
The focused argument regression fails against the previous push and passes
with the correction; the real signed-tag fixture passes too. No signing/OIDC
job separation, completion gate, registry publication or protected setting
changes. Security scan returns the ten already reviewed upstream residuals:
six trigger warnings, three CMD limitations and one informational suggestion.
No new suppression is added.

Current checks pass full typecheck, zero-warning scoped lint, 125 Azure unit
cases, release/E2E/walkthrough contracts, action pinning, harness checks and
408-decision/1,759-specification validation. Fresh web/backend/helper/delivery/plugin builds and executor/ACP/stream race
tests pass. All nine desktop and four phone browser cases pass with retries disabled.
The earlier automated walkthrough remains a no-output deadline, separately
from browser correctness. New-head CI has not run yet.


## Main f150 navigation and transcript integration (2026-10-27)

After the normal-hook merge commit 93cf43a61ad4, main f150e6accd66 adds
transcript segment identity, presentation and upload fixes. Five conflicts
preserve the isolated offline PR fixture and cleanup, optional executor input,
actual-click sidebar recovery with held successful reads, turn-change source
inventory and both navigation ownership contracts. A controlled existing
regression fails when main's unconditional selection omission leaves a stale
session selected after authoritative empty recovery. Conditional recovery
preserves that exception and all incoming selection-preservation tests.

Current checks pass rebuilt web/backend/helper/delivery/plugin artifacts,
41 enrichment/presentation tests,17 subscription tests, full typecheck,
zero-warning scoped lint and targeted SQLite canonical-delivery, lifecycle
notification and exact-upload race tests. No new capture, subscription or
runtime semantics are introduced by conflict resolution. Independent review
finds no remaining actionable merge issue. All four rebuilt desktop cases pass: retained first-turn history, three isolated
PR layouts, non-auto-start admission and keep-both upload. Both phone cases
pass: historical diff and held sidebar recovery. Retries are disabled.
Documentation coverage passes528 changed paths,34 work orders,110 documents.
Docs validate409 decisions and1,762 specifications; size limits are preserved.


## User-requested rebase onto main (2026-10-27)

Rebased with merge history preserved onto 9c7359d3aa9e. Rebased integration
head 4be5097655a7 has exactly the previously tested SDK tree plus main's six-file
advisory walkthrough update; no application code changed. Comparing the full
result with the precomputed combined tree finds no missing merge resolution.
Main now records generation failure as a warning and skips publication/linking
without a valid generated artifact. This changes CI policy; it does not prove
the silent generation request repaired. E2E workflow code is unchanged.

Walkthrough contracts and action pinning pass. Zizmor retains the same ten
reviewed residuals. The prior-head CI monitor was stopped for this explicit
rebase request, not interpreted as a terminal CI result. Its partial artifact
audit covered 46 reports: 1,657 passed, 24 skipped, zero retries or parse errors.
Normal hooks, leased push and fresh-head checks remain required. The user-owned
NOTIFICATION_GUIDE.md checksum is unchanged through the rebase.


### Final rebase target and verification

Main advanced to 86a6b04e4923 during the operation. Rebased onto that exact
commit with merge topology preserved; intermediate head 56a7a6ba7ab8 matches
the precomputed combined tree 087d6cac52ff exactly. SDK package/module files,
tag authentication and protected release jobs remain byte-identical to d9c00f6.
The five historical resolutions retain authoritative recovery and fixture
isolation. Incoming clarification hosts keep detailActive forwarding.

Playwright matches main: failOnFlakyTests is false in CI unless
E2E_FAIL_ON_FLAKY=1. No new strict-flake override is introduced. Main's
advisory walkthrough policy is retained; this is not a generation repair.
Fresh web/backend/mock-agent builds, 129 focused UI/subscription tests, full
typecheck, zero-warning scoped lint, targeted mock/MCP/sysprompt race tests,
31 walkthrough contracts, pinning, harness and docs/spec checks pass. No
application change was added beyond main. Final checkpoint hooks and leased
push follow; the notification guide remains uncommitted.

### Advanced-main conflict correction

Main advanced to a6144c3cb8ef during CI and introduced one pseudo-locale
conflict. Rebased again with merge topology preserved. Intermediate head
6484b4b04c75 matches the precomputed combined tree 29fe142e878b exactly.
The locale fixture keeps main's cookie-before-navigation behavior and this
branch's failure diagnostics. All assertions and timeouts remain intact.
Incoming plugin launch-profile tests initially failed to compile because one
new test retained the removed SDK import. That test now imports the extracted
module; the intentionally old compatibility fixture is unchanged.

Plugin and backend adapter race checks pass after that import correction.
Full typecheck, scoped zero-warning lint, fresh managed web/backend/fixture
builds and all three focused pseudo-locale browser cases pass. Browser retries
are disabled. Initial local attempts failed before test bodies because of
inherited host configuration and missing Chromium libraries; the passing run
uses isolated loopback startup and verified existing Nix library paths.
Docs validate 410 decisions and 1,765 specifications. SDK package/module,
protected release jobs and main's informational retry policy are unchanged.
The prior-head CI had 92 passed checks, no failures and one pending at this
checkpoint. It is not terminal evidence for the updated head. Normal hooks,
leased push and fresh exact-head CI follow; the guide stays uncommitted.

### Required CI release-notice localization correction

Head b5e5644407f6 finished CI with 91 passed checks and two failed checks:
E2E shard 24 and its aggregate. Nineteen pseudo-locale cases exhausted all
retries on the same English Kandev release notice. The toast hook preferred
populated backend copy over existing catalog keys. Two new browser/native
delivery regressions reproduce that precedence after a locale switch.

The correction always resolves Kandev release copy from the active catalog,
preserving version and occurrence identity. Runtime/session notification
policy, backend provider payloads, SDK code and main's retry policy are
unchanged. The owning platform requirement, i18n design, update-notification
plan/Task 04 and public operations guide record the boundary.

Both new tests fail before the correction. All 57 related unit tests, all 22
rebuilt pseudo-locale browser cases and the existing phone toast/composer case
pass afterward, with browser retries disabled. Full typecheck, scoped lint,
localization checks and docs/spec validation pass. This is a product copy
correction, not a waived assertion or recovered-flake finding. Normal hooks,
push and fresh current-head CI remain required.

The owning legacy work order and plan now declare their requirement, acceptance
and design links in the current format. Direct documentation coverage passes
535 changed paths, 35 work orders and 115 linked documents without changing
the validator.

### Reconcile the landed localization and accessibility fixes

Head 3ee4e17e6ac1 finished exact-head CI with 93 passed checks, no failures
and no pending checks. The waiter returned exit 1 only because main advanced
to 04c4d5d02291 and introduced a release-toast conflict. Main now owns the
same versioned localization correction plus an explicit unversioned legacy
fallback, deleted-session read cleanup and executor-card keyboard access.

The new rebase preserves merge topology and the upstream compatibility guard.
Its pre-consolidation tree exactly matches the precomputed combination
e24d489285fa. The redundant native-only locale test is covered by the retained
browser/native delivery-time matrix; main's legacy fallback test and controlled
WebSocket pseudo-locale case remain. Localization authority is consolidated
into the landed requirement/design/work order, removing the temporary parallel
amendments while preserving the chosen @kdlbs SDK namespace. Focused rebase
validation and normal checkpoint hooks precede the next leased push.

Rebase validation passed 126 frontend tests across ten files, backend
session/Git race checks, all 29 freshly rebuilt desktop browser cases and
both phone cases, with retries disabled. Typecheck, scoped lint, localization
and docs/spec validation passed (410 decisions and 1,767 specifications).
Direct documentation coverage passed 532 changed paths, 35 work orders and
114 linked documents. A two-release WebSocket control reproduced an ambiguous
title-only toast locator; version scoping passes while retaining localized
title/body assertions. Standard release producers include the version, and
main's unversioned legacy fallback remains intact. SDK and protected release
files are unchanged from the prior 93-check green head. The next commit and
leased push still require fresh exact-head CI; prior results are historical.

### 2026-10-28 main integration

Published head 4956948f78a4 retained 93 green checks, but main advanced to
`d179ce206fd46c7793bda916b025d753036c7f3b` and introduced three conflicts.
Integrate that main revision into the PR without changing SDK release identity,
wire compatibility, independent versioning or the recovered-retry policy.

The locale fixture retains main's cookie header assertion plus this branch's
navigation diagnostics and version-scoped two-release check. The session-resume
fixture adopts main's drain-before-hold detail-read sequencing, preserving its
list-reconciliation assertions and first-failure traces. The phone marketplace
keeps the finite detail-animation boundary and main's labelled sizing check.
A new plugin-executor identity test imports the extracted SDK; the intentional
legacy binary fixture remains unchanged. Same-task optional enrichment still
preserves admitted sessions and permits authoritative unavailable-list recovery.
One incoming work order's trailing blank line is removed for whitespace checks.

Focused verification passes 81 frontend tests across nine files; backend
plugin-executor and WebApp race checks; full typecheck; scoped Go/web lint;
23 rebuilt pseudo-locale cases; all three ACP restart cases; and the phone
marketplace case. Browser retries are disabled. Localization and docs/spec
checks pass (412 decisions and 1,777 specifications). Direct documentation
coverage passes 534 changed paths, 36 work orders and 118 linked documents.
Normal merge hooks, push and fresh current-head CI follow. Historical-head
CI cannot certify this combined head.

The uncommitted notification guide now includes the maintainer walkthrough.
A fresh anonymous npm lookup is 404; saved authenticated npm commands return
401, so the operator must log in again before confirming scope ownership.
Both SDK environment confirmations remain absent. Existing signing and channel
secret names are present, but secret values were not read or tested. No package,
release, environment setting, migration or external notification was changed.

### 2026-10-28 final conflict reconciliation

The user will merge PR #4534. Integrate main
`fb4c2bc3ef98da2fc303fe05cb144bcd6c167d58` without merging the PR or
continuing the cancelled npm bootstrap investigation.

Keep main's shared Go cache families and main-only cache publication roles.
The shared cache identity still hashes both backend and standalone SDK module
files. Its CLI regression checks SDK dependency invalidation alongside the
new save-role assertions. Keep main's frontend timing allocation and the
informational recovered-retry policy. The pseudo-locale regression uses main's
two explicit release versions, verifies both notices, and retains the exact
localized body and version-scoped locator. Restore the archive handler's time
import after incoming attachment changes remove its former use.

Validation passes 106 cache/workflow/timing/gate checks, archive HTTP/WebSocket
race tests, full frontend typecheck and scoped Go/web lint. All 23 pseudo-locale
browser cases pass on a fresh build with retries disabled. Go module tidy adds
no changes and module verification passes. Documentation validation passes
414 decisions and 1,791 specifications; direct coverage passes 36 work orders
and 118 linked documents without errors.

Normal merge hooks and push follow. Historical head c3fa104f6907 had 95 passing
checks; those results do not certify the new integrated head. Fresh GitHub
checks must pass before the operator merges. NOTIFICATION_GUIDE.md remains
uncommitted. SDK release code and protected publication contracts are unchanged.


### 2026-10-28 Windows CI compilation fixup

Current-head backend job `122779656390` reaches its ten-minute Windows
persistence compilation cap after five package binaries compile. Its module
and build caches both miss. The compiler is cancelled before selected tests
start. The correction changes only that compilation allowance to twenty
minutes; test deadlines, selections and required failure propagation remain.
The owning CI completion design and task-16 work order record the evidence
and validation. Fresh hosted Windows CI remains required before merge.

### 2026-10-28 integration with MCP assignment main

The next main revision is `4222c3654f9bb1efd1a7d27b5151c4b4c4f1ffd2`.
Resolve three conflicts while preserving both input contracts: cancel the
pending review-anchor restoration first, then admit direct-root pointer input
or touch contact inside the scroller. Mouse and pen descendant presses retain
suppression. The real-hook regression adds touch and pen cases under a pending
restoration; the touch case fails before the combined correction and passes
afterward. Main's certified-crossing tests remain intact.

The silent-output fixture retains the shared unique mock gates, both silent
before release, with a separate completion hold and finally release. This
preserves main's replacement of the timed delay without duplicating gate
plumbing. Auto-merged locale, Stats and chat fixtures preserve their existing
branch corrections. SDK/package/release automation and the twenty-minute
Windows persistence compilation allowance are unchanged.

Validation passes 39 frontend tests, backend MCP eligibility race checks,
full typecheck, scoped lint and docs/spec validation (414 decisions and 1,797
specifications). Fresh browser checks pass all 23 pseudo-locale cases and two
desktop/two phone review/stall cases with retries disabled. Direct documentation
coverage passes 36 work orders and 118 linked documents. Remove the duplicate
trace-retention declaration introduced by the automatic merge; one remains.

Normal merge hooks, commit and push follow. The preceding head's 95 green
checks are historical; fresh GitHub CI must verify the new head. The user owns
the merge, and NOTIFICATION_GUIDE.md remains uncommitted.

Main advanced during verification to
`3f3aabc53f23cc8b08f0bf2b1b766b88fbeaf7f2`, adding prompt suggestions and
rich-output error disclosure. Its ACP replacement update conflicts with the
standalone SDK entries in go.mod. Keep the new ACP version and both the
standalone SDK requirement/local replacement. Go module tidy produces no extra
changes and verification passes. The initial three-conflict merge is recorded
in `8febdc23fbab`; the final integration also includes this newer main revision.
Full typecheck and 135 focused review, notification, prompt-suggestion and
rich-output tests pass. Backend rich-output race checks pass. All thirty fresh
browser cases pass with retries disabled: twenty-three pseudo-locale, three
desktop and four phone review, stall and rich-output cases. Docs/spec validation
passes 414 decisions and 1,800 specifications. Module tidy and verification
pass without further changes. Direct coverage passes 36 work orders and 118
linked documents. These results validate the later ACP combination, rather
than relying on the earlier browser run.
