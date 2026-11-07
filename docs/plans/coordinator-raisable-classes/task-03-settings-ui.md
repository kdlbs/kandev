---
id: "03-settings-ui"
title: "Eligibility settings UI"
status: todo
wave: 2
depends_on: ["01-eligibility-settings"]
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-AUTOMATIC-001
  - REQ-COORDINATOR-AUTOMATIC-005
  - REQ-COORDINATOR-AUTOMATIC-006
  - REQ-COORDINATOR-AUTOMATIC-007
  - REQ-COORDINATOR-AUTOMATIC-008
acceptance_criteria:
  - AC-COORDINATOR-AUTOMATIC-001.2
  - AC-COORDINATOR-AUTOMATIC-006.5
  - AC-COORDINATOR-AUTOMATIC-005.6
  - AC-COORDINATOR-AUTOMATIC-008.3
system_design:
  - ../../specs/coordinator/system-design/automatic.md
  - ../../specs/coordinator/system-design/automatic-classes.md
---

# Task 03: Eligibility settings UI

## Summary

Show the five fields, bounds, the one-limit-across-classes copy, raise lists
for the new classes and configured-window labels. Copy in seven locales
(`pnpm run i18n:zh-hant` for the Traditional Chinese pair).

## Scope

The coordinator permission settings component and locales.

## Exclusions

Backend behaviour.

## Acceptance conditions

- Out-of-bounds values are blocked client-side and server-refused.
- `history_30d` reads the configured window.

## Verification

`cd apps/web && pnpm run typecheck && pnpm run i18n:check` plus the Playwright spec for the settings page.

## Likely files and risks

Settings component under `apps/web` coordinator settings; locale catalogs.

## Round 3 additions

- Show the lowered class from `lowered_class` with its display label, never the wire token (AC-008.3).

## Round 4 additions

- With phase 3 not effective the eligibility group is read-only and never sent; a 400 on `policy.eligibility` shows above the group.
