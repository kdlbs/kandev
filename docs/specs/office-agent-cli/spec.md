---
status: draft
created: 2026-04-27
owner: cfl
---

# Office: Agent CLI & MCP Mode

## Why

Office agents need to call the kandev API for task management, comments, memory, and delegation. Three problems with the current approach:

1. **MCP token bloat**: MCP tool schemas are included in every LLM API turn. The kanban ModeTask registers 13 tools (~3-5K tokens/turn). Adding office-specific tools would push this to 40+. For autonomous agents running many short sessions, this compounds fast.

2. **Curl is fragile**: The kandev-protocol skill currently teaches agents to call the HTTP API via curl. This leads to JSON escaping bugs, hallucinated endpoints, and forgotten auth headers.

3. **MCP overlap**: Interactive kanban sessions and office agent sessions both need task operations, but through different interfaces. Users opening an office task in advanced mode should still have MCP tools available for interactive use.

## What

### 1. agentctl CLI subcommands

Add a `kandev` command group to the agentctl binary. Office agents call these instead of raw curl:

```
# Task operations (singular form, back-compat for the original spec).
agentctl kandev task get [--id ID]
agentctl kandev task update [--id ID] [--status STATUS] [--comment BODY]
agentctl kandev task create --title TITLE [--parent ID] [--assignee AGENT_ID] [--priority P]

# Task operations (plural form, added for office mutations).
agentctl kandev tasks list [--status S] [--assignee ID] [--project ID]
agentctl kandev tasks move --id T-1 --step STEP_ID [--prompt MSG]
agentctl kandev tasks archive --id T-1
agentctl kandev tasks message --id T-1 --prompt MSG
agentctl kandev tasks conversation --id T-1

# Comments + memory + checkout + labels + docs (unchanged from the original spec).
agentctl kandev comment add --task ID --body BODY
agentctl kandev comment list --task ID [--limit N] [--after COMMENT_ID]
agentctl kandev memory get [--layer LAYER] [--key KEY]
agentctl kandev memory set --layer LAYER --key KEY --content CONTENT
agentctl kandev memory summary
agentctl kandev checkout --task ID

# Agents — CEO-only roster control.
agentctl kandev agents list   [--role ROLE] [--status STATUS]
agentctl kandev agents create --name N --role R [--budget-monthly-cents …] [--reason …]
agentctl kandev agents update --id A-1 [--name …] [--budget-monthly-cents …]
agentctl kandev agents delete --id A-1

# Routines — schedule recurring work (cron triggers attached in the same call).
agentctl kandev routines list
agentctl kandev routines create --name N --task-title T --assignee A-1 \
  [--cron "0 9 * * MON-FRI"] [--timezone TZ] [--concurrency …]
agentctl kandev routines pause   --id R-1
agentctl kandev routines resume  --id R-1
agentctl kandev routines delete  --id R-1

# Approvals — decide hire / budget / task approvals.
agentctl kandev approvals list   [--status pending|approved|rejected]
agentctl kandev approvals decide --id AP-1 --decision approve|reject [--note …]

# Budget — read-only spend visibility.
agentctl kandev budget get [--agent-id A-1]
```

Each subcommand maps 1:1 to an HTTP endpoint under `/api/v1/office/…` (or the kanban-shared `/api/v1/tasks/…` for move/archive). System skills under `apps/backend/internal/office/configloader/skills/` teach agents when to use each subcommand; see [office-skills](../office-skills/spec.md).

**Auth is automatic**: the CLI reads `KANDEV_API_URL`, `KANDEV_API_KEY`, `KANDEV_RUN_ID`, `KANDEV_AGENT_ID`, `KANDEV_TASK_ID` from environment. No headers or tokens in agent output.

**Output is structured JSON** by default, compact and parseable. Optional `--format text` for human-readable output.

**Errors are clear**: non-zero exit code + JSON `{"error": "message", "code": 409}`. Agent sees structured failures, not HTTP noise.

**Task ID defaults to `$KANDEV_TASK_ID`** when `--id` / `--task` is omitted. Agents working on their assigned task don't need to pass the ID.

### 2. ModeOffice for MCP

Add a third MCP mode alongside ModeTask and ModeConfig:

| Mode | Tools | Token cost/turn | Used by |
|------|-------|-----------------|---------|
| `ModeTask` | 13 (kanban + plans + ask_user) | ~3-5K | Interactive kanban sessions |
| `ModeConfig` | 29 (workflows + agents + executors) | ~8-10K | Config setup sessions |
| `ModeOffice` | 5 (plans + ask_user) | ~1-2K | Office agent sessions |

ModeOffice includes:
- **Plan tools** (4): `create_task_plan`, `get_task_plan`, `update_task_plan`, `delete_task_plan` -- agents may create structured plans during work
- **ask_user_question** (1): when the user opens the task in advanced mode, the agent can ask questions

ModeOffice excludes:
- Kanban tools (`create_task`, `update_task`, `list_tasks`, etc.) -- replaced by CLI
- Config tools -- not needed during task execution
- `list_workspaces`, `list_workflows`, `list_workflow_steps` -- not relevant

If an agent somehow calls an excluded tool (e.g. via a stale skill reference), the MCP server returns a clear error: `"Tool not available in office mode. Use $KANDEV_CLI instead."`.

### 3. KANDEV_CLI environment variable

Injected into office agent sessions alongside the existing KANDEV_* vars:

| Variable | Value | Purpose |
|----------|-------|---------|
| `KANDEV_CLI` | `/path/to/agentctl` | Path to CLI binary for API operations |

**Resolution per executor type:**
- **Docker** (`local_docker`): `/usr/local/bin/agentctl` (baked into image, always on PATH)
- **Standalone** (`local_pc`): The path found by `launcher.findAgentctlBinary()`, stored in launcher state
- **Sprites/Remote**: The agentctl binary path inside the remote environment

The env var is set during session prep in `buildEnvVars()`, only for office sessions.

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

When a user opens an office task in the web UI's advanced mode (terminal, files, changes panels), the session uses ModeOffice. The user can interact via the terminal and the agent can ask questions via `ask_user_question`. The agent still uses the CLI for task operations -- the user sees these as shell commands in the terminal panel, which is more transparent than hidden MCP tool calls.

## Scenarios

- **GIVEN** a worker agent woken for `task_assigned`, **WHEN** it needs to update the task status, **THEN** it runs `$KANDEV_CLI kandev task update --status in_progress` which reads auth from env vars, calls `PATCH /api/v1/office/tasks/:id`, and returns structured JSON.

- **GIVEN** a CEO agent delegating work, **WHEN** it creates a subtask, **THEN** it runs `$KANDEV_CLI kandev task create --title "..." --parent $KANDEV_TASK_ID --assignee agent_id` which calls `POST /api/v1/tasks` with the correct headers and returns the created task ID.

- **GIVEN** a user viewing an office task in advanced mode, **WHEN** the agent needs clarification, **THEN** it uses the `ask_user_question` MCP tool (still available in ModeOffice) and the user sees the question in the UI.

- **GIVEN** an office agent in ModeOffice, **WHEN** something tries to call `create_task_kandev` MCP tool, **THEN** the MCP server returns an error saying to use `$KANDEV_CLI` instead.

- **GIVEN** a regular kanban task (non-office), **WHEN** a user starts a session, **THEN** ModeTask is used with all 13 MCP tools. No change to existing behavior.

- **GIVEN** a Docker executor, **WHEN** the agent runs `$KANDEV_CLI kandev task get`, **THEN** agentctl is at `/usr/local/bin/agentctl` (on PATH), resolves to the same binary, reads env vars, calls the backend API.

### 6. CWD-based skill & instruction delivery

Skills are written into the agent's worktree (CWD) before each session. Because every executor type (standalone, Docker, Sprites) runs the agent with its worktree as the CWD, this approach works uniformly without executor-specific file delivery logic.

**Delivery path (depends on agent type):**

| Agent CLI | Path |
|-----------|------|
| Claude Code | `<worktree>/.claude/skills/kandev-<slug>/SKILL.md` |
| All others | `<worktree>/.agents/skills/kandev-<slug>/SKILL.md` |

- Claude Code reads project skills from `.claude/skills/`, not `.agents/skills/`.
- `kandev-` prefix distinguishes injected skills from team-committed skills in the repo.
- `kandev-*` patterns are added to `<worktree>/.git/info/exclude` so injected skills never appear as dirty files.

**Per executor type:**

| Executor | Worktree location | How skills arrive |
|----------|-------------------|-------------------|
| `local_pc` / `worktree` | Host filesystem | Written directly by the scheduler |
| `local_docker` | Host dir, mounted into container at same path | Written on host before container start |
| `sprites` | Local staging dir, uploaded during instance setup | Written to staging, uploaded via Sprites filesystem API |

**No symlinks, no HOME directory pollution, no cleanup hooks.** When the worktree is deleted at session end, all injected skill directories are removed automatically.

**File manifest structure:**

The office service builds this during session prep and passes it through the executor request:

```go
type SkillManifest struct {
    Skills       []ManifestSkill       // Slug + SKILL.md content
    Instructions []ManifestInstruction // Filename + content
    AgentTypeID  string
    WorkspaceSlug string
    AgentID      string
}
```

**Instructions export (unchanged):**

Instructions (AGENTS.md, HEARTBEAT.md, SOUL.md) are exported to `~/.kandev/runtime/<ws>/instructions/<agentId>/` and the path is injected into the agent prompt. This path remains on the host (standalone and Docker see it directly; Sprites uploads the files to the equivalent path on the sprite).

**Prompt path correctness:**

The `BuildAgentPrompt()` embeds the instructions directory path. Skills do not require a path directive since the agent discovers them automatically via `.agents/skills/` in the CWD.

### 7. Agent permissions: enforcement + UI

Skills are written into each agent's worktree CWD and are isolated per session. However, agents could in principle call office API endpoints beyond their role. The backend must be the gatekeeper.

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

Auth middleware on office API routes:
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

When a CEO agent calls `POST /office/agents`:
- Must have `can_create_agents` permission (enforced)
- Specifies `role` (required) -- defaults applied automatically
- Can optionally pass `permissions` overrides
- Cannot grant permissions it doesn't have itself (no privilege escalation)

## Scenarios

(existing scenarios remain, plus:)

- **GIVEN** a worker agent running in a Docker container, **WHEN** the scheduler prepares the session, **THEN** skill files are written to the worktree on the host (`.claude/skills/kandev-*/` for Claude, `.agents/skills/kandev-*/` for others), the worktree is mounted into the container, and the agent discovers skills in its CWD.

- **GIVEN** a CEO agent running on Sprites, **WHEN** the scheduler prepares the session, **THEN** skill files and instruction files are uploaded to the sprite via the filesystem API, with skills landing at the appropriate path for the agent type and instructions at `~/.kandev/runtime/<ws>/instructions/<agentId>/`.

- **GIVEN** a standalone agent, **WHEN** the scheduler prepares the session, **THEN** skill files are written directly to the worktree on the host and the agent discovers them via `.agents/skills/` in its CWD.

- **GIVEN** a worker agent that calls `POST /office/agents` to create a new agent, **WHEN** the backend validates the JWT, it loads the worker's permissions, sees `can_create_agents: false`, and returns 403 Forbidden.

- **GIVEN** a CEO agent creating a worker, **WHEN** it passes `role: "worker"` with no permission overrides, **THEN** the backend applies worker defaults. The CEO can optionally pass `permissions: {"can_assign_tasks": true}` to give this worker delegation ability.

- **GIVEN** a user on the agent detail page, **WHEN** they click the Permissions tab, **THEN** they see all permissions as toggles with the role default indicated. They can override any permission and save.

- **GIVEN** a CEO agent trying to create an agent with `can_create_agents: true`, **WHEN** the CEO itself has that permission, **THEN** it's allowed. If a worker (who lacks it) somehow tries the same, it's rejected.

## Out of scope

- Bash completion for agentctl kandev subcommands (nice-to-have, later)
- Offline/cached mode for CLI (always calls API)
- CLI commands for workspace config CRUD (workflows, executors — handled by ModeConfig MCP tools used by IDE agents)
- Incremental skill sync (full manifest uploaded each session -- skills are small)

(Routine management, approvals, and budget read-back were originally listed as out of scope but were promoted to first-class CLI surfaces alongside the office system-skill rollout. The CEO uses these on every heartbeat through the `kandev-routines`, `kandev-approvals`, and `kandev-budget` system skills.)
