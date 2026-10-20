---
id: "01-count-committed-completions"
title: "Count committed task completions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-COMPLETION-STATS-001
acceptance_criteria:
  - AC-TASKS-COMPLETION-STATS-001.1
  - AC-TASKS-COMPLETION-STATS-001.2
  - AC-TASKS-COMPLETION-STATS-001.3
  - AC-TASKS-COMPLETION-STATS-001.4
  - AC-TASKS-COMPLETION-STATS-001.5
  - AC-TASKS-COMPLETION-STATS-001.6
  - AC-TASKS-COMPLETION-STATS-001.7
  - AC-TASKS-COMPLETION-STATS-001.8
system_design:
  - ../../specs/tasks/system-design/completion-statistics.md
---

# Task 01: Count Committed Task Completions

## Summary

Make all three analytics completion consumers use committed task state for
unarchived tasks, retaining archived compatibility. Supply permanent real-writer
regression evidence inside the owning analytics repository package.

## In scope

- The three predicates and unused joins/projection in `stats.go`.
- New small real-schema fixtures and the five focused tests named in the plan.
- Correcting affected legacy fixtures without weakening numeric assertions.
- Updating these four artifacts' statuses/results after implementation passes.

## Out of scope

Task/service production or test changes, lifecycle/session/provider/metadata
writes, migrations, new APIs/metrics/registries/caches, all-time horizon changes,
frontend changes, broad backend/task suite or full backend lint, and unrelated
resources. Do not read/copy/replay ROOT's private probe or failed fixture sources.

## Acceptance

1. All three actual queries pass the state/workflow regression matrix and
   positive controls; activity has independent causal RED/GREEN evidence.
2. The compatibility and timestamp oracle preserves every boundary mapped in
   the plan, including non-completion aggregates and read-only behavior.
3. All focused cases/dialect fixtures pass before full owning-package race
   validation; changed-Go lint and document coverage pass with actual results.

## Execution gate and resources

ROOT separately released implementation to this same primary and granted the
sole global local-heavy lease through install/tests/hooks. `/tdd` and backend
test guidance govern execution. Keep the
same primary/profile; no delegates, recursive tasks, sessions, or model switch.
Recheck current branch/main dependencies without replaying discovery. Preserve
managed worktree, dependencies, caches, evidence and protected resources.

Use this canonical prefix (pnpm directory intentionally has no `/bin` suffix):

```bash
export PATH="/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH"
export GOTOOLCHAIN=local GOMAXPROCS=2 GOFLAGS=-p=1
unset KANDEV_TEST_POSTGRES_DSN
```

Confirm actual pnpm 9.15.9, Node 24.21.0 and Go 1.26.0 before execution. If
workspace dependencies are absent, run `(cd apps && pnpm install --frozen-lockfile)`
once under that lease before package commands or normal hooks. Do not bypass
hooks. No live PostgreSQL without separate authorization for a disposable DB.

## Verification

Run from repo root with the environment above. Tests use a process-local
`GOMEMLIMIT=512MiB`, one build package at a time, and the normal `fts5` tag.
Initially add the focused tests and execute the first command for causal RED;
after the expected assertion failures, implement and run the focused command
for GREEN. Include every new case, positive control and dialect fixture, plus
only actually changed legacy tests, before the one complete owning-package race
run. The legacy test below is the known affected fixture; adjust that part of
the focused selector to the actual changed legacy tests, without adding a broad
existing-regression selector.

GNU timeout bounds focused tests at six minutes with Go's internal five-minute
timeout, and the race package at eleven minutes with Go's internal ten-minute
timeout. Both allow ten seconds before forced termination. An unexpected
failure or timeout requires the original command's actual join, current
known-owned process absence and bounded evidence to ROOT; do not automatically
replay the command or increase resources.

```bash
(cd apps/backend && env GOMEMLIMIT=512MiB timeout --kill-after=10s 6m go test -trimpath -tags=fts5 -p=1 -timeout=5m -count=1 ./internal/analytics/repository/sqlite -run '^(TestCompletionStatistics.*|TestGetCompletedTaskActivity_ExcludesEphemeralTasks)$' -v)
(cd apps/backend && env GOMEMLIMIT=512MiB timeout --kill-after=10s 11m go test -trimpath -tags=fts5 -p=1 -race -timeout=10m -count=1 ./internal/analytics/repository/sqlite)
env GOMEMLIMIT=3GiB timeout --kill-after=10s 6m bash scripts/lint-go-changed apps/backend/internal/analytics/repository/sqlite/stats.go apps/backend/internal/analytics/repository/sqlite/completion_statistics_test.go apps/backend/internal/analytics/repository/sqlite/completion_statistics_fixture_test.go apps/backend/internal/analytics/repository/sqlite/repository_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

Adjust lint's explicit path list to actual changed Go files if a test helper is
split. Its separate process-local 3 GiB memory cap is the proven bounded lint
setting; keep it within the sole heavy lease. Do not run a full task/backend
suite or full backend lint. Run the documentation preflight below after
implementation; it uses
actual tracked/untracked changed-file records and the four document contents:

```bash
node <<'JS'
const fs = require('node:fs');
const cp = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/tasks/requirements/completion-statistics.md',
  'docs/specs/tasks/system-design/completion-statistics.md',
  'docs/plans/completion-statistics/plan.md',
  'docs/plans/completion-statistics/task-01-count-committed-completions.md',
];
const tracked = cp.execFileSync('git', ['diff', '--name-only', '-z', 'HEAD']);
const untracked = cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z']);
const changed = [...new Set(Buffer.concat([tracked, untracked]).toString().split('\0').filter(Boolean))];
const result = validateCoverage({
  changedFiles: changed.map(filename => ({ filename, status: 'modified' })),
  fileContents: Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
const expected = process.env.KANDEV_DOCS_COVERAGE_EXPECTED || 'covered';
if (!result.ok || result.status !== expected || result.errors.length) process.exitCode = 1;
JS
```

During design, run this wrapper with `KANDEV_DOCS_COVERAGE_EXPECTED=exempt` and
the explicit Node binary from the canonical prefix. Require the actual
four-document change set, `exempt`, no errors, and exit 0. The initial wrong
wrapper exit 1 and prospective production-trigger `covered` exit 0 are separate
historical evidence, not substitutes for that actual-change-set check. Future
actual code changes require the default `covered` result and no errors.
Check local Markdown paths and all REQ/AC links, plan/design inclusion and
work-order requirement-to-design declarations.

Every `scripts/run-quiet` invocation, if used, gets a fresh task-owned
`KANDEV_RUN_QUIET_DIR` via `mktemp -d`; never reuse another invocation's directory.
Record bounded resources, real command exits, native session/join handles, and
current known-owned process absence before returning the heavy lease. Preserve
handles across ROOT interrupts and join every running command. No duplicate
runs to infer completion, historical PID audit, cache wipe, or foreign cleanup.

## Files likely touched

- `apps/backend/internal/analytics/repository/sqlite/stats.go`
- `apps/backend/internal/analytics/repository/sqlite/completion_statistics_test.go`
- `apps/backend/internal/analytics/repository/sqlite/completion_statistics_fixture_test.go`
- `apps/backend/internal/analytics/repository/sqlite/repository_test.go`
- `apps/backend/internal/analytics/repository/sqlite/stats_aggregate_parity_test.go`
  only if an affected old assumption requires correction.
- The four artifacts in this package.

## Dependencies

None. ROOT's qualified receipt is existing lifecycle evidence, not a source to
copy. No lease was acquired during initial design. ROOT granted the heavy lease for
implementation; serial authority remains separate.

## Risks

Constructor order/bootstrap, legacy fixture assumptions, timestamp precedence,
and unavailable live PostgreSQL evidence are detailed in the plan. No migration
or frontend package checks are required for this correction.

## Parallelism

`sequential`

## Inputs

- [Completion statistics requirements](../../specs/tasks/requirements/completion-statistics.md)
- [Completion statistics design](../../specs/tasks/system-design/completion-statistics.md)
- [Existing task completion authority](../../specs/tasks/requirements/task-completion.md)
- `newRealSchemaTestDB`, `stats_aggregate_parity_test.go`, and
  `internal/backendapp/storage.go` for fixture/constructor patterns.

## Results

Completed on 2026-10-18. The exact focused command above established semantic
RED (exit 1, 2.845 s), then GREEN (exit 0, 2.902 s) after the minimal production
correction and a diagnosed fixture baseline correction. All three actual
queries, controls and dialect cases executed before the one complete race
package (exit 0, 7.143 s). Scoped lint on all four changed Go files passed with
zero issues. Original command handles and the initial fixture failure are
recorded in the [manifest verification results](plan.md#verification-results).

Catalog, all-spec, reference/whitespace and actual changed-code documentation
coverage pass. No linter unit-test replay, frontend/package checks or live
PostgreSQL was added. Spec statuses are active/current and the plan implemented.
Normal hooks/commit/PR receipts remain delivery evidence. Ordinary delivery is
released by ROOT; no second publication checkpoint is required. Hosted observer,
READY, retry, and separate expected-head squash gates still apply. No merge or
CI retry without ROOT's later grant.
