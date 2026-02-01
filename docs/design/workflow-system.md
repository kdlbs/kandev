# Workflow System Design

## Overview

A **Workflow** defines the lifecycle of **task sessions** on a board through a sequence of **Steps**. Each step maps to a column and defines:
- **Behavior**: What happens when a session enters the step (auto-start agent, append prompts, etc.)
- **Transitions**: Rules for moving to next/previous steps

### Session-Level Workflows

**Important**: Workflows operate at the **session level**, not the task level. A task can have multiple sessions (different attempts, different agents, parallel work), and each session progresses through the workflow independently.

```
Task
 ├── Session 1 (Claude) ──▶ Planning ──▶ Review ──▶ Implementation ──▶ Done ✓
 ├── Session 2 (Codex)  ──▶ Planning ──▶ Review ──▶ (rejected, abandoned)
 └── Session 3 (Claude) ──▶ Planning ──▶ (in progress...)  ← PRIMARY
```

**Rationale**:
- A task might need multiple attempts before completion
- Different agents can work on the same task in parallel
- Review feedback and approval are per-session (each attempt is reviewed independently)
- Session history provides audit trail of all attempts

### Primary Session

Each task has a **primary session** - the session that represents the task's current workflow position and state.

- **Default**: The latest created session automatically becomes primary
- **User can change**: Pin a different session as primary if needed
- **Determines task display**: Task card appears in the column of the primary session's step
- **Default for operations**: Approve/reject buttons operate on the primary session

```
Task: "Implement auth"
├── Session 1 (done)      → archived, was primary before
├── Session 2 (abandoned) → user abandoned after rejection
└── Session 3 (active)    → PRIMARY - task appears in this session's column
```

When a new session is created, it automatically becomes the primary session (latest attempt is most relevant). Other sessions become "alternative attempts" - viewable but not the main path.

## Design Decisions

### Columns ARE Workflow Steps

Simplify the model: **columns and workflow steps are the same entity**. The `columns` table is replaced by `workflow_steps`.

```
Board → Workflow (required)
     → Workflow Steps (these ARE the columns)
```

**Rationale**:
- **Simplicity**: One entity instead of two with a relationship
- **No indirection**: Step behavior is directly on the column
- **Cleaner data model**: Every board has a workflow, every column is a step

### Rejection Behavior: Stay in Review

When a user rejects at a review step, the **session stays in the same step** but gets marked with "changes requested" status. This enables iterative review cycles without moving the session back and forth.

**How it works**:
1. User clicks "Request Changes" and provides feedback
2. Session stays in the review step
3. Session metadata is updated with rejection info
4. Agent can be re-triggered to address the feedback
5. User reviews again, can approve or request more changes
6. On approval, session moves to the configured `onApproval` step

**Session metadata on rejection**:
```json
{
  "review_status": "changes_requested",
  "review_feedback": [
    {
      "id": "fb-1",
      "message": "Need to handle edge case X",
      "created_at": "2024-01-15T10:30:00Z",
      "resolved": false
    },
    {
      "id": "fb-2",
      "message": "Add unit tests for the new function",
      "created_at": "2024-01-15T11:00:00Z",
      "resolved": true
    }
  ],
  "review_iteration": 2
}
```

### Custom Workflows Supported

Users can:
1. **Use a template as-is** - Quick start with pre-configured workflow
2. **Customize a template** - Clone a template and modify steps
3. **Create from scratch** - Blank workflow, add steps manually

The `workflow_templates` table has `is_system` flag:
- `is_system = 1`: Built-in templates (read-only, can be cloned)
- `is_system = 0`: User-created templates (fully editable)

---

## Key Concepts

### 1. Workflow Template

A pre-defined workflow type that boards can adopt. Examples:

| Template | Description | Steps |
|----------|-------------|-------|
| `dev` | Software development | Backlog → Planning → Implementation → Review → Done |
| `architecture` | System design | Backlog → Research → Design → Review → Approved |
| `bug-fix` | Bug resolution | Triage → Reproduce → Fix → Verify → Done |
| `simple` | Basic kanban | Todo → In Progress → Done |

### 2. Workflow Step

Each step has:

```typescript
interface WorkflowStep {
  id: string;
  name: string;                    // Display name (e.g., "Planning")
  position: number;                // Order in workflow
  stepType: WorkflowStepType;      // Semantic type (see below)

  // Behavior configuration
  behavior: {
    autoStartAgent: boolean;       // Auto-start agent when task enters
    promptPrefix?: string;         // Prepended to user/task prompt
    promptSuffix?: string;         // Appended to user/task prompt
    systemPromptOverride?: string; // Override agent's system prompt
    planMode?: boolean;            // Use plan mode (no execution)
    requireApproval?: boolean;     // Require user approval before proceeding
  };

  // Transition rules
  transitions: {
    onComplete?: string;           // Step ID to move to on agent completion
    onApproval?: string;           // Step ID to move to on user approval
    allowManualMove?: boolean;     // Can user drag to any column?
  };

  // Visual
  color: string;
  taskState: TaskState;            // Maps to existing TaskState enum
}
```

### 3. Step Types

Semantic step types that define default behaviors:

| Type | Default Behavior |
|------|------------------|
| `backlog` | No auto-action. Tasks wait here. |
| `planning` | Auto-start agent in plan mode. Produces `plan.md`. |
| `implementation` | Auto-start agent with implementation prompt. Uses plan if available. |
| `review` | Agent paused. Awaits user review of changes. |
| `verification` | Auto-start agent to verify/test changes. |
| `done` | Terminal state. No actions. |
| `blocked` | Task is blocked. No auto-actions. |

## Workflow Templates

### Dev Workflow (Default)

```yaml
id: dev
name: Development
description: Standard software development workflow with planning phase
steps:
  - id: backlog
    name: Backlog
    stepType: backlog
    position: 0
    taskState: TODO
    color: bg-neutral-400
    behavior:
      autoStartAgent: false
    transitions:
      allowManualMove: true

  - id: planning
    name: Planning
    stepType: planning
    position: 1
    taskState: IN_PROGRESS
    color: bg-purple-500
    behavior:
      autoStartAgent: true
      planMode: true
      promptPrefix: |
        [PLANNING PHASE]
        Analyze this task and create a detailed implementation plan.
        Do NOT make any code changes yet - only analyze and plan.
        
        Create a plan that includes:
        1. Understanding of the requirements
        2. Files that need to be modified or created
        3. Step-by-step implementation approach
        4. Potential risks or considerations
        
        Save your plan to `.kandev/plans/{task_id}.md`
    transitions:
      onComplete: review-plan
      allowManualMove: true

  - id: review-plan
    name: Review Plan
    stepType: review
    position: 2
    taskState: REVIEW
    color: bg-yellow-500
    behavior:
      autoStartAgent: false
      requireApproval: true
    transitions:
      onApproval: implementation
      allowManualMove: true

  - id: implementation
    name: Implementation
    stepType: implementation
    position: 3
    taskState: IN_PROGRESS
    color: bg-blue-500
    behavior:
      autoStartAgent: true
      promptPrefix: |
        [IMPLEMENTATION PHASE]
        Implement the task according to the plan in `.kandev/plans/{task_id}.md`.
        Follow the plan step by step.
    transitions:
      onComplete: review-code
      allowManualMove: true

  - id: review-code
    name: Code Review
    stepType: review
    position: 4
    taskState: REVIEW
    color: bg-orange-500
    behavior:
      autoStartAgent: false
      requireApproval: true
    transitions:
      onApproval: done
      allowManualMove: true

  - id: done
    name: Done
    stepType: done
    position: 5
    taskState: COMPLETED
    color: bg-green-500
    behavior:
      autoStartAgent: false
    transitions:
      allowManualMove: true
```

### Architecture Workflow

```yaml
id: architecture
name: Architecture
description: System design and architecture workflow
steps:
  - id: backlog
    name: Ideas
    stepType: backlog
    position: 0
    taskState: TODO
    behavior:
      autoStartAgent: false

  - id: research
    name: Research
    stepType: planning
    position: 1
    taskState: IN_PROGRESS
    behavior:
      autoStartAgent: true
      planMode: true
      promptPrefix: |
        [RESEARCH PHASE]
        Research this architecture topic. Analyze:
        1. Current state of the codebase
        2. Industry best practices
        3. Trade-offs of different approaches

        Document your findings in `.kandev/research/{task_id}.md`
    transitions:
      onComplete: design

  - id: design
    name: Design
    stepType: planning
    position: 2
    taskState: IN_PROGRESS
    behavior:
      autoStartAgent: true
      planMode: true
      promptPrefix: |
        [DESIGN PHASE]
        Based on research in `.kandev/research/{task_id}.md`, create a design:
        1. High-level architecture
        2. Component diagram
        3. API contracts
        4. Migration path (if applicable)

        Save design to `.kandev/designs/{task_id}.md`
    transitions:
      onComplete: review

  - id: review
    name: Review
    stepType: review
    position: 3
    taskState: REVIEW
    behavior:
      requireApproval: true
    transitions:
      onApproval: approved

  - id: approved
    name: Approved
    stepType: done
    position: 4
    taskState: COMPLETED
```

## Data Model

### Schema

```sql
-- Workflow templates (system + user-defined)
-- These are blueprints that can be used when creating a board
CREATE TABLE workflow_templates (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT,
  is_system INTEGER DEFAULT 0,    -- System templates are read-only
  steps JSON NOT NULL,            -- Array of step definitions (blueprint)
  created_at TIMESTAMP,
  updated_at TIMESTAMP
);

-- Boards now require a workflow
-- The workflow_template_id tracks which template was used (for reference only)
CREATE TABLE boards (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  name TEXT NOT NULL,
  description TEXT,
  workflow_template_id TEXT REFERENCES workflow_templates(id),  -- Template used (nullable for custom)
  created_at TIMESTAMP,
  updated_at TIMESTAMP
);

-- Workflow steps replace columns entirely
-- Each step IS a column with behavior attached
CREATE TABLE workflow_steps (
  id TEXT PRIMARY KEY,
  board_id TEXT NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  step_type TEXT NOT NULL,        -- backlog, planning, implementation, review, done, blocked
  position INTEGER NOT NULL,
  color TEXT NOT NULL,

  -- Behavior configuration
  auto_start_agent INTEGER DEFAULT 0,
  plan_mode INTEGER DEFAULT 0,
  require_approval INTEGER DEFAULT 0,
  prompt_prefix TEXT,
  prompt_suffix TEXT,

  -- Transition rules
  on_complete_step_id TEXT REFERENCES workflow_steps(id),  -- Step to move to on agent completion
  on_approval_step_id TEXT REFERENCES workflow_steps(id),  -- Step to move to on approval
  allow_manual_move INTEGER DEFAULT 1,

  created_at TIMESTAMP,
  updated_at TIMESTAMP
);

-- Tasks remain simple - workflow progress is tracked per-session
CREATE TABLE tasks (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id),
  board_id TEXT NOT NULL REFERENCES boards(id),
  title TEXT NOT NULL,
  description TEXT,
  priority INTEGER DEFAULT 0,
  position INTEGER DEFAULT 0,
  metadata JSON,
  created_at TIMESTAMP,
  updated_at TIMESTAMP
);

-- Task sessions track workflow progress (each session goes through workflow independently)
-- Extends existing task_sessions table with workflow fields
CREATE TABLE task_sessions (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,

  -- Primary session flag (NEW)
  is_primary BOOLEAN DEFAULT FALSE,  -- Only one session per task should be primary

  -- Workflow tracking (NEW)
  workflow_step_id TEXT REFERENCES workflow_steps(id),  -- Current step in workflow
  review_status TEXT,              -- NULL, 'pending', 'changes_requested', 'approved'
  review_feedback JSON,            -- Array of feedback items

  -- Existing fields
  agent_profile_id TEXT NOT NULL,
  executor_id TEXT,
  environment_id TEXT,
  repository_id TEXT,
  base_branch TEXT,
  agent_profile_snapshot JSON,
  executor_snapshot JSON,
  environment_snapshot JSON,
  repository_snapshot JSON,
  state TEXT NOT NULL DEFAULT 'CREATED',  -- CREATED, RUNNING, COMPLETED, etc.
  error_message TEXT,
  metadata JSON,
  started_at TIMESTAMP NOT NULL,
  completed_at TIMESTAMP,
  updated_at TIMESTAMP NOT NULL
);

-- Index to quickly find primary session for a task
CREATE INDEX idx_task_sessions_primary ON task_sessions(task_id, is_primary) WHERE is_primary = TRUE;

-- Session step history (audit trail per session)
CREATE TABLE session_step_history (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL REFERENCES task_sessions(id) ON DELETE CASCADE,
  from_step_id TEXT REFERENCES workflow_steps(id),
  to_step_id TEXT NOT NULL REFERENCES workflow_steps(id),
  trigger TEXT NOT NULL,          -- manual, auto_complete, approval
  actor_id TEXT,                  -- User or system
  metadata JSON,
  created_at TIMESTAMP
);
```

### Key Changes from Current Schema

| Current | New |
|---------|-----|
| `columns` table | Replaced by `workflow_steps` |
| `task.column_id` | Removed (tasks don't track workflow position) |
| No workflow on sessions | `task_sessions.workflow_step_id` tracks session's position |
| No review status | `task_sessions.review_status` + `review_feedback` |
| Column has `state` field | Step has `step_type` + behavior fields |
| No workflow concept | `workflow_templates` + board has `workflow_template_id` |
| No primary session | `task_sessions.is_primary` flag (one per task) |

### Primary Session and Task Display

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Task: "Implement user authentication"                                       │
│  Board: Development Board                                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Session 1 (completed)                                                      │
│  ├── is_primary: FALSE                                                      │
│  ├── workflow_step: "Done" ✓                                                │
│  └── review_status: "approved"                                              │
│                                                                             │
│  Session 2 (abandoned)                                                      │
│  ├── is_primary: FALSE                                                      │
│  ├── workflow_step: "Review Plan"                                           │
│  └── review_status: "changes_requested"                                     │
│                                                                             │
│  Session 3 (active) ← PRIMARY                                               │
│  ├── is_primary: TRUE                                                       │
│  ├── workflow_step: "Implementation"                                        │
│  └── review_status: NULL                                                    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘

Task's kanban position = PRIMARY session's workflow_step
(latest session is automatically primary; user can change)
```

## Backend Components

### 1. Workflow Service (`internal/workflow/`)

```go
package workflow

type Service struct {
  repo            Repository
  sessionService  *task.SessionService
  orchestrator    *orchestrator.Service
  eventBus        *events.Bus
}

// Core operations
func (s *Service) CreateWorkflowFromTemplate(ctx, boardID, templateID string) error
func (s *Service) GetBoardWorkflow(ctx, boardID string) (*Workflow, error)
func (s *Service) GetStepForColumn(ctx, columnID string) (*WorkflowStep, error)

// Primary session operations
func (s *Service) GetPrimarySession(ctx, taskID string) (*TaskSession, error)
func (s *Service) SetPrimarySession(ctx, taskID, sessionID string) error  // Unsets previous primary

// Session transitions (workflow operates at session level)
func (s *Service) StartSession(ctx, taskID string, stepID string) (*TaskSession, error)  // Auto-sets as primary
func (s *Service) MoveSessionToStep(ctx, sessionID, toStepID string) error
func (s *Service) OnAgentComplete(ctx, sessionID string) error
func (s *Service) ApproveSession(ctx, sessionID string) error
func (s *Service) RejectSession(ctx, sessionID, reason string) error
func (s *Service) GetSessionStep(ctx, sessionID string) (*WorkflowStep, error)
```

### 2. Step Executor (`internal/workflow/executor/`)

Handles step-specific behaviors for sessions:

```go
type StepExecutor struct {
  orchestrator *orchestrator.Service
}

func (e *StepExecutor) ExecuteStep(ctx, session *TaskSession, task *Task, step *WorkflowStep) error {
  // 1. Apply prompt modifications
  prompt := e.buildPrompt(task, session, step)

  // 2. Start agent if autoStartAgent
  if step.Behavior.AutoStartAgent {
    return e.orchestrator.StartSession(ctx, session.ID, prompt, step.Behavior.PlanMode)
  }

  return nil
}

func (e *StepExecutor) buildPrompt(task *Task, session *TaskSession, step *WorkflowStep) string {
  var sb strings.Builder

  if step.Behavior.PromptPrefix != "" {
    sb.WriteString(e.interpolate(step.Behavior.PromptPrefix, task, session))
    sb.WriteString("\n\n")
  }

  sb.WriteString(task.Description)

  // Include previous feedback if session has changes_requested
  if session.ReviewStatus == "changes_requested" && len(session.ReviewFeedback) > 0 {
    sb.WriteString("\n\n[FEEDBACK TO ADDRESS]\n")
    for _, fb := range session.ReviewFeedback {
      if !fb.Resolved {
        sb.WriteString("- " + fb.Message + "\n")
      }
    }
  }

  if step.Behavior.PromptSuffix != "" {
    sb.WriteString("\n\n")
    sb.WriteString(e.interpolate(step.Behavior.PromptSuffix, task, session))
  }

  return sb.String()
}
```

### 3. Transition Handler

Listens for events and triggers session transitions:

```go
func (s *Service) handleAgentComplete(event *events.AgentCompleteEvent) {
  // Workflow operates at session level, not task level
  step := s.GetSessionStep(ctx, event.SessionID)

  if step.Transitions.OnComplete != "" {
    nextStep := s.GetStep(ctx, step.Transitions.OnComplete)
    s.MoveSessionToStep(ctx, event.SessionID, nextStep.ID)
  }
}
```

## Frontend Design Diagrams

### User Flow: Board Creation with Workflow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           BOARD CREATION FLOW                                │
└─────────────────────────────────────────────────────────────────────────────┘

  ┌──────────────┐     ┌───────────────────┐     ┌──────────────────────────┐
  │ Click "New   │────▶│  Board Creation   │────▶│  Select Workflow         │
  │ Board"       │     │  Dialog Opens     │     │  Template                │
  └──────────────┘     └───────────────────┘     └──────────────────────────┘
                                                            │
                       ┌────────────────────────────────────┼────────────────┐
                       │                                    │                │
                       ▼                                    ▼                ▼
              ┌─────────────────┐              ┌─────────────────┐  ┌─────────────────┐
              │  "Development"  │              │  "Architecture" │  │    "Custom"     │
              │                 │              │                 │  │                 │
              │ Backlog         │              │ Ideas           │  │ (Empty board,   │
              │ Planning        │              │ Research        │  │  add steps      │
              │ Review Plan     │              │ Design          │  │  manually)      │
              │ Implementation  │              │ Review          │  │                 │
              │ Code Review     │              │ Approved        │  │                 │
              │ Done            │              │                 │  │                 │
              └─────────────────┘              └─────────────────┘  └─────────────────┘
                       │                                    │                │
                       └────────────────────────────────────┼────────────────┘
                                                            │
                                                            ▼
                                               ┌──────────────────────────┐
                                               │  Preview Workflow Steps  │
                                               │  (icons show behaviors)  │
                                               │                          │
                                               │  🤖 = auto-start agent   │
                                               │  ✓  = requires approval  │
                                               │  📋 = plan mode          │
                                               └──────────────────────────┘
                                                            │
                                                            ▼
                                               ┌──────────────────────────┐
                                               │  Click "Create Board"    │
                                               │                          │
                                               │  → Creates board         │
                                               │  → Creates workflow_steps│
                                               │  → Redirects to board    │
                                               └──────────────────────────┘
```

### User Flow: Session Lifecycle Through Workflow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                   SESSION LIFECYCLE (Development Workflow)                   │
│                                                                             │
│  Note: Each SESSION progresses through steps independently.                 │
│        A task can have multiple sessions at different steps.                │
└─────────────────────────────────────────────────────────────────────────────┘

  ┌─────────┐    ┌──────────┐    ┌─────────────┐    ┌──────────────┐    ┌──────────┐    ┌──────┐
  │ BACKLOG │───▶│ PLANNING │───▶│ REVIEW PLAN │───▶│IMPLEMENTATION│───▶│CODE REVIEW│───▶│ DONE │
  └─────────┘    └──────────┘    └─────────────┘    └──────────────┘    └──────────┘    └──────┘
       │              │                 │                  │                  │              │
       │              │                 │                  │                  │              │
       ▼              ▼                 ▼                  ▼                  ▼              ▼
  ┌─────────┐   ┌───────────┐    ┌───────────┐     ┌───────────┐      ┌───────────┐   ┌─────────┐
  │ User    │   │ 🤖 Agent  │    │ ✓ User    │     │ 🤖 Agent  │      │ ✓ User    │   │ Session │
  │ starts  │   │ auto-     │    │ reviews   │     │ auto-     │      │ reviews   │   │ complete│
  │ new     │   │ starts    │    │ plan      │     │ starts    │      │ code      │   │         │
  │ session │   │ (plan     │    │           │     │ (uses     │      │           │   │         │
  │         │   │  mode)    │    │           │     │  plan)    │      │           │   │         │
  └─────────┘   └───────────┘    └───────────┘     └───────────┘      └───────────┘   └─────────┘
                     │                 │                  │                  │
                     ▼                 │                  ▼                  │
              ┌───────────┐            │           ┌───────────┐             │
              │ Creates   │            │           │ Makes     │             │
              │ plan.md   │            │           │ code      │             │
              │ in work-  │            │           │ changes   │             │
              │ tree      │            │           │           │             │
              └───────────┘            │           └───────────┘             │
                     │                 │                                     │
                     ▼                 ▼                                     ▼
              ┌───────────┐     ┌─────────────┐                       ┌─────────────┐
              │ On agent  │     │  Approve    │                       │  Approve    │
              │ complete  │     │  session?   │                       │  session?   │
              │ → session │     │             │                       │             │
              │ moves to  │     │ YES → next  │                       │ YES → Done  │
              │ Review    │     │ NO  → stay  │                       │ NO  → stay  │
              │ Plan      │     │     + feedback                      │     + feedback
              └───────────┘     └─────────────┘                       └─────────────┘

  ┌─────────────────────────────────────────────────────────────────────────────┐
  │ MULTIPLE SESSIONS EXAMPLE:                                                  │
  │                                                                             │
  │ Task: "Implement auth"                                                      │
  │ ├── Session 1: ────▶ Planning ────▶ Review ────▶ Implementation ── (done)   │
  │ ├── Session 2: ────▶ Planning ────▶ Review (rejected, abandoned)            │
  │ └── Session 3: ────▶ Planning (in progress...) ← PRIMARY                    │
  │                                                                             │
  │ Task card shows in column of PRIMARY session (Session 3 → "Planning")       │
  │ User can change primary session if needed (e.g., pin Session 1 as primary)  │
  └─────────────────────────────────────────────────────────────────────────────┘
```

### User Flow: Session Review and Rejection Cycle

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     SESSION REVIEW & REJECTION FLOW                          │
│                                                                             │
│  Review and approval happen per-SESSION, not per-task.                      │
│  Each session's plan/work is reviewed independently.                        │
└─────────────────────────────────────────────────────────────────────────────┘

                              ┌─────────────────────┐
                              │   Session in REVIEW │
                              │   PLAN step         │
                              └─────────────────────┘
                                        │
                                        ▼
                              ┌─────────────────────┐
                              │  User opens task    │
                              │  → selects session  │
                              │  to review          │
                              └─────────────────────┘
                                        │
                                        ▼
                    ┌─────────────────────────────────────────┐
                    │        SESSION REVIEW PANEL              │
                    │  ┌─────────────────────────────────┐    │
                    │  │  Session: #3 (Claude 3.5)       │    │
                    │  │  Step: Review Plan              │    │
                    │  │  ─────────────────────────────  │    │
                    │  │  📋 Implementation Plan          │    │
                    │  │  ## Overview                    │    │
                    │  │  This task will implement...    │    │
                    │  │  ## Steps                       │    │
                    │  │  1. Create new component        │    │
                    │  │  2. Add API endpoint            │    │
                    │  │  ...                            │    │
                    │  └─────────────────────────────────┘    │
                    │                                          │
                    │  ┌──────────────┐  ┌──────────────────┐ │
                    │  │ ✓ Approve    │  │ ✗ Request        │ │
                    │  │   Session    │  │   Changes        │ │
                    │  └──────────────┘  └──────────────────┘ │
                    └─────────────────────────────────────────┘
                                        │
                       ┌────────────────┴────────────────┐
                       │                                 │
                       ▼                                 ▼
              ┌─────────────────┐              ┌─────────────────────┐
              │    APPROVE      │              │   REQUEST CHANGES   │
              │                 │              │                     │
              │ Session moves   │              │ Feedback dialog     │
              │ to next step    │              │ opens               │
              │ (IMPLEMENTATION)│              │                     │
              └─────────────────┘              └─────────────────────┘
                                                         │
                                                         ▼
                                               ┌─────────────────────┐
                                               │  Enter feedback:    │
                                               │  ┌─────────────────┐│
                                               │  │ "Need to handle ││
                                               │  │  edge case X"   ││
                                               │  └─────────────────┘│
                                               │  [Submit Feedback]  │
                                               └─────────────────────┘
                                                         │
                                                         ▼
                              ┌─────────────────────────────────────────┐
                              │   Session stays in REVIEW PLAN step     │
                              │   with "Changes Requested" status       │
                              │                                         │
                              │   ┌─────────────────────────────────┐   │
                              │   │ Session #3: Claude 3.5          │   │
                              │   │ ┌─────────────────────────────┐ │   │
                              │   │ │ 🔴 Changes Requested (1)    │ │   │
                              │   │ └─────────────────────────────┘ │   │
                              │   └─────────────────────────────────┘   │
                              └─────────────────────────────────────────┘
                                                         │
                                                         ▼
                              ┌─────────────────────────────────────────┐
                              │        SESSION REVIEW PANEL (updated)   │
                              │                                         │
                              │  ⚠️ Session Feedback History            │
                              │  ┌─────────────────────────────────┐   │
                              │  │ 🟡 "Need to handle edge case X" │   │
                              │  │    2 minutes ago                │   │
                              │  │    [Mark Resolved]              │   │
                              │  └─────────────────────────────────┘   │
                              │                                         │
                              │  ┌─────────────────────────────────┐   │
                              │  │ 🤖 Re-run Session Agent         │   │
                              │  │    to Address Feedback          │   │
                              │  └─────────────────────────────────┘   │
                              │                                         │
                              │  ⚠️ 1 unresolved feedback item.        │
                              │     Resolve all before approving.       │
                              └─────────────────────────────────────────┘
                                                         │
                                                         ▼
                              ┌─────────────────────────────────────────┐
                              │  User clicks "Re-run Session Agent"     │
                              │  → Agent restarts in same session       │
                              │  → Agent sees previous plan + feedback  │
                              │  → Agent updates plan file              │
                              │  → On complete, user reviews again      │
                              └─────────────────────────────────────────┘
```

### UI Component Layout: Kanban Board

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│  📋 My Project Board                                                    [⚙️ Settings]       │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐       │
│  │ ○ Backlog   │  │ ○ Planning  │  │ ○ Review    │  │ ○ Implement │  │ ○ Done      │       │
│  │             │  │ 🤖 📋       │  │ ✓           │  │ 🤖          │  │             │       │
│  ├─────────────┤  ├─────────────┤  ├─────────────┤  ├─────────────┤  ├─────────────┤       │
│  │             │  │             │  │             │  │             │  │             │       │
│  │ ┌─────────┐ │  │ ┌─────────┐ │  │ ┌─────────┐ │  │ ┌─────────┐ │  │ ┌─────────┐ │       │
│  │ │ Task A  │ │  │ │ Task B  │ │  │ │ Task D  │ │  │ │ Task E  │ │  │ │ Task G  │ │       │
│  │ │ (no     │ │  │ │ ●●○○○○  │ │  │ │ 🔴 (1)  │ │  │ │ ●●●●○○  │ │  │ │ ●●●●●●  │ │       │
│  │ │ session)│ │  │ │ 🤖 S#1  │ │  │ │ S#2     │ │  │ │ 🤖 S#3  │ │  │ │ ✓ S#1   │ │       │
│  │ └─────────┘ │  │ └─────────┘ │  │ └─────────┘ │  │ └─────────┘ │  │ └─────────┘ │       │
│  │             │  │             │  │             │  │             │  │             │       │
│  │ ┌─────────┐ │  │ ┌─────────┐ │  │             │  │             │  │ ┌─────────┐ │       │
│  │ │ Task C  │ │  │ │ Task F  │ │  │             │  │             │  │ │ Task H  │ │       │
│  │ │ (no     │ │  │ │ ●●○○○○  │ │  │             │  │             │  │ │ ●●●●●●  │ │       │
│  │ │ session)│ │  │ │ S#1     │ │  │             │  │             │  │ │ ✓ S#2   │ │       │
│  │ └─────────┘ │  │ └─────────┘ │  │             │  │             │  │ └─────────┘ │       │
│  │             │  │             │  │             │  │             │  │             │       │
│  │ [+ Add]     │  │             │  │             │  │             │  │             │       │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘       │
│                                                                                             │
└─────────────────────────────────────────────────────────────────────────────────────────────┘

LEGEND:
  ○ Column color indicator          🤖 Agent auto-starts in this column
  📋 Plan mode enabled              ✓  Requires approval
  ●●●○○○ Session workflow progress  🔴 (1) Changes requested (on session)
  🤖 S#1 Active session running     S#2 Session indicator (shows primary session)
  ★ Primary session indicator

NOTE: Task card position in column = PRIMARY session's workflow step
      Latest session is automatically primary; user can change
      Click task → select specific session to view/manage
```

### UI Component Layout: Task Detail Panel (Session-Based)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  ← Back                                                          [⋮ More]  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Implement user authentication                                              │
│  ════════════════════════════════════════════════════════════════════════  │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ Sessions                                               [+ New Session]│   │
│  ├─────────────────────────────────────────────────────────────────────┤   │
│  │                                                                      │   │
│  │  ┌────────────────┐ ┌────────────────┐ ┌────────────────┐           │   │
│  │  │ ● Session #3   │ │ ○ Session #2   │ │ ○ Session #1   │           │   │
│  │  │ Claude 3.5     │ │ Codex          │ │ Claude 3.5     │           │   │
│  │  │ In: Review     │ │ Abandoned      │ │ ✓ Completed    │           │   │
│  │  │ 🔴 (1 issue)   │ │                │ │                │           │   │
│  │  └────────────────┘ └────────────────┘ └────────────────┘           │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ═══════════════════════════════════════════════════════════════════════   │
│  SESSION #3 DETAILS                                                         │
│  ═══════════════════════════════════════════════════════════════════════   │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ Session Workflow Progress                                            │   │
│  │                                                                      │   │
│  │  ✓ Backlog  →  ✓ Planning  →  ● Review  →  ○ Implement  →  ○ Done   │   │
│  │                                  Plan                                │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ ⚠️ Session Feedback History                                          │   │
│  ├─────────────────────────────────────────────────────────────────────┤   │
│  │                                                                      │   │
│  │  ┌───────────────────────────────────────────────────────────────┐  │   │
│  │  │ 🟡 "Consider using httpOnly cookies for refresh tokens"       │  │   │
│  │  │    5 minutes ago                                              │  │   │
│  │  │                                              [Mark Resolved]  │  │   │
│  │  └───────────────────────────────────────────────────────────────┘  │   │
│  │                                                                      │   │
│  │  ┌───────────────────────────────────────────────────────────────┐  │   │
│  │  │ ✅ "Add rate limiting to login endpoint"                      │  │   │
│  │  │    1 hour ago                                       Resolved  │  │   │
│  │  └───────────────────────────────────────────────────────────────┘  │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                                                                      │   │
│  │  ⚠️ 1 unresolved feedback item. Resolve all before approving.       │   │
│  │                                                                      │   │
│  │  ┌─────────────────────────┐  ┌─────────────────────────────────┐   │   │
│  │  │ 🤖 Re-run Session       │  │ [Abandon Session]               │   │   │
│  │  │    Agent                │  │                                 │   │   │
│  │  └─────────────────────────┘  └─────────────────────────────────┘   │   │
│  │                                                                      │   │
│  │  ┌─────────────────────────┐  ┌─────────────────────────────────┐   │   │
│  │  │ ✓ Approve Session       │  │ ✗ Request Changes               │   │   │
│  │  │        (disabled)       │  │                                 │   │   │
│  │  └─────────────────────────┘  └─────────────────────────────────┘   │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### UI Component Layout: Workflow Editor (Board Settings)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  ⚙️ Board Settings                                                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  General │ Workflow │ Permissions                                           │
│  ────────  ════════   ───────────                                           │
│                                                                             │
│  Workflow Steps                                              [+ Add Step]   │
│  ═══════════════════════════════════════════════════════════════════════   │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ ⋮⋮ │ 🟢 │ Backlog          │ backlog    ▼ │              │ 🗑️ │ ▼ │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ ⋮⋮ │ 🔵 │ Planning         │ planning   ▼ │ 🤖 📋        │ 🗑️ │ ▼ │   │
│  ├─────────────────────────────────────────────────────────────────────┤   │
│  │                                                                      │   │
│  │  ☑️ Auto-start agent       ☑️ Plan mode (no execution)              │   │
│  │  ☐ Require approval                                                  │   │
│  │                                                                      │   │
│  │  Prompt Prefix:                                                      │   │
│  │  ┌───────────────────────────────────────────────────────────────┐  │   │
│  │  │ [PLANNING PHASE] Analyze the task and create a detailed       │  │   │
│  │  │ implementation plan. Do NOT write any code yet.               │  │   │
│  │  └───────────────────────────────────────────────────────────────┘  │   │
│  │                                                                      │   │
│  │  On Completion, move to:  [ Review Plan          ▼ ]                │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ ⋮⋮ │ 🟡 │ Review Plan      │ review     ▼ │ ✓           │ 🗑️ │ ▶ │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ ⋮⋮ │ 🟣 │ Implementation   │ implement  ▼ │ 🤖          │ 🗑️ │ ▶ │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ ⋮⋮ │ 🟠 │ Code Review      │ review     ▼ │ ✓           │ 🗑️ │ ▶ │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ ⋮⋮ │ ⬜ │ Done             │ done       ▼ │              │ 🗑️ │ ▶ │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│                                                                             │
│  LEGEND:                                                                    │
│  ⋮⋮ = Drag handle    🤖 = Auto-start agent    📋 = Plan mode               │
│  ✓ = Requires approval    ▼/▶ = Expand/collapse step details               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### State Diagram: Session in Review Step

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    SESSION STATE IN REVIEW STEP                              │
└─────────────────────────────────────────────────────────────────────────────┘

                         ┌──────────────────────┐
                         │                      │
                         │   PENDING_REVIEW     │◀─────────────────────────┐
                         │                      │                          │
                         └──────────────────────┘                          │
                                    │                                      │
                    ┌───────────────┴───────────────┐                      │
                    │                               │                      │
                    ▼                               ▼                      │
         ┌──────────────────┐            ┌──────────────────┐              │
         │                  │            │                  │              │
         │     APPROVED     │            │ CHANGES_REQUESTED│              │
         │                  │            │                  │              │
         └──────────────────┘            └──────────────────┘              │
                    │                               │                      │
                    │                               │                      │
                    ▼                               ▼                      │
         ┌──────────────────┐            ┌──────────────────┐              │
         │                  │            │                  │              │
         │  Move session to │            │  Agent re-runs   │──────────────┘
         │  next step       │            │  with feedback   │
         │                  │            │                  │
         └──────────────────┘            └──────────────────┘


  Session Fields (in task_sessions table):
  ┌─────────────────────────────────────────────────────────────────────┐
  │ workflow_step_id: "step-review-plan"   -- Current workflow position │
  │ review_status: "changes_requested"     -- pending | approved | ...  │
  │ review_feedback: [                                                  │
  │   {                                                                 │
  │     "id": "fb-1",                                                   │
  │     "message": "Need to handle edge case X",                        │
  │     "created_at": "2024-01-15T10:30:00Z",                          │
  │     "resolved": false                                               │
  │   },                                                                │
  │   {                                                                 │
  │     "id": "fb-2",                                                   │
  │     "message": "Add error handling",                                │
  │     "created_at": "2024-01-15T09:00:00Z",                          │
  │     "resolved": true,                                               │
  │     "resolved_at": "2024-01-15T10:00:00Z"                          │
  │   }                                                                 │
  │ ]                                                                   │
  └─────────────────────────────────────────────────────────────────────┘

  NOTE: Each session tracks its own workflow position independently.
        A task's position on the kanban board is derived from its most
        advanced active session.
```

### Data Flow: Session Start with Agent Auto-Start

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                DATA FLOW: SESSION START WITH AGENT AUTO-START                │
└─────────────────────────────────────────────────────────────────────────────┘

  ┌──────────────┐
  │   Frontend   │
  │ (Task Panel) │
  └──────────────┘
         │
         │ 1. User clicks "New Session" or "Start" on task
         │    (selects target step, e.g., "Planning")
         │
         ▼
  ┌──────────────┐     2. WS: session.create                ┌──────────────┐
  │   WebSocket  │ ──────────────────────────────────────▶ │   Backend    │
  │   Client     │    { task_id, step_id }                  │   (WS GW)    │
  └──────────────┘                                         └──────────────┘
                                                                  │
                                                                  │ 3. WorkflowService.StartSession()
                                                                  ▼
                                                           ┌──────────────┐
                                                           │ Workflow     │
                                                           │ Service      │
                                                           │              │
                                                           │ Creates new  │
                                                           │ TaskSession  │
                                                           │ with step_id │
                                                           └──────────────┘
                                                                  │
                                                                  │ 4. Get target step config
                                                                  ▼
                                                           ┌──────────────┐
                                                           │ WorkflowStep │
                                                           │ auto_start:  │
                                                           │   true       │
                                                           │ plan_mode:   │
                                                           │   true       │
                                                           │ prompt_      │
                                                           │   prefix:... │
                                                           └──────────────┘
                                                                  │
                                                                  │ 5. Publish event
                                                                  ▼
                                                           ┌──────────────┐
                                                           │  Event Bus   │
                                                           │              │
                                                           │ session.     │
                                                           │  created     │
                                                           └──────────────┘
                                                                  │
                                                                  │ 6. Orchestrator handles event
                                                                  ▼
                                                           ┌──────────────┐
                                                           │ Orchestrator │
                                                           │              │
                                                           │ if step.     │
                                                           │ auto_start:  │
                                                           │   LaunchAgent│
                                                           └──────────────┘
                                                                  │
                                                                  │ 7. Build prompt with prefix + feedback
                                                                  ▼
                                                           ┌──────────────────────────────┐
                                                           │ Prompt = step.prompt_prefix  │
                                                           │        + task.description    │
                                                           │        + session.feedback    │
                                                           │        + step.prompt_suffix  │
                                                           │                              │
                                                           │ "[PLANNING PHASE] Analyze    │
                                                           │  the task and create a       │
                                                           │  detailed implementation     │
                                                           │  plan. Do NOT write code.\n" │
                                                           │  + "Implement user auth..."  │
                                                           │  + "[FEEDBACK TO ADDRESS]..."│
                                                           └──────────────────────────────┘
                                                                  │
                                                                  │ 8. Start agent for session
                                                                  ▼
                                                           ┌──────────────┐
                                                           │  Lifecycle   │
                                                           │  Manager     │
                                                           │              │
                                                           │ StartSession │
                                                           │ (session has │
                                                           │ workflow_    │
                                                           │ step_id)     │
                                                           └──────────────┘
                                                                  │
         ┌────────────────────────────────────────────────────────┘
         │ 9. WS: session.started, session.update (streaming)
         ▼
  ┌──────────────┐
  │   Frontend   │
  │   (Updates   │
  │   session in │
  │   task panel │
  │   with 🤖)   │
  └──────────────┘

  NOTE: Task card position on kanban = PRIMARY session's workflow step
```

## Frontend Integration

### Board Creation with Workflow Selection

When creating a new board, users select a workflow template that determines the columns and their behaviors.

**Dialog Flow**:
1. User clicks "New Board"
2. Dialog opens with board name input and workflow template selector
3. Selecting a template shows a preview of the workflow steps
4. User can choose "Custom" to start with a blank workflow
5. On create, columns are generated from the workflow steps

```tsx
<Dialog>
  <DialogContent className="max-w-2xl">
    <DialogHeader>
      <DialogTitle>Create Board</DialogTitle>
    </DialogHeader>

    <div className="space-y-4">
      <Input label="Board Name" placeholder="My Project Board" />

      <div>
        <Label>Workflow Template</Label>
        <RadioGroup value={selectedTemplate} onValueChange={setSelectedTemplate}>
          <RadioGroupItem value="standard">
            <div className="flex flex-col">
              <span className="font-medium">Standard</span>
              <span className="text-sm text-muted-foreground">
                Todo → Plan → Implementation → Done
              </span>
            </div>
          </RadioGroupItem>
          <RadioGroupItem value="architecture">
            <div className="flex flex-col">
              <span className="font-medium">Architecture</span>
              <span className="text-sm text-muted-foreground">
                Ideas → Research → Design → Review → Approved
              </span>
            </div>
          </RadioGroupItem>
          <RadioGroupItem value="simple">
            <div className="flex flex-col">
              <span className="font-medium">Simple Kanban</span>
              <span className="text-sm text-muted-foreground">
                Todo → In Progress → Done (no automation)
              </span>
            </div>
          </RadioGroupItem>
          <RadioGroupItem value="custom">
            <div className="flex flex-col">
              <span className="font-medium">Custom</span>
              <span className="text-sm text-muted-foreground">
                Start with a blank workflow and add steps manually
              </span>
            </div>
          </RadioGroupItem>
        </RadioGroup>
      </div>

      {/* Workflow Preview */}
      {selectedTemplate && selectedTemplate !== 'custom' && (
        <div className="border rounded-lg p-4 bg-muted/50">
          <Label className="text-sm">Workflow Preview</Label>
          <div className="flex items-center gap-2 mt-2 overflow-x-auto">
            {templateSteps.map((step, i) => (
              <React.Fragment key={step.id}>
                <div className={cn("px-3 py-1.5 rounded text-sm", step.color)}>
                  {step.name}
                  {step.behavior.autoStartAgent && (
                    <IconRobot className="inline ml-1 h-3 w-3" />
                  )}
                  {step.behavior.requireApproval && (
                    <IconCheck className="inline ml-1 h-3 w-3" />
                  )}
                </div>
                {i < templateSteps.length - 1 && (
                  <IconArrowRight className="h-4 w-4 text-muted-foreground" />
                )}
              </React.Fragment>
            ))}
          </div>
          <div className="flex gap-4 mt-2 text-xs text-muted-foreground">
            <span><IconRobot className="inline h-3 w-3" /> Auto-starts agent</span>
            <span><IconCheck className="inline h-3 w-3" /> Requires approval</span>
          </div>
        </div>
      )}
    </div>

    <DialogFooter>
      <Button variant="outline" onClick={onClose}>Cancel</Button>
      <Button onClick={handleCreate}>Create Board</Button>
    </DialogFooter>
  </DialogContent>
</Dialog>
```

### Kanban Board with Workflow Columns

The kanban board displays columns based on workflow steps. Each column header shows:
- Column name
- Step type indicator (icon)
- Auto-start badge if enabled

```tsx
<div className="kanban-board flex gap-4 overflow-x-auto p-4">
  {columns.map((column) => (
    <div key={column.id} className="kanban-column w-80 flex-shrink-0">
      {/* Column Header */}
      <div className="flex items-center justify-between p-3 bg-muted rounded-t-lg">
        <div className="flex items-center gap-2">
          <div className={cn("w-3 h-3 rounded-full", column.step?.color)} />
          <span className="font-medium">{column.name}</span>
          <Badge variant="outline" className="text-xs">
            {column.step?.stepType}
          </Badge>
        </div>
        <div className="flex items-center gap-1">
          {column.step?.behavior.autoStartAgent && (
            <Tooltip content="Agent auto-starts when tasks enter this column">
              <IconRobot className="h-4 w-4 text-blue-500" />
            </Tooltip>
          )}
          {column.step?.behavior.requireApproval && (
            <Tooltip content="Tasks require approval to proceed">
              <IconShieldCheck className="h-4 w-4 text-yellow-500" />
            </Tooltip>
          )}
        </div>
      </div>

      {/* Task Cards */}
      <div className="space-y-2 p-2 bg-muted/30 min-h-[200px]">
        {column.tasks.map((task) => (
          <TaskCard key={task.id} task={task} step={column.step} />
        ))}
      </div>
    </div>
  ))}
</div>
```

### Task Card with Primary Session Workflow Status

Task cards show workflow-specific information based on the **primary session**:
- Primary session workflow progress dots
- Review status badge (if primary session has changes requested)
- Primary session indicator (★ marker)
- Agent status (running, waiting, etc.)

```tsx
function TaskCard({ task, step }: { task: Task; step: WorkflowStep }) {
  // Get the primary session for display (latest session by default)
  const primarySession = usePrimarySession(task.id);
  const sessionStep = primarySession?.workflow_step_id;
  const hasChangesRequested = primarySession?.review_status === 'changes_requested';
  const feedbackCount = primarySession?.review_feedback?.filter(f => !f.resolved).length ?? 0;
  const sessionCount = task.sessions?.length ?? 0;

  return (
    <Card className="p-3 cursor-pointer hover:shadow-md transition-shadow">
      {/* Header */}
      <div className="flex items-start justify-between">
        <h4 className="font-medium text-sm line-clamp-2">{task.title}</h4>
        {hasChangesRequested && (
          <Badge variant="destructive" className="text-xs">
            Changes Requested ({feedbackCount})
          </Badge>
        )}
      </div>

      {/* Primary Session Workflow Progress Dots */}
      {primarySession && (
        <div className="flex items-center gap-1 mt-2">
          {workflowSteps.map((s, i) => (
            <div
              key={s.id}
              className={cn(
                "w-2 h-2 rounded-full",
                i < currentStepIndex && "bg-green-500",           // Completed
                i === currentStepIndex && "bg-blue-500 ring-2 ring-blue-200", // Current
                i > currentStepIndex && "bg-gray-300"             // Future
              )}
            />
          ))}
        </div>
      )}

      {/* Primary Session Indicator */}
      <div className="flex items-center gap-1 mt-2 text-xs text-muted-foreground">
        {primarySession ? (
          <>
            <span>★ Session #{primarySession.number}</span>
            {sessionCount > 1 && (
              <Badge variant="outline" className="text-xs ml-1">
                +{sessionCount - 1} more
              </Badge>
            )}
          </>
        ) : (
          <span className="text-muted-foreground">No sessions</span>
        )}
      </div>

      {/* Primary Session Agent Status */}
      {primarySession && (
        <div className="flex items-center gap-1 mt-1 text-xs text-muted-foreground">
          {primarySession.state === 'RUNNING' && (
            <>
              <IconLoader className="h-3 w-3 animate-spin" />
              <span>Agent working...</span>
            </>
          )}
          {primarySession.state === 'WAITING_FOR_INPUT' && (
            <>
              <IconMessageCircle className="h-3 w-3" />
              <span>Waiting for input</span>
            </>
          )}
        </div>
      )}
    </Card>
  );
}
```

### Review Step UI - Session Review Panel

When a **session** is in a review step, the session detail panel shows:
1. Previous feedback (if any)
2. Approval actions

```tsx
// Session-based review panel - operates on a specific session, not the task
function SessionReviewPanel({ session, task, step }: {
  session: TaskSession;
  task: Task;
  step: WorkflowStep
}) {
  return (
    <div className="space-y-4">
      {/* Session Info Header */}
      <div className="flex items-center justify-between p-2 bg-muted rounded">
        <span className="text-sm font-medium">
          Session #{session.number} • {session.agent_profile?.name ?? 'Agent'}
        </span>
        <Badge variant={session.review_status === 'changes_requested' ? 'destructive' : 'secondary'}>
          {session.review_status ?? 'pending'}
        </Badge>
      </div>

      {/* Session Feedback History */}
      {existingFeedback.length > 0 && (
        <div className="space-y-2">
          <Label>Session Feedback History</Label>
          {existingFeedback.map((fb) => (
            <div
              key={fb.id}
              className={cn(
                "p-3 rounded-lg border",
                fb.resolved ? "bg-green-50 border-green-200" : "bg-yellow-50 border-yellow-200"
              )}
            >
              <div className="flex items-start justify-between">
                <p className="text-sm">{fb.message}</p>
                {fb.resolved ? (
                  <Badge variant="outline" className="text-green-600">Resolved</Badge>
                ) : (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => markSessionFeedbackResolved(session.id, fb.id)}
                  >
                    Mark Resolved
                  </Button>
                )}
              </div>
              <span className="text-xs text-muted-foreground">
                {formatRelativeTime(fb.created_at)}
              </span>
            </div>
          ))}
        </div>
      )}

      {/* Session Approval Actions */}
      <div className="flex flex-col gap-3 pt-4 border-t">
        {!showFeedbackInput ? (
          <div className="flex gap-2">
            <Button
              className="flex-1"
              onClick={() => approveSession(session.id)}  // Session-level approval
              disabled={unresolvedFeedback.length > 0}
            >
              <IconCheck className="mr-2 h-4 w-4" />
              Approve Session
            </Button>
            <Button
              variant="outline"
              className="flex-1"
              onClick={() => setShowFeedbackInput(true)}
            >
              <IconX className="mr-2 h-4 w-4" />
              Request Changes
            </Button>
          </div>
        ) : (
          <div className="space-y-2">
            <Textarea
              placeholder="Describe what changes are needed..."
              value={feedback}
              onChange={(e) => setFeedback(e.target.value)}
              rows={3}
            />
            <div className="flex gap-2">
              <Button
                onClick={() => {
                  rejectSession(session.id, feedback);  // Session-level rejection
                  setFeedback('');
                  setShowFeedbackInput(false);
                }}
                disabled={!feedback.trim()}
              >
                Submit Feedback
              </Button>
              <Button
                variant="ghost"
                onClick={() => {
                  setFeedback('');
                  setShowFeedbackInput(false);
                }}
              >
                Cancel
              </Button>
            </div>
          </div>
        )}

        {unresolvedFeedback.length > 0 && (
          <p className="text-sm text-muted-foreground">
            ⚠️ {unresolvedFeedback.length} unresolved feedback item(s).
            Resolve all feedback before approving this session.
          </p>
        )}

        {/* Re-run Session Agent Button */}
        {unresolvedFeedback.length > 0 && (
          <Button
            variant="secondary"
            onClick={() => rerunSessionWithFeedback(session.id)}  // Session-level re-run
          >
            <IconRobot className="mr-2 h-4 w-4" />
            Re-run Session Agent with Feedback
          </Button>
        )}

        {/* Abandon Session Option */}
        <Button
          variant="ghost"
          className="text-destructive"
          onClick={() => abandonSession(session.id)}
        >
          Abandon This Session
        </Button>
      </div>
    </div>
  );
}
```

### Workflow Editor (Settings)

Users can customize workflows in the board settings. This allows:
- Reordering steps
- Editing step behaviors (prompts, auto-start, etc.)
- Adding/removing steps
- Changing step colors

```tsx
function WorkflowEditor({ boardId }: { boardId: string }) {
  const { workflow, updateStep, addStep, removeStep, reorderSteps } = useWorkflow(boardId);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-medium">Workflow Steps</h3>
        <Button variant="outline" size="sm" onClick={() => addStep()}>
          <IconPlus className="mr-2 h-4 w-4" />
          Add Step
        </Button>
      </div>

      <DndContext onDragEnd={handleDragEnd}>
        <SortableContext items={workflow.steps.map(s => s.id)}>
          {workflow.steps.map((step, index) => (
            <SortableStepCard
              key={step.id}
              step={step}
              index={index}
              onUpdate={(updates) => updateStep(step.id, updates)}
              onRemove={() => removeStep(step.id)}
            />
          ))}
        </SortableContext>
      </DndContext>
    </div>
  );
}

function SortableStepCard({ step, index, onUpdate, onRemove }) {
  const [expanded, setExpanded] = useState(false);

  return (
    <Card className="p-4">
      <div className="flex items-center gap-3">
        <IconGripVertical className="h-5 w-5 text-muted-foreground cursor-grab" />

        <div className={cn("w-4 h-4 rounded", step.color)} />

        <Input
          value={step.name}
          onChange={(e) => onUpdate({ name: e.target.value })}
          className="flex-1"
        />

        <Select value={step.stepType} onValueChange={(v) => onUpdate({ stepType: v })}>
          <SelectItem value="backlog">Backlog</SelectItem>
          <SelectItem value="planning">Planning</SelectItem>
          <SelectItem value="implementation">Implementation</SelectItem>
          <SelectItem value="review">Review</SelectItem>
          <SelectItem value="done">Done</SelectItem>
        </Select>

        <Button variant="ghost" size="sm" onClick={() => setExpanded(!expanded)}>
          {expanded ? <IconChevronUp /> : <IconChevronDown />}
        </Button>

        <Button variant="ghost" size="sm" onClick={onRemove}>
          <IconTrash className="h-4 w-4 text-destructive" />
        </Button>
      </div>

      {expanded && (
        <div className="mt-4 pl-8 space-y-4 border-t pt-4">
          {/* Behavior Settings */}
          <div className="grid grid-cols-2 gap-4">
            <div className="flex items-center gap-2">
              <Switch
                checked={step.behavior.autoStartAgent}
                onCheckedChange={(v) => onUpdate({ behavior: { ...step.behavior, autoStartAgent: v }})}
              />
              <Label>Auto-start agent</Label>
            </div>
            <div className="flex items-center gap-2">
              <Switch
                checked={step.behavior.planMode}
                onCheckedChange={(v) => onUpdate({ behavior: { ...step.behavior, planMode: v }})}
              />
              <Label>Plan mode (no execution)</Label>
            </div>
            <div className="flex items-center gap-2">
              <Switch
                checked={step.behavior.requireApproval}
                onCheckedChange={(v) => onUpdate({ behavior: { ...step.behavior, requireApproval: v }})}
              />
              <Label>Require approval</Label>
            </div>
          </div>

          {/* Prompt Prefix */}
          <div>
            <Label>Prompt Prefix</Label>
            <Textarea
              value={step.behavior.promptPrefix ?? ''}
              onChange={(e) => onUpdate({ behavior: { ...step.behavior, promptPrefix: e.target.value }})}
              placeholder="Text prepended to the task description when agent starts..."
              rows={3}
            />
          </div>

          {/* Transition Settings */}
          <div>
            <Label>On Completion, move to:</Label>
            <Select
              value={step.transitions.onComplete ?? ''}
              onValueChange={(v) => onUpdate({ transitions: { ...step.transitions, onComplete: v }})}
            >
              <SelectItem value="">Stay in this step</SelectItem>
              {workflow.steps.filter(s => s.id !== step.id).map(s => (
                <SelectItem key={s.id} value={s.id}>{s.name}</SelectItem>
              ))}
            </Select>
          </div>
        </div>
      )}
    </Card>
  );
}
```

### Drag-and-Drop Behavior

When a task is dragged to a new column:

1. **Check if manual move is allowed** - Some steps may restrict manual moves
2. **Trigger step entry behavior** - If the new step has `autoStartAgent: true`, start the agent
3. **Show confirmation for review steps** - If moving to a review step, confirm the user wants to submit for review

```tsx
function handleTaskDrop(taskId: string, fromColumnId: string, toColumnId: string) {
  const fromStep = getStepForColumn(fromColumnId);
  const toStep = getStepForColumn(toColumnId);

  // Check if move is allowed
  if (!toStep.transitions.allowManualMove) {
    toast.error(`Cannot manually move tasks to "${toStep.name}"`);
    return;
  }

  // Confirm if moving to review
  if (toStep.stepType === 'review' && fromStep.stepType !== 'review') {
    const confirmed = await confirm({
      title: 'Submit for Review?',
      description: 'This will submit the task for review. The agent will stop working.',
    });
    if (!confirmed) return;
  }

  // Execute move
  await moveTask(taskId, toColumnId);

  // Backend handles step entry behavior (auto-start agent, etc.)
}

## API Endpoints

### HTTP Endpoints

```
# Workflow Templates & Board Workflows
GET    /api/v1/workflow-templates              # List available templates
GET    /api/v1/workflow-templates/:id          # Get template details
POST   /api/v1/boards/:id/workflow             # Apply workflow to board
GET    /api/v1/boards/:id/workflow             # Get board's workflow
GET    /api/v1/boards/:id/workflow/steps       # Get workflow steps

# Session Workflow Operations (workflow operates at session level)
POST   /api/v1/sessions/:id/workflow/approve   # Approve session's current step
POST   /api/v1/sessions/:id/workflow/reject    # Reject session / request changes
POST   /api/v1/sessions/:id/workflow/move      # Move session to a different step
GET    /api/v1/sessions/:id/step               # Get session's current workflow step

# Primary Session Operations
GET    /api/v1/tasks/:id/primary-session       # Get task's primary session
PUT    /api/v1/tasks/:id/primary-session       # Set primary session { session_id }
```

### WebSocket Actions

```
# Session-level workflow actions
session.workflow.approve   { session_id }                # Approve session step
session.workflow.reject    { session_id, reason }        # Reject session with feedback
session.workflow.move      { session_id, step_id }       # Move session to step

# Primary session actions
task.primary_session.set   { task_id, session_id }       # Change primary session

# Notifications
session.step.changed       { session_id, step_id, ... }  # Session moved to new step
session.review.requested   { session_id, ... }           # Session entered review step
session.approved           { session_id, ... }           # Session was approved
task.primary_session.changed { task_id, session_id }     # Primary session changed
```

## Resolved Design Decisions

| Decision | Resolution | Rationale |
|----------|------------|-----------|
| **Columns vs Steps** | Columns ARE steps (`workflow_steps` replaces `columns`) | Simplicity - one entity instead of two with a relationship |
| **Plan Storage** | Files in worktree (`.kandev/plans/{session_id}.md`) | Simple, no database storage needed, accessible in editor |
| **Custom Workflows** | Full support (templates + from scratch) | Users can use templates, customize them, or create entirely new workflows |
| **Rejection Behavior** | Stay in review + mark "changes requested" | Enables iterative review cycles without moving cards back and forth |
| **Backward Compatibility** | None required | Implementing from scratch, following current patterns and abstractions |
| **Workflow Level** | Session-level (not task-level) | Multiple sessions can work on a task independently, each progressing through the workflow |
| **Task Display** | Primary session determines kanban column | Latest session is auto-primary; user can change. Simpler than "most advanced" logic |

## Open Questions

1. **Step Skipping**: Should users be able to skip steps when starting a new session?
   - Currently: Sessions can start at any step
   - Consider: Add `allowedStartSteps` array for more control

---

## Next Steps

### Phase 1: Database Schema
1. [ ] Create `workflow_templates` table with seed data (dev, architecture, simple)
2. [ ] Create `workflow_steps` table (replaces `columns`)
3. [ ] Update `boards` table with `workflow_template_id`
4. [ ] Update `task_sessions` table: add `is_primary`, `workflow_step_id`, `review_status`, `review_feedback`
5. [ ] Create `session_step_history` table

### Phase 2: Backend Services
6. [ ] Implement `WorkflowService` (CRUD for templates and steps)
7. [ ] Update `SessionService` with workflow step operations (`MoveSessionToStep`, etc.)
10. [ ] Implement primary session operations (`GetPrimarySession`, `SetPrimarySession`)
11. [ ] Add workflow event handlers (on session created → auto-set as primary, on agent complete)
12. [ ] Add session approval/rejection endpoints

### Phase 3: Frontend - Board Creation
13. [ ] Add workflow template selector to board creation dialog
14. [ ] Show workflow preview when template selected
15. [ ] Create workflow steps when board is created

### Phase 4: Frontend - Session Workflow
16. [ ] Display task card based on primary session's step
17. [ ] Add primary session indicator (★) to task cards
18. [ ] Add session selector to task detail panel with "Set as Primary" action
19. [ ] Add session workflow progress indicator
20. [ ] Add session review status badge to task cards
21. [ ] Implement session review panel with approval/rejection UI
22. [ ] Add session feedback input and history display
23. [ ] Implement "Re-run Session Agent" button for addressing feedback
24. [ ] Add "Start New Session" flow (new session auto-becomes primary)

### Phase 5: Frontend - Workflow Editor
25. [ ] Add workflow editor to board settings
26. [ ] Implement step reordering (drag and drop)
27. [ ] Implement step behavior editing (prompts, auto-start, etc.)
28. [ ] Add step add/remove functionality
