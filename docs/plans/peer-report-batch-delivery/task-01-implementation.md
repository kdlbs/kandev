---
id: "01-implementation"
title: "Bounded peer-report batch delivery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PEER-REPORT-BATCH-001
acceptance_criteria:
  - AC-TASKS-PEER-REPORT-BATCH-001.1
  - AC-TASKS-PEER-REPORT-BATCH-001.2
  - AC-TASKS-PEER-REPORT-BATCH-001.3
  - AC-TASKS-PEER-REPORT-BATCH-001.4
  - AC-TASKS-PEER-REPORT-BATCH-001.5
  - AC-TASKS-PEER-REPORT-BATCH-001.6
  - AC-TASKS-PEER-REPORT-BATCH-001.7
  - AC-TASKS-PEER-REPORT-BATCH-001.8
system_design:
  - ../../specs/tasks/system-design/peer-report-batch-delivery.md
---

# Bounded peer-report batch delivery

## Outcome and scope

Reserve and compose bounded consecutive ordinary peer reports through both existing queue drains while atomically folding their durable claims.

Out of scope: New admission limits, periodic drains, child stall producers, task message operation readback and shared inference admission.

## Implementation acceptance

1. The affected native entrypoint follows the linked requirement and design while preserving unrelated behavior and history.
2. Rejected, stale and failed operations retain their existing refusal or rollback semantics.
3. The focused implementation checks and linked documentation coverage have honest recorded results; unexecuted follow-up checks remain pending.

## Files and dependencies

The source files are the current main-based candidate diff. This work order adds its delivery record and links existing requirements and system design; it introduces no runtime gate or permanent test.

- [System design](../../specs/tasks/system-design/peer-report-batch-delivery.md)
- [Plan](plan.md)

## Verification and results

Source and dependency inspection is complete. Disposable fixtures are prepared outside normal collection for FIFO, sender attribution, limits, atomic fold rollback, both actual drain entrypoints and unknown-outcome recovery. Candidate Go verification has not run yet. The current source pin is 2df317716f576b587afe3a29e7905ae8a8dd4ef7; original topic 6b5b514ba and final fork 28aacb239 are provenance, not whole-branch imports.

Documentation-only correction: git diff --check and the existing .github/scripts/pr-docs.cjs validateCoverage entrypoint with the candidate changed paths and the linked candidate documents supplied on stdin. No GitHub status is synthesized locally.

The execution results below establish this candidate; normal commit hooks must pass before publication.


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

## Review follow-up: cancellation and replay boundaries

Partial reservations must return to the original FIFO queue even when the
request is cancelled. Compensation keeps the captured service generation and
uses the existing bounded recovery window; it must not transfer old claims to a
restarted service owner. A composed batch is a separate delivery boundary when
it returns to the queue, so a context-reset retry cannot nest report labels or
exceed the existing report-count limit.

The disposable baseline reproduced three failing cases: a cancelled partial
SQLite reservation hid its first report (orchestrator 0.506s), and memory/SQLite
retries reserved an eight-report combined prompt plus seven new reports
(messagequeue 0.179s). The stopped-generation control passed before the fix.

The candidate passed those cases with race detection, together with existing
SQLite compensation/shutdown-admission and FIFO controls (orchestrator 2.083s,
messagequeue 1.074s). The exact task/session incarnation, original queue order,
message IDs and pending-claim readback were retained. Temporary cases stay outside automatic collection and are
removed after delivery; no standing test, schema, setting, new timeout limit or
operating mutation is introduced.

The first normal commit attempt hit the existing cyclomatic limit (16, maximum
15). The composed-boundary condition moved into the existing metadata
separability helper without changing its predicate. Only the affected
memory/SQLite retry-boundary selection was rerun with race detection and passed
(1.179s); the earlier unchanged compensation/FIFO results remain valid. The
complexity criterion was not weakened.
