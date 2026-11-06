package orchestrator

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
)

type failedRestoreCleaner interface {
	CleanupFailedRestoreIfCurrent(context.Context, string, string, time.Time, func(context.Context) error) error
}

type silentRestoreStartupContextKey struct{}

func (s *Service) settleFailedSilentRestoreCandidate(
	ctx context.Context, store taskrepo.SilentRestoreRepository, attemptID string,
	expected models.SilentRestoreCheckpoint, inputs *silentRestoreInputs,
) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	releaseRoute := s.acquireSessionRouteOperationLock(inputs.session.ID)
	defer releaseRoute()
	releaseLifecycle := s.acquireSessionLifecycleLock(inputs.session.ID)
	defer releaseLifecycle()
	attempt, err := store.GetRestoreAttempt(ctx, attemptID)
	if err != nil {
		return err
	}
	if attempt == nil || attempt.Checkpoint == nil ||
		!sameSilentRestorePinnedSource(*attempt.Checkpoint, expected) ||
		attempt.Checkpoint.CandidateExecutionID != expected.CandidateExecutionID {
		return errors.New("silent restore candidate changed before failed startup cleanup")
	}
	checkpoint := *attempt.Checkpoint
	if checkpoint.Stage != models.SilentRestoreStageCandidateLaunching {
		return nil
	}
	ctx, err = s.revalidateSilentRestoreInputs(ctx, inputs, checkpoint)
	if err != nil {
		return err
	}
	running, err := s.readSilentRestoreCandidateExecution(ctx, inputs.session.ID, checkpoint.CandidateExecutionID)
	if err != nil {
		return err
	}
	if running == nil {
		return errors.New("failed silent restore has no durable execution evidence")
	}
	if s.agentManager.IsAgentReadyForPrompt(ctx, running.SessionID) &&
		s.currentACPSessionID(running.SessionID) == checkpoint.SourceGeneration.NativeSessionID {
		return nil
	}
	if prober, ok := s.agentManager.(rowLivenessProber); ok && prober.RowLiveness(running) == models.ProcessLivenessDead {
		_, err = s.markSilentRestoreCandidateDead(ctx, store, attemptID, checkpoint, running, inputs)
		return err
	}
	cleaner, ok := s.agentManager.(failedRestoreCleaner)
	if !ok {
		return errors.New("failed silent restore cleanup capability is unavailable")
	}
	return cleaner.CleanupFailedRestoreIfCurrent(
		ctx, running.SessionID, running.AgentExecutionID, running.UpdatedAt,
		func(stoppedCtx context.Context) error {
			_, err := s.recordSilentRestoreCandidateDead(stoppedCtx, store, attemptID, checkpoint, inputs)
			return err
		},
	)
}
