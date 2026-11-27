---
created: 2026-10-09
status: done
requirements:
  - REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007
system_design:
  - ../../specs/platform/system-design/runtime-failure-attribution.md
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
legacy_specs: []
---

# Database writer contention investigation

## Overview

Identify the workload occupying SQLite's writer before selecting a production repair.
Platform owns shared database capacity and health. Domain systems retain their transaction and delivery contracts.

This package reuses the existing requirements and designs. It adds no product behavior or persistence policy.
The single work order produces a repeatable experiment, measured evidence, and a specific repair proposal.
It does not claim that diagnostic work alone improves application throughput.

## Evidence

On October 9, 2026, persistence became unhealthy at 11:26:30.293 Europe/Lisbon and recovered at 11:26:58.289.
The retained backend log contains 280 HTTP 503 responses during that interval, including 28 sidebar queries.
Two `writer_ping` failures reported a two-second deadline and one writer connection in use.
Between those observations, writer waits increased by 42 and aggregate wait duration increased by approximately 56.4 seconds.
That duration is summed across waiters. It is not the duration of one transaction.

The running build was `1769b9abafea5c45c0798409e33dbaa6236d0f3b`.
Evidence came from `/root/.kandev/logs/backend-logs.log` and frontend bundle `d3923fbfbee677f1271b3405edd2f120`.
The bundle contains bounded browser history and reports earlier backend file-sink losses.
Absence of an error from these sources does not prove that it never occurred.
The exact writer operation remains unknown. The retained log can rotate.

The source has one SQLite writer connection and four separate readers.
`beginSidebarQuerySnapshot` uses the reader pool. A slow sidebar response does not establish writer ownership.
`Health.pingObserved` checks the writer first. A failed probe closes stateful HTTP admission and WebSocket upgrades.
Agentctl filesystem failures continued after database recovery and remain outside this investigation.

## Scope

### In scope

- An inventory of writer access and possible long transaction spans.
- Disposable, file-backed WAL experiments using real repository operations.
- Separate measurements for application operation time, transaction entry, transaction occupancy, and aggregate pool wait.
- Reproduction controls for writer occupancy, reader saturation, and recovery.
- A ranked repair proposal supported by measured operation ownership.

### Out of scope

- Load against the live installation or copying its messages into fixtures.
- Database engine migration, extra writer connections, and health-policy changes.
- A global retry layer, new transaction framework, or permanent driver replacement.
- Workspace filesystem remediation and rendered UI changes.

## Technical approach

The existing [contention evidence](../runtime-log-reliability/persistence-evidence.md)
proved connection-checkout failure with deliberately held transactions.
Its [completed work order](../runtime-log-reliability/task-01-persistence-contention.md)
did not attribute a production workload. Preserve that result and extend its next experiment.

Start with `ReceiveAgentDeliveryEvent`, `ProjectCanonicalAgentDeliveryEvents`, and `SettleAgentDeliveryTerminal`.
They form concrete write paths for active agent output. Their inclusion is a hypothesis, not incident attribution.
Projection already batches compatible events. Measure actual batch sizes and transaction costs before recommending more batching.
Inventory other writer paths, including status updates, background services, and maintenance, before claiming complete coverage.

Use real `OpenSQLite` and `OpenSQLiteReader` factories with separate pools.
Measure idle control, each candidate alone, and their mixed workload alongside sidebar snapshots.
Use synthetic conversations at multiple payload sizes and one, four, and eight active sessions.
Record event rate, batch sizes, dataset size, database size, CPU, filesystem, build, and warm-up conditions.
The matrix is an experiment, not a promised production capacity target.

Temporary instrumentation records fixed operation names and numeric timings.
Capture transaction entry and settlement inside the repository boundary in a disposable source copy.
Call duration around `BeginTxx` includes pool checkout and native transaction entry.
Do not label that duration as pure SQLite lock time or pure pool wait.
`DBStats` deltas are aggregate measurements and cannot assign another caller's wait to one operation.
Only successful admission followed through actual commit or rollback establishes an observed transaction span.
Record cancelled entry, automatic rollback, and settlement uncertainty separately.
Do not log SQL parameters, message content, database paths, or connection strings.

Run throughput measurements without the race detector. Use race tests for lifecycle correctness.
Compare instrumentation enabled and disabled to report its measurement overhead.
Retain only deterministic regression controls as permanent tests. Remove temporary production instrumentation.

## Repair selection

| Measured finding | Candidate repair | Required preservation |
| --- | --- | --- |
| Long transaction containing avoidable preparation | Move independent preparation before admission | Revalidate authoritative predicates within the transaction |
| Repeated equivalent derived-state writes | Coalesce that projection or skip unchanged writes | Preserve canonical events, revisions, and terminal ordering |
| Oversized projection batches | Bound batch work and release between batches | Preserve contiguous cursor advancement and replay idempotency |
| Background producer dominates writer occupancy | Add bounded admission or smaller chunks at that producer | Preserve durable work and cancellation ownership |
| Read-only snapshots occupy writer | Use the existing reader where authority permits | Keep read-modify-write decisions and admission checks on writer |
| Sustained useful write demand exceeds capacity | Benchmark the supported PostgreSQL path separately | Plan data migration and operational ownership explicitly |

Health changes need a separate design after workload attribution.
A busy writer and damaged storage can require different operational responses, but current readiness policy treats failed probes as unavailable.
Any proposed overload distinction must bound staleness and preserve rejection for missing schema, closed pools, and real storage failures.
Increasing timeouts or adding a health-only connection does not establish that application writes can make progress.

## Tests

`TestPersistenceContentionFixture` remains the mechanism control.
The work order adds a representative repository fixture and records operation spans alongside probe outcomes.
It must verify durable event order, idempotency, settlement, and reader progress with the existing repository assertions.
Performance results are evidence, not machine-dependent CI timing assertions.

The relevant criteria are `AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.1`
and `AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.1`, `.3`, `.4`, `.6`, `.7`, and `.8`.
No browser behavior changes in this package, so it adds no browser test.

## Work orders

- [x] [Task 01: Attribute writer occupancy](task-01-attribute-writer-occupancy.md)

## Verification results

Design validation passed on October 9, 2026:

- `python3 scripts/list-docs.py validate`: 368 decisions and 1,493 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/plans/database-writer-contention`: passed.
- PR-documentation preflight: actual documentation diff exempt, linked work-order references passed with a synthetic runtime trigger.

Work-order execution is complete. See [evidence.md](evidence.md) and [measurements.json](measurements.json).
The fixture reproduces health deadline failures with large-message updates while readers continue.
A disposable reservation-write removal improved elapsed time in all three alternating pairs, with substantial variability.
It did not eliminate failures. No production repair was applied, and historical writer ownership remains unknown.

## Risks

- A synthetic workload can identify a reproducible bottleneck without proving it caused the retained incident.
- Incomplete instrumentation can leave other writer owners unknown.
- The running build and checkout can differ. Compare relevant source before attributing historical behavior.
- Throughput improvements must preserve transactional authority and durable event ordering.

## Handoff

The completed work order supports a narrow message-update optimization package.
Preserve error mapping, immediate admission, retained-payload safeguards, and PostgreSQL locking.
If the producer remains unknown, report the coverage gap and next bounded observation.
Do not mark the contention problem fixed or select a guessed query for optimization.

## Follow-up optimization

The [message update package](../message-update-writer-occupancy/plan.md) turns the measured redundant reservation write into one scoped implementation work order.
This investigation remains complete; its evidence and unresolved incident attribution are unchanged.
