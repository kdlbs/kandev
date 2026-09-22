---
id: plugins-task-dependency-refresh-design
title: Refresh contract for the task dependency projection
status: draft
system: plugins
requirements:
  - REQ-PLUGINS-TASK-DEPS-005
owners:
  - kandev
created: 2026-09-17
last_updated: 2026-09-17
---

# Refresh contract for the task dependency projection System Design

## Purpose and boundaries

This design owns the fourth question the capability raises -- **when a consumer
learns its cached graph is stale, and what it is promised about that.** It covers
what canvas events carry, the refetch-on-signal rule and the `task.state_changed`
clause, the publication-is-not-delivery gap, and the no-lock and no-snapshot
rules (`REQ-PLUGINS-TASK-DEPS-005`).

It is separated from [task dependency projection](task-dependency-projection.md)
by question, not by size, on the same principle as the other splits: that design
settles what a response says and where it is attached, and this one settles what
happens after it is read. `REQ-PLUGINS-TASK-DEPS-005` keeps the identity it was
reviewed under, its criteria stay in the projection requirements document, and
nothing is renumbered.

Adjacent contracts used but not owned:

- [Task dependency projection](task-dependency-projection.md) -- the field set,
  the derivation and the attachment rule whose staleness this governs.
- [Isolated web-app contributions](isolated-web-app-contributions.md) -- the
  canvas event stream, its field allowlist and its scope routing.
- [Task dependencies](../../tasks/system-design/task-dependencies.md) -- the
  events edge mutation already publishes.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLUGINS-TASK-DEPS-005` | [Events and refresh](#events-and-refresh) |

## Events and refresh

Canvas events carry no dependency projection. The event projection is a flat
field allowlist with no per-entry scope evaluation, so an edge list on an event
would carry other tasks' titles past the redaction rule that the data path
enforces. The allowlist stays as it is.

Edge mutation already publishes a task-updated event for both ends, and the two
dependency lifecycle events are published too. What the consumer receives is a
change signal without a payload, and the contract is explicitly
refetch-on-signal: the signal does not identify which fields changed, because
the changed-field list is not on the event allowlist either, so the rule is to
re-read the affected tasks on any of the three event types.

Those three cover edge mutation and resolution, and they miss the commonest
change of all: a predecessor simply moving state. A `depends_on` entry carries
the predecessor's `state`, and the dependency handler publishes a
dependent-facing event only when the dependent's own blocked verdict resolves or
fails -- a predecessor advancing while another predecessor is still pending
publishes nothing for the dependent. The only signal is `task.state_changed` for
the predecessor itself, which a consumer holding a cached dependent has no reason
to connect to that dependent. Left there, the rendered edge state is stale
indefinitely, which is the one staleness this contract would not have declared.

The fix is a consumer rule over an event that is already published, not a new
event (`AC-PLUGINS-TASK-DEPS-005.7`, `AC-PLUGINS-TASK-DEPS-005.8`): a consumer
re-reads a cached task on `task.state_changed` for any task named in its
`depends_on` or `blocks`. A dependent-facing event was rejected because the
event contract is owned elsewhere, the fan-out is unbounded in the dependent
count, and `AC-PLUGINS-TASK-DEPS-005.2` already declines to promise
scope-complete delivery, so a new event would inherit the same routing gap it was
added to close.

Publication is not delivery, and this design does not promise the gap away.
The canvas event gateway matches an event against the instance's scope using the
identities the event itself carries, and for repository and session scopes that
is a *single* identity, while the direct-read predicate admits a task on any of
its repositories or sessions. A repository- or session-scoped canvas can
therefore miss a `task.updated` for a task it could read directly. Widening
event routing to the full set would change the event contract, which this design
does not own, so the honest guarantee is the one in
`AC-PLUGINS-TASK-DEPS-005.3`: publication for both ends, plus a documented
refetch rule that a consumer can also drive from its own refresh. Closing the
routing gap is listed as out of scope in the requirements rather than assumed
here.

One response is not a transactional snapshot. The derivation issues its reads
sequentially without a surrounding transaction, so an edge mutated mid-response
can appear on one end and not the other. Guaranteeing symmetry would mean
holding a transaction across the derivation that every board read also uses.
The consumer rule is therefore: treat the union of what it read, and refetch on
signal.

Reads take no dependency mutation lock. The process-wide lock that serializes
validate-then-insert on the write path is not acquired for a derivation, so a
read never delays an edge mutation and concurrent reads never serialize against
each other. Acquiring it for reads would put every plugin and board read behind
dependency writes to buy a snapshot the contract does not promise.

## Test strategy

- Events carry no projection: the canvas event projection for `task.updated`,
  `task.dependencies_resolved` and `task.dependency_failed` carries no projection
  field, no edge list, and no edge end's title or state, asserted directly rather
  than inferred from the field allowlist. The internal event *does* carry the
  recomputed projection, so this holds only because the allowlist omits those
  keys; the test is what makes a later widening fail here rather than silently
  publish every edge end's title to every canvas.
- Reads take no mutation lock: a derivation driven through a blocker source that
  fails if the mutation lock is held completes normally, and concurrent reads of
  one task do not serialize.

## Documentation

Two of the consumer-contract items listed in the
[projection design](task-dependency-projection.md#documentation) are this
design's: the refresh rule including its `task.state_changed` clause, and the
non-snapshot rule. They belong in `docs/public/canvases.md`,
`docs/public/plugins-authoring.md`, and the canvas authoring browser-API
reference.

## Related decisions

- [ADR 0043 -- Plugin host data API](../../../decisions/0043-plugin-host-data-api.md)
- [Isolated web-app contributions](isolated-web-app-contributions.md)
- [Task dependency projection](task-dependency-projection.md)
