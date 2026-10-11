---
status: active
system: platform
created: 2026-10-05
owners:
  - kandev
---

# Backend agent delivery retention requirements

## Overview

Platform owns authenticated durable delivery, including its recovery evidence.
This capability removes redundant backend transport data while preserving canonical conversation and safe recovery.
It extends [durable delivery](durable-agent-delivery.md). Agentctl journal retention remains a separate contract.
Implementation is in progress. The paired [design](../system-design/backend-agent-delivery-retention.md) and [plan](../../../plans/backend-agent-delivery-retention/plan.md) define delivery.

## Terminology

- **Raw payload:** Redundant received event or fully composed outbound prompt content.
- **Replay receipt:** Evidence that one authenticated event identity and content already committed.
- **Retirement checkpoint:** A contiguous projected prefix that rejects expired event replay without applying it.
- **Fully settled:** Projection, outcome, bound recovery notice, queue claims and required workflow effects have finished durably.
- **Protected evidence:** Data still needed to establish ownership, projection, dispatch uncertainty or settlement.

## Requirements

### REQ-PLATFORM-DELIVERY-RETENTION-001: Safe payload retention

**Intent:** Remove redundant transport content without changing canonical history or recovery authority.

#### Acceptance criteria

- **AC-PLATFORM-DELIVERY-RETENTION-001.1:** After the configured grace, eligible projected event payloads shall be removed, subject to cleanup capacity and protected dependencies, while canonical messages, tool history and turns remain unchanged.
- **AC-PLATFORM-DELIVERY-RETENTION-001.2:** Unprojected events and evidence required by active or uncertain submissions, pending effects or unsettled terminals shall remain available until their dependencies finish.
- **AC-PLATFORM-DELIVERY-RETENTION-001.3:** Fully settled completed, failed or cancelled prompt payloads shall be removed after the grace. Immutable submission identity, hash, outcome and claim association shall remain available.

### REQ-PLATFORM-DELIVERY-RETENTION-002: Authenticated replay after cleanup

**Intent:** Cleanup must never turn an old event or submission into new work.

#### Acceptance criteria

- **AC-PLATFORM-DELIVERY-RETENTION-002.1:** Within retained replay evidence, an authenticated identical duplicate shall return its committed result without duplicate messages, workflow effects or prompt/tool dispatch. Changed content or identity shall fail.
- **AC-PLATFORM-DELIVERY-RETENTION-002.2:** Beyond retained replay evidence, old events shall return an explicit expired result without insertion, projection or event-derived ACK advancement. Unknown, retired or stale identities shall fail closed.
- **AC-PLATFORM-DELIVERY-RETENTION-002.3:** After a lost ACK and cleanup, authenticated reconciliation shall recover the exact persisted projected position and retry ACK without recovering deleted payloads or resending prompts.
- **AC-PLATFORM-DELIVERY-RETENTION-002.4:** Repeated completed submission identifiers shall retain hash-conflict and no-resend behavior for the session lifetime. Cleanup shall not reset ownership, incarnation or harness generation.

### REQ-PLATFORM-DELIVERY-RETENTION-003: Crash-safe settlement and concurrency

**Intent:** Retention must preserve recovery across interrupted and concurrent work.

#### Acceptance criteria

- **AC-PLATFORM-DELIVERY-RETENTION-003.1:** At each interruption between terminal projection, outcome settlement, claim settlement and final completion, restart shall finish the original settlement once, even after other eligible data is removed.
- **AC-PLATFORM-DELIVERY-RETENTION-003.2:** Concurrent receive, projection, cleanup, owner replacement and deletion shall preserve contiguous checkpoints and canonical effects. Stale observations shall not authorize deletion or successor mutation.
- **AC-PLATFORM-DELIVERY-RETENTION-003.3:** Cleanup shall never resolve recovery blocks, release queue claims, invent outcomes or authorize provider continuation. Existing continuation admission and independent native-state, runtime, authorization and operator causes shall remain effective.

### REQ-PLATFORM-DELIVERY-RETENTION-004: Explicit deletion and orphan recovery

**Intent:** Delivery data must follow authorized session and task deletion without creating resurrection paths.

#### Acceptance criteria

- **AC-PLATFORM-DELIVERY-RETENTION-004.1:** Authorized session deletion shall atomically retire its delivery identities and schedule removal of all associated delivery records. Completion shall mean every delivery category is purged.
- **AC-PLATFORM-DELIVERY-RETENTION-004.2:** Task, workspace, ephemeral and bulk deletion shall apply the same retirement rule. Archive and ordinary Stop shall preserve recoverable evidence.
- **AC-PLATFORM-DELIVERY-RETENTION-004.3:** Existing orphan delivery records shall be removed in bounded restartable work only after authoritative ownership checks. Delayed old writes shall not recreate them or affect a replacement session.

### REQ-PLATFORM-DELIVERY-RETENTION-005: Bounded cleanup and honest storage bounds

**Intent:** Reduce transport storage without interrupting streaming or hiding retained growth.

#### Acceptance criteria

- **AC-PLATFORM-DELIVERY-RETENTION-005.1:** Cleanup shall bound rows, bytes, transaction duration and work per pass. It shall yield to database maintenance, cancel on shutdown and resume after restart or transient errors.
- **AC-PLATFORM-DELIVERY-RETENTION-005.2:** Without protected gaps and with sufficient cleanup capacity, retained replay evidence shall fit the configured recent window. Long streams shall not retain one permanent receipt per streaming event.
- **AC-PLATFORM-DELIVERY-RETENTION-005.3:** Reported bounds shall distinguish recent payloads, pinned backlog, compact prompt/terminal/workflow evidence and canonical history. Cleanup shall not claim that total database growth stops.
- **AC-PLATFORM-DELIVERY-RETENTION-005.4:** Logical cleanup shall not claim physical file shrinkage or secure erasure. Physical reclamation shall use the established database-specific maintenance contract.

### REQ-PLATFORM-DELIVERY-RETENTION-006: Upgrade, operations and surface parity

**Intent:** Provide automatic safe cleanup on both supported database engines with clear operational limits.

#### Acceptance criteria

- **AC-PLATFORM-DELIVERY-RETENTION-006.1:** Fresh startup, old-schema upgrade, repeated startup and interrupted migration shall preserve delivery evidence on SQLite and PostgreSQL. Unsupported retention formats shall fail before admission.
- **AC-PLATFORM-DELIVERY-RETENTION-006.2:** Cleanup shall operate automatically without a feature flag. Validated startup configuration shall govern retention and work budgets without weakening safety boundaries.
- **AC-PLATFORM-DELIVERY-RETENTION-006.3:** Operators shall receive aggregate progress, removed bytes, backlog and bounded skip/error reasons through production structured logs on both engines. Development/pprof expvar shall supplement logs; no production metrics endpoint is required. Neither surface shall expose payloads, credentials or identity labels.
- **AC-PLATFORM-DELIVERY-RETENTION-006.4:** Supported rollback shall preserve checkpoints and recovery guards. A pre-retention binary shall not operate on a pruned database.
- **AC-PLATFORM-DELIVERY-RETENTION-006.5:** Existing desktop and phone history and recovery controls shall remain complete and usable after cleanup and reload. Public guidance shall distinguish backend retention, journal pruning and physical reclamation.

## Out of scope

- Canonical conversation expiry, canonical tool-payload removal policy or changes to native harness state. Redundant projected tool-update inbox payloads remain in scope.
- Agentctl journal pruning, physical compaction or executor lifetime changes.
- Automatic prompt/tool resend, synthetic workflow completion or bypassing recovery guards.
- A retention settings page, new desktop/mobile layout, feature flag or online database rewrite.
- Exactly-once external side effects or retention guarantees after operator removal of backups and executor volumes.
