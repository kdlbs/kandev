package orchestrator

import (
	"context"
	"errors"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/repository"
)

type alreadyResumedDeliveryReconciler interface {
	ReconcileAlreadyResumedDelivery(context.Context, agentruntime.AgentDeliveryRecoveryIdentity, string) (bool, error)
}

func (s *Service) finishAlreadyResumedRestartRecovery(ctx context.Context, inputs *silentRestoreInputs) (bool, error) {
	if inputs.recovery.HistoryGap != nil {
		return false, nil
	}
	manager, ok := s.agentManager.(alreadyResumedDeliveryReconciler)
	if !ok {
		return false, nil
	}
	releaseRoute := s.acquireSessionRouteOperationLock(inputs.session.ID)
	defer releaseRoute()
	releaseLifecycle := s.acquireSessionLifecycleLock(inputs.session.ID)
	defer releaseLifecycle()
	ownerCtx, err := s.revalidateSilentRestoreInputs(ctx, inputs, inputs.checkpoint)
	if err != nil {
		return false, err
	}
	if err := s.pauseSilentRestoreQueue(ownerCtx, inputs.task.ID, inputs.session.ID); err != nil {
		return false, err
	}
	if err := s.validateSoleRestartRecoveryBlock(ownerCtx, inputs); err != nil {
		return false, err
	}
	recovery := inputs.recovery
	identity := agentruntime.AgentDeliveryRecoveryIdentity{TaskID: inputs.task.ID, SessionID: recovery.SessionID,
		ExecutionID: recovery.AgentExecutionID, OriginalRuntime: recovery.OriginalRuntime,
		IncarnationID: recovery.IncarnationID, HarnessGeneration: uint64(recovery.HarnessGeneration),
		SubmissionID: recovery.SubmissionID, StreamID: recovery.StreamID, PromptGeneration: recovery.PromptGeneration}
	restored, err := manager.ReconcileAlreadyResumedDelivery(ownerCtx, identity, inputs.generation.NativeSessionID)
	if err != nil || !restored {
		return false, err
	}
	if _, err := s.revalidateSilentRestoreInputs(ownerCtx, inputs, inputs.checkpoint); err != nil {
		return false, err
	}
	blocks, ok := s.repo.(sessionRecoveryBlockStore)
	if !ok {
		return false, errors.New("restart recovery block store is unavailable")
	}
	changed, handled, err := s.resolveNativeDeliveryBlock(ownerCtx, inputs.block, time.Now().UTC())
	if !handled {
		changed, err = blocks.ResolveSessionRecoveryBlock(ownerCtx, inputs.block.ID, "resume", time.Now().UTC())
	}
	if err == nil && !changed {
		err = errors.New("restart recovery block changed before resolution")
	}
	if err == nil {
		if session, readErr := s.repo.GetTaskSession(ownerCtx, inputs.session.ID); readErr == nil && session != nil {
			s.publishTaskSessionStateChanged(ownerCtx, session.TaskID, session.ID, session.State, session.State, session.ErrorMessage, &session.UpdatedAt, session)
		}
	}
	return err == nil, err
}

func (s *Service) validateSoleRestartRecoveryBlock(ctx context.Context, inputs *silentRestoreInputs) error {
	list, ok := s.repo.(repository.SessionRecoveryBlockListRepository)
	if !ok {
		return errors.New("restart recovery block inventory is unavailable")
	}
	open, err := list.ListOpenSessionRecoveryBlocks(ctx, inputs.session.ID, inputs.recovery.IncarnationID, inputs.recovery.HarnessGeneration)
	if err != nil {
		return err
	}
	if len(open) != 1 || open[0].ID != inputs.block.ID {
		return errors.New("independent restart recovery block remains open")
	}
	return nil
}
