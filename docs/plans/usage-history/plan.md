---
created: 2026-10-28
status: in_progress
requirements:
  - REQ-COSTS-USAGE-HISTORY-001
  - REQ-COSTS-USAGE-HISTORY-002
  - REQ-COSTS-USAGE-HISTORY-003
  - REQ-COSTS-USAGE-HISTORY-004
  - REQ-COSTS-USAGE-HISTORY-005
  - REQ-COSTS-USAGE-HISTORY-006
  - REQ-COSTS-USAGE-HISTORY-007
system_design:
  - ../../specs/costs/system-design/usage-history.md
legacy_specs: []
---

# Implementation Plan: Retained Usage History

## Overview

Retain token and cost history independently of live tasks and collectors.
Capture trusted org/user attribution, resolve equivalent work once, and serve charts from durable daily summaries.
Expand storage first, preserve deletion safely, then migrate source selection, summaries, reads, and the plugin adapter.

The user accepted retention after archive/deletion, the three query improvements, generic collectors, and org/user attribution.
The assumption check reuses those decisions. Mixed or unknown attribution remains explicit.
Engineering choices retain the existing native ledger and org erasure contract, without adding a new permission or release flag.
There is no new product decision that blocks decomposition.

Source baseline: core `9aecf156c65e7522a43f2a1c8310737179c25d70`, plugin `127bc4a6e98f9121c216135b71c5e17aa8832cf1`.
These commits are investigation evidence, not a promise that their open PRs are merge-ready.
Reconcile intervening main changes before implementation, without discarding user edits.

Execution started after rebasing onto main `a33ff4139ad`.
That revision extracts the Go SDK into root `pluginsdk/`, module `github.com/kdlbs/kandev/pluginsdk`.
Work-order paths and verification must use that module layout. The user authorized six Luna/max workers and coordinator integration/review.

## Scope

### In scope

- [Requirements](../../specs/costs/requirements/usage-history.md), all seven REQ groups and their acceptance criteria.
- Retained native/external observations, immutable accounting subjects, trusted scope/actor evidence, and safe lifecycle integration.
- Source-neutral Host contracts, bounded indexed selection, durable summaries, SQL reporting, and compatible Session Cost adoption.
- Existing statistics presentation for deleted tasks, mixed/unknown attribution, projection preparation, and stale summaries.
- Restart-safe migration, SQLite/PostgreSQL parity, explicit erasure, and five-year performance evidence.

### Out of scope

- New protocol collectors, remote transcript transport, invoice reconciliation, currency conversion, and automatic expiry.
- New org-wide dashboards, access roles, or history-erasure controls.
- Implementing this package during its design turn, publishing plugins, merging PRs, or creating persistent Kandev subtasks.

## Technical approach

The [system design](../../specs/costs/system-design/usage-history.md) owns schemas and transaction boundaries.
The [ADR](../../decisions/2026-10-28-retained-usage-history.md) owns retention and projection rationale.

1. Add `usage_scopes` and `usage_subjects`. Extend the existing measurement/bucket tables with retained binding fields.
   Capture normalized native observations within `CreateTaskUsageEvent`, preserving its existing ledger/rollup transaction.
2. Backfill old native/external rows with durable keyset checkpoints. Gate task cascade on committed preservation.
   Register retained tables with workspace/org erasure and E2E reset. Do not expose deleted links as live tasks.
3. Extend root `pluginsdk/usage.go`, its `plugin.proto`, and `internal/plugins/host_data.go` additively.
   Separate source provenance from proven work identity. Match only indexed identity/coverage partitions.
   Add snapshot/keyset target discovery and authorized retained corrections.
4. Persist selected facts and daily summaries through durable dirty-partition work and atomic projection revisions.
   Rebuild shadow generations without replacing known history with partial results.
5. Move `ListSessionUsage` and saved canonical reads to retained facts/summaries.
   Apply range/provider filters in SQL, aggregate before database pagination, and maintain scoped existence metadata.
   Update current statistics DTOs and views for retained labels and preparation/freshness states.
6. Adapt the sibling plugin to the released contract. Preserve unknown dimensions, one subprocess, disabled behavior, and resumable import.

### Compatibility matrix

| Source or mode | Admission and identity | Intended result | Required evidence / unsupported fallback |
| --- | --- | --- | --- |
| Native ACP ledger | Existing event ID plus trusted task/session/turn binding | Atomic retained observation, unchanged task rollup | Duplicate/FK/delete races. Unknown actor stays unknown |
| Native Codex observations | Existing response/turn identity and category contract | Preserve exact versus fallback coverage | Response/fallback replay and reasoning-subset fixtures. Unknown schemas rejected |
| Session Cost / tokscale 4.15.1 | Authenticated plugin plus approved transcript/calendar binding | Lifetime and daily observations without duplicate totals | Sanitized nullable/provider/calendar fixtures. Missing provider remains unknown |
| Replacement collector fixture | Different plugin, same proven underlying work | One canonical contribution with separate provenance | Cross-collector equivalence test. Unproven overlap stays incomplete |
| Older supported plugin DTO | Existing live-task fields mapped by core | Additive compatibility, unknown new dimensions | Old/new wire and package tests. Explicit upgrade state if unsupported |
| Auth off / on / MultiTenancy | Existing synthetic identity / authenticated identity and org reach | Same isolation plus durable historical attribution | Mode matrix, guests/operators, suspension and erasure. Fail closed on missing scope |
| Deleted task/session | Core-issued retained binding and optional detached live links | Saved history and authorized corrections survive | Tombstone and ID-reuse tests. Arbitrary orphan IDs rejected |

### Existing companion packages

The earlier [core Token Usage package](../token-usage/plan.md) and
[plugin collection package](../../../../kdlbs-kandev-plugin-session-cost/docs/plans/token-usage/plan.md)
record earlier implementation, including its task-deletion policy.
Their completed results are historical evidence, not evidence for the new retention or scale contract.
This package owns new work orders and updates those packages with follow-up links only.
Task 06 owns the sibling plugin diff and records exact SDK pin and integration evidence here.

## ASCII UI preview

### UI-H01: Desktop retained task history

Entry: Statistics > Token Usage > Tasks, after ordinary task deletion.

```text
Statistics [Overview | Token Usage]   [Last Month] [Copy Stats]
Cost: $1.20 estimated     Tokens: 120,000     Updated: 2m ago
[Models | Daily | Monthly | Tasks | Sessions] [Provider: All]
Task                 Model          Tokens      Cost     Coverage
Deleted task         exact-model-id 120,000     $1.20    Complete
Archived live task >  exact-model-id  40,000     $0.40    Complete
```

Deleted rows have no task link. Archived live rows keep their existing navigation.
Amounts are illustrative. Preserve complete filtered totals and the current desktop control density.
Covers AC-COSTS-USAGE-HISTORY-001.1-.3 and 007.4.

### UI-H02: Phone retained task detail

Entry: navigation sheet > Stats > Token Usage. Reuse the existing phone breakdown list.

```text
Statistics
[Overview] [Token Usage]
[Last Month v]                       [Copy]
[View: Tasks v] [Provider: All v] [Sort v]
+------------------------------------------+
| Deleted task                             |
| 120,000 tokens     $1.20 estimated         |
| [Details]                                |
+------------------------------------------+
Details: Deleted task
  Model: exact-model-id   Provider: Unknown
  User attribution: Mixed users
  Input / Output / Cache read / Cache write
  Reasoning / Total / Calendar / Coverage
```

The detail control uses the existing touch-accessible surface. No action opens a deleted task.
One page body scrolls and clears safe areas. Keep 44px touch targets and keyboard focus return.
No new user/org selector or org-wide privilege is introduced.
Covers AC-COSTS-USAGE-HISTORY-002.3, 003.2 and 007.4-.5.

### UI-H03: Projection state variants, shared content

```text
First build:   Preparing usage history. [Retry status]
Later rebuild: Saved values remain visible. Updating history...
Read failure:  Could not load usage history. [Retry]
Details:       User attribution: Unknown
               Cost: Unavailable / estimated / reported
               Calendar: Source timezone when exact rebucketing is unavailable
```

Reuse existing summary, loading, error and detail primitives on both viewports.
All copy is localized. The first build never displays authoritative zero usage.
Covers AC-COSTS-USAGE-HISTORY-006.5-.6 and 007.5-.6.

## Tests

All names below are planned test additions unless an existing file is identified explicitly.

| Criteria | Test file and methods |
| --- | --- |
| 001.1-.5, 002.1-.4, 003.1-.7, 007.3 | `analytics/repository/sqlite/usage_history_test.go`: `TestUsageHistoryAdmission`, `TestUsageHistoryAttribution`; `task/repository/sqlite/usage_history_admission_test.go`: `TestUsageHistoryNativeAtomic` |
| 001.2-.4/.6, 002.5-.7, 007.1-.3/.7 | `usage_history_lifecycle_test.go`: `TestUsageHistoryDeleteRace`, `TestUsageHistoryPruneBeforeBackfill`, `TestUsageHistoryMigrationResume`, `TestUsageHistoryIDReuse`; org lifecycle test: `TestUsageHistoryOrgErasure` |
| 003.2-.5/.7, 004.1-.6 | `usage_history_selection_test.go`: `TestUsageHistoryCollectorReplacement`, `TestUsageHistoryUnknownDimensionOverlap`, `TestUsageHistoryDimensionCorrection`, `TestUsageHistoryIntervalIndex`; `plugins/host_usage_history_test.go`: `TestUsageHistoryHostScope`, `TestUsageHistoryTargetSnapshot` |
| 006.1-.7, 004.3, 007.1/.7 | `usage_history_projection_test.go`: `TestUsageHistoryProjectionCrash`, `TestUsageHistorySummaryCorrection`, `TestUsageHistoryGenerationRebuild`, `TestUsageHistoryGenerationLiveCorrection` |
| 005.1-.6, 006.5-.7, 007.4-.6 | `usage_history_queries_test.go`: `TestUsageHistoryRangeSQL`, `TestUsageHistoryColdCalendar`, `TestUsageHistoryMetadata`; `usage_history_bench_test.go`: `BenchmarkUsageHistoryFiveYears` |
| 007.4-.5, 003.7 | `apps/web/app/stats/token-usage-history.test.tsx`: `retained rows`, `projection states`, `large integer transport` |
| 004.1-.6, 003.1-.5, 007.1 | Plugin `server/usage_history_test.go`: `TestUsageHistoryAdapter`, `TestUsageHistoryImportResume`, `TestUsageHistoryOldHost` |

Add `TestUsageHistoryPostgres*` parity cases with `KANDEV_TEST_POSTGRES_DSN` configured.
Use real isolated PostgreSQL schemas through `internal/testutil`, including concurrent first insert, correction, deletion, and non-UTC sessions.
Do not report skipped database tests as passed parity.

## E2E tests

| File / project | Flow and criteria |
| --- | --- |
| `layout/stats-usage-history.spec.ts` / chromium | Native + external history survives archive/delete, exact totals and disabled links: 001.1-.5, 007.4 |
| `layout/mobile-stats-usage-history.spec.ts` / mobile-chrome | Same totals, detail metrics, unknown/mixed states, touch/focus/overflow: 002.3, 003.2, 007.4-.5 |
| `auth/usage-history-scope.spec.ts` / auth | Two users/orgs, forged source scope, deleted work, suspension and erasure: 001.6, 002.1-.7, 004.5 |
| `layout/stats-usage-history.spec.ts` / chromium | Rebuild/crash leaves old values or preparation status, retry and Copy Stats agree: 005.3/.6, 006.3-.6, 007.1/.7 |
| Plugin `test/host-action-smoke.spec.mjs` and `test/mobile-session-cost-action-smoke.spec.mjs` | Compatible Host, collector replay/replacement, saved canonical reads, missing provider/actor, import restart: 003.1-.5, 004.1-.6 |

Task 05 owns native-page/browser evidence. Task 06 owns installed-plugin evidence.
Use deterministic transcript fixtures. Run affected browser cases with retries disabled.

## Work orders

- [ ] [Task 01: Capture retained usage and ownership](task-01-retained-admission.md)
- [ ] [Task 02: Preserve history through deletion and migration](task-02-lifecycle-migration.md)
- [ ] [Task 03: Normalize collector identities and indexed selection](task-03-collector-contract.md)
- [ ] [Task 04: Maintain canonical daily summaries](task-04-daily-projection.md)
- [ ] [Task 05: Serve retained statistics with bounded SQL](task-05-retained-reporting.md)
- [ ] [Task 06: Adopt the generic contract in Session Cost](task-06-session-cost-adapter.md)

Dependency order: 01 -> 02 -> 03 -> 04 -> 05 -> 06.
The integration handoffs follow this order because they share schema, projection rules, and SDK contracts.
Independent phases may overlap when their write sets are disjoint.
Task 03's tested SDK phase allows Task 06 adapter and fixture tests while core lifecycle work continues.
Task 05 may implement frontend presentation and component tests against additive DTO fields while Task 04 owns backend projection work.
Its SQL mapping, benchmarks, and browser runs wait for the tested projection handoff.
Real Host validation, browser evidence, and the final immutable SDK pin remain gated on core integration.
Task 04 preparation identified selector and replay dependencies; its implementation waits for those handoffs.

## Verification results

Implementation: In progress. The design-package results below predate code changes.
Each work order's Results owns its actual implementation and test evidence.
Tasks 01 and 02 reopened for scoped lint cleanup found during Task 03 integration.
Their original workers completed helper extractions, then Task 03's combined lint, race, real PostgreSQL, and static checks passed.
Tasks 01 through 03 have initial implementation/test handoffs. Coordinator review is checking components after their own tests pass.

Coordinator review on 2026-10-29 reopened Task 03 after its tested handoff.
Shared transcripts across subjects need one accounting contribution, and a fresh binding must not assign earlier organization usage to a new owner.
Collection-target discovery must also bound metadata refresh per request.
The original contract worker owns these corrections. The plugin worker owns the related cache, import-state, startup-scan, and source-day rollover findings.
Their earlier passing receipts remain recorded; they do not close the new findings.
The full scale retry and final browser, review, push, and CI gates remain open.

Review also reopened Task 02: preservation must advance to the last copied row, rather than the end of all remaining rows.
The lifecycle fix now passes SQLite and PostgreSQL race tests with 451 native events, measurements, and dated buckets each.
The optional state-key API and bounded plugin cleanup also pass their focused tests. Final Host integration remains open.
Task 01 reopened for native turn-count and reasoning-subset corrections, including repair of already published values.
Task 03 also needs distinct model groups to survive selection, and current source checks must avoid archive-wide lookup.
Task 05 is correcting client aggregation that can recreate an unavailable server rate. Its final checks remain open.

Native turn/reasoning integration now passes 13 focused cases normally and under race on SQLite and PostgreSQL 18.6.
The client rate and numeric-sort corrections also pass their focused tests, lint, and typechecking.
Task 04 reopened for model-segment selection and projection repair. New sessions must not reindex unrelated history.
The projection worker owns this work; the lifecycle worker owns bounded current-target lookup and source-scope safety.
The plugin passes its full Go/race suite against the typed eligibility/preparation checkpoint.
It still needs the final lookup/segment contract, actual Host proof, immutable source pin, packaging, and browser checks.

At the 2026-10-29 15:12 UTC checkpoint, shared usage labels distinguish unallocated work from deleted subjects.
The focused component cases, lint, typechecking, and locale checks pass. Five public and contract guides also pass validation.
The core's focused frozen-transcript and targeted context-repair cases pass; final source-selection integration remains open.
Current-target privacy and erasure cases pass on PostgreSQL, while an actual SQL-visit regression still requires correction.
The plugin's point/explicit-segment checkpoint passes its full Go and race suites.
Ordinary Tokscale group declarations and ambiguous membership handling remain in progress after verification of the pinned producer source.
The complete benchmarks, browser cases, final rebase, source pin, package, pushes, and current-head CI remain required.

The 16:02 UTC checkpoint closes the focused lookup test gate: race cases pass on SQLite and PostgreSQL 18.6.
Actual SQL visits remain constant after adding 902 unrelated sessions. Full frontend and normal documentation validation also pass.
The plugin's producer checkpoint passes 152 Go functions and 105 subtests in normal and race runs, plus 58 UI cases.
Core coverage-window selection now preserves disjoint days across collectors in both databases, including the oversized path.
Review still requires fixes for unknown lifetime cost, selected-cost estimate metadata, and canonical metric applicability across the Host boundary.
The plugin consumer must distinguish an absent metric from an unknown value through the typed contract.
Core production remains unfrozen, and every final integration and delivery gate remains open.

Coordinator integration checkpoint on 2026-10-29:

Component implementations have tested, reviewed handoffs. Integration remains in progress.
The full analytics package passed 390 test/subtest results before the final lifecycle and transfer corrections.
Those corrections passed focused normal and race cases on SQLite and PostgreSQL 18.6.
They protect uncopied native records, stale completed checkpoints, and captured ownership after actual task transfers.
The migration fixture retains its original assertions and now also runs on PostgreSQL.
The standalone SDK, frontend components, plugin, and persistence conformance have passing component receipts.
Small benchmark fixtures pass on both engines through real admission and production publication.
The large five-year runs, fresh browsers, final SDK pin/packages, primary rebase, pushes, and current-head CI remain required.
Earlier failed receipts remain historical evidence and are not relabeled as passes.

Package checks on 2026-10-28:

- `python3 scripts/list-docs.py validate`: passed. New requirement/design and ADR are discoverable.
- `python3 scripts/lint-spec-files.py --all`: passed, including the reduced legacy size ceiling.
- `.github/scripts/pr-docs.cjs.validateCoverage`: covered, six accepted work orders, zero errors.
  The local preflight supplied the six new work orders and a planned analytics-code trigger to exercise coverage rather than exemption.
- Local reference/dependency audit: all 46 ACs have work-order coverage. Links resolve and every dependency precedes its task.
- `git diff --check -- docs` and the sibling plugin's `git diff --check`: passed.
- `git status --short -- docs/plans/usage-history`: confirms the new package remains unstaged and uncommitted.

Independent design review identified pruning, corrected dimensions, unknown-dimension overlap, rebuild catch-up, org erasure, actor binding, and calendar risks.
The design and work orders now specify those boundaries and regression scenarios.

## Risks

- Existing native and external rows can disappear during backfill unless deletion admission protects them.
- Work-ID equivalence, actor identity, or source calendars can be unavailable. Unknown and ambiguous states must remain explicit.
- Native retention shares a transaction with existing ledger retry and rollup rules. An import cycle or second writer would violate that boundary.
- Summary updates require database concurrency protection and crash tests, not only a process mutex.
- An older binary can cascade-delete uncopied rows. A read rollback alone is not a safe binary downgrade.
- Years of facts increase storage. Benchmarks must distinguish ingestion, projection, indexed reads, and expensive transcript collection.
- Existing core/plugin PR conflicts and CI failures remain separate delivery gates. This package does not declare them resolved.
