---
id: "10-override-switch"
title: "Override switch"
status: done
wave: 8
depends_on: ["09-local-overrides"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PROMPT-SYNC-007
acceptance_criteria:
  - AC-INTEGRATIONS-PROMPT-SYNC-007.1
  - AC-INTEGRATIONS-PROMPT-SYNC-007.2
system_design:
  - ../../specs/integrations/system-design/prompt-sync.md
---

# Task 10: Override switch

## Summary

Test-phase feedback: the Override and Revert icon buttons looked like the
ordinary edit and delete buttons, so a prompt could be overridden by accident.
Replace both with a labelled "Local override" switch, and confirm before
turning it off because that discards the local version.

## In scope

- `PromptOverrideToggle` and `PromptOverrideBadge` in
  `components/settings/prompt-override-controls.tsx`, used by synced and
  override rows.
- Confirmation through `MobileActionConfirmation` with an inline fallback.
- Copy in seven locales; remove the retired icon-button label.

## Out of scope

Backend changes.

## ASCII UI preview

See [plan.md](plan.md#ascii-ui-preview), UI-05.

## Acceptance

AC-INTEGRATIONS-PROMPT-SYNC-007.1 and 007.2.

## Verification

```bash
cd apps/web
pnpm exec vitest run components/settings/prompts-settings components/settings/prompt-sync-section.test.tsx hooks/domains/settings
pnpm e2e:run --host -- tests/settings/prompt-sync.spec.ts
pnpm e2e:run --host --no-build --project mobile-chrome -- tests/settings/mobile-prompt-sync.spec.ts tests/settings/mobile-prompts-settings.spec.ts
```

## Results

- RED: the component tests found no "Local override" switch and no revert
  confirmation.
- Component, hook, and section tests: 589 passed.
- Desktop spec: 2 passed. Mobile specs: 3 passed.
- `pnpm run lint`, `pnpm run typecheck`, and `pnpm run i18n:check`: pass.
