# Session Resumption LLD

## Goals
- Support multiple active sessions per task.
- Resume sessions after backend restart without relying on user navigation.
- Distinguish resumable vs non-resumable agents (no `resume_token`).
- Keep session state consistent across backend, agentctl, and UI.
- Support local and remote executors.

## Key Assumptions
- A session is the unit of execution; each session has at most one active agent execution.
- Task can have multiple sessions concurrently (active or historical).
- Agents that support session resumption provide a `resume_token` in session metadata.
- Agents that do not support resumption cannot resume after restart.
- Remote executors keep running across backend restarts; the backend must reconnect.

## Data Model (existing + additions)
- `task_sessions`
  - `id`, `task_id`, `state`, `agent_profile_id`, `agent_execution_id`, `metadata` (includes `resume_token`)
  - `worktree_id`, `worktree_path`, `worktree_branch`
  - `executor_id` (local/remote)
- `executors_running` (persistent table)
  - `id` (executor instance id), `session_id`, `task_id`, `runtime`
  - `agentctl_url`, `agentctl_port`, `container_id` (docker), `pid` (local)
  - `status`: `starting | ready | error | stopped`
  - `supports_session_resume`, `supports_workspace_only`
  - `resume_token`
  - `workspace_path`, `worktree_id`, `worktree_path`, `worktree_branch`
  - `last_seen_at`, `error_message`
- `executors_cache` (in-memory only)
  - `execution_id`, `session_id`, `runtime`, `status`, `agentctl_url/port`, `workspace_path`

Note: agentctl state is derived from `executors_running.status` and health checks.

## Agent Capability
- Add `supports_session_resume` boolean to agent registry or agent profile.
- When `false`, mark session as non-resumable on backend restart.
- UI should show: “This agent cannot resume previous sessions. Create a new session.”

## Executor Types
- **Local Docker**: containers survive backend restart; recover via Docker APIs.
- **Local Standalone**: agentctl processes are local and typically *do not* survive restart.
- **Remote Executor**: agentctl runs remotely; survives backend restart; backend must reconnect.

## Startup Flow (Backend)
Goal: recover agentctl instances from `executors_running` on backend start, not when a user opens a session.

1) **Load executors**
   - Query `executors_running` where `status` in (`starting`, `ready`).

2) **Reject non-resumable executors**
   - If `supports_session_resume=false`, mark related sessions `ERROR` and set `error_message=not_resumable`.

3) **Resume executors by runtime**
   - **Local Docker**:
     - Verify Docker daemon running.
     - Locate containers by `executors_running.container_id` or session label.
     - If container missing: mark executor `error`, mark sessions `ERROR`.
     - If container running: healthcheck agentctl.
   - **Local Standalone**:
     - Verify worktree path exists.
     - If missing: mark executor `error`, mark sessions `ERROR`.
     - Start agentctl process and healthcheck.
   - **Remote Executor**:
     - Call `Health()` on `agentctl_url`.
     - If unreachable: mark executor `error`, mark sessions `ERROR`.

4) **Sync executor cache**
   - Register running executions in `executors_cache` keyed by `session_id`.
   - Update `executors_running.last_seen_at`.

5) **Enable workspace access**
   - If agentctl is healthy, set `executors_running.status=ready`.
   - Shell/fs/git are enabled regardless of agent resume.

6) **Resume agent session**
   - If `supports_session_resume=true` and `resume_token` present, resume agent.
   - On success: set `session.state = WAITING_FOR_INPUT`.
   - On failure: set `session.state = ERROR`, update `error_message`.

7) **Publish updates**
   - Emit session agentctl events to UI.

## Executor Resumption Capability
Executors should report whether agent session resume is supported by the runtime + agent, so the backend can decouple executor lifecycle from agent lifecycle.

### Agentctl Capability Probe
After agentctl is reachable, call a capability endpoint (new or existing) to determine:
- `supports_session_resume`: whether the agent can resume with the session resumption token.
- `supports_workspace_only`: whether workspace-only mode is available (for git/fs/shell without agent).

Note: `requires_new_session` is redundant with `supports_session_resume=false`. If we keep it, it must be a derived/alias field, not an independent signal.

This allows:\n
- Executor resume (agentctl + workspace streams) even if agent resume is not possible.
- The UI to enable shell/git/files regardless of agent resumption outcome.

## Session Resumption Flow (Backend)
Triggered when user explicitly requests resume.

1) Validate session:
   - Session exists and belongs to task.
   - Agent profile supports resume and session has `resume_token`.
   - Worktree exists if required.

2) Create agentctl instance (local only):
   - Call `LaunchAgent` with `session_id`, `worktree_path`, `worktree_branch`.
   - Persist `executors_running.status=starting`.

3) Start agent process and resume:
   - Resume agent session with `resume_token`.
   - On success:
     - `session.state = WAITING_FOR_INPUT`
     - `executors_running.status = ready`
     - `task.state = REVIEW`
   - On failure:
     - `session.state = ERROR`
     - `executors_running.status = error`
     - Persist `error_message`.

## Session Status API
### `task.session.status`
Return session-specific status (no task-level fallback):
- `session_id`, `task_id`, `state`, `agent_profile_id`
- `executor_status`, `agent_execution_id`, `runtime`
- `is_agent_running` (based on execution for that session)
- `is_resumable` (resume supported + `resume_token` present)
- `needs_resume` (no running execution + resumable)
- `resume_reason` (e.g., `agent_not_running`, `not_resumable`, `missing_resume_token`)
- `worktree_path`, `worktree_branch`

## WebSocket Events
Session-scoped events:
- `session.agentctl_starting`
- `session.agentctl_ready`
- `session.agentctl_error`
- `session.state_changed`
- `session.message.added`, `session.message.updated`

Payloads must include `session_id` and `task_id`.

## Frontend Flow
1) **SSR page** loads session + task + messages and hydrates store.
2) **useSessionResumption**
   - Calls `task.session.status`.
   - If `needs_resume && is_resumable`: show “Resume session” CTA or auto-resume (configurable).
   - If `!is_resumable`: show “Create new session” CTA.
3) **UI states**
   - `executor_status=starting`: show spinner and disable shell/file panels.
   - `executor_status=ready`: allow shell/file access.
   - `executor_status=error`: show error badge and retry/resume action.

## Multi-Session per Task Considerations
- Executor and lifecycle manager must key executions by `session_id` (done).
- `task.session.status` must only check `execution.SessionID == sessionID`.
- WebSocket subscriptions should be per-session to avoid cross-session updates.

## Non-Resumable Agents
- Add `supports_session_resume=false` in agent registry/profile.
- Backend returns `is_resumable=false` in status.
- UI displays a banner: “This agent doesn’t support resuming. Start a new session.”

## Open Decisions
- Whether to keep `executors_running` lean or store full runtime metadata.
- Auto-resume on backend startup vs on-demand (current request: startup).
- Whether to auto-resume all resumable sessions or only those in RUNNING/WAITING_FOR_INPUT.
- Remote executor reconnect timeout/backoff policy.

## Resumption Flow Diagrams

### Local Docker Executor (agentctl container survives)
```
Backend Restart
  |
  |-- load executors_running (starting/ready)
  |-- docker: RecoverInstances()
  |        -> map container to executors_running.id or session_id label
  |-- agentctl Health()
  |-- agentctl Capabilities? (supports_session_resume)
  |-- register AgentExecution(session_id, runtime=local_docker)
  |-- executors_running.status = ready  (workspace-only enabled)
  |
  |-- if supports_session_resume & resume_token present:
  |       ResumeSession
  |       session.state = WAITING_FOR_INPUT
  |   else:
  |       session.state = ERROR (not_resumable)
  |
  '-- WS events to UI:
        session.agentctl_ready / session.state_changed
```

### Local Standalone Executor (agentctl does NOT survive)
```
Backend Restart
  |
  |-- load executors_running (starting/ready)
  |-- no local instances to recover
  |-- LaunchAgent from executors_running (workspace-only)
  |-- executors_running.status = starting
  |-- agentctl Health()
  |-- agentctl Capabilities? (supports_session_resume)
  |-- executors_running.status = ready  (workspace-only enabled)
  |
  |-- if supports_session_resume & resume_token present:
  |       ResumeSession
  |       session.state = WAITING_FOR_INPUT
  |   else:
  |       session.state = ERROR (not_resumable)
  |
  '-- WS events to UI:
        session.agentctl_starting / ready / error
```

### Remote Executor (agentctl survives remotely)
```
Backend Restart
  |
  |-- load executors_running: agentctl_url, id
  |-- connect to agentctl Health()
  |-- agentctl Capabilities? (supports_session_resume)
  |-- register AgentExecution(session_id, runtime=remote)
  |-- executors_running.status = ready  (workspace-only enabled)
  |
  |-- if supports_session_resume & resume_token present:
  |       ResumeSession
  |       session.state = WAITING_FOR_INPUT
  |   else:
  |       session.state = ERROR (not_resumable)
  |
  '-- WS events to UI:
        session.agentctl_ready / session.state_changed
```

## Executor vs Agent Resumption
Separating executor resume from agent resume allows:
- Shell/git/files to function as soon as agentctl is reachable.
- Agent resume to fail gracefully without losing workspace access.

Example UI behavior:
1) `session.agentctl_ready` -> enable shell/git/files panels.
2) If `supports_session_resume=false` -> show “Create new session to continue messaging.”
3) If resume fails -> show error but keep workspace panels active.
