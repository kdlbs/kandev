---
id: "05-retained-reporting"
title: "Serve retained statistics with bounded SQL"
status: in_progress
wave: 5
depends_on: ["04-daily-projection"]
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-HISTORY-001
  - REQ-COSTS-USAGE-HISTORY-002
  - REQ-COSTS-USAGE-HISTORY-003
  - REQ-COSTS-USAGE-HISTORY-005
  - REQ-COSTS-USAGE-HISTORY-006
  - REQ-COSTS-USAGE-HISTORY-007
acceptance_criteria:
  - AC-COSTS-USAGE-HISTORY-001.1
  - AC-COSTS-USAGE-HISTORY-001.2
  - AC-COSTS-USAGE-HISTORY-001.3
  - AC-COSTS-USAGE-HISTORY-002.3
  - AC-COSTS-USAGE-HISTORY-002.5
  - AC-COSTS-USAGE-HISTORY-003.2
  - AC-COSTS-USAGE-HISTORY-003.7
  - AC-COSTS-USAGE-HISTORY-005.1
  - AC-COSTS-USAGE-HISTORY-005.2
  - AC-COSTS-USAGE-HISTORY-005.3
  - AC-COSTS-USAGE-HISTORY-005.4
  - AC-COSTS-USAGE-HISTORY-005.5
  - AC-COSTS-USAGE-HISTORY-005.6
  - AC-COSTS-USAGE-HISTORY-006.5
  - AC-COSTS-USAGE-HISTORY-006.6
  - AC-COSTS-USAGE-HISTORY-006.7
  - AC-COSTS-USAGE-HISTORY-007.4
  - AC-COSTS-USAGE-HISTORY-007.5
  - AC-COSTS-USAGE-HISTORY-007.6
system_design:
  - ../../specs/costs/system-design/usage-history.md
---

# Task 05: Serve retained statistics with bounded SQL

## Summary

Move statistics and saved canonical reads to retained facts and daily summaries.
Show preserved amounts after deletion and verify recent-range performance against years of history.

## In scope

- Apply SQL date predicates before loading facts. Use indexed daily summaries for charts and canonical detail for task/session reads.
- Push provider filtering, grouping, stable sorting, pagination, and full totals into database queries.
- Use scoped existence/freshness metadata and provider dimensions without repeated whole-history scans.
- Preserve calendar-boundary partial coverage, nullable rates, distinct currencies, and separate lifetime totals.
- Use UTC/source-calendar summaries when available. For other calendars, aggregate indexed timed canonical facts against bounded UTC day intervals.
- Add backward-compatible DTOs for deleted links, actor attribution, projection state, and large integer transport.
- Update current Token Usage tables/details, Copy Stats, localized strings, and saved-value preparation/error behavior.
- Add deterministic native/external deletion, scope isolation, rebuild, and desktop/phone browser flows.
- Add five-year fixtures, query-plan assertions, and benchmark evidence for SQLite and real PostgreSQL.
- Include nonmaterialized browser calendars, DST and fractional offsets in the same evidence, without source-observation reconciliation.
- Update relevant public statistics/retention docs. Cut reads over only after Task 02 and Task 04 watermarks prove readiness.

## Out of scope

- New org-wide dashboard, user-management roles, new pricing, and plugin process changes.

## Acceptance

- A recent chart reads date-bounded summaries, with no raw-history pairwise pass. Filters, pagination, totals, and provider options agree.
- Deleted tasks retain totals without broken links. Mixed/unknown attribution, preparing history, and stale values remain explicit on both viewports.
- Record visited rows, query plans, latency, memory, and fixture/hardware details for five years. Do not substitute skipped PostgreSQL evidence.

## ASCII UI preview

UI-H01/UI-H02/UI-H03 are defined in the [full preview](plan.md#ascii-ui-preview).

```text
Desktop tasks table:
  Deleted task          exact-model-id   120,000 tokens   $1.20 estimated
  Archived live task >  exact-model-id    40,000 tokens   $0.40 estimated

Phone tasks list:
  [Deleted task]
  120,000 tokens / $1.20 estimated
  [Details] -> model, provider, user attribution, all categories, calendar

First build: Preparing usage history.
Later rebuild: Keep saved values visible. Updating history...
```

Deleted rows have no navigation link. Phone has one body scroll owner, safe-area spacing, and 44px touch controls.
Use existing focused details and desktop control density. All labels are localized.
Covers 001.1-.3, 002.3, 003.2, 006.5-.6, and 007.4-.6.

## Verification

Run from the Kandev root. Install dependencies first in a fresh worktree.
Managed browser runs are sequential and rebuild the actual frontend/backend.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -race -trimpath ./internal/analytics/... ./internal/plugins/... -run 'TestUsageHistory|TestSessionUsage' -count=1)
(cd apps/backend && : "${KANDEV_TEST_POSTGRES_DSN:?Set an isolated PostgreSQL test DSN}" && go test -trimpath ./internal/analytics/... -run 'TestUsageHistoryPostgres' -count=1)
(cd apps/backend && CGO_ENABLED=1 CGO_CFLAGS='-O2 -g -DSQLITE_ENABLE_STMT_SCANSTATUS' GOMAXPROCS=2 GOMEMLIMIT=3GiB KANDEV_USAGE_BENCH_EVIDENCE_DIR="$PWD/../../docs/plans/usage-history/evidence" go test -p 1 -trimpath -tags 'sqlite_trace usage_history_scanstatus' ./internal/analytics/repository/sqlite -run '^$' -bench '^BenchmarkUsageHistoryFiveYears$' -benchtime=10x -benchmem -count=1 -timeout=60m)
(cd apps/backend && : "${KANDEV_TEST_POSTGRES_DSN:?Set an isolated PostgreSQL test DSN}" && CGO_ENABLED=1 CGO_CFLAGS='-O2 -g -DSQLITE_ENABLE_STMT_SCANSTATUS' GOMAXPROCS=2 GOMEMLIMIT=3GiB KANDEV_USAGE_BENCH_EVIDENCE_DIR="$PWD/../../docs/plans/usage-history/evidence" go test -p 1 -trimpath -tags 'sqlite_trace usage_history_scanstatus' ./internal/analytics/repository/sqlite -run '^$' -bench '^BenchmarkUsageHistoryPostgresFiveYears$' -benchtime=10x -benchmem -count=1 -timeout=60m)
(cd apps/web && pnpm exec vitest run app/stats && pnpm run typecheck && pnpm run i18n:check)
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium -- tests/layout/stats-usage-history.spec.ts --retries=0
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome -- tests/layout/mobile-stats-usage-history.spec.ts --retries=0
pnpm --dir apps/web e2e:run --host --shards 1 --project auth -- tests/auth/usage-history-scope.spec.ts --retries=0
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/analytics/repository/sqlite/{usage.go,usage_history_queries*.go,usage_history_bench_test.go}`
- `apps/backend/internal/analytics/{handlers/stats_handlers.go,models/usage.go,dto}`
- `apps/backend/internal/plugins/host_data.go` and saved canonical readers
- `apps/web/app/stats/token-usage-*`, `lib/api/domains/stats-api.ts`, `lib/types/http.ts`, and locale catalogs
- New `apps/web/e2e/tests/{layout,auth}/*usage-history*` files
- Relevant files under `docs/public`

## Dependencies

Task 04, including completed migration and projection watermarks from preceding tasks.

## Risks

Response pagination does not prove database work is bounded. Capture query plans and visited rows.
Provider/day summaries must not merge currencies, calendars, or unknown actors into misleading complete values.
Existing direct task/session readers must not bypass retained-scope authorization after deletion.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/costs/requirements/usage-history.md), listed ACs.
- [Design](../../specs/costs/system-design/usage-history.md), SQL reads, Ownership, Presentation, and projection-generation contract.
- [Plan](plan.md), test/E2E matrix and UI-H01 through UI-H03.
- Existing `stats_reference_bench_test.go`, Token Usage UI tests, and stats authorization fixtures.

## Results

### Current browser integration checkpoint

The current frontend passed 90 component/helper tests, lint, full typechecking, localization checks, and a production build.
The first actual desktop browser run executed five cases with retries disabled: all five failed, with zero skips or flakes.
Its logs, traces, and screenshots remain preserved. Discovery-only receipts are not browser passes.

The correction prepares real sessions, handles nullable/preparing target pages, and scopes assertions to the intended usage component.
Policy cleanup reads the current CAS revision and preserves an original assertion if restoration also fails.
All token, cost, attribution, and authorization assertions remain intact.
The corrected helpers passed 23 focused tests, ten-file lint, formatting, and a full TypeScript check.
The new tests cover target snapshots, request serialization, current-revision restoration, and preservation of operation errors.
Successful collector/native flows now capture actual retained details separately for desktop and phone.
Those captures and all 16 browser results remain pending a fresh combined build.

Earlier implementation and review receipts follow as historical evidence.

Retained SQL reporting and HTTP mapping are implemented; integrated verification remains pending.
Independent frontend CI preparation completed:
full `pnpm lint` passed, the focused `useScrollToMessage` run passed 9 cases (99 unrelated cases skipped),
and both `e2e/helpers/task-navigation-helpers.test.ts` cases passed.
The three Git-refresh browser cases were discovered, but their managed execution remains pending until backend work is stable.
The initial-loading test now observes a correlated pending notification while enrichment remains held.
The same worker resumes after Task 04 for SQL reporting, benchmarks, and full required browser evidence.

Frontend Phase B is ready for coordinator review. Worker 5 froze the frontend changes after the final checks passed.
The frontend shows retained availability, independent token and cost actors, server accounting metadata, and exact integer amounts.
Saved values survive retries and projection rebuilds within the same query scope.
Summary cards, table footers, and Copy Stats use complete filtered totals.

The timestamp gate required one missing import in `apps/web/lib/ws/handlers/agent-session.ts`.
The handler now uses the existing strict RFC3339 parser. The parser retains nanosecond ordering with `bigint`.
The existing handler suite reproduced 47 missing-reference errors before this correction.
No timestamp helper or test assertions changed.

Passing receipts from `apps/web`:

- `pnpm exec vitest run app/stats/token-usage-*.test.ts app/stats/token-usage-*.test.tsx lib/api/domains/stats-api.test.ts --reporter=dot`: 44 tests passed across seven files, without skips.
- `pnpm exec vitest run lib/ws/handlers/agent-session.test.ts lib/state/slices/session/turn-actions.test.ts --reporter=dot`: 135 tests passed across two files, without skips.
- `pnpm exec tsc --noEmit --pretty false`: passed after the import correction.
- `pnpm exec eslint --max-warnings=0 lib/ws/handlers/agent-session.ts`: passed.
- Scoped lint for the changed Phase B frontend sources and tests passed before the import correction.
- `pnpm run i18n:check`: passed for all seven languages and the pseudo locale.

The additive HTTP mappings now have server implementations.
`apps/web/lib/types/http-usage-history.ts` records the projection, availability, actor, calendar, history-gap, and decimal transport fields.
Each HTTP report reads rows, full totals, provider options, availability, and freshness within one authorized publication boundary.
Continued chart pages pin that opaque publication and its resolved range bounds.
Saved public canonical reads require completed retention watermarks and use publication-aware cursors bound to the filter.
Internal projection readers remain available for migration and maintenance.

Worker 5 froze production and harness code for the coordinator's integrated-check window.
The source-owned reader now returns bounded accepted observations without canonical reconciliation.
It preserves retained authorization after workspace transfer.
Explicit undated range reads do not invent chart dates.
Report overflow returns a typed error and HTTP 422; unlike currencies do not form an invalid monetary sum.
Unused raw-history reconciliation readers were removed. Worker 3's seven parity files retain their handoff hashes.

The guarded E2E controls pause and resume the real projector, request a real rebuild, and occupy the real SQLite writer connection.
The existing analytics deadline produces the busy response. No report payload is substituted by these controls.
Shutdown joins the worker, releases the held connection, and prevents late restarts.
The packaged fixture collector uses public SDK calls with exact decimal inputs.
Its authenticated action fixes the workspace to the verified request context.
No SDK or sibling plugin repository files were changed by Worker 5.

Current focused receipts:

- `/tmp/usage-history-worker5-freeze-targeted.log`: 19 top-level Go cases passed across repository, HTTP, Host, lifecycle, and packaged fixture boundaries; zero skips. The command uses `GOMAXPROCS=2`, `-p 1`, `-trimpath`, and an explicit owned-test expression. This run does not claim race or PostgreSQL coverage.
- `/tmp/usage-history-worker5-all-time-grid-check.log`: the focused all-time single-point chart case passed without a production change.
- `/tmp/usage-history-worker5-freeze-web-tests.log`: the two affected pagination/API files passed all 22 Vitest cases without skips after the lint refactor.
- `/tmp/usage-history-worker5-freeze-web-lint.log`: scoped ESLint passed with zero warnings.
- `/tmp/usage-history-worker5-freeze-typecheck.log`: `pnpm exec tsc --noEmit --pretty false` passed.
- `/tmp/usage-history-worker5-freeze-docs.log`: `node scripts/validate-public-docs.mjs` validated all 55 published pages.
- `/tmp/usage-history-worker5-freeze-discovery.log`: Playwright discovered 16 cases across four files: five desktop usage cases, four phone usage cases, four auth cases, and the three Phase A Git-refresh cases. Discovery does not count as browser execution.

The new real browser flows cover collector archive/deletion/correction, native delivery pruning, tool payload retention, unavailable session links, preparation/rebuild, deadline/retry, scope forgery, transfer, disabled users, suspension, and erasure.
Public explanation and retention guidance now live in `docs/public/usage-history.md`, with links from Sessions and Review and Operations.

Task 05 remains incomplete. The coordinator must release the sequential integrated-check windows.
Remaining checks include current combined races and Worker 3 parity cases, actual PostgreSQL 18 reporting/resolver cases, scoped Go lint/vet/SQLguard, affected persistence conformance, fresh managed desktop/phone/auth/Git browsers, and both five-year benchmarks.
The new PostgreSQL amount/source/undated edge cases are prepared but have not run.
Earlier PostgreSQL and broad lint receipts predate these changes and are not current signoff.
Worker 4 owns the isolated benchmark/instrumentation/evidence files and must supply the real SQLite/PostgreSQL benchmark receipts.
No benchmark or browser pass, measured latency, visited-row count, or memory claim is made here.


### Contract review freeze checkpoint

This checkpoint supersedes the earlier pending integration checklist.
Production and test writes are frozen for the coordinator's contract-fix handoff.
Task 05 remains incomplete. No benchmark or browser ran in this integration window.

The current 1075-file benchmark frame is `9aeaf6a732e628cfacbd609dbcc70a013016435928b9989bb53f5a9727a02fdb`.
Its hash record is `/tmp/usage-history-worker5-contract-freeze-source-hashes.json`.
The record also includes the reboot test, which is outside that benchmark frame.
No production Go, SQL, HTTP, SDK, or frontend code changed in this window.
Four test files changed: the contract, selection, and concurrent-writer fixtures, plus `storage_usage_rollup_boot_test.go`.
The saved-reader fixtures now use production task schemas and complete real bounded retention backfill.
The reboot fixture creates its workspace and supplies fixed UTC event timestamps.
All original revision, amount, model, deletion, and rollup assertions remain.

Current receipts:

- `/tmp/usage-history-worker5-integrated-current-race.log`: 187 top-level passes and five fixture failures; zero skips and race reports.
- `/tmp/usage-history-worker5-integrated-repairs-native-race.log`: 28 passes, including all four saved-reader repairs and additional native/organization/startup cases. The reboot case still lacked its event timestamp in that run.
- `/tmp/usage-history-worker5-integrated-reboot-race.log`: the repaired reboot case passed with race detection.
- Those three receipts provide passing coverage for all 216 unique selected integration cases. No full rerun of the already-passing cases is claimed.
- `/tmp/usage-history-worker5-integrated-persistence-race.log`: 36 top-level cases and 608 subcases passed on SQLite and PostgreSQL 18.6. Both prior-stable upgrade paths passed. No case skipped.
- `/tmp/usage-history-worker5-integrated-lint-reporting.log` and `...-lint-native-persistence.log`: all 11 selected packages passed with zero issues.
- `/tmp/usage-history-worker5-integrated-vet.log`: all 16 changed backend packages passed vet.
- `/tmp/usage-history-worker5-calendar-match-sqlguard.log`: the full internal SQLguard pass remains current. SQLguard excludes test files, and its production inputs did not change.
- Public docs validation passed for 55 pages. Document catalog validation passed for 417 decisions and 1828 specifications. Specification lint passed.

Runtime/Host lint remains unpassed for five packages.
The original combined lint timed out during package loading.
A runtime-group attempt encountered another checkout's global lint lock.
Its first serial attempt reported zero issues but exited 4 on timeout, so that attempt is not a pass.
At the coordinator's freeze request, Worker 5 stopped only its own second serial waiter.
That waiter was PID 1032196, session 81416, and exited 143.
No unrelated process was stopped. No owned command remains running.
The exact remaining lint command and all failed receipts are in `/tmp/usage-history-worker5-contract-freeze-handoff.txt`.

The coordinator identified three contract gaps for Ptolemy: transcript identity across sessions, fresh bindings after organization transfer, and unbounded target refresh.
These gaps require fixes and affected reporting/integration verification before benchmark or browser signoff.
Both full five-year benchmarks and all 16 managed browser cases remain unrun.
The browser set includes desktop, phone, auth, and the three Git-refresh cases.
Final integration review must also resolve captured-scope erasure for plugin receipt/progress metadata.
The plugin worker owns its returned findings. This checkpoint adds no core API for that question.
