package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

var ErrInvalidTransitionCursor = errors.New("invalid transition cursor")

type stepTransitionReader interface {
	ListTaskStepTransitions(context.Context, string, int64, int) ([]models.StepTransition, error)
	ListWorkflowTransitionGroups(context.Context, string, string, string, int) ([]models.TransitionGroup, error)
}

type transitionCursor struct {
	Scope string `json:"scope"`
	ID    int64  `json:"id,omitempty"`
	Key   string `json:"key,omitempty"`
}

func decodeTransitionCursor(encoded, scope string) (transitionCursor, error) {
	if encoded == "" {
		return transitionCursor{Scope: scope}, nil
	}
	if len(encoded) > 1024 {
		return transitionCursor{}, ErrInvalidTransitionCursor
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return transitionCursor{}, ErrInvalidTransitionCursor
	}
	var cursor transitionCursor
	if json.Unmarshal(data, &cursor) != nil || cursor.Scope != scope {
		return transitionCursor{}, ErrInvalidTransitionCursor
	}
	return cursor, nil
}

func encodeTransitionCursor(cursor transitionCursor) string {
	data, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(data)
}

func transitionReadLimit(limit int) (int, error) {
	if limit < 0 {
		return 0, fmt.Errorf("invalid transition limit")
	}
	if limit == 0 {
		return 50, nil
	}
	if limit > 200 {
		return 200, nil
	}
	return limit, nil
}

// ListTaskStepTransitions returns one bounded page of retained task movement.
func (s *Service) ListTaskStepTransitions(ctx context.Context, taskID string, limit int, encodedCursor string) ([]models.StepTransition, string, error) {
	if err := s.authorizeTaskID(ctx, taskID); err != nil {
		return nil, "", err
	}
	reader, ok := s.tasks.(stepTransitionReader)
	if !ok {
		return nil, "", fmt.Errorf("task transition reads unavailable")
	}
	pageSize, err := transitionReadLimit(limit)
	if err != nil {
		return nil, "", err
	}
	cursor, err := decodeTransitionCursor(encodedCursor, "task:"+taskID)
	if err != nil || (encodedCursor != "" && (cursor.ID <= 0 || cursor.Key != "")) {
		return nil, "", ErrInvalidTransitionCursor
	}
	items, err := reader.ListTaskStepTransitions(ctx, taskID, cursor.ID, pageSize+1)
	if err != nil {
		return nil, "", err
	}
	if len(items) <= pageSize {
		return items, "", nil
	}
	items = items[:pageSize]
	return items, encodeTransitionCursor(transitionCursor{Scope: "task:" + taskID, ID: items[len(items)-1].ID}), nil
}

// ListWorkflowTransitionGroups returns bounded route counts for a workflow.
func (s *Service) ListWorkflowTransitionGroups(ctx context.Context, workflowID string, limit int, encodedCursor string) ([]models.TransitionGroup, string, error) {
	if err := s.authorizeWorkflowID(ctx, workflowID); err != nil {
		return nil, "", err
	}
	workflow, err := s.workflows.GetWorkflow(ctx, workflowID)
	if err != nil {
		return nil, "", err
	}
	if workflow == nil {
		return nil, "", fmt.Errorf("workflow not found")
	}
	reader, ok := s.tasks.(stepTransitionReader)
	if !ok {
		return nil, "", fmt.Errorf("workflow transition reads unavailable")
	}
	pageSize, err := transitionReadLimit(limit)
	if err != nil {
		return nil, "", err
	}
	cursor, err := decodeTransitionCursor(encodedCursor, "workflow:"+workflowID)
	if err != nil || (encodedCursor != "" && (cursor.Key == "" || cursor.ID != 0)) {
		return nil, "", ErrInvalidTransitionCursor
	}
	items, err := reader.ListWorkflowTransitionGroups(ctx, workflow.WorkspaceID, workflowID, cursor.Key, pageSize+1)
	if err != nil {
		return nil, "", err
	}
	if len(items) <= pageSize {
		return items, "", nil
	}
	items = items[:pageSize]
	last := items[len(items)-1]
	key := strings.Join([]string{last.Kind, stepValue(last.FromStepID), stepValue(last.ToStepID)}, "|")
	return items, encodeTransitionCursor(transitionCursor{Scope: "workflow:" + workflowID, Key: key}), nil
}

func stepValue(step *string) string {
	if step == nil {
		return ""
	}
	return *step
}
