---
status: current
system: ui
requirements:
  - REQ-UI-STATS-CALENDAR-LABELS-001
---

# Statistics Calendar Labels System Design

## Purpose and boundaries

This is the local Statistics date-only presentation contract, shared by
completed activity and general activity. No global date policy or new date
abstraction is introduced. [Completion statistics](../../tasks/system-design/completion-statistics.md)
retains eligibility, repository attribution, timestamp selection, and backend
bucket authority. [Interactive reads](../../platform/system-design/interactive-read-availability.md)
retains request and recovery behavior.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-STATS-CALENDAR-LABELS-001` | Calendar formatting; Consumers and compatibility; Rendered verification |

## Calendar formatting

`apps/web/app/stats/stats-charts.tsx` already converts completed-activity
calendar keys to UTC midnight and derives Monday/week and month buckets with
UTC getters and setters. Its local `formatDate`, `formatWeekLabel`, and
`formatMonthLabel` shall format those representations with explicit
`timeZone: "UTC"`. Preserve their existing `undefined` locale argument and
month/day/year field options. This retains the current regional presentation
while preventing a UTC bucket from displaying as the previous local date.

Do not replace UTC grouping with local getters or convert date-only data to a
browser-local instant. Do not use a midday offset workaround.
`lib/i18n/formats.ts` provides general instant formatters with an active-i18next
locale contract; adopting it here would change the existing locale selection
and widen scope. Keep it unchanged. The three local option additions suffice.

## Consumers and compatibility

`CompletedTasksChart` uses the same series label for endpoint text and
`BucketBarChart` tooltips. Retain that single derivation, bucket selection,
bar sizing, counts, and sorting. `HeatmapGrid` shares `formatDate` for activity
tooltip dates, so it receives the same correction. `ActivityHeatmap` already
uses local calendar construction for weekday padding and month markers;
leave that separate placement path unchanged.

`CompletedTaskActivityDTO` and `DailyActivityDTO` in `lib/types/http-agents.ts`
remain date-only strings with existing numeric fields. Routes, clients, state,
backend storage and analytics queries stay unchanged. The fix creates no new
failure, retry, persistence, authorization, metric, or logging path.

Both viewports share the chart implementation. The existing desktop grid and
phone stacked Statistics cards remain the composition. The nearest live
exemplars are `mobile-stats-nav.spec.ts` and `mobile-stats-read-recovery.spec.ts`:
phone navigation remains Menu > Utilities > Stats with a single Statistics
scroll body. The curated `mobile-menu-sheet.tsx` remains the navigation model.
No new overlay or control geometry is required; no tooltip redesign is included.

## Rendered verification

Use the real Statistics route, React components, Button/Tooltip primitives and
i18n. The native Playwright `page` inherits context locale/timezone options;
the existing `testPage` is retained as the backend-reset fixture because its
separate context does not propagate those options. Browser contexts explicitly set `locale: "en-US"` and timezone IDs
`America/Los_Angeles`, `UTC`, and `Asia/Tokyo`. Intercept only the current
workspace's completed/daily activity endpoints using existing public DTO
shapes; let other requests reach the isolated fixture backend. These are
presentation fixtures, not evidence about backend query semantics.

Independently authored data spanning December/January and a Monday proves
calendar attribution alongside numeric and cardinality controls. Fresh managed
production builds must expose the visible Los Angeles label assertion failing
before the fix; zero tests, routing, backend, locale, or selector failures are
setup failures. Desktop verifies chart and heatmap tooltips using real hover.
Phone verifies visible chart labels after real bucket-control taps; it does not
claim a new coarse-pointer tooltip contract. Exact scenarios and commands live
in the [work order](../../../plans/stats-calendar-labels/task-01-calendar-labels.md).

## Related decisions

No ADR is needed: this local formatting correction preserves the existing UTC
bucket representation and makes no system-wide or persistence choice.

## Implementation plans

- [Statistics calendar labels](../../../plans/stats-calendar-labels/plan.md)
