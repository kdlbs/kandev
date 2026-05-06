# Office Agent CLI & MCP Mode Implementation Plan

**Date:** 2026-04-27
**Status:** proposed
**Spec:** `docs/specs/office-agent-cli/spec.md`

## Phase 1: agentctl CLI subcommands

### 1.1 Command framework

**File:** `cmd/agentctl/kandev.go` (NEW)

agentctl is a flag-based binary (no cobra/subcommand framework). Add a top-level `kandev` argument that routes to a subcommand handler:

```go
// In main.go, before starting the HTTP server:
if len(os.Args) > 1 && os.Args[1] == "kandev" {
    os.Exit(runKandevCLI(os.Args[2:]))
}
```

```go
// kandev.go
func runKandevCLI(args []string) int {
    if len(args) == 0 {
        fmt.Fprintln(os.Stderr, "Usage: agentctl kandev <command> [flags]")
        return 1
    }
    switch args[0] {
    case "task":
        return runTaskCmd(args[1:])
    case "comment":
        return runCommentCmd(args[1:])
    case "agents":
        return runAgentsCmd(args[1:])
    case "memory":
        return runMemoryCmd(args[1:])
    case "checkout":
        return runCheckoutCmd(args[1:])
    default:
        fmt.Fprintf(os.Stderr, "Unknown command: %s\n", args[0])
        return 1
    }
}
```

### 1.2 HTTP client

**File:** `cmd/agentctl/kandev_client.go` (NEW)

Thin HTTP client that reads env vars once and reuses them:

```go
type kandevClient struct {
    apiURL      string // KANDEV_API_URL
    apiKey      string // KANDEV_API_KEY
    runID       string // KANDEV_RUN_ID
    agentID     string // KANDEV_AGENT_ID
    taskID      string // KANDEV_TASK_ID (default for --id/--task flags)
    workspaceID string // KANDEV_WORKSPACE_ID
    http        *http.Client
}

func newKandevClient() (*kandevClient, error) {
    apiURL := os.Getenv("KANDEV_API_URL")
    apiKey := os.Getenv("KANDEV_API_KEY")
    if apiURL == "" || apiKey == "" {
        return nil, fmt.Errorf("KANDEV_API_URL and KANDEV_API_KEY must be set")
    }
    return &kandevClient{
        apiURL:      apiURL,
        apiKey:      apiKey,
        runID:       os.Getenv("KANDEV_RUN_ID"),
        agentID:     os.Getenv("KANDEV_AGENT_ID"),
        taskID:      os.Getenv("KANDEV_TASK_ID"),
        workspaceID: os.Getenv("KANDEV_WORKSPACE_ID"),
        http:        &http.Client{Timeout: 30 * time.Second},
    }, nil
}

func (c *kandevClient) do(method, path string, body any) ([]byte, int, error) {
    // Builds request with Authorization + X-Kandev-Run-Id headers
    // Returns response body, status code, error
}
```

### 1.3 Task commands

**File:** `cmd/agentctl/kandev_task.go` (NEW)

```go
func runTaskCmd(args []string) int {
    if len(args) == 0 { usage(); return 1 }
    switch args[0] {
    case "get":    return taskGet(args[1:])
    case "update": return taskUpdate(args[1:])
    case "create": return taskCreate(args[1:])
    }
}
```

- `task get [--id ID]` → `GET /api/v1/office/tasks/:id` (defaults to `$KANDEV_TASK_ID`)
- `task update [--id ID] --status S [--comment C]` → `PATCH /api/v1/office/tasks/:id`
- `task create --title T [--parent ID] [--assignee A] [--priority P]` → `POST /api/v1/tasks`

### 1.4 Comment commands

**File:** `cmd/agentctl/kandev_comment.go` (NEW)

- `comment add --task ID --body BODY` → `POST /api/v1/office/tasks/:id/comments`
- `comment list --task ID [--limit N] [--after CID]` → `GET /api/v1/office/tasks/:id/comments`

`--task` defaults to `$KANDEV_TASK_ID`. `--body` reads from stdin if value is `-`:
```bash
echo "Multi-line comment here" | $KANDEV_CLI kandev comment add --body -
```

### 1.5 Agent commands

**File:** `cmd/agentctl/kandev_agents.go` (NEW)

- `agents list [--role R] [--status S]` → `GET /api/v1/office/workspaces/:wsId/agents`

### 1.6 Memory commands

**File:** `cmd/agentctl/kandev_memory.go` (NEW)

- `memory get [--layer L] [--key K]` → `GET /api/v1/office/agents/:id/memory` or `GET .../memory/:layer/:key`
- `memory set --layer L --key K --content C` → `PUT /api/v1/office/agents/:id/memory`
- `memory summary` → `GET /api/v1/office/agents/:id/memory/summary`

Uses `$KANDEV_AGENT_ID` for the agent ID path param.

### 1.7 Checkout command

**File:** `cmd/agentctl/kandev_checkout.go` (NEW)

- `checkout --task ID` → Calls the checkout endpoint with CAS pattern
- Returns task details on success, clear error on 409 (already claimed)

---

## Phase 2: ModeOffice for MCP

### 2.1 Add mode constant

**File:** `internal/agentctl/server/mcp/server.go`

```go
const (
    ModeTask        = "task"
    ModeConfig      = "config"
    ModeOffice = "office"
)
```

### 2.2 Register office tools

**File:** `internal/agentctl/server/mcp/server.go`

Add case to `registerTools()`:

```go
case ModeOffice:
    if !s.disableAskQuestion {
        s.registerInteractionTools()
    }
    s.registerPlanTools()
```

5 tools total (4 plan + 1 ask_user).

### 2.3 Set mode for office sessions

**File:** `internal/orchestrator/executor/executor_execute.go`

When launching an office agent session, set `McpMode = ModeOffice`:

```go
if isOfficeSession(session) {
    req.McpMode = mcp.ModeOffice
}
```

The `isOfficeSession()` check looks at the task's workflow -- office tasks use the system "Office" workflow. Or add a flag to the session/execution metadata.

### 2.4 Update MCP server tests

**File:** `internal/agentctl/server/mcp/server_test.go`

Add test:
```go
func TestModeOffice(t *testing.T) {
    s := New(newTestLogger(t), nil, "office", false)
    tools := getRegisteredToolNames(s)
    assert.Len(t, tools, 5)
    assert.Contains(t, tools, "create_task_plan_kandev")
    assert.Contains(t, tools, "ask_user_question_kandev")
    assert.NotContains(t, tools, "create_task_kandev")
    assert.NotContains(t, tools, "list_tasks_kandev")
}
```

---

## Phase 3: KANDEV_CLI injection

### 3.1 Store agentctl binary path

**File:** `internal/agentctl/client/launcher/launcher.go`

The launcher already stores `binaryPath`. Expose it:

```go
func (l *Launcher) BinaryPath() string {
    return l.binaryPath
}
```

### 3.2 Pass path through to office service

**File:** `internal/office/service/service.go`

Add `agentctlBinaryPath string` field, set during provider initialization:

```go
type Service struct {
    // existing fields...
    agentctlBinaryPath string
}
```

### 3.3 Inject into env vars

**File:** `internal/office/service/env_builder.go`

Add to `buildEnvVars()`:

```go
if si.svc.agentctlBinaryPath != "" {
    env["KANDEV_CLI"] = si.svc.agentctlBinaryPath
}
```

For Docker executors, override to `/usr/local/bin/agentctl` (since the host path doesn't apply inside containers). This can be handled in the executor-specific env resolution.

---

## Phase 4: Rewrite kandev-protocol skill

**File:** `internal/office/configloader/skills/kandev-protocol/SKILL.md`

Replace all curl examples with CLI equivalents. Keep:
- Heartbeat procedure (8 steps)
- Wake reasons and payload format
- Critical rules
- Role-based instructions

Change:
- All `curl` commands → `$KANDEV_CLI kandev ...` commands
- Remove auth header instructions (CLI handles it)
- Add CLI reference section with all available subcommands
- Add note about `--body -` for stdin input (multiline comments)
- Add error handling guidance (check exit code, parse JSON error)

---

## Phase 5: CWD-based skill & instruction delivery

### 5.1 Skill manifest model

**File:** `internal/office/service/skill_manifest.go` (NEW)

```go
type SkillManifest struct {
    Skills        []ManifestSkill
    Instructions  []ManifestInstruction
    AgentTypeID   string   // e.g. "claude-acp", "codex-acp"
    WorkspaceSlug string
    AgentID       string
}

type ManifestSkill struct {
    Slug    string
    Content string // SKILL.md content
}

type ManifestInstruction struct {
    Filename string // "AGENTS.md", "HEARTBEAT.md", etc.
    Content  string
    IsEntry  bool
}
```

Built by `buildSkillManifest()` during session prep.

### 5.2 Refactor scheduler_integration.go

**File:** `internal/office/service/scheduler_integration.go`

```go
func (si *SchedulerIntegration) processWakeup(ctx context.Context, wakeup *models.WakeupRequest) {
    // ... guards, checkout, budget check ...

    // Build skill manifest (always -- just data, no side effects)
    manifest := si.buildSkillManifest(ctx, agent, defaultWorkspaceName)

    // Resolve executor and worktree path
    execCfg, err := si.resolveExecutorForWakeup(ctx, agent, wakeup.Payload)
    worktreePath := execCfg.WorktreePath // CWD for the agent process

    // Deliver skills into worktree CWD
    instructionsDir := si.deliverSkills(ctx, manifest, worktreePath)

    // Build prompt with correct instructions path
    prompt := si.svc.BuildAgentPrompt(wakeup, agent, instructionsDir, false, wakeContext)

    // Launch
    si.launchOrLog(ctx, ...)
}
```

### 5.3 CWD-based skill delivery

**File:** `internal/office/service/skill_delivery.go` (NEW)

Skills are always written into the agent's worktree (CWD), regardless of executor type. The worktree is the single point of delivery for all execution environments (local_pc, local_docker, sprites):

- **local_pc / worktree**: CWD is on the host filesystem. Write directly.
- **local_docker**: CWD is a host directory mounted into the container at the same path. Write on host; container sees it via the existing worktree mount.
- **sprites**: CWD is uploaded during instance creation. Write to local staging area; the executor uploads the worktree contents including injected skills.

```go
func (si *SchedulerIntegration) deliverSkills(
    ctx context.Context, manifest *SkillManifest, worktreePath, agentTypeID string,
) (instructionsDir string, err error) {
    // 1. Write each skill to the agent-specific path:
    //    Claude: <worktree>/.claude/skills/kandev-<slug>/SKILL.md
    //    Others: <worktree>/.agents/skills/kandev-<slug>/SKILL.md
    skillBase := ".agents"
    if agentTypeID == "claude-acp" {
        skillBase = ".claude"
    }
    for _, skill := range manifest.Skills {
        dir := filepath.Join(worktreePath, skillBase, "skills", "kandev-"+skill.Slug)
        os.MkdirAll(dir, 0o755)
        os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skill.Content), 0o644)
    }

    // 2. Add kandev-* to .git/info/exclude so injected skills don't show as dirty
    ensureExcludePattern(worktreePath, "kandev-*")

    // 3. Export instructions to runtime dir (path returned for prompt injection)
    instructionsDir = si.exportInstructions(ctx, manifest)
    return instructionsDir, nil
}
```

No cleanup is needed: skills are stored inside the worktree. When the worktree is deleted at session end, all injected skill directories are removed automatically.

### 5.4 Instructions export (unchanged)

**File:** `internal/office/service/scheduler_integration.go`

Instructions are still exported to the shared runtime dir (outside the worktree) so they can be referenced across sessions via the path directive injected into the prompt:

```go
func (si *SchedulerIntegration) exportInstructions(ctx context.Context, agent *models.AgentInstance) (string, error) {
    // Target dir: ~/.kandev/runtime/<workspace-slug>/instructions/<agentId>/
    dir := filepath.Join(si.svc.kandevBasePath(), "runtime", workspaceSlug, "instructions", agent.ID)
    return dir, si.svc.ExportInstructionsToDir(ctx, agent.ID, dir)
}
```

### 5.7 KANDEV_CLI path per executor

The `KANDEV_CLI` env var value depends on executor type:

| Executor | KANDEV_CLI value | Why |
|----------|-----------------|-----|
| `local_pc` | `launcher.BinaryPath()` (host path) | Agent runs on host |
| `worktree` | `launcher.BinaryPath()` (host path) | Agent runs on host |
| `local_docker` | `/usr/local/bin/agentctl` | Baked into image |
| `sprites` | `/usr/local/bin/agentctl` | Uploaded to sprite |

Set in `buildEnvVars()` based on executor type from the resolved config.

---

## Phase 6: Agent permissions -- enforcement + UI

### 6.1 Auth middleware

**File:** `internal/office/handlers/middleware.go` (NEW)

```go
func agentAuthMiddleware(svc *service.Service) gin.HandlerFunc {
    return func(c *gin.Context) {
        auth := c.GetHeader("Authorization")
        if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
            // No JWT -- UI request, pass through as admin
            c.Next()
            return
        }
        token := strings.TrimPrefix(auth, "Bearer ")
        claims, err := svc.ValidateAgentJWT(token)
        if err != nil {
            c.AbortWithStatusJSON(401, gin.H{"error": "invalid token"})
            return
        }
        // Load agent + resolved permissions
        agent, err := svc.GetAgentInstance(c.Request.Context(), claims.AgentInstanceID)
        if err != nil {
            c.AbortWithStatusJSON(401, gin.H{"error": "agent not found"})
            return
        }
        c.Set("agent_claims", claims)
        c.Set("agent_caller", agent)
        c.Next()
    }
}
```

Apply to the office route group in `handlers.go`:

```go
api := router.Group("/api/v1/office")
api.Use(agentAuthMiddleware(h.ctrl.Svc))
```

### 6.2 Permission resolution

**File:** `internal/office/service/permissions.go` (NEW)

Resolve effective permissions by merging role defaults with agent overrides:

```go
func ResolvePermissions(role models.AgentRole, overrides string) map[string]interface{} {
    defaults := defaultPermsForRole(role)
    if overrides == "" || overrides == "{}" {
        return defaults
    }
    var custom map[string]interface{}
    json.Unmarshal([]byte(overrides), &custom)
    // Override specific keys
    for k, v := range custom {
        defaults[k] = v
    }
    return defaults
}

func HasPermission(perms map[string]interface{}, key string) bool {
    v, ok := perms[key]
    if !ok { return false }
    b, ok := v.(bool)
    return ok && b
}
```

### 6.3 Enforce permissions in service layer

**File:** `internal/office/service/agents.go` (MODIFY)

Add permission checks to mutating operations:

```go
func (s *Service) CreateAgentInstance(ctx context.Context, agent *AgentInstance) error {
    if caller := agentCallerFromCtx(ctx); caller != nil {
        perms := ResolvePermissions(caller.Role, caller.Permissions)
        if !HasPermission(perms, "can_create_agents") {
            return ErrForbidden
        }
        // No privilege escalation: caller can't grant perms it doesn't have
        if agent.Permissions != "" {
            if err := validateNoEscalation(perms, agent.Permissions); err != nil {
                return err
            }
        }
    }
    // ... proceed with creation
}
```

Similar checks for:
- `UpdateAgentStatus` -- only CEO or admin
- `DeleteAgent` -- only CEO or admin
- Task assignment (`can_assign_tasks`)
- Approval decisions (`can_approve`)

### 6.4 Task scope enforcement

**File:** `internal/office/handlers/middleware.go`

For task-scoped operations, verify the agent is authorized:

```go
func taskScopeCheck() gin.HandlerFunc {
    return func(c *gin.Context) {
        caller, _ := c.Get("agent_caller")
        if caller == nil { c.Next(); return } // UI admin
        agent := caller.(*models.AgentInstance)
        claims := c.MustGet("agent_claims").(*service.AgentClaims)

        taskID := c.Param("id") // from route
        perms := service.ResolvePermissions(agent.Role, agent.Permissions)

        // CEO with can_assign_tasks can operate on any task
        if service.HasPermission(perms, "can_assign_tasks") {
            c.Next()
            return
        }
        // Others can only operate on their assigned task
        if claims.TaskID != "" && taskID != claims.TaskID {
            c.AbortWithStatusJSON(403, gin.H{"error": "cannot operate on unassigned task"})
            return
        }
        c.Next()
    }
}
```

### 6.5 Permissions in meta endpoint

**File:** `internal/office/handlers/meta.go` + `internal/office/dto/meta.go`

Add permission definitions and role defaults to the meta response:

```go
type PermissionMeta struct {
    Key         string `json:"key"`
    Label       string `json:"label"`
    Description string `json:"description"`
    Type        string `json:"type"` // "bool" or "int"
}

// In MetaResponse:
Permissions        []PermissionMeta                `json:"permissions"`
PermissionDefaults map[string]map[string]interface{} `json:"permissionDefaults"`
```

### 6.6 Agent detail page: Permissions tab

**File:** `apps/web/app/office/agents/[id]/components/agent-permissions-tab.tsx` (NEW)

- Fetch agent from store, get permissions + role
- Fetch meta for permission definitions + role defaults
- For each permission: show toggle switch with label and description
- Show "(role default)" badge when value matches default for that role
- `max_subtask_depth`: number input instead of toggle
- Save on change via `PATCH /agents/:id` with updated permissions JSON
- Show warning when granting powerful permissions (create_agents, approve)

**File:** `apps/web/app/office/agents/[id]/page.tsx` (MODIFY)

Add "Permissions" tab to the tab list (after Overview, before Instructions).

---

## Tests

| Test | What it verifies |
|------|-----------------|
| `TestKandevCLI_TaskGet` | Reads env vars, calls correct endpoint, returns JSON |
| `TestKandevCLI_TaskUpdate` | Sends PATCH with status + comment, includes run ID header |
| `TestKandevCLI_TaskCreate` | Sends POST with parent/assignee, returns created task |
| `TestKandevCLI_CommentAdd` | Posts comment, supports stdin mode |
| `TestKandevCLI_CommentList` | Lists comments with pagination |
| `TestKandevCLI_AgentsList` | Lists agents with role/status filters |
| `TestKandevCLI_MemorySet` | Upserts memory entry |
| `TestKandevCLI_MemoryGet` | Gets memory by layer/key |
| `TestKandevCLI_Checkout` | Calls checkout, handles 409 conflict |
| `TestKandevCLI_MissingEnv` | Returns clear error when KANDEV_API_URL missing |
| `TestKandevCLI_DefaultTaskID` | Uses KANDEV_TASK_ID when --id omitted |
| `TestModeOffice` | 5 tools registered, kanban tools excluded |
| `TestModeOffice_ExcludesKanban` | create_task, update_task, list_tasks NOT present |
| `TestEnvBuilder_KandevCLI` | KANDEV_CLI set in office env vars |
| `TestSkill_CLIExamples` | Skill references $KANDEV_CLI, no curl commands |
| `TestDeliverSkillsLocal` | Writes skills to worktree CWD, sets exclude pattern |
| `TestDeliverSkillsDocker` | Writes to worktree (host-side mount path) |
| `TestDeliverSkillsSprites` | Writes to local staging area for upload |
| `TestEnsureExcludePattern` | Adds `kandev-*` to `.git/info/exclude` idempotently |
| `TestSkillManifest` | Manifest contains all desired skills + instructions |
| `TestKandevCLI_DockerPath` | KANDEV_CLI = /usr/local/bin/agentctl for Docker |
| `TestKandevCLI_StandalonePath` | KANDEV_CLI = launcher binary path for standalone |
| `TestAuthMiddleware_ValidJWT` | Extracts agent from valid JWT, sets context |
| `TestAuthMiddleware_NoJWT` | Passes through as admin (UI request) |
| `TestAuthMiddleware_ExpiredJWT` | Returns 401 |
| `TestPermission_CEOCanCreateAgent` | CEO with can_create_agents succeeds |
| `TestPermission_WorkerCannotCreateAgent` | Worker gets 403 on create agent |
| `TestPermission_NoEscalation` | CEO can't grant perms it doesn't have |
| `TestPermission_TaskScope` | Worker can only update own assigned task |
| `TestPermission_CEOCrossTask` | CEO with can_assign_tasks can update any task |
| `TestResolvePermissions` | Role defaults merged with overrides correctly |
| `TestMetaPermissions` | Meta endpoint returns permission definitions + defaults |
| `TestPermissionsTab` | Frontend tab shows toggles, saves overrides |

## Files to create/modify

| File | Action |
|------|--------|
| `cmd/agentctl/kandev.go` | NEW: CLI router |
| `cmd/agentctl/kandev_client.go` | NEW: HTTP client with env var auth |
| `cmd/agentctl/kandev_task.go` | NEW: task get/update/create |
| `cmd/agentctl/kandev_comment.go` | NEW: comment add/list |
| `cmd/agentctl/kandev_agents.go` | NEW: agents list |
| `cmd/agentctl/kandev_memory.go` | NEW: memory get/set/summary |
| `cmd/agentctl/kandev_checkout.go` | NEW: checkout command |
| `cmd/agentctl/main.go` | MODIFY: route `kandev` subcommand before server start |
| `internal/agentctl/server/mcp/server.go` | MODIFY: add ModeOffice constant + case |
| `internal/agentctl/server/mcp/server_test.go` | MODIFY: add office mode tests |
| `internal/orchestrator/executor/executor_execute.go` | MODIFY: set ModeOffice for office sessions |
| `internal/agentctl/client/launcher/launcher.go` | MODIFY: expose BinaryPath() |
| `internal/office/service/service.go` | MODIFY: add agentctlBinaryPath field |
| `internal/office/service/env_builder.go` | MODIFY: inject KANDEV_CLI per executor type |
| `internal/office/service/skill_manifest.go` | NEW: SkillManifest model + builder |
| `internal/office/service/skill_delivery.go` | NEW: executor-aware delivery (local/docker/sprites) |
| `internal/office/service/scheduler_integration.go` | MODIFY: use manifest + delivery instead of unconditional export |
| `internal/agent/lifecycle/container.go` | MODIFY: add kandev_runtime_dir mount for office |
| `internal/agent/lifecycle/executor_sprites_operations.go` | MODIFY: add uploadSkillFiles step |
| `internal/office/configloader/skills/kandev-protocol/SKILL.md` | REWRITE: curl → CLI examples |
| `cmd/kandev/main.go` | MODIFY: pass agentctl binary path to office service |
| `internal/office/handlers/middleware.go` | NEW: JWT auth middleware + task scope check |
| `internal/office/service/permissions.go` | NEW: ResolvePermissions, HasPermission, validateNoEscalation |
| `internal/office/service/agents.go` | MODIFY: add permission checks to create/update/delete |
| `internal/office/handlers/handlers.go` | MODIFY: apply auth middleware to route group |
| `internal/office/handlers/meta.go` | MODIFY: add permissions + defaults to meta response |
| `internal/office/dto/meta.go` | MODIFY: add PermissionMeta type |
| `agents/[id]/components/agent-permissions-tab.tsx` | NEW: permissions toggle UI |
| `agents/[id]/page.tsx` | MODIFY: add Permissions tab |
