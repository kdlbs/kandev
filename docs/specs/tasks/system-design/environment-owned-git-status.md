---
status: draft
system: tasks
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
---

# Environment-Owned Git Status System Design

## Purpose and boundaries

The task environment owns the current workspace. It also owns the current Git
status that desktop and mobile workspace views show. A task session is a
transport handle for status requests and events. It is not a separate owner of
workspace state.

This design covers status hydration, WebSocket delivery, frontend cache
ordering, and the Changes surfaces. It does not change Git polling inside
agentctl, Git operations, commit history, or workspace materialization.

The workspace-path repair in PR #3167 prevents a resumed session from keeping
an incorrect workspace path. This design adds a separate boundary. It prevents
historical or delayed sibling-session data from becoming authoritative after a
task environment already has a canonical workspace.

## Requirement mapping

| Requirement                                         | Design section                                                                                                                      |
| --------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.5` | [Backend status authority](#backend-status-authority), [Frontend ordering](#frontend-ordering), [Workspace views](#workspace-views) |

## Backend status authority

`buildSessionDataProvider` and `buildSessionGitDataProvider` in
`apps/backend/internal/backendapp/helpers.go` accept a session ID because the
WebSocket protocol subscribes to sessions. Before either provider reads live
or persisted Git status, it resolves the session's `TaskEnvironment` and the
all sessions bound to that environment. This includes inherited or shared
sessions whose task differs from the environment owner's task.

The provider selects a source session with these rules:

1. If the requested session has no task environment, keep the legacy
   session-scoped behavior.
2. Read the canonical workspace path from
   `task_environments.workspace_path`.
3. Keep only sessions that reference the same task environment and whose raw
   recorded workspace path is present and exactly matches the canonical path.
   The projected environment path cannot prove that a historical session was
   recorded against the canonical workspace.
4. Prefer a matching live execution. Its workspace path must match the
   canonical environment. A recovered execution with an empty task-environment
   ID is accepted only after the session binding and workspace checks pass. A
   non-empty mismatched ID is rejected before the provider asks agentctl for
   status.
5. If no live query succeeds, load the latest ranked snapshot for each
   matching source session and repository, then select the newest observation
   for each repository.
6. Publish the selected status with the requested subscription session ID so
   existing WebSocket routing and environment mapping remain compatible. A
   multi-repository workspace publishes one event per repository.

The provider does not use a clean status, a non-empty file list, or a primary
session flag as authority. A clean canonical workspace is valid. Session
selection uses workspace identity, and snapshot selection uses observation
time only after the workspace identity matches.

The implementation uses environment-bound session queries,
`GetTaskEnvironment`, and a per-session/per-repository snapshot query. It does
not add a database table or copy Git status onto the task-environment row.

## Frontend ordering

`session.git.event` continues to carry a session ID. The session-runtime store
continues to map that ID through `environmentIdBySessionId` and to cache status
under `gitStatus.byEnvironmentId` and `gitStatus.byEnvironmentRepo`.

For each environment and repository key, `setGitStatus` keeps the newest valid
status timestamp it has observed. It rejects an incoming entry with an older
valid timestamp. If an entry has the same Git content and a newer timestamp,
the store advances the timestamp watermark without invalidating the cumulative
diff cache. This prevents an older hydration response from winning only because
it arrived later.

An entry with no valid timestamp cannot replace an entry that already has a
valid timestamp. This rule keeps a legacy snapshot with unknown age from
replacing a dated live observation. Existing undated state can still accept a
new entry, which preserves compatibility during initial hydration.

Timestamp ordering is a safeguard, not the source-authority rule. The backend
must first exclude data from a different workspace.

## Workspace views

Desktop and mobile Changes surfaces use the same environment-keyed store and
Git-status hooks. This change does not add controls, change layout, or alter
touch behavior. Both surfaces receive the same corrected state without separate
presentation logic.

The desktop end-to-end regression uses two sessions in one environment. It
creates an uncommitted file, reloads the task, and proves that the file stays
visible after both session hydrations and a sibling-session switch. A focused
store test proves that an older sibling event cannot replace newer environment
state. No separate mobile end-to-end test is required because this design does
not change mobile composition or interaction, and the mobile surface reads the
same store contract.

## Failure and recovery

If environment or sibling-session lookup fails, the provider logs the failure
and does not publish an unverified session snapshot. The next Git poll, focus
refresh, reconnect, or explicit refresh can retry. The provider keeps legacy
session behavior only when the session has no task-environment identity.

If a live status request times out, the provider can use only a snapshot whose
source session matches the canonical environment workspace. If none exists, it
returns no status rather than clearing the current frontend state with suspect
data. The live probe has one deadline for all eligible sources, so a stuck
sibling cannot extend hydration once per source.

A corrected session path from PR #3167 makes that session eligible on its next
hydration. No database migration or historical snapshot rewrite is required.

## Persistence

Git snapshots remain session-attributed history in
`task_session_git_snapshots`. The source session preserves the audit trail.
Environment authority is resolved when current status is projected. This keeps
historical records intact and avoids a second status ownership model. Live
monitor snapshots are retained one per session and repository so fallback can
rebuild a multi-repository status.

## Observability

Backend logs for live-query and snapshot fallback include the requested session
ID, selected source session ID, task-environment ID, and a bounded reason when a
candidate is rejected. Logs must not include workspace paths.

Frontend debug logs include the environment key, repository name, incoming and
stored timestamps, and whether ordering rejected the event. Existing file-count
and cache-invalidation fields remain.

## Verification

- Backend tests seed same-environment sessions, including an inherited session
  from another task, with different workspace paths. They prove that live and
  snapshot hydration select only canonical sources, preserve one event per
  repository, and route results to the requested subscription.
- Frontend tests deliver a newer dirty status and then an older clean status
  from a sibling session. They prove that the environment cache stays dirty and
  the cumulative diff cache is not invalidated by the rejected event.
- The desktop end-to-end test proves that uncommitted files remain visible
  through reload and sibling-session hydration.

## Related decisions

- [Keep Worktree Ownership at the Task Lifecycle](../../../decisions/2026-08-08-task-owned-worktree-lifetime.md)
- [Project Current Git Status From the Task Environment](../../../decisions/2026-08-30-environment-owned-git-status.md)
