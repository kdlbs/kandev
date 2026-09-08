---
spec: docs/specs/ui/app-status-bar.md
created: 2026-08-23
status: completed
---

# Implementation Plan: Mobile Status Metrics Grid

## Overview

Replace the phone Status drawer's single-line host-metrics strip with a width-aware grid while leaving desktop, tablet, and topbar presentations unchanged. The phone drawer will use its vertical space to show every enabled host metric, then targeted component and Pixel 5 Playwright regressions will prove wrapping and containment.

## Confirmed root cause

`DrawerSourceMetrics` in `apps/web/components/system-metrics/status-surface-metrics.tsx` puts the detailed Host marker and `MetricValues` in one horizontal flex row. `MetricValues` is another non-wrapping flex row with `overflow-hidden`, while every `MetricValue` is `shrink-0`; their combined minimum width therefore cannot reflow when it exceeds the drawer row. The drawer path also passes `limit={4}`, carrying a desktop strip constraint into a vertically scrollable phone surface. The mobile metrics section is an auto-width child of the drawer row, so it shrink-wraps its contents instead of claiming the available row width. Existing mobile E2E checks only document-level overflow and the outer status-row height, so clipped or internally overflowing metric indicators are not detected.

Smallest reproduction: enable the app status surface and all five current host metrics, open Status in the configured Pixel 5 project, and inspect `[data-status-item-id="builtin:metrics"]`. The metric children remain one horizontal row and the fifth enabled metric is absent rather than continuing below.

## Frontend

### Presentation-specific metric layout

- `apps/web/components/system-metrics/status-surface-metrics.tsx`: keep `BarSourceMetrics` on the current bounded inline layout and density limits. Change `DrawerSourceMetrics` to a vertical source-and-values composition: detailed mode renders `SourceBadge` as the leading source line, followed by a full-width responsive grid; simplified mode renders only that grid.
- Give `MetricValues` an explicit inline-versus-grid layout input instead of making shared metric rendering infer presentation from viewport state. The grid uses an auto-fit/minimum-cell rule so its column count derives from actual available width, produces two columns at Pixel 5 width, and falls back to fewer or grows to more columns only when space requires it.
- The drawer grid consumes all `source.metrics` entries in their received order. Keep the existing slice limits for bar/topbar paths. Grid cells remain touch-inspectable, use the existing icon, meter, value, color, accessible name, and tooltip behavior, and never create an internal horizontal scroll owner.

## Mobile design contract

- **Desktop outcome:** the 24 px status bar and pre-status-bar topbar fallback retain their current inline density, ordering, limits, and drag behavior.
- **Mobile entry point:** existing native Status triggers continue opening the global inset bottom drawer; no new route or control is added.
- **Nearest shipped exemplars:** `apps/web/components/app-status-bar/app-status-drawer.tsx` supplies the inset surface, fixed header, safe-area clearance, and one vertical scroll owner. `apps/web/components/task/chat/messages/kandev/rich-output/metrics-block.tsx` supplies the closest shipped width-aware metric-grid geometry.
- **Hierarchy and presentation:** System metrics remains one saved status item. In detailed mode, source identity precedes the metric grid; metrics then scan left-to-right and top-to-bottom. This read-only, frequently scanned content stays inline in the existing drawer rather than opening another surface.
- **Scrolling and touch:** `app-status-drawer-scroll-region` remains the sole scroll owner. Metric cells fit the item width, remain touch-inspectable, and add height to the drawer content instead of horizontal scrolling. Existing dynamic viewport and bottom safe-area handling remain unchanged.
- **Shared versus responsive behavior:** source selection, subscriptions, ordering, formatting, thresholds, simplified preference, and tooltips stay shared. Only drawer composition and its removal of the desktop display limit are phone-specific.
- **Mobile proof:** the configured Pixel 5 project enables all five current host metrics, opens Status, and proves two columns, three occupied rows, complete indicator visibility, item containment, and zero metric-item/drawer/document horizontal overflow.

## Tests

- **What:** phone rendering keeps every received metric and selects the grid layout, while desktop/bar rendering retains its four-metric limit and inline layout.
- **File:** `apps/web/components/system-metrics/status-surface-metrics.test.tsx`
- **How:** extend the existing state-provider harness with six ordered samples; assert all six mobile indicators render under the grid test surface in detailed and simplified modes, while the bar still renders only its configured first four. Keep the existing subscription, source filtering, host-marker, meter, and loading assertions.

## E2E Tests

- **Scenario:** **GIVEN** all five supported host metrics and detailed Status metrics are enabled, **WHEN** a phone user opens Status, **THEN** all five indicators render left-to-right across two Pixel 5 columns and continue into three rows without exceeding the metrics item, drawer, or document width.
- **File:** `apps/web/e2e/tests/settings/mobile-resource-metrics-display.spec.ts`
- **What to verify:** capture and restore both user display settings and global metrics settings; assert indicator count and bounding-box row/column relationships rather than fixed pixel widths; assert grid `scrollWidth <= clientWidth`, every indicator's horizontal bounds stay inside the grid/status item, the drawer keeps one vertical scroll owner, and document horizontal overflow remains zero. Preserve the existing simplified-mode scenario.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Reflow mobile host metrics](task-01-reflow-mobile-host-metrics.md) - sequential

No task is parallel-safe because component, unit, and E2E changes form one Red-Green regression path over the same responsive behavior.

## Risks and out of scope

- Auto-fit minimum cell width must yield two columns in the configured Pixel 5 viewport without forcing two columns below the legal content width. Geometry assertions should test relationships with a small subpixel tolerance, not frozen coordinates.
- The E2E mutation of install-wide metrics settings must restore its baseline in `afterEach`, including after failure, because the backend is worker-scoped.
- Desktop/tablet status-bar geometry, status-item ordering, metrics collection, metric definitions, persisted preferences, APIs, translations, and plugin contributions are unchanged.

## Verification Results

- Component RED: the added phone assertion failed on the fifth metric (`CPU temperature 71°C`); 1 failed and 7 passed.
- Pixel 5 RED: the original renderer produced one occupied row instead of three; 1 failed and the existing simplified scenario passed.
- Component GREEN: `pnpm --filter @kandev/web test -- components/system-metrics/status-surface-metrics.test.tsx` passed all 8 tests.
- Mobile GREEN: a fresh production build completed, then `pnpm e2e:run --no-build --project mobile-chrome tests/settings/mobile-resource-metrics-display.spec.ts` passed both scenarios. The geometry regression observed five indicators in two columns and three rows with no internal or document overflow.
- Static checks: `pnpm run typecheck` passed. Targeted ESLint passed with no findings.
- Visual check: `/tmp/mobile-status-metrics-grid.png` was captured from the final Pixel 5 metrics section and inspected. It shows the Host line followed by a two-column, three-row grid with the final unpaired metric on the left and no clipping.
- Cleanup: both Playwright scenarios completed their asserted `afterEach` restoration of user display settings, global metric settings, and app-status settings; the managed runner exited normally.
