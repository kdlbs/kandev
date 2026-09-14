---
status: current
system: tasks
requirements:
  - REQ-TASKS-BOUNDED-SESSION-HISTORY-001
  - REQ-TASKS-BOUNDED-SESSION-HISTORY-002
---

# Bounded session history storage System Design

## Purpose and boundaries

The task system owns conversation history, tool execution records, Git
snapshots, and plan revisions. This design defines cursor-bounded hydration
and digest-backed payload storage with retention candidates. The backup-gated
maintenance command that consumes those candidates is owned by the system-page
system.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-BOUNDED-SESSION-HISTORY-001` | Hydration; Reader freshness |
| `REQ-TASKS-BOUNDED-SESSION-HISTORY-002` | Payload storage; Retention candidates |

## Hydration

`task_session_messages` keeps a lightweight metadata projection per message.
List endpoints read by cursor (keyset pagination on the message cursor), and
detail payloads are fetched only through the explicit payload endpoint.
Legacy hydration paths use the same bounded reads, so no full-row fallback
remains. A test with separate reader and writer pools repeats committed-write
observation to prove cross-pool freshness (`TestPlanService_SeparateReaderWriterPoolsObserveCommittedWrites`).

## Payload storage

A payload above the inline threshold is stored in
`task_message_payload_content` keyed by digest with compression encoding, byte
size, and an internal reference; message rows retain the lightweight metadata
projection. Explicit detail loading verifies the digest before returning
content. Payload metadata keyed by payload identity is deduplicated between
sessions. Equivalence-class Git snapshots within one session and digest group
share one stored snapshot; distinct sessions keep distinct rows.

## Retention candidates

`ListOrphanedMessagePayloadCandidates`,
`ListObsoletePlanRevisionCandidates`, and the snapshot candidate queries expose
counts and references without deleting. Retention windows protect HEAD
revisions, revert ancestry, recency defaults, and query limits, so a
maintenance run only ever selects what policy allows.

## Components

- `apps/backend/internal/task/repository/sqlite/message.go`,
  `message_payload.go`, `git_snapshots.go`, `git_snapshot_digest.go`,
  `plan.go`: bounded reads, payload and snapshot persistence, candidate
  queries.
- `apps/backend/internal/task/service/service_messages.go`: cursor API and
  explicit payload hydration.

## Failure and recovery

A payload whose digest does not verify fails the explicit detail request
rather than returning unverified content. Missing-table startup states are
ignored by logical storage scans; other scan or query failures surface as
errors.

## Related decisions

- [Install-wide storage maintenance uses typed ownership providers and quarantine](../../../decisions/0045-install-wide-storage-maintenance.md)
- [Keep PR watches task owned](../../../decisions/2026-08-31-task-owned-pr-watch-identity.md)
