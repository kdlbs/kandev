# Persistence contention evidence

## Scope and revision

This investigation uses the repository revision `f18fe2d9e3bafb567aff643f25356ac3ed651120` and disposable in-memory SQLite pools. It does not change the health timeout, readiness policy, pool configuration, middleware, or live installation.

## Retained instance evidence

The retained record at `backend-logs-2026-10-05-000004.log:49108` reports a `writer_ping` timeout after 2,000.57 ms. The snapshot had one writer connection in use. It also reported cumulative pool counters of 4,662 writer waits / 246,043 ms and 27,352 reader waits / 295,554 ms. These counters are process-lifetime totals, not deltas for the failed probe. The record contains no query, transaction, or caller identity, so it establishes pressure but cannot identify the producer.

## Controlled pool checkout reproduction

`TestPersistenceContentionFixture` opens independent one-connection writer and reader pools, initializes one required table, and holds a transaction on one selected pool. `Health.Check` uses a 100 ms context. The test records `sql.DB.Stats()` deltas, verifies the observed probe stage, releases the transaction, and requires health to recover.

| Held operation | Probe stage | Wait count delta | Wait duration | Probe elapsed | Recovery |
| --- | --- | ---: | ---: | ---: | --- |
| Transaction holding the sole writer connection | `writer_ping` | 1 | 100.31 ms | 100.40 ms | Successful after rollback |
| Transaction holding the sole reader connection | `reader_ping` | 1 | 100.18 ms | 100.21 ms | Successful after rollback |

This reproduces how a pool checkout wait becomes a failed health probe and how the existing health state recovers. It does not reproduce or identify a production query. The fixture's held transaction is intentionally named; no such operation identity is present in the retained log.

## Existing controls

The focused regression set covers reader and writer probe attribution, maintenance-time deferral and state preservation, strict startup checks, recovery after maintenance, missing-table failure and recovery, context-bounded table probes, and persistence middleware rejection and recovery. These controls preserve the existing fail-closed behavior.

The retained failure occurred at `writer_ping`, before the table catalog sweep. The current evidence therefore does not attribute the outage to table-probe cost. No successful-sweep timing was recorded by the production log, so table-sweep cost remains unmeasured against this instance.

## Conclusion and next experiment

**Cause: unresolved.** The observed `writer_ping` timeout and cumulative pool waits are consistent with connection checkout contention, but neither identifies the writer transaction or operation responsible. No workload repair is proposed.

The next bounded experiment is to capture per-operation writer transaction start/finish timing and caller identity in a disposable reproduction of the suspected workload, alongside per-probe pool-stat deltas. Measure a successful catalog sweep separately at the instance's descriptor/table count. Do not change timeout, pools, health policy, or middleware unless that evidence identifies a specific producer and a repair regression protects it.

## Verification

The focused commands and their results are recorded in `task-01-persistence-contention.md` after they complete.
