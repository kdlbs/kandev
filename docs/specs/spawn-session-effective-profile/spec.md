---
status: shipped
created: 2026-08-12
owner: kandev
---

# Spawn Session Effective Agent Profile

## Why

Agents use `spawn_session_kandev` to start another session with a selected agent profile. The tool must report the profile that the new session uses.

Without this guarantee, a caller can receive confirmation for one profile while a workflow profile starts instead.

## What

- `spawn_session_kandev` reports the effective agent profile for each successful launch.
- The reported profile is the profile on the new session, after workflow profile resolution.
- A workflow step launch profile keeps its current precedence. A pinned step profile wins first. Otherwise, the workflow default wins on that step.
- Without a workflow launch profile, an explicit `agent_profile_id` keeps its current precedence.
- Without an explicit profile, the current inheritance rules remain unchanged.
- The tool description explains the precedence and the response field.

## API surface

The successful `spawn_session_kandev` response has this shape:

```json
{
  "task_id": "string",
  "session_id": "string",
  "state": "string",
  "agent_profile_id": "string"
}
```

`agent_profile_id` identifies the effective profile on the new session. It does not echo an earlier profile choice when workflow resolution selects another profile.

## Failure modes

- If the session launch fails, the tool returns the existing launch error and no success response.
- If the task workspace is still materializing, the tool returns `CONFLICT`
  with `reason: workspace_preparing`, `recoverable: true`, and retry guidance.
- If the canonical workspace cannot be safely attached, the tool returns
  `CONFLICT` with `reason: workspace_reuse_unsafe`; it never provisions a
  replacement checkout as part of spawning a session.
- A successful session launch returns the nonempty effective profile from the launch result.
- A session name update can still fail without changing the successful launch result.

## Scenarios

- **GIVEN** an unpinned workflow step with workflow default A, **WHEN** a caller requests profile B, **THEN** the new session and response use profile A.
- **GIVEN** a workflow step pinned to profile A, **WHEN** a caller requests profile B, **THEN** the new session and response use profile A.
- **GIVEN** no workflow launch profile, **WHEN** a caller requests profile B, **THEN** the new session and response use profile B.
- **GIVEN** no explicit profile or workflow launch profile, **WHEN** a same-task session spawns another session, **THEN** the new session and response use the inherited profile.

## Out of scope

- Changing workflow, step, explicit, or inherited profile precedence.
- Changing `create_task_kandev`. Its workflow-default response behavior is already covered by the MCP-created task profile spec.
- Changing session persistence, executor selection, or profile validation.

## Implementation plan

See [`../../plans/spawn-session-effective-profile/plan.md`](../../plans/spawn-session-effective-profile/plan.md).
