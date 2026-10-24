---
status: done
created: 2026-10-23
requirements:
  - REQ-TASKS-PEER-REPORT-BATCH-001
system_design:
  - ../../specs/tasks/system-design/peer-report-batch-delivery.md
---

# Bounded peer-report batch delivery

Reserve and compose bounded consecutive ordinary peer reports through both existing queue drains while atomically folding their durable claims.

## Delivery order and dependencies

The implementation is one existing PR slice. Preserve its native dependencies and existing feature defaults, then verify the narrow affected paths. No dependent PR is imported into this package.

- [Implementation and verification](task-01-implementation.md)

## Scope and risks

New admission limits, periodic drains, child stall producers, task message operation readback and shared inference admission. These remain outside this delivery slice. Preserve existing files, configuration and history; report unavailable integration coverage without inventing a successful result.

## Verification strategy

Reuse the completed focused implementation checks recorded below and the existing documentation-coverage evaluator. Only remaining review fixes require their affected checks to run again.


## Observed execution results

The explicit temporary overlay passed the memory/SQLite FIFO, user boundary,
entry and byte bounds, Auto-run OFF, sender attribution, persistent fallback,
composed leading claim, both actual drain entrypoints, predecessor settlement,
and committed-unknown/startup-parking checks. Messagequeue package: 0.065s.
The first rollback probe failed before product entry because dispatch-claim
schema is created lazily. Calling the existing pending-claim reader before its
failure-injection trigger fixed the fixture; the actual rollback/FIFO requeue
case then passed (orchestrator 0.278s). No product change or assertion reduction
was made for that setup failure.

Focused existing dispatch, requeue, claim identity and predecessor settlement
checks passed (messagequeue 0.156s; orchestrator 0.984s). Existing SQL guard passed.
The existing message-queue store conformance slice passed with -race on SQLite
(fresh/replay/crud/boolean/timestamp/conflict/transaction; package 2.254s).
No PostgreSQL execution is present in this result; no live database or provider
interaction was performed.

Commands use the excluded temporary runner's explicit overlay, existing,
sqlguard and conformance modes. The conformance selection is
^TestStoreConformance$/(sqlite|postgres)/message-queue$, with -race, fts5,
-trimpath, -p 2, -count=1 and the unchanged 90-second runtime limit.
Temporary files are removed after preserving these results. No standing tests,
CI changes, new protocol/hash checks or operating settings were added.
