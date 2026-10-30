---
id: "06-session-cost-adapter"
title: "Adopt the generic contract in Session Cost"
status: in_progress
wave: 6
depends_on: ["05-retained-reporting"]
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-HISTORY-001
  - REQ-COSTS-USAGE-HISTORY-002
  - REQ-COSTS-USAGE-HISTORY-003
  - REQ-COSTS-USAGE-HISTORY-004
  - REQ-COSTS-USAGE-HISTORY-007
acceptance_criteria:
  - AC-COSTS-USAGE-HISTORY-001.5
  - AC-COSTS-USAGE-HISTORY-002.1
  - AC-COSTS-USAGE-HISTORY-002.2
  - AC-COSTS-USAGE-HISTORY-002.3
  - AC-COSTS-USAGE-HISTORY-003.1
  - AC-COSTS-USAGE-HISTORY-003.2
  - AC-COSTS-USAGE-HISTORY-003.3
  - AC-COSTS-USAGE-HISTORY-003.4
  - AC-COSTS-USAGE-HISTORY-003.5
  - AC-COSTS-USAGE-HISTORY-004.1
  - AC-COSTS-USAGE-HISTORY-004.2
  - AC-COSTS-USAGE-HISTORY-004.3
  - AC-COSTS-USAGE-HISTORY-004.4
  - AC-COSTS-USAGE-HISTORY-004.5
  - AC-COSTS-USAGE-HISTORY-004.6
  - AC-COSTS-USAGE-HISTORY-007.1
system_design:
  - ../../specs/costs/system-design/usage-history.md
---

# Task 06: Adopt the generic contract in Session Cost

## Summary

Update the sibling Session Cost plugin to the compatible retained-usage Host contract.
Keep tokscale as one replaceable adapter, with durable import progress and canonical saved-value reads.

## In scope

- Pin the finalized published core SDK revision and adapt the supported versioned measurement DTOs.
- Resolve and retain core-issued work bindings. Never assign the collector/refresher as the usage actor.
- Preserve nullable categories, exact model IDs, optional provider/client data, source calendars, and estimated cost semantics.
- Use snapshot/keyset target discovery for resumable import. Correct the current bounded-sample missing-session overcount and label legacy unreliable diagnostics explicitly.
- Preserve the existing single subprocess, timeout, cache/coalescing, enabled/disabled behavior, final stabilization, and shutdown contracts.
- Consume accepted write statuses and canonical readback. Older unsupported hosts show upgrade guidance and retain saved values.
- Add replay/replacement-collector fixtures without adding a new tool. Keep counts after plugin disablement/uninstall.
- Update plugin README and the earlier companion plan with the exact SDK and results. Do not publish a release in this work order.

## Out of scope

- Core schemas or source priority, a second native writer, remote transcript access, and new toolbar composition.

## Acceptance

- Core attributes org/workspace/actor and deduplicates equivalent work independently of the collector tool. Unsupported fields remain unknown.
- Restarted imports traverse the fixed target snapshot without recounting missing sessions or skipping records after concurrent deletion.
- Tests and packaged host smoke flows preserve responsive saved values, compatibility states, and the one-process invariant.

## Verification

Commands start at the Kandev root and use the sibling layout in `go.mod`.
The smoke config starts its isolated host fixture. Run both projects sequentially with retries disabled.

```bash
(cd ../kdlbs-kandev-plugin-session-cost && go mod tidy -diff)
(cd ../kdlbs-kandev-plugin-session-cost && make check-format vet test)
(cd ../kdlbs-kandev-plugin-session-cost && go test -race ./server/... -run 'TestUsageHistory|TestReport|TestCollector|TestHistorical' -count=1)
(cd ../kdlbs-kandev-plugin-session-cost && make build verify-package-host verify-package)
make build-backend build-web-e2e build-e2e-plugin-package
(cd apps && pnpm install --frozen-lockfile)
USAGE_HOST_ROOT=$PWD
USAGE_PLUGIN_ROOT=$(cd ../kdlbs-kandev-plugin-session-cost && pwd)
(cd apps/web && NODE_OPTIONS='--import=tsx' KANDEV_HOST_ROOT="$USAGE_HOST_ROOT" SESSION_COST_PACKAGE_PATH="$USAGE_PLUGIN_ROOT/kandev-session-cost-0.5.0.tar.gz" SESSION_COST_EXPECT_ACTION=1 E2E_PORT_OFFSET=22 pnpm exec playwright test --config "$USAGE_PLUGIN_ROOT/test/host-action-smoke.playwright.config.mjs" --project=chromium --workers=1 --retries=0)
(cd apps/web && NODE_OPTIONS='--import=tsx' KANDEV_HOST_ROOT="$USAGE_HOST_ROOT" SESSION_COST_PACKAGE_PATH="$USAGE_PLUGIN_ROOT/kandev-session-cost-0.5.0.tar.gz" SESSION_COST_EXPECT_ACTION=1 E2E_PORT_OFFSET=23 pnpm exec playwright test --config "$USAGE_PLUGIN_ROOT/test/host-action-smoke.playwright.config.mjs" --project=mobile-chrome --workers=1 --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
(cd ../kdlbs-kandev-plugin-session-cost && git diff --check)
```

The packaged-smoke environment follows the
[`README.md`](../../../../kdlbs-kandev-plugin-session-cost/README.md#packaged-browser-smoke-test).
Record the actual paths, package version, SDK pin, and results. Update the archive filename if the planned version changes.
The two required suites are `test/host-action-smoke.spec.mjs` and `test/mobile-session-cost-action-smoke.spec.mjs`.
No missing fixture, skipped smoke, or mismatched SDK counts as successful integration evidence.

## Files likely touched

- `../kdlbs-kandev-plugin-session-cost/{.kandev-sdk-ref,README.md,manifest.yaml}`
- `../kdlbs-kandev-plugin-session-cost/server/{plugin.go,collector.go,import.go}` and new `usage_history_test.go`
- `../kdlbs-kandev-plugin-session-cost/test/{bundle.test.mjs,responsive-bundle.test.mjs,host-action-smoke.spec.mjs,mobile-session-cost-action-smoke.spec.mjs}`
- `../kdlbs-kandev-plugin-session-cost/docs/plans/token-usage/plan.md`
- This work order and its parent plan for final integration evidence

## Dependencies

Task 05. Core must expose the compatible retained contract and canonical reads before its SDK pin is published for the plugin.

## Risks

Tokscale can omit provider, actor, or dated detail. Do not derive those fields from guesses.
An older checkpoint cannot reconstruct an exact missing-session count from its 100-ID sample.
Changing the measurement source must not create another cumulative contribution for the same transcript.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/costs/requirements/usage-history.md), listed ACs.
- [Design](../../specs/costs/system-design/usage-history.md), Measurement contract, Source identity, and Migration and recovery.
- [Earlier plugin package](../../../../kdlbs-kandev-plugin-session-cost/docs/plans/token-usage/plan.md) and existing responsive/collection/import tests.
- The finalized generated SDK from Tasks 03 through 05.

## Results

Task 06 remains in progress. The component results below are earlier checks against local SDK states; they do not prove the final SDK pin, package, or Host integration.

### Prior Phase B and C component results (historical)

SDK-layout preparation completed against the then-current local core:
new module imports/replacement, sparse checkouts, and package-tool layout were adapted.
`go test ./server/...`, `go vet ./server/...`, explicit-output Go build, `make check-format`, `make test`, and `make verify-package` passed.
The package covered all five declared platforms and 44 UI tests. Workflow YAML parsing and repeated tidy also passed.
Phase B implemented retained bindings, durable revision replay, bounded observation batches, corrections/supersession,
snapshot/keyset import, diagnostic fixes, canonical readback, nullable token display, and upgrade guidance.
Independent checks passed with `GOMAXPROCS=2` and `GOFLAGS=-p=1`:

- `make check-format vet test`: Go checks and 47 UI tests.
- `go test -p 1 -race ./server/... -run 'TestUsageHistory|TestReport|TestCollector|TestHistorical' -count=1`.
- `make -j1 build verify-package-host verify-package`: host package and all five platforms.
- `go mod tidy -diff`: clean.

The 23 new Go fixtures cover replay, deletion/resume, correction, partial absence, rejected receipts, cancellation, and diagnostics.
Receipts and integration details: `/tmp/kandev-plugin-session-cost-phase-b/integration-handoff.txt` and `final-scope.txt`.
These are fixture/package results, not real Host or browser evidence.
Task 03 added trusted target dates and canonical-generation metadata after the Phase B handoff.
Phase C used those fields for bounded dated import after deletion and restart-safe saved canonical values during preparing/pending work.
Generation tokens remain opaque. Tokscale parsed message counts do not populate Kandev turns.
Phase C focused metadata/import/refresh tests, 16 new metadata test functions, full Go checks, 49 UI tests,
focused race checks, tidy, and diff checks passed. Commands and SDK hashes are in `/tmp/kandev-plugin-session-cost-phase-c/handoff.txt`.
The Phase B package archive did not contain the Phase C changes; final package/browser validation must rebuild it after integration.
At the Phase C handoff, Tokscale model-label changes were treated as ambiguous without stable source-record evidence.
Real Host selection/readback, lifecycle/uninstall retention, packaged desktop/mobile smoke, and the final reachable core pin were pending at that checkpoint.
At that checkpoint, the coordinator planned to resume the worker after the core handoffs. No complete Task 06 result was claimed.

### Earlier adapter preparation checkpoint (historical; NOT COMPLETE)

E2 prepared a packaged Host test matrix for native tokens, external cost, corrections, nullable values, currencies, action scope, and retention.
E2 added a guard that rechecks the current task/session binding in the verified workspace before canonical reads.
E2R and E2R2 prepared the unsupported-Host response for a missing history interface, unimplemented point lookup, or missing point-lookup acknowledgement.
Those cases return upgrade guidance before canonical reads or source commands. Denied and malformed responses remain fail-closed.
E2T prepared the installed-smoke revision sequence. Its live Host run had not run at that checkpoint.

The E2 preparation receipts record syntax, formatting, and whitespace checks only. E2 did not run Go tests or the full Node suite during the C2 benchmark window.
After E3a corrected the legacy Host fixture, the full `server/...` Go suite passed normally and under race. Vet, tidy, format, and all 61 Node tests also passed.
The E3a Go checks use the sibling local SDK. They do not set the final SDK pin or prove package and Host integration. Receipt: `/tmp/usage-history-luna-E3a-ready.json`.

H3/K3/J3 prepared installed checks for authorization, pruning, ownership transfer, erasure, and plugin-state privacy. These preparations were not actual Host or browser passes. J4/L3 were adapting the live smoke helper at that checkpoint.
At that checkpoint, the C3 full SQLite run appeared in progress. It later failed at page 2453 after 1,681 seconds with zero samples.
The later core `907` checkpoint stopped the C4 SQLite run before the full fixture completed. The full PostgreSQL run had not started at that checkpoint.
Current-source full-run status is in the parent plan.
E2 preparation receipts: `/tmp/usage-history-luna-E2-prep.json`, `/tmp/usage-history-luna-E2R-prep.json`, `/tmp/usage-history-luna-E2R2-prep.json`, and `/tmp/usage-history-luna-E2T-prep.json`.

### Current integration status (2026-10-30, NOT COMPLETE)

The validated core source revision is `6a4b002a22ba959dbd636f2fb42eefc31c7dafd2` on main `aebac5d5395e9983124b398198bace146d38506b`.
The SDK tree is `a9a0a09dc8b30ac02876ccc7e46eb18102079600`.
A6 replayed 97 feature commits: 96 were range-equal and one persistence-catalog change adapted automatically. No commits were dropped, and no conflicts occurred.
All 550 feature paths remain. The adaptation preserves `workspace_collaboration_grants` and every usage feature table.
Background `UpsertSessionUsage`, `ListSessionUsage`, `ListUsageCollectionTargetsV1`, `ReportUsageObservationsV1`, and `ListStateKeys` calls now use plugin-owned capability classification.
Exact task usage stays caller-bound. Capability, workspace-visibility, source-ownership, and binding checks remain active.

Candidate `f306b7081105c231f56e309e18378372b8c6472d` passed the full SDK, standalone consumer, and focused Host authorization, state-key, task-deletion, archive-borrower, native-publisher, continuation, and caller checks.
At current primary HEAD `6a4`, the integration control records an SDK race pass for three packages. The generated protobuf package has no tests.
The corrected-export run passed all seven analytics packages with an actual PostgreSQL 18.6 DSN.
It recorded 463 passing tests and subtests, with zero failures and zero skips. Three packages had tests, and four had no tests.
The initial attempt lacked the exported DSN and does not count as product evidence.

The plugin tree remains at HEAD `0fbccfb4eac3de587a80540f9858a51947062b5e`, with reviewed but uncommitted adapter and M3 security changes.
Plugin PR #10 still points to `127bc4a6e98f9121c216135b71c5e17aa8832cf1`. The new source is not pushed, merged, or released.
The `.kandev-sdk-ref` pin remains `9aecf156c65e7522a43f2a1c8310737179c25d70`, an older core revision.

The M3 security review is complete. The legacy webhook returns constant upgrade guidance, including for authenticated callers.
Metrics remain available only through the authenticated current-target workspace action. Manifest API v1 access is authenticated, and no Host version floor is guessed.
Q3 passed normal and race Go checks on the older SDK, 61 bundle tests, seven privacy-validator tests, and syntax checks for 11 Node files.
These checks do not prove the final package or actual Host integration. Receipt: `/tmp/usage-history-luna-Q3-ready.json`.

The E4 production web build and local plugin SDK consumer passed on core `90767205f97f4b8268526255b935c3626c1a07fe` and SDK tree `e9f0d45ede0390e9a31f079aa7ec88524dc700b9`.
The recorded frontend digest `b12ea9ee` belongs to that earlier source frame. Main changed the frontend and SDK, so the old build and consumer do not close current gates.
Receipt: `/tmp/usage-history-luna-E4-prebuild-ready.json`.

The five current-Host cases and older-Host compatibility matrix are prepared but unrun. All five 0.5.0 archives need the final reachable core pin and a rebuild.
Earlier v0.97/v0.86 compatibility checks and Phase B archives use older source and do not prove final compatibility.

Task 06 remains open for the published SDK pin, current plugin consumer, final build, and verification of all five archives.
Actual Host and older-Host cases also remain open.
Lifecycle and uninstall-retention evidence, packaged desktop and phone runs, and final README and companion-plan results also remain open.
The public PR and current-head CI still need updates after the implementation is pushed. No complete Task 06 result is claimed.
