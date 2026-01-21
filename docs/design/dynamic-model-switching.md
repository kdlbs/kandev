# Dynamic Model Switching Feature Analysis

> **Status**: Design Document
> **Created**: 2026-01-21
> **Scope**: Mid-session model switching within the Kandev application

## Executive Summary

This document analyzes the implementation of dynamic model switching, allowing users to change the AI model mid-session without starting a new session. The feature requires UI changes to expose model selection, backend logic to detect model changes and restart agents, and careful handling of session continuity.

## Current Architecture Overview

### Session Lifecycle (Current Flow)

```
User clicks "Start Session"
    → Frontend sends orchestrator.start with agentProfileId
    → Orchestrator.StartTask → Executor.ExecuteWithProfile
    → Lifecycle Manager resolves profile → gets model from AgentProfileInfo
    → CommandBuilder.BuildCommand adds --model flag
    → Agent process started with fixed model
    → ACP session initialized
    → User sends prompts via session/prompt
```

**Key Insight**: Model is determined at session start via `AgentProfileInfo.Model` and baked into the agent command. There's no current mechanism to change the model mid-session.

### Relevant Components

| Component | Path | Role |
|-----------|------|------|
| TaskChatPanel | `apps/web/components/task/task-chat-panel.tsx` | Chat UI, displays current model |
| SessionsDropdown | `apps/web/components/task/sessions-dropdown.tsx` | Session switcher |
| Orchestrator Service | `apps/backend/internal/orchestrator/service.go` | PromptTask, StartTask, ResumeTaskSession |
| Executor | `apps/backend/internal/orchestrator/executor/executor.go` | Agent launch, prompt forwarding |
| Lifecycle Manager | `apps/backend/internal/agent/lifecycle/manager.go` | Agent process management |
| CommandBuilder | `apps/backend/internal/agent/lifecycle/command.go` | Builds CLI args including --model |
| ProfileResolver | `apps/backend/internal/agent/lifecycle/profile_resolver.go` | Resolves profile → model |
| SessionManager | `apps/backend/internal/agent/lifecycle/session.go` | ACP session init/prompt |
| TaskSession Model | `apps/backend/internal/task/models/models.go` | Session persistence |
| Message Service | `apps/backend/internal/task/service/service.go` | CreateMessage |

---

## Requirements Breakdown

### UI Requirements

1. **Model Selector in Session View**: Replace existing agent selector with model dropdown
2. **Deferred Application**: Selection does NOT take effect immediately
3. **Apply on Prompt**: Selected model applied when user sends next prompt
4. **Persistent State**: All subsequent prompts use new model until changed again

### Backend Requirements

1. **Model Change Detection**: Compare current model vs requested model on prompt
2. **System Message Insertion**: "Model switching to [model name]" message
3. **Agent Restart**: Stop current agent, restart with new `--model` flag
4. **Session Resumption**: Attempt to resume ACP session after restart (if supported)

---

## Technical Approach

### Phase 1: Frontend Model Selector

#### 1.1 Store State for Pending Model

Add to the Zustand store a per-session "pending model" that tracks what model the user wants to switch to.

**Files to modify:**
- `apps/web/lib/state/store.ts` - Add `pendingModelBySessionId: Record<string, string>`
- `apps/web/lib/types/http.ts` - Add model-related types if needed

#### 1.2 Model Selector Component

Create a model dropdown in the chat panel that:
- Shows available models for the current agent type
- Shows current model (from session's agent profile)
- Allows selection of a different model
- Stores selection in `pendingModelBySessionId[sessionId]`

**Files to modify:**
- `apps/web/components/task/task-chat-panel.tsx` - Add ModelSelector component
- May create new component: `apps/web/components/task/model-selector.tsx`

**Data source for available models:**
- From `AvailableAgent.model_config.available_models` in the agent discovery store
- Need to resolve which agent the session is using via `session.agent_profile_id`

#### 1.3 Prompt Submission with Model Override

When user submits a prompt, check if `pendingModelBySessionId[sessionId]` differs from current model:
- If different: include `model` field in WebSocket prompt payload
- Clear pending model after submission (backend will handle the switch)

**WebSocket payload change:**

```typescript
// Current
{ action: "orchestrator.prompt", payload: { taskId, sessionId, content } }

// Proposed
{ action: "orchestrator.prompt", payload: { taskId, sessionId, content, model?: string } }
```

**Files to modify:**
- `apps/web/lib/ws/actions.ts` - Add `model` to prompt action payload
- `apps/web/components/task/task-chat-panel.tsx` - Include model in prompt submission

---

### Phase 2: Backend Model Change Detection

#### 2.1 Extend Prompt Handling

Modify the orchestrator's `PromptTask` to accept an optional model parameter.

**Files to modify:**
- `apps/backend/internal/orchestrator/service.go` - `PromptTask` method
- `apps/backend/internal/ws/handlers/orchestrator.go` - Parse model from WS payload

**Logic flow:**
```
PromptTask(taskId, sessionId, content, model)
    → If model != currentExecution.Model:
        → Insert system message "Switching to {model}..."
        → Stop current agent (graceful shutdown)
        → Update session's effective model
        → Restart agent with new --model flag
        → Resume or reinitialize ACP session
    → Forward prompt to agent
```

#### 2.2 Executor Model Switch

The executor needs a new method to handle model switching:

**Files to modify:**
- `apps/backend/internal/orchestrator/executor/executor.go`
  - Add `SwitchModel(sessionId, newModel)` method
  - Coordinates stop → rebuild → restart → resume

**Implementation sketch:**
```go
func (e *Executor) SwitchModel(ctx context.Context, sessionId, newModel string) error {
    exec, ok := e.running[sessionId]
    if !ok {
        return ErrSessionNotFound
    }

    // 1. Stop current agent
    e.lifecycleManager.StopAgent(ctx, exec.AgentID)

    // 2. Update profile with new model
    newProfileInfo := exec.ProfileInfo
    newProfileInfo.Model = newModel

    // 3. Restart agent with new command
    newAgentID, err := e.lifecycleManager.Launch(ctx, LaunchOpts{
        Model: newModel,
        // ... other opts from exec
    })

    // 4. Attempt session resume
    e.lifecycleManager.InitializeSession(ctx, newAgentID, InitOpts{
        IsResume: true,
        ResumeToken: exec.ResumeToken,
    })

    return nil
}
```

#### 2.3 System Message for Model Switch

Insert a visible message in the chat indicating the model change.

**Files to modify:**
- `apps/backend/internal/task/service/service.go` - Use `CreateMessage` with type `status`

**Message format:**
```json
{
  "type": "status",
  "content": "Switching model to Claude Opus 4.5...",
  "metadata": {
    "status_type": "model_switch",
    "from_model": "sonnet4.5",
    "to_model": "opus4.5"
  }
}
```

---

### Phase 3: Session Resumption After Restart

#### 3.1 Agent-Specific Resumption

Different agents handle resumption differently (from `agents.json`):

| Agent | `resume_via_acp` | `resume_flag` | `can_recover` |
|-------|------------------|---------------|---------------|
| auggie-agent | false | `--resume` | false |
| codex-agent | true | (none) | true |

**For Codex-style agents (resume_via_acp: true):**
- Use ACP `session/load` with the stored `ResumeToken`
- Session history is preserved by the agent

**For Auggie-style agents (resume_via_acp: false, can_recover: false):**
- Cannot truly resume; must reinitialize with fresh ACP session
- **Decision**: Accept context loss, display system message: "Model switched. A new session was started."

#### 3.2 Handling Resume Failures

**Files to modify:**
- `apps/backend/internal/agent/lifecycle/session.go` - `InitializeSession` error handling

**Fallback strategy:**
1. Attempt resume with stored token
2. If resume fails → start new session
3. Emit warning message to user: "Session could not be fully resumed after model switch"


---

## Integration Points

### WebSocket Integration

| Layer | Change Required |
|-------|-----------------|
| Client (`apps/web/lib/ws/client.ts`) | No changes |
| Actions (`apps/web/lib/ws/actions.ts`) | Add `model` field to prompt action |
| Handler (`apps/backend/internal/ws/handlers/orchestrator.go`) | Parse `model` from payload |

**Wire format example:**
```json
{
  "id": "uuid",
  "type": "request",
  "action": "orchestrator.prompt",
  "payload": {
    "task_id": "task-uuid",
    "session_id": "session-uuid",
    "content": "User message",
    "model": "opus4.5"
  }
}
```

### Store Integration

**New store slice additions:**

```typescript
// In store.ts
pendingModel: {
  bySessionId: Record<string, string>, // sessionId → pending model id
  setPendingModel: (sessionId: string, modelId: string) => void,
  clearPendingModel: (sessionId: string) => void,
  getPendingModel: (sessionId: string) => string | null,
}
```

**Store → Component data flow:**
```
ModelSelector reads: settingsAgents (for available models), session.agent_profile_id
ModelSelector writes: pendingModel.bySessionId[sessionId]
TaskChatPanel reads: pendingModel.bySessionId[sessionId] on prompt submit
```

### Database Integration

**Decision: No schema change (runtime-only)**
- Model switch is runtime-only; session's `agent_profile_id` remains unchanged
- On backend restart, session resumes with original profile model
- No migration required

---

## Challenges and Edge Cases

### 1. Agent Busy During Model Switch Request

**Scenario**: User selects new model while agent is processing a previous prompt.

**Solution options:**
- **Queue the switch**: Store pending model, apply after current response completes
- **Block selection**: Disable model selector while agent is busy
- **Immediate abort**: Cancel current response, switch model (destructive)

**Recommendation**: Queue the switch; apply on next user-initiated prompt.

### 2. Rapid Model Changes

**Scenario**: User changes model multiple times before sending a prompt.

**Solution**: Only the final selection matters; each change overwrites `pendingModelBySessionId[sessionId]`.

### 3. Resume Failure After Switch

**Scenario**: Agent restarts with new model but ACP session/load fails.

**Handling:**
1. Log error for debugging
2. Fall back to new session initialization
3. Show user message: "Model switched successfully but session context could not be fully restored"

### 4. Different Agent Types

**Scenario**: User tries to switch to a model not supported by current agent type.

**Prevention**: Model selector only shows models from `agent.model_config.available_models` for the session's agent type.

### 5. Session State Persistence

**Scenario**: Backend restarts mid-session; what model should resume use?

**Behavior**: Uses `agent_profile_snapshot.model` from session creation (original profile model). Runtime model switches are not persisted.

### 6. Model Availability Changes

**Scenario**: A model is removed from agent configuration while sessions are active.

**Handling**: Validate model against current `available_models` before sending to backend. If invalid, fall back to session's original model.

---

## Alternative Approaches

### Alternative A: Runtime Model Switching via ACP

**Concept**: Send a hypothetical `session/switch_model` ACP call without restarting agent.

**Pros:**
- No process restart; faster switch
- Seamless context preservation

**Cons:**
- Not supported by current agent implementations
- Would require agent CLI changes upstream
- Different agents would need different implementations

**Verdict**: Not viable without significant upstream work.

### Alternative B: Model as Separate Session Concept

**Concept**: Treat each model switch as a "sub-session" within the same UI session.

**Pros:**
- Clean separation of concerns
- Easy to track model history

**Cons:**
- Complex UI implications
- Breaks current session model significantly
- Overhead for simple model changes

**Verdict**: Over-engineering for the requirement.

### Alternative C: Pre-select Model Before Session Start Only

**Concept**: Allow model selection only at session creation, not mid-session.

**Pros:**
- Simplest implementation
- No restart complexity

**Cons:**
- Doesn't meet the stated requirement
- Poor UX for iterative work

**Verdict**: Does not satisfy user requirements.

---

## Implementation Phases

### Phase 1: Frontend Model Selector (Est. 2-3 days)

**Goal**: Model dropdown appears in UI; selection stored locally.

**Files to modify:**
1. `apps/web/lib/state/store.ts` - Add pendingModel slice
2. `apps/web/components/task/task-chat-panel.tsx` - Add ModelSelector
3. Create `apps/web/components/task/model-selector.tsx` - New component

**Deliverable**: User can select a model, but it has no effect yet.

### Phase 2: Backend Prompt Extension (Est. 2-3 days)

**Goal**: Prompts can include model; backend detects change.

**Files to modify:**
1. `apps/web/lib/ws/actions.ts` - Add model to prompt payload
2. `apps/backend/internal/ws/handlers/orchestrator.go` - Parse model
3. `apps/backend/internal/orchestrator/service.go` - PromptTask accepts model

**Deliverable**: Model parameter flows from UI to orchestrator.

### Phase 3: Agent Restart Logic (Est. 3-4 days)

**Goal**: Backend restarts agent with new model when detected.

**Files to modify:**
1. `apps/backend/internal/orchestrator/executor/executor.go` - SwitchModel method
2. `apps/backend/internal/agent/lifecycle/manager.go` - Ensure clean restart
3. `apps/backend/internal/task/service/service.go` - Insert status message

**Deliverable**: Model switch triggers agent restart; status message appears.

### Phase 4: Session Resumption (Est. 2-3 days)

**Goal**: Resume ACP session after agent restart (where supported).

**Files to modify:**
1. `apps/backend/internal/agent/lifecycle/session.go` - Resume logic
2. `apps/backend/internal/orchestrator/executor/executor.go` - Handle resume failures

**Deliverable**: Codex-style agents resume context; Auggie-style gracefully degrade.

### Phase 5: Testing & Edge Cases (Est. 2-3 days)

**Goal**: Verify all scenarios work correctly.

**Test scenarios:**
- Model switch while agent idle
- Model switch while agent busy (queued)
- Rapid model changes
- Resume failure fallback
- Invalid model selection
- Backend restart mid-session

---

## Summary of Files to Modify

| File | Changes |
|------|---------|
| `apps/web/lib/state/store.ts` | Add pendingModel slice |
| `apps/web/components/task/task-chat-panel.tsx` | Integrate ModelSelector, pass model on prompt |
| `apps/web/components/task/model-selector.tsx` | New component |
| `apps/web/lib/ws/actions.ts` | Add model field to prompt action |
| `apps/backend/internal/ws/handlers/orchestrator.go` | Parse model from WS payload |
| `apps/backend/internal/orchestrator/service.go` | Extend PromptTask signature |
| `apps/backend/internal/orchestrator/executor/executor.go` | Add SwitchModel method |
| `apps/backend/internal/agent/lifecycle/manager.go` | Ensure clean restart support |
| `apps/backend/internal/agent/lifecycle/session.go` | Handle resume after restart |
| `apps/backend/internal/task/service/service.go` | Create model switch status message |

---

## Estimated Total Effort

| Phase | Estimate |
|-------|----------|
| Phase 1: Frontend Model Selector | 2-3 days |
| Phase 2: Backend Prompt Extension | 2-3 days |
| Phase 3: Agent Restart Logic | 3-4 days |
| Phase 4: Session Resumption | 2-3 days |
| Phase 5: Testing & Edge Cases | 2-3 days |
| **Total** | **11-16 days** |

---

## Resolved Decisions

| Decision | Resolution |
|----------|------------|
| Database schema | Runtime-only, no schema changes |
| Non-recoverable agents (Auggie) | Accept context loss, show "new session started" message |
| Agent busy during switch | Not an issue - model applied on next user prompt |
| UI placement | Replace existing agent selector with model selector |
| Confirmation dialog | None required - seamless switch |

