---
id: 02-cost-random-preview
title: Tier cost/random ranking and read-only preview
status: done
wave: 2
depends_on: [01-ordered-tiers]
plan: plan.md
requirements:
  - REQ-AGENTS-ORDERED-TIERS-001
acceptance_criteria:
  - AC-AGENTS-ORDERED-TIERS-001.4
  - AC-AGENTS-ORDERED-TIERS-001.5
  - AC-AGENTS-ORDERED-TIERS-001.6
system_design:
  - ../../specs/agents/system-design/ordered-profile-tiers.md
legacy_specs: []
---

# Cost/random tier selection work order

Extend the existing tier owner with explicit order/cost/random and failure
direction controls, per-candidate configured cost class, and a native saved or
unsaved new-selection preview. Cost is class ordering, never invented prices.
Keep group adjacency, opaque model/window data, presence merges, existing failure
policies, durable no-revisit, explicit same-candidate retry and verified success
boundaries. Feature OFF retains the native flat path and stored metadata.

Use one pure ranking calculation for execution and preview. Read circuit
availability without a probe, generation claim or health mutation; the existing
execution path retains final probe/CAS ownership. Preview is an explicit read
with normal configuration authorization and no edit-interlock mutation. Fence
late draft/auth responses and preserve failed-save/preview drafts.

Use affected existing tests and disposable native/controller/frontend fixtures
outside collectors, including deterministic random injection and runtime/preview
agreement. No permanent tests, new ledger, poller, provider/model execution or
operating configuration. Root owns Git references and integration. RR, pace and
keep-model implementations remain original later increments.

## Results and exact verification

The source increment is implemented; root owns integration/publication. Cost
and random run within the existing tier/generation/failure-policy pipeline.
Preview's original IDs, mode, configured cost class and bounded exclusions use
the same pure ranking. A read-only availability observation precedes ranking;
only the unchanged execution check acquires an actual probe. Native binding
resolution is reused without a new credential identity, hash or account reader.
Existing route reason/cause values remain intact; preview supplies explicit
mode and eligibility fields. No provider usage API or model was called.

`POST /api/v1/agent-profiles/:id/dynamic-preview` accepts a saved profile or an
explicit dynamic draft. `POST /api/v1/agent-profiles/dynamic-preview` handles an
unsaved create draft. Both use existing organization configuration permission;
the edit interlock is intentionally absent from this read-only operation. OFF
returns before parsing a document or reading profiles. Failed/stale frontend
responses do not replace drafts or newer preview results.

`PROBE` below denotes the existing disposable `ordered-tier-modes-probe-20261015`
directory outside ordinary collectors. Go commands run from `apps/backend`:

```text
go test -trimpath -p 2 -overlay PROBE/overlay.json ./internal/agent/runtime/dynamic -run '^TestTierModesProbeCostUsesActualSelector$' -count=1 -timeout 90s -v
go test -trimpath -p 2 -overlay PROBE/overlay.json ./internal/agent/runtime/dynamic ./internal/agent/settings/controller -run '^TestTierModesProbe' -count=1 -timeout 90s -v
go test -trimpath -p 2 -overlay PROBE/overlay.json ./internal/agent/runtime/dynamic ./internal/agent/settings/handlers -run '^TestTierModesProbe(Restart|ActualPreviewHTTP)' -count=1 -timeout 90s -v
go test -trimpath -p 2 ./internal/agent/runtime/dynamic ./internal/agent/settings/controller ./internal/agent/settings/handlers -run '^(TestEngineSelectsFixedOrderAndFencesGenerations|TestEnginePreferenceKeepsRetryOnCurrentCandidate|TestEngineRetrySameKeepsTheFailedCandidate|TestEngineUsesExclusiveProbeForExpiredCircuit|TestDynamicProfileCreateAndUpdatePersistsCandidates|TestRegisterRoutesProtectsEveryStateChangingAgentSettingsRoute|TestAgentSettingsMutationsRequireOrgConfigManage)$' -count=1 -timeout 90s
golangci-lint run ./internal/agent/runtime/dynamic ./internal/agent/runtime ./internal/agent/settings/controller ./internal/agent/settings/handlers ./internal/backendapp --new-from-rev=HEAD --timeout=5m --concurrency=2
```

The first cost selector run failed on the original unsupported-mode error
(`cost-before.txt`, package 0.042s), then passed after implementation
(`cost-after.txt`). The combined native engine/controller proof passed (0.046s
and 0.100s): real selector cost choice inside its tier, eligible-only injected
random, matching deterministic preview, no preview route/probe/circuit mutation,
explicit same-candidate retry, next-tier exhaustion, stale/current success,
OFF flat traversal, and real SQLite saved/draft/presence round-trip. Mode updates
preserved opaque head/model/windows, and preview preserved saved version/rules.

The added restart and HTTP proof passed (0.031s dynamic, 0.116s handlers). A new
engine loaded the serialized native route state and continued its untried cost
peers without revisiting. This reuses the existing durable store contract rather
than claiming a live process restart. Registered native HTTP routes and the real
SQLite settings owner proved saved/create preview, no edit-interlock header,
OFF-before-invalid-body and native `org.config.manage` refusal before preview.
Raw outputs are `native-ranking-controller.txt` and `native-restart-http.txt`.
The affected existing checks passed (dynamic 0.044s, controller 0.046s, handlers
0.039s) in `existing-native-result.txt`. An initial compile caught the temporary
use of a two-return validation helper as a single-return helper; correcting that
call did not weaken validation. The final five-package lint naturally exited 0
with `0 issues.` in `go-lint.txt`. No shared Go/lint handle remains active.

Final source comparison found that the new availability observation initially
treated timestamps retained on a native Closed circuit as an open circuit.
The disposable actual-selector/preview case failed before the correction
(`closed-health-before.txt`, 0.048s). Observing the same state precedence as
native `IsOpen` fixed that case without changing or acquiring a circuit/probe.
Its focused rerun passed (`closed-health-after.txt`, 0.021s); the dynamic-only
lint delta naturally exited 0 with `0 issues.` (`go-lint-health-delta.txt`).
The other four lint packages and previously valid behavior proofs were reused:

```text
go test -trimpath -p 2 -overlay PROBE/overlay.json ./internal/agent/runtime/dynamic -run '^TestTierModesProbeReadOnlyHealthMatchesNative$' -count=1 -timeout 90s -v
golangci-lint run ./internal/agent/runtime/dynamic --new-from-rev=HEAD --timeout=5m --concurrency=2
```

Frontend commands run from `apps/web`:

```text
pnpm exec eslint components/settings/dynamic-tier-selection-controls.tsx components/settings/dynamic-tier-preview.tsx components/settings/dynamic-agent-candidate-list.tsx components/settings/dynamic-agent-profile-editor.tsx components/settings/dynamic-agent-profile-editor-state.ts hooks/domains/agents/use-dynamic-tier-preview.ts lib/api/domains/dynamic-tier-preview-api.ts --max-warnings 0
pnpm exec eslint components/settings/dynamic-tier-preview.tsx --max-warnings 0
node --max-old-space-size=8192 node_modules/typescript/bin/tsc --noEmit
pnpm run i18n:check
pnpm exec vitest run --config PROBE/vitest.config.mts -t TierModesProbe
pnpm exec vitest run --config PROBE/vitest.config.mts -t 'TierModesProbe real editor'
pnpm exec vitest run components/settings/dynamic-agent-profile-editor-state.test.ts components/settings/dynamic-agent-profile-editor-save-concurrency.test.tsx lib/api/domains/agent-profile-normalize.test.ts
```

Initial frontend lint found only a nested conditional in the preview status;
extracting the unchanged status renderer resolved it. TypeScript first caught
the missing declared editor-state version field; adding that field made the full
process-local 8 GiB compiler run pass. No persistent memory/tooling setting was
changed. i18n checks passed all real/pseudo catalogs, placeholders and copy rules
(12707 English entries; 507 existing orphan warnings). Existing generators
produced traditional-Chinese/pseudo values; unrelated generated edits were removed.

The disposable frontend fixture renders the actual DynamicAgentProfileEditor,
native Save coordinator and new preview HTTP consumer. Its first configuration
merged the existing Vitest projects instead of replacing their array; that
collection was interrupted and counted as no pass. Empty projects were rejected
by Vitest. The final disposable configuration explicitly owns one project/file
under the ordinarily excluded node_modules cache and does not change the native
runner. Its first focused run passed stale/failed/create-draft preview and OFF
tests but failed the Save assertion: the native action uses an absolute API base,
while the fixture compared a relative URL. Matching its exact path suffix fixed
only that fixture. The narrowed Save case then passed, verifying actual PATCH
mode/cost fields, preserved opaque windows, and the subsequent preview body.
The other two valid cases were retained. Outputs are `frontend-focused-result.txt`
(2 pass, 1 fixture failure) and `frontend-save-final.txt` (1 pass, 2 unselected).
The existing three frontend files passed all 37 tests. No permanent test or
operating server was added and no browser/installed-provider acceptance is claimed.

The existing PR documentation validator consumed the actual 30 changed paths and
five document contents: `status: covered`, `ok: true`, `errors: []`
(`doc-coverage.json`). Existing specification/catalog checks and `git diff --check`
also passed. No new validation framework was added.

Keep these valid outputs and disposable source for root readback; repeat only
after related source/input changes or new failure evidence. RR, pace/window
inputs and keep-model persistence/idle-boundary behavior remain original work.

## Caption correction after rendered review

The feature-enabled candidate description now explains saved tier order,
configured selection within each tier, and the existing error-action boundary.
The feature-OFF branch keeps the original `dynamicCandidatesDescription` text;
the new `dynamicTierCandidatesDescription` key is complete in all eight catalogs.
Native `i18n:zh-hant` and `i18n:pseudo` generators supplied derived translations;
unrelated generated values were restored while preserving prior catalog edits.
No selection logic, flags or other contract changed.

Affected-file ESLint exited 0 (`caption-eslint.txt`); `pnpm run i18n:check`
exited 0 with 12708 English entries, all catalogs complete/pseudo synchronized
and the same 507 orphan warnings (`caption-i18n.txt`). Exact commands from
`apps/web` were `pnpm exec eslint components/settings/dynamic-agent-candidate-list.tsx --max-warnings 0`,
`pnpm run i18n:zh-hant`, `pnpm run i18n:pseudo`, and `pnpm run i18n:check`.

The existing native editor/browser fixture ran only caption checks using
`node PROBE/caption-browser.cjs`: desktop cost and phone random display the new
description, while feature OFF displays the exact original description.
`caption-browser-result.json` reports success, no page errors/external requests,
and only synthetic existing logo GETs. Supplemental captures are
`browser-desktop-caption-corrected.png` (1440x1100) and
`browser-phone-caption-corrected.png` (390x844); both were visually inspected.
The earlier four valid mode/preview PNGs remain intact. Existing 37+3 frontend,
native behavior and geometry proofs were not rerun. Browser handles closed;
the disposable Vite server was stopped and port 37447 had no listener.
