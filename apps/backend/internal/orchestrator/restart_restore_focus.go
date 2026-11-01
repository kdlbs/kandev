package orchestrator

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
)

// prioritizeRestartRestore joins the existing promptless restart work before
// ordinary resume admission encounters its unresolved delivery block.
func (s *Service) prioritizeRestartRestore(ctx context.Context, req *LaunchSessionRequest, intent SessionIntent) (*LaunchSessionResponse, error) {
	if !isRestartRestoreInspection(req, intent) {
		return nil, nil
	}
	ownerCtx := s.recoveryLifecycleContext()
	store, ok := s.repo.(taskrepo.SilentRestoreRepository)
	if !ok || ownerCtx == nil || s.silentRestoreOwnerContext == nil {
		return nil, nil
	}
	candidate, err := s.focusedRestartCandidate(ctx, req)
	if err != nil || candidate == nil {
		return nil, err
	}

	queue := s.restartRestoreScheduler(ownerCtx)
	queue.prioritize(candidate.SessionID)
	queue.submit(*candidate, func(opCtx context.Context, candidate models.SilentRestoreCandidate) error {
		return s.restoreSilentRestartCandidate(opCtx, store, candidate)
	})
	return s.sessionOpenRecoveryWaitingResponse(ctx, req, "restart_restore_pending"), nil
}

func isRestartRestoreInspection(req *LaunchSessionRequest, intent SessionIntent) bool {
	return intent == IntentResume && req.RecoveryAction == "" && req.InitialPromptSubmission == nil &&
		(req.ActivationSource == LaunchActivationSourceSessionFocus || req.ActivationSource == LaunchActivationSourceSessionOpen)
}

func (s *Service) focusedRestartCandidate(ctx context.Context, req *LaunchSessionRequest) (*models.SilentRestoreCandidate, error) {
	session, err := s.repo.GetTaskSession(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}
	if session == nil || session.TaskID != req.TaskID || !silentRestoreSessionRouteEligible(session) {
		return nil, nil
	}
	block, err := s.GetOpenSessionRecoveryBlock(ctx, session.ID)
	if err != nil {
		return nil, err
	}
	if block == nil || block.ConsumerReference != agentDeliveryConsumer {
		return nil, nil
	}
	generation, err := s.restartRecoveryNativeGeneration(ctx, session)
	if err != nil {
		return nil, err
	}
	if generation == nil || generation.NativeSessionID == "" {
		return nil, nil
	}
	task, err := s.repo.GetTask(ctx, req.TaskID)
	if err != nil {
		return nil, err
	}
	if task == nil || task.ArchivedAt != nil || task.WorkspaceID == "" {
		return nil, nil
	}
	return &models.SilentRestoreCandidate{SessionID: session.ID, TaskID: task.ID, WorkspaceID: task.WorkspaceID}, nil
}
