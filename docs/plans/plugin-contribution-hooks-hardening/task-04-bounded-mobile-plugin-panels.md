---
id: "04-bounded-mobile-plugin-panels"
title: "Bounded mobile plugin panels"
status: completed
wave: 2
depends_on: ["01-authoritative-plugin-lifecycle"]
plan: "plan.md"
spec: "../../specs/plugins/spec.md"
---

# Task 04: Bounded mobile plugin panels

## Acceptance

- Any positive number of mobile-enabled plugin panels consumes exactly one localized,
  touch-sized Panels bottom-nav entry and all registrations remain selectable from a
  `MobilePickerSheet` list.
- Selecting a panel closes the picker and renders the existing full-height plugin
  surface; loading/failed lifecycle preserves selection, while definitive removal
  changes the active session panel to Chat.
- Multiple/duplicate-titled panels remain independently selectable, picker rows are at
  least 44 px high, and the phone layout has no document horizontal overflow.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- components/task/mobile/session-mobile-bottom-nav.test.tsx components/task/mobile/session-mobile-layout.test.tsx components/task/mobile/plugin-panel-picker.test.tsx
```

Write the removal and multi-panel-capacity regressions first and confirm the current
direct-button implementation fails them.

## Files likely touched

- `apps/web/components/task/mobile/session-mobile-bottom-nav.tsx`
- `apps/web/components/task/mobile/session-mobile-bottom-nav.test.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.test.tsx`
- `apps/web/components/task/mobile/plugin-panel-picker.tsx` (new)
- `apps/web/components/task/mobile/plugin-panel-picker.test.tsx` (new)
- `apps/web/src/locales/en/common.json`
- `apps/web/src/locales/pseudo/common.json`

## Dependencies

Task 01 lifecycle snapshots and definitive removal semantics.

## Parallelism

`sequential`; this task consumes Task 01 and owns shared mobile navigation/layout files.

## Inputs

- Spec: task panels, mobile failure mode, and picker/revocation scenarios.
- Plan: **Bounded mobile plugin panels** and mobile design contract.
- `MobilePickerSheet`, `MobileSessionsPicker`, and Kandev Mobile UI Language.

## Risks

Do not overwrite a recoverable plugin selection during `loading`/`failed`, and do not
introduce a second vertical scroll owner around plugin content.

## Output contract

Report mobile hierarchy/geometry, files and locale keys changed, red-test evidence,
exact Vitest results, and synchronize task/plan status/results.

## Results

- Red phase: `rtk pnpm --filter @kandev/web test -- --run components/task/mobile/session-mobile-bottom-nav.test.tsx components/task/mobile/plugin-panel-picker.test.tsx components/task/mobile/session-mobile-layout.test.tsx`
  failed with 5 failures because the picker/helper did not exist and the nav still
  rendered direct per-panel buttons.
- Added the localized grouped Panels action and `PluginPanelPicker` with stable
  panel identities, duplicate-title support, 44px rows, shared drawer safe-area
  behavior, and an internally scrolling body. Selection dismisses the picker and
  keeps the existing full-height panel focused.
- Added lifecycle-aware mobile reconciliation: loading/failed preserves the stored
  selection, while ready-missing and removed fall back to Chat.
- The focused command above now passes — 3 files, 32 tests passed. Targeted mobile
  ESLint and full frontend lint also pass.
