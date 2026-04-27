# Orchestrate: Subtask Wakeup & Review Flow Fixes

**Date:** 2026-04-27
**Status:** proposed
**Specs affected:** `orchestrate-scheduler`, `orchestrate-inbox`
**Plans affected:** Wave 3 (task model), Wave 4 (scheduler/prompt builder), Wave 5 (execution policy)
**Reference:** Paperclip analysis (`/Users/cfl/Projects/paperclip/analysis/tasks-scheduler.md`, `agent-runtime-context.md`)

## Problem

Investigation of the subtask completion -> parent wakeup -> review verdict chain revealed four gaps where the current implementation diverges from what's needed for end-to-end autonomous agent coordination. These gaps were identified by comparing the kandev orchestrate implementation against the paperclip reference architecture.

## Changes

### 1. Enrich `task_children_completed` payload with child summaries

**Current state:** The wakeup payload is `{"task_id": "<parent_id>"}`. The prompt says "All child tasks have completed. Review their output and determine next steps." but gives zero information about what the children actually did. The parent agent wakes blind and would need N API calls to reconstruct context.

**Paperclip reference:** The `issue_children_completed` wake includes `childIssueSummaries[]` with `{ id, identifier, title, status, priority, assigneeAgentId, summary }` where `summary` is the last comment per child truncated to 500 chars. Max 20 children, with a `childIssueSummaryTruncated` flag when there are more.

**Change:**

Affected files:
- `internal/orchestrate/service/event_subscribers.go` — `queueChildrenCompletedWakeup()`
- `internal/orchestrate/service/prompt_builder.go` — `buildChildrenCompletedPrompt()`
- `internal/orchestrate/repository/sqlite/` — new query for child summaries

Implementation:
1. Add repository method `GetChildSummaries(ctx, parentID)` that returns `[]ChildSummary` with fields: `TaskID`, `Identifier`, `Title`, `Status`, `AssigneeAgentInstanceID`, `LastComment` (truncated to 500 chars). Query joins tasks with latest comment per task. Limit 20 rows. Return a `Truncated bool` if there are more than 20.
2. In `queueChildrenCompletedWakeup()`, after confirming all children are terminal, call `GetChildSummaries()` and include the result in the wakeup payload JSON.
3. In `buildChildrenCompletedPrompt()`, render child summaries into the prompt:
   ```
   All child tasks for your task KAN-1: Add OAuth2 have completed.

   Completed children:
   - KAN-2 (Auth service) [done] — "Implemented JWT token generation and validation..."
   - KAN-3 (API gateway) [done] — "Added rate limiting middleware and auth proxy..."
   - KAN-4 (Web app) [done] — "Built login/signup forms with OAuth2 redirect flow..."
   - KAN-5 (QA) [done] — "All 47 tests pass. Coverage at 89%..."

   Review their output and determine next steps.
   ```
4. If `Truncated`, append: `(showing 20 of N children — fetch the full list via API)`

**Spec updates:**
- `orchestrate-scheduler` spec, wakeup reasons table: change `task_children_completed` payload from `{task_id}` to `{task_id, children: [{id, identifier, title, status, summary}], truncated}`.
- Wave 4 plan, section 4C (prompt builder): update `task_children_completed` prompt template to include child summaries.

**Tests:**
- `queueChildrenCompletedWakeup` includes child summaries in payload
- Last comment per child truncated at 500 chars with ` [truncated]` suffix
- Max 20 children, `truncated=true` when more exist
- Prompt renders summaries correctly
- Children with no comments show no summary line (just title + status)

---

### 2. Support `blocked_by` on task creation

**Current state:** `CreateTaskRequest` has no `blocked_by` field. An agent must create a task, get its ID, then call `AddBlocker()` separately. Two-step, error-prone if the agent is interrupted between calls.

**Paperclip reference:** `POST /companies/{id}/issues` accepts `blockedByIssueIds: string[]` directly. One call creates the task with its blockers atomically.

**Change:**

Affected files:
- `internal/task/dto/requests.go` — `CreateTaskRequest`
- `internal/task/service/service_tasks.go` — `CreateTask()`
- `internal/mcp/handlers/handlers.go` — `handleCreateTask`

Implementation:
1. Add `BlockedBy []string` field to `CreateTaskRequest` (JSON: `"blocked_by"`).
2. In `CreateTask()`, after inserting the task, iterate `BlockedBy` and call `s.AddBlocker(ctx, task.ID, blockerID)` for each. `AddBlocker` already handles validation and circular dependency checks. If any blocker call fails, the whole creation fails (transaction).
3. Update MCP handler `handleCreateTask` to parse and pass through `blocked_by`.

**Spec updates:**
- `orchestrate-scheduler` spec, section "Subtask sequencing via blockers" line 99: change "the agent creates subtask 2 with `blocked_by: [subtask 1]`" — this is already written as if the feature exists. No spec text change needed, just the implementation.
- Wave 3 plan, section 3A: add `BlockedBy` to the task model extensions list.

**Tests:**
- Create task with `blocked_by: ["task-A"]` — blocker relationship created
- Create task with `blocked_by: ["task-A", "task-B"]` — both blockers created
- Create task with `blocked_by` containing circular dependency — creation fails
- Create task with `blocked_by` containing nonexistent task ID — creation fails
- Create task with empty `blocked_by` — no blockers, no error

---

### 3. Wire reviewer verdicts through task.moved events (no new endpoint)

**Current state:** `RecordParticipantResponse()` exists as a service method but has no HTTP endpoint. Only called in tests. When a reviewer agent is woken with `review_request`, it has no way to submit its verdict back to the execution policy.

**Paperclip reference:** There is no separate verdict API. The reviewer agent simply `PATCH`es the issue status: `status=done` + comment = approve, `status=in_progress` + comment = changes_requested. The `applyIssueExecutionPolicyTransition()` function intercepts the status change and records the decision. The reviewer doesn't need to know about execution policy internals.

**Change:**

Affected files:
- `internal/orchestrate/service/event_subscribers.go` — `handleTaskMoved()`
- `internal/orchestrate/service/execution_policy.go` — minor: ensure `RecordParticipantResponse` handles the intercept case

Implementation:
1. In `handleTaskMoved()`, when a task in `in_review` status is moved:
   - **To Done step** (by a reviewer agent): intercept as an approval verdict.
     - Identify the mover as a participant in the current execution policy stage (match `agent_instance_id` from the session against stage participants).
     - Call `RecordParticipantResponse(ctx, taskID, participantID, "approve", comment)`.
     - The task does NOT actually move to Done yet — `RecordParticipantResponse` handles stage advancement or rejection.
   - **To In Progress step** (by a reviewer agent): intercept as a rejection verdict.
     - Same participant matching.
     - Call `RecordParticipantResponse(ctx, taskID, participantID, "reject", comment)`.
     - The task stays in `in_review` until all responses are in, then `evaluateStageCompletion` returns it to In Progress with aggregated feedback.
2. The `task.moved` event needs to carry the `session_id` or `agent_instance_id` of the mover so the subscriber can match against execution policy participants. Verify this field is already present in `TaskMovedData` (it has `session_id` — resolve agent instance from session).
3. If the mover is NOT a participant in the current stage (e.g., the original assignee or a user dragging on kanban), fall through to the existing behavior (no interception).

**Spec updates:**
- `orchestrate-inbox` spec, section "Review/approval flow" (lines 104-112): add clarification that reviewer agents record verdicts by moving the task status (Done = approve, In Progress = reject), and the system intercepts these moves to record the verdict in the execution policy. No separate verdict API.
- Wave 5 plan, section 5D (execution policy flow): update `RecordParticipantResponse` description to note it's called from the event subscriber, not from an HTTP endpoint.

**Tests:**
- Reviewer agent moves task to Done -> verdict recorded as "approve"
- Reviewer agent moves task to In Progress -> verdict recorded as "reject"
- Non-participant moves task to Done -> standard done flow (no interception)
- All reviewers approve -> task advances to next stage (or done)
- One reviewer rejects, others approve -> task returns to in_progress with all feedback
- User drags task on kanban -> no interception, normal move
- Mover identification: session_id -> agent_instance_id -> match against stage participants

---

### 4. Consolidate review entry points (remove legacy `onMovedToInReview`)

**Current state:** Two parallel paths exist:
- `onMovedToDone()` with execution policy -> `EnterReviewStage()` (proper staged flow with participants, quorum, feedback aggregation)
- `onMovedToInReview()` -> `extractReviewers()` (legacy flat format, just wakes agents with no stage tracking)

The legacy path uses a flat `{"reviewers": [...]}` JSON format that doesn't match the `ExecutionPolicy` struct. This creates confusion about which path fires when.

**Paperclip reference:** Single path: agent marks issue done -> `applyIssueExecutionPolicyTransition()` intercepts -> enters review if policy exists. The `in_review` status is set by the system, never by the agent directly.

**Change:**

Affected files:
- `internal/orchestrate/service/event_subscribers.go` — `onMovedToInReview()`, `handleTaskMoved()`

Implementation:
1. Remove `onMovedToInReview()` and `extractReviewers()`.
2. The review flow is exclusively triggered by `onMovedToDone()` when an execution policy is present:
   - Task has execution policy -> `EnterReviewStage()` sets the task to "In Review" and wakes reviewers.
   - Task has no execution policy -> `finalizeDone()` (blockers, parent notification).
3. If a task is manually moved to "In Review" (kanban drag-drop) without an execution policy, no reviewers are woken — it's just a status indicator. The user handles review manually.
4. If a task is manually moved to "In Review" WITH an execution policy, call `EnterReviewStage()` to start the staged flow.

**Spec updates:**
- `orchestrate-scheduler` spec, manual status changes section (line 51): change "Move to 'In Review' step: if execution_policy has reviewers, wake reviewer agents" to "Move to 'In Review' step: if execution_policy exists, enter the staged review flow via `EnterReviewStage()`. Without an execution policy, no automatic reviewer wakeups."
- `orchestrate-scheduler` spec, event subscribers section in Wave 4 plan (line 44): same update.
- `orchestrate-inbox` spec, review flow section (line 104): clarify that the agent marks the task as Done (not In Review), and the system intercepts via execution policy to enter review.

**Tests:**
- Task with execution policy moved to Done -> enters review stage (not finalized as done)
- Task without execution policy moved to Done -> finalized as done directly
- Task with execution policy manually moved to In Review -> enters review stage
- Task without execution policy manually moved to In Review -> no wakeups, just status change
- `extractReviewers()` removed — no references remain

## Implementation Order

These changes span Waves 3-5 but are tightly coupled. Implement in this order:

1. **Change 2** (blocked_by on creation) — pure additive, no existing behavior changes. Wave 3A.
2. **Change 4** (consolidate review entry) — removes legacy code, simplifies flow. Wave 4A event subscribers.
3. **Change 3** (reviewer verdicts via task.moved) — depends on consolidated review flow. Wave 5D.
4. **Change 1** (children summaries) — independent, can be done anytime. Wave 4C prompt builder.

## Verification

1. `make -C apps/backend test` passes
2. Create subtasks with `blocked_by` in a single API call
3. Complete all subtasks -> parent agent wakes with child summaries in prompt
4. Reviewer agent marks task Done -> verdict recorded, execution policy advances
5. All reviewers reject -> assignee wakes with aggregated feedback from all reviewers
6. Legacy `onMovedToInReview` path removed, no regressions
