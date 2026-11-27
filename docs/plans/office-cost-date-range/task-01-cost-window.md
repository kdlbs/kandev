---
id: "01-cost-window"
title: "Wire and verify the selected cost window"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-COSTS-001
acceptance_criteria:
  - AC-OFFICE-COSTS-001.2
  - AC-OFFICE-COSTS-001.8
system_design:
  - ../../specs/office/system-design/costs-01.md
---

# Task 01: Wire and Verify the Selected Cost Window

## Summary

Repair the actual MTD/Last 30 days action from component to SQL. Keep omitted
range callers compatible and apply one event-time window to every returned
aggregate. Implementation was explicitly authorized after ROOT reviewed the complete
design package and corrections. The exclusive command lease is held by this primary.

## In scope

- Validate the optional composed-endpoint query and resolve one authoritative
  UTC request instant. Follow the owning design's half-open boundaries.
- Add range-aware service and narrow windowed SQL reads, sharing existing query
  bodies while preserving lifetime/budget methods and grouping semantics.
- Pass selected range from the existing shared component through real API
  options, including live refresh. Independently author meaningful regressions.

## Out of scope

The plan's exclusions apply. No migrations, pricing or budget refactor, raw
history browser filtering, layout/copy/touch/navigation changes, new datepicker,
settings preservation, delegates, new sessions or foreign-resource cleanup.
ROOT private fixture/proof files are excluded from all reads and changes.

## Acceptance

1. Real SQLite + CostService + Gin and real component/API/fetch regressions fail
   behaviorally before the fix, then pass with distinct lifetime/MTD/30d results,
   HTTP 400 validation and correct empty responses. All five query paths share
   exactly the same interval and all four breakdowns include only eligible events.
2. Fixed boundary and real SQL tests prove UTC calendar MTD, elapsed 720h,
   inclusive start/exclusive before, event-time eligibility, preserved workspace
   and taskless-run scope, live project/deleted-profile labels, zero-cost rows,
   group membership and per-group counts. Add isolated env-gated PG query parity.
3. Actual staged inventory passes the exact affected checks below; close the
   order and manifest with actual results before first publication. Existing
   active requirement and migration-draft design lifecycle are preserved; this
   repair does not claim completion of the wider cost/budget migration.

## Files likely touched and ownership

Existing production:

- `apps/backend/internal/office/costs/handler.go`
- `apps/backend/internal/office/costs/service.go`
- `apps/backend/internal/office/repository/sqlite/costs.go`
- `apps/web/lib/api/domains/office-api.ts`
- `apps/web/app/office/workspace/costs/cost-overview.tsx`

New bounded production helper:

- `apps/backend/internal/office/costs/range.go` (validation/window resolver and
  range-aware service helper as needed to keep function/file lint limits).

Permanent tests, existing or new:

- `apps/backend/internal/office/costs/handler_range_test.go` (new)
- `apps/backend/internal/office/costs/range_test.go` (new)
- `apps/backend/internal/office/repository/sqlite/costs_window_test.go` (new)
- `apps/backend/internal/office/repository/sqlite/costs_window_postgres_test.go` (new)
- `apps/web/lib/api/domains/office-api.test.ts` (existing)
- `apps/web/app/office/workspace/costs/cost-overview-range.test.tsx` (new)
- `apps/web/app/office/workspace/costs/cost-overview.test.tsx` only if changed
  call expectations require it; retain its existing coverage.

Design/delivery records:

- `docs/specs/office/requirements/costs.md`
- `docs/specs/office/system-design/costs-01.md`
- `docs/plans/office-cost-date-range/plan.md`
- `docs/plans/office-cost-date-range/task-01-cost-window.md`

The anticipated production inventory is separate from the actual design-only
inventory. Reconcile this list and verification paths with actual changes before
claiming coverage. Helpers must not become a broad query/time abstraction.
Do not revert others' edits or mutate their worktrees, caches, refs or processes.

## TDD sequence

1. After the later explicit implementation request and lease, mark this order
   `in_progress`. Use `/tdd` and scoped guidance. Install frozen dependencies
   once from `apps/` before the first pnpm test/lint/commit in this fresh worktree.
2. First author `TestCostsBreakdownRangeHTTP` using the real existing handler,
   SQLite repository and CostService, without new production symbols. Seed a
   fixed mid-month scenario far from boundaries under a controlled clock where
   feasible; otherwise use widely separated events and reserve precise edges
   for deterministic window/query tests. Include distinct old, previous-month
   within-30d and current-month events with independent costs and group keys.
   Assert actual HTTP body totals, group sums/counts/labels/membership for MTD,
   30d and omitted range. The original endpoint must fail with lifetime results.
3. Author the separate component regression importing the real API wrapper,
   intercepting only fetch. Return distinct payloads based on actual URL query
   and assert rendered totals, group-count cards and tables after actual button
   clicks. An absent range must return a lifetime payload and fail the expected
   MTD result. Cover live refresh after selecting 30d. Keep the existing mocked
   component suite as independent guard coverage.
4. Record both causal RED exits. Implement only the narrow contract extension.
   Add fixed window resolver tests (month/year rollover, leap February, non-UTC
   instants/DST) and a five-call bound-capture service test. Add real SQL boundary
   and attribution cases, then the isolated PostgreSQL query twin. Never call a
   compilation error, no-test collection or DSN skip a behavioral RED or PG pass.
5. Run targeted GREEN and the exact verification once. Repeat only affected
   checks after new changes, actual failures or substantive findings. Close
   statuses/results before first commit/push/PR; use normal active hooks.

## Regression matrix

- Explicit `mtd`, `30d`, omitted range; invalid, empty, repeated and duplicate
  values; storage failure retains existing HTTP 500 envelope.
- All four breakdown arrays contain only in-window groups, their sums equal the
  total, and their event counts sum to the number of eligible rows. Metric-card
  group counts agree. Empty selected window is zero with four `[]` arrays.
- Exact start included, one tick before excluded; exact before/future excluded.
  Seed occurred_at old/created_at new and occurred_at new/created_at old.
- In-workspace task and taskless run-session events, another workspace's events,
  project reassignment using the current task row, deleted agent-profile empty
  label, provider/model identities and unknown/unassigned groups, zero-cost row.
- Omitted API call retains exact unfiltered URL and transport signal/options;
  selected range is encoded and removed from transport options.
- Independent query parity on real PG with task then Office initialization;
  fixtures use typed timestamps and portable INSERT, never SQLite-only DDL in PG.

## Mobile and E2E scope

Use the plan's explicit mobile-parity data-only exemption. Shared component
button actions test the same range logic for desktop and phone. No geometry,
touch, navigation or copy change means no new preview/capture/E2E build gate.
The actual Costs E2E matcher reads pathname via `waitForHttp`; query addition
needs no change. Run the existing focused helper query-string test below. If
implementation unexpectedly changes layout or matcher behavior, reconcile the
scope before adding the exact affected rendered check.

## Verification

All commands below start at repository root. Run sequentially under the granted
heavy lease. Each local heavy command has a finite GNU timeout with a 10-second
TERM-to-KILL grace. Preserve every actual command/session handle and poll it to
completion, including after interruption; never infer a join from a duplicate
run. A timeout is a failure, not a pass. The affected Go package deadline is
280 seconds; increase it only for an evidenced package-specific need, recording
the calibration rather than replaying a full suite.
The installed binaries were verified during design. System `/usr/bin/go` is
not the toolchain for this task. Corepack resolves pnpm 9.15.9 from `apps/`;
calling it from repository root resolves a different global default.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export GOTOOLCHAIN=local
export GOMAXPROCS=2
export GOMEMLIMIT=512MiB
export COST_RANGE_BASE_SHA=2ab7324f62dbfff9b01ab50bbf325735bc3e11dc
go version
node --version
(cd apps && corepack pnpm --version)
# Exactly once after implementation authorization, before package commands.
(cd apps && timeout --signal=TERM --kill-after=10s 280s corepack pnpm install --frozen-lockfile)

# Causal RED first; repeat these same affected files/packages for GREEN.
(cd apps/backend && timeout --signal=TERM --kill-after=10s 280s go test -trimpath -tags fts5 -p=1 ./internal/office/costs -run '^TestCostsBreakdownRangeHTTP$' -count=1)
(cd apps/web && timeout --signal=TERM --kill-after=10s 180s corepack pnpm exec vitest run app/office/workspace/costs/cost-overview-range.test.tsx)

# GREEN: fts5 is required by real SQLite schema initialization.
# Affected Go packages also retain lifetime/budget compatibility coverage.
(cd apps/backend && timeout --signal=TERM --kill-after=10s 280s go test -trimpath -race -tags fts5 -p=1 ./internal/office/costs ./internal/office/repository/sqlite -count=1)
# Explicit PG evidence: inherits KANDEV_TEST_POSTGRES_DSN if available.
(cd apps/backend && timeout --signal=TERM --kill-after=10s 280s go test -trimpath -tags fts5 -p=1 ./internal/office/repository/sqlite -run '^TestPostgresCostsWindowQueries$' -count=1 -v)
(cd apps/web && timeout --signal=TERM --kill-after=10s 180s corepack pnpm exec vitest run app/office/workspace/costs/cost-overview-range.test.tsx app/office/workspace/costs/cost-overview.test.tsx lib/api/domains/office-api.test.ts)
(cd apps/web && timeout --signal=TERM --kill-after=10s 90s corepack pnpm exec vitest run e2e/helpers/causal-waits.test.ts -t 'resolves on a matching method and path, ignoring the query string')
(cd apps/web && timeout --signal=TERM --kill-after=10s 280s corepack pnpm run typecheck)
```

Before either Go or frontend changed-line lint, reconcile actual new/changed
paths and stage ALL actual owned files, including new helpers/tests and the four
design records. Use `/commit`; keep normal hooks and no bypass. Do not run
scoped golangci before staging: untracked `range.go` and new tests otherwise
escape its changed-line coverage. Inspect the actual index, including additions,
then run the single scoped Go lint below and indexed TS/TSX lint. Adjust this
anticipated add list to the actual owned inventory, omitting nonexistent paths
and including every actual new helper/test; never stage foreign changes:

```bash
git add -- apps/backend/internal/office/costs/handler.go apps/backend/internal/office/costs/service.go apps/backend/internal/office/costs/range.go apps/backend/internal/office/repository/sqlite/costs.go apps/web/lib/api/domains/office-api.ts apps/web/app/office/workspace/costs/cost-overview.tsx apps/backend/internal/office/costs/handler_range_test.go apps/backend/internal/office/costs/range_test.go apps/backend/internal/office/repository/sqlite/costs_window_test.go apps/backend/internal/office/repository/sqlite/costs_window_postgres_test.go apps/web/lib/api/domains/office-api.test.ts apps/web/app/office/workspace/costs/cost-overview-range.test.tsx apps/web/app/office/workspace/costs/cost-overview.test.tsx docs/specs/office/requirements/costs.md docs/specs/office/system-design/costs-01.md docs/plans/office-cost-date-range/plan.md docs/plans/office-cost-date-range/task-01-cost-window.md
git diff --cached --name-status
git diff --cached --name-only -z -- 'apps/backend/*.go' | xargs -0 -r gofmt -l
(cd apps/backend && timeout --signal=TERM --kill-after=10s 280s golangci-lint run ./internal/office/costs/... ./internal/office/repository/sqlite/... --new-from-rev="$COST_RANGE_BASE_SHA" --timeout=250s)
git diff --cached --name-only -z -- 'apps/web/*.ts' 'apps/web/*.tsx' | python3 -c 'import os, subprocess, sys; paths=[os.path.relpath(p.decode(), "apps/web") for p in sys.stdin.buffer.read().split(b"\0") if p]; sys.exit(subprocess.call(["timeout", "--signal=TERM", "--kill-after=10s", "180s", "corepack", "pnpm", "exec", "eslint", *paths], cwd="apps/web") if paths else 0)'
(cd apps/web && timeout --signal=TERM --kill-after=10s 180s corepack pnpm run i18n:ratchet --base "$COST_RANGE_BASE_SHA")
(cd apps/web && timeout --signal=TERM --kill-after=10s 180s corepack pnpm run i18n:check)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git diff --cached --check
```

The i18n ratchet reads the working-tree diff against base; staging all actual
new source first ensures additions are represented. Confirm it reports the
actual affected source paths. No new copy is planned; do not externalize or
polish unrelated existing copy. If gofmt/prettier hooks change files, re-stage,
rerun only affected checks and make a new normal commit. Do not amend or bypass.

Run the repository `validateCoverage` preflight against actual staged paths and
contents using `.github/scripts/pr-docs.cjs`. Supply the four linked docs and any
other actual referenced document. Inspect `ok`, `status`, `triggeringPaths`,
`workOrders`, `acceptedReferences`, and `errors`; document-only exemption alone
does not prove anticipated production traceability. The design checkpoint runs
this separately against its actual four docs and its anticipated production
inventory. Do not create a permanent validation helper just for this call.

On a later backend fixup, refresh `COST_RANGE_BASE_SHA` to the actual PR base
SHA, stage all actual owned additions before changed-line lint, and run the
required normal full command before pushing:

```bash
(cd apps/backend && timeout --signal=TERM --kill-after=10s 320s golangci-lint run ./... --new-from-rev="$COST_RANGE_BASE_SHA" --timeout=5m)
```

The 320-second outer limit gives the required five-minute linter deadline
20 seconds to report and exit; its 10-second kill grace remains finite. Do not
replay broader tests without a new change/failure that warrants them. Normal
commit/push commands (including active hooks) also need an explicit finite GNU
timeout/kill-after=10s and an actual joined handle; budget for their required
hooks and record the chosen deadline without disabling them.

## Delivery constraints

Before first publication, update order status to `done`, manifest status to
`implemented`, checkbox and actual verification results. Preserve the wider
specs' existing lifecycle; verify this repair matches their clarified contract.
Normal authorized commit/push/ready PR follows the local skills/template.
Return GLOBAL heavy lease with concise command-join/owned-cleanup evidence,
then start one original hosted wait directly in the same turn:

```bash
timeout --signal=TERM --kill-after=10s 91m scripts/pr-await "$COST_RANGE_PR_NUMBER" --deadline-min 90 --interval-sec 60
```

Set the PR number from the actual created/reused PR. Retain that original
process handle, start time and deadline across interrupts; poll handles with
bounded tool waits and communicate at least every minute. No ROOT phase-handoff
ceremony or restarted CI clock. Read completed automatic substantive FULL
issue-comment bodies early; ACK/empty bodies do not qualify. Freeze published
head except actual code/data/security/required-CI fixes. Dispose optional
style/docs/test-portability findings without pushing; never request fresh FULL
bot review per SHA. Use adequate existing substantive reviews plus bounded
later diff; focused rereview requires concrete material correctness/security.

Readiness requires actual six required checks and actual Backend/Frontend/E2E
parents successful/complete/error-free, zero actionable visible or hidden
findings, no changes-requested or human gate, and a clean candidate. READY is
not merge authority. ROOT performs one static compatibility check, then a
separate serial ordinary exact-head squash grant is mandatory. No admin,
bypass, drift-only rebase or synthetic merged tests. ROOT owns final merge/main,
owned-blob/clean-FF/archive-once/board-absence and source-release verification.
Preserve managed worktree/dependencies/cache/refs, the protected oversized task
and anonymous volume, and every foreign process/resource.

## Dependencies

None. Reviewed design package, explicit implementation request and command
lease are prerequisites, not permission to delegate or create another session.

## Risks

See the manifest. Typed options must retain cancellation/transport behavior;
concurrent query bounds must be immutable. PG availability can limit local
execution evidence; report the exact skip and do not create or alter foreign
services to obtain it.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/office/requirements/costs.md), REQ-OFFICE-COSTS-001,
  AC-OFFICE-COSTS-001.2/.8.
- [Design](../../specs/office/system-design/costs-01.md#costs-explorer-request-windows).
- Existing `costs/service_test.go` real SQLite setup, repository `costs_test.go`,
  `cost_event_contract_postgres_test.go` isolated PG fixture, actual
  `cost-overview.test.tsx` store/refetch pattern, `office-api.test.ts` fetch test,
  and `causal-waits.test.ts` query matcher case.
- Scoped backend/web/Office AGENTS, `/tdd`, `/mobile-parity`, `/docs-maintainer`.

## Results

Implemented in the same primary session after the explicit reviewed-package
request. All implementation checks below were completed before publication.

| Check | Actual result |
| --- | --- |
| Frozen workspace install, once | Exit 0, pnpm 9.15.9; handle 41061 joined |
| Real SQLite/CostService/Gin HTTP RED | Exit 1, handle 58198 joined: MTD 60000 instead of 10000; 30d 60000 instead of 40000; all four dimensions/counts lifetime; invalid ranges HTTP 200 instead of 400 |
| Real component/API/fetch RED | Exit 1, handle 49049 joined: lifetime metric/group membership displayed for MTD |
| HTTP and component first GREEN | Exit 0, original handles 16073 and 30951 joined |
| Final affected Go race/trimpath/fts5/p1 | Exit 0, both costs and repository/sqlite packages; handle 48954 joined |
| Initial PostgreSQL query twin | Command exit 0, handle 98210 joined; test SKIPPED because KANDEV_TEST_POSTGRES_DSN was absent. Superseded by the executed PostgreSQL correction below |
| Frontend affected suites | Exit 0, 3 files / 10 tests; handle 54422 joined |
| Component after required lint-label corrections | Exit 0, 1 test; final handle 12340 joined |
| Exact existing query-string matcher check | Exit 0, 1 pass / 28 filtered skips; handle 67237 joined |
| Typecheck | Original handle 45889 joined, exit 0; no duplicate run |
| Scoped Go changed-line lint | Original handle 55807 joined, exit 0 / 0 issues after all 16 files were staged |
| Frontend lint, actual four indexed TS/TSX files | Required zero-warning command exit 0, final handle 36500 joined. Earlier warning exits were corrected before publication |
| i18n changed-line ratchet | Exit 0, handle 43755 joined; 58 added + 58 modified files clean under the supplied base; 643-entry guard list intact |
| Full i18n check | Exit 0, handle 49582 joined; catalogs/pseudo complete, 4003 non-JSX source files checked. Existing orphan-key warnings do not fail the check |
| Catalog/spec validator tests/spec lint | Exit 0, 379 decisions / 1578 specs, 36 tests, all specs passed |
| gofmt, working-tree and indexed whitespace | Clean |

Fixture-only compile/type and synctest-subtest errors before causal RED, and a
query-body edit error before GREEN, were corrected; none count as behavioral
RED. The real SQL fixture was corrected for the full profile schema before the
final Go package pass. Its boundary tick is one microsecond, supported by both
SQL engines. No production changes followed those Go/TypeScript passes; later
frontend changes only extracted labels required by the zero-warning commit hook.

Actual final inventory is 16 files: six production (including new range.go),
six test files (five new, one existing API suite), and four design records.
The existing mocked cost-overview.test.tsx was run but needed no edits.
Every new file was staged before scoped Go/frontend/i18n lint. The repository
validateCoverage preflight checks this actual index, the one order and linked
REQ-OFFICE-COSTS-001 / AC-.2/.8 / costs-01 design before publication.

AC-.2/.8 are satisfied by event-time filtering through all five queries,
fixed UTC/720h tests, real SQL scope/attribution/membership/count assertions,
real HTTP compatibility/validation/empty/error cases, and actual component/API
range actions/live refresh/transport-option tests. Existing lifetime, budget,
pricing, schema, ledger and UI composition boundaries are retained. Public-doc
audit found no published Explorer/range contract to change. The original
active requirement and wider migration-draft design lifecycle are retained.

All original command handles above are terminal and joined; no disposable
runtime, container or browser was created. Managed dependencies/caches/refs
remain intact. Normal active hooks and authorized publication follow these
completed implementation checks; their exact commit/PR receipts live in the
task plan, preserving the required first-publication freeze and hosted clock.


### Required frontend review correction

Greptile thread `4253896781` at initial head `0fd79a91f3` identified a valid
AC-.8 violation: a pending or failed MTD-to-30d request left MTD totals/groups
under the selected 30d control. CodeRabbit independently noted the same issue.
The existing display guard now includes the range that produced the accepted
response. Mismatched pending/failed selections use the existing empty view;
workspace eligibility, request-generation rejection, toast handling and
same-range live refresh retain their behavior. The owning design carries this
eligibility clarification; no new requirement, design package or UI layout/copy
was introduced.

Causal RED `37729` exited 1 and joined: the real component/API/fetch path showed
four old-window groups under pending 30d when zero were expected. The deferred
failure was settled for cleanup. GREEN `48977` exited 0 and joined, both affected
component suites / six tests, including assertions after pending and failure.
The actual two changed TS/TSX paths were staged before strict zero-warning
ESLint `86694` (exit 0/join). Typecheck `82393`, i18n ratchet `74545` (zero added,
two modified source files clean against resolved fork point `e8cf123d0`), and
full i18n `19579` all exited 0 and joined. Catalog and spec lint passed after the
minimal design edit. Indexed documentation coverage and whitespace are checked
again with the five-file corrective inventory before publication.

This correction changes no Go source; the passing Go/PG/whole-web commands were
not replayed. Claude's two future-facing SQL suggestions were acknowledged and
resolved without backend changes. Its completed authenticated FULL review and
CodeRabbit's completed FULL review covered the initial head; the bounded later
diff is the selected-range display guard and its real regression. No manual
FULL review was requested. Normal active hooks and one corrective push follow
these results; retain observer `87974`, original launch `01:38:32.496Z` and
90-minute deadline `03:08:32Z` across the head change.

### Required PostgreSQL CI correction

The real PostgreSQL CI window test failed because the fixture's profiles had
no parent `claude-acp` agent. Owned PostgreSQL 16 reproduction `0470d9` exited 1
with the same foreign-key violation. Adding the parent row exposed the existing
recording path's boolean-to-integer binding (`f82c7d`, exit 1; SQLite control
passed). The query fixture now inserts portable SQL event rows directly, so this
correction does not change recording, schema or ledger behavior. A temporary
missing-import compile failure (`97c096`, exit 1) was corrected and is not causal
RED evidence.

With valid rows, PostgreSQL RED `ca5699` exited 1: the provider label selected
raw `e.provider` outside the canonical grouped expression (SQLSTATE 42803).
Reusing that grouped expression in the fallback preserves provider labels and
group memberships. GREEN `5e734f` exited 0: real PostgreSQL and SQLite window
tests both executed and passed with race/trimpath/fts5/p1. Existing lifetime
breakdown and provider-label controls `da314f` also exited 0. Each command
returned its actual terminal exit; no DSN skip is counted as execution.

The owned database used task-labelled, memory-bounded PostgreSQL 16 on loopback
with tmpfs storage and no Docker volume. Its exact ID and task/session labels
were verified before removal (`ff2025`, exit 0), after the tests had returned.
All five affected files were staged before the required full Go lint against
actual PR base `e8cf123d03a91a51b2031e796df9e2cee26ce29e`. Original `88762`
returned exit 4 at the five-minute package-loading deadline; bounded retry
`41561` returned outer exit 124 without diagnostics. Neither is a lint pass.
After both original handles joined, the same full scope was calibrated to a
ten-minute internal deadline / 620-second outer timeout / 10-second kill grace,
with Go 1.26.0, GOTOOLCHAIN=local, GOMAXPROCS=2 and GOMEMLIMIT=512MiB unchanged.
The owned linter's measured RSS was 2363556 KiB at 5m51s. Original `42768`
returned exit 0 / zero issues; no duplicate or linter-memory switch was used.
Final indexed coverage and normal hooks precede the one corrective push;
exact delivery receipts remain in the external task checkpoint.
No frontend check is replayed for this Go/query-fixture correction. The existing
requirement/design contract and public documentation audit remain unchanged.

After the ROOT interruption, resuming original observer `87974` returned
`Unknown process id`. Exact checks for its original timeout and helper argument
strings found no process. Its terminal exit/report is unavailable; no join,
success or replacement is inferred. Its original clock and last returned
progress are retained in the external task checkpoint for ROOT reconciliation.
