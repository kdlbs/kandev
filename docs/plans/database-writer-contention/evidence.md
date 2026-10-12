# Database writer workload evidence

Measured on October 9, 2026. This completes the investigation work order, not a production repair.

## Findings

Concurrent updates to large messages can exhaust the required-persistence probe budget while sidebar reads continue.
The experiment reproduced this with real repository operations on disposable SQLite storage.
It did not identify the writer responsible for the historical outage.

Eight workers made 128 total updates to eight 16 MiB messages per run.
Across three runs, nine probes exceeded their two-second deadline; eleven succeeded.
All 318 concurrent sidebar reads succeeded, with a maximum duration of 23.89 ms.
The measured health call includes writer and table probes. Its failed stage was not captured.
The live incident specifically failed `writer_ping`; these observations must not be conflated.
Existing middleware tests cover how unhealthy persistence becomes HTTP 503. This benchmark did not send HTTP requests.

A disposable source change removed the initial `UPDATE ... SET id = id` from the message update transaction.
Median completion time fell from 10.77 to 7.69 seconds, about 29% in these runs.
Three probes still failed, and thirteen succeeded. The change reduced work but did not eliminate probe starvation.
Shorter runs also offer fewer probe opportunities, so raw failure counts are not a controlled failure-rate improvement.

## Incident and version boundary

The retained live log showed 280 HTTP 503 responses between 11:26:30.293 and 11:26:58.289 Europe/Lisbon.
These included 28 sidebar queries and eight WebSocket upgrades.
Two writer probes timed out with the sole writer connection in use and all four reader connections idle.
The writer pool accumulated 42 additional waits and approximately 56.4 aggregate wait-seconds between probes.
This is time summed across callers, not a single transaction duration.

The running build was `1769b9abafea5c45c0798409e33dbaa6236d0f3b`.
The measured checkout was `b8b740f128993b804745704591374f5a9fc7b0ab`, plus the diagnostic files in this package.
Durable-delivery reception, projection, and settlement were added after the running build.
Their measurements describe current-checkout capacity and cannot explain the earlier incident.

`message.go`, `message_payload_replay.go`, `conversation_source.go`, `conversation_journal_cleanup.go`,
and `internal/db/sqlite.go` have no diff between those commits.
The shared message-update code is therefore a relevant candidate, but synthetic data does not establish historical ownership.
Agentctl filesystem and git-status errors remained after persistence recovered. Their causes remain unresolved.

## Method and limits

- Go 1.26.0, Linux amd64, AMD Ryzen 5 7640HS; benchmark execution reported 11 available CPUs.
- Temporary databases used ext4 on `/dev/mapper/pve-vm--101--disk--1`, with about 50 GiB free.
  This is a different mount from the live database. The host was shared.
- Every fixture uses `testing.TB.TempDir` and the real SQLite writer and reader factories.
  The writer has one connection; the separate reader pool has four. WAL and immediate writer admission remain enabled.
- Each case starts with a fresh schema and synthetic tasks, sessions, and turns. Setup is excluded from elapsed workload time.
  The delivery matrix primes a sidebar read before timing; that read remains in the recorded sidebar samples.
- Sidebar reads run every 100 ms. Health calls run every second, each with a two-second budget.
  This is accelerated from the live 15-second probe cadence. Calls within each observer are sequential; ticks can be dropped.
- The health tracker covers three tables: delivery inbox, delivery cursors, and messages.
  It is not the complete production required-store catalog.
- Fixed operation counts keep dataset growth comparable. Go's one-iteration calibration records are excluded from results.
  An earlier time-calibrated experiment was stopped and discarded because message growth changed the cost per iteration.
- Performance runs were sequential and excluded the race detector. Correctness checks ran separately with the race detector.
- Operation-call time includes preparation and post-commit work. Transaction entry combines pool checkout and native BEGIN.
  A transaction span starts after successful admission and ends after the observed commit or rollback returns.
  Pool wait deltas are aggregate and must not be assigned to a specific caller.
- The overlay observes receive, canonical projection, terminal settlement, and message update transactions.
  Terminal preparation and legacy projection remain outside span coverage. Their time is included in terminal lifecycle calls.
- Worker completion defines elapsed time. Observer shutdown joins any in-flight probe afterward; counters include that final probe.
- Percentiles use sorted samples at `floor((n-1)*p)`. Reported table percentiles are medians of three per-run percentiles.
  They are not pooled percentiles or confidence intervals. Retained cases stay below the 65,536-sample reservoir capacity.
- No production source, live data, pool limits, timeouts, or runtime configuration changed.

Raw structured results are in [measurements.json](measurements.json).
They contain operation counts, p50/p95/p99, maximum and summed durations, pool waits, and synthetic dataset sizes.
No production message content or SQL parameters are included.

## Isolated operation controls

Each case runs 32 rounds with one session, 16 KiB chunks, and batch size 16.
Setup finishes before measurement. Every case passed 32 subsequent health probes with zero writer-pool waits.
These sequential probes establish recovery, not health under concurrent load.

| Operation | Observed transaction count | Span p50, ms | Span p95, ms | Span p99, ms |
| --- | --- | --- | --- | --- |
| receive | 512 | 0.085 | 0.180 | 2.048 |
| project | 32 | 3.980 | 13.608 | 13.699 |
| settle | 32 | 0.079 | 0.098 | 0.108 |

The idle control has no observed writer transactions. Receive has 512 transactions because reception is per chunk.
Projection and settlement each have 32 observed transactions.

## Current delivery workload

Each round receives the stated number of chunks, projects one batch, then prepares and settles a terminal event.
There are 512 total rounds per run, distributed over the selected sessions, and three runs per case.
The normal case creates a new message each round. The growing case appends to one message per worker.

| Sessions | Chunk bytes | Batch | Growing | Median seconds | Projection span p99, ms | Failed / completed probes |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | 128 | 1 | No | 1.124 | 14.02 | 0 / 3 |
| 1 | 16,384 | 16 | No | 9.154 | 26.36 | 0 / 24 |
| 4 | 128 | 1 | No | 1.679 | 34.31 | 0 / 3 |
| 4 | 16,384 | 16 | No | 6.580 | 21.21 | 0 / 20 |
| 8 | 128 | 1 | No | 1.073 | 13.97 | 0 / 3 |
| 8 | 16,384 | 16 | No | 10.738 | 57.69 | 0 / 29 |
| 8 | 16,384 | 16 | Yes | 47.679 | 241.18 | 0 / 137 |

The growing case appends 128 MiB total, approximately 16 MiB per message with uneven worker distribution.
Its median throughput was about 172 chunks/second, versus 763 with a new message each round.
Projection reads existing content and constructs the replacement inside the transaction.
This makes accumulated message size a concrete capacity concern even though all 219 observed probes succeeded.

## Shared message update path

These cases rewrite fixed-size messages with eight workers and 128 total updates per run.
The workload offers writes as fast as possible; it is not a measured user traffic profile.
Cases below one second had no scheduled health samples. That absence is not proof of health under sustained load.

| Message bytes | Original median seconds | Original span p99, ms | Without reservation median seconds | Without reservation span p99, ms |
| --- | --- | --- | --- | --- |
| 16,384 | 0.039 | 0.50 | 0.039 | 0.47 |
| 1,048,576 | 0.588 | 16.27 | 0.561 | 22.60 |
| 16,777,216 | 10.766 | 146.02 | 7.695 | 120.91 |

For the 16 MiB original case, median transaction-entry p99 was 1,804.94 ms.
The median admitted-span p99 was 146.02 ms. Queueing can therefore exhaust a probe without one transaction lasting two seconds.
No admitted transaction was left without an observed settlement in the retained instrumented results.

## Instrumentation comparison

Both groups retain outer call timing. Only the transaction-entry and settlement observer differs.
Each cell is the median elapsed seconds from three fixed-count runs.

| Sessions | Chunk bytes | Growing | Without transaction observer, s | With transaction observer, s | Difference |
| --- | --- | --- | --- | --- | --- |
| 1 | 128 | No | 1.276 | 1.124 | -11.9% |
| 1 | 16,384 | No | 5.641 | 9.154 | +62.3% |
| 4 | 128 | No | 0.694 | 1.679 | +142.1% |
| 4 | 16,384 | No | 13.561 | 6.580 | -51.5% |
| 8 | 128 | No | 0.820 | 1.073 | +30.8% |
| 8 | 16,384 | No | 10.516 | 10.738 | +2.1% |
| 8 | 16,384 | Yes | 38.783 | 47.679 | +22.9% |

The differences change sign and exceed plausible stable observer overhead in several cases.
These sequential runs cannot separate instrumentation cost from shared-host and storage variation.
Do not quote a single overhead percentage or treat cross-case throughput as a concurrency scaling curve.
The delivery control had 188 successful probes and two deadline failures.
Both failures occurred in one four-session, 16 KiB, batch-16 run that took 14.002 seconds.
The observed group had no failures. Probe outcomes are therefore intermittent, not a stable capacity boundary.
The large-message ablation also received alternating original/experimental runs.

### Alternating large-message comparison

Three additional pairs ran original then experimental code, with 128 updates per run and no concurrent test workloads from this investigation.
Each pair used fresh databases. These were still on the shared host.

| Pair | Original seconds | Without reservation seconds | Elapsed reduction | Original / experimental failed probes |
| --- | --- | --- | --- | --- |
| 1 | 9.064 | 6.436 | 29.0% | 2 / 1 |
| 2 | 11.114 | 5.624 | 49.4% | 3 / 0 |
| 3 | 13.486 | 12.241 | 9.2% | 4 / 3 |

All three experimental runs completed sooner, but the size of the difference varied widely.
This supports testing the narrow optimization; it is not a guaranteed production gain or a resolved availability issue.
The experimental group still had four failed probes across these pairs.

## Writer inventory and repair recommendation

| Candidate | Evidence | Consequence |
| --- | --- | --- |
| Message update | Large payloads reproduce health failure; removing one extra UPDATE reduces elapsed time | First narrow optimization candidate |
| Canonical growing-message projection | Longer admitted spans and lower throughput with accumulated content | Measure production size distribution before redesigning storage or batch policy |
| Receive and terminal settlement | Covered by isolated and mixed operation spans | No evidence they alone cause the observed large-message slowdown |
| Sidebar snapshots | Use separate readers and continue under writer pressure | Moving these reads again would not remove writer occupancy |
| Plugin artifact cleanup | `RemoveArtifactIfUnreferenced` holds a transaction across its removal callback | Possible long occupancy, but its atomic ownership contract must be preserved; not tied to incident |
| Status, Office, automation, maintenance, and other writers | Additional transaction and direct-write sites exist outside the fixture | Historical attribution remains incomplete |

The immediate production candidate is to remove redundant reservation work from `updateMessageWithPayloadGuardTx`.
The production SQLite factory already uses `BEGIN IMMEDIATE`.
A repair must keep the metadata read and authoritative update in that transaction.
It must preserve the PostgreSQL row lock and all retained-payload protections.

The experimental removal is deliberately incomplete as a production patch:
SQLite currently obtains its `message not found` error from the reservation UPDATE.
Removing it exposes `sql.ErrNoRows` from the subsequent SELECT unless error mapping is preserved.
The SQLite conversation revision trigger is conditional; do not claim every no-op UPDATE increments its revision.

Before shipping that optimization, require tests for missing-message errors, retained metadata after stale reads,
externalized payload identity, transaction admission, rollback, and conversation revision behavior.
Repeat the fixed-size benchmark and confirm large-message occupancy improves without changing message results.
Do not use fewer probe failures as the sole success criterion.

The remaining incident-attribution step is bounded production observation of writer owners and message-size buckets.
Record fixed operation labels, entry and admitted-span durations, outcome, and probe stage on the same time axis.
Keep task IDs, SQL, paths, and message content out of metric labels.
That instrumentation needs its own reviewed scope before changing the live system.
Health-policy changes or an engine migration are not justified by this experiment alone.

## Reproduction and verification

Use the commands in [Task 01](task-01-attribute-writer-occupancy.md#verification).
`measure.py` creates a private Go build overlay and removes it after the test command.
The implementation comparison runs the committed fixed path by default and uses
`--restore-message-reservation` only for the historical path in that disposable
source copy. The 70 original experiment records and their results remain in
`measurements.json`; the production fix's separate measurements are in the
[message update implementation evidence](../message-update-writer-occupancy/implementation-evidence.md).
Normal builds use the original repository code.

To reproduce the alternating comparison, run three pairs of the two message benchmark commands.
For each command, replace its benchmark filter with `^BenchmarkWriterMessageUpdate$/^bytes=16777216$`
and use `-count=1`. Run the original before the ablation in each pair.

Verification passed:

- Normal and overlaid workload correctness and isolated controls.
- Race-enabled delivery, sidebar, message, payload, and conversation-source tests.
- Race-enabled required-persistence, HTTP admission, and SQLite writer-admission tests.
- Race-enabled payload and conversation-source tests with the experimental reservation removal.
- Both fixed-count delivery matrices, both message matrices, and the three alternating pairs.
- Specification catalog validation, specification lint, Go formatting, Python syntax, and whitespace checks.

These are targeted checks, not a full repository audit. PostgreSQL integration tests were not exercised.
Existing tests do not make the incomplete experimental patch safe to ship; the missing-message behavior still needs preservation.
No production files changed. Completed fixtures and overlays were removed automatically.
The aborted time-calibrated fixture directory was removed explicitly.
