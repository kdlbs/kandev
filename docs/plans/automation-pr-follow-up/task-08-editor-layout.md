---
id: "08-editor-layout"
title: "Organize the automation editor"
status: done
wave: 4
depends_on: ["03-per-pr-continuation"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATIONS-SETTINGS-002
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006
acceptance_criteria:
  - AC-OFFICE-AUTOMATIONS-SETTINGS-002.1
  - AC-OFFICE-AUTOMATIONS-SETTINGS-002.2
  - AC-OFFICE-AUTOMATIONS-SETTINGS-002.3
  - AC-OFFICE-AUTOMATIONS-SETTINGS-002.4
  - AC-OFFICE-AUTOMATIONS-SETTINGS-002.5
  - AC-OFFICE-AUTOMATIONS-SETTINGS-002.6
  - AC-OFFICE-AUTOMATIONS-SETTINGS-002.7
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006.1
  - AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006.4
  - AC-OFFICE-AUTOMATION-CONTINUITY-001.6
  - AC-OFFICE-AUTOMATION-RETRIES-001.8
system_design:
  - ../../specs/office/system-design/automations-settings-04.md
---

# Task 08: Organize the automation editor

## Summary

Selecting **One task per pull request** revealed a follow-up field far above
the choice, often off screen. Move every choice above the fields it affects,
replace stacked radio cards with compact single choices, show placeholders
once, group agent tools, and correct copy that was untrue in some setups.

## In scope

- `SegmentedChoice`, `TargetModeSection`, and the new Then, Agent tools, and
  Limits and retries composition.
- Continuation, retry, prompt, schedule, and trigger-summary changes.
- Renamed labels in every catalog, the specs, and the public guide.
- Desktop and phone E2E updates.

## Out of scope

- Whether settings pages use bordered cards or full-width bands. That is a
  shared design decision for every settings page.

## Acceptance

1. Component tests cover the choice semantics, section options and
   descriptions, the placeholder disclosure, schedule wording, and summaries.
2. Desktop and phone E2E scenarios cover section order, stacked touch-sized
   options, the shared-task lock, retry details, and the follow-up position.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/automations)
(cd apps/web && pnpm run i18n:check && pnpm run lint)
(cd apps/web && pnpm e2e:run --host --project chromium tests/automations-settings.spec.ts tests/automations-pr-follow-up.spec.ts tests/automations-mcp-selection.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/mobile-automations-scroll.spec.ts tests/settings/mobile-automations-settings.spec.ts tests/mobile-automations-pr-follow-up.spec.ts tests/plugins/mobile-managed-automation.spec.ts)
```

## Parallelism

`sequential`

## Results

Done on 2026-11-25.

- Desktop automation E2E (8 specs, chromium): 61 passed.
- Phone automation E2E (6 specs, mobile-chrome): 14 passed.
- `pnpm run i18n:check`, `pnpm run lint:i18n`, and `pnpm run i18n:ratchet`: clean.

Deviation: the mockup's lowercase trigger summary uses the localized event
labels as written ("PR: Opened, New commits, Review requested"), because
lowercasing is not locale-safe. Tooltips are disabled on coarse pointers,
where a tap selects and the description line already explains the choice.
