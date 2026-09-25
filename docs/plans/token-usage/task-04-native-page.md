---
id: "04-native-page"
title: "Native Token Usage page"
status: done
wave: 4
depends_on: ["02-usage-apis", "03-session-cost"]
plan: "plan.md"
requirements:
  - REQ-COSTS-TOKEN-USAGE-004
  - REQ-COSTS-TOKEN-USAGE-005
acceptance_criteria:
  - AC-COSTS-TOKEN-USAGE-004.1
  - AC-COSTS-TOKEN-USAGE-004.2
  - AC-COSTS-TOKEN-USAGE-004.3
  - AC-COSTS-TOKEN-USAGE-004.4
  - AC-COSTS-TOKEN-USAGE-004.5
  - AC-COSTS-TOKEN-USAGE-004.6
  - AC-COSTS-TOKEN-USAGE-004.7
  - AC-COSTS-TOKEN-USAGE-005.1
  - AC-COSTS-TOKEN-USAGE-005.2
  - AC-COSTS-TOKEN-USAGE-005.3
  - AC-COSTS-TOKEN-USAGE-005.4
  - AC-COSTS-TOKEN-USAGE-005.5
  - AC-COSTS-TOKEN-USAGE-004.8
  - AC-COSTS-TOKEN-USAGE-004.9
  - AC-COSTS-TOKEN-USAGE-004.10
  - AC-COSTS-TOKEN-USAGE-004.11
  - AC-COSTS-TOKEN-USAGE-004.12
system_design:
  - ../../specs/costs/system-design/token-usage.md
---

# Task 04: Native Token Usage page

## Summary

Add the native Token Usage route and statistics navigation. Show core data with responsive breakdowns and distinct setup, range and error states.

## In scope

- Add Models, Daily and Monthly tables with token categories, totals and Cost/1M.
- Add numeric sorting, provider filtering, complete-result totals, partial-month labels and phone row details.
- Implement the desktop and phone previews with shared route, range and query state.
- Add core API client, summaries, charts, model/task/session details and Copy Stats.
- Add setup links based on scoped data existence and plugin availability.
- Add all locale keys and update public usage documentation for the shipped behavior.

## Out of scope

- Plugin registration slots, provider collection and budget controls.

## Acceptance

- The view renders native or retained plugin history without the plugin UI bundle.
- Desktop navigation precedes date tabs, and phone controls preserve all required actions.
- Empty database, empty range and query failure produce distinct states.

## ASCII UI preview

### UI-01: Desktop Token Usage, saved data

Entry: Statistics > Token Usage. Header remains outside the scrolling body.

```text
Statistics [Overview | Token Usage] [Last Week | Last Month | All Time] [Copy Stats]
+----------------------+----------------------+----------------------+
| Cost (estimated)     | Input / Output       | Cache read / write   |
+----------------------+----------------------+----------------------+
| Usage over time: [Cost | Tokens] [Day | Week | Month]               |
+-------------------------------------------------------------------+

[Models | Daily | Monthly | Tasks | Sessions]       [Provider: All v]
Model        Provider  Input Output Cache R Cache W Total   Cost    Cost/1M
model-a      ProviderA 1.0M  500K   3.0M    500K     5.0M  $20.00  $4.00
model-b      ProviderB 2.0M  1.0M  11.0M    1.0M    15.0M  $10.00  $0.67
All filtered records                              20.0M  $30.00  $1.50

Daily view
Date         Input Output Cache R Cache W Total   Cost    Cost/1M
2026-09-13   1.0M  500K   3.0M    500K     5.0M  $12.00  $2.40
2026-09-12   2.0M  1.0M  11.0M    1.0M    15.0M  $18.00  $1.20

Monthly view
Month        Input Output Cache R Cache W Total   Cost    Cost/1M
2026-09 *    3.0M  1.5M  14.0M    1.5M    20.0M  $30.00  $1.50
* Partial month within selected range
```

Only the selected table appears. Values are illustrative and the three example totals reconcile.
Columns have sortable numeric headers. The totals row covers every filtered page.
Cost/1M is a blended effective rate, not a list price. Missing coverage shows an unavailable rate.

### UI-02: Phone Token Usage, saved data

Entry: navigation sheet > Stats > Token Usage.

```text
Statistics
[Overview] [Token Usage]
[Last Month v]                 [Copy]
+----------------------------------+
| Cost and token summary           |
| Coverage / last update           |
+----------------------------------+
| Usage trend [Cost | Tokens]       |
+----------------------------------+
[View: Models v] [Sort: Cost v]
[Provider: All v]
+----------------------------------+
| model-a / ProviderA              |
| 5.0M tokens     $20.00            |
| $4.00 / 1M tokens     [Details >] |
+----------------------------------+

[View: Daily v]  [Sort: Date v]
| 2026-09-13                       |
| 5.0M tokens  $12.00  $2.40 / 1M  |
| [Token breakdown >]              |

[View: Monthly v] [Sort: Month v]
| September 2026 (partial)         |
| 20.0M tokens $30.00  $1.50 / 1M  |
| [Token breakdown >]              |
```

The header stays fixed. One body scrolls and clears the bottom safe area.
Only the selected view appears. The selector also offers Tasks and Sessions.
A detail surface exposes every token category and coverage status.
No wide desktop table appears on phones. Touch controls have at least 44px hit areas.
These previews cover AC-COSTS-TOKEN-USAGE-004.1 through .12.

### UI-03: Token Usage state variants

The shared header remains available on desktop and phone.

```text
Loading:       [Summary skeleton] [Trend skeleton]
No history:    No token usage yet. [Install Session Cost]
Installed:     Enable token collection. [Open plugin settings]
Collecting:    Waiting for the first measurement. [View collection status]
Empty range:   No usage in this range. [All Time]
Read error:    Could not load token usage. [Retry]
Undated only:  Lifetime usage available. Daily coverage unavailable.
```

The install action opens the existing install flow. It does not install automatically.
Native history counts as history. Plugin uninstall does not restore the installation placeholder.


See the [full preview](plan.md#ascii-ui-preview). Covers REQ-COSTS-TOKEN-USAGE-004 and REQ-COSTS-TOKEN-USAGE-005.

## Verification

Validation was run from the Kandev repository root; the executed commands and results are recorded below.

```bash
(cd apps/web && pnpm exec tsc --noEmit)
(cd apps/web && pnpm exec eslint --max-warnings 0)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm exec vitest run app/stats/token-usage-utils.test.ts lib/api/domains/stats-api.test.ts src/spa-routing.test.ts)
(cd apps/web && pnpm run build)
```

## Files likely touched

- `apps/web/app/stats/stats-page-client.tsx`
- `apps/web/app/stats/token-usage-page-client.tsx (new)`
- `apps/web/app/stats/token-usage.test.tsx (new)`
- `apps/web/lib/api/domains/stats-api.ts`
- `apps/web/lib/routing`
- `apps/web/src/locales`
- `apps/web/e2e/tests/layout/token-usage.spec.ts (new)`
- `apps/web/e2e/tests/layout/mobile-token-usage.spec.ts (new)`

## Dependencies

02-usage-apis, 03-session-cost.

## Risks

Range changes must discard old responses. Native-only records must not trigger installation guidance.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/costs/requirements/token-usage.md) and the acceptance IDs in frontmatter.
- [System design](../../specs/costs/system-design/token-usage.md).
- [Core projection ADR](../../decisions/2026-09-13-core-session-usage-projections.md).
- Existing source paths and test patterns listed in the [plan](plan.md#tests).

## Results

Implemented the native Token Usage route, navigation, range and grouping queries, summary cards, dated trend, model/day/month/task/session tables, nullable metrics, coverage notices, copy action, localized states and responsive touch controls.

Validation from `apps/web`: `pnpm exec tsc --noEmit`, `pnpm exec eslint --max-warnings 0`, `pnpm run i18n:check`, targeted Prettier, focused Vitest (24 tests) and `pnpm run build` passed. No browser E2E was run.
