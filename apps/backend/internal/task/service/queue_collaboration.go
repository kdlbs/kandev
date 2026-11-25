package service

import (
	"context"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// AuthorizeQueueSessionScope applies the requested native scope and conversation
// grant. Identity/reach for disabled callers stays in the existing access path.
func (s *Service) AuthorizeQueueSessionScope(ctx context.Context, sessionID string, scope authz.Scope) error {
	if !s.collaborationApplies(ctx) {
		return nil
	}
	if scope == authz.ScopeSessionPrompt {
		return s.AuthorizeSessionPromptAccess(ctx, sessionID)
	}
	return s.AuthorizeSessionScope(ctx, sessionID, scope)
}

// AuthorizeQueueSessionIdentity retains task/session/incarnation ownership. The
// queue handler separately checks its action before touching queued state.
func (s *Service) AuthorizeQueueSessionIdentity(ctx context.Context, taskID, sessionID, incarnationID string) error {
	if !s.collaborationApplies(ctx) {
		if taskID == "" {
			return s.AuthorizeSessionAccess(ctx, sessionID)
		}
		if incarnationID == "" {
			return s.AuthorizeTaskSessionAccess(ctx, taskID, sessionID)
		}
		return s.AuthorizeTaskSessionIncarnationAccess(ctx, taskID, sessionID, incarnationID)
	}
	session, err := s.sessions.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil || (taskID != "" && session.TaskID != taskID) {
		return repoerrors.ErrTaskNotFound
	}
	if incarnationID != "" && (taskID == "" || session.QueueIncarnationID != incarnationID) {
		return repoerrors.ErrTaskNotFound
	}
	return nil
}
