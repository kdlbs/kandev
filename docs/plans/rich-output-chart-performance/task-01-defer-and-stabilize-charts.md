---
id: "01-defer-and-stabilize-charts"
title: "Defer and stabilize charts"
status: completed
wave: 1
parallelism: sequential
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-rich-output.md"
---

# Task 01: Defer and stabilize charts

## Acceptance

1. A chart plot below the viewport or in a background tab does not mount. Its
   title, summary, and fixed plot space remain in the transcript.
2. A near-viewport chart in the visible tab mounts once and retains the current
   Recharts line/bar animation defaults. Scrolling away/back or rerendering an
   unchanged parent never remounts or restarts it.
3. Parsed presentations, chart data/configuration, axis formatters, callbacks,
   and immutable props retain identity for unchanged dependencies. A legend
   toggle changes only local series visibility.
4. Missing `IntersectionObserver` or Page Visibility support falls back to an
   immediately usable chart.
5. Desktop and Pixel 5 flows retain axes, raw tooltip values, keyboard/touch
   legend filtering, CSV replay, containment, and file opening.
6. The isolated multi-tab profile shows that background examples and offscreen
   plots no longer mount/animate concurrently. Results are recorded below.

## TDD order

1. Add visibility-hook tests for background documents, offscreen blocks,
   intersection, visibility changes, monotonic mount eligibility, cleanup, and
   unsupported browser APIs. Observe RED because plots currently mount
   immediately.
2. Add a mocked chart component regression proving derived inputs churn across
   a parent rerender/legend toggle and proving the default animation prop is
   retained after the change.
3. Implement visible-near-viewport mount-once scheduling, stable placeholder
   geometry, parsed-output memoization, chart prop memoization, and the memoized
   component boundary.
4. Update desktop/mobile E2E to scroll deferred charts into view before
   interaction and prove they still animate only when eligible.
5. Run focused checks and repeat single-chart, maximum-density, and multi-tab
   measurements in the isolated environment.

## Files

- `apps/web/components/task/chat/messages/kandev/rich-output/chart-block.tsx`
- `apps/web/components/task/chat/messages/kandev/rich-output/chart-visibility.ts`
- `apps/web/components/task/chat/messages/kandev/rich-output/chart-block.test.tsx`
- `apps/web/components/task/chat/messages/kandev/rich-output/rich-output-renderer.test.tsx`
- `apps/web/components/task/chat/messages/kandev/rich-output/rich-output-renderer.tsx`
- `apps/web/e2e/tests/chat/rich-output.spec.ts`
- `apps/web/e2e/tests/chat/mobile-rich-output.spec.ts`
- `docs/plans/rich-output-chart-performance/task-01-defer-and-stabilize-charts.md`

## Targeted commands

Run from `apps/web`:

```text
pnpm exec vitest run components/task/chat/messages/kandev/rich-output/chart-block.test.tsx components/task/chat/messages/kandev/rich-output/rich-output-renderer.test.tsx components/task/chat/messages/kandev-tool-message.test.tsx
pnpm run typecheck
pnpm run lint
pnpm e2e:raw --project=chromium e2e/tests/chat/rich-output.spec.ts
pnpm e2e:raw --project=mobile-chrome e2e/tests/chat/mobile-rich-output.spec.ts
pnpm run lint:e2e-sleeps -- e2e/tests/chat/rich-output.spec.ts e2e/tests/chat/mobile-rich-output.spec.ts
```

Then run `git diff --check` from the repository root.

## Risks

- Initial observer/visibility ordering must not strand a chart placeholder.
- A once-mounted plot must never unmount merely because it leaves the viewport.
- Recharts direct-child discovery prevents hiding axes/tooltip/legend behind a
  custom wrapper.
- Chart data or locale changes must invalidate only the relevant memoized
  values and still update the mounted plot.

## Results

Implemented monotonic, visible-near-viewport plot eligibility with a 200-pixel
prewarm margin and immediate fallback when Intersection Observer is absent.
Mounted plots stay mounted. Parsed output, chart data/configuration, formatters,
callbacks, and the chart component boundary now retain stable identities.

The warmed four-chart stress case changed from four plots mounting together to
two initial near-viewport plots. Chart mutation records fell from 5,004 to
3,415, and the sampled maximum frame gap changed from approximately 1.52 to
1.15 seconds. These values are diagnostic development measurements, not a
performance budget.

Headless Chromium cannot emulate this branch because it keeps all pages
`visible`; direct unit coverage verifies hidden-document deferral, later
visibility, mount-once behavior, and observer fallback. Desktop and mobile E2E
verify eligibility while retaining axes, legends, CSV replay, containment, and
file behavior.
