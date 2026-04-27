# Orchestrate Agent Context Implementation Plan

**Date:** 2026-04-27
**Status:** proposed
**Spec:** `docs/specs/orchestrate-agent-context/spec.md`

## Phase 1: Environment Variables (4h)

### 1A: Build env var map in scheduler

**File:** `internal/orchestrate/service/scheduler_integration.go`

In `processWakeup()`, after resolving executor and building prompt, construct the env var map:

```go
func (si *SchedulerIntegration) buildEnvVars(
    wakeup *models.WakeupRequest,
    agent *models.AgentInstance,
    workspaceID string,
) map[string]string {
    env := map[string]string{
        "KANDEV_API_URL":        si.svc.apiBaseURL,
        "KANDEV_API_KEY":        jwt,  // already minted via agent_auth.go
        "KANDEV_AGENT_ID":       agent.ID,
        "KANDEV_AGENT_NAME":     agent.Name,
        "KANDEV_WORKSPACE_ID":   workspaceID,
        "KANDEV_RUN_ID":         wakeup.ID,
        "KANDEV_WAKE_REASON":    wakeup.Reason,
    }
    // Task-specific vars
    taskID := extractTaskID(wakeup.Payload)
    if taskID != "" {
        env["KANDEV_TASK_ID"] = taskID
    }
    commentID := extractCommentID(wakeup.Payload)
    if commentID != "" {
        env["KANDEV_WAKE_COMMENT_ID"] = commentID
    }
    return env
}
```

### 1B: Pass env vars to task starter

The `TaskStarter` interface needs to accept env vars. Update the interface:

```go
type TaskStarter interface {
    StartTask(ctx context.Context, taskID, agentProfileID, executorID,
        executorProfileID string, priority int, prompt, workflowStepID string,
        planMode bool, attachments []v1.MessageAttachment,
        env map[string]string,  // NEW
    ) error
}
```

The orchestrator's `StartTask` already supports env vars via the agent lifecycle manager. The adapter just needs to pass them through.

### 1C: Add apiBaseURL to service

The service needs to know the backend's URL. Add it as a config field:

```go
func (s *Service) SetAPIBaseURL(url string) { s.apiBaseURL = url }
```

Wire in `main.go` from `cfg.Server.Port`.

### Tests
- Build env vars: all vars populated correctly
- Task ID extracted from payload
- Comment ID extracted from payload

---

## Phase 2: Wake Payload (6h)

### 2A: Build wake payload JSON

**File:** `internal/orchestrate/service/prompt_builder.go` (extend)

```go
func (s *Service) BuildWakePayload(ctx context.Context, wakeup *models.WakeupRequest) (string, error) {
    taskID := extractTaskID(wakeup.Payload)
    if taskID == "" {
        return "", nil  // non-task wakeup (heartbeat)
    }

    // Fetch task details
    task, err := s.repo.GetTaskBasicInfo(ctx, taskID)

    // Fetch recent comments (last N since last run, or all if fresh)
    comments, err := s.repo.ListRecentComments(ctx, taskID, lastRunAt, limit)

    // Build payload struct
    payload := WakePayload{
        Task: WakePayloadTask{
            ID: task.ID, Identifier: task.Identifier,
            Title: task.Title, Description: task.Description,
            Status: task.Status, Priority: task.Priority,
            // ... project, assignee, blockedBy, childTasks
        },
        NewComments: mapComments(comments),
        CommentWindow: CommentWindow{
            Total: totalCount, Included: len(comments),
            FetchMore: totalCount > len(comments),
        },
    }

    return json.Marshal(payload)
}
```

### 2B: Set KANDEV_WAKE_PAYLOAD_JSON env var

In `buildEnvVars()`:
```go
payloadJSON, err := si.svc.BuildWakePayload(ctx, wakeup)
if err == nil && payloadJSON != "" {
    env["KANDEV_WAKE_PAYLOAD_JSON"] = payloadJSON
}
```

### 2C: Add repo methods for comment fetching

**File:** `repository/sqlite/comments.go` (extend)

- `ListRecentComments(ctx, taskID string, since time.Time, limit int) -> []*models.TaskComment`
- `CountComments(ctx, taskID string) -> int`

### Tests
- Wake payload includes task details
- Wake payload includes only new comments (since last run)
- Comment window metadata correct
- Non-task wakeup returns empty payload

---

## Phase 3: Instruction Bundles (8h)

### 3A: Write role-specific instruction skills

Create skill directories embedded in the binary:

```
configloader/skills/
  kandev-protocol/SKILL.md    # exists, needs expansion
  memory/SKILL.md             # exists
  kandev-ceo/SKILL.md         # NEW
  kandev-worker/SKILL.md      # NEW
  kandev-reviewer/SKILL.md    # NEW
```

Each is a SKILL.md following the content in the spec.

### 3B: Auto-assign role skills to agents

When a CEO agent is created, auto-add `kandev-ceo` to its `desired_skills`.
When a worker is created, auto-add `kandev-worker`.

In `agents.go` `CreateAgentInstance()`:
```go
// Auto-assign role instruction skill
roleSkill := "kandev-" + agent.Role  // "kandev-ceo", "kandev-worker"
if !containsSkill(agent.DesiredSkills, roleSkill) {
    agent.DesiredSkills = appendSkill(agent.DesiredSkills, roleSkill)
}
// Always include protocol + memory
for _, required := range []string{"kandev-protocol", "memory"} {
    if !containsSkill(agent.DesiredSkills, required) {
        agent.DesiredSkills = appendSkill(agent.DesiredSkills, required)
    }
}
```

### 3C: Update bundled.go to include new skills

The `embed.FS` in `bundled.go` already picks up all dirs under `configloader/skills/`. Just adding the new directories is enough.

### Tests
- CEO agent created -> has kandev-ceo, kandev-protocol, memory in desired_skills
- Worker agent -> has kandev-worker, kandev-protocol, memory
- Bundled skills include all role skills

---

## Phase 4: Enhanced Protocol Skill (10h)

### 4A: Rewrite kandev-protocol SKILL.md

Expand from 43 lines to ~200 lines covering:
- Authentication (env vars, JWT, run ID header)
- 8-step heartbeat procedure
- API reference (tasks CRUD, comments, agents, memory)
- Critical rules (no 409 retry, always comment before status change)
- Resume delta fast path (use KANDEV_WAKE_PAYLOAD_JSON)

Content is fully specified in the spec.

### 4B: Add comment endpoints to handlers

Check that these endpoints exist and work:
- `GET /api/v1/orchestrate/tasks/:id/comments`
- `POST /api/v1/orchestrate/tasks/:id/comments`

If they route to the existing `task_comments` repo, verify the DTO format matches what the protocol skill documents.

### 4C: Add task status update endpoint

Check that `PATCH /api/v1/orchestrate/tasks/:id` accepts `{"status": "done"}` and moves the task to the correct orchestrate workflow step.

### Tests
- Protocol skill content is valid markdown
- Comment endpoints return expected format
- Task status update moves workflow step

---

## Phase 5: Prompt Structure (4h)

### 5A: Multi-section prompt builder

Refactor `prompt_builder.go` to build a sectioned prompt:

```go
func (s *Service) BuildSessionPrompt(ctx context.Context, wakeup *models.WakeupRequest, isResume bool) string {
    var sections []string

    // Section 1: Instructions (skip on resume)
    if !isResume {
        // Instructions come via skills (symlinked), not inline in prompt
        // But we can add a brief "You are [agent name], a [role] agent" header
        sections = append(sections, buildIdentityHeader(agent))
    }

    // Section 2: Wake context (always)
    sections = append(sections, buildWakeContext(wakeup))

    // Section 3: Workspace status (CEO heartbeat only)
    if wakeup.Reason == WakeupReasonHeartbeat && agent.Role == "ceo" {
        sections = append(sections, buildWorkspaceStatus(ctx))
    }

    return strings.Join(sections, "\n\n---\n\n")
}
```

### 5B: Resume detection

In `scheduler_integration.go`, check if this is a resume:
```go
isResume := existingSessionID != "" && canResumeSession(agent, task)
prompt := si.svc.BuildSessionPrompt(ctx, wakeup, isResume)
```

### Tests
- Fresh session: includes identity header + wake context
- Resume session: only wake context (no identity)
- CEO heartbeat: includes workspace status section

---

## Verification

1. `make -C apps/backend fmt && make -C apps/backend test` -- all pass
2. Scheduler wakes agent -> env vars set correctly in process
3. Wake payload JSON parseable by agent
4. Agent can call kandev API with JWT from env
5. Role-specific skills are symlinked for each agent type
6. Resume wakeup sends only new comments (not full context)
