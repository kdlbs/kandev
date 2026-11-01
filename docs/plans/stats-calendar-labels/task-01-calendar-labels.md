---
id: "01-calendar-labels"
title: "Keep Statistics labels on their calendar dates"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-STATS-CALENDAR-LABELS-001
acceptance_criteria:
  - AC-UI-STATS-CALENDAR-LABELS-001.1
  - AC-UI-STATS-CALENDAR-LABELS-001.2
  - AC-UI-STATS-CALENDAR-LABELS-001.3
  - AC-UI-STATS-CALENDAR-LABELS-001.4
  - AC-UI-STATS-CALENDAR-LABELS-001.5
system_design:
  - ../../specs/ui/system-design/stats-calendar-labels.md
---

# Task 01: Keep Statistics Labels on Their Calendar Dates

## Summary

Prove the western-timezone visible label regression on a fresh build, then
correct the three local formatting options. Preserve numeric behavior and prove
the same date attribution on desktop and phone.

## In scope

The [plan's A/M inventory](plan.md#affected-file-inventory) is the complete
owned file set. Implement only its chart change and three E2E files. Update
results here and in the manifest after actual verification.

## Out of scope

All [plan exclusions](plan.md#scope), especially backend projections, generic
timestamp formatting, neighboring stats files and touch/layout redesign.

## Acceptance

1. Fresh pre-fix production assets show a real Los Angeles endpoint-label
   failure after successful route/locale/data setup; retain original handle,
   logs, assertion and actual exit. Do not count setup or zero-test failures.
2. All desktop timezone scenarios pass day/week/month endpoints, cardinality,
   count and tooltip assertions with real components; heatmap date disclosure
   is correct. Satisfy `AC-UI-STATS-CALENDAR-LABELS-001.1`, `.2`, `.3`, `.5`.
3. Phone Los Angeles taps all buckets and sees the same correct dates/count
   structure. Preserve existing composition and localization (`AC-UI-STATS-CALENDAR-LABELS-001.4`).

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

Full combined preview: [UI-01](plan.md#ascii-ui-preview).

## Browser scenarios and independent oracle

Use test-base fixtures and context `test.use({ locale: "en-US", timezoneId })`
within separate describes. Do not change browser config or runner projects.
Route only exact current-workspace `/stats/completed-activity` and
`/stats/daily-activity` API paths before `/stats?workspaceId=<seed workspace>`.
Fulfill bare typed arrays as the public client expects; preserve other real
API requests. Use `waitForHttp` armed before navigation for both successful
reads, and assert browser locale/timezone and loaded chart uniquely. Context
routing is isolated, no persisted settings or backend storage modifications.

Completed fixture: `2025-12-31:2`, `2026-01-01:3`, `2026-01-05:4`.
Expected ordered series:

- Day: Dec 31 (2), Jan 1 (3), Jan 5 (4); three bars, endpoints Dec 31 / Jan 5.
- Week: Dec 29 (5), Jan 5 (4); two bars, endpoints Dec 29 / Jan 5.
- Month: Dec 25 (2), Jan 26 (7); two bars, endpoints Dec 25 / Jan 26.

Month text is short month plus two-digit year, not a day number. Never derive
expected dates with the production formatter or browser-local Date conversion.
Use literal en-US strings; count/control assertions are independent of labels.
Desktop hover each actual bar after moving the pointer away; scope active
Radix tooltip content and assert the matching label/count. Assert bar heights
from the existing count/max rule (day 50/75/100%, week 100/80%, month 29/100%)
and cardinality, keeping count attribution observable without storage writes.

Daily fixture: contiguous Jan 1 through Jan 5 2026, with Jan 1 having turns 2,
messages 3, tasks 1 and remaining dates zero. Hover the Jan 1 real heatmap
trigger and assert Jan 1 plus supplied turns/messages. Retain grid cardinality
and month marker Jan; do not alter heatmap padding/month computation.

Desktop `stats-calendar-labels.spec.ts`: test
`calendar keys retain labels, counts and tooltip attribution` in three
explicit contexts: Los Angeles (causal case), UTC and Tokyo (positive controls).
Each covers all three buckets and the heatmap tooltip, including year/month
and Monday boundaries. Before-fix expected western examples include Dec 30 /
Jan 4 for day and Nov 25 / Dec 25 for month.

Phone `mobile-stats-calendar-labels.spec.ts`: test
`phone bucket taps preserve calendar attribution` in Los Angeles using Pixel 5
project. Navigate via the existing route as in mobile-stats-read-recovery;
existing mobile-stats-nav owns hamburger reachability. Use `.tap()` for
Day/Week/Month, assert the same endpoints, bar cardinalities/heights, and
capture the phone chart for comparison to UI-01. Phone proof does not depend
on a Radix tooltip opening on tap. No hover/touch redesign.

## Verification

Design-only gates (repo root; no dependency installation):

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/ui docs/plans/stats-calendar-labels
git status --short -- docs/specs/ui docs/plans/stats-calendar-labels
```

From a fresh managed worktree, use Node 24.21.0 and install the workspace once
before the first package command. Run web commands from `apps/web`. Example:

```bash
node --version
(cd apps && pnpm install --frozen-lockfile)
```

Author the regressions first, with production unchanged. Browser discovery
must collect three desktop scenarios and one phone scenario, separately:

```bash
(cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts --project=chromium e2e/tests/layout/stats-calendar-labels.spec.ts --list)
(cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts --project=mobile-chrome e2e/tests/layout/mobile-stats-calendar-labels.spec.ts --list)
```

RED (fresh build mandatory; record failing visible label, not setup):

```bash
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium -- e2e/tests/layout/stats-calendar-labels.spec.ts --grep 'Los Angeles' --retries=0 --workers=1)
```

Name the western describe `Los Angeles` so the grep selects exactly one test.
After actual RED join/exit, make the minimal production fix. GREEN:

```bash
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium -- e2e/tests/layout/stats-calendar-labels.spec.ts --retries=0 --workers=1)
(cd apps/web && pnpm e2e:run --host --shards 1 --no-build --project mobile-chrome -- e2e/tests/layout/mobile-stats-calendar-labels.spec.ts --retries=0 --workers=1)
(cd apps/web && pnpm exec eslint --max-warnings 0 app/stats/stats-charts.tsx e2e/tests/layout/stats-calendar-labels.spec.ts e2e/tests/layout/mobile-stats-calendar-labels.spec.ts e2e/tests/layout/stats-calendar-labels-helpers.ts)
(cd apps/web && pnpm run typecheck)
```

Phone `--no-build` is permitted only after recording successful desktop fresh
GREEN assets and unchanged source/base/backend/web/plugin inputs and hashes.
If inputs change, omit `--no-build` and rebuild before phone verification.
All invocations are headless, resource-guarded, one project and one worker.
No broad browser or passing-suite replay. Existing recovery/navigation specs
are references, not changed behaviors requiring replay here. No new copy or
locale files are expected; normal active hooks retain their i18n checks.

Repeat the specification gates after final artifact updates. Run the local
PR-documentation `validateCoverage` preflight with the actual A/M files and all
four artifact contents, checking non-exempt coverage against the chart path.
Keep command results here and in the manifest. Normal commit hooks remain
active.

## Files likely touched

See the exact [A/M file inventory](plan.md#affected-file-inventory); no other
production, permanent-test, config or dependency files are authorized.

## Dependencies

None.

## Risks

Do not silently turn missing requests, wrong locale, zero collection or stale
assets into behavioral RED. Keep setup failures distinct from product regression evidence.

## Parallelism

`sequential`.

## Inputs

- [Requirement](../../specs/ui/requirements/stats-calendar-labels.md).
- [System design](../../specs/ui/system-design/stats-calendar-labels.md).
- Public fixture and source paths in the manifest.

## Results

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
