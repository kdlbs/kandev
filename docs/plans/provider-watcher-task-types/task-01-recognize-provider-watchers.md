---
id: "01-recognize-provider-watchers"
title: "Recognize provider watcher task types consistently"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PROVIDER-WATCHER-TYPES-001
acceptance_criteria:
  - AC-UI-PROVIDER-WATCHER-TYPES-001.1
  - AC-UI-PROVIDER-WATCHER-TYPES-001.2
  - AC-UI-PROVIDER-WATCHER-TYPES-001.3
  - AC-UI-PROVIDER-WATCHER-TYPES-001.4
  - AC-UI-PROVIDER-WATCHER-TYPES-001.5
  - AC-UI-PROVIDER-WATCHER-TYPES-001.6
  - AC-UI-PROVIDER-WATCHER-TYPES-001.7
system_design:
  - ../../specs/ui/system-design/provider-watcher-task-types.md
---

# Task 01: Recognize Provider Watcher Task Types Consistently

## Summary

Make persisted Sidebar, complete local Sidebar, HTTP/WS/initial task mapping,
and mapped Threads filters recognize the closed first-party watcher key sets.
Prove actual consumer outcomes with independent permanent regressions.

## In scope

- Extend existing SQL projection and neutral frontend metadata helpers; route
  local Sidebar through those helpers. Keep other consumer algorithms intact.
- Add the consumer tests named in the [manifest](plan.md#tests), with all nine
  keys, usable/invalid/mixed/both values, negative filters, ordinary/manual
  links, unknown suffix lookalikes, and GitHub compatibility controls.
- Record causal RED/GREEN, focused validation, specification lifecycle, and
  actual changed-path documentation coverage.

## Out of scope

All [manifest exclusions](plan.md#out-of-scope) apply. Do not read or reuse
ROOT's private candidate tests or runner. Do not change provider writers,
metadata write policy, dialect helpers, API types, query stages, cache policy,
rendered controls, or localization. No external PostgreSQL connection.

## Acceptance

1. The actual SQL query and actual `toKanbanTask` to `queryThreadView` tests
   fail for omitted provider categories before implementation, with GitHub and
   ordinary controls passing, then pass with all planned provider/value cases.
2. Initial mapping, local Sidebar projection/page and existing mapped Sidebar
   filtering agree with the reviewed classification contract. Existing filter
   identities, review-first priority, eligibility and paging remain intact.
3. Required checks pass with actual results recorded; all owned changed paths
   are documentation `covered` with no errors. No claim exceeds executed evidence.

## Verification

Run sequentially in this same primary after ROOT's later explicit implementation
interrupt and the sole global heavy lease grant. Mark this order `in_progress`
first. No native delegates. Read `/tdd`, backend test guidance and scoped
AGENTS.md before test edits. Recheck current main/affected dependencies on resume.

Canonical shell setup from repository root:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH"
export GOTOOLCHAIN=local GOMAXPROCS=2 GOMEMLIMIT=512MiB
```

The actual pnpm executable is `/home/jcfs/.local/share/mise/installs/pnpm/9.15.9/pnpm`.
Its directory must precede Node's bin directory; do not use a nonexistent pnpm
`bin` subdirectory or let Node's package-manager shim take precedence.

If dependencies are missing, run `(cd apps && pnpm install --frozen-lockfile)`
once under that lease before frontend checks. Do not reinstall resolved deps or
modify lockfiles. Verify canonical versions before execution. If using
`scripts/run-quiet`, create a new unique task-owned `KANDEV_RUN_QUIET_DIR` for
each invocation; never use its shared default retention directory.

Write independent tests first. These two commands are the causal RED checks;
they must fail through the intended consumer assertions, not fixture/setup:

```bash
(cd apps/backend && env -u KANDEV_TEST_POSTGRES_DSN timeout --kill-after=10s 180s go test -v -race -trimpath -tags=fts5 -p=1 -count=1 -run '^TestQuerySidebarTaskPageWatcherTaskTypes$' ./internal/task/repository/sqlite)
(cd apps/web && timeout --kill-after=10s 120s pnpm exec vitest run lib/threads/thread-view-watch-types.test.ts --maxWorkers=1 --reporter=verbose)
```

The existing frontend conformance file is
`apps/web/lib/sidebar/sidebar-local-conformance.test.ts`. Backend
`sidebar_task_query_test.go::newRepoForSidebarConformance` passes PostgreSQL
through `internal/testutil/postgres.go::PostgresDSNFromEnv`, which reads exactly
`KANDEV_TEST_POSTGRES_DSN` and skips when it is absent. Every local Go test
command above and below unsets that variable before execution, preventing
external PostgreSQL access; do not supply a DSN for this work.

After the minimum production correction, rerun the two commands for GREEN,
then run these complete affected-package and scoped frontend checks once:

```bash
(cd apps/backend && env -u KANDEV_TEST_POSTGRES_DSN timeout --kill-after=10s 16m go test -race -trimpath -tags=fts5 -p=1 -count=1 -timeout=15m ./internal/task/repository/sqlite)
GOMEMLIMIT=3GiB timeout --kill-after=10s 6m bash scripts/lint-go-changed apps/backend/internal/task/repository/sqlite/sidebar_task_query_base.go apps/backend/internal/task/repository/sqlite/sidebar_task_query_watch_test.go
(cd apps/web && timeout --kill-after=10s 180s pnpm exec vitest run lib/metadata-utils.test.ts lib/kanban/map-task.test.ts lib/ssr/mapper.test.ts lib/threads/thread-view-watch-types.test.ts lib/sidebar/sidebar-watch-types.test.ts lib/sidebar/sidebar-local-conformance.test.ts --maxWorkers=1)
(cd apps/web && timeout --kill-after=10s 180s pnpm exec eslint --max-warnings 0 lib/metadata-utils.ts lib/metadata-utils.test.ts lib/sidebar/sidebar-local-projection.ts lib/kanban/map-task.test.ts lib/ssr/mapper.test.ts lib/threads/thread-view-watch-types.test.ts lib/sidebar/sidebar-watch-types.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 timeout --kill-after=10s 10m pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

The 8 GiB typecheck heap is process-local and required from the first attempt;
ROOT already qualified default 4 GiB OOM versus identical 8 GiB success. Do not
change repository configuration or repeat the insufficient default attempt.
Do not run unchanged spec-linter self-tests, full-backend lint, or broad suites.

Documentation coverage preflight, from repository root, uses only actual tracked
diff and untracked files. It is meaningful for four-doc design and later actual
implementation; do not add invented triggering paths:

```bash
node <<'JS'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [...new Set([
  ...execFileSync('git', ['diff', '--name-only', 'HEAD', '-z']).toString().split('\0'),
  ...execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z']).toString().split('\0'),
].filter(Boolean))];
const changedFiles = paths.map(filename => ({ filename, status: 'modified' }));
const fileContents = Object.fromEntries(paths.filter(p => p.endsWith('.md') && fs.existsSync(p)).map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.errors.length) process.exitCode = 1;
JS
```

At implementation completion promote the paired requirement to `active`, design
to `current`, order to `done`, and manifest to `implemented` only when tests and
design conformance are established. Record every required result, including
spec/diff/coverage and explicit PostgreSQL skips. Normal commit hooks remain
active after ROOT's reviewed release; fix hook changes without bypassing hooks.

After authorized implementation and before committing, stage the actual owned
paths, including newly added source/test files, rather than relying on an
unstaged ratchet pass. From repository root, stage the paths this order changed,
inspect the staged list, then run the required new-code copy check from the web
package. Adjust this explicit list only to reflect actual owned changes:

```bash
git add -- apps/backend/internal/task/repository/sqlite/sidebar_task_query_base.go apps/backend/internal/task/repository/sqlite/sidebar_task_query_watch_test.go apps/web/lib/metadata-utils.ts apps/web/lib/metadata-utils.test.ts apps/web/lib/sidebar/sidebar-local-projection.ts apps/web/lib/sidebar/sidebar-watch-types.test.ts apps/web/lib/kanban/map-task.test.ts apps/web/lib/ssr/mapper.test.ts apps/web/lib/threads/thread-view-watch-types.test.ts docs/specs/ui/requirements/provider-watcher-task-types.md docs/specs/ui/system-design/provider-watcher-task-types.md docs/plans/provider-watcher-task-types/plan.md docs/plans/provider-watcher-task-types/task-01-recognize-provider-watchers.md
git diff --cached --name-only
(cd apps/web && timeout --kill-after=10s 180s pnpm run i18n:ratchet)
```

Record its actual result. This explicit staged check supplements the normal
active hooks; do not bypass hooks or stage anything during the design turn.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/sidebar_task_query_base.go`
- `apps/backend/internal/task/repository/sqlite/sidebar_task_query_watch_test.go` (new)
- `apps/web/lib/metadata-utils.ts` and `metadata-utils.test.ts`
- `apps/web/lib/sidebar/sidebar-local-projection.ts`
- `apps/web/lib/sidebar/sidebar-watch-types.test.ts` (new)
- `apps/web/lib/kanban/map-task.test.ts`
- `apps/web/lib/ssr/mapper.test.ts`
- `apps/web/lib/threads/thread-view-watch-types.test.ts` (new)
- The four requirement/design/manifest/order files in this package.

Read-only consumer anchors: `lib/kanban/map-task.ts`, `lib/ssr/mapper.ts`,
`lib/threads/thread-view-query.ts`, `lib/sidebar/apply-view.ts`, and
`lib/sidebar/sidebar-local-view.ts` under `apps/web`.

## Dependencies

None. ROOT's later explicit implementation interrupt and heavy lease are
execution prerequisites, not additional work orders.

## Risks

- Keep test files below scoped size limits; do not expand the already large
  Threads query test file. No tests may mirror a standalone invented predicate.
- Review the recognized malformed-value compatibility delta in the design;
  do not change interpretation of unrelated metadata.
- Track every actual native process handle to completion. Record current owned
  groups absent and return the heavy lease before hosted observation.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/provider-watcher-task-types.md),
  AC-UI-PROVIDER-WATCHER-TYPES-001.1 through .7.
- [Design](../../specs/ui/system-design/provider-watcher-task-types.md), all sections.
- [Manifest](plan.md), qualified receipts and ownership reconciliation.
- Existing `newRepoForSidebarConformance`, Sidebar local conformance tests,
  HTTP/WS `map-task.test.ts`, initial `mapper.test.ts`, and Threads query fixtures.

## Results

Implementation checks passed after ROOT's explicit implementation release. Original
complete-package run native `2704`, actual join `1c7077`, failed with exit 1
after 600.126 seconds: the new dialect fixture used `postgres` instead of the
canonical `pgx`, and the package reached Go's implicit ten-minute timeout in
an existing task-transfer fixture during schema setup. Exact known PID/PGID
were absent at join `88dc42`; original diagnostics remain at
`/tmp/kandev-child138-go-package.ZF97NdR5/kandev-run.backend-test.XaJqiLol.log`.
The dialect fixture was corrected without changing production dialect helpers.
ROOT authorized exactly one fresh full-package race rerun, after focused GREEN,
with explicit `-timeout=15m` and outer 16-minute/kill-after-10-second bound.
The verification command above reflects that grant; all other resource and
external-DSN constraints remain unchanged. No passing suite was replayed.

At the earlier design-only checkpoint, catalog, all-spec lint, four-document
whitespace/reference checks, and actual docs-only coverage preflight passed;
see the manifest's verification results. ROOT subsequently released this order
for implementation; the completed order is now `done`.

Actual joins: catalog `f48393` exit 0; all-spec lint `c54bc7` exit 0;
four-document whitespace/reference check `d88f01` exit 0; final document-only
check `56c742` exit 0; actual four-doc coverage `e3e6ce` exit 0, `exempt`,
`errors: []`. No task-owned background processes were launched and no native
handles remain outstanding. Heavy lease unclaimed; no package install or
implementation check ran. The manifest lists all four actual coverage paths.

After ROOT's final design corrections, join `29485c` exit 0 revalidated catalog,
all-spec lint and tracked whitespace; join `be4fd5` exit 0 verified the actual
four-document set, direct whitespace, `exempt`/`errors: []`, and no staged
paths. No owned background groups or native handles remain. Implementation
was then `pending`; the staged ratchet command remains planned until execution
reaches that gate.

Execution receipts so far:

| Check | Actual result and evidence |
| --- | --- |
| Frozen install, once for missing dependencies | Native `31330`, join `0d1fdc`, exit 0; `/tmp/kandev-child138-install.d3qFCh9H/kandev-run.install.CBWMhYUo.log` |
| Actual SQLite consumer RED | Native `99543`, join `b29af4`, expected exit 1; provider omissions and malformed values fail, GitHub/ordinary controls pass; `/tmp/kandev-child138-sql-red.xZViTIl5/kandev-run.backend-test.14TkFOMZ.log` |
| Actual mapped Threads RED | Native `17866`, join `4e549e`, expected exit 1; provider omissions fail, GitHub/ordinary controls pass; `/tmp/kandev-child138-threads-red.Y3bN5Gnj/kandev-run.web-test.bpYR8cJJ.log` |
| SQLite consumer GREEN | Native `93092`, join `74fe9f`, exit 0, package 2.104s; `/tmp/kandev-child138-sql-green.KoASYa0v/kandev-run.backend-test.jbV6GnsZ.log` |
| Mapped Threads GREEN | Native `44595`, join `7e892a`, exit 0, 22 tests; `/tmp/kandev-child138-threads-green.e553NGlR/kandev-run.web-test.UrTaclsN.log` |
| Corrected SQL and dialect GREEN | Native `36857`, join `d0ce03`, exit 0, package 2.218s; `/tmp/kandev-child138-dialect-green.J8JbEHh0/kandev-run.backend-test.ZC1E17tr.log` |
| Actual implementation documentation coverage | Join `dd7dc4`, exit 0, `covered`, `errors: []`, three production triggers and one work order |
| Single authorized complete-package rerun | Native `12830`, actual join `41cc03`, exit 0, package 706.596s; PID/PGID `2497701` absent; `/tmp/kandev-child138-go-rerun-67r45per/join.json` and `kandev-run.backend-test.iRtmnXg4.log` |
| Scoped Go lint, actual two paths | Native `10383`, join `4d2c65`, exit 0, zero issues; `/tmp/kandev-child138-go-lint.tbqCfhzB/kandev-run.backend-lint.Lesewpmz.log` |
| Scoped frontend six suites | Native `28837`, join `505d24`, exit 0, 264 tests; `/tmp/kandev-child138-web-tests.S1swwShD/kandev-run.web-test.1HkDbki7.log` |
| Scoped frontend lint | Native `43549`, join `b69eb2`, exit 0; `/tmp/kandev-child138-web-lint.1GyNCxab/kandev-run.web-lint.NvNurCFX.log` |
| First full web typecheck, 8 GiB | Native `10164`, join `f61acc`, exit 2, TS2322 in the new Sidebar test's nullable executor field; `/tmp/kandev-child138-typecheck.8krtVsLg/kandev-run.web-typecheck.aUqBj4vB.log` |
| Changed Sidebar suite and lint after test-only type fix | Native `6547`, join `c21537`, exit 0, 22 tests; `/tmp/kandev-child138-sidebar-recheck.W3qEZzbK/kandev-run.web-test.urFb0daz.log`; file lint `e49521`, exit 0, `/tmp/kandev-child138-sidebar-lint.bAy6NTx1/kandev-run.web-lint.rPbuJkCG.log` |
| Final full web typecheck, 8 GiB | Native `52869`, join `861bb2`, exit 0; `/tmp/kandev-child138-typecheck-final.K3HDhe3D/kandev-run.web-typecheck.xsltfyF9.log` |
| Explicit staged i18n ratchet | Actual thirteen owned paths staged first; native `49344`, join `07df28`, exit 0; `/tmp/kandev-child138-staged-i18n.cn4SyYhc/kandev-run.i18n.SLFslUAb.log` |
| Final catalog/spec/whitespace/status checks | Join `9bf63f`, exit 0; 386 decisions/1,635 specifications validated; all-spec lint, working/staged whitespace and gofmt checks passed; exactly thirteen owned paths staged |
| Final actual changed-path coverage | Join `29e56a`, exit 0; `covered`, `errors: []`, thirteen actual changed paths, three production triggers, exactly this work order |

All local Go test commands unset the external PostgreSQL DSN. PostgreSQL
behavioral subtests were skipped; generated SQL was tested for sqlite3 and pgx.
No live PostgreSQL, provider, or browser execution is claimed. The type fix
passes actual mapped watcher flags in a bounded Sidebar item to `applyFilters`;
it does not reimplement classification or widen production types. Only that
changed suite/file lint and the failed typecheck were rerun; other passing
suite evidence remained valid. Publication still uses the normal active hooks.
