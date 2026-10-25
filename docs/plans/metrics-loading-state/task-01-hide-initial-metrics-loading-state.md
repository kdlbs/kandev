---
id: "01-hide-initial-metrics-loading-state"
title: "Hide initial metrics loading state"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/app-status-bar.md"
---

# Task 01: Hide initial metrics loading state

## Inputs

- [App status bar failure modes](../../specs/ui/requirements/app-status-bar.md#failure-modes)
- [App status bar scenarios](../../specs/ui/requirements/app-status-bar.md#scenarios)
- [Metrics loading-state plan](plan.md)
- Existing subscription and responsive rendering in `apps/web/components/system-metrics/status-surface-metrics.tsx`

## Acceptance

1. With metrics display enabled and `system.metrics === null`, desktop/tablet and an open phone Status drawer keep the normal WebSocket subscription active but render no metrics item or unavailable copy.
2. After a non-null host snapshot arrives, the existing bar/drawer values, detailed or simplified presentation, and mobile row geometry remain unchanged.
3. A non-null snapshot without a backend host source and received unavailable metric samples retain their genuine degraded presentation.

## TDD sequence

1. RED: extend `status-surface-metrics.test.tsx` with null-snapshot desktop and open-phone cases; confirm they fail because “Metrics unavailable” is rendered.
2. RED: instrument the existing desktop and mobile resource-metrics E2E flows before reload/first drawer open; confirm the observer records the transient unavailable copy.
3. GREEN: add the minimal post-subscription null-snapshot render gate in `StatusSurfaceMetrics`.
4. REFACTOR: remove no-longer-needed loading-only markup only if it is unreachable for every received-snapshot failure case; otherwise keep `EmptyMetrics` for genuine missing-host snapshots.

## Files likely touched

- `apps/web/components/system-metrics/status-surface-metrics.tsx`
- `apps/web/components/system-metrics/status-surface-metrics.test.tsx`
- `apps/web/e2e/tests/settings/resource-metrics-display.spec.ts`
- `apps/web/e2e/tests/settings/mobile-resource-metrics-display.spec.ts`

## Dependencies

None.

## Parallelism

`sequential`. The unit/component and E2E regressions share the same production loading gate and must prove one Red-Green sequence.

## Verification

```sh
cd apps && pnpm --filter @kandev/web test -- components/system-metrics/status-surface-metrics.test.tsx
cd apps/web && pnpm e2e:run tests/settings/resource-metrics-display.spec.ts tests/settings/mobile-resource-metrics-display.spec.ts
```

The managed E2E runner rebuilds the production Vite assets before exercising both desktop and `mobile-chrome` coverage.

## Output contract

Completed with the expected RED failures for null-snapshot desktop and phone rendering plus browser-observed transient unavailable copy. The final targeted component, Chromium, and mobile-chrome checks pass; no blockers remain.
