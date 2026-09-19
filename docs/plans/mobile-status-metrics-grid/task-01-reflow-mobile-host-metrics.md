---
id: "01-reflow-mobile-host-metrics"
title: "Reflow mobile host metrics"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/app-status-bar.md"
---

# Task 01: Reflow mobile host metrics

## Acceptance

1. The phone Status drawer renders every received host metric in enabled order through a width-aware grid; Pixel 5 uses two columns and adds rows as the count grows, with the detailed Host marker on its own leading line.
2. Every metric indicator stays horizontally inside the grid and status item, no metric-specific horizontal scroll exists, and the drawer retains one safe-area-aware vertical scroll owner.
3. Simplified-mode semantics remain intact, while desktop/tablet status bars and topbar fallback retain current inline layout, density limits, formatting, subscriptions, and tooltips.

## Inputs

- [App status bar What](../../specs/ui/app-status-bar.md#what), [responsive contract](../../specs/ui/app-status-bar.md#responsive-and-layout-contract), [accessibility](../../specs/ui/app-status-bar.md#accessibility), and [scenarios](../../specs/ui/app-status-bar.md#scenarios)
- [Mobile Status metrics grid plan](plan.md), especially **Confirmed root cause**, **Presentation-specific metric layout**, and **Mobile design contract**
- `apps/web/components/app-status-bar/app-status-drawer.tsx` for drawer geometry and scroll ownership
- `apps/web/components/task/chat/messages/kandev/rich-output/metrics-block.tsx` for responsive metric-grid precedent

## TDD sequence

1. **RED:** extend `status-surface-metrics.test.tsx` with six ordered host samples. Prove the phone currently truncates after four and lacks the grid surface; retain a bar assertion that four remains its limit.
2. **RED:** extend `mobile-resource-metrics-display.spec.ts` to enable all five supported host metrics and assert complete two-column/three-row Pixel 5 geometry plus metric-item containment. Confirm the current single-row renderer fails for the expected count/layout reason.
3. **GREEN:** separate inline and grid `MetricValues` layouts, make the drawer consume every received host metric, and move the detailed Host marker above the grid without changing shared formatting or subscription logic.
4. **REFACTOR:** keep presentation-specific classes at the drawer/bar boundary, reuse `MetricValue`, and avoid a second responsive state branch or scroll container.

## Files likely touched

- `apps/web/components/system-metrics/status-surface-metrics.tsx`
- `apps/web/components/system-metrics/status-surface-metrics.test.tsx`
- `apps/web/e2e/tests/settings/mobile-resource-metrics-display.spec.ts`

## Dependencies

None.

## Parallelism

`sequential`. Unit, implementation, and E2E changes share one responsive renderer and one global-settings fixture.

## Verification

```sh
cd apps && \
pnpm install --frozen-lockfile && \
pnpm --filter @kandev/web test -- components/system-metrics/status-surface-metrics.test.tsx && \
cd web && \
pnpm run typecheck && \
pnpm e2e:run --project mobile-chrome tests/settings/mobile-resource-metrics-display.spec.ts
```

Confirm Playwright discovers the intended mobile spec/tests. Inspect the final Pixel 5 rendering or screenshot and record its artifact path; if visual capture cannot run, record the exact blocker. Record baseline restoration and any managed-runner cleanup evidence.

## Output contract

Return intent/acceptance, changed files, exact RED and GREEN commands/results with discovered test counts, Pixel 5 rendered evidence, settings restoration/cleanup evidence, blockers, risks, and synchronized task/plan statuses. Reconcile **Files likely touched** with the actual diff before marking done.

## Results

- Intent and acceptance met. The phone Status surface now claims the drawer row width, places the detailed Host marker above a responsive metric grid, and renders every received host metric. Desktop keeps its existing inline layout and density limit.
- Changed production file: `apps/web/components/system-metrics/status-surface-metrics.tsx`.
- Changed regression files: `apps/web/components/system-metrics/status-surface-metrics.test.tsx` and `apps/web/e2e/tests/settings/mobile-resource-metrics-display.spec.ts`.
- Durable artifacts updated: `docs/specs/ui/app-status-bar.md`, this task, and `plan.md`.
- RED component command: `cd apps && pnpm --filter @kandev/web test -- components/system-metrics/status-surface-metrics.test.tsx`. Result: 1 failed, 7 passed; the phone could not find `CPU temperature 71°C` because the drawer stopped after four metrics.
- RED mobile command: `cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/settings/mobile-resource-metrics-display.spec.ts`. Result: the new geometry scenario failed with one row instead of three while the existing simplified scenario passed. An intermediate run then exposed the auto-width root as 128 px, producing five single-cell rows, while its parent row was 353 px.
- GREEN component command: `cd apps && pnpm --filter @kandev/web test -- components/system-metrics/status-surface-metrics.test.tsx`. Result: 1 file and all 8 tests passed, including all six received metrics in detailed and simplified phone rendering and the unchanged desktop four-metric limit.
- GREEN mobile command: `cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/settings/mobile-resource-metrics-display.spec.ts`. Result: both tests passed. The Pixel 5 regression found five enabled metrics in two columns and three occupied rows, all inside the grid, with `scrollWidth <= clientWidth`, one drawer vertical scroll owner, and no document overflow.
- Additional checks: a fresh managed E2E production build completed; `cd apps/web && pnpm run typecheck` passed; targeted ESLint passed with no findings.
- Rendered evidence: `/tmp/mobile-status-metrics-grid.png` was captured and inspected at Pixel 5 geometry. It shows the Host line plus the expected two-column/three-row layout without clipping.
- Cleanup: the passing E2E `afterEach` assertions restored the baseline user metrics display, install-wide metric selection, and app-status settings. The managed runner exited normally. No blockers remain.
