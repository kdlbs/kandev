# Office Agent Context Implementation Plan

**Date:** 2026-04-27
**Status:** proposed
**Spec:** `docs/specs/office-agent-context/spec.md`

## Phase 1: Instruction bundle storage + UI (backend + frontend)

### Backend

**DB schema** -- add `office_agent_instructions` table:
```sql
CREATE TABLE IF NOT EXISTS office_agent_instructions (
    id TEXT PRIMARY KEY,
    agent_instance_id TEXT NOT NULL,
    filename TEXT NOT NULL,       -- "AGENTS.md", "HEARTBEAT.md", etc.
    content TEXT NOT NULL DEFAULT '',
    is_entry INTEGER DEFAULT 0,   -- 1 for AGENTS.md (the injected file)
    created_at DATETIME,
    updated_at DATETIME,
    UNIQUE(agent_instance_id, filename)
);
```

**Repository** (`repository/sqlite/instructions.go`):
- `ListInstructions(ctx, agentInstanceID) -> []*InstructionFile`
- `GetInstruction(ctx, agentInstanceID, filename) -> *InstructionFile`
- `UpsertInstruction(ctx, agentInstanceID, filename, content, isEntry) error`
- `DeleteInstruction(ctx, agentInstanceID, filename) error`

**Service** (`service/instructions.go`):
- `CreateDefaultInstructions(ctx, agentInstanceID, role string) error` -- create AGENTS.md, HEARTBEAT.md from role templates
- `ExportInstructionsToDir(ctx, agentInstanceID, targetDir string) error` -- write all files to disk
- Role templates embedded in binary (like bundled skills)

**Handlers** (`handlers/instructions.go`):
- `GET /api/v1/office/agents/:id/instructions` -> list files
- `GET /api/v1/office/agents/:id/instructions/:filename` -> get content
- `PUT /api/v1/office/agents/:id/instructions/:filename` -> create/update
- `DELETE /api/v1/office/agents/:id/instructions/:filename` -> delete

**On agent creation** (`service/agents.go`):
- After creating agent instance, call `CreateDefaultInstructions(ctx, agentID, role)`
- CEO gets AGENTS.md + HEARTBEAT.md with delegation/checklist content
- Worker gets AGENTS.md with implementation guide
- Reviewer gets AGENTS.md with review checklist

### Frontend

**Agent detail Instructions tab** (`agents/[id]/components/agent-instructions-tab.tsx`):
- Left panel: file list (AGENTS.md marked ENTRY, HEARTBEAT.md, SOUL.md, TOOLS.md)
- Right panel: markdown editor for selected file
- "+" button to add new instruction file
- Save/delete buttons
- Fetch from `GET /agents/:id/instructions`

### Role templates

Embed in binary at `configloader/instructions/`:
```
configloader/instructions/
  ceo/
    AGENTS.md       -- delegation rules, routing table
    HEARTBEAT.md    -- 8-step wakeup checklist
  worker/
    AGENTS.md       -- implementation guide
  reviewer/
    AGENTS.md       -- review checklist
  assistant/
    AGENTS.md       -- conversation + channel handling
```

---

## Phase 2: Session preparation -- export + inject (backend)

### Export instructions to disk

**File:** `service/scheduler_integration.go`

Before launching agent, export instruction files from DB to disk:
```go
func (si *SchedulerIntegration) exportInstructions(ctx context.Context, agent *models.AgentInstance) (string, error) {
    // Target dir: ~/.kandev/runtime/<workspace-slug>/instructions/<agentId>/
    dir := filepath.Join(si.svc.kandevBasePath(), "runtime", workspaceSlug, "instructions", agent.ID)
    return dir, si.svc.ExportInstructionsToDir(ctx, agent.ID, dir)
}
```

### Export skills to disk

Already implemented in `skill_materialization.go` -- `MaterializeSkills` writes to cache dir. Verify it's called in the session prep flow.

### Build prompt with AGENTS.md + path directive

**File:** `service/prompt_builder.go` (rewrite)

```go
func (s *Service) BuildAgentPrompt(ctx context.Context, wakeup *WakeupRequest, agent *AgentInstance, instructionsDir string, isResume bool) string {
    var sections []string

    // Section 1: Instructions (skip on resume)
    if !isResume {
        agentsMd := readFile(filepath.Join(instructionsDir, "AGENTS.md"))
        // Append path directive
        agentsMd += fmt.Sprintf("\n\nThe above instructions were loaded from %s/AGENTS.md.\nResolve relative file references from %s.\nThis directory contains: ./HEARTBEAT.md, ./SOUL.md, ./TOOLS.md.\n", instructionsDir, instructionsDir)
        sections = append(sections, agentsMd)
    }

    // Section 2: Wake context (always)
    sections = append(sections, buildWakeContext(wakeup))

    // Section 3: Workspace status (CEO heartbeat only)
    if wakeup.Reason == "heartbeat" && agent.Role == "ceo" {
        sections = append(sections, buildWorkspaceStatus(ctx))
    }

    return strings.Join(sections, "\n\n---\n\n")
}
```

### Set env vars

**File:** `service/scheduler_integration.go`

```go
func (si *SchedulerIntegration) buildEnvVars(wakeup *WakeupRequest, agent *AgentInstance, jwt, workspaceID string) map[string]string {
    env := map[string]string{
        "KANDEV_API_URL":      si.svc.apiBaseURL,
        "KANDEV_API_KEY":      jwt,
        "KANDEV_AGENT_ID":     agent.ID,
        "KANDEV_AGENT_NAME":   agent.Name,
        "KANDEV_WORKSPACE_ID": workspaceID,
        "KANDEV_RUN_ID":       wakeup.ID,
        "KANDEV_WAKE_REASON":  wakeup.Reason,
    }
    if taskID := extractTaskID(wakeup.Payload); taskID != "" {
        env["KANDEV_TASK_ID"] = taskID
    }
    if commentID := extractCommentID(wakeup.Payload); commentID != "" {
        env["KANDEV_WAKE_COMMENT_ID"] = commentID
    }
    return env
}
```

### Build wake payload

```go
func (s *Service) BuildWakePayload(ctx context.Context, wakeup *WakeupRequest) (string, error) {
    taskID := extractTaskID(wakeup.Payload)
    if taskID == "" { return "", nil }

    task, _ := s.repo.GetTaskBasicInfo(ctx, taskID)
    comments, _ := s.repo.ListRecentComments(ctx, taskID, limit)

    payload := WakePayload{Task: mapTask(task), NewComments: mapComments(comments)}
    data, _ := json.Marshal(payload)
    return string(data), nil
}
```

Set as env var:
```go
if payload, err := si.svc.BuildWakePayload(ctx, wakeup); err == nil && payload != "" {
    env["KANDEV_WAKE_PAYLOAD_JSON"] = payload
}
```

### Pass env + instructions dir to task starter

Update `TaskStarter` interface to accept env vars and instructions dir path. The lifecycle manager uses:
- `--add-dir <instructionsDir>` for Claude (agent can read sibling files)
- `--append-system-prompt-file <instructionsDir>/AGENTS.md` for Claude
- Prepend AGENTS.md content to prompt for other agents

---

## Phase 3: Enhanced kandev-protocol skill (content only)

Rewrite `configloader/skills/kandev-protocol/SKILL.md` from 43 lines to ~200 lines:

- Authentication section (env vars, JWT, run ID header)
- 8-step heartbeat procedure
- API reference (tasks, comments, agents, memory endpoints)
- Critical rules (no 409 retry, comment before status change, check blockers)
- Resume fast path (use KANDEV_WAKE_PAYLOAD_JSON)

---

## Tests

- Instruction CRUD: create, read, update, delete
- Default templates: CEO agent gets AGENTS.md + HEARTBEAT.md on creation
- Export to dir: files written to correct paths
- Prompt builder: includes AGENTS.md content + path directive on fresh, skips on resume
- Env vars: all 10 vars populated correctly
- Wake payload: includes task details + recent comments
- Protocol skill: valid markdown, references correct endpoints

## Files to create/modify

| File | Action |
|------|--------|
| `repository/sqlite/instructions.go` | NEW: instruction CRUD |
| `repository/sqlite/base.go` | ADD: office_agent_instructions table |
| `service/instructions.go` | NEW: defaults, export |
| `service/agents.go` | MODIFY: create defaults on agent creation |
| `service/scheduler_integration.go` | MODIFY: export + env vars + wake payload |
| `service/prompt_builder.go` | MODIFY: multi-section prompt with path directive |
| `handlers/instructions.go` | NEW: REST endpoints |
| `handlers/handlers.go` | MODIFY: register routes |
| `configloader/instructions/ceo/AGENTS.md` | NEW: CEO template |
| `configloader/instructions/ceo/HEARTBEAT.md` | NEW: CEO checklist |
| `configloader/instructions/worker/AGENTS.md` | NEW: worker template |
| `configloader/instructions/reviewer/AGENTS.md` | NEW: reviewer template |
| `configloader/skills/kandev-protocol/SKILL.md` | REWRITE: 200+ lines |
| `agents/[id]/components/agent-instructions-tab.tsx` | NEW: frontend tab |
| `office-api.ts` | ADD: instruction API functions |
