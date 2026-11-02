---
id: "01-go-module"
title: "Extract Go SDK and preserve host execution"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-SDK-DISTRIBUTION-001
  - REQ-PLUGINS-SDK-DISTRIBUTION-004
acceptance_criteria:
  - AC-PLUGINS-SDK-DISTRIBUTION-001.1
  - AC-PLUGINS-SDK-DISTRIBUTION-001.3
  - AC-PLUGINS-SDK-DISTRIBUTION-004.1
system_design:
  - ../../specs/plugins/system-design/sdk-distribution.md
---

# Task 01: Extract Go SDK and preserve host execution

## Summary

Create the independently fetchable Go module and migrate the host in one pass.
Prove that module isolation does not hide missing dependencies and that existing
plugin binaries continue to execute against the extracted host contract.

## In scope

- Move SDK source/tests and canonical proto/Buf outputs to `pluginsdk`; introduce
  a minimal module with current dependency/toolchain versions and license.
- Migrate every SDK/proto caller, including both plugin fixtures. Keep backend
  module identity; add its local `../../pluginsdk` replacement.
- Create `pluginsdk/Makefile`, preserving backend proto delegation. Update CI
  discovery, Docker Go-stage layout, build/test scripts, and deprecation ledger
  paths affected by the move.
- Create local Go consumer archive/proxy support and tests under
  `scripts/pluginsdk/`; support `--backend-only --skip-compatibility-check`.
  Enforce the required isolation and CI cases below, including the exact-module
  checksum exception and SDK-only backend/E2E selection tests.
  Add `check-proto-generation.mjs` and its tests to snapshot generated outputs,
  regenerate, and byte-compare the results without staging them.
- Add `sdk_binary_compatibility_test.go` under the host runtime tests. Build a
  bounded legacy plugin from pre-extraction `v0.97.0` in a disposable snapshot;
  prove actual process handshake plus Host state call against the new host.

## Required isolation and CI cases

Use the design's [clean-consumer environments](../../specs/plugins/system-design/sdk-distribution.md#clean-consumers).
The local proxy has no client fallback and sets
`GONOSUMDB=github.com/kdlbs/kandev/pluginsdk` only for local consumers.
Keep other dependencies authenticated. Set `GOWORK=off`
and `GOENV=off`. Allocate fresh caches outside the checkout. Deleting the SDK
archive must fail even when that version exists publicly.

Update `.github/workflows/backend-tests.yml` and `.github/workflows/e2e-tests.yml`
for `pluginsdk/**` and `scripts/pluginsdk/**`. Preserve Markdown exclusions and
  required-check reporting. Add `.github/scripts/plugin-sdk-ci-contract_test.py`
with single-path fixtures for SDK Go, proto, module, tooling, and README changes.
Read actual workflow patterns through `changed-paths.py`. Assert SDK race/vet,
host compile/runtime coverage, and E2E build/fixture selection for source changes.
Assert the existing documentation skip for README-only changes.

Include both modules in cache/dependency preparation. Upstream now has
`.github/actions/cache-go/`, `go-cache-identity.*`, and `ci-test-only-changes.py`.
Inspect these on the implementation base. Prove SDK production changes cannot
select a test-only shortcut or reuse a stale host/fixture artifact. Wire the new
contract test into CI so future selection changes run it.

## Out of scope

SDK metadata/checker, frontend packaging, publication, or new wire methods.
Do not leave old production SDK/proto copies or aliases.

## Acceptance

1. An isolated Go archive consumer builds, vets, and tests with no backend graph,
   SDK replacement, sibling checkout, or workspace dependency.
2. The host and fixtures compile, SDK/runtime tests pass, and a legacy plugin
   subprocess handshakes and uses Host state with unchanged wire identities.
3. Proto generation is reproducible, CI explicitly covers the SDK module, and
   Docker's module layout passes its focused build-stage check. SDK-only source
   changes schedule SDK, host, and E2E build checks. README-only changes follow
   the documented skip policy.

## Verification

Run from repo root. The Docker command is a targeted builder check. Run it locally when Docker
is available; otherwise require the `Plugin SDK Docker builder` CI job on the
exact implementation commit before completing this work order.

```bash
node scripts/pluginsdk/check-proto-generation.mjs
(cd pluginsdk && go test -trimpath -race ./... && go vet ./...)
(cd apps/backend && go test -trimpath ./internal/plugins/... ./internal/automation/...)
(cd apps/backend && go test -trimpath -run '^$' ./...)
make -C apps/backend build-kandev
node --test scripts/pluginsdk/consumer-smoke.test.mjs scripts/pluginsdk/check-proto-generation.test.mjs
node scripts/pluginsdk/consumer-smoke.mjs --mode local --backend-only --skip-compatibility-check
docker build --target go-builder -f e2e.Dockerfile -t kandev-sdk-go-builder-check .
python3 .github/scripts/plugin-sdk-ci-contract_test.py
python3 .github/scripts/changed-paths_test.py
python3 scripts/lint-architecture.test.py
git diff --check
```

The generation script compares regeneration to the intended new generated
outputs, including untracked files. A mismatch fails without staging files.

## Files likely touched

- `pluginsdk/{go.mod,go.sum,Makefile,LICENSE,*.go,proto/**}` (new/moved).
- `apps/backend/{go.mod,go.sum,Makefile,pkg/pluginsdk,proto}`.
- Host imports under `internal/{plugins,automation,backendapp,agent/runtime/lifecycle,task}`
  and `apps/backend/{cmd/plugin-fixture,internal/plugins/runtime/testdata/fixtureplugin}`.
- `e2e.Dockerfile`, `.github/workflows/backend-tests.yml`,
  `.github/workflows/e2e-tests.yml`, and `config/architecture-lint/deprecation_ledger.json`.
- `.github/scripts/plugin-sdk-ci-contract_test.py` (new), plus cache and test-only
  selection helpers present on the implementation base.
- `scripts/pluginsdk/{consumer-smoke.mjs,consumer-smoke.test.mjs,check-proto-generation.mjs,check-proto-generation.test.mjs}`
  (new) and runtime binary-compatibility tests.

## Dependencies

None.

## Risks

An incomplete import/build-context migration or backend-only CI can conceal
breakage. Same-process gRPC tests do not prove binary compatibility. Legacy
source must stay pinned and test-only.

## Parallelism

`sequential`

## Inputs

- [Design: Go module boundary and host migration](../../specs/plugins/system-design/sdk-distribution.md#go-module-boundary).
- Existing `serve_test.go`, `protocol_test.go`, runtime subprocess tests,
  backend Makefile proto target, `e2e.Dockerfile`, and scoped backend guidance.

## Results

Implementation started on 2026-10-19 at PR head `ede49b4467`.
The SDK and canonical protocol now live in root `pluginsdk/`; host imports,
module replacement, generation targets, Docker layout, and CI selection changed
atomically in the working tree. No registry publication occurred.

Validation completed locally:

- SDK `go test -trimpath -race ./...` and `go vet ./...`: passed.
- Protocol regeneration and byte comparison: passed, including untracked outputs.
- Legacy binary subprocess: passed. The source is pinned to the peeled `v0.97.0`
  commit `e43881c7555372897b57ec51c705f1e05da43c40`. The fixture completed its
  handshake and called Host state through gRPC.
- Host plugin/automation tests: passed with `umask 022`. The first run inherited
  `0077` and failed the existing package-install mode test (0700 versus 0755).
  No production permission behavior was changed to accommodate this environment.
- Full backend compile-only test and `make -C apps/backend build-kandev`: passed.
- Consumer/protocol helper tests: nine passed, including missing-archive rejection
  with a fresh Go cache, no SDK proxy fallback, trust isolation, and replacement rejection.
- Local isolated consumer build/vet/test passed. A cleanup failure from read-only
  Go cache directories was corrected with explicit `-modcacherw`; the final end-to-end
  rerun, including cleanup and the replacement-parser adjustment, passed.
- After merging main `76954ff53e`, CI contract tests: seven passed. Existing
  changed-path tests: 23 passed. Architecture-linter tests: 99 passed.
  Shared cache identity tests: seven passed, including SDK module/sum changes
  through the actual CLI. Test-only selection tests: eight passed. Backend
  workflow contracts: 25 passed. External-runner contracts: five passed.
  Both the shared cache action and direct workflow caches include the SDK.
  SDK production changes cannot select the test-only shortcut.
- Documentation catalog and specification lint: passed.

Docker is unavailable locally. The required build-stage check runs in the
`Plugin SDK Docker builder` CI job, included in the backend gate. This work
order was left `in_progress` until CI validated the remediation below. The
completion record now confirms that gate passed.

CI follow-up: the shrink-only deprecation baseline cannot accept a renamed path.
The existing `Task.Labels` deprecation now has an explicit compatibility-ledger
entry at its new path. The field and wire mapping remain unchanged. The 1.0.0
host checkpoint requires review, not automatic removal.

The exact-head `Plugin SDK Docker builder` CI job passed for
`f2eeaf6c93393d696a84b09e6090e332a6f98447` (run 39245035281, job 118035509627).
Preview CI exposed a second source mount at `cmd/preview/build.go`. Its Docker
mount now preserves `apps/backend` and read-only `pluginsdk` siblings. Fork
preview artifacts include both paths and retain the repository-relative layout.
A fake-Docker command regression test covers the mounts. Later CI passed the
Docker builder and preview checks, but exposed SDK lint and legacy-checkout
issues. Final remediation adds release history to runtime shards, retains the
existing adapter-only file-length exemption at the new path, and documents the
deprecated labels read required for API v1 compatibility.

## Completion

Completed on 2026-10-23 after local acceptance checks and implementation CI
passed. The [plan's cleanup record](plan.md#validation-and-review-cleanup)
contains the validation evidence and review dispositions. Public publication,
registry configuration, external repository migration, and author notices
remain separate follow-up work; completion here does not certify those actions.
