---
id: "07-chart-axes-legends"
title: "Chart axes and legends"
status: done
wave: 7
depends_on: ["06-csv-chart-sources"]
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-rich-output.md"
---

# Task 07: Chart axes and legends

## Acceptance

1. Inline and CSV-backed line/bar charts visibly render x- and y-axis tick
   values plus the existing grid and tooltip.
2. ISO date/time x values and large y values receive restrained host-owned tick
   formatting; tooltips retain the original x value and full numeric values.
3. One-series charts show their series label. Multi-series charts expose every
   label as a keyboard/touch legend filter with visible and accessible state.
4. Legend controls wrap inside the chart, meet the 44px phone hit-target rule,
   and introduce no document-level horizontal overflow.
5. Desktop and mobile production-build E2E prove axes, legends, filtering,
   tooltip data, containment, and replay.

## Root cause

`chart-block.tsx` placed `CartesianGrid`, `XAxis`, `YAxis`, and `ChartTooltip`
inside a custom `ChartAxes` React component. Recharts 2.15 discovers those
primitives by scanning a chart's direct children, so the wrapper prevented
registration and produced a plot with no ticks, grid, or tooltip.

## TDD order

1. Extend focused desktop/mobile E2E with axis, single/multi legend, filtering,
   tooltip, touch-target, and containment assertions; run both to observe RED.
2. Add failing pure tests for tick-formatting behavior.
3. Implement direct chart children, formatting helpers, and local legend state.
4. Re-run focused tests, typecheck, lint, and change-aware checks; inspect both
   desktop and Pixel 5 renderings in the isolated gallery.

## Files likely touched

- `apps/web/components/task/chat/messages/kandev/rich-output/chart-block.tsx`
- `apps/web/components/task/chat/messages/kandev/rich-output/chart-format.ts`
- `apps/web/components/task/chat/messages/kandev/rich-output/chart-format.test.ts`
- `apps/web/e2e/tests/chat/rich-output-helpers.ts`
- `apps/web/e2e/tests/chat/rich-output.spec.ts`
- `apps/web/e2e/tests/chat/mobile-rich-output.spec.ts`
- `docs/specs/agents/requirements/agent-rich-output.md`
- `docs/plans/agent-rich-output/plan.md`

## Risks

- Axis formatting must not mutate persisted data or collapse distinct timestamp
  labels into indistinguishable ticks.
- Recharts child discovery is structural; future refactors must keep axes,
  tooltip, and legend as direct chart children or fragments, not custom wrapper
  components.
- Filtering must remain local presentation state and must not add callbacks or
  payload fields to the MCP contract.

## Results

- Removed the opaque `ChartAxes` component so Recharts discovers x/y axes,
  grid, and tooltip as direct chart children for both line and bar charts.
- Added pure locale-aware formatting for compact ISO dates/times, bounded
  categories, large values, small decimals, and the pseudo-locale fallback.
  Persisted labels and tooltip values remain unchanged.
- Added an informational one-series legend and 44px pressed-state buttons for
  multi-series charts. Toggling a button changes only local `hide` state and
  never writes to the MCP payload or calls a tool.
- RED browser proof found zero axis text on both desktop and Pixel 5. GREEN
  desktop and mobile production runs passed with x/y tick assertions, raw
  tooltip values, six-to-three bar filtering, replay, containment, and native
  file opening.
- Verification passed: 62 focused frontend tests, backend MCP tests, typecheck,
  full web lint, i18n check/ratchet, focused E2E sleep lint, both focused E2E
  projects, 61 public-doc validator tests, and all 41 published pages.
- The isolated Tailscale gallery passed desktop and phone inspection with four
  axes, four 44px legend controls, working filter/restore, and zero horizontal
  overflow. The main `:9998` instance was not restarted or modified.
