package scheduler

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// Canonical lowercase status values used inside the pipeline. Backend
// uppercase task states are normalised to these via normalisedStatus.
const (
	statusBacklog    = "backlog"
	statusTodo       = "todo"
	statusInProgress = "in_progress"
	statusInReview   = "in_review"
	statusBlocked    = "blocked"
	statusDone       = "done"
	statusCancelled  = "cancelled"
)

// TaskSnapshot is the minimal pre-update view of a task that the
// reactivity pipeline reads. Built by the caller (typically the
// dashboard adapter) from whatever repo methods it has access to.
type TaskSnapshot struct {
	ID                     string
	WorkspaceID            string
	State                  string // pre-update DB state, e.g. "TODO" / "REVIEW" / "BLOCKED"
	AssigneeAgentProfileID string
	ParentID               string
}

// TaskMutation describes a property change being applied to a task. The
// reactivity pipeline reads this struct + the task's current state to
// compute downstream runs, stage transitions, and side-effects.
//
// Pointer fields are nil when the field is unchanged. `Cancel = true`
// means status is moving to "cancelled" and the active session should
// be hard-interrupted.
type TaskMutation struct {
	// What's changing.
	NewStatus     *string // nil = unchanged
	NewAssigneeID *string
	NewPriority   *string
	Comment       *MutationComment // user/agent comment if this mutation includes one
	ReopenIntent  bool             // explicit reopen=true (or status moves done|cancelled → todo|in_progress)
	ResumeIntent  bool             // explicit resume=true (validated upstream to require comment)

	// Who is acting.
	ActorID   string
	ActorType string // "user" | "agent"
}

// MutationComment is the slim view of a comment relevant to the
// reactivity pipeline.
type MutationComment struct {
	ID         string
	Body       string
	AuthorType string // "user" | "agent"
	AuthorID   string
}

// ApplyTaskMutationResult summarises the pipeline's side-effects.
// Returned for tests and observability; the pipeline applies them
// itself (queues runs, etc.) before returning.
type ApplyTaskMutationResult struct {
	Runs               []QueuedRunSummary // for tests/observability
	InterruptSessionID string             // task ID whose active session should be hard-cancelled
}

// QueuedRunSummary is a thin record of a run queued by the pipeline.
type QueuedRunSummary struct {
	AgentID string
	Reason  string
	TaskID  string
}

// ApplyTaskMutation runs the reactivity pipeline for a task property
// mutation. Call it AFTER persisting the user's change so the pipeline
// reads the new state. The current implementation is effectively
// "best-effort synchronous" — runs are queued one at a time and a
// single failure doesn't abort the rest, matching the existing event-
// subscriber behaviour. Failures are logged and surfaced as the
// returned error from the LAST failed step.
//
// The returned summary lists every run actually queued (post-dedupe)
// so tests can assert on shape.
func (ss *SchedulerService) ApplyTaskMutation(
	ctx context.Context, task *TaskSnapshot, change TaskMutation,
) (*ApplyTaskMutationResult, error) {
	res := &ApplyTaskMutationResult{}
	if task == nil {
		return res, fmt.Errorf("ApplyTaskMutation: task is nil")
	}

	// Per-mutation dedupe keyed by "{agentID}:{taskID}:{reason}" so
	// the same agent never gets two runs for the same task+reason
	// in one mutation.
	seen := map[string]struct{}{}

	queue := func(agentID string, c RunContext) {
		if agentID == "" {
			return
		}
		key := fmt.Sprintf("%s:%s:%s", agentID, c.TaskID, c.Reason)
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		if err := ss.QueueRunCtx(ctx, agentID, c); err != nil {
			ss.logger.Error("reactivity run failed",
				zap.String("agent", agentID),
				zap.String("reason", c.Reason),
				zap.Error(err))
			return
		}
		res.Runs = append(res.Runs, QueuedRunSummary{
			AgentID: agentID, Reason: c.Reason, TaskID: c.TaskID,
		})
	}

	// --- Status change reactions ---
	if change.NewStatus != nil {
		ss.reactToStatusChange(ctx, task, *change.NewStatus, change, queue, res)
	}

	// --- Assignee handoff ---
	if change.NewAssigneeID != nil && *change.NewAssigneeID != task.AssigneeAgentProfileID {
		ss.reactToAssigneeChange(task, *change.NewAssigneeID, change, queue, res)
	}

	// --- Comment reactions (assignee + @mentions) ---
	if change.Comment != nil {
		ss.reactToComment(ctx, task, change.Comment, queue)
	}

	return res, nil
}

// reactToStatusChange queues runs based on what the new status
// triggers. Mutates `res` directly for results that aren't runs
// (interrupt session ID).
func (ss *SchedulerService) reactToStatusChange(
	ctx context.Context,
	task *TaskSnapshot,
	newStatus string,
	change TaskMutation,
	queue func(string, RunContext),
	res *ApplyTaskMutationResult,
) {
	prev := normalisedStatus(task.State)
	next := normalisedStatus(newStatus)

	switch {
	case next == statusDone:
		// Wake dependents — their blocker is now resolved.
		ss.cascadeBlockersResolved(ctx, task, queue)
		// Wake parent if all siblings are done.
		ss.cascadeChildrenCompleted(ctx, task, queue)

	case next == statusCancelled:
		// Hard-cancel the active session.
		res.InterruptSessionID = task.ID

	case next == statusInReview && prev != statusInReview:
		// Fan out task_review_requested to every reviewer AND every
		// approver listed on the task. Reads participants directly
		// off the office repo — chosen over an extra interface
		// because the scheduler already holds `ss.repo` and there is
		// only one caller. Role is included in the payload so the
		// agent's prompt builder can render the correct framing.
		ss.cascadeReviewRequested(ctx, task, change, queue)

	case prev == statusBlocked && next != statusBlocked:
		// Unblocked — wake assignee with task_unblocked.
		queue(task.AssigneeAgentProfileID, RunContext{
			Reason:      RunReasonTaskUnblocked,
			TaskID:      task.ID,
			WorkspaceID: task.WorkspaceID,
			ActorID:     change.ActorID,
			ActorType:   change.ActorType,
		})

	case (prev == statusDone || prev == statusCancelled) && (next == statusTodo || next == statusInProgress):
		// Reopen — different reason if a comment was attached.
		reason := RunReasonTaskReopened
		commentID := ""
		if change.Comment != nil {
			reason = RunReasonTaskReopenedComment
			commentID = change.Comment.ID
		}
		if change.ResumeIntent && change.Comment != nil {
			// Explicit resume always uses the comment-flavoured reason.
			reason = RunReasonTaskReopenedComment
		}
		queue(task.AssigneeAgentProfileID, RunContext{
			Reason:      reason,
			TaskID:      task.ID,
			WorkspaceID: task.WorkspaceID,
			ActorID:     change.ActorID,
			ActorType:   change.ActorType,
			CommentID:   commentID,
		})
	}
}

// reactToComment wakes the assignee (with the self-comment carve-out)
// and any @-mentioned agents.
// reactToAssigneeChange handles a task being reassigned. The pipeline:
//   - hard-cancels the previous assignee's active session (if any) by
//     setting InterruptSessionID — caller is expected to invoke the
//     TaskCanceller post-commit
//   - wakes the new assignee with reason "task_assigned" + actor context
//
// The previous assignee is NOT separately notified — interrupting their
// run is the signal that they're no longer in charge.
func (ss *SchedulerService) reactToAssigneeChange(
	task *TaskSnapshot,
	newAssigneeID string,
	change TaskMutation,
	queue func(string, RunContext),
	res *ApplyTaskMutationResult,
) {
	// Cancel the prior assignee's session if there was one. We re-use
	// InterruptSessionID — status→cancelled also sets it; either reason
	// for hard-cancelling produces the same downstream call.
	if task.AssigneeAgentProfileID != "" && res.InterruptSessionID == "" {
		res.InterruptSessionID = task.ID
	}

	// Wake the new assignee.
	commentID := ""
	if change.Comment != nil {
		commentID = change.Comment.ID
	}
	queue(newAssigneeID, RunContext{
		Reason:      RunReasonTaskAssigned,
		TaskID:      task.ID,
		WorkspaceID: task.WorkspaceID,
		ActorID:     change.ActorID,
		ActorType:   change.ActorType,
		CommentID:   commentID,
	})
}

func (ss *SchedulerService) reactToComment(
	ctx context.Context,
	task *TaskSnapshot,
	comment *MutationComment,
	queue func(string, RunContext),
) {
	closed := isClosedStatus(task.State)
	selfComment := comment.AuthorType == "agent" && comment.AuthorID == task.AssigneeAgentProfileID

	// Assignee wake — skip if self-comment or task is closed.
	if !selfComment && !closed {
		queue(task.AssigneeAgentProfileID, RunContext{
			Reason:      RunReasonTaskComment,
			TaskID:      task.ID,
			WorkspaceID: task.WorkspaceID,
			ActorID:     comment.AuthorID,
			ActorType:   comment.AuthorType,
			CommentID:   comment.ID,
		})
	}

	// @mentions — additive; uses different reason so the runtime can pick
	// a mentioned-system-prompt and skip auto-checkout (which wakes the
	// mentioned agent without stealing ownership from the assignee).
	mentioned, err := ss.FindMentionedAgents(ctx, task.WorkspaceID, comment.Body)
	if err != nil {
		ss.logger.Error("resolve mentions failed",
			zap.String("task_id", task.ID), zap.Error(err))
		return
	}
	for _, agentID := range mentioned {
		// Skip if the mentioned agent IS the comment author (no self-mention loop).
		if agentID == comment.AuthorID {
			continue
		}
		queue(agentID, RunContext{
			Reason:      RunReasonTaskMentioned,
			TaskID:      task.ID,
			WorkspaceID: task.WorkspaceID,
			ActorID:     comment.AuthorID,
			ActorType:   comment.AuthorType,
			CommentID:   comment.ID,
		})
	}
}

// cascadeReviewRequested fans a task_review_requested run out to
// every agent in the task's reviewers AND approvers lists. Each
// recipient gets the role they hold in the RunContext so the prompt
// builder can render an appropriate "you are the reviewer/approver"
// framing.
//
// We read participants via the scheduler's repo directly. The
// alternative was an extra interface on dashboard.ReactivityApplier;
// chosen the direct read because (a) the scheduler already owns the
// repo handle, (b) there is only one caller, and (c) the participants
// table is part of the office schema the scheduler manages.
func (ss *SchedulerService) cascadeReviewRequested(
	ctx context.Context,
	task *TaskSnapshot,
	change TaskMutation,
	queue func(string, RunContext),
) {
	parts, err := ss.repo.ListAllTaskParticipants(ctx, task.ID)
	if err != nil {
		ss.logger.Error("list participants for review fanout failed",
			zap.String("task_id", task.ID), zap.Error(err))
		return
	}
	for _, p := range parts {
		if p.AgentProfileID == "" {
			continue
		}
		queue(p.AgentProfileID, RunContext{
			Reason:      RunReasonTaskReviewRequested,
			TaskID:      task.ID,
			WorkspaceID: task.WorkspaceID,
			ActorID:     change.ActorID,
			ActorType:   change.ActorType,
			Role:        p.Role,
		})
	}
}

// cascadeBlockersResolved wakes any task blocked by `task` whose other
// blockers are also resolved. Re-uses the existing helper which scans
// the blockers table.
func (ss *SchedulerService) cascadeBlockersResolved(
	ctx context.Context, task *TaskSnapshot, queue func(string, RunContext),
) {
	blockedTaskIDs, err := ss.repo.ListTasksBlockedBy(ctx, task.ID)
	if err != nil {
		ss.logger.Error("list blocked-by tasks failed",
			zap.String("task_id", task.ID), zap.Error(err))
		return
	}
	for _, blockedID := range blockedTaskIDs {
		// Verify all OTHER blockers are also resolved.
		ready, err := ss.allBlockersResolvedExcept(ctx, blockedID, task.ID)
		if err != nil || !ready {
			continue
		}
		assignee, err := ss.repo.GetTaskAssignee(ctx, blockedID)
		if err != nil || assignee == "" {
			continue
		}
		queue(assignee, RunContext{
			Reason:                RunReasonTaskBlockersResolved,
			TaskID:                blockedID,
			WorkspaceID:           task.WorkspaceID,
			ResolvedBlockerTaskID: task.ID,
		})
	}
}

// cascadeChildrenCompleted wakes the parent if all its children are now
// in a terminal state.
func (ss *SchedulerService) cascadeChildrenCompleted(
	ctx context.Context, task *TaskSnapshot, queue func(string, RunContext),
) {
	if task.ParentID == "" {
		return
	}
	allDone, err := ss.repo.AreAllChildrenTerminal(ctx, task.ParentID)
	if err != nil || !allDone {
		return
	}
	parentAssignee, err := ss.repo.GetTaskAssignee(ctx, task.ParentID)
	if err != nil || parentAssignee == "" {
		return
	}
	queue(parentAssignee, RunContext{
		Reason:      RunReasonTaskChildrenCompleted,
		TaskID:      task.ParentID,
		WorkspaceID: task.WorkspaceID,
		ChildTaskID: task.ID,
	})
}

// allBlockersResolvedExcept returns true if every blocker on `taskID`
// other than `excludeBlockerID` is in a terminal step.
func (ss *SchedulerService) allBlockersResolvedExcept(
	ctx context.Context, taskID, excludeBlockerID string,
) (bool, error) {
	blockers, err := ss.repo.ListTaskBlockers(ctx, taskID)
	if err != nil {
		return false, err
	}
	for _, b := range blockers {
		if b.BlockerTaskID == excludeBlockerID {
			continue
		}
		done, err := ss.repo.IsTaskInTerminalStep(ctx, b.BlockerTaskID)
		if err != nil {
			return false, err
		}
		if !done {
			return false, nil
		}
	}
	return true, nil
}

// normalisedStatus maps both backend uppercase task states (TODO,
// IN_PROGRESS, REVIEW, COMPLETED, …) and lowercase office canonical
// names (todo, in_progress, in_review, done, …) to the canonical
// lowercase form used in the reactivity pipeline.
func normalisedStatus(s string) string {
	switch s {
	case "TODO", statusTodo, "CREATED", "SCHEDULING":
		return statusTodo
	case "IN_PROGRESS", statusInProgress, "WAITING_FOR_INPUT":
		return statusInProgress
	case "REVIEW", "review", statusInReview:
		return statusInReview
	case "BLOCKED", statusBlocked, "FAILED":
		return statusBlocked
	case "COMPLETED", "completed", "DONE", statusDone:
		return statusDone
	case "CANCELLED", statusCancelled, "canceled":
		return statusCancelled
	case "BACKLOG", statusBacklog:
		return statusBacklog
	}
	return s
}

// isClosedStatus returns true if the status represents a closed task
// (done or cancelled). User comments on closed tasks don't wake the
// assignee unless an explicit reopen flow is used.
func isClosedStatus(s string) bool {
	n := normalisedStatus(s)
	return n == "done" || n == "cancelled"
}
