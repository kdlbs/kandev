# ADR-2026-09-16-passive-session-inspection: Keep inspection separate from execution ownership

**Status:** proposed
**Date:** 2026-09-16
**Area:** workflow

## Context

Workflow parking and process loss both appear as resumable sessions without a
running agent. Treating these cases alike lets opening an old conversation wake
it. The same resume request currently receives manual ceiling privileges.
A sibling ready event can then reconcile a queued task to Review.

## Decision

Passive inspection carries explicit intent across the client/server boundary.
The server verifies workflow parking and queued ownership before launch side
effects. Workflow parking is durable policy state, separate from execution-stamped
stop-event suppression and from the background-work projection.

The exact workflow entry and destination own the deferred launch. The visible
conversation does not own it. Task reconciliation considers valid deferred work
as well as running sessions. Queue UI consumes a bounded task-status projection,
not transcript text or a second client queue.

Explicit user execution remains possible and retains current manual admission.
It does not transfer workflow ownership from the queued recipient. A valid
workflow re-entry can release parking for its selected recipient.

## Consequences

The implementation adds a durable parking marker and explicit inspection
semantics. Old tasks need conservative recognition of existing park provenance.
The browser must propagate inspection intent through recovery fallbacks.
Existing manual callers remain compatible; this is not a new authorization grant.

Counts in queue details are observations with freshness, not queue positions.
Task and summary revisions must prevent a delayed update from undoing a clear.

## Alternatives considered

- Disable all automatic recovery: changes unrelated recovery workflows and the
  existing user preference. Suppress only ineligible passive execution.
- Mark predecessors Completed: loses the deliberate nonterminal park/reuse behavior.
- Treat any stop-intent tombstone as permanent parking: a tombstone must survive
  legitimate reuse to suppress delayed callbacks, so it cannot own current policy.
- Add a hard limit to every manual launch: changes the existing admission contract.
- Repair only the sidebar spinner: leaves unintended execution and wrong task state intact.

## Related records

- [Requirements](../specs/tasks/requirements/queued-session-ownership.md)
- [Design](../specs/tasks/system-design/queued-session-ownership.md)
- [Workflow lifecycle decision](2026-08-31-workflow-profile-session-switch-policy.md)
