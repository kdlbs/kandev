---
status: draft
created: 2026-04-27
owner: cfl
---

# Orchestrate: Agent Task Context & Instructions

## Why

When the scheduler wakes an agent to work on a task, the agent receives a plain text prompt with minimal context. It doesn't know its own identity, can't call kandev APIs, has no structured procedure to follow, and gets the full task context on every wakeup (wasting tokens on resume). Agents need a rich, structured context to work autonomously -- identity, task details, API access, and step-by-step instructions.

## What

### Environment variables

Before each agent session, the scheduler injects these environment variables into the agent subprocess:

| Variable | Value | Purpose |
|----------|-------|---------|
| `KANDEV_API_URL` | `http://localhost:<port>/api/v1` | Base URL for kandev API calls |
| `KANDEV_API_KEY` | Per-run JWT | Bearer token for authentication (already implemented via `agent_auth.go`) |
| `KANDEV_AGENT_ID` | Agent instance ID | Agent's own identity |
| `KANDEV_AGENT_NAME` | Agent name (e.g. "CEO") | Human-readable name |
| `KANDEV_WORKSPACE_ID` | Workspace ID | Scope for API calls |
| `KANDEV_TASK_ID` | Task ID (if task-related wakeup) | Which task to work on |
| `KANDEV_RUN_ID` | Wakeup request ID | Audit trail -- include in API calls as `X-Kandev-Run-Id` header |
| `KANDEV_WAKE_REASON` | Wakeup reason string | Why the agent was woken (task_assigned, task_comment, heartbeat, etc.) |
| `KANDEV_WAKE_COMMENT_ID` | Comment ID (if comment wakeup) | Which specific comment triggered the wake |
| `KANDEV_WAKE_PAYLOAD_JSON` | Inline JSON with task summary + new comments | Pre-computed context to avoid API round-trips (see resume delta below) |

These are set in `scheduler_integration.go` when building the agent session, passed to the lifecycle manager which injects them into the agent subprocess environment.

### Wake payload (resume delta)

`KANDEV_WAKE_PAYLOAD_JSON` contains pre-computed context so the agent doesn't need to fetch task details on every run:

```json
{
  "task": {
    "id": "task-123",
    "identifier": "KAN-42",
    "title": "Add OAuth2 login",
    "description": "Implement OAuth2...",
    "status": "in_progress",
    "priority": "high",
    "project": "Backend",
    "assignee": "Frontend Worker",
    "blockedBy": [],
    "childTasks": ["KAN-43", "KAN-44"]
  },
  "newComments": [
    {
      "id": "comment-1",
      "author": "CEO",
      "authorType": "agent",
      "body": "Please prioritize the login flow first.",
      "createdAt": "2026-04-27T10:00:00Z"
    }
  ],
  "commentWindow": {
    "total": 15,
    "included": 3,
    "oldestIncludedAt": "2026-04-27T09:00:00Z",
    "fetchMore": false
  },
  "approval": null
}
```

On **fresh session**: full task context included.
On **resume session**: only new comments since last run included. The agent's existing conversation context has the task details from before.

### Instruction bundles (per role)

Each agent role gets an instruction bundle -- markdown files that define the agent's persona, delegation rules, and operating procedure. These are injected as the agent's system prompt or via a skill.

**CEO bundle** (`~/.kandev/skills/kandev-ceo/`):

`SKILL.md` (the main instructions):
```markdown
---
name: kandev-ceo
description: CEO agent operating instructions for kandev orchestrate
---

# You are the CEO

You manage a team of AI agents. You do NOT write code or implement features yourself.

## Your job
- Read task assignments and break them into subtasks
- Delegate to the right agent based on task type
- Monitor progress and unblock stuck agents
- Hire new agents when the team lacks a capability
- Approve or reject reviews

## Delegation routing
| Task type | Assign to |
|-----------|-----------|
| Code, features, bugs, infrastructure | CTO or developer workers |
| Testing, QA, verification | QA agent |
| Security review | Security agent |
| Documentation | Technical writer |
| Cross-functional (needs multiple) | Split into subtasks, one per agent |
| Unknown capability | Hire a new agent first |

## When you wake up
1. Read your wake reason (KANDEV_WAKE_REASON env var)
2. If task_assigned: read the task, break it down, create subtasks, assign them
3. If task_comment: read the new comment, respond or take action
4. If task_children_completed: check all subtasks done, mark parent complete
5. If approval_resolved: read the decision, act accordingly
6. If heartbeat: check workspace status, reassign stalled tasks

## Creating subtasks
Use the kandev API (see kandev-protocol skill):
- POST /api/v1/tasks with parent_id set to your task
- Set blocked_by to enforce ordering (subtask 2 blocked by subtask 1)
- Set requires_approval=true on subtasks that need human review
- Assign each subtask to the appropriate agent by agent_instance_id

## Hiring agents
If no suitable agent exists for a task type:
- Create a hire request via the kandev API
- The user will approve in the inbox
- Once approved, assign tasks to the new agent

## Rules
- Never implement tasks yourself -- always delegate
- Always post a comment explaining your delegation decisions
- If a task is unclear, post a comment asking for clarification
- Check blockers before starting -- don't work on blocked tasks
```

**Worker bundle** (`~/.kandev/skills/kandev-worker/`):

`SKILL.md`:
```markdown
---
name: kandev-worker
description: Worker agent operating instructions for kandev orchestrate
---

# You are a Worker Agent

You implement tasks assigned to you by the CEO or team leads.

## When you wake up
1. Read your task (from KANDEV_WAKE_PAYLOAD_JSON or fetch via API)
2. Understand the requirements (title, description, comments)
3. Read any blockers -- if blocked, post a comment and exit
4. Do the work in your assigned repository/workspace
5. Post incremental comments as you make progress
6. When done, update the task status to done (or in_review if requires_approval)
7. If the task is too large, create subtasks and assign to yourself

## Working with code
- Read the existing codebase before making changes
- Follow the project's coding conventions
- Write tests for new functionality
- Make small, focused commits with clear messages
- If you need to create a PR, use the git tools available

## Communication
- Post comments on your task to report progress
- If you're stuck, post a comment explaining the blocker
- If you need clarification, ask via a comment
- Keep comments concise and actionable

## Rules
- Only work on tasks assigned to you
- Don't modify files outside your task's scope
- Always check the task description and comments for requirements
- If requires_approval is set, move to in_review instead of done
```

**Reviewer bundle** (`~/.kandev/skills/kandev-reviewer/`):

`SKILL.md`:
```markdown
---
name: kandev-reviewer
description: Reviewer agent operating instructions for kandev orchestrate
---

# You are a Reviewer Agent

You review work done by other agents and provide feedback.

## When you wake up as a reviewer
1. Read the task you're reviewing (from wake payload)
2. Check what the assignee agent did (read comments, check git changes)
3. Review the code changes for:
   - Correctness (does it do what the task asks?)
   - Quality (is the code clean, tested, well-structured?)
   - Security (any vulnerabilities introduced?)
   - Performance (any obvious performance issues?)
4. Post your review as a comment with specific feedback
5. Approve or reject:
   - Approve: POST to the approval endpoint with verdict=approve
   - Reject: POST with verdict=reject and detailed feedback

## Review standards
- Be specific -- point to exact files and lines
- Suggest fixes, don't just flag problems
- Approve if the work meets requirements, even if not perfect
- Reject only for real issues (bugs, security, missing requirements)
```

### Enhanced kandev-protocol skill

The existing `kandev-protocol` skill (43 lines) needs to be expanded to teach agents the full API surface:

```markdown
---
name: kandev-protocol
description: How to interact with the kandev orchestrate API
---

# Kandev Protocol

## Authentication
All API calls use: `Authorization: Bearer $KANDEV_API_KEY`
All mutating calls include: `X-Kandev-Run-Id: $KANDEV_RUN_ID`
Base URL: `$KANDEV_API_URL`

## Heartbeat procedure
When you wake up, follow these steps:

### Step 1: Identify yourself
Read KANDEV_AGENT_ID and KANDEV_AGENT_NAME from env vars.

### Step 2: Read wake reason
Check KANDEV_WAKE_REASON:
- task_assigned: you have a new task
- task_comment: someone commented on your task
- task_blockers_resolved: your blocked task is now unblocked
- task_children_completed: all your subtasks are done
- approval_resolved: an approval you requested was decided
- heartbeat: periodic check-in (CEO only)

### Step 3: Read task context
If KANDEV_WAKE_PAYLOAD_JSON is set, parse it for inline task + comment context.
If not set, fetch: GET /api/v1/orchestrate/tasks/{KANDEV_TASK_ID}

### Step 4: Do the work
Based on your role and the task, implement the required changes.

### Step 5: Post progress
POST /api/v1/orchestrate/tasks/{taskId}/comments
Body: {"body": "your progress update"}

### Step 6: Update status
PATCH /api/v1/orchestrate/tasks/{taskId}
Body: {"status": "done"} or {"status": "in_review"} if requires_approval

### Step 7: Create subtasks (if needed)
POST /api/v1/tasks
Body: {
  "title": "subtask title",
  "description": "what to do",
  "parent_id": "{your task ID}",
  "assignee_agent_instance_id": "{worker agent ID}",
  "project_id": "{project ID}",
  "blocked_by": ["{previous subtask ID}"]
}

### Step 8: Exit
Your session will end. You'll be woken again when:
- Someone comments on your task
- A subtask completes
- An approval is decided
- Your heartbeat timer fires

## API reference

### Tasks
- GET /api/v1/orchestrate/tasks/{id} -- read task details
- PATCH /api/v1/orchestrate/tasks/{id} -- update status, priority
- POST /api/v1/tasks -- create task/subtask
- GET /api/v1/orchestrate/tasks/{id}/comments -- list comments
- POST /api/v1/orchestrate/tasks/{id}/comments -- post comment

### Agents
- GET /api/v1/orchestrate/agents -- list agents (for delegation)
- GET /api/v1/orchestrate/agents/{id} -- get agent details

### Memory
- GET /api/v1/orchestrate/agents/{id}/memory/summary -- read your memory
- PUT /api/v1/orchestrate/agents/{id}/memory -- store a memory
- DELETE /api/v1/orchestrate/agents/{id}/memory/{entryId} -- delete memory

## Critical rules
- Always include X-Kandev-Run-Id header on mutating API calls
- Never retry a 409 Conflict (another agent has the task)
- Always post a comment before changing task status
- Don't work on blocked tasks -- check blockedBy field first
- If you can't complete a task, post a comment explaining why
```

### Prompt structure

The scheduler builds a multi-section prompt for each wakeup:

**Section 1: Instructions** (skipped on resume)
- Role-specific instructions from the agent's instruction skill (CEO, worker, reviewer)
- Only sent on fresh sessions, not on resume (agent CLI retains from previous session)

**Section 2: Wake context** (always sent)
- Why the agent was woken
- Task summary (from wake payload)
- New comments since last run
- Approval result (if approval_resolved)

**Section 3: Workspace status** (CEO heartbeat only)
- Agent counts by status
- Task counts by status
- Budget utilization
- Recent errors

On **resume**, only Section 2 is sent (saves 5-10K tokens by skipping instructions).

### Implementation locations

| Component | File | What to change |
|-----------|------|---------------|
| Env var injection | `service/scheduler_integration.go` | Set env vars before calling task starter |
| Wake payload builder | `service/prompt_builder.go` | Build JSON payload with task + comments |
| Instruction bundles | `configloader/skills/kandev-ceo/SKILL.md` etc. | Write the skill files |
| Protocol skill | `configloader/skills/kandev-protocol/SKILL.md` | Expand from 43 to 200+ lines |
| Skill injection | `service/skill_injection.go` | Already implemented, symlinks bundles |
| Resume detection | `service/scheduler_integration.go` | Check if session ID exists, skip Section 1 |

## Scenarios

- **GIVEN** a task assigned to a worker agent, **WHEN** the scheduler wakes it, **THEN** the agent subprocess has KANDEV_TASK_ID, KANDEV_API_KEY, KANDEV_WAKE_REASON=task_assigned set, and KANDEV_WAKE_PAYLOAD_JSON contains the task details + any comments.

- **GIVEN** a CEO agent on heartbeat, **WHEN** the scheduler wakes it with reason=heartbeat, **THEN** the prompt includes workspace status (agent counts, task counts, budget) and the CEO's instruction bundle teaches it to check for stalled tasks and reassign.

- **GIVEN** a worker agent being woken for a task_comment, **WHEN** it's a resume session (same task, same agent, session ID preserved), **THEN** only the new comment is sent in the prompt (Section 2), not the full instructions (Section 1).

- **GIVEN** a reviewer agent woken to review a task, **WHEN** the execution policy assigns it as a reviewer, **THEN** its prompt includes the reviewer instruction bundle and the task's git changes/PR info.

## Out of scope

- SOUL.md (voice/tone guidelines) -- nice to have for v2
- TOOLS.md (auto-generated tool reference) -- nice to have for v2
- Agent hiring via skill (CEO creating agents via API) -- depends on approval flow maturity
- Multi-workspace wake payload (cross-workspace task references)
