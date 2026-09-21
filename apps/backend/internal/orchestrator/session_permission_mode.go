package orchestrator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

const sessionPermissionAcceptEdits = "acceptEdits"

func (s *Service) SetTaskSessionPermissionMode(ctx context.Context, taskID, sessionID, mode string) error {
	if mode != "default" && mode != sessionPermissionAcceptEdits && mode != "auto" {
		return fmt.Errorf("select default, acceptEdits or auto; permission bypass is unavailable")
	}
	if err := s.authorizeTaskSessionPair(ctx, taskID, sessionID); err != nil {
		return err
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.TaskID != taskID {
		return ErrPermissionTaskOrSessionNotFound
	}
	if s.agentManager == nil || s.agentManager.IsPassthroughSession(ctx, sessionID) {
		return fmt.Errorf("native session controls unavailable")
	}
	if s.agentManager.IsAgentRunningForSession(ctx, sessionID) {
		if err := s.agentManager.SetSessionModeBySessionID(ctx, sessionID, mode); err != nil {
			return err
		}
	}
	s.persistSessionMode(ctx, sessionID, mode)
	updated, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if updated.Metadata[models.SessionMetaKeySessionMode] != mode {
		return fmt.Errorf("session permission mode was not persisted")
	}
	return nil
}
