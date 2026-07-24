---
id: "02-settings-and-rendering"
title: "Add simplified settings and rendering"
status: done
wave: 2
depends_on: ["01-persist-preference"]
plan: "plan.md"
spec: "../../specs/ui/app-status-bar.md"
---

# Task 02: Add simplified settings and rendering

## Acceptance

- Appearance settings expose a self-documenting **Simplified metrics** choice governed by the shared Save/discard coordinator.
- Detailed mode remains visually unchanged.
- Simplified mode omits the host marker and percentage meter bars in bar, fallback, and phone drawer presentations while retaining metric icons, values, colors, limits, accessible names, and tooltip details.

## Files likely touched

- `apps/web/components/settings/general-settings.tsx`
- `apps/web/components/settings/system-metrics-settings-card.tsx`
- `apps/web/components/settings/system-metrics-settings-card.test.tsx`
- `apps/web/components/system-metrics/status-surface-metrics.tsx`
- `apps/web/components/system-metrics/status-surface-metrics.test.tsx`

## Inputs

- Spec: **What**, **Responsive and layout contract**, **Accessibility**, and detailed/simplified scenarios.
- Plan: **Appearance setting**, **Metrics rendering**, and **Mobile design contract**.
- Existing mobile exemplar: `apps/web/components/app-status-bar/app-status-drawer.tsx`.

## TDD sequence

1. Add failing renderer tests for detailed default and simplified desktop/phone output.
2. Add a failing settings coordinator test for the simplified choice and dirty marker.
3. Implement the minimum shared state/save wiring and conditional rendering.
4. Refactor only if needed to keep the component within existing complexity limits.

## Verification

- `rtk pnpm --filter @kandev/web test -- --run components/settings/system-metrics-settings-card.test.tsx components/system-metrics/status-surface-metrics.test.tsx` from `apps`
- `rtk pnpm run typecheck` from `apps/web`
- Focused rendered desktop and Pixel 5 inspection through the existing Appearance and Status surfaces.

## Dependencies

- `01-persist-preference`

## Output contract

Return a compact handoff capsule containing intent/acceptance, base/head SHA, changed files and entry points, named spec sections, `localized`, `user-flow`, and `persistence` risk tags, exact commands/results, visual evidence, uncertainties, and this task file updated to `done`. Do not edit `plan.md`.
