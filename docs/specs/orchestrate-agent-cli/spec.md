---
status: draft
created: 2026-04-27
owner: cfl
---

# Orchestrate: Agent CLI & MCP Mode

## Why

Orchestrate agents need to call the kandev API for task management, comments, memory, and delegation. Three problems with the current approach:

1. **MCP token bloat**: MCP tool schemas are included in every LLM API turn. The kanban ModeTask registers 13 tools (~3-5K tokens/turn). Adding orchestrate-specific tools would push this to 40+. For autonomous agents running many short sessions, this compounds fast.

2. **Curl is fragile**: The kandev-protocol skill currently teaches agents to call the HTTP API via curl. This leads to JSON escaping bugs, hallucinated endpoints, and forgotten auth headers.

3. **MCP overlap**: Interactive kanban sessions and orchestrate agent sessions both need task operations, but through different interfaces. Users opening an orchestrate task in advanced mode should still have MCP tools available for interactive use.

## What

### 1. agentctl CLI subcommands

Add a `kandev` command group to the agentctl binary. Orchestrate agents call these instead of raw curl:

```
agentctl kandev task get [--id ID]
agentctl kandev task update [--id ID] [--status STATUS] [--comment BODY]
agentctl kandev task create --title TITLE [--parent ID] [--assignee AGENT_ID] [--priority P]
agentctl kandev comment add --task ID --body BODY
agentctl kandev comment list --task ID [--limit N] [--after COMMENT_ID]
agentctl kandev agents list [--role ROLE] [--status STATUS]
agentctl kandev memory get [--layer LAYER] [--key KEY]
agentctl kandev memory set --layer LAYER --key KEY --content CONTENT
agentctl kandev memory summary
agentctl kandev checkout --task ID
```

**Auth is automatic**: the CLI reads `KANDEV_API_URL`, `KANDEV_API_KEY`, `KANDEV_RUN_ID`, `KANDEV_AGENT_ID`, `KANDEV_TASK_ID` from environment. No headers or tokens in agent output.

**Output is structured JSON** by default, compact and parseable. Optional `--format text` for human-readable output.

**Errors are clear**: non-zero exit code + JSON `{"error": "message", "code": 409}`. Agent sees structured failures, not HTTP noise.

**Task ID defaults to `$KANDEV_TASK_ID`** when `--id` / `--task` is omitted. Agents working on their assigned task don't need to pass the ID.

### 2. ModeOrchestrate for MCP

Add a third MCP mode alongside ModeTask and ModeConfig:

| Mode | Tools | Token cost/turn | Used by |
|------|-------|-----------------|---------|
| `ModeTask` | 13 (kanban + plans + ask_user) | ~3-5K | Interactive kanban sessions |
| `ModeConfig` | 29 (workflows + agents + executors) | ~8-10K | Config setup sessions |
| `ModeOrchestrate` | 5 (plans + ask_user) | ~1-2K | Orchestrate agent sessions |

ModeOrchestrate includes:
- **Plan tools** (4): `create_task_plan`, `get_task_plan`, `update_task_plan`, `delete_task_plan` -- agents may create structured plans during work
- **ask_user_question** (1): when the user opens the task in advanced mode, the agent can ask questions

ModeOrchestrate excludes:
- Kanban tools (`create_task`, `update_task`, `list_tasks`, etc.) -- replaced by CLI
- Config tools -- not needed during task execution
- `list_workspaces`, `list_workflows`, `list_workflow_steps` -- not relevant

If an agent somehow calls an excluded tool (e.g. via a stale skill reference), the MCP server returns a clear error: `"Tool not available in orchestrate mode. Use $KANDEV_CLI instead."`.

### 3. KANDEV_CLI environment variable

Injected into orchestrate agent sessions alongside the existing KANDEV_* vars:

| Variable | Value | Purpose |
|----------|-------|---------|
| `KANDEV_CLI` | `/path/to/agentctl` | Path to CLI binary for API operations |

**Resolution per executor type:**
- **Docker** (`local_docker`): `/usr/local/bin/agentctl` (baked into image, always on PATH)
- **Standalone** (`local_pc`): The path found by `launcher.findAgentctlBinary()`, stored in launcher state
- **Sprites/Remote**: The agentctl binary path inside the remote environment

The env var is set during session prep in `buildEnvVars()`, only for orchestrate sessions.

### 4. Rewritten kandev-protocol skill

The skill switches from curl examples to CLI examples:

```markdown
## API Access

Use the `$KANDEV_CLI` command for all kandev API operations.
Authentication and headers are handled automatically from environment variables.

### Read your task
$KANDEV_CLI kandev task get

### Update task status (always post a comment first)
$KANDEV_CLI kandev comment add --body "Completed the implementation with tests."
$KANDEV_CLI kandev task update --status done

### Create a subtask and delegate
$KANDEV_CLI kandev task create --title "Write unit tests" --parent $KANDEV_TASK_ID --assignee agent_worker_1

### List available agents for delegation
$KANDEV_CLI kandev agents list

### Memory operations
$KANDEV_CLI kandev memory set --layer knowledge --key "api-patterns" --content "Uses REST with JWT auth"
$KANDEV_CLI kandev memory get --layer knowledge --key "api-patterns"
```

The heartbeat procedure, wake reasons, critical rules, and role-based instructions remain unchanged. Only the API call mechanism changes.

### 5. Interactive advanced mode

When a user opens an orchestrate task in the web UI's advanced mode (terminal, files, changes panels), the session uses ModeOrchestrate. The user can interact via the terminal and the agent can ask questions via `ask_user_question`. The agent still uses the CLI for task operations -- the user sees these as shell commands in the terminal panel, which is more transparent than hidden MCP tool calls.

## Scenarios

- **GIVEN** a worker agent woken for `task_assigned`, **WHEN** it needs to update the task status, **THEN** it runs `$KANDEV_CLI kandev task update --status in_progress` which reads auth from env vars, calls `PATCH /api/v1/orchestrate/tasks/:id`, and returns structured JSON.

- **GIVEN** a CEO agent delegating work, **WHEN** it creates a subtask, **THEN** it runs `$KANDEV_CLI kandev task create --title "..." --parent $KANDEV_TASK_ID --assignee agent_id` which calls `POST /api/v1/tasks` with the correct headers and returns the created task ID.

- **GIVEN** a user viewing an orchestrate task in advanced mode, **WHEN** the agent needs clarification, **THEN** it uses the `ask_user_question` MCP tool (still available in ModeOrchestrate) and the user sees the question in the UI.

- **GIVEN** an orchestrate agent in ModeOrchestrate, **WHEN** something tries to call `create_task_kandev` MCP tool, **THEN** the MCP server returns an error saying to use `$KANDEV_CLI` instead.

- **GIVEN** a regular kanban task (non-orchestrate), **WHEN** a user starts a session, **THEN** ModeTask is used with all 13 MCP tools. No change to existing behavior.

- **GIVEN** a Docker executor, **WHEN** the agent runs `$KANDEV_CLI kandev task get`, **THEN** agentctl is at `/usr/local/bin/agentctl` (on PATH), resolves to the same binary, reads env vars, calls the backend API.

### 6. Executor-aware skill & instruction delivery

The current skill injection (`InjectSkillsForAgent`) and runtime export (`prepareRuntime`) only work for local executors. They write files and create symlinks on the HOST filesystem, which remote agents (Docker, Sprites) cannot see.

**The problem by executor type:**

| Executor | Host symlinks visible? | Host runtime dir visible? | Solution |
|----------|----------------------|--------------------------|----------|
| Standalone | Yes | Yes | Current approach works |
| Docker | No (isolated FS) | No (not mounted) | Mount runtime dir + symlink inside container |
| Sprites | No (remote VM) | No (remote VM) | Upload files via Sprites API + symlink inside sprite |

**The solution: executor-aware file delivery**

The orchestrate scheduler builds a **file manifest** during session prep -- the list of skill files and instruction files that the agent needs. How those files reach the agent depends on the executor:

**Standalone (local_pc, worktree):**
- Current behavior unchanged: write to `~/.kandev/runtime/`, create symlinks on host
- Agent sees them directly via shared filesystem

**Docker (local_docker):**
- Mount `~/.kandev/runtime/<workspace>/` into the container at the same path
- Add a step to the prepare script (or a post-create command via agentctl) that creates symlinks inside the container:
  ```bash
  # Symlink skills into agent CLI discovery dir
  for skill_dir in ~/.kandev/runtime/<ws>/skills/*/; do
    slug=$(basename "$skill_dir")
    ln -sf "$skill_dir" ~/.claude/skills/"$slug"
    ln -sf "$skill_dir" ~/.agents/skills/"$slug"
  done
  ```
- Instructions dir path in the prompt is valid because the mount makes it accessible

**Sprites (sprites):**
- Upload skill and instruction files via Sprites filesystem API (same mechanism as agentctl binary upload)
- Add a new step in the sprite setup flow (`stepUploadSkills`) between credential upload and prepare script:
  ```go
  // For each skill in the manifest:
  sprite.Filesystem().WriteFileContext(ctx,
      fmt.Sprintf("/root/.kandev/runtime/%s/skills/%s/SKILL.md", ws, slug),
      content, 0o644)
  // For each instruction file:
  sprite.Filesystem().WriteFileContext(ctx,
      fmt.Sprintf("/root/.kandev/runtime/%s/instructions/%s/%s", ws, agentID, filename),
      content, 0o644)
  ```
- Then create symlinks via command execution (same pattern as credential setup scripts)
- Retry logic already exists via `writeFileWithRetry()`

**File manifest structure:**

The orchestrate service builds this during session prep and passes it through the executor request:

```go
type SkillManifest struct {
    Skills []SkillFile        // Slug + SKILL.md content
    Instructions []InstructionFile  // Filename + content
    InstructionsDir string    // Target path inside execution env
    AgentType string          // For determining skill discovery dirs
}
```

**Integration with existing executor flows:**

The `ExecutorCreateRequest` already carries env vars, MCP config, and prepare scripts. Adding a skill manifest follows the same pattern. Each executor backend decides how to deliver the files:
- Standalone: ignores manifest (already handled by host-side export)
- Docker: adds volume mount + post-create symlink step
- Sprites: adds upload step in `stepSetupEnvironment()`

**Prompt path correctness:**

The `BuildAgentPrompt()` embeds the instructions directory path. For remote executors, this path must point to where the files actually land:
- Standalone: `~/.kandev/runtime/<ws>/instructions/<agentId>/` (host path)
- Docker: same path (mounted from host)
- Sprites: `/root/.kandev/runtime/<ws>/instructions/<agentId>/` (uploaded)

The path is determined by the executor, not hardcoded by the scheduler.

### 7. Agent permissions: enforcement + UI

Skills are symlinked into shared directories (`~/.claude/skills/`), so any agent on the same standalone host can discover any skill. A worker could read the CEO's delegation skill and try to create agents. The backend must be the gatekeeper.

**Permission model:**

Role defaults are the baseline, overridable per agent:

| Permission | CEO | Worker | Specialist | Assistant | Reviewer |
|-----------|-----|--------|------------|-----------|----------|
| `can_create_tasks` | yes | yes | yes | yes | no |
| `can_assign_tasks` | yes | no | no | yes | no |
| `can_create_agents` | yes | no | no | no | no |
| `can_approve` | yes | no | no | no | yes |
| `can_manage_own_skills` | yes | no | no | yes | no |
| `max_subtask_depth` | 3 | 1 | 1 | 1 | 0 |

When creating an agent (via UI or CEO API call), the role determines default permissions. Individual permissions can be toggled on/off as overrides.

**Backend enforcement:**

Auth middleware on orchestrate API routes:
1. Extract `Authorization: Bearer <JWT>` header
2. Validate JWT signature + expiration
3. Load agent instance + resolved permissions from DB
4. Set agent context on request (ID, role, permissions)
5. UI requests (no JWT / session cookie) bypass as admin

Permission checks at the service layer:

```go
func (s *Service) CreateAgentInstance(ctx context.Context, agent *AgentInstance) error {
    if caller := agentFromCtx(ctx); caller != nil {
        if !caller.HasPermission("can_create_agents") {
            return ErrForbidden
        }
    }
    // ... proceed with creation
}
```

Task scope enforcement -- agent can only operate on tasks assigned to it:

```go
if claims.TaskID != "" && requestedTaskID != claims.TaskID {
    // Agent trying to modify a task it's not assigned to
    return ErrForbidden
}
```

Exception: CEO agents with `can_assign_tasks` can operate on any task (for delegation).

**Agent detail page: Permissions tab**

New tab alongside Overview, Instructions, Skills, Runs, Memory, Channels:

- Shows all permissions as labeled toggles with on/off state
- Role defaults shown as the baseline (dimmed label: "from role: worker")
- User can toggle individual permissions to override the default
- `max_subtask_depth` shown as a number input
- Changes saved immediately to DB via `PATCH /agents/:id`

**Meta endpoint: permission definitions**

Add permission definitions to the `/meta` response so the frontend knows what permissions exist:

```json
{
  "permissions": [
    {"key": "can_create_tasks", "label": "Create tasks", "description": "Agent can create new tasks and subtasks"},
    {"key": "can_assign_tasks", "label": "Assign tasks", "description": "Agent can assign tasks to other agents"},
    {"key": "can_create_agents", "label": "Create agents", "description": "Agent can hire/create new agent instances"},
    {"key": "can_approve", "label": "Approve/reject", "description": "Agent can decide approval requests"},
    {"key": "can_manage_own_skills", "label": "Manage own skills", "description": "Agent can add/remove its own skills"}
  ],
  "permissionDefaults": {
    "ceo": {"can_create_tasks": true, "can_assign_tasks": true, ...},
    "worker": {"can_create_tasks": true, "can_assign_tasks": false, ...}
  }
}
```

**CEO creating agents via API:**

When a CEO agent calls `POST /orchestrate/agents`:
- Must have `can_create_agents` permission (enforced)
- Specifies `role` (required) -- defaults applied automatically
- Can optionally pass `permissions` overrides
- Cannot grant permissions it doesn't have itself (no privilege escalation)

## Scenarios

(existing scenarios remain, plus:)

- **GIVEN** a worker agent running in a Docker container, **WHEN** the scheduler prepares the session, **THEN** skill files are written to `~/.kandev/runtime/` on the host, the runtime dir is mounted into the container, symlinks are created inside the container at `~/.claude/skills/`, and the instructions dir path in the prompt points to the mounted location.

- **GIVEN** a CEO agent running on Sprites, **WHEN** the scheduler prepares the session, **THEN** skill files and instruction files are uploaded to the sprite via the filesystem API, symlinks are created via command execution, and the instructions dir path in the prompt points to the uploaded location.

- **GIVEN** a standalone agent, **WHEN** the scheduler prepares the session, **THEN** behavior is unchanged from current implementation -- host symlinks and runtime dir exports work as before.

- **GIVEN** a worker agent that discovered the CEO's delegation skill via shared `~/.claude/skills/`, **WHEN** it calls `POST /orchestrate/agents` to create a new agent, **THEN** the backend validates the JWT, loads the worker's permissions, sees `can_create_agents: false`, and returns 403 Forbidden.

- **GIVEN** a CEO agent creating a worker, **WHEN** it passes `role: "worker"` with no permission overrides, **THEN** the backend applies worker defaults. The CEO can optionally pass `permissions: {"can_assign_tasks": true}` to give this worker delegation ability.

- **GIVEN** a user on the agent detail page, **WHEN** they click the Permissions tab, **THEN** they see all permissions as toggles with the role default indicated. They can override any permission and save.

- **GIVEN** a CEO agent trying to create an agent with `can_create_agents: true`, **WHEN** the CEO itself has that permission, **THEN** it's allowed. If a worker (who lacks it) somehow tries the same, it's rejected.

## Out of scope

- Bash completion for agentctl kandev subcommands (nice-to-have, later)
- Offline/cached mode for CLI (always calls API)
- CLI commands for config operations (handled by ModeConfig MCP tools)
- CLI commands for cost/budget/routine management (admin-only, not needed by agents)
- Incremental skill sync (full manifest uploaded each session -- skills are small)
