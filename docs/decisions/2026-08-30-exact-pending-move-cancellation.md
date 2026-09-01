# ADR-2026-08-30-exact-pending-move-cancellation: Bind Administrative Pending-Move Cancellation to an Exact Row Generation

**Status:** accepted
**Date:** 2026-08-30
**Area:** backend, protocol, security, workflow

## Context

`TakePendingMove(sessionID)` consumes whichever pending move is keyed to a
session at execution time. A separate read followed by that consume cannot prove
that the row is still the reviewed row: another request can replace it between
the operations. For a Human-QA task, messaging the session to repair the state
can itself resume the session and apply the stale move.

Automatic TTL and orphan reaping solve a different lifecycle problem. They do
not provide an authenticated administrative operation that can remove one named
live row before expiry.

## Decision

Kandev identifies each persisted pending-move generation with an opaque row ID.
Replacement rotates this ID; transfer and snapshot restore preserve it. The
automation Coordinator surface exposes one administrative cancellation tool
that requires the row ID and six relationship/state predicates: keyed session,
task, move, workflow, current step, and target step.

Authorization, relation validation, exact comparison, audit, and deletion occur
inside one repository transaction serialized with other pending-move operations
for the keyed session. The delete repeats the complete tuple and must affect one
row. Any mismatch, replacement, missing relation, revoked/stale caller, or
authorization denial performs no mutation and returns one stable non-leaking
result.

Caller identity is server-attested. The operation consumes the shared
workspace Coordinator grant and validates the caller's live task, session,
execution, workspace, and workspace-owner identity. Request arguments cannot
assert authority. Ordinary-agent self-only authorization is unchanged, Support
authorization is deferred, and no predicate-light or raw SQL variant exists.
The grant itself is database-bound to a non-empty workspace/task pair through
the unique `tasks(workspace_id, id)` key and a composite cascading foreign key;
an authorization-time join does not replace that storage invariant.

The outcome audit and successful delete are atomic. Evidence records only safe
identifiers, prior step state, outcome, changed flag, and timestamp. Tool
payloads and secrets are excluded from logs. Schema-invalid MCP calls reach the
backend denial-audit path; transport validation cannot silently bypass durable
evidence, and audit failure returns the sanitized internal failure.

## Consequences

- A reviewed row can be disarmed without messaging or resuming its session.
- Concurrent replacement cannot cause deletion of a newly queued move.
- A client that loses a success response can retry safely; the retry receives
  the stable miss and makes no further mutation.
- Coordinator grant management remains a dependency on the shared trust
  control plane. An installation with no grant is fail closed.
- Invalid or cross-workspace Coordinator designations cannot be represented,
  even if a future caller bypasses the runtime authorization join.
- Row identity and audit schema are additive and may remain after rollback.
- TTL/orphan reaping remains separately owned and can compose through the same
  row identity without duplicating policy.

## Alternatives Considered

### Preflight followed by `TakePendingMove(sessionID)`

Rejected because the consume is keyed only by session and can delete a
replacement created after preflight.

### Match only `move_id` or session plus target step

Rejected because neither identifies the reviewed database row generation or
proves the current task/workflow relationship.

### Owner-authenticated REST endpoint or raw SQL support procedure

Rejected because it would create another trust surface, permit predicate-light
mutation, or bypass application audit and transaction invariants.

### Reuse automatic TTL/orphan cleanup

Rejected because waiting for expiry does not solve urgent administrative
disarm, while broadening the reaper would conflate lifecycle policy with a
reviewed exact mutation.

### Permit a reviewed Support principal in the first release

Deferred until the shared trust contract defines a server-attested Support
principal and review lifecycle with equivalent workspace scoping and audit.

## Addendum: Read-Only Exact Pending-Move Census

**Date:** current session
**Area:** backend, protocol, security, workflow

### Context

The Coordinator surface exposed exact cancellation, but a caller preparing a
recovery handoff had no authoritative way to learn whether a target task still
had an armed pending move, or to obtain its exact seven-predicate tuple, without
already holding it. A caller cannot construct `session_id`, `move_id`,
`workflow_id`, or the two expected workflow-step IDs out of thin air, so the
existing cancellation tool was unreachable for a task the caller had not
directly instrumented. Falling back to a null/absent read carries no proof: an
unauthorized caller and an authorized caller who happens to find nothing are
otherwise indistinguishable, and that ambiguity is unsafe to build a resume
decision on.

### Decision

Kandev adds `read_pending_move_kandev`, a companion automation Coordinator tool
keyed only by `task_id`, reusing the exact same authorization transaction as
cancellation (self-target and reachable-task-tree checks included) and the same
`pending_move_cancellation_audit` table, discriminated by a new additive
`action` column (`cancel` or `read`, default `cancel` for existing rows).

The read diverges from cancellation's failure model in exactly one place:
an authorized caller who finds no armed row receives a genuine, audited
`found: false` success, not a denial. Every authorization or relation failure
still collapses into the identical `pending_move_not_found_or_changed` used by
cancellation, so an unauthorized caller can never distinguish "denied" from
"empty" — the zero-row outcome is only observable once authorization has
already been proven. The read never deletes, never touches `pending_moves`,
and a row it surfaces remains cancellable afterward with the identifiers it
returned.

### Consequences

- A Coordinator can now discover a target task's exact pending-move tuple
  before attempting cancellation, closing the gap where cancellation's inputs
  were otherwise undiscoverable for tasks the caller had not already tracked.
- Zero-row and denied are provably distinct only to an authorized caller;
  authorization is not weakened to make the census usable.
- The read is a pure query: it takes no lock, blocks no concurrent
  cancellation, and does not change what a subsequent cancellation call can
  see or affect.
- Sharing the audit table with an `action` column avoids a second evidence
  store and second redaction/observability surface for what is otherwise the
  same authorization and relation logic.
- Rollback of the read-only tool leaves cancellation, its audit history, and
  the shared `action` column untouched.

### Alternatives Considered

#### Return the same non-leaking miss for both denial and zero-row

Rejected because it reproduces the exact ambiguity that motivated this
addendum: an authorized caller could never trust a "nothing to cancel" result
enough to resume a waiting session on it.

#### A separate audit table for read operations

Rejected because it duplicates the existing table's schema, indexes, and
redaction handling for no additional isolation; the two operations already
share one authorization and relation model.

#### Return the full pending-move row to any authenticated caller

Rejected because it would remove authorization from an operation that exposes
task/session/workflow relationship data, undermining the same trust boundary
cancellation enforces.
