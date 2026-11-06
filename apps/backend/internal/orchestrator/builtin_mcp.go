package orchestrator

import (
	"context"

	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

// PrepareTaskSessionMCPPrompt captures the binding before message persistence.
// Callers carry the returned context through dispatch to the same session.
func (s *Service) PrepareTaskSessionMCPPrompt(ctx context.Context, taskID, sessionID string) (context.Context, error) {
	if err := s.authorizeTaskSessionPair(ctx, taskID, sessionID); err != nil {
		return nil, err
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return s.prepareBuiltinPromptContext(ctx, taskID, session)
}

func preparedBuiltinPackages(ctx context.Context, taskID, sessionID string) *p.BuiltinManifest {
	profile := executor.PreparedMCPProfile(ctx, taskID, sessionID)
	if profile == nil {
		return nil
	}
	return profile.BuiltinPackages
}

func (s *Service) prepareBuiltinPromptContext(ctx context.Context, taskID string, session *models.TaskSession) (context.Context, error) {
	if session.State == models.TaskSessionStateCreated {
		return s.executor.PrepareMCPLaunch(ctx, taskID, session, true)
	}
	return s.executor.PrepareMCPContinuation(ctx, taskID, session)
}
