package orchestrator

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

func (s *Service) focusTaskSession(
	ctx context.Context,
	taskID, sessionID string,
) (*executor.TaskExecution, *models.TaskSession, bool, error) {
	var result focusTaskSessionResult
	err := s.withSessionPromptAdmission(ctx, sessionID, func(admittedCtx context.Context) error {
		var err error
		result, err = s.focusTaskSessionAdmitted(admittedCtx, taskID, sessionID)
		return err
	})
	if err != nil {
		return nil, result.session, false, err
	}
	return result.execution, result.session, result.resumed, nil
}

type focusTaskSessionResult struct {
	execution *executor.TaskExecution
	session   *models.TaskSession
	resumed   bool
}

func (s *Service) focusTaskSessionAdmitted(
	ctx context.Context,
	taskID, sessionID string,
) (focusTaskSessionResult, error) {
	current, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return focusTaskSessionResult{}, err
	}
	result := focusTaskSessionResult{session: current}
	if !eligibleFocusedSession(current, taskID) {
		return result, nil
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil || task.ArchivedAt != nil {
		return result, nil
	}
	if allowed, _ := s.autoResumeEligibility(ctx, task, current); !allowed {
		return result, nil
	}
	isOfficeTask, err := s.lookupOfficeTask(ctx, taskID)
	if err != nil || isOfficeTask {
		return result, nil
	}
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil || running == nil {
		return result, nil
	}
	if running.IdleSuspensionState == models.ExecutorIdleSuspensionSuspended {
		return s.resumeFocusedIdleSuspension(ctx, taskID, sessionID, current)
	}
	if running.IdleSuspensionState != models.ExecutorIdleSuspensionNone {
		return result, nil
	}
	s.refreshFocusedLiveSession(ctx, taskID, sessionID)
	return result, nil
}

func eligibleFocusedSession(session *models.TaskSession, taskID string) bool {
	if session == nil || session.TaskID != taskID || !idleTaskSessionState(session.State) {
		return false
	}
	_, workflowParked := models.LoadWorkflowParking(session.Metadata)
	return !workflowParked
}

func (s *Service) resumeFocusedIdleSuspension(
	ctx context.Context,
	taskID, sessionID string,
	session *models.TaskSession,
) (focusTaskSessionResult, error) {
	execution, err := s.ResumeTaskSessionWithOptions(ctx, taskID, sessionID, executor.ResumeOptions{
		AllowCompletedSessionResume:     session.State == models.TaskSessionStateCompleted,
		RequireIdleSuspensionProvenance: true,
		Origin:                          string(launchOriginManual),
	})
	if errors.Is(err, ErrIdleSuspensionProvenanceRequired) {
		return focusTaskSessionResult{session: session}, nil
	}
	if err != nil {
		return focusTaskSessionResult{session: session}, err
	}
	s.resetIdleParkingCandidates(sessionID)
	return focusTaskSessionResult{execution: execution, session: session, resumed: execution != nil}, nil
}

func (s *Service) refreshFocusedLiveSession(ctx context.Context, taskID, sessionID string) {
	releaseLifecycleLock := s.acquireSessionLifecycleLock(sessionID)
	defer releaseLifecycleLock()
	latest, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || !eligibleFocusedSession(latest, taskID) {
		return
	}
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil || running == nil || running.IdleSuspensionState != models.ExecutorIdleSuspensionNone {
		return
	}
	live, ok := s.executor.GetExecutionBySession(sessionID)
	if !ok || live == nil || !s.agentManager.IsAgentReadyForPrompt(ctx, sessionID) {
		return
	}
	s.resetIdleParkingCandidates(sessionID)
}
