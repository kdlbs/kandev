package executor

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
)

// DispatchGuard revalidates server-owned context immediately before native
// launch/resume/prompt/steer. Nil preserves ordinary tasks' existing behavior.
// It is configured once during startup, before dispatch begins.
type DispatchGuard func(context.Context, *models.Task, *models.TaskSession, string) error

func (e *Executor) SetDispatchGuard(guard DispatchGuard) { e.dispatchGuard = guard }

func (e *Executor) CheckDispatch(ctx context.Context, taskID, sessionID, profileID string) error {
	if e.dispatchGuard == nil {
		return nil
	}
	task, err := e.repo.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	session, err := e.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if task == nil || session == nil || session.TaskID != taskID {
		return ErrExecutionNotFound
	}
	if profileID == "" {
		profileID = session.AgentProfileID
	}
	return e.dispatchGuard(ctx, task, session, profileID)
}

func (e *Executor) guardedLaunch(ctx context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
	if err := e.CheckDispatch(ctx, req.TaskID, req.SessionID, req.AgentProfileID); err != nil {
		return nil, err
	}
	return e.agentManager.LaunchAgent(ctx, req)
}

func (e *Executor) guardedProcessStart(ctx context.Context, taskID, sessionID, executionID string) error {
	if err := e.CheckDispatch(ctx, taskID, sessionID, ""); err != nil {
		return err
	}
	return e.agentManager.StartAgentProcess(ctx, executionID)
}
