---
created: 2026-10-19
status: implemented
requirements:
  - REQ-UI-STATS-CALENDAR-LABELS-001
system_design:
  - ../../specs/ui/system-design/stats-calendar-labels.md
legacy_specs: []
---

# Implementation Plan: Statistics Calendar Labels

## Overview

Correct date-only Statistics labels in one sequential work order. The source
uses UTC bucket arithmetic but browser-local formatting, which moves labels
backward in western timezones. Independently authored real-browser regressions
cover date, month, and year boundaries with stable count controls.

## Scope

Own only the chart's three date-label options and focused rendered regressions.
The shared activity tooltip is included. Preserve grouping, counts, ordering,
locale behavior, APIs, lifecycle, loading/retry, layout and controls. No backend,
token analytics, generic timestamps, timezone preference, dependency, storage,
accessibility redesign, or companion preservation work.

## Ownership

The existing completion-statistics pair owns backend eligibility and projection
values. UI owns the independent date-only presentation contract shared by
completion and activity consumers. The new pair leaves completion eligibility
unchanged. No ADR or UI README boundary change is needed.

## Technical approach

Add explicit UTC formatting to `formatDate`, `formatWeekLabel`, and
`formatMonthLabel` in `apps/web/app/stats/stats-charts.tsx`; keep their locale
and field options. Preserve UTC grouping and ActivityHeatmap placement.
Independently author browser fixtures using the existing route interception
and workspace fixture patterns. Scope to `#completed`'s card containing the
unique Completed Tasks Over Time title and `#activity`'s activity card.
No production test IDs should be needed: existing bucket buttons, endpoint
text, chart bar styling and Radix trigger/content slots suffice. Assert locator
cardinality before interaction; never hide duplicates with `.first()`.

## Affected file inventory

- M: `apps/web/app/stats/stats-charts.tsx` (only three formatting options).
- A: `apps/web/e2e/tests/layout/stats-calendar-labels.spec.ts`.
- A: `apps/web/e2e/tests/layout/mobile-stats-calendar-labels.spec.ts`.
- A: `apps/web/e2e/tests/layout/stats-calendar-labels-helpers.ts` (shared
  typed fixture/oracles, route setup and chart assertions only).
- A: this manifest and its one order, plus the UI requirement/design pair.

No Vitest project manifest change, other stats files, package/config/lockfiles,
or backend edits are planned. Public source/test patterns:
`stats-read-recovery.spec.ts`, `mobile-stats-read-recovery.spec.ts`,
`mobile-stats-nav.spec.ts`, `fixtures/test-base.ts`, and `helpers/causal-waits.ts`.
Nearby token analytics and page changes are outside this package.

## ASCII UI preview

UI-01: Statistics, loaded completed activity, en-US, Los Angeles.
The New Year values below are the independent regression-test oracle.

```text
Desktop /stats                  Phone: Menu > Utilities > Stats
[Completed Tasks Over Time]     [Completed Tasks Over Time]
[Day] [Week] [Month]             [Day] [Week] [Month]
[existing bars/counts]           [existing bars/counts]
Day before: Dec 30 ... Jan 4     Day before: Dec 30 ... Jan 4
Day after:  Dec 31 ... Jan 5     Day after:  Dec 31 ... Jan 5
Week after: Dec 29 ... Jan 5     Week after: Dec 29 ... Jan 5
Month after: Dec 25 ... Jan 26   Month after: Dec 25 ... Jan 26
[Activity / other cards]         [Activity / other cards below]
```

The desktop grid and phone card stack, existing scroll owner, controls and
navigation are retained. Spacing is illustrative. Correct calendar labels and
unchanged counts are required (AC-UI-STATS-CALENDAR-LABELS-001.1 through .5).
Desktop tooltips must agree with the visible bucket; phone proof uses visible
labels and taps, without introducing hover-dependent phone functionality.
Loading/error/empty states are unchanged.

## Tests

Permanent browser coverage directly exercises changed formatting logic; a new
exported helper or mock-driven unit suite is unnecessary. If component tests
become necessary, keep them beside the component, render real primitives/i18n,
and use the `browser-locales` Vitest project for new unreviewed files. A
zero-test selection is never RED or GREEN. Do not add a manifest exemption.

## E2E tests

The [single work order](task-01-calendar-labels.md) defines the independent
New Year data oracle, exact test names, every AC mapping, desktop timezone
controls, phone taps, fresh-build RED/GREEN commands and build reuse rule.

## Public documentation impact

Internal docs only. Public `feature-status.md` describes Statistics availability,
recovery and phone navigation; `docs/screenshots.md` lists overview media.
Neither prescribes date conversion. This correction restores the supplied
calendar attribution without new terminology, navigation, settings, APIs or
screenshots. No public docs change is needed for this bounded fix.

## Work orders

- [x] [Task 01: Keep Statistics labels on their calendar dates](task-01-calendar-labels.md)

## Verification results

- Desktop discovery: exactly 3 scenarios; phone discovery: exactly 1.
- Fresh managed Los Angeles RED: exit 1 on visible Day endpoints,
  expected Dec 31 / Jan 5 versus actual Dec 30 / Jan 4, after setup passed.
- Fresh managed Chromium GREEN: 3 passed (11.3s), Los Angeles, UTC, Tokyo.
- Separate mobile-chrome GREEN: 1 passed (4.3s), bucket-control taps.
  The no-build run reused identical recorded source/base/backend/web/plugin
  fingerprints. Desktop and phone captures matched UI-01's retained composition.
- Exact scoped ESLint command above: exit 0.
- Canonical `pnpm run typecheck`: exit 0 with
  `NODE_OPTIONS=--max-old-space-size=6144`; the initial default-heap attempt
  exhausted V8's 4 GiB heap and returned 134 without a TypeScript verdict.
- Catalog validation, full specification lint, non-exempt chart-path
  documentation traceability, and diff/whitespace checks: passed.
- Production change: only three explicit UTC formatting options. Backend
  queries, DTOs, bucket arithmetic, locales and layouts unchanged.

## Risks

Locale punctuation must not be confused with date correctness; en-US tests use
literal expected labels. Route interception must observe the intended payload
before assertions. Tooltip locators must target the active portal. Fresh assets
are mandatory after production changes. Mobile assertions cover existing
visible labels and controls, not a new touch tooltip capability.
