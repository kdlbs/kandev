---
spec: docs/specs/agents/requirements/agent-rich-output.md
created: 2026-08-16
status: complete
---

# Fix Plan: Rich-output Chart Performance

## Outcome

Keep the existing Recharts line and bar entrance animations, but spend that
work only on a chart the user is actually viewing. Charts in background tabs
or below the transcript viewport defer their plot, unchanged messages do not
rebuild or restart plots, and users can disable rich-output animation per
device. Axes, tooltips, legends, CSV replay, and desktop/mobile composition stay
unchanged.

## Confirmed evidence and revised direction

The isolated application contains no performance fix; it is the before case.
It feels smoother after Vite/browser caches warm, after charts settle, and now
that the profiling Chromium process is closed.

The current `ChartBlock` uses Recharts 2.15.4 defaults: `Line` animates for
1,500 ms and `Bar` for 400 ms. A foreground harness measured 97 React commits
and 211 animated SVG attribute writes through roughly 1.9 seconds for one
10-point/two-series chart. Four maximum-density charts produced 5,004 DOM
mutation records and a 1.52-second frame gap. Those animations are intentionally
retained for a visible chart by user direction.

The avoidable work is concurrency and repetition:

- Every chart plot mounts immediately, including plots below the scrollport or
  in a background tab, so several examples can animate concurrently without
  being seen.
- `chart-block.tsx` rebuilds chart data, configuration, formatters, callbacks,
  and inline object/array props on every parent render.
- `rich-output-renderer.tsx` reparses stable persisted values, recreating block
  identities and preventing a memoized chart boundary from helping.
- There is no explicit per-device animation opt-out, and Recharts does not
  automatically honor the operating system's reduced-motion preference.

Settled charts do not have an infinite idle loop. Separate development tabs
also duplicate the full SPA's memory, so final claims use warmed production
builds and equivalent single-tab/background-tab comparisons.

## Implementation

### Wave 1: schedule each plot once and stabilize rerenders

- Add a small visibility hook beside the chart renderer. It observes both
  `document.visibilityState` and `IntersectionObserver` with a modest prewarm
  margin. A chart becomes eligible only when its block is near the viewport in
  the visible tab.
- Render the title, summary, and a fixed-height plot placeholder before
  eligibility so transcript geometry and accessible context stay stable.
- Once eligible, mount the real plot permanently. Do not unmount it when the
  user scrolls away; that would replay the entrance animation on return.
- Keep `isAnimationActive` at its Recharts default when motion is enabled.
- Memoize `parseRichOutput(args, result)`, `chartData`, `chartConfig`, x/y
  formatters, legend callbacks, and immutable chart props around their true
  dependencies. Memoize `ChartBlock` so an unchanged block does not rerender
  with its parent.
- Preserve axes, grid, tooltip, and legend as direct Recharts children because
  Recharts discovers them structurally.

### Wave 2: add the per-device motion opt-out

- Store one boolean preference in local storage, defaulting to enabled. Follow
  the existing `settingsMenu` preview/commit/restore pattern in the UI Zustand
  slice rather than adding an account setting or backend field: performance and
  motion preferences differ by device, and the value must be available before
  a chart mounts.
- Add one compact card to the existing Appearance section, adjacent to the
  color theme rather than creating another settings page. Use the shared save
  coordinator, a touch-safe `Switch`, explicit explanatory copy, and settings
  discovery metadata.
- Compute effective chart motion as saved/previewed device preference AND not
  `prefers-reduced-motion: reduce`. Subscribe to media-query changes so an OS
  preference change takes effect without reload.
- Pass the effective value to every Recharts `Line` and `Bar` through
  `isAnimationActive`. When enabled, retain the library's current style and
  duration exactly; when disabled, render complete geometry immediately.

## Files

### Chart scheduling and rerender stability

- `apps/web/components/task/chat/messages/kandev/rich-output/chart-block.tsx`
- `apps/web/components/task/chat/messages/kandev/rich-output/chart-visibility.ts`
- `apps/web/components/task/chat/messages/kandev/rich-output/chart-block.test.tsx`
- `apps/web/components/task/chat/messages/kandev/rich-output/chart-motion.ts`
- `apps/web/components/task/chat/messages/kandev/rich-output/chart-motion.test.tsx`
- `apps/web/components/task/chat/messages/kandev/rich-output/rich-output-renderer.tsx`
- `apps/web/components/task/chat/messages/kandev/rich-output/rich-output-renderer.test.tsx`
- `apps/web/e2e/tests/chat/rich-output.spec.ts`
- `apps/web/e2e/tests/chat/mobile-rich-output.spec.ts`

### Per-device animation preference

- `apps/web/lib/settings/rich-output-motion.ts`
- `apps/web/lib/settings/rich-output-motion.test.ts`
- `apps/web/lib/settings/constants.ts`
- `apps/web/lib/state/slices/ui/rich-output-motion-actions.ts`
- `apps/web/lib/state/slices/ui/rich-output-motion-actions.test.ts`
- `apps/web/lib/state/slices/ui/types.ts`
- `apps/web/lib/state/slices/ui/ui-slice.ts`
- `apps/web/lib/state/store.ts`
- `apps/web/components/settings/rich-output-motion-settings-card.tsx`
- `apps/web/components/settings/rich-output-motion-settings-card.test.tsx`
- `apps/web/components/settings/appearance-settings-state.ts`
- `apps/web/components/settings/appearance-account-sections.tsx`
- `apps/web/components/settings/general-settings.tsx`
- `apps/web/components/settings/general-settings.test.tsx`
- `apps/web/lib/settings-discovery/catalog/preferences.ts`
- `apps/web/lib/settings-discovery/catalog.test.ts`
- `apps/web/src/locales/en/settings.json`
- `apps/web/src/locales/pseudo/settings.json`
- `apps/web/e2e/tests/settings/settings-manual-save.spec.ts`
- `apps/web/e2e/tests/settings/mobile-general-settings.spec.ts`

### Durable artifacts

- `docs/specs/agents/requirements/agent-rich-output.md`
- `docs/plans/rich-output-chart-performance/plan.md`
- `docs/plans/rich-output-chart-performance/task-01-defer-and-stabilize-charts.md`
- `docs/plans/rich-output-chart-performance/task-02-rich-output-motion-setting.md`

No backend, MCP schema, database, persisted tool payload, dependency, or public
documentation change is required.

## Mobile design contract

The completed presentation remains inline in the same phone transcript. A
deferred plot reserves its existing height, so scrolling does not jump when it
mounts. Near-viewport charts animate once by default; offscreen charts wait.
Legend controls retain 44-pixel targets and the transcript remains the sole
vertical scroll owner.

The Appearance control uses the same settings page and shared floating Save
surface on desktop and mobile. Its row wraps explanatory text without
horizontal overflow, keeps a 44-pixel switch target, and requires no phone-only
modal or navigation branch.

## Verification

Run from `apps/web` unless noted:

1. `pnpm exec vitest run components/task/chat/messages/kandev/rich-output/chart-block.test.tsx components/task/chat/messages/kandev/rich-output/chart-motion.test.tsx components/task/chat/messages/kandev/rich-output/rich-output-renderer.test.tsx components/task/chat/messages/kandev-tool-message.test.tsx`
2. `pnpm exec vitest run lib/settings/rich-output-motion.test.ts lib/state/slices/ui/rich-output-motion-actions.test.ts components/settings/rich-output-motion-settings-card.test.tsx components/settings/general-settings.test.tsx lib/settings-discovery/catalog.test.ts`
3. `pnpm run typecheck`
4. `pnpm run lint`
5. `pnpm run i18n:check`
6. `pnpm run i18n:ratchet`
7. `pnpm e2e:run --no-build --project chromium tests/chat/rich-output.spec.ts tests/settings/settings-manual-save.spec.ts`
8. `pnpm e2e:run --no-build --project mobile-chrome tests/chat/mobile-rich-output.spec.ts tests/settings/mobile-general-settings.spec.ts`
9. `pnpm run lint:e2e-sleeps -- e2e/tests/chat/rich-output.spec.ts e2e/tests/chat/mobile-rich-output.spec.ts e2e/tests/settings/settings-manual-save.spec.ts e2e/tests/settings/mobile-general-settings.spec.ts`
10. From the repository root: `git diff --check`

## Waves

1. [x] [Defer and stabilize charts](task-01-defer-and-stabilize-charts.md)
2. [x] [Add rich-output motion setting](task-02-rich-output-motion-setting.md)

Both waves are sequential because they share `chart-block.tsx`, the performance
harness, and final browser measurements. No subagents are authorized.

## Performance evaluation

The warmed diagnostic stress presentation contained four 100-point,
four-series charts. Before the change, all four plots mounted together, with
5,004 chart DOM mutation records and an approximately 1.52-second maximum frame
gap. After the change, only two near-viewport plots mounted initially, with
3,415 chart mutation records and an approximately 1.15-second maximum frame
gap. These development-browser samples are directional rather than a CI
threshold because their instrumentation scopes are not perfectly identical.

With the per-device setting disabled, the same browser produced complete line
geometry without Recharts' animated dash attribute and 1,758 chart mutation
records in the sampled reload. The preference was restored to enabled after
measurement.

Headless Chromium reports every page as `document.visibilityState = "visible"`,
even after another page is brought to the foreground, so it cannot prove the
real background-tab branch. Unit coverage directly exercises hidden-document
intersection, later visibility, one-time mounting, and unsupported API
fallback. Desktop and mobile E2E cover real Recharts animation, the static
device opt-out, viewport eligibility, persistence, and touch geometry.

Final verification passed: 66 focused unit tests, TypeScript typecheck, full
workspace lint, both i18n checks, E2E sleep lint, the E2E production build,
8 desktop E2E cases, 7 mobile E2E cases, and `git diff --check`.

## Risks and considerations

- `IntersectionObserver` and Page Visibility events can race on initial load.
  Eligibility must be monotonic, and unsupported APIs must fall back to
  immediate rendering rather than a permanent placeholder.
- A deferred placeholder must preserve chart height and accessible title/
  summary without pretending an unmounted plot is interactive.
- Memo dependencies must include locale and payload changes; stale ticks or
  values are worse than extra work.
- Default animation must remain unchanged for visible charts. The opt-out and
  OS reduced-motion path are the only reasons to pass
  `isAnimationActive={false}`.
- Dense SVG retains a one-time visible mount cost. Data sampling, lower limits,
  canvas rendering, and changed animation duration remain out of scope.
- The local preference is intentionally device-owned. It does not sync across
  browsers and must not be added to the user-settings API accidentally.

The record review found no ADR is needed: this follows the established
per-device Appearance preference boundary rather than creating a new ownership
model.
