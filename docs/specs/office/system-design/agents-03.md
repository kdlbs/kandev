---
status: draft
system: office
requirements:
  - REQ-OFFICE-AGENTS-001
created: 2026-04-25
owners:
  - cfl
---
# Office: Agents System Design Part 3

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-AGENTS-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AGENTS-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Persistence guarantees

What survives a kandev backend restart:

- **Agent identity** persists in `agent_profiles` (Office rows: `workspace_id != '' AND deleted_at IS NULL`). Name, role, icon, `reports_to`, Office permissions, budget, concurrency/cooldown settings, skills, instructions, executor preference, and failure policy are durable on the stable identity. Startup profile reconciliation must not replace an Office agent's name with a generated model label.
- **Execution configuration** is referenced, not copied, for Office launches. The resolved `execution_profile_id` supplies the provider CLI, account environment, model, mode, ACP config options, CLI flags and permissions, passthrough behavior, and MCP configuration. Route attempts and sessions retain that ID for audit and restart recovery.
- **Runtime status** persists in `office_agent_runtime` (PK `agent_id`). On restart a `paused` agent stays `paused`, `pause_reason` (e.g. `"budget"`) is preserved, and `last_run_finished_at` is retained so the cooldown guard works across restarts.
- **Reconciliation at startup**: `infra.Reconciler.ReconcileAll` (called once during boot) drops `office_agent_runtime` rows whose `agent_id` no longer exists in `agent_profiles`, deletes `office_channels` and `office_budget_policies` rows that reference removed agents/projects, and seeds default routine triggers for routines without one. Reconciliation is best-effort: any sub-step that errors is logged but does not block boot.
- **Hire requests** persist as `pending_approval` agent rows plus an approval entry in the inbox. A restart mid-hire leaves the approval visible; the user can still approve or reject and the same activation/deletion paths run.
- **Instructions** (`AGENTS.md`, `HEARTBEAT.md`, `SOUL.md`, `TOOLS.md` and any custom files) live in the office DB (`office_agent_instructions`) as the source of truth. The exported copy under `~/.kandev/runtime/<workspace-slug>/instructions/<agentId>/` is regenerated from DB on every session preparation — losing or wiping this directory between runs has no observable effect.
- **Skills** persist in the workspace `skill` table (DB is the source of truth for inline skills; git-sourced skills cache their `file_inventory` in DB and re-clone on demand). Skill content materialized into `<worktree>/<ProjectSkillDir>/<DirName(slug)>/` is ephemeral: it is rewritten at the start of every session and the `kandev-*` patterns added to the common git dir's `info/exclude` are idempotent.
- **System skills** are re-synced from the embedded `//go:embed` set on every boot via `office.service.SystemSkills.Sync`. Removed slugs are deleted, content is upserted, and per-agent `desired_skills` references are preserved across content updates.
- **Run history** (`office_runs`, `office_run_events`, `office_activity_log`, `office_cost_events`) is fully durable. Per-run lookups (`tasks_touched`, costs, events) survive restarts because each row carries `run_id`/`session_id`.
- **Filesystem config** (`workspace/agents/<name>.yml`, `workspace/skills/`, `workspace/routines/`) is a separate snapshot used by `config.ScanFilesystem` for the Sync UI. It is not authoritative: the DB is, and a missing or stale on-disk config never breaks the runtime — at most the Incoming/Outgoing diff is empty or noisy until the user re-syncs.

What does NOT survive a restart:

- **In-flight agent sessions**: the agent subprocess and its agentctl HTTP server are owned by the kandev process; both die when the backend exits. The `TaskSession.ACPSessionID` and `office_runs` row are retained, so the orchestrator's `RecoverInstances` path can resume the session — but the partially-streamed turn at the moment of shutdown is lost unless the underlying CLI itself supports replay.
- **Queued wakeups for capacity-saturated agents**: wakeups that were already persisted survive (they live in the same DB tables as the rest). Wakeups that were only held in memory at the time of shutdown are lost; the originating event (task assign, comment, routine fire) must re-emit to re-queue.
- **Per-run JWTs** (`KANDEV_API_KEY`) and the rest of the agent's environment block are minted fresh per session. Old JWTs are not honoured after restart.
- **Worktrees on disk** belong to the task environment's repository rows. They survive backend restart and deletion of every session; physical cleanup begins only from task/environment lifecycle operations. The worktree GC treats the task-environment inventory as authoritative. Injected `kandev-*` skill directories under a surviving worktree are stale until the next session rewrites them (the clean-slate step at session start handles this).

There are no TTLs on agent rows, runtime rows, instructions, skills, run history, or activity logs. Retention is by user action only.

## Scenarios

### Agent lifecycle

- **GIVEN** a workspace with no agent instances, **WHEN** the user creates a CEO instance (selecting a profile, role=ceo), **THEN** the instance appears in the agents list with status `idle` and the sidebar shows it under "Agents".

- **GIVEN** an existing CLI profile with a custom mode, permission behavior, flags, config options, environment variables, and MCP setup, **WHEN** routing selects it for an Office launch, **THEN** that complete execution profile is used while the Office name, role, instructions, skills, permissions, budget, and history remain unchanged.

- **GIVEN** the same Office agent later resolves from a Codex execution profile to a Claude execution profile, **WHEN** the new route launches, **THEN** the task and Office identity remain stable, Claude receives its own complete runtime configuration, and no Codex-native resume token is sent to Claude.

- **GIVEN** an Office task assigned directly to an Office agent and no existing task session, **WHEN** the task session is prepared, **THEN** the assignee's stable `agent_profile_id` is used before the workspace default so the session can start.

- **GIVEN** a running CEO instance, **WHEN** the CEO determines a task requires a frontend specialist and no suitable worker exists, **THEN** the CEO submits a hire request for a new worker with appropriate skills, and the request appears in the user's inbox as a pending approval.

- **GIVEN** a pending hire approval, **WHEN** the user approves it, **THEN** the new agent activates (status=idle), appears in the org tree under the CEO, and the CEO receives a wakeup notification.

- **GIVEN** a worker instance with `can_create_tasks=true`, **WHEN** the worker creates a subtask exceeding `max_subtask_depth`, **THEN** the creation is rejected and the worker is informed.

- **GIVEN** a worker instance with status `working`, **WHEN** the user clicks "Pause", **THEN** the current session completes its turn, the instance moves to `paused`, and no new wakeups are processed.

### Runtime capabilities

- **GIVEN** an agent run scoped to task `KAN-1`, **WHEN** the agent posts a comment on `KAN-1`, **THEN** Office records an agent-authored comment tied to that run context.

- **GIVEN** an agent run scoped to task `KAN-1`, **WHEN** the agent tries to update `KAN-2` without explicit scope, **THEN** the runtime denies the action and no task mutation is attempted.

The following operator-boundary scenarios are acceptance criteria that must
pass before launcher settings are described as an isolation boundary:

- **GIVEN** a running Office agent that can reach the backend, **WHEN** it
  submits an execution-profile mutation with its runtime JWT or without a
  credential, **THEN** the backend rejects the request and persists no launcher
  or environment change.

- **GIVEN** an authenticated operator authorized for the target profile,
  **WHEN** the settings UI saves an execution-profile change, **THEN** the
  backend persists the change without exposing the operator credential to any
  agent runtime surface.

- **GIVEN** an Office agent requests execution-profile discovery, **WHEN** the
  backend returns the agent-facing catalog, **THEN** the response contains no
  literal profile environment values, MCP environment values, MCP headers, or
  resolved secret material.

- **GIVEN** agent-controlled JavaScript is rendered in a task preview, **WHEN**
  it attempts to use the operator session, **THEN** browser origin isolation
  prevents it from reading operator state or issuing an authenticated
  control-plane mutation.

- **GIVEN** an agent run with `create_subtask` capability and mutation scope for a parent task, **WHEN** it creates a subtask under that parent, **THEN** Office creates the task through the runtime action surface and preserves the caller agent identity.

- **GIVEN** an agent run without `create_subtask` capability or without mutation scope for the requested parent, **WHEN** it attempts parented task creation, **THEN** Office rejects the request before reading parent relationships or creating any task.

- **GIVEN** a run without a capability, **WHEN** the agent attempts the matching action, **THEN** Office returns a forbidden error and logs no downstream mutation.

### Context and instructions

- **GIVEN** a CEO agent assigned a new task, **WHEN** the scheduler wakes it, **THEN** the agent's `AGENTS.md` with delegation rules is in the system prompt, `HEARTBEAT.md` is on disk at the instructions dir, env vars are set, and the wake payload contains the task details.

- **GIVEN** a worker agent being resumed for a `task_comment`, **WHEN** it's a resume session, **THEN** only the new comment is sent in the prompt (instructions not re-injected; agent CLI retains them).

- **GIVEN** a user editing the CEO's `AGENTS.md` in the Instructions tab, **WHEN** they save, **THEN** the DB is updated. The next time the CEO wakes, the updated instructions are exported to disk and used.

- **GIVEN** a reviewer agent woken for a review, **WHEN** the scheduler prepares the session, **THEN** the reviewer's `AGENTS.md` (review checklist) is in the prompt, its desired skills are written to the worktree, and the wake payload contains the task's changes.

### Skills

- **GIVEN** a user on `/office/workspace/skills`, **WHEN** they click "Add Skill" and enter a name, description, and SKILL.md content, **THEN** the skill appears in the registry and is available for assignment.

- **GIVEN** a skill assigned to a Claude Code worker, **WHEN** the worker starts a new session, **THEN** the skill's `SKILL.md` is written to `<worktree>/.claude/skills/<DirName(slug)>/SKILL.md` (Claude's `ProjectSkillDir`) and begins with YAML frontmatter. For non-Claude agents, the path is `<worktree>/.agents/skills/<DirName(slug)>/SKILL.md`.

- **GIVEN** a skill sourced from a git URL, **WHEN** the user creates the skill entry, **THEN** the repository is cloned and cached and the file inventory displays in the UI.

- **GIVEN** a running session with injected skills, **WHEN** the user edits the skill in the registry, **THEN** the running session is unaffected (file already written). The next session for that agent picks up updated content.

- **GIVEN** an agent with three assigned skills, **WHEN** the user removes one from the agent config, **THEN** the next session only writes the remaining two skills to the worktree.

### CLI and MCP

- **GIVEN** a worker agent woken for `task_assigned`, **WHEN** it needs to update task status, **THEN** it runs `$KANDEV_CLI kandev task update --status in_progress`, which reads auth from env vars, calls `POST /api/v1/office/runtime/tasks/:id/status`, and returns structured JSON.

- **GIVEN** an Office agent wants to add a comment without changing status, **WHEN** it invokes `task update --comment` without `--status`, **THEN** the CLI rejects the command locally and directs it to the signed `tasks message` surface.

- **GIVEN** a CEO agent delegating work with `create_subtask` capability and scope for the parent, **WHEN** it creates a subtask, **THEN** it runs `$KANDEV_CLI kandev task create --title "..." --parent $KANDEV_TASK_ID --assignee agent_id`, which calls `POST /api/v1/office/runtime/tasks` with the run token and returns the created task ID.

- **GIVEN** an Office agent tries to move a task between workflow steps or archive it, **WHEN** it invokes `tasks move` or `tasks archive`, **THEN** the CLI fails before making an HTTP request and directs the operation to a human or admin because no signed Office runtime capability exists for it.

- **GIVEN** a CEO with `can_create_projects=true`, **WHEN** the setup task requires a project for a repository, **THEN** it runs `$KANDEV_CLI kandev projects create --name "..." --repository "..."`, the project is created in `KANDEV_WORKSPACE_ID`, and the structured response contains its ID.

- **GIVEN** an Office agent without `can_create_projects`, **WHEN** it attempts `projects create`, **THEN** the request is rejected with 403 and no project is created.

- **GIVEN** an Office agent creating a follow-up task for an existing project, **WHEN** it passes `--project PROJECT_ID`, **THEN** `task create` sends `project_id`, runtime validation forces the token workspace and verifies the project, parent, and assignee are owned by it, and the created runner is assigned to the requested Office agent.

- **GIVEN** a direct Office runtime caller, **WHEN** it supplies a cross-workspace project, parent, or assignee, **THEN** task creation is denied before persistence or wakeup.

- **GIVEN** an Office agent passes priority, blocker, or workspace-policy flags to `task create`, **WHEN** the runtime cannot preserve those fields, **THEN** the CLI rejects the request explicitly instead of silently dropping them.

- **GIVEN** a user viewing an office task in advanced mode, **WHEN** the agent needs clarification, **THEN** it uses `ask_user_question` (available in ModeOffice) and the user sees the question in the UI.

- **GIVEN** an office agent in ModeOffice, **WHEN** something tries to call `create_task_kandev` MCP tool, **THEN** the MCP server returns an error saying to use `$KANDEV_CLI` instead.

- **GIVEN** an office agent in ModeOffice, **WHEN** its first-turn system context is generated, **THEN** every advertised MCP tool is registered in ModeOffice, including `step_complete_kandev`.

- **GIVEN** an Office-owned task without a scheduler-prepared signed run context, **WHEN** a generic manual or workflow task/session path attempts to start it in ModeOffice, **THEN** Kandev starts no agent process and returns an error directing the caller to start or wake the task through Office.

- **GIVEN** a regular kanban task (non-office), **WHEN** a user starts a session, **THEN** ModeTask is used with the full Kanban MCP surface, including `step_complete_kandev`. No Office CLI capability changes that behavior.

- **GIVEN** a regular task-mode session, **WHEN** an agent invokes `agentctl kandev` without Office runtime credentials, **THEN** the CLI explains that `KANDEV_API_URL` and `KANDEV_API_KEY` are injected automatically only for Office runs and directs the agent to its Kandev MCP tools.

- **GIVEN** a regular Kanban task on a step with a default agent profile, **WHEN** the runner projection resolves that profile, **THEN** the profile selects the execution identity without changing Office ownership or the session's `ModeTask` MCP surface.

- **GIVEN** a Docker executor, **WHEN** the agent runs `$KANDEV_CLI kandev task get`, **THEN** agentctl resolves to `/usr/local/bin/agentctl` (on PATH inside the container), reads env vars, and calls the backend API.

- **GIVEN** a worker in a Docker container, **WHEN** the scheduler prepares the session, **THEN** skill files are written to the worktree on the host (under the agent type's `ProjectSkillDir`), the worktree is mounted into the container, and the agent discovers skills in its CWD.

- **GIVEN** a CEO on Sprites, **WHEN** the scheduler prepares the session, **THEN** skill and instruction files are uploaded via the filesystem API to the equivalent paths inside the sprite.

### Permissions

- **GIVEN** a worker that calls `POST /office/agents`, **WHEN** the backend validates the JWT, it loads the worker's permissions, sees `can_create_agents: false`, and returns 403 Forbidden.

- **GIVEN** a CEO creating a worker, **WHEN** it passes `role: "worker"` with no permission overrides, **THEN** the backend applies worker defaults. The CEO can optionally pass `permissions: {"can_assign_tasks": true}` to give the worker delegation ability.

- **GIVEN** a user on the agent detail page, **WHEN** they click Configuration, **THEN** they see all permissions as toggles with the role default indicated. They can override any permission and save.

- **GIVEN** a CEO trying to create an agent with `can_create_agents: true`, **WHEN** the CEO itself has that permission, **THEN** it's allowed. If a worker (who lacks it) tries the same, it's rejected.

### Dashboard and runs

- **GIVEN** an agent with run history, **WHEN** the user opens `/office/agents/:id/dashboard`, **THEN** the page returns server-rendered with the latest run card, the four 14-day charts, recent tasks, and costs populated from a single `summary` round-trip.

- **GIVEN** a RUNNING run, **WHEN** the user opens its run detail page, **THEN** the SSR-supplied transcript and events render immediately and the Client Component subscribes to the run's WS channel, merging appended messages into state without a refetch.

- **GIVEN** a FAILED run with an auth error, **WHEN** the user opens the run detail, **THEN** the header shows an "auth required" banner linking to the agent settings.

- **GIVEN** a long run list, **WHEN** the user scrolls `/office/agents/:id/runs`, **THEN** "Load more" fetches the next page using a `(requested_at, id)` cursor with stable order.

## Out of scope

- Agent-to-agent real-time communication. Agents communicate via tasks and comments only.
- Custom agent binaries. Instances use the existing agent registry (Claude, Codex, Copilot, etc.).
- Automatic scaling of agent instances by workload; agent migration between workspaces.
- Replacing the existing scheduler. Distributed scheduling or multi-backend leader election. A public external API for third-party agent runtimes.
- `SOUL.md` / `TOOLS.md` content for v1 (empty files created, content later). Automatic `TOOLS.md` generation from API schema.
- Per-task instruction overrides. All agents of a role share the same instructions.
- Skill marketplace or cross-workspace skill sharing. Skill versioning beyond what git-sourced skills offer.
- Skill-level permissions. All skills are available to all agents in the workspace; assignment is the access control.
- Automatic skill recommendation based on task content.
- Bash completion for `agentctl kandev` subcommands. Offline/cached mode for CLI. Incremental skill sync.
- CLI commands for workspace config CRUD (workflows, executors). Handled by ModeConfig MCP tools used by IDE agents.
- CLI creation of additional Office workspaces. Office agents operate inside `KANDEV_WORKSPACE_ID`; humans create or import workspaces through onboarding and settings.
- Configurable date range on dashboard charts. Fixed 14 days for v1.
- Bespoke transcript renderer or adapter-specific "Nice mode" parsers. The existing session-messages component is reused.
- Object-store offload for run logs. The `office_run_events` table covers the structured event log.
- Per-run browser/system notifications. The inbox covers the failure path.
- Pagination for the recent-runs sidebar beyond a fixed ~30 row window.
