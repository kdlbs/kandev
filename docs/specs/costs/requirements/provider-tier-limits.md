---
status: active
system: costs
created: 2026-10-13
owners:
  - kandev
---

# Provider account tier limits

## Overview

Free inference, flat-fee subscriptions, and pay-as-you-go inference have separate account budgets. This extends [subscription usage tracking](subscription-usage.md); dynamic routing consumes the measurements without changing candidate order.

## Requirements

### REQ-COSTS-TIER-LIMITS-001: Account and tier admission

- **AC-COSTS-TIER-LIMITS-001.1:** Each provider account shall have separate free, subscription, and paid configurations. All support concurrent turns and requests/minute. Free adds daily requests and reset; subscriptions add rolling-5h/week/month budgets with unit/reset; paid adds optional daily/monthly spend caps. Metadata shall select the tier where known, with a per-profile owner override for ambiguity.
- **AC-COSTS-TIER-LIMITS-001.2:** At a new turn, dynamic routing shall skip or defer unavailable candidates without changing candidate order, interrupting admitted turns, choosing models for the owner, or changing the 60-minute idle timeout. Existing global ceilings and selection modes remain.
- **AC-COSTS-TIER-LIMITS-001.3:** Settings/API shall show defaults, learned values, and owner overrides, including unknown values. Desktop shall show Free/Subscription/Paid columns; phone shall edit one focused tier with shared drafts and the same save outcome.
- **AC-COSTS-TIER-LIMITS-001.4:** Classified errors shall record requests, turns, tokens, and cost in the account/tier windows. Unknown limits shall learn with a small admission reserve. Owner overrides take priority. Existing Claude/Codex, Go, and DevPass windows and recovery behavior shall remain available through the subscription layer.
- **AC-COSTS-TIER-LIMITS-001.5:** OpenRouter free rate errors shall refresh the same account's key endpoint: positive daily remaining means a minute block; zero means the daily reset. Unavailable evidence preserves lane 8's existing ladder.
- **AC-COSTS-TIER-LIMITS-001.6:** UI/docs shall distinguish provider measurements and local request estimates. Credentials shall remain inside existing resolution paths and never enter responses.

### REQ-COSTS-TIER-LIMITS-002: Account, model, and shared-group scopes

- **AC-COSTS-TIER-LIMITS-002.1:** A free tier shall support three request scopes: account-wide (the existing daily allowance), per-model, and a shared group of models with one combined allowance. A scope names provider model IDs; every applicable scope shall be checked for a model.
- **AC-COSTS-TIER-LIMITS-002.2:** Each scope value shall resolve as owner value, then learned value, then catalog default. Catalog defaults shall keep their provenance (published, measured, or approximate) and shall be data rather than routing code. Initial measured values: OpenRouter high-balance free account 1000/day; OpenCode Zen `nemotron-3-ultra-free` 1000/day, `step-5-preview-free` 1000/day, `longcat-2.5-preview-free` about 120/day, and `big-pickle` + `muse-spark-1.3-contributor-free` + `space-bunny-free` about 500/day combined; all reset at 00:00 UTC.
- **AC-COSTS-TIER-LIMITS-002.3:** At a new turn, dynamic routing shall skip or defer a candidate whose applicable scope is exhausted until that scope resets, under REQ-COSTS-TIER-LIMITS-001.2's order, interruption, and model-choice constraints.
- **AC-COSTS-TIER-LIMITS-002.4:** A classified per-model daily error shall replace the learned value of the scope that covers the model with the requests counted in that scope so far. An account-wide daily error keeps REQ-COSTS-TIER-LIMITS-001.4 learning, because the provider key counter already measures that allowance. Owner values are never replaced. Learning shall use naturally occurring errors only.
- **AC-COSTS-TIER-LIMITS-002.5:** Classified per-model daily errors from two or more ungrouped models of one account within the same minute shall propose a provisional learned group. The settings card shall mark it provisional; an owner value of zero disables it.
- **AC-COSTS-TIER-LIMITS-002.6:** The existing provider-limit card shall show and edit scopes in each account row with their source and provenance, on desktop and phone, with the existing save path, permissions, and locale catalogs.

## Exclusions

Production writes, runtime builds/swaps, new permission gates, a new usage ingestion pipeline, route reordering, model selection changes, synthetic quota exhaustion, recurring provider probes, and permanent test infrastructure are outside this change.
