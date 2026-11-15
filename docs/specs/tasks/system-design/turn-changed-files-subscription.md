---
status: draft
system: tasks
requirements:
  - REQ-TASKS-TURN-CHANGES-005
owners:
  - kandev
---

# Turn changed-files subscription reconciliation

## Boundary

This design refines [turn changed-files](turn-changed-files.md) live projection
for AC-TASKS-TURN-CHANGES-005.1 and AC-TASKS-TURN-CHANGES-005.9. Capture,
persistence, and revision ownership remain in the original design.

## Subscription and history

After session-subscription readiness, revalidate the newest history page so an
update between an initial read and registration cannot be lost. An earlier
in-flight page must settle before that read starts. Revision-aware merging
preserves newer live updates when a page arrives late. Revalidate after
reconnection or client replacement and release the subscription with its consumer.
A client installed while connection status is unchanged still acquires a lease.
Background completion can update local error state only for its active consumer. Transcript
consumers subscribe only while their session detail is active, and reconcile
when it becomes active again. Offline reads remain available.

The transcript and historical-diff consumers share this behavior across
desktop and phone; their presentation and scroll ownership remain unchanged.
