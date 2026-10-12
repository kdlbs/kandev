---
id: "01-shared-tab-states"
title: "Refine shared settings tab states"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SETTINGS-HEADER-TABS-001
acceptance_criteria:
  - AC-UI-SETTINGS-HEADER-TABS-001.1
  - AC-UI-SETTINGS-HEADER-TABS-001.2
  - AC-UI-SETTINGS-HEADER-TABS-001.3
  - AC-UI-SETTINGS-HEADER-TABS-001.4
  - AC-UI-SETTINGS-HEADER-TABS-001.5
  - AC-UI-SETTINGS-HEADER-TABS-001.6
system_design:
  - ../../specs/ui/system-design/settings-header-tabs.md
---

# Task 01: Refine Shared Settings Tab States

## Summary

Give settings tabs a distinct selected surface and consistent default, hover, and focus states.
Override the faint primitive treatment at the shared settings boundary.

## In scope

- Track border/inset, selected surface/border/shadow, default/hover colors, and focus ring.
- Reduced-motion transition behavior and unclipped responsive target geometry.
- Focused component and browser regressions for both settings pages.

## Out of scope

- Changes to the global UI primitive or unrelated settings behavior.

## Acceptance

1. Selection visibly changes surface and border in both themes, using the actual Radix state attribute.
2. Default, hover, selected, and keyboard focus remain distinct with stable segment widths.
3. Desktop targets measure 28px within 1px; phone/coarse targets measure at least 44px without clipping or document overflow.

## ASCII UI preview

See the [combined preview](plan.md#ascii-ui-preview).

### UI-01: Desktop header, Office retention selected

```text
Storage                           +--------------------------------+
Manage disk use and history.       | Host   +---------------------+ |
                                  |        | Office retention    | |
                                  |        +---------------------+ |
                                  +--------------------------------+
--------------------------------------------------------------------
```

The outer border is quiet. The inner selected segment has a stronger surface, border, and small shadow.
Inactive hover receives a light fill. Keyboard focus has its own ring, including when focus is on Host.
The sketch indicates hierarchy, not literal nested boxes or exact pixel spacing.

### UI-02: Phone header, Office retention selected

```text
Storage
Manage disk use and history.

+--------------------------------+
| Host   +---------------------+ |
|        | Office retention    | |
|        +---------------------+ |
+--------------------------------+
----------------------------------
```

Tabs stay below the description. Touch targets remain at least 44px, with the track inset outside them.
The existing page remains the vertical scroll owner. There is no new sticky region or overlay.

### UI-03: State comparison

```text
Default:  readable muted label, transparent segment
Hover:    stronger label, subtle segment fill
Selected: foreground label, raised surface, border and shadow
Focus:    visible ring on either default or selected segment
```

The selected treatment must remain identifiable without relying on text color alone.
Maps to AC-UI-SETTINGS-HEADER-TABS-001.1 through .6.

## Verification

Use TDD for state behavior and a rendered regression for the broken selected surface.
Confirm the browser assertion fails before the correction because the selected surface is translucent and the hover fill is missing.
Run from the repository root. Install dependencies once if this worktree lacks them.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/settings/settings-tabs.test.tsx)
(cd apps/web && pnpm exec eslint components/settings/settings-tabs.tsx components/settings/settings-tabs.test.tsx e2e/tests/system/settings-header-tabs.spec.ts e2e/tests/system/mobile-settings-header-tabs.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/system/settings-header-tabs.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/system/mobile-settings-header-tabs.spec.ts)
git diff --check
```

The managed E2E runner builds current sources. Do not pass `--no-build` or override workers.
Record desktop light/dark and phone screenshots alongside computed-style and geometry assertions.
No copy changes are expected. If copy changes, include all locales and run the repository i18n gates.

## Files likely touched

- `apps/web/components/settings/settings-tabs.tsx`
- `apps/web/components/settings/settings-tabs.test.tsx`
- `apps/web/e2e/tests/system/settings-header-tabs.spec.ts`
- `apps/web/e2e/tests/system/mobile-settings-header-tabs.spec.ts`

## Dependencies

Existing shared SettingsTabs implementation. No additional work order.

## Risks

Theme colors can coincide. Track overflow can clip inset borders, focus rings, or touch targets.

## Parallelism

`sequential`

## Inputs

- [Header requirements](../../specs/ui/requirements/settings-header-tabs.md).
- [Header design](../../specs/ui/system-design/settings-header-tabs.md), visual-state refinement.
- Existing settings tab tests and shared control-sizing contract.

## Results

Implemented in the shared settings component. Selected tabs use an opaque theme surface,
border, and shadow; inactive hover and keyboard focus have separate treatments.
The track overrides the primitive horizontal height so 44px touch targets remain inset.

- RED: dark selected surface alpha was 77 instead of 255; inactive hover remained transparent.
- RED: mobile containment detected the inherited 32px track clipping 44px controls.
- GREEN: component tests 2/2; desktop browser tests 9/9; mobile browser tests 5/5.
- Targeted ESLint, TypeScript typecheck, specification validators, and diff whitespace check passed.
- Browser coverage includes both pages and themes, keyboard activation, reduced motion,
  stable widths, 28px desktop controls, and 44px touch targets at 390/767/768/1024px.
- Inspected desktop light/dark focus screenshots and the 390px phone screenshot.
- Public documentation does not need an update: labels, navigation, and operations are unchanged.
