# Workspace orchestrators

Status: implemented and verified in the isolated preview. The user approved replacing the Office-facing integration with a first-class workspace orchestration concept.

Scope clarification (2026-09-17): the implemented linked-task list is not a central
workspace task overview. That newly accepted direction has a separate pending
[requirement/design package](../orchestration/README.md) and
[implementation plan](../../plans/workspace-coordinator-view/plan.md). This legacy
document continues to own the existing coordinator foundation.

## Behavior
A workspace has any number of orchestrator assignments, each named by its global role. Each selects a global role, an existing enabled execution profile and an executor, and workspace context. Roles contain names and behavioral instructions, not models or credentials. Global Settings > Orchestration manages global roles; workspace settings manage instances. Editing a role updates all assignments on their next turn. Names, icons and instructions are configured only on the global role. Deleting a role in use is rejected.

Each orchestrator has its own persistent conversation, pause/resume/delete controls and configuration. Conversations show chat and execution feedback, not delivery task status, labels, project, blockers, reviewers or approvers. Navigation links workspace cards, workspace overview/detail tabs, sidebar/mobile navigation, settings search, role settings, orchestrator configuration and conversations. Normal task pages link back to their coordinating orchestrator. Setup follows Kanban onboarding; profile selection uses existing agents and never requires duplicate worker personas.

Delivery tasks remain canonical Kanban tasks. The coordinator reads available execution profiles, adopts existing tasks without changing their assigned agent, and explicitly assigns a profile for new work. Existing task assignments are honored on start/resume; intentional user/profile changes are supported. No automatic provider fallback crosses profiles. Task events notify only their coordinating orchestrator, including multiple orchestrators in one workspace. Stable external task IDs prevent duplicate creation. Scope validation applies to workspace, profile, task and conversation.

## Persistence and API
Own persona instructions, memory, conversation mappings, runtime and API in `internal/orchestration`. Reuse core execution profiles, tasks, comments, sessions, runtime authentication and run dispatch. Transfer pre-separation registered personas once, preserving IDs/history; do not continue reading Office storage. Add durable global roles and workspace orchestrator registrations, with workspace/agent deletion cleanup. Expose /api/v1/orchestration/roles and /api/v1/orchestration/workspaces/:wsId/orchestrators for list/create/update/delete and conversation open. Orchestration owns its scoped runtime and conversation endpoints.

## Experimental rollout
features.orchestration / KANDEV_FEATURES_ORCHESTRATION defaults off in prod/dev/e2e. It is independent of the legacy Office product flag. Disabled routes and jobs must not launch orchestrators; saved data remains intact. Existing Office configuration/history remains compatible. Enable only the isolated preview for verification.

## Acceptance
Create two orchestrators in one existing workspace with different execution profiles; no extra workflow/workspace; navigate to each from list/detail/sidebar; configure a reusable role; converse without issue-property UI; delegate directly to a normal execution profile and observe the matching task; preserve explicit agent changes; disabled feature hides/rejects its entry points; mobile and desktop share functionality without clipped controls.


## Standalone runtime boundary

Orchestration runs with Office disabled and with no Office tables in its runtime fixture. The full Go dependency closure of `internal/orchestration/...` contains no Office package. The browser uses its own conversation routes, APIs, locale namespace and comment/retry transports, with core session updates and shared chat rendering. Office routes reject registered personas and conversations even when Office is enabled.

Each turn receives a fresh core runtime credential scoped to its persona, workspace, run and prepared session. Finished runs cannot mutate resources. Callback runs are durable and idempotent; streamed turn messages bridge to the existing conversation. Explicit retry launches with fresh credentials. Before-result transient failures also receive up to five durable retries (15/30/60/60/60 seconds), each through the same authorized launch path. The execution owner decides retry before terminal session UI; raw failure subscribers cannot finish the run first. Retry claims are conditional on run, session, status and retry count; Stop/Cancel retires queued retries. Later conversation turns cannot overtake recovery. Output, tool effects, unknown evidence and permanent failures prohibit automatic replay. Restart marks interrupted runs failed and never automatically replays uncertain external writes. Core stale-run recovery excludes Orchestration profiles.

Migration transfers instructions, memory and conversation mappings transactionally and records a per-persona completion marker. Later Office inserts cannot leak in, and deleted Orchestration memory cannot be resurrected. Explicit assistant import performs the same transfer immediately. Default prompt guidance and customized role policies remain separate.
