---
status: draft
system: platform
created: 2026-09-12
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-001
  - REQ-PLATFORM-CI-PERFORMANCE-002
  - REQ-PLATFORM-CI-PERFORMANCE-003
  - REQ-PLATFORM-CI-PERFORMANCE-004
  - REQ-PLATFORM-CI-PERFORMANCE-005
  - REQ-PLATFORM-CI-PERFORMANCE-006
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
| REQ-PLATFORM-CI-PERFORMANCE-005 | October capacity-constrained continuation |
| REQ-PLATFORM-CI-PERFORMANCE-006 | Conservative test-only selection |

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

The September implementation introduced three projects in `apps/web/vitest.config.ts`.
On October 8, only 23 files use Node and two use English-only browser setup.
The remaining 2,732 files use `vitest.setup.ts` and `vitest.setup.locales.ts`.
The latter calls `loadAllLocalesForTests()` for every isolated file.

Expand the existing reviewed project lists for Node-compatible helpers and browser-dependent tests.
Do not infer environment requirements from `.ts` versus `.tsx` extensions.
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
For sharding, require at least 30% lower median frontend execution critical path and no more than 10% additional frontend runner minutes.
The October capacity constraint replaces the earlier 25% allowance. Compare against the optimized unsharded baseline, not the old expensive setup.
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

## October capacity-constrained continuation

The maintainer reports approximately 60 concurrent GitHub jobs, with excess jobs queued.
The supplied dashboard shows approximately 60 active jobs and more than 200 queued jobs.
Treat 60 as an observed planning constraint, not a verified account entitlement or a repository configuration value.
No new fleet, paid tier, global admission service, or runner-held polling job is required.

### Full frontend inventory

The [October evidence](../../../plans/ci-performance/evidence-2026-10-08.md) includes every selected frontend file and a reproducible local experiment.
The inventory is triage data, not an executable classification rule.
Review imports, mocks, setup assumptions, locale changes, and indirect dependencies before changing a file's assignment.
Unresolved workspace packages, computed imports, side effects, and browser globals keep the existing full setup until proved safe.

Use the existing `REVIEWED_NODE_TEST_FILES` and `REVIEWED_BROWSER_TEST_FILES` lists.
Migrate bounded batches, starting with the 32 experimentally checked files.
Run each batch under both setups with identical source, test identities, worker count, and inherited production environment.
Preserve assertions and skip states. Never make a test pass by introducing a translation mock or deleting its behavior.
Retain the full multilingual setup for direct and indirect locale-switching contracts.

The static inventory contains 450 Node candidates, 1,554 English-browser candidates, and 753 retained or unresolved files.
These counts overlap the current reviewed lists and are not migration promises.
Record each reviewed file's disposition and evidence; rebase the inventory against the implementation head before measuring the complete suite.
Any test consolidation requires the TDD test-audit keeper and mutation evidence. This package requires no test deletion.

### Fewer scheduling stages

Combine change detection and runner allocation into one GitHub-hosted bootstrap job in each test workflow.
Keep the existing `changes` job ID and publish both change outputs and the allocation `plan` from it.
Update consumers to `needs.changes.outputs.plan`; remove the separate `runner_plan` job.
Planner failure fails the bootstrap. Preserve sidebar-resource dispatch outputs and existing event-specific comparison logic.
Keep the runner allocator, percentage semantics, action pins, and protected execution boundaries intact.
Change detection remains hosted; this removes an eligible external singleton but never widens the external trust boundary.
Update its documented job inventory and workflow contract tests together.

Make the existing hosted E2E report job the required `E2E Tests Passed` job.
Remove the separate `e2e-gate` job and its allocation family.
The combined job uses `if: always()` and depends on change detection, build, image resolution, every shard family, desktop, and Kubernetes compatibility.
Conditional report steps run only for a relevant test run with available evidence.
The final result fails if a required dependency failed, was cancelled, unexpectedly skipped, or produced incomplete test evidence.
Report-step failures remain job failures; no later success command can erase them.
A successful irrelevant-change decision skips report installation and merging, then publishes a successful required result with the reason.
Preserve merged reports, timing profiles, retry diagnostics, and successful-main profile authority.

This removes four jobs per fully selected backend/frontend/E2E workflow set.
It removes two serial queue boundaries from the E2E path: the separate allocator and final gate.
It does not reserve runner slots or guarantee a queue percentile.
Keep separate backend and frontend gates where they must join independent required jobs.
Do not run an idle gate that polls for other jobs while consuming capacity.

### Measurement and adoption

Add a bounded, read-only CI measurement helper under `.github/scripts/`.
It consumes saved run, attempt-specific job, and step responses; optional collection uses authenticated GET requests.
It emits machine-readable data and a concise report with provenance, counts, cache evidence, and incomplete-data markers.
Its fixtures cover overlaps, retries, skipped jobs, missing timestamps, unfinished jobs, cancellations, and multiple workflow definitions.
Use dependency-ready-to-start delay only when the matching workflow graph is known.
Never label all time after workflow creation as runner queue time.

Run the helper on the sampled runs and later candidate runs; it does not create another always-on workflow job.
Record runner-minutes by event and change class, including superseded attempts.
Do not claim p90 or p95 improvements from three samples or from a changed PR mix.
At 60 slots, 600 runner-minutes consume ten minutes of the whole fleet's service capacity.
This is a capacity illustration, not a prediction of arrival rate or actual throughput.

Frontend setup has a delivery target of 40% lower median execution and 30% fewer frontend runner-minutes.
These are targets for the full suite; the local 32-file experiment proves only that sample.
Select one, two, or four frontend test partitions after setup optimization.
Adopt the smallest candidate that meets AC-PLATFORM-CI-PERFORMANCE-005.3, or retain one partition with a recorded rejection.
No candidate increases the 14 normal or six container E2E shards.

### E2E cost follow-up

Keep the [duration-aware E2E contract](e2e-duration-aware-sharding.md) authoritative for manifests, profiles, and test coverage.
The October container sample predicts 720 seconds for an opt-in file that skips without `KANDEV_E2E_FULL_WORKER_IMAGE`.
Other shards exceed their predicted duration. Treat skip context and fixture overhead as separate calibration problems.
Task 10 measures these costs and supplies a bounded follow-up proposal; it does not invent a new manifest schema or omit skipped declarations.

Measure shared `testPage` reset calls, integration cleanup, seed Git restoration, browser creation, worker startup, and test bodies separately.
Keep repository restoration and cleanup ordering unless their owning invariant and failure behavior are proved equivalent.
Existing one-worker resource guards stay in force.
Real-clock idle, LSP release, retry-exhaustion, and coordinator tests remain until a domain-specific replacement proves the same lifecycle.
Increasing their timeout or reducing their retries is not a performance fix.

## Conservative test-only selection

Add a small classifier to the E2E workflow's existing bootstrap, after a valid diff is available.
The first implementation recognizes only pull requests whose non-documentation changes are regular `apps/backend/**/*_test.go` files.
Go excludes these files from the application build. Backend tests and static checks retain their current selection.
Use the repository's existing documentation exclusions; do not broaden them.

Before adoption, audit repository build scripts, `go:embed` patterns, and E2E fixtures for dependencies on those test files.
A dependency makes that path ineligible for the exemption. Protect known exceptional dependencies in the classifier tests.
Parse a NUL-delimited name/status diff, including both rename paths and file modes.
Renames into or out of production paths, symlinks, submodules, malformed input, an empty ambiguous diff, or an unavailable base select the current full path.
Only an explicit, successful test-only classification sets E2E `run=false` with a distinct reason output.
Unrecognized paths retain the current full verification path.

Frontend test-only changes remain outside the first exemption.
Their transitive imports, source-reading tests, and bundler inputs require stronger proof than a filename suffix.
E2E specifications and fixtures always retain application E2E selection.
Push-to-main, merge-group, and dispatch behavior remains unchanged, including their existing documentation-only skips.
No results or build artifacts are reused across different source revisions.

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
- [Capacity-aware verification decision](../../../decisions/2026-10-08-capacity-aware-ci-verification.md)

The existing runner package is complete. It needs no repeated implementation.
The E2E efficiency package retains open rollout evidence. Link new measurements there without marking unperformed rollout work complete.
The October decision records capacity budgets and the narrow test-only selection boundary.
