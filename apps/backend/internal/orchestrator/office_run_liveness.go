package orchestrator

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
)

// OfficeTaskRunExecutionLive reports whether the task session behind a
// claimed Office run still owns a live execution. A STARTING session is
// live because its launch is in progress; a RUNNING session is live while
// its agent runtime answers. sessionID is the run's recorded session; when
// it is empty the (task, agent) session is used. A session that belongs to
// another task or agent is never live for this run.
func (s *Service) OfficeTaskRunExecutionLive(ctx context.Context, taskID, agentProfileID, sessionID string) (bool, error) {
	if taskID == "" || agentProfileID == "" {
		return false, nil
	}
	session, err := s.officeRunSession(ctx, taskID, agentProfileID, sessionID)
	if err != nil || session == nil {
		return false, err
	}
	if session.TaskID != taskID || session.AgentProfileID != agentProfileID {
		return false, nil
	}
	switch session.State {
	case models.TaskSessionStateStarting:
		return true, nil
	case models.TaskSessionStateRunning:
		return s.probeAgentRunning(ctx, session.ID)
	default:
		return false, nil
	}
}

func (s *Service) officeRunSession(ctx context.Context, taskID, agentProfileID, sessionID string) (*models.TaskSession, error) {
	if sessionID == "" {
		return s.repo.GetTaskSessionByTaskAndAgent(ctx, taskID, agentProfileID)
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if errors.Is(err, models.ErrTaskSessionNotFound) {
		return nil, nil
	}
	return session, err
}
