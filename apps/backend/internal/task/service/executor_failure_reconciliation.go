package service

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
	"sync"
	"time"
)

type executorFailureRepository interface {
	ObserveExecutorFailure(context.Context, models.ExecutorObservationTarget, *models.ExecutorObservation) (*models.ExecutorFailureEpisode, bool, error)
	GetExecutorFailure(context.Context, string) (*models.ExecutorFailureEpisode, error)
	ListExecutorObservationTargets(context.Context, string, int) ([]models.ExecutorObservationTarget, error)
}

// SetExecutorInspector wires read-only resource inspection before loops start.
func (s *Service) SetExecutorInspector(inspect func(context.Context, models.ExecutorObservationTarget) (*models.ExecutorObservation, error)) {
	s.executorInspector = inspect
}

// ReconcileExecutorFailures inspects retained environments even without agents.
func (s *Service) ReconcileExecutorFailures(ctx context.Context) {
	store, ok := s.tasks.(executorFailureRepository)
	if !ok || s.executorInspector == nil || !s.executorObservationMu.TryLock() {
		return
	}
	defer s.executorObservationMu.Unlock()
	passCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	targets, err := store.ListExecutorObservationTargets(passCtx, s.executorObservationCursor, 16)
	if err != nil {
		s.logger.Warn("executor observation inventory unavailable", zap.Error(err))
		return
	}
	type result struct{ observation *models.ExecutorObservation }
	results := make([]result, len(targets))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			for index := range jobs {
				results[index].observation, _ = s.inspectExecutorObservation(passCtx, targets[index])
			}
		})
	}
	for index := range targets {
		jobs <- index
	}
	close(jobs)
	workers.Wait()
	for index, target := range targets {
		if passCtx.Err() != nil {
			return
		}
		observation := results[index].observation
		_, changed, err := store.ObserveExecutorFailure(passCtx, target, observation)
		if err != nil {
			s.logger.Debug("executor observation admission rejected", zap.String("task_id", target.TaskID), zap.Error(err))
		} else {
			if changed {
				s.PublishTaskUpdatedByID(passCtx, target.TaskID)
				s.publishExecutorFailureHistory(passCtx, target.TaskID)
			}
			s.settleExecutorFailureSessions(passCtx, target, observation)
		}
		s.executorObservationCursor = target.Cursor
	}
	if len(targets) < 16 {
		s.executorObservationCursor = ""
	}
}

// RecheckExecutorFailure validates the visible episode before any remote read.
func (s *Service) RecheckExecutorFailure(ctx context.Context, taskID, episodeID string, revision int64) (*models.ExecutorFailureEpisode, error) {
	if err := s.AuthorizeTaskAccess(ctx, taskID); err != nil {
		return nil, err
	}
	store, ok := s.tasks.(executorFailureRepository)
	if !ok || s.executorInspector == nil {
		return nil, fmt.Errorf("executor inspection unavailable")
	}
	episode, err := store.GetExecutorFailure(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if episode == nil || episode.ID != episodeID || episode.Revision != revision {
		return nil, fmt.Errorf("executor failure changed; reload task status")
	}
	after := ""
	for {
		targets, err := store.ListExecutorObservationTargets(ctx, after, 100)
		if err != nil {
			return nil, err
		}
		for _, target := range targets {
			if !executorFailureTargetMatches(target, episode) {
				continue
			}
			return s.recheckRecordedExecutorFailure(ctx, store, target, episode)

		}
		if len(targets) < 100 {
			break
		}
		after = targets[len(targets)-1].Cursor
	}
	return nil, fmt.Errorf("recorded executor ownership changed; reload task status")
}

func (s *Service) SetExecutorLossRetirer(retire func(context.Context, models.ExecutorObservationTarget, *models.ExecutorObservation) (bool, error)) {
	s.executorLossRetirer = retire
}

type executorFailureSettlementRepository interface {
	ListExecutorFailureAffectedSessions(context.Context, string) ([]models.ExecutorFailureAffectedSession, error)
	CompleteExecutorFailureSettlement(context.Context, models.ExecutorFailureAffectedSession) error
}

func (s *Service) settleExecutorFailureSessions(ctx context.Context, target models.ExecutorObservationTarget, observation *models.ExecutorObservation) {
	if s.executorLossRetirer == nil || observation == nil || (observation.Outcome != models.ExecutorOutcomeTerminated && observation.Outcome != models.ExecutorOutcomeMissing && observation.Outcome != models.ExecutorOutcomeRestarted) {
		return
	}
	store, ok := s.tasks.(executorFailureSettlementRepository)
	if !ok {
		return
	}
	recovery, ok := s.sessions.(orphanedSessionRepository)
	if !ok {
		return
	}
	refs, err := store.ListExecutorFailureAffectedSessions(ctx, target.TaskID)
	if err != nil {
		return
	}
	for _, ref := range refs {
		if ref.EnvironmentID != target.EnvironmentID || ref.ResourceKey != target.ResourceKey || ref.OwnershipGeneration != target.OwnershipGeneration {
			continue
		}
		s.settleExecutorFailureSession(ctx, store, recovery, target, observation, ref)

	}
}

// ObserveExecutorFailure is the same durable admission path used by periodic
// inspection, invoked immediately for a correlated managed-stream loss.
func (s *Service) ObserveExecutorFailure(ctx context.Context, target models.ExecutorObservationTarget, observation *models.ExecutorObservation) error {
	store, ok := s.tasks.(executorFailureRepository)
	if !ok {
		return fmt.Errorf("executor failure persistence unavailable")
	}
	_, changed, err := store.ObserveExecutorFailure(ctx, target, observation)
	if err != nil {
		return err
	}
	if changed {
		s.PublishTaskUpdatedByID(ctx, target.TaskID)
		s.publishExecutorFailureHistory(ctx, target.TaskID)
	}
	s.settleExecutorFailureSessions(ctx, target, observation)
	return nil
}

func (s *Service) inspectExecutorObservation(ctx context.Context, target models.ExecutorObservationTarget) (*models.ExecutorObservation, error) {
	if target.ResourceKey == "" {
		return &models.ExecutorObservation{Outcome: models.ExecutorOutcomeUnknown, Runtime: target.Runtime, ObservedAt: time.Now().UTC(), Workspace: models.ExecutorOutcomeUnknown}, nil
	}
	observation, err := s.executorInspector(ctx, target)
	if observation == nil {
		observation = &models.ExecutorObservation{Outcome: models.ExecutorOutcomeUnknown, ResourceKey: target.ResourceKey, Runtime: target.Runtime, ObservedAt: time.Now().UTC(), Workspace: models.ExecutorOutcomeUnknown}
	} else {
		observation = observation.Clone()
	}
	if err != nil {
		observation.Outcome = models.ExecutorOutcomeUnknown
	}
	return observation, err
}

func (s *Service) recheckRecordedExecutorFailure(ctx context.Context, store executorFailureRepository, target models.ExecutorObservationTarget, episode *models.ExecutorFailureEpisode) (*models.ExecutorFailureEpisode, error) {
	observation, inspectErr := s.inspectExecutorObservation(ctx, target)
	updated, changed, err := store.ObserveExecutorFailure(ctx, target, observation)
	if err != nil {
		return nil, err
	}
	if changed {
		s.PublishTaskUpdatedByID(ctx, target.TaskID)
		s.publishExecutorFailureHistory(ctx, target.TaskID)
	}
	if inspectErr != nil {
		return nil, inspectErr
	}
	if observation.Outcome == models.ExecutorOutcomeUnknown {
		return nil, fmt.Errorf("current executor status cannot be verified")
	}
	if updated == nil {
		return episode, nil
	}
	return updated, nil
}

func (s *Service) settleExecutorFailureSession(ctx context.Context, store executorFailureSettlementRepository, recovery orphanedSessionRepository, target models.ExecutorObservationTarget, observation *models.ExecutorObservation, ref models.ExecutorFailureAffectedSession) {
	candidate := ref.Candidate
	session, err := s.sessions.GetTaskSession(ctx, candidate.SessionID)
	if err != nil || session == nil || session.TaskEnvironmentID != target.EnvironmentID {
		return
	}
	correlated := target
	correlated.SessionID = candidate.SessionID
	correlated.ExecutionID = candidate.ExpectedExecutorAgentExecutionID
	correlated.ExpectedExecutorUpdatedAt = candidate.ExpectedExecutorUpdatedAt
	retired, err := s.executorLossRetirer(ctx, correlated, observation)
	if err != nil || !retired {
		return
	}
	recovered, err := recovery.RecoverTaskSessionByCandidate(ctx, candidate, time.Time{})
	if err != nil {
		return
	}
	if recovered == nil {
		if pending, ok := models.LoadInterruptedRecoverySettlement(session.Metadata); ok && pending.ExpectedExecutorAgentExecutionID == candidate.ExpectedExecutorAgentExecutionID {
			candidate.RecoveredUpdatedAt = pending.RecoveredUpdatedAt
			recovered = session
		} else {
			return
		}
	}
	s.settleRecoveredSession(ctx, candidate, recovered)
	latest, err := s.sessions.GetTaskSession(ctx, candidate.SessionID)
	if err != nil || latest == nil {
		return
	}
	if _, pending := models.LoadInterruptedRecoverySettlement(latest.Metadata); pending {
		return
	}
	if err = store.CompleteExecutorFailureSettlement(ctx, ref); err != nil {
		s.logger.Warn("executor interruption settlement remains pending", zap.Error(err))
	}
}

func executorFailureTargetMatches(target models.ExecutorObservationTarget, episode *models.ExecutorFailureEpisode) bool {
	return target.TaskID == episode.TaskID && target.EnvironmentID == episode.EnvironmentID && target.SessionID == episode.SessionID && target.OwnershipGeneration == episode.OwnershipGeneration && target.ResourceKey == episode.ResourceKey
}
