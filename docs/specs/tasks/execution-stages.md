---
status: shipped
created: 2026-04-29
owner: cfl
---

# Task Execution Stages

## Why

Tasks today have a single assignee that does all the work. There is no built-in way to route a task through implement → review → approval → ship. Users manually coordinate handoffs between agents, lose context when switching, and have no visibility into where a task sits in its lifecycle. The execution policy system exists but only supports "review" and "approval" stage types — it cannot represent the full lifecycle of a task from implementation through deployment.

## What

### Execution policy stages

- A task MAY have an `execution_policy` with an ordered list of stages. Tasks without a policy run as today: assignee works, completes, done.
- Four stage types: `work`, `review`, `approval`, `ship`.
  - **`work`**: assignee agent implements. Signals completion via `kandev task update --status done --comment "..."`. Auto-advances to next stage.
  - **`review`**: one or more reviewer agents/users evaluate the work. Each signals their verdict via status update (see below). Requires `approvals_needed` threshold. On rejection, returns to most recent `work` stage with aggregated feedback.
  - **`approval`**: one or more agents/users approve. Same threshold logic as review. Human participants see the task in their inbox and approve/reject via UI. On rejection, returns to most recent `work` stage.
  - **`ship`**: assignee agent is re-woken (session resumes) to commit + create PR. Signals completion via `status: done`. Auto-completes the task when done.
- Participants in any stage can be `agent` or `user`. Agent participants are woken via the scheduler. User participants interact via the inbox/UI.
- Stages execute sequentially. Within a stage, all participants are woken in parallel.

### Stage completion via status update

Agents signal stage outcomes by updating the task status. The server applies execution policy transitions automatically — agents do not manage handoffs or need to know about stages.

| Status update | Stage type | Effect |
|---|---|---|
| `status: done` + comment | `work` / `ship` | Stage complete — advance to next stage |
| `status: done` + comment | `review` / `approval` | Approve — record verdict, advance if threshold met |
| `status: in_progress` + comment | `review` / `approval` | Request changes — record verdict, return to most recent `work` stage if threshold met |

**Comment is mandatory** on every stage transition. If an agent exits without updating status, the system retries. The comment becomes the audit trail entry for the stage.

**Checkout lock**: only one agent works on a task at a time. The active participant holds the lock; the system releases it when the agent completes its status update.

### Agent decision tree

1. Agent wakes — receives task context (description, stage type, feedback if rework)
2. Agent does work (or review)
3. Agent runs `kandev task update --status done --comment "..."` (or `--status in_progress` to request changes)
4. System handles everything else: records verdict, reassigns, wakes next participant

### Rejection re-entry

- When a review or approval stage rejects, the task returns to the most recent `work` stage (not stage 0).
- The assignee is re-woken on the **same task** (ACP session resumes, full context preserved).
- The wakeup payload (`execution_changes_requested`) includes aggregated feedback from all reviewers: verdict + comments per participant.
- After the assignee completes rework, the execution policy re-enters the rejecting stage (same participants, fresh responses).
- This loop repeats until the stage approves or a human intervenes.

### Reviewer context

- When a reviewer agent is woken (wakeup reason: `execution_review_requested`), it gets a fresh ACP session (not the builder's session).
- The prompt includes: task title/description, stage info (stage ID, type, that this is a review), builder's latest comments on the task, and the git diff of changes.
- Reviewers submit their verdict via `kandev task update --status done --comment "LGTM"` (approve) or `--status in_progress --comment "Needs fix: ..."` (request changes).

### Task creation with policy

- `kandev task create` accepts `--execution-policy <json>` and `--blocked-by <task_id,...>` flags so the CEO agent can create fully-configured subtask pipelines in one shot.
- The MCP `createTask` handler accepts `execution_policy` and `assignee_agent_instance_id` fields.
- The execution policy is optional. Omitting it means no stages — the task runs as a simple assignee-completes flow.

### UI stage indicator

- Tasks with an execution policy show a stage progress bar in the task detail top bar, alongside the existing workflow step indicator.
- Format: step pills showing each stage name, with the current stage highlighted and completed stages marked.
- For simple tasks (no policy), no stage bar is shown.
- Each stage pill shows participant avatars/icons and their verdict status (pending/approved/rejected).

## Scenarios

- **GIVEN** a task with policy `[work, review, approval]` and a builder assignee, **WHEN** the builder runs `kandev task update --status done --comment "Implementation complete"`, **THEN** the execution policy advances to the review stage and all review participants are woken.

- **GIVEN** a task in review stage with 2 reviewers and `approvals_needed: 2`, **WHEN** both reviewers run `kandev task update --status done --comment "LGTM"`, **THEN** the stage advances to approval and the human approver sees the task in their inbox.

- **GIVEN** a task in review stage, **WHEN** a reviewer runs `kandev task update --status in_progress --comment "Needs fix: ..."`, **THEN** the task returns to the work stage, the builder is re-woken with aggregated feedback, and the builder's existing session resumes.

- **GIVEN** a task in review stage, **WHEN** a reviewer requests changes and the builder fixes and runs `kandev task update --status done --comment "Fixed"`, **THEN** the execution policy re-enters the same review stage with fresh responses from all reviewers.

- **GIVEN** a task with a `ship` stage after approval, **WHEN** the human approves, **THEN** the builder is re-woken (session resumes) with a prompt to commit and create a PR, and completes via `kandev task update --status done --comment "PR created: ..."`.

- **GIVEN** a task with no execution policy, **WHEN** the assignee completes, **THEN** the task moves to done with no stage processing (existing behavior unchanged).

- **GIVEN** a CEO agent creating subtasks, **WHEN** it runs `kandev task create --title "Build feature" --assignee builder --execution-policy '{"stages":[...]}'  --blocked-by TASK-1`, **THEN** the task is created with the policy and blocker in one call.

- **GIVEN** a task with stages, **WHEN** viewing the task detail, **THEN** a stage progress bar shows: `[Implement] → [Review] → [Approval] → [Ship]` with the current stage highlighted.

- **GIVEN** an agent that exits without calling `kandev task update`, **WHEN** the session ends, **THEN** the system retries the wakeup rather than silently advancing the stage.

## Out of scope

- Conditional stage skipping (e.g. "skip review if change is < 10 lines")
- Sequential reviewers within a single stage (reviewers always run in parallel)
- Custom stage types beyond work/review/approval/ship
- Automatic retry limits on the rejection loop (loops until approved or human intervenes)
- Stage-level timeouts or SLAs
