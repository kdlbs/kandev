# ADR-2026-10-15-private-executor-admission: Private executor admission

**Status:** accepted
**Date:** 2026-10-15
**Area:** backend

## Context

A shared board grants collaboration reach, but it must not grant the use of
another engineer's compute and profile-seeded credentials. Configuration is
currently shared, and internal scheduler contexts can bypass user scoping.
Issue [#4512](https://github.com/kdlbs/kandev/issues/4512) proposes optional
ownership without changing existing installs. The implementation preserves
shared defaults while enforcing ownership at configuration and compute admission.

## Decision

Use single-user ownership on executor and profile records, shared when
unset, with conjunctive parent/profile eligibility. Use current
`org.config.manage` authority as the management/use bypass within existing
resource reach. Preserve workspace and tenant authorization independently.

Owned-compute admission requires a real accountable user, or the positively
established authentication-disabled single-user identity. A scheduler's
unscoped context is not authority. Autonomous launches use a recorded Kandev
creator; interactive requests also check the actual caller. Unknown legacy
creators cannot use private compute. Ownership changes take effect at subsequent
admission, without terminating work already admitted.

Protect configuration, notifications, and existing session execution surfaces
as well as launch pickers. Otherwise a shared task, stale client, or mutable
automation could bypass the compute boundary.

## Consequences

Existing unowned resources continue to work. Private resources require durable
creator/actor attribution, filtered projections, and guarded admission at shared
execution seams. Administrators can recover resources with missing owners.
Grant lists and broader configuration-resource ownership are separate future
contracts. Local/Worktree host credentials remain an OS isolation limitation.

## Alternatives Considered

- Profile-only checks: simpler, but a private parent could be reached through a
  sibling profile unless parent eligibility is also enforced.
- Picker filtering or auditing only: leaves direct APIs, WS notifications, MCP,
  background runs, and existing-session execution outside the boundary.
- Service/admin identity for background work: turns scheduler privilege into
  private-compute authority and cannot enforce creator revocation.
- Workspace-owner inference for legacy sources: changes who authorized execution
  and can silently lend another user's compute. Unknown attribution fails closed.
- Allowed-user grants or a generic resource-ownership framework: expands policy
  and management beyond the requested single-owner primitive.

## References

- [Requirements](../specs/executors/requirements/per-user-ownership.md).
- [System design](../specs/executors/system-design/per-user-ownership.md).
