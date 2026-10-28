---
id: "06-session-cost-adapter"
title: "Adopt the generic contract in Session Cost"
status: pending
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

Pending.
