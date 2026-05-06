# Office Wave 4: Wakeup Queue & Heartbeat Scheduler

**Date:** 2026-04-26
**Status:** proposed
**Spec:** `office-scheduler`
**Depends on:** Wave 3 (tasks can be assigned to agent instances)

## Problem

The existing scheduler is reactive -- tasks only execute when a user explicitly starts them. Office needs agents to wake autonomously when events happen (task assigned, comment posted, blockers resolved, etc.). This wave adds the wakeup queue and extends the scheduler to process it.

## Scope

### 4A: Wakeup Queue Service (backend)

**Repository** (`internal/office/repository/sqlite/wakeup.go`):
- `Enqueue(ctx, wakeup)` -- insert with idempotency check
- `Claim(ctx)` -- atomic SELECT + UPDATE where status=queued ORDER BY requested_at LIMIT 1
- `Finish(ctx, id, status)` -- mark claimed wakeup as finished/failed
- `Coalesce(ctx, agentInstanceID, reason, windowSecs)` -- find existing queued wakeup for same agent+reason within window, increment coalesced_count and merge payload
- `ListQueued(ctx, workspaceID)` -- for dashboard status
- `CleanExpired(ctx, olderThan)` -- purge finished wakeups older than retention period
- `RecoverStale(ctx, claimedOlderThan)` -- reset stale `claimed` wakeups back to `queued` (crash recovery)

**Service** (`internal/office/service/wakeup.go`):
- `QueueWakeup(ctx, agentInstanceID, reason, payload, idempotencyKey)`:
  1. Check agent instance status (skip if paused/stopped)
  2. Check idempotency (skip if duplicate key within 24h)
  3. Try to coalesce with existing queued wakeup (same agent + reason within 5s window)
  4. If no coalesce target: insert new wakeup
  5. Emit `office.wakeup.queued` event
- Wakeup reason constants: `task_assigned`, `task_comment`, `task_blockers_resolved`, `task_children_completed`, `approval_resolved`, `routine_trigger`, `heartbeat`, `budget_alert`, `agent_error`

**Event subscribers** (trigger wakeups from system events):

All subscribers guard on `task.assignee_agent_instance_id != ""` -- non-office tasks are ignored.

- Task assignee changed -> `task_assigned` wakeup
- Comment posted on task (non-self) -> `task_comment` wakeup
- Task moved to "Done"/"Cancelled" step (via `task.moved` event):
  - Check if any tasks are blocked_by this task, if all blockers resolved -> `task_blockers_resolved` wakeup for blocked task's assignee
  - Check if parent task's children are all terminal -> `task_children_completed` for parent's assignee
- Task moved to "In Progress" step (via `task.moved` event, e.g. kanban drag-drop) -> `task_assigned` wakeup for assignee
- Task moved to "In Review" step -> if execution_policy has reviewers, wake reviewer agents
- Task moved from "In Review" to "In Progress" step -> `task_comment` wakeup for assignee with rejection context
- Approval resolved -> `approval_resolved` wakeup for requesting agent

Note: `task.moved` events are emitted by both the office scheduler (programmatic status changes) and the kanban drag-drop (manual user moves). The same subscribers handle both -- the user can manually intervene on office tasks via the kanban board.

### 4B: Scheduler Extension (backend)

**Extend existing scheduler** (`internal/orchestrator/scheduler/`):

New method `processWakeupQueue()` called from the existing tick loop alongside `processLoop()`:
1. Call `wakeupRepo.Claim()` to atomically claim next queued wakeup
2. If no wakeup: return (loop continues)
3. **Guard checks:**
   - Load agent instance. If status is `paused`/`stopped`: mark wakeup finished, return
   - (Concurrency is handled at the claim query level -- agents at capacity are skipped, not re-queued)
4. **Build context:**
   - Load the task from wakeup payload (if task-related reason)
   - Build `context_snapshot`: task summary, new comments since last run, approval result
   - For CEO `heartbeat`: build workspace status summary (agent statuses, task counts, budget usage)
5. **Resolve executor** (`internal/office/service/executor_resolver.go`):
   - Walk the resolution chain (first non-null wins):
     1. Task-level override (`task.execution_policy.executor_config`)
     2. Agent instance preference (`agent_instance.executor_preference`)
     3. Project executor config (`project.executor_config`)
     4. Workspace default executor
   - Result: executor type, image, resource limits, environment ID, worktree strategy
   - If task targets a repo and worktree_strategy=per_task: create git worktree for the session
6. **Create one-shot session:**
   - Create a `TaskSession` via existing orchestrator pipeline with the resolved executor config
   - Set agent profile from agent instance's `agent_profile_id`
   - Set executor from resolved config (passed to lifecycle manager)
   - Build the wakeup prompt (structured text describing why the agent was woken)
7. **Launch agent:**
   - Call existing `executor.LaunchAgent()` / `StartAgentProcess()` with resolved executor backend
   - Send the wakeup prompt as initial message
7. **On session complete:**
   - Mark wakeup `finished`
   - Parse agent output for follow-up actions (new tasks, status changes, comments)
   - Emit `office.wakeup.processed` event
   - Log to activity log

**Agent API authentication** (`internal/office/service/agent_auth.go`):
- Before launching, mint a per-run JWT scoped to the agent instance and task:
  - Claims: `agent_instance_id`, `task_id`, `workspace_id`, `session_id`, `exp` (expiry: session timeout + buffer)
  - Signed with the backend's signing key (same key used for existing session tokens)
- Inject as `KANDEV_API_KEY` environment variable in the agent subprocess
- API middleware on office endpoints validates the JWT and scopes access:
  - Agent can read/write its own memory (`/agents/:id/memory` where id matches JWT)
  - Agent can read/update its assigned task and create subtasks
  - Agent can read workspace-level resources (skills, project list, agent list)
  - Agent cannot access other agents' memory or modify other agents' config

**Skill injection at session start** (`internal/office/service/skill_injection.go`):
- Before launching agent process, resolve agent instance's `desired_skills`
- Call `MaterializeSkills()` from Wave 2B to get SKILL.md content per skill
- Write each skill to the agent-specific path: `<worktree>/.claude/skills/kandev-<slug>/SKILL.md` for Claude, `<worktree>/.agents/skills/kandev-<slug>/SKILL.md` for all others
- Add `kandev-*` patterns to `<worktree>/.git/info/exclude` (both `.claude/skills/kandev-*` and `.agents/skills/kandev-*`)
- No cleanup needed: injected skills are deleted automatically with the worktree

**Resume delta prompt:**
- If resuming a session (same agent, same task, session ID preserved):
  - Skip full instructions, send only the new information since last run
  - Include: new comments, approval results, status changes
- If fresh session: include full context

**Heartbeat timer:**
- For agent instances with heartbeat enabled (configurable interval, e.g. 60s):
  - Scheduler maintains a timer map `map[agentInstanceID]nextHeartbeat`
  - On each tick: check if any heartbeat timers are due
  - If due: queue a `heartbeat` wakeup
  - Timer reset after wakeup is processed

### 4C: Wakeup Prompt Builder (backend)

**`internal/office/service/prompt_builder.go`**:

Build structured prompts per wakeup reason:

```
task_assigned:
  "You have been assigned task [KAN-3]: [title].
   Project: [project name]
   Description: [task description]
   Priority: [priority]
   Read the description and start working."

task_comment:
  "New comment on your task [KAN-3]: [title]
   From: [author name] ([author type])
   Comment: [comment body]
   Address this comment."

task_blockers_resolved:
  "All blockers for your task [KAN-3]: [title] have been resolved.
   Resolved blockers: [list of blocker task titles]
   You can now proceed with your work."

approval_resolved:
  "Your approval request [type] has been [approved/rejected].
   Decision note: [note]
   [For hire_agent: The new agent [name] is now active.]
   [For task_review rejected: Review feedback: [notes]. Address the feedback.]"

heartbeat (CEO):
  "Workspace status update:
   Agents: [N] idle, [M] working, [K] paused
   Tasks: [X] in progress, [Y] completed since last heartbeat, [Z] pending assignment
   Budget: [P]% used this month
   Errors: [list any failed sessions since last heartbeat]
   Review the status and take action if needed."
```

## Tests

- Wakeup queue: enqueue, claim (atomic with concurrency check), coalesce, idempotency, stale recovery, cleanup
- Event subscribers: task assignment triggers wakeup, comment triggers wakeup, blocker resolution triggers wakeup
- Scheduler: processes wakeup, respects agent status guards, respects concurrency limits
- Prompt builder: correct prompt per reason type
- Executor resolver: task override wins over agent preference, agent wins over project, project wins over workspace default
- Executor resolver: worktree created when task targets a repo with per_task strategy
- Agent JWT: minted with correct claims, validated on API endpoints, scoped access enforced
- Agent JWT: expired token rejected, wrong agent_instance_id rejected
- Manual drag-drop: move office task to "Done" on kanban -> blockers resolve, wakeups fire
- Manual drag-drop: move office task to "In Progress" -> assignee agent woken
- Manual drag-drop: move non-office task -> no office wakeups fired (guard works)
- Skill injection: skills written to worktree CWD before session, cleaned up with worktree
- Integration: assign task to agent -> wakeup queued -> scheduler claims -> session created -> agent launched

## Verification

1. `make -C apps/backend test` passes
2. Assign a task to an agent instance via UI -> wakeup appears in queue -> scheduler processes it -> agent session starts
3. Post a comment on an assigned task -> agent is woken and responds
4. Complete a blocker task -> blocked task's agent is woken
5. CEO heartbeat fires on interval
6. Paused agents don't get woken
7. Concurrency limit respected (wakeup stays queued, claim query skips busy agents)
8. Slow agent with 20 queued tasks: processes them sequentially, no failures, no re-queue limits
8. Skills written to worktree CWD at the agent-specific skill path during session
