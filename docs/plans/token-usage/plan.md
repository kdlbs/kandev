---
created: 2026-09-13
status: complete
requirements:
  - REQ-COSTS-TOKEN-USAGE-001
  - REQ-COSTS-TOKEN-USAGE-002
  - REQ-COSTS-TOKEN-USAGE-003
  - REQ-COSTS-TOKEN-USAGE-004
  - REQ-COSTS-TOKEN-USAGE-005
system_design:
  - ../../specs/costs/system-design/token-usage.md
legacy_specs:
  - ../../specs/task-cost-ledger/spec.md
---

# Implementation Plan: Native Token Usage

## Overview

Build the accounting service first, then expose its Host API.
Connect Session Cost to that service, then add native statistics.
This order establishes attribution and idempotency before any collector writes cumulative totals.

The user selected core persistence, future source support, saved cost during refresh, and native topbar navigation.
The costs system owns this package because it owns the accounting source of truth.
The assumption check reuses those settled choices. No new product approval is required.

Source inspection found an existing native event ledger after the initial task investigation.
This package preserves that ledger and adds external measurement storage plus a canonical statistics projection.
The earlier task note claiming no native recording is incomplete and is not implementation authority.

## Scope

### In scope

- [Requirements](../../specs/costs/requirements/token-usage.md), their five REQ IDs, and all listed AC IDs.
- Core session measurements, dated buckets, native-ledger projection and source selection.
- Host APIs, bounded session discovery, optional tokscale collection and saved session display.
- Native Token Usage navigation, responsive data views, copy summary and setup/error states.

### Out of scope

- New ACP/app-server collectors, remote transcript transport, Office budget migration and billing reconciliation.
- Generic stats plugin slots, publishing packages and automatic installation.

## Technical approach

The [system design](../../specs/costs/system-design/token-usage.md) owns model and selection rules.
The [ADR](../../decisions/2026-09-13-core-session-usage-projections.md) owns storage rationale.

1. Extend `internal/analytics/{models,repository,service}` with `UsageService` and portable measurement tables.
   Read `TaskUsageEvent` through an explicit repository/service seam. Preserve the existing ledger writer and rollups.
   Add fixtures for duplicate reports, source overlap, unknown fields, corrections and source dates.
2. Extend `plugin.proto`, `pkg/pluginsdk/host.go`, and `internal/plugins` with typed read/write RPCs.
   Optimize `host_data_queries.go` through task-service SQL filters and stable pagination.
   Add authenticated analytics queries and browser DTOs.
3. Update the sibling Session Cost repository's manifest, server and bundle.
   Add one worker, shared command execution, durable checkpoints and separate saved-read/refresh actions.
4. Extend `apps/web/app/stats`, `stats-api.ts`, routing and locale catalogs.
   Use the existing `StatsPageClient` shell and `useStatsSections` request-identity pattern.

New table names: `session_usage_measurements` and `session_usage_buckets`.
The existing `task_usage_events` table is not a destination for external cumulative writes.
The plugin repository is `../kdlbs-kandev-plugin-session-cost` relative to the Kandev root.

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

### UI-04: Session Cost refresh states

Entry: chat cost icon. Desktop uses a popover. Phone uses an explicit touch drawer.

```text
Saved + refresh                 No saved measurement
SESSION COST                    SESSION COST
$1.24 estimated                 Calculating cost...
Input / Output / Cache
Saved 2 minutes ago             Failed initial calculation
Refreshing...                   Could not calculate. [Retry]

Saved + failed refresh
$1.24 estimated
Saved 2 minutes ago
Refresh failed. [Retry]
```

The amount is illustrative. The structure and preservation of saved values are required.
Desktop controls retain standard density. Phone drawers have one scroll body and a close control.
Keyboard focus returns to the cost trigger after dismissal.

UI-01/02 cover AC-COSTS-TOKEN-USAGE-004.1 through .12.
UI-03 covers AC-COSTS-TOKEN-USAGE-005.1 through .5 and 004.4 through .5.
UI-04 covers AC-COSTS-TOKEN-USAGE-003.1 through .4.
Exact wording, spacing and chart styling are illustrative and use localized shared primitives.

## Table scope refinement

The user requested tokscale-inspired model/provider, daily and monthly tables on 2026-09-13.
The core service computes total tokens, total cost and blended Cost/1M from matching canonical coverage.
The plugin preserves provider and date detail. It does not compute cross-session rates or own the native tables.
The [design](../../specs/costs/system-design/token-usage.md#usage-tables-and-effective-rates) defines unknown-rate and partial-period behavior.

## Tests

The table is the planned acceptance coverage map. Executed evidence is recorded in the verification results and task records.
Paths in this table start at `apps/backend/`, except the explicitly named plugin and web paths.

| Acceptance criteria | Planned file and test |
| --- | --- |
| 001.1, 001.2, 001.4 | `internal/analytics/service/usage_test.go`: `TestUsageBatchRevisionsAndPresence` |
| 001.3, 001.7 | `internal/analytics/service/usage_test.go`: `TestUsageSourceSelectionPreservesLedger` |
| 001.5 | `internal/analytics/repository/sqlite/usage_test.go`: `TestUsageRetentionAcrossDeletionAndUninstall` |
| 001.6 | `internal/plugins/host_usage_test.go`: `TestUsageCapabilityAndSourceIdentity`. `internal/analytics/handlers/usage_authz_test.go`: `TestUsageWorkspaceIsolation` |
| 002.1, 002.2 | Plugin `server/collector_test.go`: `TestCollectionSettings` |
| 002.3, 002.4 | Plugin `server/collector_test.go`: `TestConcurrentRefreshAndIdleSkip` |
| 002.5, 002.6, 002.7 | Plugin `server/collector_test.go`: `TestFinalCollectionRecoveryAndAttribution` |
| 003.1, 003.2, 003.3 | Plugin `test/bundle.test.mjs`: `saved usage survives refresh and ignores old session responses` |
| 003.4 | Plugin `test/bundle.test.mjs`: `touch and keyboard expose cost and refresh` |
| 004.1, 004.2, 004.6, 004.7 | Web E2E matrix below |
| 004.3, 004.5 | `apps/web/app/stats/token-usage.test.tsx`: `renders partial usage and copies the selected view` |
| 004.4 | `internal/analytics/service/usage_test.go`: `TestUsageDateCoverageAndTimezone` |
| 005.1, 005.2, 005.3, 005.4, 005.5 | `apps/web/app/stats/token-usage.test.tsx`: `distinguishes setup, native history, empty range and errors` |

Additional table evidence, in the same test suites and verification commands:

| Acceptance criteria | Planned file and test |
| --- | --- |
| 004.8, 004.12 | `internal/analytics/service/usage_test.go`: `TestUsageModelProviderGrouping` |
| 004.9, 004.12 | `internal/analytics/service/usage_test.go`: `TestUsageWeightedEffectiveRateAndUnknownCoverage` |
| 004.10, 004.11 | `internal/analytics/service/usage_test.go`: `TestUsageDailyMonthlyReconciliationAndPartialRange` |
| 004.11 | `internal/analytics/handlers/usage_handlers_test.go`: `TestUsageSortPaginationAndFilteredTotals` |
| 004.8 through .12 | `apps/web/app/stats/token-usage.test.tsx`: `renders model and period tables with effective rates` |

Each short AC suffix in this table expands to `AC-COSTS-TOKEN-USAGE-<suffix>`.
Repository tests run on SQLite and PostgreSQL through existing database test fixtures.
Tests also cover subprocess cancellation, overflow and incompatible Host versions.

## E2E tests

Paths start at `apps/web/e2e/tests/`. The configured projects own their usual devices and worker budgets.

| File / project | Scenario and acceptance criteria |
| --- | --- |
| `layout/token-usage.spec.ts` / `chromium` | Topbar order, direct route, workspace/range/back navigation and Copy Stats: 004.1, 004.2, 004.3 |
| `layout/token-usage.spec.ts` / `chromium` | Empty/setup, native history, empty range, failure and uninstall-retained history: 004.5, 005.1 through .5 |
| `layout/mobile-token-usage.spec.ts` / `mobile-chrome` | Sheet entry, range picker, breakdown details, copy, 44px targets and zero horizontal overflow: 004.2, 004.6, 004.7 |
| `plugins/session-cost-refresh.spec.ts` / `chromium` | Installed plugin shows saved amount during delayed refresh, failure and session switch: 003.1 through .3 |
| `plugins/mobile-session-cost-refresh.spec.ts` / `mobile-chrome` | Real touch drawer, refresh, dismissal and focus return: 003.4 |

Plugin E2E uses the sibling package and a controlled tokscale fixture command.
It must exercise the installed bundle, not a separately reimplemented fixture UI.
Additional scenarios in the existing native page suites:

| File / project | Scenario and acceptance criteria |
| --- | --- |
| `layout/token-usage.spec.ts` / `chromium` | Model/provider rows, weighted totals, numeric sorting, daily/monthly switching, partial month and unknown rate: 004.8 through .12 |
| `layout/mobile-token-usage.spec.ts` / `mobile-chrome` | All three table choices, sort, provider filter and every metric through row details: 004.8, 004.10, 004.12 |

Companion setup scenarios from the plugin package:

| File / project | Scenario and acceptance criteria |
| --- | --- |
| `plugins/session-cost-collection.spec.ts` / `chromium` | Settings validation, explicit import, cancel/resume and no implicit historical scan: 002.1, 002.2, 002.6 |
| `plugins/mobile-session-cost-collection.spec.ts` / `mobile-chrome` | Same settings/import outcome with reachable touch controls: 002.1, 002.6 |

Managed runners rebuild application assets. Causal HTTP/WS waits replace arbitrary sleeps.

## Work orders

- [x] [Task 01: Core usage accounting](task-01-core-usage.md)
- [x] [Task 02: Host usage and query APIs](task-02-usage-apis.md)
- [x] [Task 03: Session Cost collection and saved display](task-03-session-cost.md)
- [x] [Task 04: Native Token Usage page](task-04-native-page.md)

Dependency order: 01 -> 02 -> 03 -> 04. Work runs sequentially.
Task 03 expands into four sequential [plugin work orders](../../../../kdlbs-kandev-plugin-session-cost/docs/plans/token-usage/plan.md).
The companion package owns plugin-local implementation details. Both packages share acceptance IDs and integration evidence.
This package does not authorize subagents or persistent child tasks.

## Verification results

Feature verification completed on 2026-09-13.

Backend validation:

- The changed analytics, plugin, task and public plugin SDK packages pass the focused Go test suite with inherited KANDEV configuration variables unset.
- `go build ./cmd/kandev` passes.
- SQLite usage tests cover revisions, idempotency, corrections, unknown values, workspace ownership, provider grouping, source overlap, dated buckets and effective rates.
- Session repository tests cover workspace, task, state, update-time filters and SQL pagination.
- The full `go test ./...` run was attempted. Changed packages passed. Existing process probe tests remain environment-sensitive because real process descendants were observed as live during the settling window. Existing config and launcher failures caused by inherited `KANDEV_*` variables pass when those variables are unset.

Plugin validation:

- `make test vet build package-host` passes.
- `go test -race ./server/...` passes.
- The plugin bundle test suite passes all 12 tests, and `node --check ui/bundle.js` passes.

Web validation:

- TypeScript, ESLint with zero warnings, localization checks, Prettier, and the focused Vitest suite pass. The focused suite covers 24 tests across token usage utilities, the stats API and SPA routing.
- `pnpm run build` passes.

Documentation validation:

- Kandev document catalog validation, the 36 specification linter tests, full specification lint, local link checks and `git diff --check` pass in both repositories.

No installed-plugin browser E2E scenarios were added or run, and no live tokscale archive benchmark was run. The implementation uses deterministic unit and bundle fixtures and keeps undated historical coverage explicit when tokscale does not return source dates.

## Table refinement validation

The model/provider and daily/monthly refinement passed documentation validation on 2026-09-13.
The host package maps all 35 ACs. The plugin package retains all 11 collection/display ACs and 15 total mapped references.
Local links and whitespace checks passed in both packages.
Kandev catalog validation found 266 decisions and 822 specifications. All 36 linter tests and the full specification lint passed.
Commands ran from the Kandev repository root after an initial workspace-root invocation could not locate the specification configuration.
The E2E tables remain the follow-up browser coverage map. The checks listed above are the executed product and integration evidence.

## Risks

- Native event coverage and external lifetime coverage can differ. Tests must reject invented additive totals.
- Tokscale's dated export needs a pinned-version fixture before collector work. The bounded daily-query fallback preserves accuracy but delays historical import.
- Shared transcript identities can prevent external session attribution. The UI must expose that gap.
- Source-local daily aggregates cannot accurately split across arbitrary timezone boundaries.
- Both repositories must agree on the SDK version. Older hosts must report an upgrade requirement.
- PostgreSQL integration checks require the repository's database fixture environment.
