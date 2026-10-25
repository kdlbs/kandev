# Isolated persistence probe experiment

Date: October 19, 2026. All load used disposable, synthetic storage.
The user's live instance on port 9998 received no experimental traffic or changes.

## Result

Eight concurrent clients updating separate 16 KiB task descriptions reproduced
`table_probe` deadline failures on the current backend. A temporary batched
schema lookup removed those failures in all three paired runs.

| Run | Successful writes | Write 503s | Read 503s / samples | Failed probes | Loaded probe duration, ms |
| --- | ---: | ---: | ---: | ---: | --- |
| Baseline 1 | 817 | 1,023 | 65 / 175 | 1 | 2,001.23 timeout; 4.56 recovery |
| Batched 1 | 1,118 | 0 | 0 / 175 | 0 | 274.10; 223.69 |
| Baseline 2 | 626 | 1,021 | 65 / 175 | 1 | 2,000.59 timeout; 5.80 recovery |
| Batched 2 | 965 | 0 | 0 / 176 | 0 | 93.29; 119.64 |
| Baseline 3 | 614 | 1,020 | 65 / 175 | 1 | 2,000.62 timeout; 7.17 recovery |
| Batched 3 | 935 | 0 | 0 / 175 | 0 | 184.97; 315.47 |

Read samples in this table refer to `GET /api/v1/workspaces`. `/ready` returned
matching failure counts. Persistence diagnostics remained reachable throughout.
The original build produced 3,064 write 503s and 195 read 503s in these runs.
The candidate produced none. Successful writes totaled 2,057 and 3,018 respectively.

The candidate improves availability under this workload. It does not establish
uniformly faster individual writes. Successful-write p95 latency ranged from
493 to 638 ms on the baseline and 537 to 648 ms on the candidate. Rejected
requests return quickly, so combining their latency with successful writes
would create a misleading comparison.

## Scope and versions

- Checkout: `595df36ed5c1cf02fd90cb972b623ad6edc53df8`.
- Incident binary: `76954ff53ea20b0263dd15c01c7149a531668341`.
- The incident binary and checkout use the same relevant health-probe mechanism.
- The full backend ran with the dev profile, mock providers, separate SQLite
  writer/readers, and its normal 15-second probe interval and two-second deadline.
- No agent session, real provider request workload, or browser was required.
- Original isolated root: `/tmp/kandev-iso-48429-SjNvMa`, API port 48429.
- Binaries and Go overlays lived under `/tmp/kandev-probe-experiment-7uilwp4k`.
- [measurements.json](measurements.json) records source hashes, host information,
  each sample, individual probe outcomes, and successful-write distributions.

The task used `scripts/dev-isolated --timeout 120` to build and launch a fresh
backend. It seeded 16 synthetic tasks through HTTP with `start_agent=false`
and `prepare_session=false`. Every measured writer updated a separate task.

The idle control ran for 16 seconds with 80 successful samples per read route.
One writer ran for 35 seconds and completed 1,467 updates without a 503.
An initial uninstrumented eight-writer run reproduced the failure with 742
successful updates and 1,029 write rejections. Its 175 read samples included
65 rejections. This reproduced the mechanism before adding diagnostic logs.

## Method

Each paired run used a fresh copy of the same 6.1 MiB synthetic SQLite seed.
Both paired binaries included the same temporary observation of total probe
duration and pool-statistic deltas. Neither changed the task-update path.
There was no task-owned compilation or second stress workload during measurement.
Other workloads can run on this shared host.

Each run issued eight concurrent, sequential-per-client HTTP PATCH loops for
35 seconds. Each description contained exactly 16,384 ASCII bytes, with a
changing numeric prefix. The clients paused for 100 ms after a rejected request
to bound retry pressure. An independent observer sampled readiness, workspaces,
and persistence diagnostics approximately every 200 ms.

The execution order was baseline/batched, batched/baseline, baseline/batched.
The original code performs 97 required-table checks for 96 distinct names.
Every point query acquires the shared writer connection again. Under the known
synthetic write workload, the combined wait exceeds the sweep's two-second
budget. Late stores inherit the expired context and global admission closes.

The prototype keeps the writer and reader pings. It deduplicates the required
names and performs one filtered `sqlite_master` query through the same writer.
It compares each descriptor against the result set. No timeout, pool size,
maintenance admission, or HTTP health rule changes.

Pool wait counts and durations are aggregate observations across callers.
They are not the isolated wait time of the health probe. The controlled source
comparison, actual task-write workload, and probe outcomes support the batching
result without assigning aggregate wait duration to one transaction.

## Fault controls

The experimental build passed the existing focused health controls, including:

- A held writer or reader connection still causes a deadline and unhealthy state.
- Releasing the held connection allows a complete health check to recover.
- Maintenance deferral preserves state, including mixed healthy and unhealthy stores.
- Startup checks remain strict during maintenance.
- Missing required tables fail, and restored tables recover.

The controls also passed with the race detector alongside backend persistence
middleware tests. The selected packages completed successfully:
`internal/persistence/requiredstores` and `internal/backendapp`.

A separate full-instance fault control removed the empty `plugin_user_state`
table from the disposable database. The next batch marked only
`plugin-user-state` unhealthy. `/ready` and `/api/v1/workspaces` returned 503,
while `/health` remained 200. Recreating the saved schema restored all three
responses to 200 without restarting the backend. The table's schema was
restored even if the diagnostic assertion failed.

Exact focused commands from `apps/backend`:

```bash
go test -trimpath -tags fts5 -overlay /tmp/kandev-probe-experiment-7uilwp4k/batched.json ./internal/persistence/requiredstores -run 'Test(PersistenceContentionFixture|RuntimeHealth|StartupHealth|HealthCheck|ProbeTables)' -count=1 -v
go test -trimpath -tags fts5 -race -overlay /tmp/kandev-probe-experiment-7uilwp4k/batched.json ./internal/persistence/requiredstores ./internal/backendapp -run 'Test(PersistenceContentionFixture|RuntimeHealth|StartupHealth|HealthCheck|ProbeTables|RequiredPersistence|PersistenceMiddleware)' -count=1
```

## Replay and retained artifacts

The task workspace retains a replay harness and synthetic seed under
`.kandev/diagnostics/stress-503/`. These ignored artifacts contain experimental
sources, not production edits or permanent regression tests.

From the repository root:

```bash
python3 .kandev/diagnostics/stress-503/replay.py --rounds 3 --seconds 35
```

The harness requires the measured checkout revision and existing built helpers.
It creates its own temporary home, database, processes, and available ports.
It accepts no live URL or database target. The load helper validates the test
process's database environment before issuing mutations.

`--candidate current` compares future production edits at the same checkout
against the captured original source. That option intentionally omits the
prototype's success instrumentation. Failure logs, HTTP outcomes, and sampled
health timestamps remain available.

Preserved local artifacts include `pair*-retained.log`, `pair*.json`,
`original-backend-logs.log`, `baseline-idle.json`, `baseline-1x16k.json`,
`baseline-8x16k.json`, `fault-control.json`, `fault-tests.log`, and
`fault-race-tests.log`. The source snapshots and synthetic seed reside in
`replay-sources/`. Do not commit the generated databases or binary artifacts.
The replay wrapper receives a separate execution check recorded below.

## Limits and next work

The historical incident's writer operation remains unknown. This experiment
proves that ordinary task writes can trigger the same mechanism, not that they
caused the October 19 incident. Heavy retention activity remains a separate
possible contributor. No retention optimization was tested or selected.

Batching removes repeated pool admission. It cannot prevent timeouts when one
writer or reader checkout exceeds the budget. It preserves failure for blocked
connections and missing schema. It is not a guarantee of unlimited capacity.

The prototype needs production integration in the shared schema layer, query-count
regression coverage, and error-path tests before delivery. The
[work order](task-01-batch-schema-probes.md) remains pending. No deployment,
commit, or PR was made.

## Validation and cleanup

- Required-store fault tests: passed.
- Required-store and middleware fault tests with `-race`: passed.
- Documentation catalog: 384 decisions and 1,616 specifications validated.
- Specification lint: passed.
- Specification-linter tests: 36 passed.
- Documentation coverage preflight: actual diff exempt; planned runtime path covered.
- Replay wrapper: one complete 35-second pair passed. The baseline returned 1,028
  write 503s and 65 workspace-read 503s. The candidate completed 1,237 writes with
  no 503s. Evidence is in `replay-20261019T143533/` under the artifact directory.
- Whitespace checks: passed.
- All owned test backends and agentctl processes stopped. The original isolated
  home, temporary builds, and initial helper scripts were removed. The replay
  wrapper also removed its own temporary home and binaries.
- Source overlays, the synthetic seed, raw logs, and replay scripts remain only
  in the ignored diagnostic artifact directory. No production source or permanent
  test file changed.
