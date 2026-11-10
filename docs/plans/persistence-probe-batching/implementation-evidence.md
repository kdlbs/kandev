# Implementation evidence

Implementation authorized after review of the design package. Tests and
measurements run on October 20, 2026, against the working tree based on
`595df36ed5c1cf02fd90cb972b623ad6edc53df8`.

## Change

`db.ExistingSQLiteTablesContext` deduplicates requested names, binds them into
one SQLite catalog query, and returns a complete table inventory. Query, scan,
and iteration errors discard all results. Rows close before the helper returns.
The helper rejects other drivers.

After both existing pool pings, SQLite health uses this inventory for independent
store results. Missing shared tables fail all dependent stores. An incomplete
inventory fails all stores and produces one batch diagnostic. PostgreSQL retains
its existing point-query path. The timeout, interval, maintenance admission,
tracker publication, middleware, and writer/reader pool sizes are unchanged.

## Regression proof

Before production edits, this command failed with **expected 1, actual 97**:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/persistence/requiredstores -run '^TestHealthSQLiteCatalogUsesOneSchemaQuery$' -count=1 -v)
```

After implementation, it passed with one executed schema query. A test-local
SQLite driver wrapper counts actual queries across the complete production
catalog. It adds no production observation hook.

Further tests cover bound unusual names, duplicate requests, views and indexes,
empty inventories, shared missing tables, multi-table stores, scan failures,
partial iteration failures, query failures, cancellation, connection release,
and recovery. Factory-created writer and reader pools verify that holding all
connections still fails the probe at the correct ping stage and that releasing
them restores health.

## Checks

Passed:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/db ./internal/persistence/requiredstores ./internal/backendapp -run 'Test(ExistingSQLiteTablesContext|HealthSQLiteCatalog|PersistenceContentionFixture|RuntimeHealth|StartupHealth|HealthCheck|ProbeTables|RequiredPersistence|PersistenceMiddleware)' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
```

Also passed:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/persistence/storeconformance -count=1)
PATH="/tmp/kandev-merge-tools:$PATH" bash scripts/lint-go-changed apps/backend/internal/db/schema_tables.go apps/backend/internal/db/schema_tables_test.go apps/backend/internal/persistence/requiredstores/health.go apps/backend/internal/persistence/requiredstores/health_batch_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

Store conformance passed in 181.415 seconds. Lint reported zero issues. The
linter was absent from the initial PATH; the existing local golangci-lint
2.9.0 binary, built with Go 1.26.6, supplied the required command. Documentation
coverage passed for all three changed production paths. Catalog validation
covered 384 decisions and 1,616 specifications.

The final isolated comparison passed, as recorded below.
PostgreSQL integration coverage requires `KANDEV_TEST_POSTGRES_DSN`, which is
not configured in this environment. No PostgreSQL behavior changes in this patch.


## Final isolated HTTP comparison

```bash
python3 .kandev/diagnostics/stress-503/replay.py --candidate current --rounds 3 --seconds 35
```

Each run restored the same synthetic seed. Eight clients updated separate task
descriptions with 16 KiB payloads for 35 seconds. The order alternated between
original and implemented probes. No task-owned compilation, race test, or other
load ran during measurement. Both binaries were built before measurement.

| Run | Successful writes | Write 503s | Workspace-read 503s | Successful-write p95 (ms) | Probe failures |
| --- | ---: | ---: | ---: | ---: | ---: |
| pair1-baseline | 866 | 1031 | 64 | 475.416 | 1 |
| pair1-batched | 1268 | 0 | 0 | 533.703 | 0 |
| pair2-batched | 1606 | 0 | 0 | 377.536 | 0 |
| pair2-baseline | 1011 | 1021 | 64 | 380.070 | 1 |
| pair3-baseline | 1023 | 1021 | 64 | 374.429 | 1 |
| pair3-batched | 1510 | 0 | 0 | 411.598 | 0 |

Across the three runs, the original probe produced 3,073 write 503s and 192
workspace-read 503s. It completed 2,900 writes. The implemented probe completed
4,384 writes with zero write, workspace-read, or readiness errors. All three
original runs reported a table-probe deadline failure; all implemented runs
remained healthy. Diagnostics recorded startup plus two periodic probe timestamps
in each implemented run, so zero failures does not mean the probe was skipped.

Successful-write latency remained in a similar range. This change improves
availability for the tested workload; it does not establish faster individual
writes or unlimited write capacity. Other host workloads can affect timing.
The workload does not identify the writer operation behind the historical incident.

The baseline uses the preserved original algorithm with its temporary total-time
logging. The candidate uses the measured implementation without success logging.
Do not compare successful probe durations between these two builds. Both retain
the normal failure diagnostic and the same timeout and interval.

Exact SHA-256 hashes:

- Baseline health source: `100cb85696302ab72e6816982846e8f0279d17b9398ae5bc4806848231f95299`.
- Implemented health source: `310a63968440fc7aa776f8bdf0f44e0b602ea76cd9d51c1a719ceca3f1ede7af`.
- New schema helper: `2e83a7cd844038be049161a4f435c0eafca44f5eb84a1c0ca185e3d0e842f992`.

[implementation-measurements.json](implementation-measurements.json) contains
counts, latencies, sampled health states, timestamps, diagnostics, and hashes.
Raw evidence remains in the ignored task-local directory
`.kandev/diagnostics/stress-503/replay-20261020T091747/`.

## Cleanup and delivery

All six backends stopped. A detached Hermes discovery helper survived process-group
shutdown and recreated its disposable home. Its environment identified this
replay root; it was stopped separately, and its files were removed. No process
with the replay root in its command or environment remained. The live instance
was not changed. The implementation is ready for code review; it has not been deployed.


## CI import-cycle correction

The first PR head passed the scoped checks but failed backend static lint in
job `118559826223` because `internal/db/dialect` tests import `internal/db`.
The new helper's reverse import created a test-package cycle. The exact local
reproduction was:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/db/dialect -count=1)
```

It failed with `import cycle not allowed in test`. The helper now compares the
same `sqlite3` driver-name value directly, matching the shared schema layer's
existing approach. SQL, bindings, pool selection, and runtime behavior are
unchanged. The three stress pairs above remain measurements of the pre-correction
source hashes; this import-only correction does not add a new performance claim.
The work order now requires the entire dialect test package as well.

Post-correction validation:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/db/dialect -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/db ./internal/persistence/requiredstores ./internal/backendapp -run 'Test(ExistingSQLiteTablesContext|HealthSQLiteCatalog|PersistenceContentionFixture|RuntimeHealth|StartupHealth|HealthCheck|ProbeTables|RequiredPersistence|PersistenceMiddleware)' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
PATH="/tmp/kandev-merge-tools:$PATH" bash scripts/lint-go-changed apps/backend/internal/db/schema_tables.go apps/backend/internal/db/dialect/aggregate.go apps/backend/internal/persistence/requiredstores/health.go
```

The complete dialect package, all new and changed health/schema regressions,
existing fault/recovery controls, and SQL guard passed after the correction.
Changed-package lint also passed, including `internal/db/dialect`, with zero
issues. The correction was pushed. Subsequent CI exposed the SSH cleanup
failure described below. Final CI evidence remains pending.

Corrected schema-helper SHA-256: `10eae37d25e553371ab3b9a7a5fddb94b76c31c4d8b5fbe4ccbba03829d4c75d`.

## CI SSH cleanup correction

Backend job `118580188441` in run `39405794933` failed
`TestSSHOrphanStopCommandKillsProcessAndDirectChildProcessGroup`. Its log reported
that the target remained running immediately after SIGKILL. The same existing
test passed 20 local runs with race detection, so those passes did not establish
that signal delivery had completed before the final state read.

The generated stop script checked liveness once after sending SIGKILL. The new
`TestSSHOrphanStopCommandWaitsForKillDelivery` controls the shell's OS boundary:
the process remains live until the script yields after SIGKILL, then either exits,
becomes a zombie, or remains live. The exit and zombie cases failed before the
fix with the same error as CI. The live case verifies that cleanup remains
blocked when termination does not complete.

The script now polls for completed signal delivery within a second bounded
five-second window. It retains the existing identity checks, child-group
signals, zombie handling, and session-directory ownership checks. This is CI
remediation of the existing SSH stop contract, separate from the SQLite query
optimization. It changes none of the measured health-probe sources.

Validation commands from `apps/backend`:

```bash
go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle -run '^TestSSHOrphanStopCommandKillsProcessAndDirectChildProcessGroup$' -count=20
go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle -run '^TestSSHOrphanStopCommandWaitsForKillDelivery$' -count=1
go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle -run '^TestSSHOrphanStopCommand' -count=3
go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle -count=3
```

The first command passed before the correction. The second produced the expected
RED result. After the correction, the third passed, including the new regression.
The full package run did not pass locally. This Nix host has no `/bin/sleep`,
which two existing process-fixture tests require. The checkout also contains
`bin/agentctl-darwin-amd64`, which invalidates an existing binary-absence test.
A separate pruned-delivery test exhausted its three-second context during
repository setup. The repeated suite then exceeded Go's ten-minute test limit.
These results are retained in the task-local log and are not a passing package
result. The pruned-delivery setup failure also reproduced in a focused three-run
command. Its three-second context now starts after repository creation, so the
existing budget applies to database operations and delivery diagnosis instead
of schema setup. This test-only correction changes no runtime behavior or
timeout value.

Final scoped validation passed three times after both corrections:

```bash
go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle -run '^(TestSSHOrphanStopCommand|TestPrunedDeliveryStartupReportsAuthenticatedGapBeforeWebSocketAndACP)' -count=3
```

The command completed in 39.079 seconds. It covered the real-process stop tests,
the deterministic delivery regression, and both pruned-delivery modes. The local
shell coverage used `sh` and `bash`; `zsh` was unavailable and its cases skipped.
Scoped lint passed with zero issues:

```bash
PATH="/tmp/kandev-merge-tools:$PATH" bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/executor_ssh_orphan_stop_wait_test.go apps/backend/internal/agent/runtime/lifecycle/executor_ssh_orphan_sweep.go apps/backend/internal/agent/runtime/lifecycle/executor_ssh_orphan_sweep_scheduler.go apps/backend/internal/agent/runtime/lifecycle/pruned_delivery_startup_test.go
```

Updated-head CI remains pending.

## CI mobile transcript correction

The backend workflow passed at `5ee76233a861cba768ea15ad8315e883fbd0d20d`.
E2E shard 24 failed the mobile delayed changed-files test after reload, with a
176 px gap above the transcript bottom. Build and test jobs both used merge
`a40075cf1ddbed41775ad37ca348baa9afa9a55c`. Its tree matched the local synthetic
merge `02c1f175e7f168793673610624bdd5240e7f17ca`.

The isolated test and its 30-test CI partition initially passed without retries.
A disposable test variant with four-times Chromium CPU throttling reproduced
the same reload failure, with a 175 px gap. Geometry traces showed content growth
without a scroll event, followed by content shrinkage and native scroll anchoring.
The follow-intent snapshot missed the intermediate growth and classified the
anchoring offset as reader navigation.

The fix records geometry on content resize and recognizes matching anchoring
offsets for both growth and shrinkage. Existing reader-input, explicit-navigation,
hidden-panel, and disabled-auto-scroll guards remain in force. A pure geometry
regression failed before the change. An integrated blocked-placement regression
also failed with expected scroll offset 800, actual 500, before resize tracking.

Validation on the corrected source:

- Five focused scroll test files passed: 147 tests.
- The mobile reproduction passed three times with four-times CPU throttling,
  diagnostic traces, and zero retries.
- The original desktop and mobile delayed changed-files tests passed without
  diagnostic code or throttling: two tests, zero retries.
- Scoped ESLint, documentation catalog validation, and specification lint passed.
- Type checking passed with `NODE_OPTIONS=--max-old-space-size=8192`. The first
  attempt exhausted Node's default heap before reporting type errors.

All browser runs used disposable data and the exact CI source tree. The native
Nix host needed cached Chromium library paths and removal of inherited Kandev
service configuration from the child environment. No live instance changed.
Temporary browser instrumentation is excluded from the patch. The PR branch
now includes `main` through `e34761417d28e561ce9bd616cfa78c1b723df9ad`, which owns
the transcript code exercised by CI.


## Additional partition replay

A replay of CI partition 24-4 with one worker restricted to two CPUs completed
31 tests and failed two tests. All three Jira status-filter tests passed. The
original Jira CI flake also did not recur in five focused runs at four-times
CPU slowdown. No speculative Jira change was made.

The tablet LSP test failed both under CPU restriction and at normal CPU
availability. Reload correctly restored the editor tab, but the test waited
for a visible chat panel. The test now waits for the tablet layout before its
existing editor, retained-process, initialization-count, and progress checks.
The corrected test passed with the desktop and mobile delayed-card tests:
three tests, one worker, no retries. The two failing runs and the passing run
are retained in `/tmp/kandev-run.e2e.bZQdpKmc.log`,
`/tmp/kandev-run.e2e.hoVgftQW.log`, and
`/tmp/kandev-run.e2e.AEPQDPjl.log`.

The other pressure-run failure rejected native resume because received delivery
events were not fully projected. The focused test passed at normal CPU
availability. This observation remains unresolved and is not counted as a
passing pressure check. Fresh CI and review evidence remain pending.

Desktop and mobile screenshots show the complete latest changed-files card
after delayed loading and reload. Capture preparation closed the disposable
terminal panel to exclude the local host prompt. A failed capture attempt hit
the terminal confirmation and a port conflict. The final capture used free
explicit test ports and passed both viewport assertions.


## Latest-main scroll conflict reconciliation

The backend CI failure occurred before test execution when `proxy.golang.org`
returned an HTTP/2 `INTERNAL_ERROR` while downloading `sprites-go`. The first
failed-job rerun passed. All 85 checks on that revision then passed.

A new main commit changed the same follow-intent predicate and exposed a merge
conflict. Its regression requires upward native restoration during content
growth. This branch also requires upward anchoring during content shrinkage and
resize tracking when growth emits no scroll event. The resolution preserves all
three behaviors and main's older-page preparation change.

The incoming growth regression failed with this branch's predicate before the
resolution. After resolution, six focused scroll and turn-summary test files
passed all 168 tests. Type checking with an 8 GiB Node heap, scoped ESLint,
specification lint, and documentation catalog validation passed. Fresh backend,
web, plugin, and delivery-fixture builds passed. Desktop and mobile delayed-card
checks and the tablet LSP check passed together: three tests, no retries.
Database dialect, schema, required-store health, backend health, SSH cleanup,
and pruned-delivery regressions passed with race detection. SQL guard passed.

Browser setup first found a missing worktree-local delivery fixture, then a
port used by another process. The final run built the fixture and selected free
ports before startup. No test assertion was suppressed. Final screenshots and
remote checks remain pending for the merge commit.


## Git refresh fixture overhead under parallel CI load

The latest CI shard failed twice because the real `session.git.refresh` response
returned `success: false`. Isolated runs, repeated two-CPU runs, and the exact
partition on one CPU passed. A replay with four isolated test processes on four
CPUs reproduced `status_timeout` at the existing two-second refresh deadline.
The CI checkout and the branch had identical source trees.

The held-diff fixture started a Node process for every Git command while any
delay control existed. This included unrelated status and metadata commands.
The POSIX launcher now accepts a command hint owned by that gate. An unrelated
plain subcommand executes Git directly. Matching commands, global options,
missing or invalid hints, and active push controls retain the Node parser.
Windows retains its existing launcher path. The gate removes its hint when it
rearms or disposes. Production deadlines and Git validation do not change.

A new launcher regression failed before the fix because `git status` loaded
diff instrumentation. The focused launcher and real-Git integration suites
then passed all 13 tests. Coverage checks argument, input, output, environment,
and exit-status preservation. A positive control proves the hinted diff still
blocks until release. Temporary response logging is excluded from the patch.

The baseline parallel replay also exposed a transient session-model title and
a restored-terminal sentinel failure. These are separate observations. The
session-resume suite configures one retry, despite the command-line zero-retry
option. Its retry passed. Neither observation counts as a passing pressure check.
The candidate parallel replay passed the complete failing 29-test partition.
Across all four partitions, 126 tests passed and three failed. The terminal
reader failed again. A workflow preview failed with a browser protocol error
while reading a response body. MiniMax setup exceeded its existing 90-second
limit. These last two pressure observations remain unconfirmed.

The terminal screenshot showed successful command output split across two
visual rows. The buffer reader incorrectly inserted a newline at the wrap.
It now joins wrapped rows, preserves spaces at wrap boundaries, and retains
actual line breaks. Two new regression cases failed before this correction.

Final local checks passed:

- Launcher, real-Git integration, and terminal-reader tests: 16 tests.
- Fresh web E2E build, type checking with an 8 GiB heap, and scoped ESLint.
- Eleven focused browser tests without retries: desktop and phone Git recovery,
  both ended-session terminal paths, all workflow-preview cases, and MiniMax
  setup. The latter two observations did not recur in this isolated run.
- Documentation catalog validation, specification lint, and whitespace checks.

The pressure baseline and candidate evidence directories are
`/tmp/kandev-503-parallel23-w8h7jupr` and
`/tmp/kandev-503-parallel23-d8zls9oh`. The regression logs are
`/tmp/kandev-503-scoped-shim-red.log` and
`/tmp/kandev-503-terminal-reader-red.log`. Final local results are in
`/tmp/kandev-503-fixture-final-unit.log` and
`/tmp/kandev-run.e2e.OIAq8vuJ.log`. Fresh remote checks remain pending.
