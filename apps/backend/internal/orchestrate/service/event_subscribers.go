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
	subs := []struct {
		subject string
		handler bus.EventHandler
	}{
		{events.TaskUpdated, s.handleTaskUpdated},
		{events.TaskMoved, s.handleTaskMoved},
		{events.OrchestrateApprovalResolved, s.handleApprovalResolved},
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

	switch categorizeStep(data.ToStepName) {
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

func (s *Service) onMovedToInReview(ctx context.Context, data *TaskMovedData) error {
	reviewers := extractReviewers(data.ExecutionPolicy)
	for _, reviewerID := range reviewers {
		payload := mustJSON(map[string]string{"task_id": data.TaskID})
		key := fmt.Sprintf("review_request:%s:%s", data.TaskID, reviewerID)
		if err := s.QueueWakeup(ctx, reviewerID, WakeupReasonTaskAssigned, payload, key); err != nil {
			s.logger.Error("review wakeup failed",
				zap.String("reviewer", reviewerID), zap.Error(err))
		}
	}
	return nil
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

// queueChildrenCompletedWakeup checks if all children of a parent are terminal.
func (s *Service) queueChildrenCompletedWakeup(ctx context.Context, parentID string) error {
	allDone, err := s.repo.AreAllChildrenTerminal(ctx, parentID)
	if err != nil || !allDone {
		return err
	}
	assignee, err := s.repo.GetTaskAssignee(ctx, parentID)
	if err != nil || assignee == "" {
		return err
	}
	payload := mustJSON(map[string]string{"task_id": parentID})
	key := fmt.Sprintf("children_completed:%s", parentID)
	return s.QueueWakeup(ctx, assignee, WakeupReasonTaskChildrenCompleted, payload, key)
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

// extractReviewers parses reviewer agent IDs from execution policy JSON.
func extractReviewers(policyJSON string) []string {
	if policyJSON == "" || policyJSON == "{}" {
		return nil
	}
	var policy struct {
		Reviewers []string `json:"reviewers"`
	}
	if err := json.Unmarshal([]byte(policyJSON), &policy); err != nil {
		return nil
	}
	return policy.Reviewers
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
