---
status: current
system: agents
created: 2026-10-14
requirements:
  - REQ-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001
---

# Explicit Spawn Session Launch Intent

## Purpose and boundaries

Implement [the spawn-session requirement](../requirements/spawn-session-effective-profile.md)
at the core backend MCP handler boundary. An explicit request for an additional
session creates a distinct session even when the active workflow step targets
an existing session with reuse. Ordinary workflow launch routing remains valid.
The repair adds no API field, storage schema, plugin, executor policy, or
deployment requirement.

## Components and data flow

1. `internal/mcp/handlers/spawn_session.go` verifies target access and caller
   attribution through the existing handler guards. It resolves a supplied
   profile first, then a verified same-task caller profile, then the target
   primary session profile.
2. The handler calls `LaunchSession` with start intent and
   `ProfileExplicit: true` for every explicit spawn, including an unresolved
   profile. This expresses additional-session intent at the existing routing
   boundary; it does not bypass profile or Office validation.
3. `internal/orchestrator/task_operations.go` consults a workflow session route
   only when `ProfileExplicit` is false. Explicit spawn therefore uses normal
   new-session allocation and launch preparation rather than promoting or
   renaming the workflow's existing target.
4. The successful MCP response projects the actual launch result's session and
   profile IDs. Naming applies to that new session. Existing source-session
   identity, name, profile, source binding and committed workflow route remain
   intact.

Additional MCP spawns also carry server-only `PreserveWorkflowBinding` intent,
separate from `ProfileExplicit`, through the launch request and start options.
It prevents a sibling session from replacing a pinned source step's binding.
Ordinary workflow and selector-backed launches still record their source session.
The ceiling start payload persists this intent and restores it on replay; an
older payload without the field retains ordinary recording behavior. The flag
is not accepted or exposed through the public session-launch JSON schema.

## Profile and failure behavior

A non-empty resolved spawn profile wins over workflow pins, defaults, and
session targets. When none resolves, existing launch defaults and errors still
apply. A step with `SessionTarget` intentionally supplies no standalone profile
default, so a missing profile can fail with `ErrNoAgentProfileID`; it must not
fall back to starting the existing target. A normal step with a valid default
can still create a distinct session using that default.

Existing workspace attachment, authorization, caller validation, profile
preflight, error mapping and Office scheduler ownership remain authoritative.
The response reports a successful launch rather than echoing the requested
profile; a subsequent name-update failure retains the existing behavior.

## Requirement mapping and verification

`REQ-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001` is covered by the handler
boundary, profile/failure behavior and response projection above.

The real handler-to-orchestrator regression uses isolated temporary SQLite
stores and a fake external agent runtime. It verifies returned, persisted and
launched identity, exactly one new row, same/different supplied profiles,
caller/primary inheritance, empty-profile error and normal-default behavior,
Office ownership, unchanged source state, and ordinary workflow reuse. Existing
handler tests cover authorization, caller attribution and launch/name failures.
Tool registration tests and public documentation describe the same contract.

Pinned-source regressions cover same/different supplied profiles, caller and
primary inheritance, and unchanged complete binding records. Persisted ceiling
replay preserves the original binding and downstream source-target resolution;
ordinary workflow and selector launches are positive recording controls.

This is source-level acceptance. Release acceptance requires the containing
release to launch a distinct session into a guarded turn with the intended
profile and preserve ordinary reuse before deployment-overlay retirement.

## Delivery links

The [repair plan](../../../plans/explicit-mcp-spawn-distinct/plan.md) links the
integration regression, launch-intent implementation and contract work orders.
No UI or mobile flow changes; no long-running test service is required.
