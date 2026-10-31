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

### Planned implementation sequence (historical)

Dependency order: 01 -> 02 -> 03 -> 04 -> 05 -> 06.
The integration handoffs followed this order because the tasks share schema, projection rules, and SDK contracts.
Independent phases may overlap when their write sets are disjoint.
Task 03's tested SDK phase allowed Task 06 adapter and fixture tests while core lifecycle work continued.
The plan allowed Task 05 to implement frontend presentation and component tests against additive DTO fields while Task 04 handled backend projection work.
Task 05's SQL mapping, benchmarks, and browser runs were gated on the tested projection handoff.
Real Host validation, browser evidence, and the final immutable SDK pin remain open under the remaining integration gates described below.
Task 04 preparation identified selector and replay dependencies; its current implementation and validation status is summarized below.

## Verification results

### Current validation status (2026-10-30, NOT FINAL)

The validated core source revision is `6a4b002a22ba959dbd636f2fb42eefc31c7dafd2` on exact main `aebac5d5395e9983124b398198bace146d38506b`.
The SDK tree is `a9a0a09dc8b30ac02876ccc7e46eb18102079600`.

A6 replayed 97 feature commits onto main. Ninety-six were range-equal, and one persistence-catalog commit adapted automatically.
The replay dropped zero commits and had zero conflicts. All 550 original feature paths remain.
The catalog keeps `workspace_collaboration_grants` and every usage feature table.

Commit `f306b7081105c231f56e309e18378372b8c6472d` classifies background `UpsertSessionUsage`, `ListSessionUsage`, `ListUsageCollectionTargetsV1`, `ReportUsageObservationsV1`, and `ListStateKeys` calls as plugin-owned capability calls.
Exact task usage remains caller-bound. Capability checks, workspace visibility, source ownership, and binding checks remain active.
Normal source commit `7d5fd6ee76cb6791fc2fa77b8dd6c87582371cf7` replayed range-equal as validated source `6a4b002a22ba959dbd636f2fb42eefc31c7dafd2` on A6.
The integration control reports all four C4 source files and the 15 A6 scoped files match their recorded hashes.
The four C4 source findings remain closed: provider query projection, undated interval providers, bounded resumable migration and per-work pages, and snapshot revision invalidation.

At A6 HEAD `f306b7081105c231f56e309e18378372b8c6472d`, the full SDK, standalone consumer, and focused Host authorization, state-key, task-deletion, archive-borrower, native-publisher, continuation, and caller checks passed.
At current HEAD `6a4`, the integration control records an SDK race pass for three packages. The generated protobuf package has no tests.
The first analytics attempt did not export the PostgreSQL DSN. Required PostgreSQL cases failed, and optional cases skipped, so that attempt is invalid.
The corrected run used an exported, isolated PostgreSQL 18.6 DSN and passed all seven analytics packages.
It recorded 463 passing tests and subtests, with zero failures and zero skips. Three packages had tests, and four had no tests.
Receipt: [integrated source verification](evidence/usage-history-integrated-20261030/verification.json), including the analytics event log and SDK race log.

| Task | Current status | Evidence or open gates |
| --- | --- | --- |
| 01 — Retained admission | Included in the integrated source. | Seven-package analytics validation passed on PostgreSQL 18.6. Full-scale and delivery gates remain open. |
| 02 — Lifecycle and migration | Included in the integrated source. | Full-scale measurements and storage/resource review remain open. |
| 03 — Collector contract | Included in the integrated source. | Final SDK pin and actual Host integration remain open. |
| 04 — Daily projection | Included in the integrated source. | Full SQLite and PostgreSQL five-year runs remain open. |
| 05 — Retained reporting | Included in the integrated source. | Current frontend build, browser evidence, screenshots, and scale review remain open. |
| 06 — Session Cost adapter | In progress. | Final SDK pin, five rebuilt archives, actual Host cases, and installed browser checks remain open. |

Recorded passes apply only to their stated source and scope. They do not close the benchmark, package, Host, browser, or delivery gates.

### Full five-year measurement status (PENDING)

Both full runs remain pending on the current source: SQLite and real PostgreSQL 18.6, each with 1,000,000 native rows, 100,000 external rows, and ten samples.

| Required run | Status |
| --- | --- |
| Current-source SQLite, 1,000,000 native and 100,000 external rows, ten samples | Pending. |
| Current-source PostgreSQL 18.6, same fixture and sample count | Pending. |
| C4 construction diagnostics, SQLite | Complete at 20,000/2,000 rows in 59.597 seconds and 100,000/10,000 rows in 664.684 seconds. |
| 100,000/10,000 actual rows | 100,005 native, 10,013 external, 110,017 facts, and 220,032 contributions. The recorder dropped zero SQL statements. |

The completed coverage trace made 100,012 calls and recorded 1,250,150,022 row visits in 245.712 seconds.
Native pending repair made 552 calls and recorded 55,102,763 visits in 129.948 seconds.

The diagnosis traced the quadratic coverage cost to overlap query access. Disjoint point-start access and a coarse-interval `coverage_end` partial index reduce the measured lookup work.
Overlap responses contain at most 201 references. Complete result sets of at most 200 references are sorted in Go; larger sets return `Preparing` before consumption.
Ordered scalar first-row probes now handle pending-accounting checks. SQLite uses its existing-index hint, and PostgreSQL uses its default planner.
The fix adds no readiness fast path. Source, count, oracle, and benchmark deadline contracts remain unchanged.
C4 diagnostics do not grant C-ready or full-performance approval.

SQLite and PostgreSQL passed exact result parity for native events, coarse intervals, and touching boundaries.
The proof includes two distinct native events at one point. SQLite normal and race scan proofs passed.
Focused repair and oversized-resume cases passed normally on both engines. Focused race passed on SQLite, and scoped lint and vet passed.
The PostgreSQL coarse case read 53 rows after the fix, versus 52 before. Buffer use fell from 26 to 20.
Per-query results are in [the C4 overlap proof](evidence/usage-history-luna-C4-overlap-fix-20261030T2216Z/README.md).

Temporary profiling touched nine test-only files. The patch is archived at `evidence/usage-history-luna-C4-construction-diagnosis-20261030T2040Z/diagnostic-instrumentation.patch`.
The original seven committed harness files were restored byte-for-byte.
Two untracked profiling tests were archived. The preservation record is `/tmp/usage-history-C4-diagnostic-source-preservation.json`, and its patch check passed.
The tagged package compile with the restored harness and scan proof passed without running tests. Receipt: `/tmp/usage-history-C4-restored-harness-compile-final.log`.
The parent corrected an initial cross-filesystem move error. Backups remain intact, and this orchestration error was not a product fault.

### Earlier source-frame receipts (historical)

At core HEAD `90767205f97f4b8268526255b935c3626c1a07fe`, seven analytics packages passed normally in 185.084 seconds with an actual PostgreSQL DSN.
Fifteen tagged cases passed on both database engines in 268.785 seconds. Thirty compatibility bridge, user-store, and session-list cases also passed.
The parent harness linted 205 files, passed 19 harness tests, and passed 36 spec tests. These results apply to `907`, not current HEAD `6a4`.

C3 failed at page 2,453 after 1,681 seconds and produced zero samples. The C3 failure receipt remains unchanged.
The C4 run on core `907` stopped after 48 minutes 4 seconds. Its recent phase published 200,005 native rows, 20,013 external rows, 220,017 facts, and 440,032 contributions.
That phase recorded 64,362 calls over 2,201.321 seconds, with ten samples for each of 16 recent cases.
A later phase inserted 300,160 native rows but published no result. It inserted zero older external rows, and the 1,000,000/100,000 phase did not complete.
The interrupted C4 receipt remains unchanged. It is incomplete and is not a current-source full run.

The C2 SQLite attempt also ended at its deadline with zero samples. Its scaled `seedRecent` control expected 2,010 external rows and recorded 2,013.
The C4 dense plan fixture had 50,001 facts and 100,002 contributions. Of those facts, 401 came from real admission; the remaining rows were plan-only.
The bounded query recorded 200 fact visits and 400 contribution visits on each engine.
The C2 receipts remain in `evidence/usage-history-luna-C2-20261030T1032Z/` and `evidence/usage-history-luna-C2-20261030T1211Z/`.

At this prepublication checkpoint, the new core and plugin source remain unpushed. Public core and plugin heads are `9aecf156c65e7522a43f2a1c8310737179c25d70` and `127bc4a6e98f9121c216135b71c5e17aa8832cf1`.
The parent plans to publish reviewed core after integrated checks, so CI can run alongside the remaining manual gates.
Acceptance remains unchanged. Merge readiness requires both full runs, the final SDK pin, and five rebuilt archives.
It also requires five actual Host cases, older-host checks, and browser evidence. Current-head CI must pass with zero flaky artifacts.
The final current-head review audit remains pending.

### Earlier coordinator review (historical, superseded)

A3 prepared candidate `b43760b` over main `8380430` by replaying 96 patches. Ninety-four patches were identical. Two conflict resolutions regenerated the protobuf descriptor and retained both Automation history and Retained usage rows in the authoring guide. At that checkpoint, the parent harness passed 19 tests, linted 204 harness files, passed 36 spec-linter tests, and passed full spec lint. The current source and remaining gates are listed above.

I3's scoped reruns for the old CI frontend failures passed two navigation tests and 108 scroll tests. Scoped ESLint had no diagnostics, so no source edit was needed. Those checks did not replace the full frontend suite, build, or current-head CI.

H3/K3/J3 prepared installed acceptance cases for authorization, pruning, ownership transfer, erasure, and plugin-state privacy. Those preparations and syntax checks were not installed Host passes. J4/L3 were adapting the live smoke helper at that checkpoint.

### Earlier integration checkpoint (historical)

The earlier current-main integration checkpoint recorded full analytics, focused lifecycle, persistence conformance, backend static checks, and backend build passes. Frontend component tests, typechecking, lint, localization, and a production build also passed. Browser runs then exposed fixture/selector defects and a stamped-turn transaction defect. The reviewed corrections preserved the original accounting and authorization assertions. Focused helper tests and normal/race backend regressions passed; fresh browser validation was still pending at that checkpoint.

A prior rebase candidate preserved 94 patches and passed its incoming pull-request fetch regression. A3 later prepared the 96-patch candidate described above. These historical notes do not describe the current source frame. Earlier checkpoints below are historical evidence, not current completion claims.

Overall implementation remains in progress. The design-package results below predate code changes. The dated notes below preserve earlier states and do not override the current snapshot above.

### Historical implementation and review notes

Each work order's Results owns its actual implementation and test evidence.
Tasks 01 and 02 reopened for scoped lint cleanup found during Task 03 integration.
Their original workers completed helper extractions, then Task 03's combined lint, race, real PostgreSQL, and static checks passed.
Tasks 01 through 03 had initial implementation/test handoffs. Coordinator review then checked components after their own tests passed.

Coordinator review on 2026-10-29 reopened Task 03 after its tested handoff.
Shared transcripts across subjects need one accounting contribution, and a fresh binding must not assign earlier organization usage to a new owner.
Collection-target discovery must also bound metadata refresh per request.
At that time, the contract worker owned these corrections. The plugin worker owned the related cache, import-state, startup-scan, and source-day rollover findings.
At that checkpoint, earlier passing receipts had not closed the new findings. C4 later closed the source findings listed in the current snapshot above.
The full-scale retry and final browser, review, push, and CI gates were still open at that checkpoint.

Review also reopened Task 02: preservation had to advance to the last copied row, rather than the end of all remaining rows.
The lifecycle fix passed SQLite and PostgreSQL race tests with 451 native events, measurements, and dated buckets each.
The optional state-key API and bounded plugin cleanup also passed their focused tests. Final Host integration remained open at that checkpoint.
Task 01 reopened for native turn-count and reasoning-subset corrections, including repair of already published values.
Task 03 also needed distinct model groups to survive selection, and current source checks had to avoid archive-wide lookup.
Task 05 was correcting client aggregation that could recreate an unavailable server rate. Its final checks remained open at that checkpoint.

Native turn/reasoning integration passed 13 focused cases normally and under race on SQLite and PostgreSQL 18.6.
The client rate and numeric-sort corrections also passed their focused tests, lint, and typechecking.
Task 04 reopened for model-segment selection and projection repair. New sessions must not reindex unrelated history.
The projection worker owned this work; the lifecycle worker owned bounded current-target lookup and source-scope safety.
The plugin passed its full Go/race suite against the typed eligibility/preparation checkpoint.
It still needed the final lookup/segment contract, actual Host proof, immutable source pin, packaging, and browser checks at that checkpoint.

At the 2026-10-29 15:12 UTC checkpoint, shared usage labels distinguished unallocated work from deleted subjects.
The focused component cases, lint, typechecking, and locale checks passed. Five public and contract guides also passed validation.
The core's focused frozen-transcript and targeted context-repair cases passed; final source-selection integration remained open.
Current-target privacy and erasure cases passed on PostgreSQL, while an actual SQL-visit regression still required correction.
The plugin's point/explicit-segment checkpoint passed its full Go and race suites.
Ordinary Tokscale group declarations and ambiguous membership handling were still in progress after verification of the pinned producer source.
The complete benchmarks, browser cases, final rebase, source pin, package, pushes, and current-head CI were still required then.

The 16:02 UTC checkpoint closed the focused lookup test gate: race cases passed on SQLite and PostgreSQL 18.6.
Actual SQL visits remained constant after adding 902 unrelated sessions. Full frontend and normal documentation validation also passed.
The plugin's producer checkpoint passed 152 Go functions and 105 subtests in normal and race runs, plus 58 UI cases.
Core coverage-window selection then preserved disjoint days across collectors in both databases, including the oversized path.
Review then required fixes for unknown lifetime cost, selected-cost estimate metadata, and canonical metric applicability across the Host boundary.
The plugin consumer had to distinguish an absent metric from an unknown value through the typed contract.
Core production was unfrozen, and final integration and delivery gates remained open at that checkpoint.

The coordinator integration checkpoint on 2026-10-29 recorded:

Component implementations had tested, reviewed handoffs. Integration remained in progress.
The full analytics package passed 390 test/subtest results before the final lifecycle and transfer corrections.
Those corrections passed focused normal and race cases on SQLite and PostgreSQL 18.6.
They protect uncopied native records, stale completed checkpoints, and captured ownership after actual task transfers.
The migration fixture retains its original assertions and now also runs on PostgreSQL.
The standalone SDK, frontend components, plugin, and persistence conformance have passing component receipts.
Small benchmark fixtures pass on both engines through real admission and production publication.
The large five-year runs, fresh browsers, final SDK pin/packages, primary rebase, pushes, and current-head CI were still required at that checkpoint.
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


### CI and cost-point correction

The functional PostgreSQL fixtures now use the existing optional-DSN helper in SQLite-only jobs. Required PostgreSQL jobs still execute the same assertions. Functional projection cases accept the supported PostgreSQL 16 baseline; the separate PostgreSQL 18 performance gate is unchanged. Both setup failures were reproduced before the correction. The focused SQLite run passed 39 test results, with 20 PostgreSQL skips. PostgreSQL 16.14 passed 43 results under race detection with no skips. Evidence is in `evidence/usage-history-ci-fixtures-20261030/`.

PostgreSQL timestamp precision collapsed a point's one-nanosecond interval, so a matching reported price was discarded. The selector now recognizes equality for explicit point coverage. Ending interval boundaries remain exclusive. Matching and unrelated prices, unknown and measured-zero costs, partial coverage, currency and actor cases pass normal and race checks on SQLite and PostgreSQL 18.6. Evidence is in `evidence/usage-history-cost-point-20261030/`. The retained projection is new in this PR, so released databases need no repair migration.

These focused passes do not close delivery. The current CI artifact audit found three browser flakes after retry. Their root causes, fresh remote checks, installed-plugin flows and both complete performance gates remain open.


### Browser fixture verification

The exact-head retry audit found three flaky tests despite successful E2E jobs. The Git fixture now pauses the active content source atomically and verifies HTTP 404 before restore and HTTP 200 after restore. The history deadline fixture waits for required persistence and publication to recover before UI Retry; it preserves both error cards and saved values until then. The mobile initial-load fixture waits for all three workload rows to persist before navigation. No product UI or timeout changed.

Reviewed local checks use one worker and zero retries. The Git case passed six runs across fixture iterations. History coverage passed five focused repetitions, all five desktop cases, the affected phone case and a final focused assertion check. The mobile case passed six repetitions and its two-case predecessor sequence. Evidence is in `evidence/usage-history-ci-browser-fixup-20261031/`. The mobile failed attempt has no trace, so its exact event ordering remains unknown. The fix preserves initial snapshot coverage and does not claim to repair a production subscription gap. Fresh full CI and its no-flake audit remain required.

### Browser follow-up verification

CI on `863769e` passed 95 checks and skipped 12, but its retry audit found four flaky tests.
The publication helper now retries only known HTTP 503 readiness errors within its existing deadline.
Direct reads and unknown failures remain strict.
The Office fixture clears claimed runs before each permission case.
Eight preceding setup cases reproduced the original scheduler queue timeout before this correction.

The subtask fixture waits for one popover to close before it opens the next.
This race did not reproduce locally, so its cause remains an inference.
The phone fixture waits for menu animations before it measures the unchanged touch-target threshold.
The original phone test failed four of five local runs.

Reviewed browser runs passed with zero retries, including the Office predecessor sequence and twelve phone cases.
The helper unit tests passed all six cases.
Evidence is in `evidence/usage-history-ci-browser-followup-20261031/`.
These corrections change test fixtures only; production code and test deadlines remain unchanged.
Fresh CI, a zero-flake artifact audit, and both full performance gates remain required.
