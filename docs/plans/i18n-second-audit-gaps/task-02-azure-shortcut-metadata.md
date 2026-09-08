---
id: "02-azure-shortcut-metadata"
title: "Localize configuration metadata"
status: done
wave: 2
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/i18n-second-audit-gaps.md"
---

# Task 02: Azure And Shortcut Metadata

## Acceptance

- Azure mode, PR status, default-query, action, and hint labels resolve from keys at render time.
- Shortcut Settings names resolve from keys while IDs, bindings, WIQL, statuses, provider values, and
  agent prompt templates remain unchanged.
- Focused tests cover stable metadata and translated consumers.

## Likely Files

`apps/web/components/azure-devops/azure-devops-mode-tabs.tsx`, `azure-devops-status.ts`,
`azure-devops-workspace-defaults.ts`, their consumers/tests, `apps/web/lib/keyboard/shortcut-overrides.ts`,
its tests and Settings consumers, catalogs, guard configuration.

## Verification

```bash
cd apps/web && pnpm test -- --run components/azure-devops/azure-devops-status.test.ts components/azure-devops/azure-devops-presets.test.ts lib/keyboard/shortcut-overrides.test.ts && pnpm run lint:i18n -- components/azure-devops lib/keyboard app/settings
```

## Risks

Configuration objects cross render and persistence boundaries; only display metadata may change shape.

## Results

Azure modes/statuses and unchanged built-in query/action copy resolve from catalog keys; customized
records, IDs, bindings, WIQL, statuses, and prompts remain stable. Focused tests and typecheck passed.
