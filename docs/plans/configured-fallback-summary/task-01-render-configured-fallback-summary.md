---
id: "01-render-configured-fallback-summary"
title: "Render configured fallback summary"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-CONFIGURED-FALLBACK-SUMMARY-001
acceptance_criteria:
  - AC-AGENTS-CONFIGURED-FALLBACK-SUMMARY-001.1
  - AC-AGENTS-CONFIGURED-FALLBACK-SUMMARY-001.2
  - AC-AGENTS-CONFIGURED-FALLBACK-SUMMARY-001.3
  - AC-AGENTS-CONFIGURED-FALLBACK-SUMMARY-001.4
system_design:
  - ../../specs/agents/system-design/configured-fallback-summary.md
---

# Task 01: Render configured fallback summary

## Summary

Add a pure fallback-state classifier and show its localized result as a badge
following the model badge in shared Agents settings profile rows. Preserve the
existing responsive row layout and saved profile data.

## In scope

- Add helper and unit tests for exact, executor-default, automatic, explicit,
  and precedence
  cases.
- Add the fallback badge and component regression coverage.
- Add required locale keys and update desktop/mobile settings E2E assertions.

## Out of scope

- Runtime fallback behavior, executor model catalogs, and profile persistence.
- New settings controls or interaction patterns.

## Acceptance

- Profile rows show the four required fallback states in the specified order.
- A saved explicit model is rendered unchanged, while automatic fallback takes
  precedence when both fields are set. Exact-model selection takes precedence
  when dormant automatic or explicit fallback values are also saved.
- Desktop and phone-sized rows remain accessible, wrapped, and free of document
  horizontal overflow.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run lib/*fallback* components/settings/agents/agent-profiles-section
cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps && pnpm --filter @kandev/web e2e:run -- settings/agents-browse-page.spec.ts settings/mobile-agent-profile-layout.spec.ts
```

## Files likely touched

- `apps/web/lib/agent-profile-fallback.ts`
- `apps/web/lib/agent-profile-fallback.test.ts`
- `apps/web/components/settings/agents/agent-profiles-section.tsx`
- `apps/web/components/settings/agents/agent-profiles-section.test.tsx`
- `apps/web/src/locales/*/agents.json`
- `apps/web/e2e/tests/settings/agents-browse-page.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-profile-layout.spec.ts`

## Dependencies

None.

## Risks

- Reusing translated interpolation values for `none` and `next` would leave
  those state names untranslated; use dedicated localized labels.
- The shared row is used by mobile and desktop, so badge order must not depend
  on viewport branches.

## Parallelism

`sequential`

## Inputs

- `docs/specs/agents/requirements/configured-fallback-summary.md`
- `docs/specs/agents/system-design/configured-fallback-summary.md`
- Existing `AgentProfile` normalization and profile-row tests.

## Results

Implemented the classifier, localized profile-row fallback badge, unit/component
regressions, and desktop/mobile E2E coverage. `make fmt`, `make typecheck`,
`make lint`, i18n checks, focused web tests, and both focused E2E projects pass.
The repository-wide `make test` command reaches unrelated backend test failures
in repository/worktree path and error-sanitization coverage.
