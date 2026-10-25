package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

// claimPreparedDependencyPrimary preserves the workspace-only conversation
// under its lifecycle lock. Workflow recipient selection precedes this claim.
func (s *Service) claimPreparedDependencyPrimary(ctx context.Context, taskID, profileID, executorID, executorProfileID string) (string, func(), error) {
	noop := func() {}
	sessions, err := s.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return "", noop, fmt.Errorf("list prepared dependency sessions: %w", err)
	}
	primary := preparedDependencyPrimary(taskID, sessions)
	if primary == nil {
		return "", noop, nil
	}
	release := s.acquireSessionLifecycleLock(primary.ID)
	keep := false
	defer func() {
		if !keep {
			release()
		}
	}()
	primary, err = s.repo.GetTaskSession(ctx, primary.ID)
	if err != nil {
		return "", noop, err
	}
	if primary == nil || primary.TaskID != taskID || !primary.IsPrimary {
		return "", noop, nil
	}
	sessions, err = s.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return "", noop, err
	}
	if s.dependencyLaunchHasActiveSession(ctx, sessions, primary.ID) {
		return "", noop, executor.ErrExecutionAlreadyRunning
	}
	if !s.isPreparedDependencyPrimary(primary, profileID, executorID, executorProfileID) {
		return "", noop, nil
	}
	empty, err := s.preparedDependencyConversationEmpty(ctx, primary)
	if err != nil || !empty {
		return "", noop, err
	}
	keep = true
	return primary.ID, release, nil
}

func (s *Service) preparedDependencyConversationEmpty(ctx context.Context, session *models.TaskSession) (bool, error) {
	messages, err := s.repo.ListMessages(ctx, session.ID)
	if err != nil || len(messages) != 0 {
		return false, err
	}
	turns, ok := s.repo.(interface {
		ListTurnsBySession(context.Context, string) ([]*models.Turn, error)
	})
	if !ok {
		return false, nil
	}
	history, err := turns.ListTurnsBySession(ctx, session.ID)
	if err != nil || len(history) != 0 {
		return false, err
	}
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, session.ID)
	if err != nil && !errors.Is(err, models.ErrExecutorRunningNotFound) && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if running != nil && running.ResumeToken != "" {
		return false, nil
	}
	_, generation, err := s.recoverySessionIdentity(ctx, session.ID)
	return generation == 0, err
}

func preparedDependencyPrimary(taskID string, sessions []*models.TaskSession) *models.TaskSession {
	var primary *models.TaskSession
	for _, session := range sessions {
		if session != nil && session.IsPrimary && session.TaskID == taskID {
			if primary != nil {
				return nil
			}
			primary = session
		}
	}
	return primary
}

func (s *Service) isPreparedDependencyPrimary(primary *models.TaskSession, profileID, executorID, executorProfileID string) bool {
	if primary.State != models.TaskSessionStateCreated || primary.CompletedAt != nil || primary.ErrorMessage != "" ||
		primary.AgentProfileID != profileID || (executorID != "" && primary.ExecutorID != executorID) || primary.ExecutorProfileID != executorProfileID ||
		primary.DownstreamACPSessionID != "" || s.currentACPSessionID(primary.ID) != "" {
		return false
	}
	if _, submitted := primary.Metadata[models.SessionMetaKeyInitialPromptSubmission]; submitted {
		return false
	}
	acp, ok := primary.Metadata["acp"].(map[string]interface{})
	return !ok || acp["session_id"] == nil || acp["session_id"] == ""
}

func (s *Service) dependencyLaunchHasActiveSession(ctx context.Context, sessions []*models.TaskSession, primaryID string) bool {
	for _, session := range sessions {
		if session.State == models.TaskSessionStateRunning || session.State == models.TaskSessionStateStarting {
			return true
		}
		// The CREATED primary can own a booting workspace. The executor serializes
		// that preparation with its first agent launch under the session lock.
		if session.ID == primaryID && session.State == models.TaskSessionStateCreated {
			continue
		}
		if s.agentManager.IsAgentRunningForSession(ctx, session.ID) {
			return true
		}
	}
	return false
}
