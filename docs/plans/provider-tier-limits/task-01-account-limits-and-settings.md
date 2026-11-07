---
status: in progress
---

# Account limits and settings

## Outcome and scope

Implement configuration, selection-time allowance admission, classified observations, and responsive settings together. The OpenRouter key endpoint is read on a free rate error; the Go/DevPass response-header measurement remains a follow-up. Owned areas are `agent/usage`, `agent/runtime/dynamic`, the provider-limit controller, `backendapp` composition, task SQLite persistence, the existing settings card/API/hooks, locale catalogs, and public agent documentation.

Excluded: route reordering, model choice, idle-timeout changes, production writes, runtime builds/swaps, new approval gates, new integration/E2E/CI or recurring test infrastructure, and unrelated Windows failures. Depends on [the plan](plan.md); the original execution used `origin/main 07fa2d5e7`, and this port is based on `85942d927e40` (P2 usage base).

## Contract references

- `REQ-COSTS-TIER-LIMITS-001`.
- `AC-COSTS-TIER-LIMITS-001.1`, `AC-COSTS-TIER-LIMITS-001.2`, `AC-COSTS-TIER-LIMITS-001.3`, `AC-COSTS-TIER-LIMITS-001.4`, `AC-COSTS-TIER-LIMITS-001.5`, `AC-COSTS-TIER-LIMITS-001.6`.
- [System design](../../specs/costs/system-design/provider-tier-limits.md).

OpenRouter 1000/day and LLM Gateway free 20/min are measured installation defaults, so their display, edit, and admission effects do not depend on a usage client. DevPass 120/min and 50 concurrent turns are the Subscription-tier defaults; because this base has no DevPass plan observation, the owner maps the DevPass profile to Subscription once in profile tier mapping. Without that mapping the profile keeps the paid default. The OpenRouter key endpoint is read on a free rate error: positive daily remaining gives a minute block, zero gives the daily reset, and unavailable evidence keeps the existing ladder. The Go/DevPass response-header measurement remains a follow-up. The implemented admission check runs at selection inside the engine mutex; claim-time recheck and resource-wait restart scheduling are not part of this change.

## Acceptance

1. Account/tier budgets persist, display sources, and change admission without changing order or interrupting running work.
2. Classified observations learn unknown limits from the recorded ledger. Unavailable evidence preserves the existing retry ladder. OpenRouter 1000/day and LLM Gateway free 20/min are measured installation defaults; DevPass 120/min applies after the profile is mapped to Subscription. The OpenRouter key endpoint is read on a free rate error; the Go/DevPass response-header measurement remains a follow-up.
3. Desktop and phone edit/save the same settings through the normal settings entry point.

## ASCII UI preview

UI-01 from [the plan](plan.md#ascii-ui-preview): desktop has three tier columns; phone has a visible tier navigator and one editor, followed by profile mapping and Save/status. The page owns scrolling and source values stay visible beside each input.

## Verification

Run only the affected package/file checks. Companion units cover defaults/owner precedence, learned persistence, scope/group evaluation, and selection-time skip. Existing cases cover monthly resets and block-until behavior.

- Focused go test in usage, dynamic, SQLite, backendapp, and provider-limit handlers with explicit provider-limit filters.
- Build changed Go packages; `scripts/lint-go-changed` for changed files.
- Focused provider-settings Vitest, web typecheck (8 GiB heap), changed-file ESLint, and `pnpm run i18n:ratchet`.
- Existing spec catalog/lint; normal commit hooks without bypass.

## Current execution results

Base `origin/main` 07fa2d5e7. The port reuses the provider-limit data model and UI from the fork line (#53/#302/#338/#341) on the existing circuits/selection engine; it does not port the fork dynamic-routing v2.

Implemented: default-off `features.providerUsageLimits`; `GET/PUT /api/v1/provider-limits`; `provider_account_limits`, `dynamic_provider_limits`, `usage_limit_observations`; three scope levels with defaults/learned/owner precedence; measured catalog defaults; monthly reset and block-until candidate suspension; selection-time skip that preserves order. The operator monthly reset is used as the retry hint for a paid-model quota failure without a provider reset.

Verification: `go build ./internal/...`; focused tests in `agent/usage`, `agent/runtime/dynamic`, `task/repository/sqlite`, `backendapp`, `agent/settings/handlers`, `runtimeflags`, and `profiles` passed; `lint-go-changed` reported 0 issues; web Vitest 9 cases, `tsc --noEmit`, changed-file ESLint, and `i18n:ratchet` passed; spec validate/lint passed.

Required values: OpenRouter 1000/day at 00:00 UTC and LLM Gateway free models 20/minute are measured installation defaults; DevPass 120/minute with 50 concurrent turns is applied after the profile is mapped to Subscription in profile tier mapping (readback required because the base has no DevPass plan observation). The OpenRouter key endpoint is read on a free rate error (positive remaining gives a minute block, zero gives the daily reset, unavailable evidence keeps the existing ladder); the Go/DevPass response-header measurement remains a follow-up. Claim-time recheck and resource-wait restart scheduling are not part of this change; selection-time skip is the implemented behavior. The repository-wide `i18n:check` still reports the base line's orphan catalog entries and is not used as a pass signal.

Readback on the installed card (operator-owned settings writes are outside this change):

- The DevPass profile is mapped to Subscription (record who and when).
- The same account's Subscription effective values are 120/minute and 50 turns, with default/measured provenance.
- Credit-key profiles on the same account remain paid.
- No artificial limit exhaustion: the admission effect is accepted only when a real in-use skip is observed.
