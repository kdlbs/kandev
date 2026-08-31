---
id: task-05-profile-editor
title: Profile editor provider section (frontend)
status: pending
wave: 3
depends_on:
  - task-01-provider-primitive
plan: plan.md
requirements:
  - REQ-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-001
acceptance_criteria:
  - AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-001.2
  - AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-001.3
  - AC-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-001.5
system_design:
  - docs/specs/agents/system-design/openai-compatible-providers.md
---

# Task 05: Profile editor provider section (frontend)

## Summary

Add a provider section to the agent profile editor: a provider-kind control
(`Native` / `OpenAI-compatible`), a base-URL input, and an API-key secret
picker (reusing the existing profile secret component). The whole section is
hidden unless the profile API projection reports `provider_supported`.

## Scope

- Provider section component + wiring into the profile editor form and its
  store slice / API client.
- Client validation mirroring the backend: non-empty absolute http(s) base URL,
  no `/` in the model when `openai_compatible`; block save with localized
  inline errors.
- All copy via `t()` / `<Trans>`; add keys in all five locales
  (`pnpm run i18n:zh-hant` for the Traditional Chinese pair).

## Exclusions

- No backend changes (task-01 owns the contract).
- No dynamic provider-option resolution changes.

## Implementation acceptance conditions

1. The section renders only for a `provider_supported` agent; switching to
   `Native` hides the base-URL and key fields and the saved payload clears
   them.
2. Save is blocked with a localized message for an empty/relative base URL or a
   slash-containing model while `OpenAI-compatible` is selected.
3. Desktop and mobile Playwright specs cover showing the section, entering a
   base URL + key, and the validation block; one-column touch layout, no
   horizontal document overflow on mobile.

## Verification

1. `cd apps && pnpm --filter @kandev/web test -- <provider section hook/validation test files>`
2. `cd apps/web && pnpm run typecheck`
3. `cd apps/web && pnpm run lint`
4. `cd apps/web && pnpm run i18n:check`
5. `cd apps/web && pnpm e2e:run <provider profile spec> -- --retries=0`
6. `cd apps/web && pnpm e2e:run --project mobile-chrome <mobile provider profile spec> -- --retries=0`

## Likely files

- `apps/web/components/settings/**/agent-profile*` editor + new provider section
- profile store slice + API client types
- `apps/web/src/locales/*/` new keys
- `apps/web/e2e/tests/settings/**` desktop + mobile specs

## Risks

- Secret picker reuse: confirm the existing profile env-var secret component can
  be driven for a single-value field without the key/value pair UI.
