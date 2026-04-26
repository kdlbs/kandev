---
status: draft
created: 2026-04-25
owner: cfl
---

# Orchestrate: Wakeup Queue & Heartbeat Scheduler

## Why

Kandev's existing scheduler is reactive -- tasks enter the queue only when a user explicitly starts them or sends a prompt. There is no mechanism for agents to be woken autonomously when events happen: a task is assigned, a comment is posted, blockers are resolved, a routine fires, or an approval is decided. Without autonomous wakeups, agents cannot operate independently; every interaction requires a human to initiate it.

Orchestrate adds a wakeup queue that sits alongside the existing task queue. Events in the system generate wakeup requests for agent instances. The scheduler claims and processes wakeups by spawning one-shot agent sessions -- a heartbeat model integrated with kandev's existing orchestrator pipeline.

## What

### Wakeup queue

- A SQLite-persisted queue of wakeup requests.
- Each wakeup request contains:
  - `id`: unique identifier.
  - `agent_instance_id`: which agent instance to wake.
  - `reason`: why the agent is being woken (discriminator -- see table below).
  - `payload`: JSON with reason-specific context (task ID, comment ID, approval ID, etc.).
  - `status`: `queued`, `claimed`, `finished`, `failed`.
  - `coalesced_count`: number of wakeups merged into this one.
  - `idempotency_key`: deduplication key to prevent duplicate wakeups from the same event.
  - `requested_at`, `claimed_at`, `finished_at`: lifecycle timestamps.
  - `context_snapshot`: JSON with pre-computed context for the agent prompt (task summary, new comments, etc.) so the agent doesn't need to fetch it.

### Wakeup reasons

| Reason | Trigger | Payload |
|--------|---------|---------|
| `task_assigned` | Task's `assignee_agent_instance_id` is set or changed | `{task_id}` |
| `task_comment` | Comment posted on a task assigned to this agent (non-self). This is also how channels work: inbound Telegram/Slack messages become comments on a channel task, triggering this wakeup. See [orchestrate-assistant](../orchestrate-assistant/spec.md). | `{task_id, comment_id}` |
| `task_blockers_resolved` | All blocking tasks of an assigned task reach `done` | `{task_id, resolved_blocker_ids}` |
| `task_children_completed` | All child tasks of an assigned task reach terminal state | `{task_id}` |
| `approval_resolved` | An approval requested by this agent is approved/rejected | `{approval_id, status, decision_note}` |
| `routine_trigger` | A routine's cron/webhook fires and creates a task assigned to this agent | `{routine_id, routine_run_id, task_id}` |
| `heartbeat` | Periodic timer (configurable per agent, e.g. 60s) | `{}` |
| `budget_alert` | Budget threshold crossed for a sub-agent (CEO only) | `{agent_instance_id, budget_pct}` |
| `agent_error` | A sub-agent's session failed (CEO only) | `{agent_instance_id, session_id, error}` |

### Manual status changes (kanban drag-drop)

- Orchestrate tasks live on the system orchestrate workflow and appear on the kanban board. Users can drag them between columns (steps), which emits a `task.moved` event.
- The orchestrate event subscribers handle `task.moved` events for orchestrate tasks (identified by `assignee_agent_instance_id != null`) and fire the appropriate wakeups:
  - Move to "In Progress" step: `task_assigned` wakeup for the assignee agent.
  - Move to "Done" or "Cancelled" step: resolve blocker dependencies, fire `task_blockers_resolved` / `task_children_completed` wakeups.
  - Move to "In Review" step: if execution_policy has reviewers, wake reviewer agents.
  - Move from "In Review" to "In Progress" step: `task_comment` wakeup with rejection context for the assignee.
- Non-orchestrate tasks (those without `assignee_agent_instance_id`) are ignored by orchestrate subscribers, even if moved on any workflow.

### Coalescing

- Multiple wakeups for the same agent instance with the same reason within a coalescing window (default 5 seconds) merge into a single queued wakeup.
- The merged wakeup's `coalesced_count` is incremented and the `payload` is updated to include all relevant IDs.
- Example: 5 subtasks complete within 3 seconds, generating 5 `task_children_completed` wakeups. These coalesce into 1 wakeup with `coalesced_count=5`.
- Coalescing prevents thrashing when batch events fire in rapid succession.

### Idempotency

- Each wakeup source provides an `idempotency_key` (e.g. `task_assigned:<task_id>:<timestamp>`).
- Duplicate keys within a 24-hour window are silently dropped.
- This prevents webhook re-deliveries, event bus replays, or restart recovery from creating duplicate wakeups.

### Scheduler processing

- The scheduler runs a tick loop (configurable interval, default 5 seconds) that processes both the existing task queue and the new wakeup queue.
- Processing a wakeup:
  1. **Claim**: atomically set `status=claimed`, `claimed_at=now()`. Zero rows updated means another process claimed it.
  2. **Guard**: check agent instance status. If `paused` or `stopped`, mark wakeup `finished` with no action.
  3. **Build context**: assemble the agent's prompt from the wakeup reason, payload, and context snapshot. Include the workspace state summary for CEO heartbeats.
  4. **Resolve executor**: walk the executor resolution chain: task override -> agent instance preference -> project config -> workspace default. This determines which executor backend (local_pc, local_docker, sprites, etc.) and what resource limits, image, and worktree strategy to use. See [orchestrate-agents](../orchestrate-agents/spec.md) for the resolution chain.
  5. **Create session**: create a `TaskSession` through the existing orchestrator pipeline with the resolved executor config. The session's task is determined by the wakeup payload (the assigned task, or a "coordination" task for CEO heartbeats). If the task targets a repo, a git worktree is created per the project's worktree strategy.
  6. **Launch**: start the agent process via the existing lifecycle manager -> executor backend -> agentctl -> agent subprocess chain.
  6. **Complete**: when the session ends, mark the wakeup `finished`. Parse agent output for follow-up actions (new tasks, status changes, comments, approvals).

### One-shot session model

- Each wakeup produces a single agent session that runs to completion and exits.
- The agent receives a structured prompt describing why it was woken:
  - For `task_assigned`: "You have been assigned task [title]. Read the description and start working."
  - For `task_comment`: "A new comment was posted on your task [title]: [comment body]. Address it."
  - For CEO `heartbeat`: "Status update: [N] tasks in progress, [M] completed since last heartbeat, [K] pending assignment. Budget [X]% used. Review and take action."
- The agent responds (writes code, posts comments, creates subtasks, updates status), then the session completes.
- Session resume is supported: if the agent CLI supports session resume and the context hasn't changed, the next wakeup for the same task reuses the session ID for continuity.

### Resume delta prompt

- When resuming a session (same agent, same task, session ID preserved), the agent receives only a resume delta -- the new information since the last run.
- The full instructions and context are skipped (the agent CLI retains them from the previous session).
- This saves significant tokens on follow-up wakeups (5-10K tokens per heartbeat).

### Subtask sequencing via blockers

- Orchestrate does not have a separate workflow/template engine for subtask ordering. The agent's instructions (via skills) define how to decompose work and which subtasks to create.
- Sequencing is enforced through the existing blocker system: the agent creates subtask 2 with `blocked_by: [subtask 1]`.
- The scheduler respects blockers: a `task_assigned` wakeup for a blocked task is held until blockers resolve.
- When a subtask completes:
  1. If `requires_approval=true`: task moves to `in_review`, inbox item created. On user approval, task moves to `done`.
  2. If `requires_approval=false`: task moves directly to `done`.
  3. On reaching `done`: the system checks if any sibling tasks had this task as a blocker. For each newly-unblocked task, a `task_blockers_resolved` wakeup is queued for that task's assigned agent.
- This creates a natural pipeline: Spec (requires_approval) -> Build (blocked_by Spec) -> Review (blocked_by Build) -> Ship (blocked_by Review). The user only intervenes at approval gates; the rest flows automatically.
- The CEO agent is woken via `task_children_completed` when all subtasks under a parent reach terminal state.

### Integration with existing scheduler

- The existing `scheduler.Scheduler` and `queue.TaskQueue` continue to handle user-initiated task execution unchanged.
- The wakeup queue is a parallel path: same scheduler tick loop, different queue, different processing logic.
- Both paths converge at the lifecycle manager -- the same `LaunchAgent()` / `StartAgentProcess()` calls are used regardless of whether the session was user-initiated or wakeup-initiated.

### Agent concurrency

- Each agent instance has a configurable `max_concurrent_sessions` (default 1).
- The scheduler's claim query skips agents that are at capacity by joining against active session counts. Wakeups for busy agents stay in `queued` status and are picked up naturally when the agent has a free slot.
- No re-queuing, no retry limits, no expiry. A slow QA agent with 20 tasks queued processes them sequentially without failures -- each wakeup waits in the queue until the agent finishes its current work.
- Concurrency > 1 is useful for agents handling independent tasks (e.g. multiple code reviews in parallel).

### Claim query

The claim query atomically selects the next eligible wakeup, skipping agents at capacity:
```sql
SELECT w.* FROM orchestrate_wakeup_queue w
JOIN orchestrate_agent_instances a ON a.id = w.agent_instance_id
WHERE w.status = 'queued'
  AND a.status IN ('idle', 'working')
  AND (
    SELECT COUNT(*) FROM task_sessions ts
    WHERE ts.agent_instance_id = w.agent_instance_id
      AND ts.state IN ('STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
  ) < a.max_concurrent_sessions
ORDER BY w.requested_at
LIMIT 1
```

### Heartbeat cooldown

- Each agent instance has a configurable `cooldown_sec` (default 10 seconds).
- After a wakeup finishes, the agent cannot be woken again until the cooldown period has elapsed.
- The claim query checks `last_wakeup_finished_at` on the agent instance and skips agents still in cooldown.
- Prevents rapid re-runs when multiple events fire in quick succession (e.g. several comments posted, multiple blockers resolved simultaneously).
- Without cooldown, the agent could be spawned dozens of times per minute in event-heavy scenarios.

### Retry on agent errors

- When an agent session fails (process crash, timeout, unrecoverable error), the wakeup is not simply marked as failed.
- Instead, the wakeup is retried with exponential backoff: 4 attempts at [2m, 10m, 30m, 2h] with 25% jitter.
- `retry_count` and `scheduled_retry_at` fields on the wakeup track retry state.
- The claim query skips wakeups where `scheduled_retry_at > now()`.
- After 4 failed attempts, the wakeup is marked `failed`, an `agent.error` inbox item is created, and the CEO receives a `agent_error` wakeup.

### Atomic task checkout

- When an agent starts working on a task, it must acquire an exclusive lock via atomic checkout.
- The checkout uses a CAS (compare-and-swap) pattern:
  ```sql
  UPDATE tasks SET checkout_agent_id = $agent, checkout_at = now()
  WHERE id = $task AND checkout_agent_id IS NULL
  RETURNING *
  ```
- Zero rows returned = another agent already holds the lock -> wakeup skipped, no retry.
- When the agent finishes (or fails), the lock is released.
- This prevents two agents from working on the same task simultaneously when concurrency > 1 or multiple agents are assigned.

### Pre-execution budget check

- Before launching an agent session, the scheduler checks all applicable budget policies.
- If any budget is exceeded with `action_on_exceed=pause_agent`, the agent is paused and the wakeup is skipped.
- This prevents wasting tokens on a run that will immediately be followed by a budget-exceeded pause.
- The check evaluates: workspace budget -> project budget -> agent budget.

## Scenarios

- **GIVEN** a task is assigned to a worker agent instance, **WHEN** the assignment is saved, **THEN** a `task_assigned` wakeup is queued for that agent. The scheduler claims it, creates a session, and the agent starts working on the task.

- **GIVEN** a worker agent is currently running a session (at capacity), **WHEN** a `task_comment` wakeup arrives for the same agent, **THEN** the wakeup stays in `queued` status. When the current session completes, the next scheduler tick picks up the wakeup and the agent processes the comment.

- **GIVEN** a task with three subtasks assigned to different workers, **WHEN** all three subtasks reach `done`, **THEN** a single `task_children_completed` wakeup (coalesced) is queued for the parent task's assignee.

- **GIVEN** a CEO agent with a 60-second heartbeat interval, **WHEN** 60 seconds elapse since the last heartbeat, **THEN** a `heartbeat` wakeup is queued. The CEO receives a workspace status summary and can create tasks, reassign work, or take no action.

- **GIVEN** a wakeup for a `paused` agent instance, **WHEN** the scheduler claims it, **THEN** the wakeup is marked `finished` with no session created. The wakeup is not retried.

- **GIVEN** a backend restart, **WHEN** the scheduler starts, **THEN** it reads all `queued` wakeup requests from SQLite and resumes processing them. No wakeups are lost.

- **GIVEN** a parent task with subtasks [Spec (requires_approval, assigned to planner), Build (blocked_by Spec, assigned to developer)], **WHEN** the planner completes the Spec subtask, **THEN** Spec moves to `in_review`. **WHEN** the user approves, **THEN** Spec moves to `done`, Build's blocker resolves, and a `task_blockers_resolved` wakeup is queued for the developer agent. The developer starts working on Build automatically.

## Out of scope

- Distributed scheduling across multiple backend instances (single-process scheduler).
- Priority ordering of wakeups (FIFO within the queue; task priority is handled at assignment time).
- Wakeup scheduling with future timestamps (e.g. "wake at 3pm") -- routines handle scheduled execution.
- Rate limiting per agent beyond the single-concurrency guard.
