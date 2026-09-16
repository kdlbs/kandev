---
created: 2026-09-15
status: completed
requirements:
  - REQ-UI-SETTINGS-HEADER-TABS-001
system_design:
  - ../../specs/ui/system-design/settings-header-tabs.md
legacy_specs: []
---

# Settings Tab Visual States

## Overview

Strengthen visible selection and refine default, hover, and focus states on shared settings tabs.
The user's screenshot shows a flat track with little selected-state distinction.
Rendered checks confirm the missing hover surface. Compiled CSS confirms that the existing active selector works but uses a faint translucent dark fill.
This repair satisfies the existing shared selected/focus treatment contract. No new requirement is necessary.

## Scope

### In scope

- Shared settings tab track, trigger surfaces, and explicit active-state overrides.
- Desktop and phone geometry, light/dark themes, keyboard focus, and reduced motion.

### Out of scope

- Global tab primitive migration, icons, new tab labels, navigation changes, or retention behavior.

## Technical approach

`apps/packages/ui/src/tabs.tsx` uses a 30% opaque dark selected fill and no inactive hover background.
Compiled Tailwind expands `data-active` to match Radix, contrary to the initial source-only hypothesis.
`SettingsTabsList` owns the stronger surface treatment and overrides the inherited horizontal track height.
Implement the correct surface classes in `apps/web/components/settings/settings-tabs.tsx`.
Use existing theme tokens and preserve manual keyboard activation and mounted panel state.
The system design defines the visual treatment and scope boundary.

## ASCII UI preview

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

## Tests

Extend `settings-tabs.test.tsx` for active-state changes and keyboard focus separate from selection.
Computed appearance is a browser contract; class-string assertions alone cannot prove the repair.

## E2E tests

In `settings-header-tabs.spec.ts`, assert that active and inactive segments have different computed surfaces.
Assert that hover does not erase selected distinction and keyboard focus remains visible before activation.
Cover both consumers and light/dark themes, with focused screenshots for visual comparison.
In `mobile-settings-header-tabs.spec.ts`, assert phone placement, 44px targets, and unclipped track/trigger geometry.
Include the existing 767/768 checks and coarse-pointer tablet case.
Map these cases to AC-UI-SETTINGS-HEADER-TABS-001.1-.6.

## Work orders

- [x] [Task 01: Refine shared tab states](task-01-shared-tab-states.md)

## Verification results

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

## Risks

- Other consumers retain their existing primitive styling; this package limits implementation to settings tabs.
- Track padding can clip 44px targets or focus rings when its height is fixed.
- Theme backgrounds can coincide even with correct selectors. Browser checks must inspect the actual rendered surface.
