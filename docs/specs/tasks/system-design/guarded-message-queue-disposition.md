---
status: current
system: tasks
requirements:
  - REQ-TASKS-GUARDED-MESSAGE-QUEUE-001
  - REQ-TASKS-GUARDED-MESSAGE-QUEUE-002
---

# Guarded Message Queue Census and Disposition System Design

## Purpose and boundaries

The task system owns the durable per-session FIFO and its agent-facing MCP
contract. The implementation extends the existing `queued_messages` boundary;
it does not add an operator SQL workflow, Support relay, or deployment-specific
image customization.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-GUARDED-MESSAGE-QUEUE-001` | [Census and exact disposition](#census-and-exact-disposition), [Authorization](#authorization), [Failure and recovery](#failure-and-recovery) |
| `REQ-TASKS-GUARDED-MESSAGE-QUEUE-002` | [Scheduled routine wake coalescing](#scheduled-routine-wake-coalescing), [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- `internal/orchestrator/messagequeue.Service` produces content-free census descriptors and validates exact disposition requests.
- The memory and SQLite/PostgreSQL `messagequeue.Repository` implementations serialize disposition with every queue mutation for the same session.
- `internal/mcp/handlers` binds both operations to the server-derived MCP principal and stamps trusted scheduled automation messages with routine identity.
- `internal/mcp/server` exposes `get_message_queue_census_kandev` and `dispose_message_queue_entries_kandev` without callable task, session, workspace, force, clear-all, or message-body fields.
- `pkg/websocket` carries the two internal MCP bridge actions. Raw WebSocket access remains outside the supported contract.

## Census and exact disposition

The census reads visible pending rows in persisted position order. Each
descriptor contains the entry ID, position, queued time and provenance,
content SHA-256 and byte count, attachment count, optional routine identity,
and an opaque claim. It does not contain `content`, attachment data, or
unfiltered metadata.

The claim is a SHA-256 digest over the complete stored `QueuedMessage`
snapshot, including session, task, position, content, model, attachments,
metadata, queued time, and provenance. It is an optimistic compare token, not
an authorization token.

Disposition takes one or more `(id, claim)` pairs. Under the existing
per-session lock and one database transaction, the repository reads the
current rows, compares each complete snapshot, removes only matches, and
returns `removed`, `changed`, or `not_found`. Before and after counts are
computed from visible pending rows in the same transaction. Duplicate or empty
claims fail before mutation.

## Authorization

The local task MCP server injects its own task and session IDs into a fresh
backend payload. The backend ignores caller attempts to widen the scope and
requires those IDs to equal `mcpscope.Principal.CallerTaskID` and
`CallerSessionID`. The principal resolver has already verified that the
session belongs to the task and derived the task workspace, so a non-empty
matching principal binds all three identities.

Both tools operate only on the calling session. They cannot target a child,
sibling, another session on the same task, or any foreign workspace.

## Scheduled routine wake coalescing

Only a principal with the automation MCP surface can produce a routine wake
marker. The handler additionally requires:

- the principal task and session to match the sender task and session;
- task origin `automation_run`;
- task metadata `trigger_type="scheduled"`;
- matching non-empty `automation_id` and `trigger_id`.

The semantic key hashes stable automation and trigger identity plus the
complete expanded prompt. Session scoping and reserved `queued_by` provenance
remain part of the repository match. A matching pending row is replaced in
place, preserving its immutable entry ID and FIFO position. Any distinct
payload gets a distinct key, and non-scheduled sources never enter this path.

## Failure and recovery

- A disposition loser observes `not_found`; a row changed after census reports
  `changed`. Neither outcome removes another row.
- A durable lifecycle row reserved in flight is absent from census. If it is
  reserved after census, its snapshot changes and disposition fails closed.
- Queue rows and routine metadata already live in `queued_messages`, so restart
  recovery and FIFO order use the existing persistence contract. No schema
  migration or destructive data rewrite is required.
- Coalescing checks for an existing semantic key before enforcing capacity, so
  an identical wake remains admissible when the queue is full while distinct
  work still receives the normal `queue_full` result.
- When the guarded tools are unavailable, the agent must preserve the queue and
  wait for normal FIFO delivery or ask an operator to use the authenticated UI.

## Observability

Census responses expose before count and ordered IDs. Disposition responses
expose atomic before and after counts and every per-entry outcome. Backend logs
record the same session, counts, IDs, and outcomes. Routine coalescing logs the
retained entry ID and stable routine identity. No path logs a message body.

## Related decisions

- [Guard agent queue disposition with exact snapshot claims](../../../decisions/2026-09-01-guard-agent-queue-disposition.md)
- [Separate Message Queue Provenance, Cancellation, and Capacity](../../../decisions/2026-08-03-separate-message-queue-provenance-cancellation-and-capacity.md)
