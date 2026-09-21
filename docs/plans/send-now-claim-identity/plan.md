---
created: 2026-09-21
status: complete
requirements:
  - REQ-UI-MESSAGE-QUEUE-SEND-NOW-001
system_design:
  - ../../specs/ui/system-design/message-queue-send-now.md
legacy_specs: []
---

# Implementation Plan: Preserve Send Now claim identity across session handoff

Session handoff must carry pending Send Now claims to the successor session
without carrying the retired session's incarnation or operation generation.
This corrective package records the narrow persistence repair and its restart
reconciliation proof. The existing Send Now requirement and system design
remain authoritative for queue ownership, FIFO ordering, and fencing.

## Scope

- Rebind accepted and unaccepted pending claims to the live destination
  session identity during both direct and durable owned transfers.
- Preserve claim IDs, accepted state, source ordering, dispatch metadata,
  transaction rollback, and stale source fencing.
- Prove recovery after the source session is removed and the process restarts.

## Out of scope

Queue feature behavior, UI changes, provider routing, schema changes, incident
database repair, deployment, and backup cleanup.

## Delivery

See [Task 01: Rebind transferred Send Now claims](task-01-rebind-claims.md).

