---
spec: office-costs
created: 2026-05-12
---

# Office Costs — Implementation Notes (Wave Wedge)

## Probe corpus

Real ACP traffic captured at `/tmp/acp-probe-{claude-acp,opencode-acp,codex-acp,gemini,auggie,copilot}.jsonl`. Every distinct `modelId` observed across the six CLIs is pinned by the table-driven test in `internal/office/costs/modelsdev/normalize_test.go`. New CLIs or re-released probes must extend that corpus.

## Per-CLI usage extraction (Wave 2)

| CLI            | Usage source                                   | Cost source                       | Normalize Strategy |
|----------------|------------------------------------------------|-----------------------------------|--------------------|
| claude-acp     | `result.usage` (typed via ACP SDK)             | `usage_update.cost.amount` (USD)  | StrategySkip (alias) → Layer A only |
| opencode-acp   | `result.usage` (typed; has `thoughtTokens`)    | `usage_update.cost` (often 0 BYOK)| StrategyLookup after route strip |
| gemini         | `result._meta.quota.token_count[.model_usage]` | none                              | StrategyLookup |
| codex-acp      | none per-turn — adapter synthesises from `usage_update.used` delta | none | StrategyLookup after /effort strip |
| auggie         | none                                           | none                              | n/a — unsupported |
| copilot-acp    | none — `/usage` slash-command only             | `_meta.copilotUsage="1x"` is a billing multiplier, not a count | n/a — unsupported |

`adapter.go` consumes the per-session usage tracker on prompt-complete: if the typed `resp.Usage` is empty (codex-acp), we emit a synthesised `PromptUsage{InputTokens=delta, Estimated=true}`. claude-acp's `usage_update.cost.amount` is attached to the typed-usage frame as `ProviderReportedCostSubcents` so Layer A wins downstream regardless of whether `result.usage` was also present.

## Three-layer cost lookup (Waves 3, 4, 6)

The subscriber at `internal/office/service/event_subscribers.go:handlePromptUsage` resolves cost in order:

1. **Layer A** — `data.Usage.ProviderReportedCostSubcents > 0` → store verbatim, skip pricing.
2. **Layer B** — `shared.PricingLookup.LookupForModel(ctx, model)` (the `modelsdev` Client). Misses cascade to estimated.
3. **Estimated** — record `cost_subcents=0, estimated=true`. No static fallback map.

The hardcoded `pricingTable` from the draft plan was dropped entirely because claude-acp's logical aliases (`default`/`sonnet`/`haiku`/`opus`) have no real model id to key on, and the static rates rotted within a quarter of being written. The `modelsdev.Client` is lazy on first `Lookup` — workspaces running only claude-acp never trigger an HTTP fetch because Layer A handles every event before the lookup is reached.

## Known limitations

- **opencode BYOK undercount.** `github-copilot/<model>` and `openai/<model>` routes are stripped before the models.dev lookup, so the recorded cost reflects list price of the underlying model, not what the BYOK wrapper actually billed. When `usage_update.cost` is reported (uncommon on BYOK) Layer A handles it correctly.
- **codex-acp imprecision.** codex emits no input/output split, only a cumulative `used` counter. The synthesised delta is recorded as InputTokens with `Estimated=true`. The resulting cost is off (no output split) and flagged in the row's `estimated` column. Budget totals count the row at face value per spec.
- **auggie / copilot-acp untracked.** ACP frames carry no token telemetry from these CLIs. Real coverage needs provider-API polling (auggie) or `/usage` slash-command scraping (copilot). Out of scope this iteration.
- **claude-acp model column is a logical alias.** Don't try to filter/group by it for pricing analytics — the same alias maps to different real models over time as claude-code flips defaults.

## models.dev schema assumption

`internal/office/costs/modelsdev/client.go` parses `https://models.dev/api.json` as `map[provider]{models: map[modelId]{cost: {input, output, cache_read, cache_write}}}` (dollars-per-million floats). If the live schema differs at PR time, adjust `datasetProvider` / `datasetEntry` + the test fixture in `client_test.go`. The Client tolerates schema drift gracefully (parse failure → miss → estimated), so a wrong shape degrades quietly rather than crashing.

## Manual verification

Re-run `acp-debug` against each installed CLI after the adapter change and confirm the resulting JSONL has populated usage frames. claude-acp is the only CLI exercised on every install; codex-acp / opencode-acp / gemini are nice-to-haves. The unit tests in `usage_test.go` + `normalize_test.go` are the unit-of-record contract.

## Out of scope for this wedge

- Cost explorer UI (the by-time line chart, drill-into-agent, etc.).
- Budget CRUD UI changes.
- Copilot/Amp adapter coverage.
- models.dev sync of non-pricing metadata (model name, family, deprecation flags).
- Per-turn cost limits / cost forecasting (spec is per-period).
- Surfacing the estimated share on the dashboard (the row carries the flag; UI exposure is the cost explorer follow-up).
