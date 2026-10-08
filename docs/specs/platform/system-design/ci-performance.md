---
status: draft
system: platform
created: 2026-09-12
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-001
  - REQ-PLATFORM-CI-PERFORMANCE-002
  - REQ-PLATFORM-CI-PERFORMANCE-003
  - REQ-PLATFORM-CI-PERFORMANCE-004
---

# CI performance system design

## Purpose and boundaries

This design changes repository CI execution and test infrastructure. It does not change application behavior.
The [external-runner design](external-e2e-runner-capacity.md) remains authoritative for provider allocation and protected jobs.
The [CI automation system](../../ci/README.md) retains contributor trust ownership.
No database, public API, or runtime feature flag changes are required.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-PLATFORM-CI-PERFORMANCE-001 | Review budget |
| REQ-PLATFORM-CI-PERFORMANCE-002 | Dependency cache |
| REQ-PLATFORM-CI-PERFORMANCE-003 | Test setup and workflow partitions |
| REQ-PLATFORM-CI-PERFORMANCE-004 | Measurement and capacity assessment |

## Review budget

Set job-level `timeout-minutes: 30` on `claude-review-same-repo` and `claude-review-fork` in `.github/workflows/claude-code-review.yml`.
Apply the same budget to the `claude` job in `.github/workflows/claude.yml`.
The comment workflow also handles general Claude requests. Its budget therefore applies to every invocation, including requests other than review.
Leave the short fork-label job unchanged.

Keep permissions, prompts, action pins, checkouts, approval expressions, concurrency, and event filters unchanged.
Do not add `continue-on-error`, automatic retries, or success markers after cancellation.
GitHub owns timeout cancellation and cleanup latency. The budget is not a promise about total workflow elapsed time.
Do not use the retired Claude action `timeout_minutes` input.
An explicit turn cap remains deferred until completed-review turn counts support a useful threshold.

## Dependency cache

In `.github/workflows/frontend-tests.yml`, resolve `pnpm store path --silent` before the cache restore.
Run resolution from `apps/` with the same container, user, environment, and pnpm version as installation.
Expose the absolute path as a step output and use that output in the pinned cache action.
Do not infer the path from `HOME`, or change `HOME` to fit the cache.

Use a versioned key containing runner OS, architecture, pnpm version, and lockfile hash.
A restore prefix can omit only the lockfile hash. Preserve pnpm-major and platform compatibility.
Create the store directory before restoration if necessary. Cache-service errors remain best effort.
The installation itself must fail on lockfile or dependency errors.

Keep cache keys and paths identical in any frontend jobs introduced by partitioning.
Do not copy `node_modules` between runners or share mutable working directories.
Limit the first repair to the confirmed frontend failure. Inventory matching E2E paths in the evidence report without changing those workflows speculatively.

## Test setup and workflow partitions

### Setup boundaries

`apps/web/vitest.config.ts` currently applies `happy-dom` and `vitest.setup.ts` to every test file.
`vitest.setup.ts` calls `initI18nForTests()` and awaits `loadAllLocalesForTests()` for each isolated file.
The frontend log shows substantial setup, import, and environment costs.

Introduce explicit Vitest projects for Node-compatible helpers and browser-dependent tests.
Start with a reviewed list of pure helper files. Do not infer environment requirements from `.ts` versus `.tsx` extensions.
Keep all unclassified files in the full browser-locale project. A partition
contract must detect overlap or omitted files.
Node setup must not import React, DOM globals, or locale catalogs unless a selected test requires them.

Keep English initialization for browser tests. Move complete locale loading into an explicit multilingual setup path.
Inventory tests that call `changeLanguage`, `loadAllLocalesForTests`, or inspect catalog state, including indirect helper calls.
Their setup must finish before test execution. Default unknown cases to full setup until their dependencies are established.
Preserve the `NODE_ENV=test` pin, the React `act` guard, Monaco alias, inert WebSocket, cleanup hooks, and `passWithNoTests: false`.
Keep test isolation and local worker-budget rules unchanged.

### Workflow partitions

After setup measurements, benchmark two and four Vitest shards on the same runner class.
Use built-in Vitest file sharding. Select two shards initially only if it passes the measurement gate below.
Use four only when it provides a further measured benefit within the compute budget.

Keep the existing `frontend` job for formatting, licenses, lint, typecheck, SDK checks, i18n checks, ratchets, the unsharded unit suite, and build until the adoption gate passes.
After successful change detection and a passing measurement gate, an optional
`frontend_tests` matrix may split unit tests. Until then, the matrix remains a
candidate and must not become required CI.
The active job and any measured candidate retain the existing pinned container
and repaired cache. Each test command retains `NODE_ENV: production`.
Run tests through the package script so `pretest` generation still executes.

If adopted, register the matrix through `.github/actions/plan-external-runners`
using its existing family schema. Use the standard tier without changing
protected-job placement or percentage semantics. Extend `frontend-gate` to
require the planner, change detection, `frontend`, and all test shards only
after adoption.
Keep its public name `Frontend Tests Passed`, `if: always()`, and deliberate-skip handling.
Keep `pull_request`, `push`, `merge_group`, and dispatch coverage. Do not add trigger-level path filters.

Collect unique JSON reports per shard with test file identities and test case counts.
Compare their union against an unsharded run at the same source snapshot.
Failures and cancelled shards must still block the gate. Reports must not mask a failed test exit code.

### Windows process cohorts

The process member of `test-windows` in `.github/workflows/backend-tests.yml`
retains its name, hosted runner, 90-minute job limit and required aggregate.
The independent `native` member, its checks, race flags and failure handling stay intact.
Only the process member uses `apps/backend/cmd/windows-process-tests`, a fixed
two-cohort native Go runner. This is not a configurable shard service.

From `apps/backend`, enumerate the existing `./internal/agentctl/server/process/...`
selection using `go test -race -json -timeout 25m -list '^(Test|Fuzz|Example)'`.
This builds and invokes the native test binaries, including descendant packages.
Do not infer runnable names from source text, Linux lists or retained failure logs.
Require a successful command and complete successful package records; retain each
package/name identity. Reject malformed output, duplicate names within a package,
invalid top-level identifiers, unexpected packages and an empty global inventory.
Packages with no runnable entries remain in the original package selection.

Sort the distinct top-level names by Go string order and alternate them between
exactly two nonempty cohorts. The same name in multiple packages belongs to the
same cohort in each package. Build escaped, anchored `^(name1|name2|...)$` selectors.
Verify each native package/name identity matches exactly one selector and their
union equals the inventory. A selector contains no slash, so Go runs every nested
subtest and fuzz seed below its selected top-level name. Examples retain native
Go eligibility. Benchmarks remain outside the existing default test invocation.

Start both `go test -race -v -json -timeout 25m -run <selector>
./internal/agentctl/server/process/...` commands before waiting for either.
Use argv arrays, not shell interpolation. Keep separate raw JSON and stderr files
under an owned temporary directory, and record each original PID, UTC start/end
and true exit. Join every started command even when another start or wait fails;
emit both complete diagnostic streams before returning a nonzero aggregate exit.
Validate each expected package/name has its actual terminal pass or skip record,
no unselected top-level test ran, and package completion is successful. Failure,
missing or duplicate completion, malformed JSON or unreadable diagnostics fails closed.
Do not cancel a sibling merely because one cohort fails or translate a timeout
into success. GitHub retains the existing outer job cancellation boundary.

Run the runner's focused Go tests in the process member before invoking it.
Extend the already registered `backend-tests-workflow-contract_test.py` to protect
the fixed selection, two cohorts, unchanged budgets/race checks, independent native
member, required aggregate and change detection. No new action, permissions,
runner provider, workflow trigger, dependency or retry policy is required.

The monitor fixture in `workspace_monitor_dirty_paths_test.go` waits through
`GetGitStatusWithDetails(tracker.cancelCtxOrBackground(), false)` instead of an
independent ten-second caller deadline. The existing [workspace status lifecycle](workspace-git-status.md)
owns the accepted job, its 60-second deadline, bounded successor slot, completion
error and shutdown cancellation. The cached read does not request a new observation.
Retain every exact path, diff, repository, message, cache and no-op assertion;
worker timeout, unavailable detail or supersession must still fail the fixture.
Keep production deadlines, cancellation tests and the Go package deadlock alarm.

The mode-transition fixture in `workspace_poll_mode_loop_test.go` separates the
two-second immediate-scan admission check from scan completion. Capture completed
`MonitorTickStats` after paused initialization and drain stale `tickDone`. Within
two seconds of the real fast-mode transition, require either `monitorRunning` or
an increased completed-scan count, so a short completed tick cannot be missed.
Keep both fast intervals at 30 seconds: their regular timer cannot satisfy admission.
Then receive the causally admitted scan's `tickDone`, retaining tracker cancellation
and the real file-change notification assertion with its existing two-second bound.
The fixture completion guard uses the existing 60-second status-observation
budget plus three existing ten-second Git command budgets (quick state twice and
file listing once). This 90-second guard is a fixture failure bound, not a production
whole-tick guarantee: admission queue wait and subprocess cleanup are separate.
ROOT accepted this qualification for this owned test guard. Timeout or cancellation
remains failure; do not infer a tracker deadline or enlarge package/job timeouts.

The [first-commit Unstage package](../../../plans/unstage-all-before-first-commit/plan.md)
owns this bounded delivery dependency. Its two failed Windows attempts provide
partial scheduling evidence, not a successful full-suite performance comparison.
Unobserved test times remain unknown. Native listing and successful full cohorts
are required delivery evidence; no comparative speedup or transient-cause claim
follows from those failed profiles. Existing frontend measurement gates remain intact.

## Measurement and capacity assessment

Save curated evidence in the implementation package. Keep raw logs outside version control.
Record run ID, attempt, head SHA, event, workflow version, job ID, runner label, container digest, and tool versions.
Record job `created_at`, `started_at`, and `completed_at` separately.
Execution equals completion minus start. Queue delay equals start minus job creation.
Exclude skipped jobs with invalid timestamps from duration calculations.
Do not label workflow `run_started_at` as job execution.
Approval and dependency waits can precede job creation. Report them separately when evidence exists, otherwise leave the cause unknown.
Use attempt-specific job data for reruns. Do not add concurrent job durations to obtain workflow wall time.

Use at least three comparable successful runs per candidate and baseline for performance decisions.
Compare medians and individual samples. Do not present three samples as a reliable p90 estimate.
Record failures and retries from all attempted samples, not only successful ones.
For setup optimization, require lower median unit-test execution without reduced selection or new failures.
For sharding, target at least 30% lower median frontend execution critical path and no more than 25% additional frontend runner minutes.
Measure queue time separately. Reject a partition that consistently worsens total feedback under representative load.
These are candidate-retention gates, not promised speedups. Record rejected candidates and keep the simpler passing configuration.

Inspect existing E2E timing and retry artifacts before selecting fixture changes.
Profile the Windows job by compile, package-test, and cache intervals. Preserve race checks and operating-system coverage.
The profiling work order produces a ranked follow-up report with exact targets. It does not authorize unidentified source changes.

Prepare a capacity pilot using the existing repository variables and approved tiers.
Record current variables again before proposing activation. The investigation snapshot had burst mode disabled and percentage set to 20.
Describe a 20% pilot, cost assumptions, sample count, rollback, and operator commands in the existing merge-queue runbook.
Do not activate paid capacity as part of documentation or repository implementation.
Review and Cargo Audit remain hosted; this pilot does not directly move their jobs.

## Failure and recovery

A cold cache installs dependencies normally. A failed test partition fails the existing required gate.
A timeout cancels Claude execution without a success fallback. Approval expiration remains an approval outcome.
Revert a performance candidate that loses coverage or exceeds its measured budget.
Runner rollback follows the existing external-runner design and affects new jobs only.

## Related decisions and plans

- [External-runner decision](../../../decisions/2026-09-06-opt-in-external-e2e-runners.md)
- [Existing runner package](../../../plans/external-e2e-runner-capacity/plan.md)
- [Existing E2E efficiency package](../../../plans/e2e-ci-efficiency/plan.md)
- [CI performance package](../../../plans/ci-performance/plan.md)

The existing runner package is complete. It needs no repeated implementation.
The E2E efficiency package retains open rollout evidence. Link new measurements there without marking unperformed rollout work complete.
No new architectural decision is required: this package preserves existing trust and isolation boundaries.
