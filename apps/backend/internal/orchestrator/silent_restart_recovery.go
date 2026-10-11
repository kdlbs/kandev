package orchestrator

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	runtimekind "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
)

const (
	silentRestoreCandidateLimit       = 100
	silentRestoreOperationLimit       = 2 * time.Minute
	silentRestoreCandidateLaunchLimit = 3
)

var errSilentRestoreCandidateDead = errors.New("silent restore candidate process is proven dead")

type silentRestoreRunningDeleter interface {
	DeleteExecutorRunningIfCurrent(context.Context, string, string, time.Time) error
}

func silentRestoreAttemptID(sessionID, incarnationID string, generation int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d", sessionID, incarnationID, generation)))
	return "silent-restore-" + hex.EncodeToString(sum[:])
}

func (s *Service) startSilentRestoreRecovery(parent context.Context) {
	store, ok := s.repo.(taskrepo.SilentRestoreRepository)
	if !ok || s.silentRestoreOwnerContext == nil || parent == nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	s.silentRestoreMu.Lock()
	if s.silentRestoreStopped || s.silentRestoreCancel != nil {
		s.silentRestoreMu.Unlock()
		cancel()
		return
	}
	s.silentRestoreCancel = cancel
	s.silentRestoreWorkers.Add(1)
	s.silentRestoreMu.Unlock()
	go func() {
		defer s.silentRestoreWorkers.Done()
		s.runSilentRestoreRecoveryPass(ctx, store)
	}()
}

func (s *Service) stopSilentRestoreRecovery() {
	s.silentRestoreMu.Lock()
	s.silentRestoreStopped = true
	cancel := s.silentRestoreCancel
	s.silentRestoreCancel = nil
	s.silentRestoreMu.Unlock()
	if cancel != nil {
		cancel()
		s.silentRestoreWorkers.Wait()
	}
}

func (s *Service) resetSilentRestoreRecovery() {
	s.silentRestoreMu.Lock()
	s.silentRestoreStopped = false
	s.silentRestoreMu.Unlock()
}

func (s *Service) runSilentRestoreRecoveryPass(ctx context.Context, store taskrepo.SilentRestoreRepository) {
	after := ""
	for ctx.Err() == nil {
		candidates, err := store.ListSilentRestoreCandidates(ctx, after, silentRestoreCandidateLimit)
		if err != nil {
			s.logger.Warn("could not list silent restart recovery candidates", zap.Error(err))
			return
		}
		for _, candidate := range candidates {
			if ctx.Err() != nil {
				return
			}
			opCtx, cancel := context.WithTimeout(ctx, silentRestoreOperationLimit)
			err := s.restoreSilentRestartCandidate(opCtx, store, candidate)
			cancel()
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				s.logger.Debug("silent restart recovery candidate remains pending",
					zap.String("session_id", candidate.SessionID), zap.Error(err))
			}
			after = candidate.SessionID
		}
		if len(candidates) < silentRestoreCandidateLimit {
			return
		}
	}
}

func (s *Service) reserveSilentRestoreCapacity(
	ctx context.Context,
	taskID, sessionID string,
) (*sessionKeyedCeilingReservation, bool) {
	if s.sessionCeiling == nil {
		return nil, false
	}
	decision := s.sessionCeiling.admit(ctx, admissionRequest{
		taskID: taskID, sessionID: sessionID, origin: launchOriginAutomatic, seam: "resumeTaskSession",
	})
	if !decision.admitted {
		return nil, false
	}
	return &sessionKeyedCeilingReservation{
		controller: s.sessionCeiling, key: decision.reservationKey,
		manualOverride: decision.manualOverride, population: decision.population,
		populationKnown: decision.populationKnown, ceiling: decision.ceiling,
	}, true
}

func (s *Service) ensureSilentRestoreCandidate(
	ctx context.Context,
	store taskrepo.SilentRestoreRepository,
	attemptID string,
	checkpoint models.SilentRestoreCheckpoint,
	inputs *silentRestoreInputs,
) error {
	if checkpoint.CandidateExecutionID == "" {
		expected := checkpoint
		next := expected
		next.CandidateExecutionID = uuid.NewString()
		next.Stage = models.SilentRestoreStageCandidateAllocated
		changed, err := store.CompareAndSwapSilentRestoreCheckpoint(ctx, attemptID, expected, next)
		if err != nil {
			return err
		}
		if !changed {
			return errors.New("silent restore checkpoint changed before candidate allocation")
		}
		checkpoint = next
	}
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, inputs.session.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, models.ErrExecutorRunningNotFound) {
		return err
	}
	return s.reconcileOrStartSilentRestoreCandidate(ctx, store, attemptID, checkpoint, inputs, running)
}

func (s *Service) reconcileOrStartSilentRestoreCandidate(
	ctx context.Context,
	store taskrepo.SilentRestoreRepository,
	attemptID string,
	checkpoint models.SilentRestoreCheckpoint,
	inputs *silentRestoreInputs,
	running *models.ExecutorRunning,
) error {
	var err error
	for {
		switch checkpoint.Stage {
		case models.SilentRestoreStageCandidateLaunching:
			err := s.finishLaunchedSilentRestoreCandidate(ctx, store, attemptID, checkpoint, inputs, running)
			if !errors.Is(err, errSilentRestoreCandidateDead) {
				return err
			}
		case models.SilentRestoreStageCandidateDead:
		case models.SilentRestoreStageCandidateAllocated:
		default:
			return nil
		}
		if inputs.launchAttempts >= silentRestoreCandidateLaunchLimit {
			return errors.New("silent restore candidate launch retry limit reached with dead candidate evidence retained")
		}
		if checkpoint.Stage == models.SilentRestoreStageCandidateLaunching ||
			checkpoint.Stage == models.SilentRestoreStageCandidateDead {
			checkpoint, err = s.rotateDeadSilentRestoreCandidate(ctx, store, attemptID, checkpoint, inputs)
			if err != nil {
				return err
			}
			running = nil
		}
		inputs.launchAttempts++
		err = s.startSilentRestoreCandidate(ctx, store, attemptID, checkpoint, inputs, running)
		if !errors.Is(err, errSilentRestoreCandidateDead) {
			return err
		}
		if inputs.launchAttempts >= silentRestoreCandidateLaunchLimit {
			return errors.New("silent restore candidate launch retry limit reached with dead candidate evidence retained")
		}
		checkpoint, err = s.rotateDeadSilentRestoreCandidate(ctx, store, attemptID, checkpoint, inputs)
		if err != nil {
			return err
		}
		running = nil
	}
}

func (s *Service) startSilentRestoreCandidate(
	ctx context.Context,
	store taskrepo.SilentRestoreRepository,
	attemptID string,
	checkpoint models.SilentRestoreCheckpoint,
	inputs *silentRestoreInputs,
	running *models.ExecutorRunning,
) error {
	ownerCtx, err := s.revalidateSilentRestoreInputs(ctx, inputs, checkpoint)
	if err != nil {
		return err
	}
	inputs.ownerCtx = ownerCtx
	if err := s.removeProvenDeadSourceExecution(ownerCtx, inputs, checkpoint, running); err != nil {
		return err
	}
	ownerCtx, err = s.revalidateSilentRestoreInputs(ownerCtx, inputs, checkpoint)
	if err != nil {
		return err
	}
	inputs.ownerCtx = ownerCtx
	reservation, admitted := s.reserveSilentRestoreCapacity(ownerCtx, inputs.task.ID, inputs.session.ID)
	if !admitted {
		return nil
	}
	defer reservation.releaseIfNotConsumed()
	if err := s.pauseSilentRestoreQueue(ownerCtx, inputs.task.ID, inputs.session.ID); err != nil {
		return err
	}
	callback := func(callbackCtx context.Context, executionID string) error {
		return s.markSilentRestoreCandidateLaunching(callbackCtx, store, attemptID, checkpoint, executionID, inputs)
	}
	options := executor.ResumeOptions{
		RequiredNativeConversationID:   checkpoint.SourceGeneration.NativeSessionID,
		InterruptedSubmissionID:        inputs.recovery.SubmissionID,
		InterruptedStreamID:            inputs.recovery.StreamID,
		InterruptedHarnessGeneration:   uint64(inputs.recovery.HarnessGeneration),
		NoInitialPrompt:                true,
		SuppressInitialMessageBackfill: true,
		StartAgentSynchronously:        true, Origin: string(launchOriginAutomatic),
		CandidateExecutionID: checkpoint.CandidateExecutionID,
		OnExecutionAllocated: callback,
	}
	commitWhenReady := func(readyCtx context.Context, _ *resumeAttempt, execution *executor.TaskExecution) error {
		if execution == nil || execution.AgentExecutionID != checkpoint.CandidateExecutionID {
			return errors.New("silent restart recovery resumed a different execution")
		}
		running, getErr := s.repo.GetExecutorRunningBySessionID(readyCtx, inputs.session.ID)
		if getErr != nil {
			return getErr
		}
		return s.finishLaunchedSilentRestoreCandidateLocked(
			readyCtx, store, attemptID, checkpoint, inputs, running,
		)
	}
	ownedResumeCtx := context.WithValue(ownerCtx, continuationOwnedContextKey{}, true)
	execution, err := s.resumeTaskSessionWithContinuationAndReservation(
		ownedResumeCtx, inputs.task.ID, inputs.session.ID, options, commitWhenReady, reservation,
	)
	if err != nil {
		return err
	}
	if execution == nil {
		return errors.New("silent restart restore was not admitted")
	}
	return nil
}

func (s *Service) readSilentRestoreCandidateExecution(
	ctx context.Context,
	sessionID, executionID string,
) (*models.ExecutorRunning, error) {
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, models.ErrExecutorRunningNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if running == nil {
		return nil, nil
	}
	if running.AgentExecutionID != executionID {
		return nil, errors.New("session execution changed while reconciling a dead silent restore candidate")
	}
	return running, nil
}

func (s *Service) rotateDeadSilentRestoreCandidate(
	ctx context.Context,
	store taskrepo.SilentRestoreRepository,
	attemptID string,
	expected models.SilentRestoreCheckpoint,
	inputs *silentRestoreInputs,
) (models.SilentRestoreCheckpoint, error) {
	releaseRoute := s.acquireSessionRouteOperationLock(inputs.session.ID)
	defer releaseRoute()
	releaseLifecycle := s.acquireSessionLifecycleLock(inputs.session.ID)
	defer releaseLifecycle()
	attempt, err := store.GetRestoreAttempt(ctx, attemptID)
	if err != nil {
		return models.SilentRestoreCheckpoint{}, err
	}
	if attempt == nil || attempt.Checkpoint == nil ||
		!sameSilentRestorePinnedSource(*attempt.Checkpoint, expected) ||
		attempt.Checkpoint.CandidateExecutionID != expected.CandidateExecutionID {
		return models.SilentRestoreCheckpoint{}, errors.New("silent restore checkpoint changed while rotating a dead candidate")
	}
	checkpoint := *attempt.Checkpoint
	if checkpoint.Stage != models.SilentRestoreStageCandidateLaunching &&
		checkpoint.Stage != models.SilentRestoreStageCandidateDead {
		return models.SilentRestoreCheckpoint{}, errors.New("silent restore candidate is not durably marked as launched or dead")
	}
	running, err := s.readSilentRestoreCandidateExecution(ctx, inputs.session.ID, checkpoint.CandidateExecutionID)
	if err != nil {
		return models.SilentRestoreCheckpoint{}, err
	}
	if checkpoint.Stage == models.SilentRestoreStageCandidateLaunching && running == nil {
		return models.SilentRestoreCheckpoint{}, errors.New("launched candidate has unknown process liveness without durable execution evidence")
	}
	return s.rotateSilentRestoreCandidate(ctx, store, attemptID, checkpoint, running, inputs)
}

func (s *Service) pauseSilentRestoreQueue(ctx context.Context, taskID, sessionID string) error {
	if s.messageQueue == nil {
		return nil
	}
	identity, err := s.messageQueue.ResolveSessionIdentity(ctx, taskID, sessionID)
	if err != nil {
		return fmt.Errorf("resolve queue identity before silent restore: %w", err)
	}
	paused, err := s.messageQueue.PauseAutoRunIfPendingForSession(ctx, identity)
	if err != nil {
		return fmt.Errorf("pause queued work before silent restore: %w", err)
	}
	if paused {
		s.publishTaskQueueStatusEvent(ctx, taskID, sessionID)
	}
	return nil
}

func (s *Service) removeProvenDeadSourceExecution(
	ctx context.Context,
	inputs *silentRestoreInputs,
	checkpoint models.SilentRestoreCheckpoint,
	initial *models.ExecutorRunning,
) error {
	running, err := s.loadSilentRestoreSourceExecution(ctx, inputs, checkpoint, initial)
	if err != nil {
		return err
	}
	if running == nil {
		return nil
	}
	if err := s.proveSilentRestoreSourceDead(running, inputs.recovery); err != nil {
		return err
	}
	ownerCtx, err := s.revalidateSilentRestoreInputs(ctx, inputs, checkpoint)
	if err != nil {
		return err
	}
	return s.cleanupProvenDeadSilentRestoreExecution(ownerCtx, running)
}

func (s *Service) loadSilentRestoreSourceExecution(
	ctx context.Context,
	inputs *silentRestoreInputs,
	checkpoint models.SilentRestoreCheckpoint,
	initial *models.ExecutorRunning,
) (*models.ExecutorRunning, error) {
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, inputs.session.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, models.ErrExecutorRunningNotFound) {
		return nil, err
	}
	if running == nil || errors.Is(err, sql.ErrNoRows) || errors.Is(err, models.ErrExecutorRunningNotFound) {
		if initial != nil && s.agentManager.IsAgentRunningForSession(ctx, inputs.session.ID) {
			return nil, errors.New("session has a live execution without its durable owner row")
		}
		return nil, nil
	}
	if running.SessionID != inputs.session.ID || running.TaskID != inputs.task.ID {
		return nil, errors.New("session execution owner changed during silent restart recovery")
	}
	if running.AgentExecutionID == checkpoint.CandidateExecutionID {
		return nil, errors.New("candidate execution row exists before its launch checkpoint")
	}
	if running.AgentExecutionID != inputs.recovery.AgentExecutionID {
		return nil, errors.New("session execution does not match either pinned identity")
	}
	return running, nil
}

func (s *Service) proveSilentRestoreSourceDead(
	running *models.ExecutorRunning,
	recovery models.AgentDeliveryRecovery,
) error {
	state := models.ProcessLivenessUnknown
	if prober, ok := s.agentManager.(rowLivenessProber); ok {
		state = prober.RowLiveness(running)
	}
	dead, err := silentRestoreSourceIsProvenDead(
		running, recovery, state, processidentity.OwnedSessionTerminated,
	)
	if err != nil {
		return err
	}
	if !dead {
		return errors.New("predecessor execution liveness is not proven dead")
	}
	return nil
}

func silentRestoreSourceIsProvenDead(
	running *models.ExecutorRunning,
	recovery models.AgentDeliveryRecovery,
	liveness models.ProcessLiveness,
	ownedSessionTerminated func(processidentity.Identity) (bool, error),
) (bool, error) {
	if err := validateSilentRestoreSourceIdentity(running, recovery); err != nil {
		return false, err
	}
	switch liveness {
	case models.ProcessLivenessDead:
		return true, nil
	case models.ProcessLivenessAlive:
		return false, errors.New("source execution is still alive")
	case models.ProcessLivenessUnknown:
		return proveUnknownStandaloneSilentRestoreSource(running, recovery, ownedSessionTerminated)
	default:
		return false, errors.New("source execution liveness is not proven")
	}
}

func validateSilentRestoreSourceIdentity(running *models.ExecutorRunning, recovery models.AgentDeliveryRecovery) error {
	if running == nil || recovery.AgentExecutionID == "" ||
		running.SessionID != recovery.SessionID || running.AgentExecutionID != recovery.AgentExecutionID {
		return errors.New("source execution identity changed")
	}
	return nil
}

func proveUnknownStandaloneSilentRestoreSource(
	running *models.ExecutorRunning,
	recovery models.AgentDeliveryRecovery,
	ownedSessionTerminated func(processidentity.Identity) (bool, error),
) (bool, error) {
	if running.Runtime != runtimekind.RuntimeStandalone {
		return false, errors.New("remote source execution cannot use local process proof")
	}
	identity := recovery.OriginalRuntime
	if identity.Validate() != nil || identity.GroupID != identity.PID || identity.SessionID != identity.PID {
		return false, processidentity.ErrUnverifiableIdentity
	}
	if ownedSessionTerminated == nil {
		return false, processidentity.ErrUnverifiableIdentity
	}
	terminated, err := ownedSessionTerminated(identity)
	if err != nil {
		return false, err
	}
	if !terminated {
		return false, errors.New("source process session is not fully terminated")
	}
	return true, nil
}

func (s *Service) cleanupProvenDeadSilentRestoreExecution(
	ctx context.Context,
	running *models.ExecutorRunning,
) error {
	cleaner, ok := s.agentManager.(executionIdentityCleaner)
	if !ok {
		return errors.New("identity-scoped runtime cleanup capability is unavailable")
	}
	if err := cleaner.CleanupStaleExecutionBySessionIDIfCurrent(
		ctx, running.SessionID, running.AgentExecutionID, running.UpdatedAt,
	); err != nil {
		return err
	}
	deleter, ok := s.repo.(silentRestoreRunningDeleter)
	if !ok {
		return errors.New("executor cleanup capability is unavailable")
	}
	if err := deleter.DeleteExecutorRunningIfCurrent(ctx, running.SessionID, running.AgentExecutionID, running.UpdatedAt); err != nil &&
		!errors.Is(err, models.ErrExecutorRunningNotFound) {
		return err
	}
	return nil
}

func (s *Service) markSilentRestoreCandidateLaunching(
	ctx context.Context,
	store taskrepo.SilentRestoreRepository,
	attemptID string,
	checkpoint models.SilentRestoreCheckpoint,
	executionID string,
	inputs *silentRestoreInputs,
) error {
	if executionID != checkpoint.CandidateExecutionID {
		return errors.New("silent restore candidate identity or owner changed before launch")
	}
	ownerCtx, err := s.revalidateSilentRestoreInputs(ctx, inputs, checkpoint)
	if err != nil {
		return err
	}
	next := checkpoint
	next.Stage = models.SilentRestoreStageCandidateLaunching
	changed, err := store.CompareAndSwapSilentRestoreCheckpoint(ownerCtx, attemptID, checkpoint, next)
	if err != nil {
		return err
	}
	if !changed {
		return errors.New("silent restore candidate checkpoint changed before launch")
	}
	return nil
}

func (s *Service) finishLaunchedSilentRestoreCandidate(
	ctx context.Context,
	store taskrepo.SilentRestoreRepository,
	attemptID string,
	checkpoint models.SilentRestoreCheckpoint,
	inputs *silentRestoreInputs,
	running *models.ExecutorRunning,
) error {
	releaseRoute := s.acquireSessionRouteOperationLock(inputs.session.ID)
	defer releaseRoute()
	releaseLifecycle := s.acquireSessionLifecycleLock(inputs.session.ID)
	defer releaseLifecycle()
	return s.finishLaunchedSilentRestoreCandidateLocked(ctx, store, attemptID, checkpoint, inputs, running)
}

func (s *Service) finishLaunchedSilentRestoreCandidateLocked(
	ctx context.Context,
	store taskrepo.SilentRestoreRepository,
	attemptID string,
	checkpoint models.SilentRestoreCheckpoint,
	inputs *silentRestoreInputs,
	running *models.ExecutorRunning,
) error {
	ownerCtx, ready, err := s.silentRestoreCandidateReady(ctx, inputs, checkpoint, running)
	if err != nil {
		return err
	}
	if !ready {
		return nil
	}
	inputs.ownerCtx = ownerCtx
	attempt, err := store.GetRestoreAttempt(ctx, attemptID)
	if err != nil {
		return err
	}
	if attempt == nil || attempt.Checkpoint == nil ||
		attempt.Checkpoint.Stage != models.SilentRestoreStageCandidateLaunching ||
		attempt.Checkpoint.CandidateExecutionID != checkpoint.CandidateExecutionID {
		return nil
	}
	commitCheckpoint := *attempt.Checkpoint
	ownerCtx, err = s.validateSilentRestoreCommitInputs(ownerCtx, inputs, commitCheckpoint)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	next := checkpoint.SourceGeneration
	next.Generation++
	next.PredecessorGeneration = checkpoint.SourceGeneration.Generation
	next.CreationReason = silentRestartRestoredCreationReason
	next.CreatedAt, next.CommittedAt = now, now
	committed, err := store.CommitSilentRestore(ownerCtx, &models.SilentRestoreCommit{
		AttemptID: attemptID, Checkpoint: commitCheckpoint, Generation: next, CompletedAt: now,
	})
	if err == nil && !committed {
		return errors.New("silent restart recovery checkpoint changed before commit")
	}
	if err == nil {
		if session, readErr := s.repo.GetTaskSession(ownerCtx, inputs.session.ID); readErr == nil && session != nil {
			s.publishTaskSessionStateChanged(ownerCtx, session.TaskID, session.ID, session.State, session.State, session.ErrorMessage, &session.UpdatedAt, session)
		}
	}
	return err
}

func (s *Service) silentRestoreCandidateReady(
	ctx context.Context,
	inputs *silentRestoreInputs,
	checkpoint models.SilentRestoreCheckpoint,
	running *models.ExecutorRunning,
) (context.Context, bool, error) {
	if running == nil || running.AgentExecutionID != checkpoint.CandidateExecutionID {
		return nil, false, nil
	}
	ownerCtx, err := s.revalidateSilentRestoreInputs(ctx, inputs, checkpoint)
	if err != nil {
		return nil, false, err
	}
	prober, ok := s.agentManager.(rowLivenessProber)
	if !ok {
		return nil, false, nil
	}
	switch prober.RowLiveness(running) {
	case models.ProcessLivenessDead:
		return nil, false, errSilentRestoreCandidateDead
	case models.ProcessLivenessAlive:
	default:
		return nil, false, nil
	}
	if !s.agentManager.IsAgentReadyForPrompt(ownerCtx, inputs.session.ID) {
		return nil, false, nil
	}
	nativeID := s.currentACPSessionID(inputs.session.ID)
	if nativeID == "" || nativeID != checkpoint.SourceGeneration.NativeSessionID {
		return nil, false, nil
	}
	return ownerCtx, true, nil
}

func (s *Service) validateSilentRestoreCommitInputs(
	ctx context.Context,
	inputs *silentRestoreInputs,
	checkpoint models.SilentRestoreCheckpoint,
) (context.Context, error) {
	return s.revalidateSilentRestoreInputs(ctx, inputs, checkpoint)
}

func (s *Service) rotateSilentRestoreCandidate(
	ctx context.Context,
	store taskrepo.SilentRestoreRepository,
	attemptID string,
	expected models.SilentRestoreCheckpoint,
	running *models.ExecutorRunning,
	inputs *silentRestoreInputs,
) (models.SilentRestoreCheckpoint, error) {
	dead := expected
	var err error
	if expected.Stage == models.SilentRestoreStageCandidateLaunching {
		dead, err = s.markSilentRestoreCandidateDead(ctx, store, attemptID, expected, running, inputs)
		if err != nil {
			return models.SilentRestoreCheckpoint{}, err
		}
	} else if expected.Stage != models.SilentRestoreStageCandidateDead {
		return models.SilentRestoreCheckpoint{}, errors.New("candidate is not durably marked as dead")
	}
	if running != nil {
		prober, ok := s.agentManager.(rowLivenessProber)
		if !ok || prober.RowLiveness(running) != models.ProcessLivenessDead {
			return models.SilentRestoreCheckpoint{}, errors.New("candidate execution liveness is not proven dead")
		}
		ownerCtx, err := s.revalidateSilentRestoreInputs(ctx, inputs, dead)
		if err != nil {
			return models.SilentRestoreCheckpoint{}, err
		}
		if err := s.cleanupProvenDeadSilentRestoreExecution(ownerCtx, running); err != nil {
			return models.SilentRestoreCheckpoint{}, err
		}
	}
	ownerCtx, err := s.revalidateSilentRestoreInputs(ctx, inputs, dead)
	if err != nil {
		return models.SilentRestoreCheckpoint{}, err
	}
	next := dead
	next.Stage = models.SilentRestoreStageCandidateAllocated
	next.CandidateExecutionID = uuid.NewString()
	changed, err := store.CompareAndSwapSilentRestoreCheckpoint(ownerCtx, attemptID, dead, next)
	if err != nil {
		return models.SilentRestoreCheckpoint{}, err
	}
	if !changed {
		return models.SilentRestoreCheckpoint{}, errors.New("silent restore checkpoint changed while rotating a dead candidate")
	}
	return next, nil
}

func (s *Service) markSilentRestoreCandidateDead(
	ctx context.Context,
	store taskrepo.SilentRestoreRepository,
	attemptID string,
	expected models.SilentRestoreCheckpoint,
	running *models.ExecutorRunning,
	inputs *silentRestoreInputs,
) (models.SilentRestoreCheckpoint, error) {
	if expected.Stage != models.SilentRestoreStageCandidateLaunching || running == nil ||
		running.AgentExecutionID != expected.CandidateExecutionID {
		return models.SilentRestoreCheckpoint{}, errors.New("candidate death has not been proven for the pinned execution")
	}
	prober, ok := s.agentManager.(rowLivenessProber)
	if !ok || prober.RowLiveness(running) != models.ProcessLivenessDead {
		return models.SilentRestoreCheckpoint{}, errors.New("candidate execution liveness is not proven dead")
	}
	ownerCtx, err := s.revalidateSilentRestoreInputs(ctx, inputs, expected)
	if err != nil {
		return models.SilentRestoreCheckpoint{}, err
	}
	dead := expected
	dead.Stage = models.SilentRestoreStageCandidateDead
	changed, err := store.CompareAndSwapSilentRestoreCheckpoint(ownerCtx, attemptID, expected, dead)
	if err != nil {
		return models.SilentRestoreCheckpoint{}, err
	}
	if !changed {
		return models.SilentRestoreCheckpoint{}, errors.New("silent restore checkpoint changed before recording candidate death")
	}
	return dead, nil
}
