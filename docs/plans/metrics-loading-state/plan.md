---
spec: docs/specs/ui/requirements/app-status-bar.md
created: 2026-07-30
status: complete
---

# Implementation Plan: Metrics Loading State

## Overview

Separate the initial WebSocket loading state from a received snapshot that lacks the Kandev host source. The status surface will stay subscribed but render no metrics item until the first snapshot arrives, then preserve the existing desktop, tablet, and phone presentations and genuine unavailable-sample behavior.

## Confirmed root cause

`defaultSystemState.system.metrics` starts as `null`. `StatusSurfaceMetrics` mounts, starts `useSystemMetricsSubscription`, and immediately maps the missing host source from that null snapshot to `EmptyMetrics`, so every reload briefly claims that metrics are unavailable before `system.metrics.updated` stores the first real snapshot. The backend already publishes the first snapshot through the normal subscription path, and real collection failures arrive as metric samples with `available: false`; no backend, WebSocket, or store contract change is needed.

## Frontend

### Status surface loading gate

- In `apps/web/components/system-metrics/status-surface-metrics.tsx`, keep the subscription hook ahead of all render exits.
- Return no metrics content when `snapshot` is `null`, allowing the existing `empty:hidden` status-item wrappers to collapse during initial loading.
- Continue to use `EmptyMetrics` only when a non-null snapshot lacks a backend host source. Preserve all received metric formatting, degraded samples, tooltips, source filtering, and responsive behavior.

### Mobile design contract

- Affected surfaces are the tablet/desktop app status bar and the phone Status drawer's initial metrics state.
- The nearest shipped phone exemplar remains `AppStatusDrawer` with its existing inset drawer, single internal scroll owner, safe-area handling, and 44 px rows.
- This repair adds no navigation, action, overlay, scroll, or touch change. On first open, the metrics row is absent until data arrives; the existing row appears in its saved order once ready.
- Desktop and mobile share the same snapshot state, subscription hook, source selection, and loading gate. Presentation-specific markup remains unchanged.

## Tests

- **What:** A null initial snapshot subscribes normally but renders neither the metrics surface nor “Metrics unavailable” on desktop and phone; a received host snapshot still renders values.
- **File:** `apps/web/components/system-metrics/status-surface-metrics.test.tsx`
- **How:** Extend the existing component harness to render `system.metrics: null`, assert the subscription gate remains enabled, and cover both bar and open-drawer presentations before retaining the current ready-state assertions.

## E2E Tests

- **Scenario:** GIVEN metrics are enabled, WHEN a desktop page reloads before its first metrics snapshot, THEN no transient unavailable copy is ever inserted and host values eventually appear.
- **File:** `apps/web/e2e/tests/settings/resource-metrics-display.spec.ts`
- **What to verify:** Install a pre-navigation mutation observer, reload the production SPA, wait for CPU metrics, and assert the observer never saw “Metrics unavailable.”
- **Scenario:** GIVEN metrics are enabled and the phone Status drawer has not subscribed yet, WHEN the user opens Status, THEN no transient unavailable copy is inserted and host values eventually appear in the existing touch-sized row.
- **File:** `apps/web/e2e/tests/settings/mobile-resource-metrics-display.spec.ts`
- **What to verify:** Observe the drawer's first-subscription render, wait for CPU metrics, assert no unavailable copy was seen, and retain existing containment, scrolling, and row-height checks.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Hide initial metrics loading state](task-01-hide-initial-metrics-loading-state.md) — complete

No task is marked parallel-safe because the production change and both regression levels exercise one shared loading-state behavior.

## Risks and out of scope

- The loading gate must not run before `useSystemMetricsSubscription`, or the missing snapshot would prevent the request that resolves it.
- A non-null snapshot without a backend host source and per-metric collection errors remain distinguishable genuine unavailable states.
- No sampler timing, WebSocket protocol, Zustand shape, fallback request, skeleton, reserved-width placeholder, or status-item ordering change is included.
- Public operations documentation remains accurate because its unavailable guidance describes received platform/sample failures, not the initial client loading state.

## Verification results

- `cd apps && pnpm --filter @kandev/web test -- components/system-metrics/status-surface-metrics.test.tsx` — passed (7 tests).
- `cd apps/web && pnpm e2e:run tests/settings/resource-metrics-display.spec.ts` — passed in Chromium against a fresh production build.
- `cd apps/web && pnpm e2e:run --no-build tests/settings/mobile-resource-metrics-display.spec.ts -- --project=mobile-chrome` — passed in the Pixel 5 mobile project against that build.
