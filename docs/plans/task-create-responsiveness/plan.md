---
created: 2026-11-03
status: in_progress
requirements:
  - REQ-TASKS-CREATE-RESPONSIVENESS-001
system_design:
  - ../../specs/tasks/system-design/task-create-responsiveness.md
legacy_specs: []
---

# Implementation plan: Task creation responsiveness

## Overview

Reduce synchronous creation work while other agents produce output.
First capture the full HTTP baseline. Then combine prepared-session writes.
Finally remove unchanged preference writes and prove creation with color edits.
All work orders run sequentially after an explicit implementation request.

Tasks owns this package because it owns creation acceptance and initial sessions.
Platform keeps shared writer and durable-delivery ownership.

## Evidence and limits

Read-only diagnosis used the backend at port 38429, database
`/root/.kandev/data/kandev.db`, and retained backend logs.
The diagnostic snapshot was `7841a789eade1c49c97baa292db78d70`.
Its manifest reported a partial archive because the byte limit truncated older logs.
The temporary materialized archive was removed after this evidence summary.
No live task, settings, or database record changed during reproduction.

For task `c60a44b9-4b5a-48fc-9614-145868977ce7`, the logs recorded:

- `task created` and `preparing task session` at 22:23:34.310 UTC.
- `task session prepared` at 22:23:43.911 UTC.
- Scheduling state publication at 22:23:43.984 UTC.

The preparation interval was 9.601 seconds. Existing phase logs do not show
which internal operation consumed it. The observed writer statistics showed
244,333 waits and 3,738.145 seconds of cumulative waiting across callers.
Those cumulative values are not the duration of one request.

The existing `BenchmarkDeliveryWriterContention/final` used the production
factories and 20 task-row inserts per load point:

| Sessions | Insert p50 (ms) | Insert p95 (ms) | Offered events/s | Projected events/s |
| --- | --- | --- | --- | --- |
| 0 | 0.432 | 0.526 | 0 | 0 |
| 4 | 1.047 | 3.121 | 384.7 | 399.5 |
| 8 | 10.060 | 52.890 | 799.3 | 735.2 |

The short timed window can project queued events from warmup.
These results prove insert sensitivity to load, not full-request latency or
acceptance of this package's performance criteria.
The earlier [delivery package](../delivery-writer-contention/plan.md) already
records an unmet relative latency target. Its result remains unchanged.

Source tracing found four separate acquisitions for ordinary seeded session
preparation: creation, primary promotion, preview metadata, and submission metadata.
Last-used choices also update the settings revision for identical values.
These are verified avoidable costs, not a complete attribution of the live delay.

A temporary `TestDiagnosticTaskCreationPrepareWrites` ran the real orchestrator,
executor, and repository against a disposable database with separate production
writer and reader pools. Its SQLite trace recorded two explicit commits and
two metadata autocommits during one preparation. It executed 11 INSERT/UPDATE
statements, including transaction guards, environment insertion, primary updates,
and prompt metadata. The provider resolver was a local test double.
The trace passed and its temporary test was removed after diagnosis.

## Scope

### In scope

- A full create-request and unrelated-settings contention benchmark.
- Initial metadata in the existing session insert transaction.
- Primary election in that same transaction, with committed callback ownership.
- Conditional last-used updates that preserve concurrent colors.
- Focused SQLite/PostgreSQL and desktop/phone regression evidence.

### Out of scope

- Aggregate Begin/Complete/Abort creation and archive migration.
- A second writer, priority queue, longer timeout, or general mutation retry.
- Optimistic success, API changes, new controls, and layout or locale changes.
- Agent launch, Git/provider I/O, or event fan-out redesign.

## Technical approach

`orchestrator/task_operations.go` carries initial submission state to
`orchestrator/executor/executor_execute.go` before session persistence.
The shared typed seed belongs in `task/models`.
`task/repository/sqlite/session.go` and its existing preparation variants own
atomic insertion and initial primary selection. Explicit promotion stays separate.

`user/store/sqlite.go` changes targeted last-used updates in both dialects.
`user/service/service.go` publishes only changed preferences.
Task handlers retain their current settlement, response, and dispatch sequence.

| Path | Required behavior | Evidence |
| --- | --- | --- |
| SQLite, immediate start | One preparation commit with seed and election | Trace, rollback tests, full benchmark |
| PostgreSQL, immediate start | Same result under task-before-session locks | Real concurrent connection tests |
| Bound/shared workspace or route | Same guards and committed primary result | Existing variant and retry tests |
| Office reuse | Preserve reused metadata and primary | No-write reuse regression |
| No agent, WIP queue, or blockers | Retain current preparation and launch policy | Handler regressions |
| External-ID retry | Found result performs no new work | Existing duplicate tests |
| Changed/unchanged preferences | Preserve colors and other workspace choices | Store/service concurrency tests |

No rendered UI changes require a preview.
Mobile parity uses the existing creation surface, navigation drawer, and color menu.
Public documentation needs no change for this internal design package.

## Tests

- `TestPreparedSessionSeedAtomicity`: seed, insert, rollback, cancellation,
  workspace variants, and initial runtime seed.
- `TestPreparedSessionPrimaryElection`: concurrent creators, existing primary,
  explicit promotion, callback cardinality, and binding retry.
- `TestTaskCreateLastUsedNoOp`: unchanged revision, changed event, missing user,
  omitted fields, and concurrent colors/workspace entries.
- `TestHTTPCreateTask`: preparation, settlement, duplicate identity, WIP/blockers,
  plan mode, initial submission, and asynchronous launch boundaries.
- `BenchmarkTaskCreateContention`: full request, color edits, and actual delivery
  rates for AC-TASKS-CREATE-RESPONSIVENESS-001.1 and .2.

## E2E tests

Extend the desktop and phone creation tests with another active task and a
subsequent saved color. Correlate the created task/session and await actual HTTP
completion. Keep latency acceptance in the backend benchmark.
The browser tests cover AC-TASKS-CREATE-RESPONSIVENESS-001.2, .3, and .6.

- `apps/web/e2e/tests/task/task-create-contention.spec.ts`, project `chromium`.
- `apps/web/e2e/tests/task/mobile-task-create-contention.spec.ts`, project `mobile-chrome`.
- Existing `task-create-recovery.spec.ts` and `mobile-task-create-recovery.spec.ts`
  retain uncertain-outcome coverage.

## Work orders

- [ ] [01: Measure full creation under load](task-01-measure-creation.md)
- [x] [02: Persist prepared sessions once](task-02-prepare-session-once.md)
- [ ] [03: Avoid unchanged preferences and verify interactions](task-03-preferences-and-interactions.md)

## Verification results

The implementation changes are complete, but the corrected full-path benchmark
does not establish AC-TASKS-CREATE-RESPONSIVENESS-001.1 on this shared host.
The earlier benchmark result was produced by a fixture that bypassed real
orchestration, used a repository-less create, and simulated streaming through
ordinary message inserts. That result is superseded and must not be cited.

### Backend verification

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run 'TestPreparedSession(SeedAtomicity|PrimaryElection)|TestSetSessionPrimary|TestCreateTaskSession|TestWorkspaceBindingRetryOwnership' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/orchestrator/executor -run 'TestPreparedSession|TestPrepareSession' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/orchestrator -run 'TestPreparedSession|TestFreshStartSubmission|TestPrepareTaskSession|TestInitialPrompt|TestInvalidInitialPromptSeed' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/handlers -run 'TestHTTPCreateTask|TestWSCreateTask|TestTaskCreateContentionFixture' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/user/store ./internal/user/service -run 'TestTaskCreateLastUsed|Test.*TaskCreateLastUsed|Test.*Sidebar.*Color' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/persistence/storeconformance -count=1)
```

All commands passed. The PostgreSQL-gated variants were not executed because
`KANDEV_TEST_POSTGRES_DSN` is unavailable in this environment.

### Browser verification

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/task/task-create-contention.spec.ts tests/task/sidebar-task-color-sync.spec.ts tests/task/task-create-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-create-contention.spec.ts tests/task/mobile-sidebar-task-color-sync.spec.ts tests/task/mobile-task-create-recovery.spec.ts)
```

Both commands passed in emulated Chromium projects. No physical-device
validation was available, so physical keyboard/touch behavior remains an
unverified hardware gate rather than a passed check. The contention tests start
a real mock-agent session that keeps generating, create the new task, then
change the *other* generating task's color through the desktop menu or phone
drawer context menu. They verify the generating session remains RUNNING and the
unrelated color survives reload.

### Corrected full-path benchmark

The corrected fixture uses the real orchestrator `LaunchSession` preparation
path, a real local Git repository, the production task/user services, the
production SQLite writer and reader pools, and real durable delivery
receive/projection, cursor advancement, and durability-barrier ACK work. Only
the asynchronous provider/process launch is substituted.

Retained binaries:

- Baseline: `/tmp/opencode/task-create-baseline-final.test`,
  SHA-256 `4fc967b0487c63878536044f79d80ba1e8389ca3ed9ef29569f253c6dbef90b0`.
- Candidate: `/tmp/opencode/task-create-candidate-final.test`,
  SHA-256 `671390a6614d1edcfecdc30829c6d1299936ad75279c77e52454dfff62b3b014`.

Five consecutive eight-stream runs per binary were compared on the same host.
Baseline p95 values ranged from 38.9 ms to 51.4 ms (median 39.8 ms); candidate
p95 values ranged from 37.2 ms to 47.8 ms (median 37.5 ms). The median
improvement is about 5.8%, far below the required 25%. All ten latest runs
delivered at least 98.36% of offered events, but an earlier candidate run under
load missed the 90% bound. AC .1 is unproven, and AC .2 still lacks stable
run-to-run evidence.

The corrected delivery checks are stricter than the earlier benchmark. In a
complete run, offered, projected, dropped, stream-error, ACK-error, and writer
wait counters now all describe the measured interval; draining after
`StopTimer` is cleanup only. Candidate runs that completed recorded 97.96% to
99.73% timely delivery, zero color errors, ten color samples, and a saved-color
readback on the unrelated task. Runs below 90% timely delivery fail the
benchmark. A failed color edit or fewer than three color samples also fails.

Writer acquisitions, commits, and individual SQL statements are still not
counted separately. That limitation remains recorded rather than inferred from
pool wait counters.

### Commands

```bash
(cd /tmp/opencode/kandev-baseline-fixup/apps/backend && go test -c -trimpath -tags fts5 -o /tmp/opencode/task-create-baseline-final.test ./internal/task/handlers)
(cd apps/backend && go test -c -trimpath -tags fts5 -o /tmp/opencode/task-create-candidate-final.test ./internal/task/handlers)
nice -n -20 /tmp/opencode/task-create-baseline-final.test -test.run '^$' -test.bench '^BenchmarkTaskCreateContention/final/start_agent=(true|false)/sessions_8$' -test.benchtime=100x -test.count=5
nice -n -20 /tmp/opencode/task-create-candidate-final.test -test.run '^$' -test.bench '^BenchmarkTaskCreateContention/final/start_agent=(true|false)/sessions_8$' -test.benchtime=100x -test.count=5
```

Completed diagnostic command:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run '^$' -bench '^BenchmarkDeliveryWriterContention/final/sessions_(0|4|8)$' -benchtime=20x -count=1)
```

The benchmark passed. Its measurements are recorded above.
The preparation trace passed with this command before its temporary file was removed:

```bash
(cd apps/backend && go test -trimpath -tags 'fts5 sqlite_trace' ./internal/orchestrator -run '^TestDiagnosticTaskCreationPrepareWrites$' -count=1 -v)
```

This command documents the diagnostic procedure. Task 01 adds its durable fixture.
Design validation passed:

- `python3 scripts/list-docs.py validate`: 428 decisions and 1,904 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check`: no whitespace errors.
- The repository's local `validateCoverage` API accepted all three work orders
  against the planned orchestrator and user-store source boundaries. The actual
  documentation-only change is exempt from the PR coverage gate.

The temporary test and materialized diagnostic archive were removed.
No isolated backend or browser process required teardown.
Work-order results must contain the later implementation commands and measurements.

## Risks

- The live delay can include profile reads, callbacks, and unrelated worker load.
  The full-path benchmark must establish the actual benefit.
- Combining transactions can lengthen one writer hold despite fewer acquisitions.
  Color latency and delivery throughput are acceptance gates.
- Primary callbacks can misreport ownership after a binding retry or race.
  Tests must observe the committed row and callback together.
- A settings no-op must compare current stored values without suppressing a
  concurrent choice or losing a color update.
- The separate aggregate creation draft can change these integration points.
  Re-read that package if it lands before implementation.
- Shared-host benchmark noise can obscure the target. Record a missed bound
  and leave the work pending until the same workload satisfies every bound.
