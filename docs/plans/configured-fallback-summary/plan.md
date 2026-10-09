---
created: 2026-09-08
status: complete
requirements:
  - REQ-AGENTS-CONFIGURED-FALLBACK-SUMMARY-001
system_design:
  - ../../specs/agents/system-design/configured-fallback-summary.md
legacy_specs: []
---

# Implementation Plan: Configured Fallback Summary

## Overview

Expose each saved agent profile's fallback policy as a pill in the existing
Agents settings profile rows. Implement the pure state classification first,
then render and localize it, and finally prove desktop and mobile behavior.

## Scope

### In scope

- Show `fallback: exact`, `fallback: none`, `fallback: next`, or
  `fallback: <model>` after the saved model pill.
- Derive the display from normalized profile fields with exact-model selection
  taking precedence over saved fallbacks, then automatic fallback taking
  precedence over an explicit saved model.
- Localize the new labels in all required web locales.
- Preserve wrapping, accessibility, and mobile overflow behavior.

### Out of scope

- Runtime model selection or fallback policy changes.
- Profile editor controls or persistence changes.
- Executor catalog probing and effective-model warnings.

## Technical approach

Add a pure helper under `apps/web/lib/` that classifies normalized
`AgentProfile` fallback fields into exact, executor-default, automatic, or
explicit-model display states. Use it from `ProfileRowCard` in
`apps/web/components/settings/agents/agent-profiles-section.tsx`, translating
only the display label and keeping model IDs opaque. Keep the existing
flex-wrapped metadata container and place the fallback badge after the model
badge.

Add the four labels to `agents.json` for `en`, `pt-pt`, `zh-cn`, `zh-hk`, and
`zh-tw`, plus pseudo-locale generation/ratchet coverage through the repository's
existing i18n checks.

## Tests

- `apps/web/lib/*fallback*.test.ts`: exact, executor-default, automatic,
  explicit, and precedence classification.
- `apps/web/components/settings/agents/agent-profiles-section.test.tsx`: badge
  ordering and opaque explicit model rendering.

## E2E tests

- Extend `apps/web/e2e/tests/settings/agents-browse-page.spec.ts` with a saved
  profile row assertion for the desktop outcome.
- Extend `apps/web/e2e/tests/settings/mobile-agent-profile-layout.spec.ts` with
  the same fallback summary assertion and the existing no-horizontal-overflow
  check.

## Work orders

- [x] [Task 01: Render configured fallback summary](task-01-render-configured-fallback-summary.md)

## Verification results

- `make fmt` passed.
- `make typecheck` passed.
- `make lint` passed with an external `TMPDIR` to avoid host temp-disk quota.
- `cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet` passed.
- Focused web tests passed: 17 tests across the helper and profile-row suites.
- Desktop and mobile focused E2E specs passed: 2 tests in each project.
- `make test` reached the backend suite but remains red on unrelated existing
  repository/worktree path and error-sanitization tests; an initial attempt
  also hit the host temporary-disk quota.

## Risks

- Automatic fallback must win when both `auto_fallback` and `fallback_model`
  are present, matching runtime precedence.
- Locale catalogs must retain matching keys and placeholders in every required
  locale.
