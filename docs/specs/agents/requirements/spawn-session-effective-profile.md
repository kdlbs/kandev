---
status: active
system: agents
created: 2026-08-12
owners:
  - kandev
---
# Spawn Session Effective Agent Profile Requirements

## Overview

Agents use `spawn_session_kandev` to start another session with a selected agent profile. The tool must report the profile that the new session uses.

## Requirements

### REQ-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001: Spawn Session Effective Agent Profile

**Intent:** Agents use `spawn_session_kandev` to start another session with a selected agent profile. The tool must report the profile that the new session uses.

#### Acceptance criteria

- **AC-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001.1:** `spawn_session_kandev` reports the effective agent profile for each successful launch.
- **AC-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001.2:** The reported profile equals the profile persisted and launched on the distinct new session.
- **AC-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001.3:** An explicit spawn bypasses workflow session-target reuse. A non-empty resolved spawn profile wins over workflow step pins, defaults, and session targets.
- **AC-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001.4:** Resolve the spawn profile from the supplied ID, then a verified same-task caller session, then the target primary session. If none resolves, preserve normal launch defaults or errors without reusing a workflow-targeted session.
- **AC-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001.5:** Without an explicit profile, the current inheritance rules remain unchanged.
- **AC-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001.6:** The tool description explains the precedence and the response field.
- **AC-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001.7:** **GIVEN** an unpinned workflow step with workflow default A, **WHEN** a caller requests profile B, **THEN** a distinct new session and response use profile B.
- **AC-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001.8:** **GIVEN** a workflow step pinned to profile A, **WHEN** a caller requests profile B, **THEN** a distinct new session and response use profile B.

- **AC-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001.9:** **GIVEN** existing source session S and a step targeting that source with reuse, **WHEN** an explicit MCP spawn succeeds, **THEN** it returns a persisted and launched session N != S and adds exactly one session. Naming N must not rename S or replace the recorded workflow route.
- **AC-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001.10:** **GIVEN** the same reuse target, **WHEN** an ordinary workflow launch runs without explicit spawn intent, **THEN** it may still reuse S.

## Migrated source detail

## Why

Agents use `spawn_session_kandev` to start another session with a selected agent profile. The tool must report the profile that the new session uses.

Without this guarantee, a caller can receive confirmation for one profile while a workflow profile starts instead.

## What

- `spawn_session_kandev` reports the effective agent profile for each successful launch.
- The reported profile equals the profile persisted and launched on the distinct new session.
- An explicit spawn bypasses workflow session-target reuse. A non-empty resolved spawn profile wins over workflow step pins, defaults, and session targets.
- Resolve the spawn profile from the supplied ID, then a verified same-task caller session, then the target primary session. If none resolves, preserve normal launch defaults or errors without reusing a workflow-targeted session.
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

`agent_profile_id` identifies the actual profile persisted and launched on the new session. The response remains authoritative rather than merely echoing the request.

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

- **GIVEN** no resolved caller or primary profile and a workflow session target, **WHEN** no normal launch default is available, **THEN** spawn returns a launch error and never starts or renames the existing target session.
- **GIVEN** no resolved caller or primary profile and an ordinary step with a valid launch default, **WHEN** spawn succeeds, **THEN** it creates a distinct session with that default.
- **GIVEN** a reusable source session with the same requested profile, **WHEN** spawn succeeds, **THEN** session identity is still distinct.

- **GIVEN** an unpinned workflow step with workflow default A, **WHEN** a caller requests profile B, **THEN** a distinct new session and response use profile B.
- **GIVEN** a workflow step pinned to profile A, **WHEN** a caller requests profile B, **THEN** a distinct new session and response use profile B.
- **GIVEN** no workflow launch profile, **WHEN** a caller requests profile B, **THEN** the new session and response use profile B.
- **GIVEN** no explicit profile, **WHEN** a same-task session spawns another session, **THEN** the new session and response use the inherited profile.

## Out of scope

- Changing ordinary workflow launches, which retain workflow profile precedence and intentional session reuse.
- Office scheduler ownership, profile validation, or live deployment changes.
- Changing `create_task_kandev`. Its workflow-default response behavior is already covered by the MCP-created task profile spec.
- Changing session persistence, executor selection, or profile validation.

## Implementation plan

The original effective-response work remains recorded in [`spawn-session-effective-profile`](../../../plans/spawn-session-effective-profile/plan.md). The distinct-session repair is recorded in [`explicit-mcp-spawn-distinct`](../../../plans/explicit-mcp-spawn-distinct/plan.md). Its explicit-spawn precedence supersedes the original workflow override scenarios; effective-response projection remains required.
