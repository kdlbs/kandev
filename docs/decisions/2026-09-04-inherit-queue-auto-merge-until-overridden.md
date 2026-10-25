# ADR-2026-09-04-inherit-queue-auto-merge-until-overridden: Inherit Queue Auto-merge Until Overridden

**Status:** accepted
**Date:** 2026-09-04
**Area:** backend, frontend, protocol

## Context

Automatic queued-message merging has one install-wide setting. The queue panel
needs a per-session switch without making every session a frozen copy of the
global value when it is created.

A binary switch cannot also expose a third "follow global" state. The product
contract therefore needs a precedence rule that lets untouched sessions follow
administrator changes while preserving an explicit session choice.

## Decision

Resolve automatic-merge policy for each message admission and queue-status read
with this precedence:

1. an explicit per-session override, when present;
2. otherwise the current install-wide automatic-merge value.

New sessions have no override. Existing sessions that have never changed the
queue-panel switch continue to inherit later global changes. The first user
change to the session switch persists an explicit boolean, and that session
remains independent of later global changes for its lifetime.

The message-queue domain stores the nullable override beside its existing
per-session Auto-run state. The global setting remains owned by the system
message-queue settings service. Successful queue status projects availability,
the effective boolean, and transport-only source and revision metadata. The
panel still renders a binary value and does not expose inherited state or a
reset action.

An override is attached to the exact session identity. Workflow queue transfer
does not copy it to the destination session; the destination keeps its own
override or inherits the current global value, and the source state is removed.
Deleting a session, task, or workspace removes state for the deleted session
identities atomically with that lifecycle operation. Archiving preserves state
because its sessions survive and may be unarchived. Reusing a deleted identity
therefore starts without an override.

Session deletion relies on queue repository transactional cleanup instead of a
post-delete queue mutation. A separate deletion-only post-commit callback
carries authoritative session/incarnation pairs to ephemeral memory storage;
the archive task purger never invokes it. The later recount is task-scoped with
an empty session ID, remains available to the internal status-summary
projector, and is consumed by gateway routing without browser broadcast.

Each task-session row also owns an opaque server-generated queue incarnation
UUID. It is immutable for that row and changes when a deleted textual session
ID is recreated. Its migration runs after every existing task-session table
rebuild, backfills distinct UUIDs replay-safely, and future rebuilds must
preserve it. Every session-scoped queue status carries this identity so the
client can reject delayed status from a removed incarnation before applying
entries or policy.

Ordinary admissions and Auto-merge mutations carry the expected incarnation
into persistence. A mutation request cannot be silently retargeted: a missing or
changed incarnation returns the existing non-enumerating session-not-found
response and writes nothing.

An override read failure fails closed for that admission: Kandev does not merge
messages when it cannot prove that an explicit OFF override is absent. A failed
write reports failure and leaves the previously persisted value authoritative.

An override read failure during queue-status construction does not synthesize an
effective OFF value. Session status reconciles independent queue data, marks
Auto-merge unavailable, omits its value/source/revision, and retains the status
epoch/generation allocated before the failed read. Clients apply availability
only from the newest status generation, retain any last authoritative tuple,
and disable the control until a newer successful resolution.

Global and session values each carry a persisted monotonic revision. The system
settings service commits the global value and revision together, then publishes
one immutable `{Enabled, Revision}` runtime snapshot through an atomic pointer.
Backend startup hydrates that pointer from the persisted pair before admissions;
it never reconstructs revision `0` from the boolean alone. Admissions and status
reads load the pair once, so they cannot observe a value from one generation
with another generation's revision. A global revision advances only when the
global value changes; a session revision advances with each successful override
write.

The repository owns the durable policy and lifecycle linearization point. SQL
admission, override mutation, and session deletion lock the owning task row
before the per-session queue lock, then validate the exact
task/session/incarnation. Admission reads the override and inserts or directly
folds in that transaction. This lock order matches task/workspace purge and
avoids both orphan insertion and lock inversion. The memory repository validates
session authority under an incarnation-tagged bucket mutex and deletion CAS
purges only matching buckets. The service admission mutex remains useful for
local ordering but is not the cross-connection correctness boundary. The
immutable global snapshot is process-local under Kandev's
single-orchestrator-owner deployment invariant.

Session queue status carries the immutable session incarnation plus a process
epoch and generation allocated before policy resolution. Clients first require
the status incarnation to match the current task-session row. A new matching
incarnation resets the prior queue-policy ordering state; a mismatch is
discarded. Within an incarnation, clients order available and unavailable
results by epoch/generation, then use policy source and revision to order
effective values. They accept the irreversible global-to-session transition,
reject a later global-source event for that incarnation, and compare revisions
within one source. A process restart changes the epoch and startup preserves
the persisted global policy revision. This prevents delayed failure,
pre-mutation status, or a deleted incarnation from replacing newer
authoritative value without scanning sessions when the global setting changes.

## Consequences

- Administrator changes immediately govern later admissions for every
  unmodified session without writing or scanning session rows.
- A user can make one session permanently differ from the install default with
  a compact binary switch.
- The UI cannot tell whether the displayed effective value is inherited or
  explicit, and it cannot restore inheritance after the first change.
- Successful session queue status, WebSocket events, admission logic, and
  full-capacity candidate folding resolve the same effective value, source, and
  revision. Failed session status resolution carries ordering plus an
  unavailable marker. Task-only status-summary signals are explicitly scoped
  and carry no session policy.
- Session-state storage requires a nullable override, monotonic revision, and
  replay-safe migration in both supported SQL dialects.
- The persisted global settings envelope gains an internal monotonic revision;
  public settings responses do not expose it as editable configuration.
- The queue service replaces its independent atomic boolean with one atomic
  immutable value/revision snapshot and hydrates it from persistence at startup.
- Session status gains process epoch/generation ordering so delayed unavailable
  results cannot disable newer successful state.
- Override writes and policy-aware admissions share the durable session lock,
  so a write through another database connection is ordered before or after the
  admission policy decision.
- Destructive lifecycle cleanup must distinguish session identity removal from
  task archive; only the former deletes queue policy state.
- Every task-session creation and legacy backfill needs a stable non-empty queue
  incarnation UUID. The migration runs after existing task-session rebuilds,
  and future rebuilds preserve it. Frontend deletion clears queue metadata,
  workspace deletion derives affected sessions before removing ownership
  indexes, and status apply compares against the current session incarnation.
- Task-scoped queue recounts remain internal; the gateway never broadcasts them
  through a session or empty-workspace route.
- Queue mutation requests and ordinary admissions require the expected
  incarnation; stale work cannot target a recreated textual session ID.
- Memory cleanup needs a deletion-only incarnation-aware callback distinct from
  archive queue-row purging.

## Alternatives Considered

1. **Snapshot the global value when each session is created.** Rejected because
   untouched active sessions would not inherit later administrator changes.
2. **Add a tri-state control with Follow global.** Rejected because the requested
   queue header control is a compact toggle, and no reset workflow is required.
3. **Store the override on the task or workspace.** Rejected because the user
   asked to isolate behavior per session and tasks can have several sessions.
4. **Copy the override during workflow session transfer.** Rejected because an
   override belongs to the exact session; the destination already has an
   independent policy identity.
