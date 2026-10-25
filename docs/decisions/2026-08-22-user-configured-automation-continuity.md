# ADR-2026-08-22: Let Users Configure Continuity, Not MCP Authority

**Status:** accepted (amended by ADR-2026-09-02-automation-self-archive)
**Date:** 2026-08-22
**Area:** backend, agentctl, frontend, protocol, security, workflow
**Related ADRs:** [MCP tool profiles](2026-08-08-mcp-tool-profiles.md), [task-owned worktree lifetime](2026-08-08-task-owned-worktree-lifetime.md), [live agent permission authority](2026-08-11-live-agent-permission-authority.md)
**Related specs:** [Automations in Settings](../specs/office/automations-settings.md), [Automation Runs](../specs/office/automation-runs.md)

## Context

Each automation firing currently creates a task, session, and task-owned worktree. This is a good
default for independent reports, but it is expensive and semantically wrong for heartbeat agents
that repeatedly inspect and coordinate the same workspace. Those agents benefit from one durable
conversation and worktree across scheduled firings.

Coordinator agents also need Kandev administration operations. Pending-question and live permission
tools already exist on the authenticated external MCP surface, but ordinary task sessions do not
receive them. The current external surface is not a suitable template: it includes workflow, agent,
executor, and MCP configuration mutation plus permanent task deletion, but omits coordinator
actions such as messaging and restarting task sessions.

## Decision

1. An automation stores a user-selected `continuation_policy`:
   - `new_task` is the default and preserves the current behavior;
   - `reuse_thread` reuses one automation-owned task, primary session, task environment, and
     worktree across firings.
2. Every accepted or skipped firing still creates a distinct `AutomationRun`. A dispatched run is
   bound to its exact `task_id`, `session_id`, and `turn_id`. Completion and summary projection use
   the turn identity, never "the latest run for this task."
3. `reuse_thread` permits one scheduled turn at a time. Its effective and persisted
   `max_concurrent_runs` is 1, enforced by backend validation and atomic admission. The UI explains
   and locks that value while the policy is selected.
4. The automation stores its current continuation task as runtime state. The first firing creates
   it. Later firings prompt its primary session through the normal resumable task path. If the task,
   session, runtime, or task environment cannot be resumed, Kandev creates and binds a replacement
   thread, runs the firing there, and records `created`, `resumed`, or `replaced` plus a safe reason
   on the run.
   The agent remains responsible for its normal in-session context compaction. Kandev does not
   rotate a healthy thread because of age or turn count. The non-native fallback resume prompt uses
   only the newest 50 non-empty user or assistant text messages. Tool calls and tool results do not
   appear in the prompt and do not consume the limit. Kandev keeps the selected messages in
   chronological order and retains the existing per-message truncation. The new automation prompt
   appears separately as the current request and does not consume the limit.
5. The create and edit forms expose the continuation choice directly. The section and both choices
   have persistent help text that explains conversation, files, isolation, and single-run behavior.
   Changes to launch identity rotate the continuation on the next firing. Launch identity includes
   agent profile, executor profile, repositories, workflow, and workflow step. Name, description,
   prompt, and trigger changes do not rotate it. The next prompt uses the newly saved value.
6. Add `SurfaceAutomation` as a backend-owned MCP base surface. Every automation session receives
   the same versioned tool profile; there is no per-automation MCP checkbox list or arbitrary tool
   allowlist.
7. The automation surface contains workspace discovery, task discovery, task creation and
   coordination, session recovery, pending-question resolution, and live permission resolution:
   `list_workspaces_kandev`, `list_workflows_kandev`, `list_workflow_steps_kandev`,
   `list_repositories_kandev`, `list_tasks_kandev`, `list_agents_kandev`,
   `list_executors_kandev`, `list_executor_profiles_kandev`, `list_related_tasks_kandev`,
   `get_task_conversation_kandev`,
   `list_task_sessions_kandev`, `create_task_kandev`, `update_task_kandev`, `move_task_kandev`,
   `archive_task_kandev`, `add_task_dependency_kandev`, `remove_task_dependency_kandev`,
   `message_task_kandev`, `stop_task_kandev`, `spawn_session_kandev`,
   `list_pending_questions_kandev`, `answer_question_kandev`,
   `list_pending_agent_permissions_kandev`, and `resolve_agent_permission_kandev`.
8. The surface deliberately excludes workflow/agent/executor/MCP configuration writes, permanent
   task deletion, task-local planning and walkthrough tools, user/parent questions, reviews, rich
   output, branch/source mutation, step completion, title ownership, diagnostics, and provider PR/MR
   automation. Arbitrary plugin tools are not loaded into this base surface. Autonomous
   coordination must not silently become install configuration, external-account authority, or
   permanent data destruction.
9. In-session dispatch resolves one trusted automation MCP principal from the execution's own task
   and session before any tool handler runs. The principal carries the automation ID, workspace ID,
   caller task ID, caller session ID, and surface. One shared authorization boundary constrains
   every included workspace, workflow, task, session, question, permission, and newly-created task;
   handlers do not independently derive owner-wide scope. Missing, malformed, or foreign scope
   fails closed. The agent cannot request another surface or widen it with tool arguments.
10. Permission-resolution audit records identify an automation actor and an `automation_mcp` source.
   The source is derived from trusted in-session transport context. It is not accepted from the
   tool request and does not impersonate an interactive user or the external MCP bridge.
11. The automation's own task and every session on it are invalid targets for task/session
    mutations, messaging, stopping, session spawning, and question or permission discovery or
    resolution. An automation cannot inspect or approve its own blocker through the coordinator
    surface or create concurrent sessions in its reused worktree. A session spawned on another task
    receives the target task's normal MCP profile, never `SurfaceAutomation` inherited from the
    coordinator.
12. The existing `triggered` run status is the durable admitted-but-not-yet-bound state. Both
    `triggered` and `task_created` count as open and display under Running. No duplicate
    `dispatching` status is introduced.
13. Each run stores its own rendered display title. Creating or replacing a thread also uses that
    title for the new task; resuming a thread does not rename the shared task. Run history therefore
    preserves trigger-specific titles without rewriting older entries.
14. Kandev does not reset, rebase, or otherwise rewrite a reused worktree between firings. Agent
    work and local changes remain intact. A replacement thread starts from the repository's current
    configured base branch.
15. Deleting an automation owns cleanup of every distinct hidden task it references, including its
    current continuation. The delete path stops live sessions, removes the automation and run
    references, and reclaims each now-unreferenced task/worktree through normal task cleanup. The
    deletion transaction first inserts one durable `automation_task_cleanup_jobs` row per distinct
    task. Jobs are deleted after cleanup succeeds and otherwise retried by orphan reconciliation.
16. Portable automation export includes `continuation_policy`. It excludes the fixed MCP profile,
    continuation task pointers, and run bindings because they are product/runtime behavior rather
    than per-automation portable configuration.

## Consequences

- Independent automations remain backward compatible and continue to receive a fresh task per run.
- Heartbeat automations retain conversation context and avoid repeated task environment setup.
- A shared transcript remains readable as run history because each run has a durable turn anchor.
- Run deletion and worktree retention must operate on distinct task identities. Deleting one run
  cannot delete a task still referenced by another run or by the automation continuation pointer.
- Automation agents get one predictable, autonomous coordinator surface without settings clutter.
- The question and permission services gain a workspace-bound automation caller path without
  weakening their existing external-client authorization or live-runtime authority.
- Evolving the automation surface is a versioned product decision that requires registry and
  authorization tests; individual users cannot accidentally omit a required discovery half or add
  a dangerous configuration tool.
- A continuation automation cannot run concurrent scheduled turns. Users who need concurrency use
  `new_task`.
- Agent-side compaction bounds provider context; Kandev still retains the durable transcript. This
  is the same persistence policy as ordinary tasks and is not a reason to rotate a healthy thread.
- Tool activity cannot displace conversation from fallback context because only text messages
  consume the 50-message limit.
- Automation deletion becomes a lifecycle operation rather than a single cascading row delete.

## Alternatives Considered

1. **Always reuse the previous task.** Rejected because independent reports need isolated history,
   branches, and failures, and the user explicitly needs control of the behavior.
2. **Reuse only the agent process while creating a new task and worktree.** Rejected because task
   environments own worktrees and conversation continuity belongs to the durable task/session, not
   to a replaceable process.
3. **Create a new session in one task for every firing.** Rejected because it loses working memory
   and still permits concurrent sessions to mutate one shared worktree. Token cost is
   provider-dependent and is not the deciding guarantee.
4. **Give coordinator automations the whole external MCP surface.** Rejected because the external
   surface includes unrelated configuration authority, permanent deletion, and the wrong mix of
   task coordination operations.
5. **Expose per-automation MCP capability checkboxes.** Rejected because pending discovery and
   resolution are core coordinator behavior, not an expert configuration matrix. Checkboxes can
   produce incomplete profiles, increase support states, and imply that unsafe system-configuration
   tools are reasonable opt-ins.
6. **Let the prompt or agent request arbitrary tool names.** Rejected because MCP discovery is a
   backend capability boundary. Agents cannot grant themselves tools.
7. **Fail the firing when a saved thread cannot resume.** Rejected because a deleted or incompatible
   session would permanently jam a heartbeat. A recorded fresh-thread fallback preserves service
   while making the loss of continuity observable.
8. **Rotate every reusable thread after a fixed age or turn count.** Rejected because supported
   agents compact their live context and rotation discards useful working memory without reducing
   retained database history. Kandev bounds only its fallback resume prompt and replaces a thread
   when resume actually fails.
