---
id: "01-align-chart-switch"
title: "Align the chart animation switch"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SETTINGS-COMPOSITION-001
  - REQ-UI-SETTINGS-COMPOSITION-004
  - REQ-UI-CONTROL-SIZING-001
acceptance_criteria:
  - AC-UI-SETTINGS-COMPOSITION-001.1
  - AC-UI-SETTINGS-COMPOSITION-004.4
  - AC-UI-CONTROL-SIZING-001.3
  - AC-UI-CONTROL-SIZING-001.4
  - AC-UI-CONTROL-SIZING-001.5
system_design:
  - ../../specs/ui/system-design/settings-composition.md
  - ../../specs/ui/system-design/control-sizing.md
---

# Task 01. Align the chart animation switch

## Summary

Align the visible chart animation switch track with its neighbors in the
Appearance group. The chart switch uses the existing compact desktop geometry
and the existing 44 px touch geometry.

## In scope

- Extend the desktop chart motion save case with a painted-track alignment
  check. Use the switch rectangle and its computed `::before` inset and width.
  Check both neighbors before changing the component.
- Replace the unconditional 44 px desktop sizing expectation with responsive
  geometry checks. Use the neighboring Chat animations switch as the desktop
  exemplar. Remove the chart-only wrapper minimum. Share responsive classes
  that apply touch sizing below 768 px or with a coarse pointer.
- Use the mobile chart motion save case to check touch target size and
  containment. Add a coarse-pointer width above 768 px to exercise the other
  touch sizing condition.

## Out of scope

Chart motion logic, setting state, localization, and other settings rows.

## Acceptance

1. On a fine-pointer desktop, the chart switch button measures 28 px high.
   Its track shares a right edge with both neighboring tracks at 1440 px and
   768 px.
2. The chart switch measures at least 44 × 44 px in each touch context.
   Check a 767 px fine-pointer window, a 390 px touch viewport, and a
   900 px coarse-pointer viewport. All tracks also align at 767 px.
   Resizing preserves the switch value. The phone row stays inside the viewport.
3. The chart preference still saves on this device and persists after reload.

## ASCII UI preview

UI-01 excerpt shows the fine-pointer desktop alignment in the Appearance
group. See the [full desktop and phone preview](plan.md#ascii-ui-preview).
This excerpt corresponds to `AC-UI-SETTINGS-COMPOSITION-001.1`.

```text
Animate rich-output charts        [o ]
Chat animations                   [o ]
Show image previews by default    [o ]
```

## Verification

Run from the repository root. Run tests in a user systemd unit with a 10 GiB
memory limit. The desktop E2E command builds the checkout. The mobile E2E
command uses the same build.

```bash
(cd apps && pnpm install --frozen-lockfile)
run_chart_switch_tests() {
  systemd-run --user --wait --pipe --collect --working-directory="$PWD/apps/web" \
    -p MemoryMax=10G -p MemorySwapMax=0 --setenv="PATH=$PATH" \
    --setenv=GOFLAGS=-p=2 --setenv=GOMAXPROCS=4 \
    --setenv=GOMEMLIMIT=6500MiB \
    --setenv="LD_LIBRARY_PATH=${LD_LIBRARY_PATH:-}" \
    --setenv=NODE_OPTIONS=--max-old-space-size=4096 "$(command -v pnpm)" "$@"
}
run_chart_switch_tests exec vitest run components/settings/rich-output-motion-settings-card.test.tsx components/settings/general-settings.test.tsx
run_chart_switch_tests e2e:run --host --project chromium e2e/tests/settings/settings-manual-save.spec.ts -- --grep 'persists rich-output chart motion on this device only after Save'
run_chart_switch_tests e2e:run --host --no-build --project mobile-chrome e2e/tests/settings/mobile-general-settings.spec.ts -- --grep 'keeps the device chart-motion setting contained and persistent'
git diff --check
```

If the managed build exceeds the cap, build the frontend with
`run_chart_switch_tests run build:e2e`. Use the built backend and mock agent
from this checkout. Create a temporary config beside `e2e/playwright.config.ts`
that imports and spreads the workspace config with `globalSetup: undefined`.
Run the same test paths and name filters with
`run_chart_switch_tests e2e:raw --config=e2e/<temporary-config>.ts --project=<project>`.
The settings tests use the worker backend and skip the shared setup for
unrelated fixture binaries. On this NixOS host, set `LD_LIBRARY_PATH` to the
installed `kandev-builtin-playwright-libs/lib` directory before launching.
Remove the temporary config after the checks.

## Files likely touched

- `apps/web/components/settings/rich-output-motion-settings-card.tsx`
- `apps/web/components/settings/rich-output-motion-settings-card.test.tsx`
- `apps/web/components/settings/chat-motion-settings-card.tsx`
- `apps/web/components/settings/image-preview-defaults-settings.tsx`
- `apps/web/components/settings/settings-control.ts`
- `apps/web/e2e/tests/settings/settings-manual-save.spec.ts`
- `apps/web/e2e/tests/settings/mobile-general-settings.spec.ts`

## Dependencies

None.

## Risks

The button and track have separate dimensions. Check both in the browser.

## Parallelism

Sequential.

## Inputs

- [Settings composition requirements](../../specs/ui/requirements/settings-composition.md)
- [Control sizing requirements](../../specs/ui/requirements/control-sizing.md)
- [Settings composition design](../../specs/ui/system-design/settings-composition.md)
- [Control sizing design](../../specs/ui/system-design/control-sizing.md)

## Results

Baseline Chromium measured the chart track's right edge at 1400 px.
The Chat animations track ended at 1404 px. The mobile baseline passed.
The managed runner exceeded the memory cap while compiling the send-now
fixture. The focused fallback used the production web build, the same
Playwright worker fixtures, and a 6 GiB systemd cap.
Implementation is ready for the pull request pipeline. The user stopped the
local retry due to host resource limits.
