---
id: "02-verify-debug-source-build"
title: "Verify served diagnostics and repeat the memory comparison"
status: done
wave: 2
depends_on:
  - "01-pin-build-environment"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-SOURCE-FRONTEND-BUILD-MODE-001
acceptance_criteria:
  - AC-PLATFORM-SOURCE-FRONTEND-BUILD-MODE-001.1
  - AC-PLATFORM-SOURCE-FRONTEND-BUILD-MODE-001.2
  - AC-PLATFORM-SOURCE-FRONTEND-BUILD-MODE-001.6
system_design:
  - ../../specs/platform/system-design/source-frontend-build-mode.md
---

# Task 02: Verify served diagnostics and repeat the memory comparison

## Summary

Verify production React and Kandev diagnostics through actual debug source startup.
Repeat the reported memory comparison with the corrected build and record the measured limits.
Update the public CLI and configuration references in the same implementation change.

## In scope

- Add `apps/web/scripts/debug-start-production.test.mjs` as a focused headless browser integration check.
- Launch `make start-debug` with inherited `NODE_ENV=development`, disposable data, and owned processes.
- Verify production React in the browser and the JavaScript assets served by Go.
- Verify Kandev frontend logging, backend debug file logging, pprof, and agent-message debug enablement.
- Repeat timing-entry and memory measurements over at least 35 minutes.
- Record sanitized evidence in `docs/plans/debug-start-production-react/evidence.md`.
- Update `docs/public/cli.md` and the debug section of `docs/public/configuration.md`.

## Out of scope

- A new product performance bound or a claim that every renderer crash is fixed.
- Logging changes, performance-entry clearing, or arbitrary instrumentation added to production code.
- A new CI soak workflow or changes to the standard managed E2E fixture.
- Mobile layout changes, new configuration settings, and profiling commands.

## Acceptance

1. A fresh served debug build reports production React, retains runtime debug logging, and exposes the expected backend diagnostics under inherited development.
2. The memory comparison records its workload, duration, measurements, unavailable metrics, and environment differences. Its conclusion follows the measurements without claiming unsupported crash resolution.
3. Public references explain production React with runtime diagnostics and the development alternative. Focused tests and documentation checks pass.

## Integration check

Use Node's test runner and the existing Playwright Chromium dependency.
Keep this check separate from the standard E2E fixture because the regression concerns source-start compilation and native launcher handoff.

The check owns a temporary `KANDEV_HOME_DIR`, a free `KANDEV_BACKEND_PORT`, and its browser context.
Bind `KANDEV_SERVER_HOST=127.0.0.1` and set `KANDEV_NO_BROWSER=1` for its child.
Clear inherited source selectors, configuration-file overrides, external web-dist overrides, and diagnostic threshold overrides for that child.
Do not print the complete environment or change the parent environment.
Use a fresh build and verify that the served asset hashes match that output.
Treat build failures and stale artifacts as failures despite the quiet recipe's exit behavior.

Install the React DevTools observation hook before application scripts execute.
Require the actual `react-dom` renderer to report `bundleType=0`.
Require a real renderer registration. Absence is not production evidence.
In production mode, fetch the served JavaScript chunks and reject the known React development-warning sentinels.
In explicit baseline mode, require the development renderer and its warning sentinels instead.

Assert `window.__KANDEV_DEBUG === true` and observe an actual namespaced Kandev debug message during app activity.
Verify a backend debug event in the private log file and an accessible pprof index.
Verify the agent-message debug setting in the owned supervisor manifest.
The existing diagnostic tests remain part of the evidence for the environment handoff and log thresholds.
No external agent or private conversation is necessary to establish these settings.

Always close the browser and stop only the launcher tree created by the check.
Use cleanup on assertion failure, timeout, and interruption.
Do not mutate a developer's running instance or use broad process-kill commands.

## Memory comparison procedure

`KANDEV_DEBUG_START_SOAK_MINUTES` is an integration-test input, not a runtime configuration option.
With this input set to `35`, the check keeps the corrected instance and browser active for at least 35 minutes.
It records samples at the start, each minute, and the end.

Use a fixed disposable task/workspace dataset and repeat the same task-navigation and page-navigation sequence.
The sequence must exercise React updates between samples, rather than leave an inert page open.
Record the sequence, source commit, dependency versions, browser version, operating system, dataset size, and observation duration.
Do not clear marks or measures between samples.

Record these measurements separately:

- `performance.getEntriesByType("measure").length` and the dominant entry names.
- Browser JavaScript heap usage from Chromium diagnostics.
- Heap-snapshot `PerformanceMeasure` counts and `UserTiming` retained bytes, when available.
- Renderer process memory in bytes summed across identified renderer processes, when available.

Browser heap usage and renderer process memory are different measurements.
If the automation cannot obtain a metric, record it as unavailable.
Keep raw heap snapshots local. Commit only sanitized aggregates and procedure details.

Compare initial and final samples with the issue's 34-minute observation.
For causal attribution, also repeat the identical workload on the pre-fix commit in a separate disposable checkout.
Use commit `81cb56a3550f7fb7fe5c5b4a36a2305882984500` as the investigation baseline.
Run the current check against that checkout in an explicit baseline mode that expects development React.
The check's test-only inputs are `KANDEV_DEBUG_START_REPO` for that checkout and `KANDEV_DEBUG_START_EXPECT_REACT=development` for that mode.
Default execution always expects production React and must not infer baseline mode from a failing assertion.
Install dependencies in the baseline checkout before its first package command.
Run baseline and corrected observations sequentially with fresh browser contexts and identical data.
Use identical observation hooks in both runs and record their presence.

Use the reporting macOS/Chrome environment when available.
If only a different host is available, state that difference and limit the conclusion to that host.
If the baseline cannot reproduce the growth, do not claim a measured reduction in the original failure.
If the corrected build retains substantial timing or process-memory growth, record the remaining evidence without expanding this patch automatically.

## Verification

Run from the repository root:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm run test lib/debug/log.test.ts src/boot-payload.test.ts)
(cd apps/web && NODE_ENV=development node --test scripts/debug-start-production.test.mjs)
(cd apps/web && NODE_ENV=development KANDEV_DEBUG_START_SOAK_MINUTES=35 node --test scripts/debug-start-production.test.mjs)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check -- apps/web/scripts/debug-start-production.test.mjs docs/public docs/plans/debug-start-production-react
```

For the matched baseline, use an owned temporary worktree:

```bash
baseline_dir="$(mktemp -d "${TMPDIR:-/tmp}/kandev-5050-baseline.XXXXXX")"
git worktree add --detach "$baseline_dir" 81cb56a3550f7fb7fe5c5b4a36a2305882984500
(cd "$baseline_dir/apps" && pnpm install --frozen-lockfile)
(cd apps/web && NODE_ENV=development KANDEV_DEBUG_START_REPO="$baseline_dir" KANDEV_DEBUG_START_EXPECT_REACT=development KANDEV_DEBUG_START_SOAK_MINUTES=35 node --test scripts/debug-start-production.test.mjs)
git worktree remove "$baseline_dir"
```

The integration check cleans up its runtime even if the test fails.
If a command interrupts the baseline block, remove only that owned worktree after its runtime stops.
Do not mark this task done before both observations and their limitations are recorded.
Run the package's specification and coverage checks after documentation changes.

## Public documentation

The CLI and configuration pages are reference material.
Describe production React with runtime Kandev diagnostics, including inherited development environments during source startup.
Keep `make dev` as the normal development React path.
Do not promise that this build correction eliminates all memory growth.
The root README and screenshot catalog contain no affected debug-start description at planning time.
No screenshot update is necessary.

## Files likely touched

- `apps/web/scripts/debug-start-production.test.mjs` (new)
- `apps/web/scripts/lib/debug-start-production.mjs` (new)
- `apps/web/scripts/lib/debug-start-production-process.mjs` (new)
- `docs/plans/debug-start-production-react/evidence.md` (new)
- `docs/public/cli.md`
- `docs/public/configuration.md`

## Dependencies

[Task 01](task-01-pin-build-environment.md).
The served check requires workspace dependencies, Go build prerequisites, and the installed Chromium browser.

## Risks

- React DevTools metadata is a version-specific test observation. Pair it with served-asset inspection.
- A build failure can leave an older dist or binary. Asset freshness must be proven before browser assertions.
- Native Windows startup and the reporting host's Chrome memory behavior need host-specific evidence.
- A 35-minute run cannot establish the absence of all long-duration leaks or crashes.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/platform/requirements/source-frontend-build-mode.md).
- [Diagnostic handoff and verification design](../../specs/platform/system-design/source-frontend-build-mode.md#diagnostic-handoff).
- [Issue and investigation evidence](plan.md#evidence-and-root-cause).
- `apps/web/AGENTS.md`, `/tdd`, `/e2e`, and `/docs-maintainer` guidance.

## Results

Completed on 2026-11-05.

- Added `apps/web/scripts/debug-start-production.test.mjs` with shared helpers under
  `apps/web/scripts/lib/`. The corrected source start served production React 19.2.3 with renderer
  `bundleType=0`, matched all 86 served JavaScript assets to the fresh build, omitted React
  development-warning sentinels, kept the performance measure count at zero, and retained
  `window.__KANDEV_DEBUG`, namespaced debug logging, backend debug file events, pprof, and the
  agent-message debug setting.
- The check pins a private SQLite configuration and an owned database path under its temporary
  home, overrides inherited hostile PostgreSQL configuration and environment, proves the launcher
  resolved the owned database before API writes, and confirms the outside sentinel database stayed
  untouched.
- The observation workload is mandatory: it opens Home, the created task route, Settings, and the
  theme toggle, asserts each resulting route or control state, records successful rounds, and fails
  on a document reload. The corrected short run completed 2 rounds with 0 document loads and 0
  measures. The short baseline completed 2 rounds and counted `6,947` to `33,356` measures.
- Cleanup drains the owned make, launcher, and backend process groups with portable POSIX process
  table identity checks, re-reads backend evidence on startup failure, and handles repeated stop
  requests and interruption.
- The corrected 35-minute replacement observation kept the measure count at zero across 387
  required rounds, with 0 document loads. The matched baseline replacement accumulated `4,378,212`
  measures across 351 rounds and reached `6.29` GB renderer RSS.
- Recorded the procedure, environment, measurements, unavailable metrics, and limitations in
  [evidence.md](evidence.md). The corrected build still grew renderer RSS, so the result does not
  claim that renderer crashes are resolved.
- Updated `docs/public/cli.md` and the debug section of `docs/public/configuration.md`.
- Native Windows startup and the reporting macOS/Chrome memory behavior were not available. Recipe
  generation is verified on Linux only.
