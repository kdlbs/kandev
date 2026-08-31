# ADR-2026-08-30-environment-owned-git-status: Project Current Git Status From the Task Environment

**Status:** proposed
**Date:** 2026-08-30
**Area:** backend, frontend, protocol

## Context

Multiple task sessions share one `TaskEnvironment` and one physical workspace.
Some sessions can be inherited or shared across task boundaries while still
binding to that environment.
Git-status transport is still session-scoped. Each mounted consumer can
subscribe with a different session ID, and each status event is then stored
under the shared environment key in the frontend.

This creates competing writers. A delayed sibling session can publish a status
from an obsolete workspace or an older database snapshot. The frontend then
replaces the current environment status with that result. In the observed
incident, 91 dirty files appeared and then disappeared when an older sibling's
clean snapshot arrived later.

## Decision

Treat the task environment as the authority for current Git status. Treat a
session ID only as a request and delivery handle.

Before the backend publishes current status, it must prove that the source
session is bound to the task environment and has a raw recorded workspace path
equal to the environment's canonical workspace. A live execution must match
that workspace; a recovered execution may have an empty environment ID only
after the session binding has been verified. A persisted snapshot is eligible
only when its source session has the same canonical binding. Among eligible
snapshots, the newest observation wins independently for each repository. The
outgoing event keeps the requested session ID for protocol compatibility.

The frontend keeps Git status under the environment key and applies a monotonic
timestamp rule per repository. An older observation cannot replace a newer
one. This is a defense against response reordering after the backend has
validated workspace authority.

Keep snapshots attributed to sessions. Resolve current environment authority
at projection time. Do not add a second persisted Git-status owner.

## Consequences

- Sibling subscriptions can no longer publish conflicting current status for
  one task environment.
- A clean canonical status can replace dirty state. The rule does not prefer
  non-empty file lists.
- Historical sessions with an obsolete workspace path remain in the audit
  record but cannot supply current environment status.
- Multi-repository status remains session-attributed and is retained one row per
  repository for fallback hydration.
- A lookup failure can temporarily produce no hydration event. It cannot clear
  correct frontend state with unverified data.
- PR #3167 remains necessary because it repairs the workspace path used when a
  session resumes. This decision protects the projection boundary before and
  after that repair.
- No database migration or WebSocket action change is required.

## Alternatives Considered

1. **Accept the last event received.** Rejected because network and hydration
   order do not define workspace authority.
2. **Reject only older timestamps in the frontend.** Rejected as the complete
   fix because a newer event can still come from the wrong workspace.
3. **Prefer non-empty status.** Rejected because a real clean workspace must be
   able to clear old dirty state.
4. **Unsubscribe hidden session components.** Rejected because another current
   or future consumer can recreate the same competing-writer bug.
5. **Move all snapshots to the task-environment row.** Rejected because it
   removes useful session attribution and adds a second persistence model when
   projection-time validation is sufficient.
