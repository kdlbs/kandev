---
status: current
system: costs
requirements:
  - REQ-COSTS-TIER-LIMITS-001
  - REQ-COSTS-TIER-LIMITS-002
---

# Provider account tier limits design

## Requirement mapping and ownership

| Requirement               | Design                                                             |
| ------------------------- | ------------------------------------------------------------------ |
| REQ-COSTS-TIER-LIMITS-001 | Configuration, accounting, admission, learning, and settings below |
| REQ-COSTS-TIER-LIMITS-002 | Account, model, and group scopes below                             |

Costs owns limit facts and budgets. Reuse `usageProviderAdapter` account bindings, the task usage ledger, and `usage_limit_observations`. Keep the existing dynamic route order and failure-circuit policy; this port adds a selection-time allowance check rather than a ranking layer. Internal account keys stay inside the backend; settings uses existing safe account identities.

## Configuration and defaults

`provider_account_limits` in the task repository stores `account_key`, `config_json`, `learned_json`, and `updated_at`. Separate owner/learned columns prevent learning from replacing owner edits. JSON has independent `free`, `subscription`, and `paid` settings, `profile_tiers`, optional purchased-credit metadata, and a confirmed subscription plan. Precedence is owner override, learned allowance, then derived default. Nil is unknown/inherit; zero disables the local cap. Budgets carry window, unit, reset, optional anchor/time zone, and model-class scope.

| Provider           | Free                                                       | Subscription                                                                      | Paid                                                    |
| ------------------ | ---------------------------------------------------------- | --------------------------------------------------------------------------------- | ------------------------------------------------------- |
| OpenRouter         | 20 RPM; 1,000/day measured installation default; UTC midnight | Unknown                                                                           | Owner configured                                        |
| LLM Gateway        | 20 RPM/model measured installation default                | DevPass: 120 RPM, 50 concurrent turns; measured installation default              | Tier 0: 600 RPM, 100 turns, $25 UTC day, $250 UTC month |
| OpenCode Zen       | UTC daily reset; unknown allowance is learned              | Unknown                                                                           | Owner configured                                        |
| OpenCode Go        | Unknown                                                    | Reported windows; confirmed base Go reference $12/rolling 5h, $30/week, $60/month | Owner configured                                        |
| Claude/Codex       | Unknown                                                    | Existing reported 5h/weekly percentages and resets                                | Owner configured                                        |
| Antigravity/Gemini | Metadata dependent                                         | Existing reported plan quotas                                                     | Owner configured                                        |

Gateway concurrency is requests; admission concurrency is task turns, a conservative proxy. The short request window preserves 5/10m rather than rounding it into incorrect RPM. Unknown DevPass dollar/token budgets and Go Plus budgets stay empty until measured, learned, or overridden. The historical Go usage reader's fixed `plan: go` is not evidence of the base plan, so dollar references require explicit confirmed plan metadata. Published current references are [LLM Gateway](https://docs.llmgateway.io/resources/rate-limits) and [OpenRouter](https://openrouter.ai/docs/api_reference/limits); operator-supplied base Go figures remain a separately identified reference.

## Accounting and admission

Attributed task usage rows provide token/cost totals and response-count lower bounds. A distinct native response counts once; summary-only streams are estimated. Existing agent-stream assistant/response identities supplement in-flight counts. The larger stream/ledger count avoids adding two representations. Missing events and assistant segmentation can undercount or overcount actual HTTP calls. Provider daily counters and subscription percentages remain separate measurements.

Account grouping filters out other billing tiers. Starting route claims and running task sessions count once; idle retained sessions do not occupy turn slots. In this port the allowance check runs inside the engine state mutex during selection: an exhausted candidate is skipped for that decision while the saved order, running turns, and the existing circuit/account-block behavior stay unchanged. There is no separate admission mutex, final prompt claim recheck, or lifecycle claim revalidation in this implementation; those are not part of this change. Running turns continue; admission does not throttle individual CLI HTTP requests. When execution-owned usage is active, an account-specific allowance, reset, or learning observation requires a matching provider/account/organization/model; an unverified source does not borrow the host account and the existing fail-open selection behavior remains. This leaves account limits unavailable for execution sources that cannot yet establish that match, including unsupported or remote sources. A limit-created long reset suspends the provider on its credential binding; a model-daily allowance suspends only that model. Provider-reported account resets, such as the Claude/Codex session or weekly windows, keep the upstream binding-wide suspension. A narrow suspension does not open the credential binding, so other providers on the same agent remain selectable. When `features.providerUsageLimits` is off, `GET/PUT /api/v1/provider-limits` returns 503 and the settings card is hidden.

## Learning and recovery

Extend `usage_limit_observations` through idempotent migrations with tier/dimension, minute/day request estimates, active turns, and `window_usage_json` containing requests/turns/tokens/subcent costs for 5h/day/week/month. Daily snapshots use configured daily reset; unknown long-window snapshots are recorded trailing estimates. A classified dimension learns an undisclosed allowance. An ambiguous subscription error gets a provisional rolling-5h token budget marked `window_estimated`, preserving the original `unknown` dimension and the existing retry ladder. It does not claim a weekly/monthly limit. The 5% reserve applies only to learned values, not published defaults or owner replacements.

Claude and Codex usage clients remain measurement sources. OpenRouter 1,000/day and LLM Gateway free 20 RPM are installation defaults with measured provenance, so their display, edit, and admission effects do not depend on a usage client. DevPass 120 RPM and 50 concurrent turns are the Subscription-tier defaults; because this base has no DevPass plan observation, the owner maps the DevPass profile to Subscription once in profile tier mapping, and the card shows that mapping. The OpenRouter key endpoint is read on a free rate error; the Go/DevPass response-header measurement remains a follow-up. Learned, owner, and catalog-default values still work from classified observations. `dynamicLimitCalendar` preserves the operator-entered monthly reset and the existing retry ladder; a paid-model quota failure without a provider reset uses that monthly reset as its retry hint.

The OpenRouter key endpoint is read once after a free rate error: positive daily remaining gives a minute block, zero gives the daily reset, and missing or unavailable evidence leaves the existing retry ladder unchanged. A free-model daily exhaustion is model-scoped so a paid OpenRouter model on the same credential binding is not blocked until 00:00 UTC.

## Account, model, and group scopes

The account-wide scope is the existing tier `requests_per_day`; its defaults come from the installed catalog. The OpenRouter key endpoint is read on a free rate error. `TierLimits.scopes` adds model and group scopes, each with a stable `id`, `kind` (`model` or `group`), provider model IDs without the provider prefix, `requests_per_day`, `daily_reset`, `provenance`, and learned markers (`learned`, `provisional`, `learned_at`). A model may have its own scope and one group; admission checks both. `MergeTierLimits` merges scopes by `id` field by field, so owner, learned, and default columns keep the existing precedence. Nil inherits; an owner zero disables a default or provisional scope without deleting it.

Default scopes and account-wide tier values come from an embedded limit catalog data file keyed by provider, tier, and optional plan, separate from routing code. Each entry carries its values and provenance: `published`, `measured`, or `approximate`. The initial values are OpenRouter 1,000/day at 00:00 UTC, LLM Gateway 20 RPM free, DevPass Subscription 120 RPM with 50 turns, and the paid day/month budgets; `DefaultTierLimits` reads them from the catalog. DevPass admission still requires the owner's profile-tier mapping because the base has no plan observation.

Scope usage counts requests of account profiles whose model is in the scope since the scope's daily reset, with the existing ledger/in-flight maximum used for the account count. `LimitUsage.scopes` carries those counts for admission and settings. `Check` evaluates the scopes covering the candidate's model after the tier checks and returns the existing `limit_daily` reason with the scope reset, so route order and selection admission are unchanged; limit checks run at selection inside the engine mutex.

Learning splits the classified daily dimension. `CauseFreeAccountDailyLimit` keeps `daily` and the existing fill-unknown account learning. `CauseFreeDailyLimit` becomes `model_daily`: the observed model's group scope, else its model scope, else a new learned model scope, receives the counted scope requests. Unlike unknown tier values, a scope's learned value replaces an earlier learned value, because a lower provider limit must take effect. Owner values are untouched and default values remain visible. The existing 5% learned reserve applies. A count of zero learns nothing. Admission stops before a provider block, so learning cannot raise a value; the owner value covers increases.

An observation's execution profile identifies the observed model, so no model column is added; profile model edits within one minute are negligible. A `model_daily` observation for a model without an owner- or catalog-defined scope looks for other `model_daily` observations on the same account within 60 seconds whose models also lack one. A configured model scope is evidence of an individual allowance, so those models never group. An owner value or reset entered on a learned scope counts as configured; its learned definition is kept so the owner entry keeps covering its models. A match creates a provisional learned group, or extends the provisional group one of them already belongs to under its existing ID. Models in two different provisional groups are ambiguous and change nothing. The group learns the members' combined counted requests and replaces their learned model scopes. Enrichment and insertion of an observation share the learning lock, so a near-simultaneous block sees the earlier one. Coincident independent blocks can form a false group; the provisional marker, the group's combined value, and the owner zero keep that visible and reversible. No probe or synthetic request establishes a group.

## Settings and validation

Extend the existing provider-limit API/card. Desktop has three columns; phone uses canonical responsive breakpoints and one selected tier. Shared draft/save logic retains edits across tier/viewport changes; the page owns scrolling, with 44px touch and 28px desktop controls. Defaults, learned values, owner overrides, and provisional-window uncertainty remain visible. Existing provider-wide manual blocks and release controls remain available. The tier editor labels the daily allowance as account-wide and lists model and group scopes beneath it with today's count, reset, source, provenance, and provisional marker. The owner edits a scope value or adds a model or group scope from the account's profile models through the same draft and save request. Copy uses all locale catalogs.

Source-adjacent companion units and existing focused tests cover allowances, resets, learning, explicit provider counters, admission ordering, persistence, and partial save responses. Disposable component rendering uses synthetic data without a production instance or provider call. Current verification results and limitations belong in the linked plan/work order.
