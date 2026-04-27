package service

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// TaskMovedData represents the payload of a task.moved event.
type TaskMovedData struct {
	TaskID                  string `json:"task_id"`
	WorkspaceID             string `json:"workspace_id"`
	FromStepID              string `json:"from_step_id"`
	ToStepID                string `json:"to_step_id"`
	ToStepName              string `json:"to_step_name"`
	FromStepName            string `json:"from_step_name"`
	AssigneeAgentInstanceID string `json:"assignee_agent_instance_id"`
	ParentID                string `json:"parent_id"`
	ExecutionPolicy         string `json:"execution_policy"`
	SessionID               string `json:"session_id"`
}

// TaskUpdatedData represents the payload of a task.updated event.
type TaskUpdatedData struct {
	TaskID                  string `json:"task_id"`
	WorkspaceID             string `json:"workspace_id"`
	AssigneeAgentInstanceID string `json:"assignee_agent_instance_id"`
	Title                   string `json:"title"`
}

// CommentPostedData represents a comment event payload.
type CommentPostedData struct {
	TaskID                  string `json:"task_id"`
	CommentID               string `json:"comment_id"`
	AuthorID                string `json:"author_id"`
	AuthorType              string `json:"author_type"`
	AssigneeAgentInstanceID string `json:"assignee_agent_instance_id"`
}

// ApprovalResolvedData represents an approval resolved event payload.
type ApprovalResolvedData struct {
	ApprovalID                 string `json:"approval_id"`
	Status                     string `json:"status"`
	DecisionNote               string `json:"decision_note"`
	RequestedByAgentInstanceID string `json:"requested_by_agent_instance_id"`
	Type                       string `json:"type"`
}

// RegisterEventSubscribers subscribes to system events and queues wakeups.
func (s *Service) RegisterEventSubscribers(eb bus.EventBus) error {
	s.eb = eb
	subs := []struct {
		subject string
		handler bus.EventHandler
	}{
		{events.TaskUpdated, s.handleTaskUpdated},
		{events.TaskMoved, s.handleTaskMoved},
		{events.OrchestrateApprovalResolved, s.handleApprovalResolved},
		{events.OrchestrateCommentCreated, s.handleCommentCreated},
	}
	for _, sub := range subs {
		if _, err := eb.Subscribe(sub.subject, sub.handler); err != nil {
			return fmt.Errorf("subscribe %s: %w", sub.subject, err)
		}
	}
	return nil
}

// handleTaskUpdated fires a task_assigned wakeup when an agent is assigned.
func (s *Service) handleTaskUpdated(ctx context.Context, event *bus.Event) error {
	data, err := decodeEventData[TaskUpdatedData](event)
	if err != nil || data.AssigneeAgentInstanceID == "" {
		return nil
	}
	payload := mustJSON(map[string]string{"task_id": data.TaskID})
	key := fmt.Sprintf("task_assigned:%s", data.TaskID)
	return s.QueueWakeup(ctx, data.AssigneeAgentInstanceID, WakeupReasonTaskAssigned, payload, key)
}

// handleTaskMoved fires wakeups based on the destination step.
func (s *Service) handleTaskMoved(ctx context.Context, event *bus.Event) error {
	data, err := decodeEventData[TaskMovedData](event)
	if err != nil || data.AssigneeAgentInstanceID == "" {
		return nil
	}

	fromCategory := categorizeStep(data.FromStepName)
	toCategory := categorizeStep(data.ToStepName)

	// Reviewer verdict interception: when a task is in review and a
	// reviewer agent moves it, record the verdict via the execution policy
	// instead of performing the normal move action.
	if fromCategory == stepCategoryInReview {
		if handled, hErr := s.tryRecordReviewerVerdict(ctx, data, toCategory); handled || hErr != nil {
			return hErr
		}
	}

	switch toCategory {
	case stepCategoryInProgress:
		return s.onMovedToInProgress(ctx, data)
	case stepCategoryDone:
		return s.onMovedToDone(ctx, data)
	case stepCategoryInReview:
		return s.onMovedToInReview(ctx, data)
	}
	return nil
}

func (s *Service) onMovedToInProgress(ctx context.Context, data *TaskMovedData) error {
	payload := mustJSON(map[string]string{"task_id": data.TaskID})
	key := fmt.Sprintf("task_assigned:%s:%s", data.TaskID, data.ToStepID)
	return s.QueueWakeup(ctx, data.AssigneeAgentInstanceID, WakeupReasonTaskAssigned, payload, key)
}

func (s *Service) onMovedToDone(ctx context.Context, data *TaskMovedData) error {
	fields, err := s.repo.GetTaskExecutionFields(ctx, data.TaskID)
	if err != nil {
		s.logger.Error("get task execution fields failed", zap.Error(err))
		return s.finalizeDone(ctx, data)
	}

	if fields.ExecutionPolicy != "" && fields.ExecutionPolicy != "{}" {
		policy, pErr := ParseExecutionPolicy(fields.ExecutionPolicy)
		if pErr != nil || policy == nil || len(policy.Stages) == 0 {
			s.logger.Error("parse execution policy failed", zap.Error(pErr))
			return s.finalizeDone(ctx, data)
		}
		return s.EnterReviewStage(ctx, data.TaskID, *policy)
	}

	return s.finalizeDone(ctx, data)
}

// finalizeDone resolves blockers and notifies parents -- the original done path.
func (s *Service) finalizeDone(ctx context.Context, data *TaskMovedData) error {
	if err := s.queueBlockersResolvedWakeups(ctx, data.TaskID); err != nil {
		s.logger.Error("blocker resolution wakeups failed", zap.Error(err))
	}
	if data.ParentID != "" {
		if err := s.queueChildrenCompletedWakeup(ctx, data.ParentID); err != nil {
			s.logger.Error("children completed wakeup failed", zap.Error(err))
		}
	}
	return nil
}

// onMovedToInReview handles manual moves to In Review (e.g. kanban drag-drop).
// If an execution policy exists, the staged review flow is started.
// Without a policy, no automatic reviewer wakeups occur.
func (s *Service) onMovedToInReview(ctx context.Context, data *TaskMovedData) error {
	fields, err := s.repo.GetTaskExecutionFields(ctx, data.TaskID)
	if err != nil {
		s.logger.Error("get task execution fields for in_review", zap.Error(err))
		return nil
	}
	if fields.ExecutionPolicy == "" || fields.ExecutionPolicy == "{}" {
		return nil // no policy — just a status indicator
	}
	policy, pErr := ParseExecutionPolicy(fields.ExecutionPolicy)
	if pErr != nil || policy == nil || len(policy.Stages) == 0 {
		return nil
	}
	return s.EnterReviewStage(ctx, data.TaskID, *policy)
}

// tryRecordReviewerVerdict checks whether the mover is a participant in the
// current execution policy stage. If so, it records the verdict and returns
// (true, nil). If the mover is not a participant, returns (false, nil) so the
// caller falls through to normal move handling.
func (s *Service) tryRecordReviewerVerdict(ctx context.Context, data *TaskMovedData, toCategory stepCategory) (bool, error) {
	fields, err := s.repo.GetTaskExecutionFields(ctx, data.TaskID)
	if err != nil {
		return false, nil
	}

	policy, err := ParseExecutionPolicy(fields.ExecutionPolicy)
	if err != nil || policy == nil || len(policy.Stages) == 0 {
		return false, nil
	}
	state, err := parseExecutionState(fields.ExecutionState)
	if err != nil || state == nil {
		return false, nil
	}
	if state.CurrentStageIndex >= len(policy.Stages) {
		return false, nil
	}

	// Resolve the mover's agent instance ID from the session.
	moverAgentID := s.resolveAgentFromSession(ctx, data.SessionID)
	if moverAgentID == "" {
		// No session (manual/user move) — don't intercept.
		return false, nil
	}

	// Check if the mover is a participant in the current stage.
	stage := &policy.Stages[state.CurrentStageIndex]
	if !isStageParticipant(stage, moverAgentID) {
		return false, nil
	}

	verdict := "reject"
	if toCategory == stepCategoryDone {
		verdict = "approve"
	}

	// Use the last comment on this task by this agent as the review comment.
	comment := s.getLatestCommentByAuthor(ctx, data.TaskID, moverAgentID)

	err = s.RecordParticipantResponse(ctx, data.TaskID, moverAgentID, verdict, comment)
	return true, err
}

// resolveAgentFromSession resolves the agent instance that is currently
// working on a task by checking the task's checkout_agent_id.
func (s *Service) resolveAgentFromSession(ctx context.Context, sessionID string) string {
	if sessionID == "" {
		return ""
	}
	// Look up which task this session belongs to, then get the checkout agent.
	agentID, err := s.repo.GetCheckoutAgentBySession(ctx, sessionID)
	if err != nil {
		return ""
	}
	return agentID
}

// getLatestCommentByAuthor returns the most recent comment body by an author on a task.
func (s *Service) getLatestCommentByAuthor(ctx context.Context, taskID, authorID string) string {
	body, err := s.repo.GetLatestCommentBody(ctx, taskID, authorID)
	if err != nil {
		return ""
	}
	return body
}

// isStageParticipant checks if an agent ID is a participant in a stage.
func isStageParticipant(stage *ExecutionStage, agentID string) bool {
	for _, p := range stage.Participants {
		if p.Type == participantTypeAgent && p.AgentID == agentID {
			return true
		}
	}
	return false
}

// queueBlockersResolvedWakeups checks tasks blocked by the completed task and
// queues wakeups for those whose blockers are all resolved.
func (s *Service) queueBlockersResolvedWakeups(ctx context.Context, completedTaskID string) error {
	blockedTaskIDs, err := s.repo.ListTasksBlockedBy(ctx, completedTaskID)
	if err != nil {
		return fmt.Errorf("list blocked tasks: %w", err)
	}
	for _, blockedID := range blockedTaskIDs {
		if err := s.resolveAndWakeIfUnblocked(ctx, blockedID, completedTaskID); err != nil {
			s.logger.Error("resolve blocker wakeup failed",
				zap.String("blocked_task", blockedID), zap.Error(err))
		}
	}
	return nil
}

func (s *Service) resolveAndWakeIfUnblocked(ctx context.Context, blockedTaskID, resolvedBlockerID string) error {
	blockers, err := s.repo.ListTaskBlockers(ctx, blockedTaskID)
	if err != nil {
		return err
	}
	for _, b := range blockers {
		if b.BlockerTaskID == resolvedBlockerID {
			continue
		}
		done, err := s.repo.IsTaskInTerminalStep(ctx, b.BlockerTaskID)
		if err != nil || !done {
			return err // still blocked
		}
	}
	assignee, err := s.repo.GetTaskAssignee(ctx, blockedTaskID)
	if err != nil || assignee == "" {
		return err
	}
	payload := mustJSON(map[string]string{
		"task_id":             blockedTaskID,
		"resolved_blocker_id": resolvedBlockerID,
	})
	key := fmt.Sprintf("blockers_resolved:%s", blockedTaskID)
	return s.QueueWakeup(ctx, assignee, WakeupReasonTaskBlockersResolved, payload, key)
}

// queueChildrenCompletedWakeup checks if all children of a parent are terminal
// and, if so, queues a wakeup with child summaries.
func (s *Service) queueChildrenCompletedWakeup(ctx context.Context, parentID string) error {
	allDone, err := s.repo.AreAllChildrenTerminal(ctx, parentID)
	if err != nil || !allDone {
		return err
	}
	assignee, err := s.repo.GetTaskAssignee(ctx, parentID)
	if err != nil || assignee == "" {
		return err
	}

	children, truncated, err := s.repo.GetChildSummaries(ctx, parentID)
	if err != nil {
		s.logger.Error("get child summaries failed", zap.Error(err))
		children = nil
	}

	payload := mustJSON(map[string]interface{}{
		"task_id":   parentID,
		"children":  children,
		"truncated": truncated,
	})
	key := fmt.Sprintf("children_completed:%s", parentID)
	return s.QueueWakeup(ctx, assignee, WakeupReasonTaskChildrenCompleted, payload, key)
}

// handleCommentCreated loads the comment and relays it to external channels.
func (s *Service) handleCommentCreated(ctx context.Context, event *bus.Event) error {
	data, err := decodeEventData[CommentPostedData](event)
	if err != nil {
		return nil
	}
	comment, err := s.repo.GetTaskComment(ctx, data.CommentID)
	if err != nil {
		s.logger.Error("load comment for relay failed",
			zap.String("comment_id", data.CommentID), zap.Error(err))
		return nil
	}
	if err := s.relay.RelayComment(ctx, comment); err != nil {
		s.logger.Error("relay comment failed",
			zap.String("comment_id", data.CommentID), zap.Error(err))
	}
	return nil
}

// handleApprovalResolved fires a wakeup for the requesting agent.
func (s *Service) handleApprovalResolved(ctx context.Context, event *bus.Event) error {
	data, err := decodeEventData[ApprovalResolvedData](event)
	if err != nil || data.RequestedByAgentInstanceID == "" {
		return nil
	}
	payload := mustJSON(map[string]string{
		"approval_id":   data.ApprovalID,
		"status":        data.Status,
		"decision_note": data.DecisionNote,
	})
	key := fmt.Sprintf("approval_resolved:%s", data.ApprovalID)
	return s.QueueWakeup(ctx, data.RequestedByAgentInstanceID, WakeupReasonApprovalResolved, payload, key)
}

// Step categories for task.moved events.
type stepCategory int

const (
	stepCategoryUnknown stepCategory = iota
	stepCategoryInProgress
	stepCategoryDone
	stepCategoryInReview
)

// categorizeStep maps step names to categories.
func categorizeStep(name string) stepCategory {
	switch name {
	case "In Progress", "in_progress":
		return stepCategoryInProgress
	case "Done", "done", "Cancelled", "cancelled":
		return stepCategoryDone
	case "In Review", "in_review":
		return stepCategoryInReview
	default:
		return stepCategoryUnknown
	}
}

// decodeEventData extracts typed data from an event.
func decodeEventData[T any](event *bus.Event) (*T, error) {
	b, err := json.Marshal(event.Data)
	if err != nil {
		return nil, err
	}
	var data T
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

// mustJSON marshals v to JSON string, returning "{}" on error.
func mustJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
