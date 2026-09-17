package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/kandev/kandev/internal/workflow/engine"
)

const (
	conversationErrorMessageKey = "error_message"
	conversationReasonKey       = "reason"
	conversationRunIDKey        = "run_id"
	conversationSessionIDKey    = "session_id"
)

const (
	conversationAgentIDKey = "agent_id"
	conversationTaskIDKey  = "task_id"
)

// Native conversations have no delivery workflow step: their lifecycle remains
// open across turns. Route their events into the same durable Office run queue.
func (s *Service) QueueNativeConversation(ctx context.Context, taskID string, trigger engine.Trigger, payload any, opID string) (bool, error) {
	native, err := s.repo.IsNativeConversation(ctx, taskID)
	if err != nil || !native {
		return false, err
	}
	fields, err := s.repo.GetTaskExecutionFields(ctx, taskID)
	if err != nil {
		return true, err
	}
	data := map[string]any{conversationTaskIDKey: taskID}
	var reason string
	switch trigger {
	case engine.TriggerOnComment:
		comment, ok := payload.(engine.OnCommentPayload)
		if !ok {
			return true, nil
		}
		reason = RunReasonTaskComment
		data["comment_id"] = comment.CommentID
	case engine.TriggerOnChildrenCompleted:
		reason = RunReasonTaskChildrenCompleted
		if err := addNativeChildSummaries(data, payload); err != nil {
			return true, err
		}
	case engine.TriggerOnBlockerResolved:
		reason = RunReasonTaskBlockersResolved
		if blockers, ok := payload.(engine.OnBlockerResolvedPayload); ok {
			data["resolved_blocker_ids"] = blockers.ResolvedBlockerIDs
		}
	default:
		return true, nil
	}
	body, err := json.Marshal(data)
	if err != nil {
		return true, err
	}
	if trigger != engine.TriggerOnComment {
		// A standing conversation can coordinate many successive batches. Retain
		// duplicate suppression without suppressing every batch after the first.
		identity, err := json.Marshal(payload)
		if err != nil {
			return true, err
		}
		opID = fmt.Sprintf("%s:%x", opID, sha256.Sum256(identity))
	}
	return true, s.QueueRun(ctx, fields.AssigneeAgentProfileID, reason, string(body), opID)
}

func addNativeChildSummaries(data map[string]any, payload any) error {
	children, ok := payload.(engine.OnChildrenCompletedPayload)
	if !ok {
		return fmt.Errorf("invalid children-completed payload")
	}
	rows := make([]map[string]string, 0, min(len(children.ChildSummaries), 10))
	for i, child := range children.ChildSummaries {
		if i == 10 {
			data["truncated"] = true
			break
		}
		rows = append(rows, map[string]string{
			"identifier": child.TaskID, "state": child.Status,
			"last_comment": clipConversationText(child.Summary, 1000),
		})
	}
	data["children"] = rows
	return nil
}
