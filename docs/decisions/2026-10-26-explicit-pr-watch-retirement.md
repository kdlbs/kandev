# ADR-2026-10-26-explicit-pr-watch-retirement: Explicit PR watch retirement

**Status:** accepted
**Date:** 2026-10-26
**Area:** backend

## Context

Issue #4666 reports that deleting a terminal task's watch does not stop branch reconciliation from recreating it.
The branch and task remain valid. Removing a watch row loses the operator's stop decision.
Session completion cannot determine eligibility because Review tasks still need monitoring after their sessions finish.

## Decision

Persist explicit retirement on the canonical watch row. Reserve its identity until authorized resumption or permanent owner deletion.
Only a dedicated resume operation clears retirement. Task reopening, branch activity, PR refresh, and association writes do not clear it.
Resume allocates a new watch ID so delayed provider results cannot enter the new watch lifetime.
This proposal is part of the unimplemented fix package.

## Consequences

Operators can stop monitoring while retaining task history, resources, associations, and automation configuration.
DELETE gains persistent semantics. Retired rows need authorized inspection and resumption support.
Archive cleanup must retain explicit retirement. Permanent deletion still removes owned state.
Stored automation options remain intact, but watch-driven observations stop during retirement.

## Alternatives considered

- Exclude every Done task: changes existing discovery and automation behavior without an explicit stop decision.
- Delete the row only: reproduces the reported recreation bug.
- Require archival: also affects sessions and development resources.
- Add a separate suppression table: duplicates canonical identity and cleanup responsibilities.
- Resume on push or task reopening: makes retirement depend on background activity and obscures operator intent.

## References

- [Issue #4666](https://github.com/kdlbs/kandev/issues/4666)
- [Requirements](../specs/integrations/requirements/github-pr-watch-retirement.md)
- [Design](../specs/integrations/system-design/github-pr-watch-retirement.md)
