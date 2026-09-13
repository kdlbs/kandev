---
status: current
system: tasks
requirements:
  - REQ-TASKS-CROSS-WORKSPACE-HANDOFF-001
  - REQ-TASKS-CROSS-WORKSPACE-HANDOFF-002
---

# Cross-workspace task handoff system design

## Boundary and ownership

The task system owns delivery-task identity, handoff metadata, reverse-link
updates, idempotency outcomes, and deferred session preparation. The Office
and agent systems supply the caller permission and profile records. The MCP
handler is the boundary that combines these inputs and passes trusted values
to task and orchestrator services.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-CROSS-WORKSPACE-HANDOFF-001` | Components and flow; Persistence; Security and failure behavior |
| `REQ-TASKS-CROSS-WORKSPACE-HANDOFF-002` | Deferred start state |

## Components and flow

1. The Office MCP profile registers `handoff_task_kandev` only when the trusted
   session can use the handoff capability.
2. The MCP handler re-derives permission from the trusted principal, resolves
   the target workspace and resources, and validates both selected profiles.
3. The task service creates a Kanban delivery task with handoff provenance and
   the requested `start_agent` intent. External identifiers use the existing
   idempotency repository contract.
4. The source task receives a compare-and-set reverse-link update. A replay
   repairs a missing link only when the stored source identity matches.
5. When start is requested, the orchestrator prepares and launches the new
   session. When start is deferred, preparation stores the explicit profile
   marker for the later board or workflow start path.

## Persistence

The delivery task stores a `handoff_source` metadata object. The source task
stores a `handoffs` array. Both records use the same source task, delivery task,
session, profile, and RFC 3339 handoff time. The reverse-link repository method
updates only the `handoffs` key and applies compare-and-set retries so concurrent
handoffs do not lose entries. Generic metadata mutation paths cannot create,
replace, or erase these protected records.

Deferred preparation stores an internal session marker for an explicit handoff
profile. The first successful launch clears the marker. A failed launch leaves
the marker so a later retry does not fall back to a workflow default.

## Security and failure behavior

Caller-supplied identity fields never grant permission. Missing or failed
resource reads fail before task creation or launch, and inaccessible resources
remain indistinguishable from absent resources. Same-workspace targets are
rejected by the handoff contract. Activity writes are best-effort audit output;
task and provenance state remain authoritative.

Idempotent found outcomes do not create or mutate a second delivery task. A
source mismatch refuses reverse-link repair. An unreadable stored handoff time
returns the existing partial-failure result instead of inventing a new time.

## Verification boundary

The handoff handler, task metadata service, SQLite handoff repository, and
orchestrator deferred-start path each have focused regression tests. The tests
cover permission derivation, cross-workspace resource checks, idempotent replay,
compare-and-set updates, metadata forgery protection, and explicit profile
retention.
