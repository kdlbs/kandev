# ADR-2026-09-19-mcp-self-sibling-placement: Resolve depth-limited self creation to a sibling

**Status:** accepted
**Date:** 2026-09-19
**Area:** protocol

## Context

Kanban permits one child level. A child using the natural `parent_id="self"`
creation shorthand currently fails, requiring another call with its parent's
ID. [Issue #3773](https://github.com/kdlbs/kandev/issues/3773) asks which placement
and coordination relationship should apply. The maintainer selected automatic
sibling placement with a tool-result explanation in the
[decision](https://github.com/kdlbs/kandev/issues/3773#issuecomment-5746147513)
and explicitly directed specification in that direction.

## Decision

Interpret literal self as the caller unless Kanban depth requires using its
direct parent. Report that resolution in the result. Preserve literal UUID
semantics and the one-level service invariant. The common parent remains the
coordinator; the calling session remains the causal creator. Do not introduce
an independent delegator identity.

This records the chosen direction only. The maintainer separately authorized
implementation of the linked package on 2026-09-21. Implementation acceptance
is recorded in its plan and work order.

## Consequences

Valid child-originated creation needs one tool call. Agents must read the
reported parent because created work may be a sibling. Existing parent-based
completion and question routing remain meaningful. Compatibility requires
carrying literal-self intent separately from its resolved UUID; legacy traffic
must not acquire fallback merely because a UUID equals the caller's ID.

## Alternatives considered

- Retain explicit failure: predictable, but requires the corrective call the
  maintainer wants to avoid.
- Add a public sibling option: explicit intent, but requires agents to learn and
  select another option for this common boundary.
- Add a delegator relationship: enables child-owned coordination, but changes
  permissions, completion and question routing beyond the requested outcome.

## References

- [Requirements](../specs/tasks/requirements/mcp-self-sibling-placement.md)
- [Design](../specs/tasks/system-design/mcp-self-sibling-placement.md)
