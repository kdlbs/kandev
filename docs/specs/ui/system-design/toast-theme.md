---
status: current
system: ui
requirements:
  - REQ-UI-TOAST-THEME-001
---

# Toast Theme System Design

## Purpose and boundaries

The shared toast renderer follows the resolved document theme. This design
satisfies [toast theme consistency](../requirements/toast-theme.md) without
changing plugin operations or the platform's semantic notification delivery.

## Components and responsibilities

- `apps/web/components/theme/app-theme.tsx` resolves the saved or previewed
  preference against the system theme and applies the root class in a layout
  effect. Its ordering also serves terminal initialization, documented in
  [Terminal Rendering](terminal-rendering.md).
- `apps/packages/ui/src/sonner.tsx` adapts that document class to Sonner's
  `theme` prop. It owns initial synchronization and observes later class
  changes without depending on the web application's React context.
- `apps/web/src/app-shell.tsx` mounts the shared renderer with `richColors`.
  `apps/web/src/auth-gate.tsx` uses the same wrapper.
- Plugin action hooks send localized notifications through
  `apps/web/lib/toast/sonner.ts`; they do not choose presentation colors.
- `apps/web/components/toast-provider.tsx` is the separate existing toast
  surface using application CSS variables and dark variants.

## Theme synchronization

The wrapper's render-time theme read is provisional: an ancestor can apply the
resolved root class between render and passive-effect subscription. On mount,
the wrapper subscribes to root-class mutations and immediately synchronizes
its state with the current document class. Later mutations use the same theme
read. Cleanup disconnects the observer.

Sonner receives the resolved light or dark value. Its
`data-sonner-theme` attribute selects the success, error, warning, and info
palettes when `richColors` is enabled. The wrapper's existing `--normal-*`
variables continue to reference application tokens. Preserve explicit caller
props and observer cleanup; do not remount the toast renderer on theme changes.

## Responsive behavior

The nearest affected phone surface is the existing Settings > Plugins flow in
`apps/web/e2e/tests/settings/mobile-plugin-updates.spec.ts`. The curated
`components/kanban/mobile-menu-sheet.tsx` provides the shipped theme-aware
mobile navigation surface and theme-toggle entry point.

This is a color synchronization repair inside the existing toast surface.
Reuse its placement, responsive sizing, dismissal, and content. Rendered
verification uses the configured Pixel 5 project and a desktop project.

## Requirement mapping and verification

| Criteria                  | Design boundary         | Evidence                                                                                              |
| ------------------------- | ----------------------- | ----------------------------------------------------------------------------------------------------- |
| `AC-UI-TOAST-THEME-001.1` | Initial synchronization | Real provider/Toaster mount test; plugin install success after a dark cold load on mobile and desktop |
| `AC-UI-TOAST-THEME-001.2` | Resolved document class | Explicit light/dark preferences against the opposite OS preference                                    |
| `AC-UI-TOAST-THEME-001.3` | Mutation subscription   | Visible notification survives theme preview, discard, and system changes                              |

The component regression must render the real shared wrapper beneath
`AppThemeProvider` with an initially unthemed document, then emit a toast.
Sonner creates its themed list only once a notification exists. Wait for that
list and assert `data-sonner-theme`; asserting only the root class misses the
defect. Include a mount without development StrictMode effect replay.

Browser assertions check actual toast background, foreground, and border
against the resolved semantic palette, plus a readable mobile screenshot.
Run against the production Vite build because development effect replay may
hide initialization-order defects.

## Compatibility

Existing theme storage, settings preview/discard, notification content, and
plugin API contracts remain authoritative. No migration, additional
telemetry, or new architecture decision is required for this local
synchronization correction.
