---
status: draft
system: office
requirements:
  - REQ-OFFICE-RUNTIME-001
created: 2026-05-04
owners:
  - cfl
---
# Office Agent Runtime — Error Handling Contract System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-RUNTIME-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-RUNTIME-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Scenarios

- **GIVEN** an Office agent run, **WHEN** the adapter publishes an error event for any reason (auth failure, invalid model, malformed response, transient network), **THEN** the session transitions to `FAILED`, the wakeup row is stamped `status=failed` with the raw payload in `error_message`, no follow-up wakeup is queued, and `consecutive_failures` increments by one.
- **GIVEN** a `FAILED` session, **WHEN** the topbar / chat header re-renders, **THEN** the "Agent working for Xs" timer and spinner are not displayed because `isLiveSession` reads `FAILED` as non-live.
- **GIVEN** a failed run, **WHEN** the user opens the per-task chat, **THEN** a structured error entry appears at the failure timestamp with header "The agent stopped with an error.", a `Show details` collapsible revealing the raw adapter payload, and Resume session + Start fresh action buttons.
- **GIVEN** an agent whose `consecutive_failures` reaches its effective `failure_threshold`, **WHEN** the failing wakeup is recorded, **THEN** the agent is auto-paused (`pause_reason` set), the threshold-th failure does NOT create its own `agent_run_failed` entry, and any prior `agent_run_failed` entries for that agent are auto-dismissed and replaced with a single `agent_paused_after_failures` entry listing the affected tasks.
- **GIVEN** an auto-paused agent, **WHEN** the scheduler tick attempts to claim one of its wakeups, **THEN** the wakeup is not claimed; behaviour is identical to a manually-paused agent.
- **GIVEN** an `agent_run_failed` inbox entry, **WHEN** the user clicks Mark fixed, **THEN** an `inbox_dismissals` row is inserted, the (task, agent) session is cleared from `FAILED` to `IDLE`, and a new wakeup is queued with reason `manual_resume_after_failure`.
- **GIVEN** an `agent_paused_after_failures` inbox entry, **WHEN** the user clicks Mark fixed, **THEN** the agent's `pause_reason` clears, `consecutive_failures` resets to zero, and a wakeup with reason `manual_resume_after_failure` is queued for every (task, agent) listed on the entry.
- **GIVEN** a failed task assigned to an agent, **WHEN** the user reassigns the task to a different agent, **THEN** the prior (task, old agent) wakeup is cancelled by the staleness check, the (task, old agent) `agent_run_failed` inbox entry auto-dismisses, the old agent's `consecutive_failures` counter is NOT reset, and a fresh `task_assigned` wakeup queues for the new agent.
- **GIVEN** an agent runtime call without a valid `Bearer` token, **WHEN** the request hits any `/runtime/*` endpoint, **THEN** the response is `401 Unauthorized` with `{"error": "missing runtime token"}` or `{"error": "invalid runtime token"}`.
- **GIVEN** an agent run with `Capabilities.CanCreateSubtasks = false`, **WHEN** the agent calls `POST /runtime/tasks/:id/subtasks`, **THEN** the response is `403 Forbidden` (`ErrCapabilityDenied`) and a `runtime.denied` event is appended to `office_run_events` with `action=create_subtask`.
- **GIVEN** an agent run scoped to task A, **WHEN** the agent calls `POST /runtime/tasks/B/status`, **THEN** the response is `403 Forbidden` (`ErrTaskOutOfScope`) and a `runtime.denied` event is recorded.
- **GIVEN** an agent run in workspace W1, **WHEN** the agent attempts to modify an agent in workspace W2 via `PATCH /runtime/agents/:id`, **THEN** the response is `403 Forbidden` (`ErrWorkspaceOutOfScope`).
- **GIVEN** an agent runtime call to `GET /runtime/memory/workspaces/{ws}/memory/agents/{otherAgent}/...`, **WHEN** the run's `AgentID` does not equal `{otherAgent}`, **THEN** `CanAccessMemory` rejects the request with `ErrCapabilityDenied`.
- **GIVEN** the runtime action surface succeeds (e.g. `POST /runtime/comments`), **WHEN** the response returns `201`, **THEN** a `runtime.action` event is appended to `office_run_events` with `action=post_comment`, `target_type=task`, and the affected `target_id`.
- **GIVEN** an agent JWT with capability snapshot frozen at run-claim time, **WHEN** the agent's profile permissions are revoked mid-run, **THEN** the in-flight JWT continues to authorize until expiry; the revocation only affects newly issued JWTs.
- **GIVEN** the backend restarts while an Office session is `FAILED`, **WHEN** the user clicks Resume session post-restart, **THEN** the `executors_running` row is reused, the ACP session id is restored, and the agent resumes into the same workspace / worktree.

## Out of scope

- No per-adapter error classification, no rate-limit reset parsing in this spec. Generic terminal treatment for every error.
- No "Retry now" button. Resume session / Mark fixed cover manual retry.
- No automatic agent unpause. Pause only clears via explicit user action (Mark fixed on the inbox entry, or manual unpause from the agent detail page).
- No per-error-code action variant in the chat. Until we classify, every error gets the same Resume / Start fresh affordance.
- No notification provider integration (Local/OS/Apprise). The existing `office.inbox_item` event from the inbox spec covers it once that work lands.
- The runtime does not recompute the `affected_tasks` list on the `agent_paused_after_failures` entry when tasks are reassigned away. The list is a snapshot at pause time; the user resolves it as a whole.
