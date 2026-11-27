# ADR-2026-09-28-attached-idle-runtime-parking: Workspace ACP idle suspension

**Status:** accepted
**Date:** 2026-09-28
**Area:** backend

## Context

The connected-instance reapers do not release idle ACP processes. The user requests optional workspace resource saving for all ACP agents.
The previous draft required complete OpenCode quiescence evidence. That gate exceeded the requested policy and blocked implementation.

## Decision

Use a workspace preference, disabled by default, with a two-hour default timeout.
Use Kandev-observed state and known active work to decide idle suspension, uniformly across ACP providers.
Do not require complete visibility into provider internals or a provider-specific quiescence API.
Preserve durable conversation and workspace identity, then resume on accepted messages or explicit task focus.
Identify idle suspension explicitly so focus does not override manual stops, cancellations, or workflow parking.
Keep disconnected-owner reaping and active-session admission separate.

## Consequences

Operators choose the retention tradeoff per workspace. Kandev protects known work and documents the limits of ACP visibility.
Implementation requires workspace persistence, lifecycle coordination, durable suspension provenance, and deduplicated recovery.
Executor teardown must preserve task-owned compute. The process can suspend again after each fresh idle interval.

## Alternatives considered

- Complete provider-internal quiescence proof: rejected as an unnecessary prerequisite for this opt-in policy.
- Reuse the installation-wide ACP timeout: rejected because the user requested independent workspace settings and defaults.
- Treat every stopped process as idle-suspended: rejected because focus would override explicit stop intent.

## References

- [Requirements](../specs/executors/requirements/idle-runtime-parking.md)
- [Design](../specs/executors/system-design/idle-runtime-parking.md)
