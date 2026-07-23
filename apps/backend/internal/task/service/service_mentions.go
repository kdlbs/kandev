package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var errReferenceConversationScope = errors.New("reference conversation scope is invalid")

// ResolveWorkspace derives trusted reference scope from the persisted session
// and task. assertedTaskID is optional for queue updates that identify only a
// session; when present it must match the session-owned task exactly.
func (s *Service) ResolveWorkspace(
	ctx context.Context,
	sessionID, assertedTaskID string,
) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	assertedTaskID = strings.TrimSpace(assertedTaskID)
	if sessionID == "" || s == nil || s.sessions == nil || s.tasks == nil {
		return "", errReferenceConversationScope
	}
	session, err := s.sessions.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.IsPassthrough || strings.TrimSpace(session.TaskID) == "" {
		return "", fmt.Errorf("%w: session lookup", errReferenceConversationScope)
	}
	if assertedTaskID != "" && assertedTaskID != session.TaskID {
		return "", fmt.Errorf("%w: task mismatch", errReferenceConversationScope)
	}
	task, err := s.tasks.GetTask(ctx, session.TaskID)
	if err != nil || task == nil || strings.TrimSpace(task.WorkspaceID) == "" {
		return "", fmt.Errorf("%w: task lookup", errReferenceConversationScope)
	}
	return task.WorkspaceID, nil
}
