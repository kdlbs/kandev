---
status: draft
created: 2026-04-28
owner: cfl
---

# Subscription Usage Tracking

## Why

The Office cost dashboard shows `$0` for every agent running on a subscription plan (Claude Max, Codex subscription). This is because subscriptions have no per-token billing — `cost_cents` is always zero, so budget gauges are flat and cost explorer data is meaningless for those agents.

The problem has three layers:

1. **Visibility**: subscription users have no feedback on how intensively they are using their plan quota. They cannot see whether they are at 5% or 95% of their 5-hour rate window.
2. **Budget enforcement**: the existing `BudgetPolicy` / pre-execution budget check machinery is bypassed entirely for subscription agents because there is no spend to check against. Heavy usage just continues unchecked.
3. **Smart scheduling**: the wakeup scheduler launches agents without knowing whether the underlying provider API will immediately return a rate-limit error, wasting the retry budget and introducing latency.

Subscription usage tracking closes all three gaps by fetching utilization data directly from provider APIs, surfacing it in the dashboard alongside an estimated-dollar equivalent derived from token counts, and feeding it into the scheduler so over-quota agents are deferred rather than launched into an immediate failure.

## What

### Billing type detection on agent profiles

Each `AgentProfile` gains a `billing_type` field with two values:
- `api_key` — the agent authenticates with an API key (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, etc.). Dollar costs are calculated from token counts as today.
- `subscription` — the agent authenticates via an OAuth flow or subscription credential (Claude OAuth token in `~/.claude/.credentials.json`, Codex subscription token in `~/.config/codex/auth.json`). For these agents, utilization percentage is the primary metric.

Billing type is **auto-detected** from the credential configuration on the `AgentProfile` rather than being a user-set field:
- Claude profiles: if `CLAUDE_CODE_OAUTH_TOKEN` is in the credential set, or `~/.claude/.credentials.json` exists and contains `claudeAiOauth`, billing type is `subscription`. Otherwise `api_key`.
- Codex profiles: if `~/.config/codex/auth.json` exists with a subscription token, billing type is `subscription`. Otherwise `api_key`.
- All other agents: `api_key`.

The backend computes billing type at profile read time (not stored in DB) and includes it in the `AgentProfile` response so the frontend can choose the correct display mode without business logic on the client.

### Provider usage client

A new `internal/agent/usage` package exposes a `ProviderUsageClient` interface:

```go
type UtilizationWindow struct {
    Label          string    // e.g. "5-hour", "7-day"
    UtilizationPct float64   // 0-100
    ResetAt        time.Time // when this window resets
}

type ProviderUsage struct {
    Provider  string              // "anthropic", "openai"
    Windows   []UtilizationWindow
    FetchedAt time.Time
}

type ProviderUsageClient interface {
    FetchUsage(ctx context.Context) (*ProviderUsage, error)
}
```

Two implementations:

**`ClaudeUsageClient`**
- Reads `access_token` from `~/.claude/.credentials.json` (`claudeAiOauth.accessToken`).
- Calls `GET https://api.anthropic.com/api/oauth/usage` with `Authorization: Bearer <token>`.
- Parses `five_hour.utilization` and `seven_day.utilization` from the response body.
- If the token is expired (`expiresAt` is in the past), refreshes it via `POST https://platform.claude.com/v1/oauth/token` before calling the usage endpoint, then writes the new token back to the credentials file.
- Returns two `UtilizationWindow` entries: `five_hour` and `seven_day`, with `ResetAt` computed from the window start + window duration.

**`CodexUsageClient`**
- Reads the bearer token from `~/.config/codex/auth.json`.
- Calls `GET https://chatgpt.com/backend-api/wham/usage` with `Authorization: Bearer <token>`.
- Reads `x-codex-primary-used-percent` and `x-codex-secondary-used-percent` response headers.
- Returns two `UtilizationWindow` entries: `primary` and `secondary`.

### Utilization cache

A `UsageCache` sits in front of both clients:
- Per-agent cache entry keyed on `(provider, credential_hash)`.
- Cache TTL: 5 minutes. Usage data is not real-time — polling more than once per 5 minutes is wasteful and may itself count against quotas.
- Cache is invalidated on demand when a scheduler deferral decision needs fresh data.
- The cache is in-memory only (not persisted to SQLite) — a backend restart causes a single fresh fetch per agent, which is acceptable.

The usage service exposes:
```go
func (s *UsageService) GetUsage(ctx context.Context, profileID string) (*ProviderUsage, error)
func (s *UsageService) IsPotentiallyRateLimited(ctx context.Context, profileID string) (bool, time.Time, error)
// Returns (limited, resetAt, err). resetAt is zero if not limited.
```

`IsPotentiallyRateLimited` returns `true` when any utilization window is ≥ 90%.

### Estimated dollar equivalent for subscription agents

For subscription agents, `cost_cents` from the cost events table is zero (no per-token billing). However, token counts are still recorded. The cost explorer augments subscription agent rows with an **estimated equivalent cost** computed from token counts × LiteLLM community pricing.

LiteLLM pricing table:
- Fetched from `https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json` at startup and cached in memory with a 24-hour refresh interval.
- The existing hardcoded `pricingTable` in `internal/office/costs/pricing.go` is replaced by the LiteLLM-sourced table, with the current hardcoded values serving as a fallback if the fetch fails.
- Pricing resolution: LiteLLM table first, then hardcoded fallback, then `cost_cents = 0` with "pricing unavailable" display.

Estimated cost is displayed as `~$X.XX est.` in the UI, visually distinct from actual API costs which show as `$X.XX`. The distinction matters because estimated costs for subscription users do not reflect real money — only quota consumption.

### Cost explorer changes

The `/office/workspace/costs` page is updated for dual-mode display:

**API key agents** — display unchanged:
- Dollar spend in `$X.XX` format.
- Budget utilization gauge if a `BudgetPolicy` exists.

**Subscription agents** — new display:
- Utilization progress bars per window: `5h window: [████░░░] 62%`, `7d window: [██░░░░░] 28%`.
- Window reset time shown on hover: "Resets in 2h 14m".
- Estimated equivalent cost below the gauge: `~$3.40 est.` (derived from token counts).
- A subtle "subscription" badge on the agent row to distinguish from API key agents.
- Budget policies are still available for subscription agents but operate on estimated-cost thresholds, with a tooltip explaining they are estimates.

The cost overview summary bar gains a new entry: **"Subscription quota"** showing the highest utilization across all subscription agents. If any subscription agent is ≥80%, the card turns amber; ≥90% turns red.

### Smart scheduling: subscription-aware deferral

The scheduler's `processWakeup` method gains a **utilization check** step inserted between the existing `checkBudget` step and the `resolveExecutorForWakeup` step:

```
Guard → Checkout → Budget check → Utilization check → Executor resolve → Launch
```

The utilization check:
1. Calls `UsageService.IsPotentiallyRateLimited(ctx, agent.AgentProfileID)`.
2. If the agent is a `subscription` billing type AND any window is ≥ 90%:
   - Mark the wakeup `status = "deferred"` (new status value) with `deferred_until = resetAt`.
   - Release the task checkout.
   - Log activity: `wakeup_utilization_deferred` with `{agent, window, utilization_pct, deferred_until}`.
3. A new scheduler pass runs every 30 seconds to re-queue deferred wakeups whose `deferred_until` has passed (sets status back to `queued`).
4. For `api_key` agents, the utilization check is skipped entirely — it only applies to subscription accounts where provider rate limits are enforced per-subscription rather than per-request.

The deferral threshold (90%) is configurable via a workspace setting `subscription_deferral_threshold_pct` (default 90).

### Inbox notification for utilization spikes

When a subscription agent's utilization crosses 80% for any window, an inbox item is created (once per window per crossing, not on every check):
- Title: "Claude subscription quota at 82% (5-hour window)"
- Body: "Agent [name] is near its rate limit. Wakeups will be deferred automatically at 90%. Resets in 3h 20m."

Crossing detection uses a persistent per-agent watermark stored in SQLite to avoid flooding the inbox on repeated checks.

### Agent detail page

The agent detail page at `/office/agents/[id]` Overview tab gains a **Quota** section (only shown for subscription billing type):
- Progress bar per utilization window with percentage and reset time.
- A note if the agent is currently deferred.

### Per-agent billing type in API responses

The `AgentInstance` response (from `/api/v1/office/workspaces/:id/agents`) gains a read-only `billing_type` field (`"api_key"` | `"subscription"`) and a `utilization` field (null for `api_key` agents, `ProviderUsage` struct for `subscription` agents). The frontend reads these and selects the correct display mode without any client-side provider detection logic.

## Scenarios

- **GIVEN** a Claude agent configured with OAuth credentials, **WHEN** the cost explorer loads agent data, **THEN** the agent row shows two utilization progress bars (`5h` and `7d`), an estimated cost marked as `~$X.XX est.`, and a "subscription" badge. No dollar spend column is shown.

- **GIVEN** an agent using an Anthropic API key, **WHEN** the cost explorer loads, **THEN** the agent row shows `$X.XX` actual spend with a budget gauge if a policy exists. No utilization bars are shown.

- **GIVEN** a Claude subscription agent whose 5-hour window is at 91% utilization, **WHEN** the scheduler attempts to process a queued wakeup for that agent, **THEN** the wakeup is deferred (not failed) until the window resets. An activity log entry records the deferral. When the window resets, the wakeup is re-queued and processed normally.

- **GIVEN** a Claude OAuth token that has expired, **WHEN** the usage client fetches utilization, **THEN** the token is refreshed via the OAuth refresh endpoint, the new token is written back to `~/.claude/.credentials.json`, and the utilization fetch succeeds. The caller is unaware the refresh happened.

- **GIVEN** a subscription agent crossing 80% utilization on the 7-day window, **WHEN** the usage cache is updated, **THEN** an inbox notification is created once. Subsequent checks that still show 80%+ do not create duplicate notifications.

- **GIVEN** the LiteLLM pricing fetch fails at startup, **WHEN** a cost event is recorded, **THEN** the system falls back to the hardcoded pricing table. No errors are surfaced to users.

## Out of scope

- Real-time (sub-second) utilization polling — provider APIs are not real-time and polling aggressively would itself consume quota.
- Utilization tracking for providers without a public usage API (Gemini, Copilot, Amp, etc.) — those agents remain API key billing type with dollar costs only.
- Billing integration or actual charge reconciliation — all costs remain estimates.
- Per-model quota breakdown for Codex — the Codex usage API only returns two aggregate percentages.
- Automatic token refresh for Codex — the Codex auth.json token refresh flow is not publicly documented; if the token is expired the fetch returns an error and the check is skipped (fail-open).
