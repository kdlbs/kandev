---
status: in_progress
---

# Model and group scopes

## Outcome and scope

Add model and shared-group request scopes beside the existing account-wide daily allowance, fill them from measured catalog defaults, learn scope values from naturally occurring classified daily errors, and edit them in the existing provider-limit card. Owned areas are `agent/usage` (scope types, merge, evaluation, learning, embedded catalog), `backendapp` provider-limit composition, the task SQLite observation table, the existing settings card/API types, locale catalogs, and the public agent limit documentation.

Delivery is two sequential fork PRs on this work order:

1. Scopes: types and merge, catalog defaults with provenance, scope usage and admission, `model_daily` learning into known or new model scopes, and card editing. This also stops a per-model Zen block from learning the account-wide daily allowance from the whole account count.
2. Group recognition: create provisional groups from same-minute `model_daily` observations, identifying each observed model through the observation's execution profile (no migration).

Excluded: route reordering, model choice, idle-timeout changes, tier/monthly-reset/paid-block behavior changes, production or operating-data writes, credential changes, runtime builds/swaps, synthetic quota exhaustion, recurring probes, new approval gates, new settings screens or permissions, and new integration/E2E/CI or recurring test infrastructure. Depends on [the plan](plan.md) and fork PR #302.

## Contract references

- `REQ-COSTS-TIER-LIMITS-002`.
- `AC-COSTS-TIER-LIMITS-002.1` through `AC-COSTS-TIER-LIMITS-002.6`.
- [System design](../../specs/costs/system-design/provider-tier-limits.md#account-model-and-group-scopes).

## Acceptance

1. The free tier editor of each Zen and OpenRouter account row shows the account-wide value and the catalog scopes with source and provenance, and saves owner scope values through the existing request.
2. A candidate whose model or group scope is exhausted is skipped until the scope reset, without order or running-turn changes.
3. A classified per-model daily error replaces the covering scope's learned value with its counted requests; owner values survive. Same-minute errors from ungrouped models form a visible provisional group (PR 2).

## ASCII UI preview

UI-01 from [the plan](plan.md#ascii-ui-preview): scopes sit under the free tier's account-wide daily value on desktop and stack in the focused tier editor on phone, sharing drafts and the existing Save/status controls.

## Verification

Run only the affected package and file checks. Source-adjacent companion units cover scope merge precedence, catalog decoding, scope admission, `model_daily` learning replacement, owner preservation, and (PR 2) same-minute grouping. Focused provider-settings Vitest, web typecheck, changed-file ESLint, and `pnpm run i18n:check`. Disposable desktop/phone component capture with synthetic data. Normal commit hooks without bypass.

Installed card readback and one naturally occurring learned-value readback happen after installation by the operations owner; they are not produced by a synthetic error.

## Current execution results

PR 1 (scopes) merged as fork PR #338, including a review fix that keeps an owner scope's reset when its value changes. PR 2 (group recognition) is implemented on fork main `6a81bbdbaf`.

- PR 2 disposable `backendapp` checks with fake stores were removed before commit. They showed that a second per-model daily block within 30 seconds forms a provisional group with the combined count and replaces the first model's learned scope. A third model 20 seconds later joins under the same ID, and admission then defers the group's models. Catalog-scoped models blocked in the same minute keep separate scopes. Blocks two minutes apart stay separate, and an owner zero disables the group. Ambiguous membership in two groups, the same model twice, and a block with no partner all form no group.
- PR 2 existing focused tests passed: `go test ./internal/agent/usage ./internal/backendapp -run 'Tier|Limit|Scope|Catalog|Usage|Provider|Observation|Recorder'`. Build and vet passed for both packages.

- Disposable source-adjacent Go checks, removed before commit, covered the following: catalog decoding, Zen/OpenRouter/DevPass defaults and provenance, and scope merge precedence (owner > learned > default, owner zero disables). They also covered learned replacement, including downward values; no learning from a zero count; the learned 5% reserve; and validation. In `backendapp`, with fake stores, they covered default Zen scopes, scope request counts, group exhaustion deferring only the group's models, and owner override. They also covered `model_daily` learning into the covering model scope and a new scope for an unscoped model. The account-wide daily value stayed unlearned. The settings DTO carries scopes, usage and profile models.
- Existing focused tests passed: `go test ./internal/agent/usage -run 'Tier|Limit|Scope|Catalog'`, and `go test ./internal/backendapp ./internal/agent/settings/controller -run 'Usage|Limit|Tier|Provider|Calendar|Observation'`. `go build ./...` passed. `golangci-lint --new-from-rev=origin/main` on the changed packages reported 0 issues.
- A disposable Vitest check, removed before commit, ran on desktop and phone layouts. It confirmed that sources, usage, provenance, the owner group value and a new owner group reach the save request. Existing provider-limit Vitest files passed. ESLint on the changed files reported 0 warnings, and Prettier and `pnpm run i18n:check` passed. Web typecheck reports errors only in `rich-output` files, because the local `node_modules` lacks `zod` and `@json-render/core`; the changed files report none.

Installed card readback and a natural learned-value readback remain with the operations owner after installation.
