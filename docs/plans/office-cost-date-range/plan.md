---
created: 2026-10-13
status: implemented
requirements:
  - REQ-OFFICE-COSTS-001
system_design:
  - ../../specs/office/system-design/costs-01.md
legacy_specs: []
---

# Implementation Plan: Office Cost Date Range

## Overview

Make the existing MTD and Last 30 days actions select the event-time window for
the displayed total, event counts and every existing breakdown. One sequential
work order carries the range from the actual component through the real API,
HTTP service and SQL queries. The existing Office cost requirement owns this
repair; no parallel repair or UI specification is introduced.

Implemented after the later explicit request in the same primary session,
under the granted exclusive command lease. The one work order is complete.

## Confirmed cause and assumptions

`CostOverview` refetches when range changes but calls
`getCostsBreakdown(workspaceId)` without range. The API wrapper, Gin handler,
`CostService.GetCostsBreakdown` and all five repository queries use lifetime
rows. ROOT reported independent real SQLite/service/HTTP and actual
component/API reproductions: lifetime $6, MTD incorrectly $6 instead of $1,
30 days incorrectly $6 instead of $4. These are parent-reported evidence;
permanent regressions will be authored independently from public repository
patterns, without inspecting ROOT's private fixtures.

The local HEAD at design start was `e8cf123d03a91a51b2031e796df9e2cee26ce29e`.
A scoped comparison with qualified main
`2ab7324f62dbfff9b01ab50bbf325735bc3e11dc` found no changes in the owning
requirement/design or affected cost production paths. The inherited UTC month
convention and existing half-open spend queries support the chosen boundaries.
Under autonomous planning authority, Last 30 days means 720 elapsed hours,
not 30 civil dates. Empty or repeated supplied ranges are validation errors.

## Scope

### In scope

- AC-OFFICE-COSTS-001.2/.8: one UTC interval for the total and agent, project,
  model and provider sums, counts and membership.
- Optional `range=mtd|30d` on the composed breakdown endpoint. Omission retains
  lifetime compatibility; invalid supplied values return HTTP 400.
- Shared range-aware component/API actions and live refetch.
- Meaningful real SQLite/Gin, component/real API, fixed window and env-gated
  PostgreSQL query regressions.

### Out of scope

Schema/migrations, pricing, budget evaluation/claims/admission, ledger or disk
runner writes, session rollups, workspace/run-session ownership, attribution or
labels, forecasting, new pickers, UI layout/copy, touch/navigation changes,
settings preservation, and general cost-system cleanup.

## Technical approach

Follow [Costs Explorer request windows](../../specs/office/system-design/costs-01.md#costs-explorer-request-windows).
Add the narrow range-aware service entry point, resolve one `now` before the
five existing concurrent reads, and extend their SQL through shared private
helpers and explicit `*InWindow` methods. Existing lifetime and budget-facing
methods retain their signatures and semantics. No transaction snapshot promise
is added. Use bound UTC timestamps and `r.ro.Rebind` on both SQL engines.

The frontend optional request-options extension preserves the existing
`getCostsBreakdown(workspaceId, options?)` call shape. Pass the selected range
from the shared component and capture it in the fetch callback dependencies.
Retain the current response guard and error handling.

| Consumer/path | Intended behavior | Evidence |
| --- | --- | --- |
| Composed endpoint, range omitted | Lifetime response, unchanged shape | Real SQL/Gin compatibility case and real API URL/options test |
| Composed endpoint, mtd/30d | Shared half-open occurred_at interval | Real SQL/Gin totals, all dimensions/counts, exact SQL bounds |
| Composed endpoint, invalid/empty/repeated | HTTP 400 before cost query | Real Gin invalid-query cases |
| Separate summary/group endpoints and budget reads | Existing unfiltered/period behavior | Existing affected Go package tests |
| SQLite/PostgreSQL | Same query result and grouping | SQLite suite; isolated env-gated PG query twin |
| Desktop/phone CostOverview | Same selected range through shared action | Actual component buttons, real API wrapper/intercepted fetch |

## Mobile-parity exemption

The closest shipped surface is this same `CostOverview`: one shared control
row and responsive metric grid consume the same result on desktop and phone.
This is data normalization only. Layout, touch targets, navigation, breakpoints,
scrolling, copy and control order do not change. Meaningful shared component,
API and real SQL/HTTP tests cover the changed behavior. No new mobile E2E build,
capture or layout gate is required; no ASCII layout preview is needed.

## Tests

| Criteria | Permanent evidence to add |
| --- | --- |
| AC-001.2/.8 | `handler_range_test.go`: `TestCostsBreakdownRangeHTTP`, real SQLite + CostService + Gin; distinguish lifetime/MTD/30d and verify all four groups/counts |
| AC-001.2 | `range_test.go`: `TestCostRangeWindow`, fixed UTC calendar/offset/leap-year/720h boundaries; `TestCostsBreakdownRangeSharedWindow`, identical bounds for five reads |
| AC-001.2 | `costs_window_test.go`: `TestCostsWindowQueries`, exact lower/upper bounds, event versus ingestion time, foreign workspace/taskless runs, live project/deleted profile and all memberships/counts |
| AC-001.2 | `costs_window_postgres_test.go`: `TestPostgresCostsWindowQueries`, isolated real PostgreSQL query parity when DSN is available |
| AC-001.8 | `cost-overview-range.test.tsx`: real component/API/fetch chain, click MTD/30d, rendered sums/counts/groups and live-refetch selected range |
| AC-001.2/.8 | `office-api.test.ts`: range URL, omitted URL and existing transport options/signal |

Test names above are planned, not existing execution evidence. New suites must
contain behavioral assertions; compiler failures alone do not count as RED.

## E2E compatibility

The existing `e2e/tests/office/costs.spec.ts` live project-refetch flow uses
`waitForHttp` with `/\/costs\/breakdown$/`. That helper tests `URL.pathname`,
so a query does not affect it. Run only its existing focused helper case
`resolves on a matching method and path, ignoring the query string` as the
required matcher check. No E2E source change or new browser build gate is
planned. The real component/API/fetch and SQL/HTTP paths provide the end-to-end
data evidence for this bounded repair.

## Public documentation audit

Search of `docs/public`, root README and screenshot catalog found no published
Office Costs Explorer or breakdown-range contract. README and feature-status
explicitly describe Office costs/budgets as evolving and disabled in production.
This package changes implementation intent only. Keep the supported-feature
boundary; no public docs or screenshot change is needed. Re-audit only if the
actual implementation changes a documented public contract.

## Work orders

- [x] [Task 01: Wire and verify the selected cost window](task-01-cost-window.md)

## Verification results

Design checkpoint, 2026-10-13:

- `python3 scripts/list-docs.py validate`: exit 0, 379 decisions and 1578 specs.
- `python3 scripts/lint-spec-files.test.py`: exit 0, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: exit 0.
- Tracked-document `git diff --check`: exit 0; new work orders are also checked
  separately because ordinary git diff does not inspect untracked files.
- Repository `validateCoverage`: actual four-doc inventory is `exempt`, no
  errors; anticipated six production paths plus seven test paths and four docs
  are `covered` by this single work order and REQ-OFFICE-COSTS-001/.2/.8.
  This proves document links, not implementation or execution coverage.

ROOT review corrections: the sole work order now requires `-tags fts5`,
`-p=1`, Go 1.26 with `GOTOOLCHAIN=local`, `GOMAXPROCS=2`, and
`GOMEMLIMIT=512MiB`; finite GNU timeouts/10-second kill grace and actual handle
joins; and staging every actual owned new helper/test before the single scoped
Go lint as well as frontend/i18n lint. These are command corrections only;
requirement/design semantics and the existing catalog/spec receipts are unchanged.
Correction-delta light command exited 0: four shell blocks passed `bash -n`;
assertions confirmed fts5/resource/time bounds, one frozen install and staging
before the single scoped Go lint. Coverage preflight remained actual four-doc
`exempt` and separate anticipated 13 paths plus four docs `covered`, no errors;
tracked and new-document whitespace checks were clean. No heavy command ran.

Implementation complete before publication. The
[work-order results](task-01-cost-window.md#results) record exact causal RED and
GREEN exits and original joined handles. Both affected Go packages passed with
race/trimpath/fts5/p1; three frontend suites passed 10 tests, followed by the
changed component test after required lint-label corrections. Typecheck,
scoped Go lint, zero-warning frontend lint, the focused query matcher, i18n
ratchet/check and document checks passed. At initial publication, real
PostgreSQL parity was unverified locally: its env-gated test explicitly skipped
without a DSN. The required PostgreSQL CI correction below supersedes that skip.

All 16 actual changed files (six production, six tests, four docs) were staged
before changed-line lint, including every new helper/test. The final index
preflight validates their one-order traceability. Go 1.26.0, Node 24.21.0 and
pnpm 9.15.9 were used, with one frozen install and bounded command handles.
No UI composition/copy or public-doc change was introduced; the data-only mobile
exemption and unchanged E2E pathname matcher apply. Normal active hook/commit
and publication receipts are recorded in the task plan.

The required frontend review correction is complete before its corrective
publication: pending/failed range switches cannot display another window's
spending. The existing accepted-result guard now pairs workspace and range.
Its causal real component/API RED and six-test GREEN, strict lint, typecheck,
i18n and minimal design clarification are recorded in the same work order.
This adds no Go change, new design package, layout/copy or public-doc work.
The original hosted observer and deadline are retained; optional backend
suggestions were dispositioned without published backend changes.

The required PostgreSQL CI correction uses an owned PostgreSQL 16 instance to
execute the existing window assertions. The fixture now seeds the parent agent
and inserts portable SQL event rows, leaving the recording path unchanged.
PostgreSQL also rejected the provider label's raw-column fallback outside its
grouped expression; the fallback now uses that same canonical expression, with
unchanged labels and memberships. Real PostgreSQL and SQLite window assertions
and existing lifetime/provider-label controls pass with race detection. Required
full Go changed-line lint passed with zero issues after the evidenced timeout
calibration recorded in the work order. The owned tmpfs PostgreSQL container
was removed after those tests. Normal hooks, final indexed coverage and the
single corrective publication follow these completed local checks.

## Risks

- Changing only a subset of queries could mix intervals; assert all five bounds
  and every dimension's sum/count, not just the displayed total.
- Filtering ingestion time would silently include old imported usage; seed
  opposing occurred_at/created_at values.
- SQLite success does not prove PG query execution. Report DSN skips explicitly;
  use isolated fixtures and portable SQL without altering foreign resources.
- Existing concurrent reads can observe writes between queries. This repair
  guarantees one temporal interval, not a new transaction isolation contract.
- New untracked test/helper paths must enter the actual staged inventory before
  lint/changed-line claims. Preserve normal hooks and all foreign resources.
