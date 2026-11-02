---
id: turn-changed-files-08
title: Native mobile historical navigation and localized availability
status: done
wave: 8
depends_on:
  - turn-changed-files-07
plan: plan.md
requirements:
  - REQ-TASKS-TURN-CHANGES-001
  - REQ-TASKS-TURN-CHANGES-005
  - REQ-TASKS-TURN-CHANGES-006
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-001.3
  - AC-TASKS-TURN-CHANGES-001.6
  - AC-TASKS-TURN-CHANGES-001.7
  - AC-TASKS-TURN-CHANGES-005.3
  - AC-TASKS-TURN-CHANGES-005.4
  - AC-TASKS-TURN-CHANGES-005.5
  - AC-TASKS-TURN-CHANGES-005.6
  - AC-TASKS-TURN-CHANGES-005.7
  - AC-TASKS-TURN-CHANGES-005.8
  - AC-TASKS-TURN-CHANGES-005.9
  - AC-TASKS-TURN-CHANGES-006.1
  - AC-TASKS-TURN-CHANGES-006.2
  - AC-TASKS-TURN-CHANGES-006.3
  - AC-TASKS-TURN-CHANGES-006.4
  - AC-TASKS-TURN-CHANGES-006.5
  - AC-TASKS-TURN-CHANGES-006.6
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
---

# Native mobile historical navigation and localized availability

## Summary

Complete the same review outcome on phones and coarse pointers with native drawer navigation and all required translations.

## Scope and owned files

- `components/task/mobile/{mobile-changes-panel.tsx,mobile-diff-sheet.tsx}` and existing mobile picker/focus-routing seams.
- Responsive transcript card geometry, touch full-path disclosure, and accessible unavailable/expired explanations.
- Shared historical turn/file selection, whitespace control, and viewer setting gates without mobile-only business logic.
- English and required locale catalogs, Chinese conversion, pseudo generation, and focused mobile E2E.

## Exclusions

No stacked desktop workbench, extra diff application, persisted desktop preference overrides, or hover-only phone actions.

## Implementation acceptance

1. A card/file tap opens the existing full-height diff Drawer at the exact historical target; turn/file choice uses the existing inset picker.
2. Fixed context, internal scroll, safe areas, focus return, and measured 44px phone/coarse-pointer hit targets preserve usability without document overflow.
3. Localized plural/count/availability copy passes all locale checks; mobile/desktop transitions retain selection and saved desktop preferences.

## Verification

Use the configured mobile-chrome project, without per-test device overrides. Cover fine-pointer phone width and coarse-pointer tablet in focused viewport cases.

```bash
cd apps/web
pnpm exec vitest run lib/state/historical-turn-navigation.test.ts components/task/mobile/mobile-diff-sheet.test.tsx components/task/chat/turn-changed-files-card.test.tsx
pnpm e2e:run --project mobile-chrome tests/git/mobile-turn-changed-files.spec.ts
pnpm run i18n:zh-hant
pnpm run i18n:pseudo
pnpm run i18n:check
pnpm run i18n:ratchet
pnpm run typecheck
```

Assert exact old content after subsequent edits, turn/file picker results, keyboard/back/Close focus, partial/expired states, and settings save/reload.
Measure actual action bounding boxes, viewport containment, scroll ownership, and horizontal overflow.
Compare rendered light/dark and pseudo-locale phone screenshots with UI-05. Test just below/above the 768px phone boundary.

## ASCII UI preview

UI-05: Phone card and direct historical drawer. [Combined preview](plan.md#ui-05-phone-transcript-card-and-direct-full-height-diff-drawer).

```text
Assistant: Updated validation.
+----------------------------------+
| 4 changed files      +42 -8      |
| [Expand all]       [Open diff]    |
| > src/               +30 -5      |
| A logo.png           Binary     |
+----------------------------------+

+----------------------------------+
| Turn 7, 14:32           [Close]  |
| [Turn 7 v]             +42 -8   |
| [app / validation.ts v]          |
| [ ] Ignore whitespace            |
|----------------------------------|
| - old validation                 |
| + new validation                 |
|     internally scrolling         |
|             safe area            |
+----------------------------------+
```

Card actions occupy a separate header row on phones. All actions are touch-usable; path disclosure does not depend on hover.
Drawer header is fixed; content owns vertical scrolling. Close restores focus to the originating row.
UI-01 stacked settings and UI-06 availability outcomes also apply on phones. Map to AC-005.5-.9 and AC-006.5-.6.

## Dependencies and risks

Depends on 07's shared target/projection contract. No heavy desktop panel remains mounted invisibly on phones.
Drawer primitive responsive variants must not override the caller's intended geometry.
Short translated labels cannot substitute for testing long and pseudo-localized copy.

## Results

Implemented the native mobile changed-files tree and historical diff drawer, localized all new copy, and aligned touch targets with mobile sizing. Full frontend tests, typecheck, `i18n:check`, and `i18n:ratchet` passed. The mobile-chrome changed-files and settings E2Es passed (2 tests), including drawer restoration and the 44 px close target.

### Review follow-up (2026-10-08)

The mobile historical drawer uses the shared scope target and selector, including exact Turn N navigation and Latest resolution. It shows the same summary totals and per-repository partial state as desktop. Focused mobile E2E passed for selector switching, exact historical content, drawer close, and focus restoration. Localization and touch behavior checks passed.
