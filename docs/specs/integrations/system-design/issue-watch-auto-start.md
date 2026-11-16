---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001
---

# Issue-watch auto-start system design

## Purpose and boundaries

The integration dispatch adapter supplies omitted watcher profiles through existing task-owned resolution and launch policy.
Explicit session targets retain authority to resolve their source-step profile.
The adapter also records genuine launch failures through the existing task marker.
This design reuses task-owned services. It introduces no new resolution policy or persistence boundary.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| `AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.1` through `.4` | Profile selection |
| `AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.5`, `.8` | Failure visibility |
| `AC-INTEGRATIONS-ISSUE-WATCH-AUTO-START-001.6`, `.7` | Dispatch and admission |

## Components

- `WatcherDispatchCoordinator.Dispatch` owns reservation, creation, task association, and eligibility checks in `internal/orchestrator/watcher_dispatch.go`.
- `serviceTaskStarter.Start` adapts that pipeline to `Service.StartTask` in `watcher_dispatch_wiring.go`.
- `Service.resolveTaskAgentProfile` in `session_ensure.go` composes the existing `internal/task/agentprofile.Resolve` inputs.
- `Service.StartTask` retains workflow routing, profile validation, admission, and executor preparation.
- `Service.setTaskAutoStartFailedMarker` persists and publishes the existing task failure status.

## Profile selection

The adapter keeps a nonempty `AutoStartParams.AgentProfileID` as its caller input.
`StartTask` continues to apply workflow overrides under its existing policy.
The adapter must not mark watch inputs as `ProfileExplicit` selector intent.

When that input is empty, the adapter loads the persisted task and calls `resolveTaskAgentProfile`.
This reuses step replacements, explicit-target exclusions, workflow defaults, task metadata, assignee, and workspace fallback.
Missing tasks and repository errors return contextual errors. When the effective step has an explicit `SessionTarget`, the adapter passes an empty profile to `StartTask`, even if task or workspace fallback selected one. This lets target routing resolve and validate its source profile before profile validation. If that route cannot resolve a profile, the existing launch path returns `executor.ErrNoAgentProfileID` before creating a session.
An unavailable nonempty profile remains subject to existing validation and deleted-profile handling.

The adapter passes `ExecutorProfileID` unchanged. `StartTask` already reads saved executor metadata.
`Executor.resolveExecutorConfig` already resolves an omitted executor from its existing workspace or implicit selection.
The adapter must not synthesize a profile ID from a bare executor ID.

Profile resolution retains the dispatch context and owner. It does not rewrite the watch or task's profile metadata.
`StartTask` remains responsible for dynamic routing and admission after profile selection.

## Dispatch and admission

Keep `WatcherSource`, `AutoStartParams`, task creation, and task association unchanged.
The coordinator continues to use the persisted task description and effective step.
It skips automatic start for overflow-queued tasks and steps without `auto_start_agent`.

The adapter passes `autoStart=true` to `StartTask`. Session ceilings therefore remain effective.
On `ErrCeilingLaunchDeferred`, retain the existing durable queue and skip failure marking.
On other errors, return the original error to the coordinator for its structured log.
Retain the created task and its deduplication row. Do not release, recreate, disable, or automatically retry it.

## Failure visibility

For resolution and launch errors, call `setTaskAutoStartFailedMarker` before returning the error.
The helper conditionally writes `models.MetaKeyAutoStartFailed` only when no session has active work.
This guard applies across all task sessions, including a mixed set with failed and active sessions.
It then reloads the task and publishes `task.updated`. Do not replace the metadata map.

The existing task DTO, boot payload, WebSocket merge, and kanban status icon consume this marker.
`clearTaskAutoStartFailedMarker` already removes it when a session reaches `STARTING` or `RUNNING`.
Keep that lifecycle and its active-session guard. Marker persistence errors retain their existing warning logs.

The coordinator's error log retains the launch cause. The task card exposes the existing generic failure indicator.
This design does not persist raw provider errors or change a watch's poll-owned `last_error`.

## Desktop and phone presentation

Reuse `apps/web/components/kanban-card-status-icon.tsx` and the existing localized indicator without markup changes.
The nearest phone surface is the focused kanban column in `components/kanban/mobile-column-tabs.tsx`.
Desktop and phone checks observe the same task marker. No composition or touch behavior changes.
An ASCII UI preview is unnecessary because this package changes only the marker's backend producer.

## Compatibility and persistence

Sentry, Linear, Jira, GitLab issue, and GitLab review sources share this adapter.
Their event translation and explicit inputs retain their current shapes.
Legacy GitHub and Azure DevOps paths do not receive a new fallback through this change.

The marker uses existing atomic task metadata and event publication. No migration, setting, dependency, or metric is necessary.
Omitted watch profiles resolve at dispatch time. Existing failed tasks require an explicit start after configuration repair.

## Verification

Drive the production adapter with the SQLite test repository and real scheduler/executor preparation.
Include a Sentry source event through the coordinator. A test of `agentprofile.Resolve` alone cannot prove adapter wiring.
Add focused adapter cases for default selection, configured profiles, and failure marking. Cover an explicit target to an earlier fixed-profile step with no existing session and an unusable workspace fallback; the source profile must launch and the task must remain unmarked.
Reuse existing resolver, coordinator, and marker lifecycle tests for unchanged behavior. Include capacity deferral and a mixed-session marker guard.

Use the existing Sentry mock endpoints and watch trigger for browser coverage.
Assert a session exists with the workspace profile before opening any task route.
Generate a genuine missing-profile failure to prove marker publication and rendering on desktop and phone.

## Related task contracts

- [Fixed-profile routing](../../tasks/system-design/workflow-step-fixed-profile-routing.md)
- [Workflow profile session lifecycle](../../tasks/system-design/workflow-profile-session-lifecycle.md)
- [Session ceiling](../../agents/requirements/session-concurrency-ceiling.md)
- [Task-state failure presentation precedent](../../tasks/system-design/orphaned-workspace-task-indicator.md)
