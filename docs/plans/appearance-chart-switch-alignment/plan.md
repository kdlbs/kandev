---
created: 2026-11-23
status: in_progress
requirements:
  - REQ-UI-SETTINGS-COMPOSITION-001
  - REQ-UI-SETTINGS-COMPOSITION-004
  - REQ-UI-CONTROL-SIZING-001
system_design:
  - ../../specs/ui/system-design/settings-composition.md
  - ../../specs/ui/system-design/control-sizing.md
legacy_specs: []
---

# Implementation plan for chart animation switch alignment

## Overview

Align the three switches in Settings → Preferences → Appearance on desktop.
One work order owns the responsive CSS correction and its rendered checks.

## Scope

- Match the desktop chart switch to the Chat animations and image preview switches.
- Preserve the 44 px chart switch target on phones and coarse-pointer devices.
- Scope the fix to the chart switch layout.

## Technical approach

`RichOutputMotionSettingsCard` sets its button to 44 × 44 px and offsets its
28 px track by 8 px at all widths. Its neighboring switches use 36 × 28 px
buttons and a 4 px track inset on fine-pointer desktop. The rows align the
buttons at their right edges, so the chart track sits 4 px farther left.
The chart row also applies a 44 px minimum to its control wrapper at all widths.

Use the Chat animations switch as the desktop sizing reference. Share the
responsive classes across all three Appearance switches. Apply the 44 px
dimensions and matching track inset below 768 px or with a coarse pointer.
Let `SettingsRow` supply the responsive wrapper minimum.

The phone entry point stays at Preferences → Appearance. The existing
Appearance preference group is the mobile exemplar. Labels and descriptions
wrap above the right-aligned switches in the settings scroll region. Users
tap the switch and commit the draft with the existing Save changes action.

## ASCII UI preview

UI-01 shows the Appearance switches in the scrolling settings group. Spacing
is illustrative. The tracks must share a right edge on fine-pointer desktop.

```text
Desktop, current                       Desktop, proposed
Animate rich-output charts      [o ]   Animate rich-output charts        [o ]
Chat animations                   [o ] Chat animations                   [o ]
Show image previews by default    [o ] Show image previews by default    [o ]

Phone, proposed
Animate rich-output charts
Description wraps above the switch
                                 [o ]
Chat animations
Description wraps above the switch
                                 [o ]
```

## Tests

Keep the component test for the translated label, accessible switch, dirty
state, and change callback. Remove its unconditional touch-size assertions
when Playwright covers the responsive geometry.

## E2E tests

- `chromium`, `settings-manual-save.spec.ts`. Compare the rendered track edges
  and button heights. Check all three switches at 767 px and 768 px with a fine
  pointer. Keep the device save and reload assertions.
  Covers `AC-UI-SETTINGS-COMPOSITION-001.1`,
  `AC-UI-CONTROL-SIZING-001.3`, and `AC-UI-CONTROL-SIZING-001.5`.
- `mobile-chrome`, `mobile-general-settings.spec.ts`. Keep the existing
  390 px tap, target size, containment, no-overflow, and persistence checks.
  Check that the chart switch also keeps its touch dimensions at a coarse-pointer
  viewport above the phone breakpoint.
  Covers `AC-UI-SETTINGS-COMPOSITION-004.4` and `AC-UI-CONTROL-SIZING-001.4`.

## Work orders

- [ ] [Align the chart animation switch](task-01-align-chart-switch.md)

## Verification results

The two existing settings component test files passed all 20 tests.
The local PR documentation coverage check passed for the proposed change.
Chromium reproduced the 4 px desktop offset at a 1440 px viewport.
The existing mobile chart motion test passed its sizing and save checks.
Both browser checks used a 6 GiB systemd memory cap.
Implementation checks will run in the pull request pipeline.

## Risks

- A phone-width rule alone would shrink the target on coarse-pointer tablets.
- Matching the switch button edges alone can miss an offset in the painted track.
