package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"

	"github.com/google/uuid"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/task/models"
)

type inferenceClosureRuntime interface {
	InferenceClosureStep(context.Context, string, string, utility.InferenceClosureRequest) (*utility.InferenceClosureResponse, error)
	CaptureInferenceClosureClaims(context.Context, string, string) (json.RawMessage, error)
	SettleInferenceClosure(context.Context, string, string, *utility.InferenceClosureResponse, json.RawMessage) error
	VerifyInferenceClosureClaims(context.Context, string, string) error
}

var errRestartInferenceClaimsChanged = errors.New("restart inference claims changed")

// StopForRestart preserves the existing owner scope even if capacity settings
// changed, and resumes an interrupted closure before considering a new one.
func (s *Service) StopForRestart(ctx context.Context, sid, reason string) (*models.InferenceClosureCheckpoint, bool, error) {
	if err := s.authorizeSessionControl(ctx, sid); err != nil {
		return nil, true, err
	}
	all, finite := s.InferenceCapacityPolicy()
	retained, err := s.restartHasInferenceClaims(ctx, sid, "", false)
	if err != nil {
		return nil, true, err
	}
	result, handled, err := s.stopInferenceClosureForRestart(ctx, sid, all && finite || retained || s.hasInferenceClosureComposition())
	if err != nil || handled {
		return result, handled, err
	}
	result, handled, err = s.stopLegacyRestartWithFence(ctx, sid, reason)
	if errors.Is(err, errRestartInferenceClaimsChanged) {
		return s.stopInferenceClosureForRestart(ctx, sid, true)
	}
	return result, handled, err
}

func (s *Service) hasInferenceClosureComposition() bool {
	if s == nil || s.sessionCeiling == nil {
		return false
	}
	_, hasStore := s.repo.(inferenceClosureStore)
	_, hasProbe := s.agentManager.(interface {
		ProbeInferenceClosure(context.Context, string, string) error
	})
	s.sessionCeiling.mu.Lock()
	hasCapacityStore := s.sessionCeiling.inferenceStore != nil
	s.sessionCeiling.mu.Unlock()
	return hasStore && hasProbe && hasCapacityStore
}

// stopLegacyRestartWithFence serializes the final claim check with admission.
// The exact teardown owner remains visible until the awaited stop completes.
func (s *Service) stopLegacyRestartWithFence(ctx context.Context, sid, reason string) (*models.InferenceClosureCheckpoint, bool, error) {
	release, err := s.acquireInferenceRestartAdmission(ctx, sid)
	if err != nil {
		return nil, true, err
	}
	defer release()
	if s.agentManager == nil {
		return nil, true, models.ErrInferenceClosureBlocked
	}
	execution, lookupErr := s.agentManager.GetExecutionIDForSession(ctx, sid)
	absent := errors.Is(lookupErr, agentruntime.ErrNoExecutionForSession)
	if !absent && (lookupErr != nil || execution == "") {
		return nil, true, models.ErrInferenceClosureBlocked
	}
	retained, err := s.restartHasInferenceClaims(ctx, sid, execution, absent)
	if err == nil && !retained {
		err = s.checkInferenceClosureAdmission(ctx, sid, false)
	}
	if err != nil || retained {
		if err == nil {
			err = errRestartInferenceClaimsChanged
		}
		return nil, true, err
	}
	if absent {
		return nil, true, s.settleExecutionLessRestart(ctx, sid, lookupErr)
	}
	claim, owned := s.claimExecutionTeardownWithToken(sid, execution, executionTeardownIntentGraceful)
	if !owned {
		return nil, true, models.ErrInferenceClosureConflict
	}
	release()
	return nil, true, s.awaitLegacyRestartStop(ctx, sid, execution, reason, claim)
}

func (s *Service) settleExecutionLessRestart(ctx context.Context, sid string, lookupErr error) error {
	_, resuming := s.resumeAttemptStore().current(sid)
	if resuming || s.isCancelInFlight(sid) {
		return models.ErrInferenceClosureConflict
	}
	s.resetTransientRetryWithContext(ctx, sid, true)
	if s.lspLeases != nil {
		s.lspLeases.StopLSPLeasesForSession(sid)
	}
	if settled, err := s.settleExecutionLessSessionTurn(ctx, sid); settled || err != nil {
		return err
	}
	return lookupErr
}

// A launch can already hold the cancellation guard while acquiring the
// lifecycle lock. Yield to that launch instead of waiting with both in play.
func (s *Service) acquireInferenceRestartAdmission(ctx context.Context, sid string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	releaseLifecycle, locked := s.tryAcquireSessionLifecycleLock(sid)
	if !locked {
		return nil, models.ErrInferenceClosureConflict
	}
	guard, releaseGuard := s.acquireCancelInFlightGuard(sid)
	if !guard.TryLock() {
		releaseGuard()
		releaseLifecycle()
		return nil, models.ErrInferenceClosureConflict
	}
	release := sync.OnceFunc(func() {
		guard.Unlock()
		releaseGuard()
		releaseLifecycle()
	})
	if err := ctx.Err(); err != nil {
		release()
		return nil, err
	}
	return release, nil
}

func (s *Service) awaitLegacyRestartStop(ctx context.Context, sid, execution, reason string, claim executionTeardownClaim) error {
	stop, err := s.StopSessionAwaited(ctx, sid, execution, reason)
	if err == nil {
		if observation := stop.Observation; observation != nil && observation.Settlement != nil && observation.Settlement.Unresolved != 0 {
			return models.ErrInferenceClosureBlocked
		}
		return s.verifyLegacyRestartClaims(ctx, sid, execution)
	}
	if stop == nil || !stop.Accepted {
		s.executionTeardownClaims.CompareAndDelete(terminalExecutionKey(sid, execution), claim)
	}
	return err
}

// Stop may discover or settle claims synchronously. Verify the stopped
// execution, even after it is no longer registered, before allowing a restart.
func (s *Service) verifyLegacyRestartClaims(ctx context.Context, sid, execution string) error {
	if runtime, ok := s.agentManager.(interface {
		VerifyInferenceClosureClaims(context.Context, string, string) error
	}); ok && s.hasInferenceClosureComposition() {
		if err := runtime.VerifyInferenceClosureClaims(ctx, sid, execution); err != nil {
			return errors.Join(models.ErrInferenceClosureBlocked, err)
		}
		return nil
	}
	retained, err := s.restartHasInferenceClaims(ctx, sid, execution, false)
	if err != nil {
		return err
	}
	if retained {
		return models.ErrInferenceClosureBlocked
	}
	return nil
}

func (s *Service) restartHasInferenceClaims(ctx context.Context, sid, execution string, executionAbsent bool) (bool, error) {
	if s == nil || s.sessionCeiling == nil {
		return false, nil
	}
	s.sessionCeiling.mu.Lock()
	store := s.sessionCeiling.inferenceStore
	s.sessionCeiling.mu.Unlock()
	if store == nil {
		return false, nil
	}
	bindings, err := store.LoadInferenceBindings(ctx)
	if err != nil {
		return false, err
	}
	for _, binding := range bindings {
		if binding.SessionID == "" || binding.SessionID == sid {
			return true, nil
		}
	}
	if len(bindings) == 0 || executionAbsent {
		return false, nil
	}
	if execution == "" {
		execution, err = s.restartExecutionForClaimLookup(ctx, sid)
		if err != nil || execution == "" {
			return false, err
		}
	}
	for _, binding := range bindings {
		if binding.InstanceID == execution {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) restartExecutionForClaimLookup(ctx context.Context, sid string) (string, error) {
	if s.agentManager == nil {
		return "", models.ErrInferenceClosureBlocked
	}
	execution, err := s.agentManager.GetExecutionIDForSession(ctx, sid)
	if errors.Is(err, agentruntime.ErrNoExecutionForSession) {
		return "", nil
	}
	if err != nil || execution == "" {
		return "", models.ErrInferenceClosureBlocked
	}
	return execution, nil
}

func (s *Service) stopInferenceClosureForRestart(ctx context.Context, sid string, enabled bool) (*models.InferenceClosureCheckpoint, bool, error) {
	if err := s.authorizeSessionControl(ctx, sid); err != nil {
		return nil, true, err
	}
	store, ok := s.repo.(inferenceClosureStore)
	if !ok {
		return s.legacyInferenceRestart(ctx, sid)
	}
	raw, err := store.ReadInferenceClosure(ctx, sid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, true, err
	}
	if err == nil {
		var prior models.InferenceClosureCheckpoint
		if json.Unmarshal(raw, &prior) != nil || !prior.Valid() || prior.SessionID != sid {
			return nil, true, models.ErrInferenceClosureBlocked
		}
		if prior.Stage != models.InferenceClosureConsumed {
			result, stopErr := s.StopSessionForInferenceClosure(ctx, sid, prior.SourceExecutionID, prior.OperationID)
			return result, true, stopErr
		}
	}
	if !enabled {
		return nil, false, nil
	}
	return s.startInferenceClosureForRestart(ctx, sid)
}

func (s *Service) startInferenceClosureForRestart(ctx context.Context, sid string) (*models.InferenceClosureCheckpoint, bool, error) {
	executionID, err := s.agentManager.GetExecutionIDForSession(ctx, sid)
	if errors.Is(err, agentruntime.ErrNoExecutionForSession) {
		return s.legacyInferenceRestart(ctx, sid)
	}
	if err != nil || executionID == "" {
		return nil, true, models.ErrInferenceClosureBlocked
	}
	session, err := s.inferenceClosureSourceSession(ctx, sid)
	if err != nil {
		return nil, true, err
	}
	office, err := s.lookupOfficeTask(ctx, session.TaskID)
	if err != nil {
		return nil, true, err
	}
	if office {
		return s.legacyInferenceRestart(ctx, sid)
	}
	result, err := s.StopSessionForInferenceClosure(ctx, sid, executionID, uuid.NewString())
	if result == nil && errors.Is(err, agentruntime.ErrInferenceOwnershipUnsupported) {
		return s.legacyInferenceRestart(ctx, sid)
	}
	return result, true, err
}

func (s *Service) legacyInferenceRestart(ctx context.Context, sid string) (*models.InferenceClosureCheckpoint, bool, error) {
	retained, err := s.restartHasInferenceClaims(ctx, sid, "", false)
	if err != nil {
		return nil, true, err
	}
	if retained {
		return nil, true, models.ErrInferenceClosureBlocked
	}
	return nil, false, nil
}

// StopSessionForInferenceClosure holds admission across separate durable stores.
// Incomplete scope or any interrupted boundary leaves the operation held.
func (s *Service) StopSessionForInferenceClosure(ctx context.Context, sid, executionID, operation string) (*models.InferenceClosureCheckpoint, error) {
	if err := s.authorizeSessionControl(ctx, sid); err != nil {
		return nil, err
	}
	store, ok := s.repo.(inferenceClosureStore)
	if !ok {
		return nil, models.ErrInferenceClosureBlocked
	}
	runtime, ok := s.agentManager.(inferenceClosureRuntime)
	if !ok {
		return nil, models.ErrInferenceClosureBlocked
	}
	if err := s.probeNewInferenceClosure(ctx, store, sid, executionID, operation); err != nil {
		return nil, err
	}
	raw, err := s.beginInferenceClosure(ctx, store, sid, executionID, operation)
	if err != nil {
		return nil, err
	}
	var checkpoint models.InferenceClosureCheckpoint
	if json.Unmarshal(raw, &checkpoint) != nil {
		return nil, models.ErrInferenceClosureConflict
	}
	if checkpoint.Stage == models.InferenceClosureFenced {
		if err = s.observeInferenceClosure(ctx, store, runtime, &checkpoint); err != nil {
			return &checkpoint, err
		}
	}
	if checkpoint.Stage == models.InferenceClosureObserved {
		if err = s.settleInferenceClosure(ctx, store, runtime, &checkpoint); err != nil {
			return &checkpoint, err
		}
	}
	if checkpoint.Stage == models.InferenceClosureSettled {
		if err = s.commitInferenceClosure(ctx, store, runtime, &checkpoint); err != nil {
			return &checkpoint, err
		}
	}
	if checkpoint.Stage == models.InferenceClosureCommitted {
		err = s.stopCommittedInferenceSource(ctx, &checkpoint)
	}
	return &checkpoint, err
}

func (s *Service) probeNewInferenceClosure(ctx context.Context, store inferenceClosureStore, sid, executionID, operation string) error {
	raw, err := store.ReadInferenceClosure(ctx, sid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		var prior models.InferenceClosureCheckpoint
		if json.Unmarshal(raw, &prior) != nil || !prior.Valid() || prior.SessionID != sid {
			return models.ErrInferenceClosureBlocked
		}
		if prior.OperationID == operation && prior.SourceExecutionID == executionID {
			return nil
		}
	}
	prober, ok := s.agentManager.(interface {
		ProbeInferenceClosure(context.Context, string, string) error
	})
	if !ok {
		return agentruntime.ErrInferenceOwnershipUnsupported
	}
	return prober.ProbeInferenceClosure(ctx, sid, executionID)
}

func (s *Service) beginInferenceClosure(ctx context.Context, store inferenceClosureStore, sid, executionID, operation string) (json.RawMessage, error) {
	release, err := s.acquireInferenceRestartAdmission(ctx, sid)
	if err != nil {
		return nil, err
	}
	defer release()
	session, err := s.inferenceClosureSourceSession(ctx, sid)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeInferenceClosureResume(ctx, session.TaskID, sid, recoveryActionResume); err != nil {
		return nil, err
	}
	raw, err := store.ReadInferenceClosure(ctx, sid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if replay, err := inferenceClosureBeginReplay(raw, sid, operation, executionID, session.QueueIncarnationID); err != nil || replay {
		return raw, err
	}
	current, lookupErr := s.agentManager.GetExecutionIDForSession(ctx, sid)
	if lookupErr != nil || current != executionID || s.isCancelInFlight(sid) || s.hasExecutionTeardownOwner(sid, executionID) {
		return nil, models.ErrInferenceClosureConflict
	}
	return store.BeginInferenceClosure(ctx, sid, operation, session.QueueIncarnationID, executionID)
}

func inferenceClosureBeginReplay(raw json.RawMessage, sid, operation, executionID, incarnation string) (bool, error) {
	if len(raw) == 0 {
		return false, nil
	}
	var prior models.InferenceClosureCheckpoint
	if json.Unmarshal(raw, &prior) != nil || !prior.Valid() || prior.SessionID != sid {
		return false, models.ErrInferenceClosureBlocked
	}
	return prior.OperationID == operation && prior.SourceExecutionID == executionID && prior.IncarnationID == incarnation, nil
}

func (s *Service) inferenceClosureSourceSession(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil || session.TaskID == "" {
		return nil, models.ErrInferenceClosureConflict
	}
	return session, nil
}

func (s *Service) observeInferenceClosure(ctx context.Context, store inferenceClosureStore, runtime inferenceClosureRuntime, c *models.InferenceClosureCheckpoint) error {
	observation, err := runtime.InferenceClosureStep(ctx, c.SessionID, c.SourceExecutionID, utility.InferenceClosureRequest{OperationID: c.OperationID, CandidateInstanceID: c.CandidateExecutionID, Action: "prepare"})
	if err != nil {
		return err
	}
	if !closureObservationMatches(c, observation, models.InferenceClosureObserved) {
		return models.ErrInferenceClosureConflict
	}
	claims, err := runtime.CaptureInferenceClosureClaims(ctx, c.SessionID, c.SourceExecutionID)
	if err != nil {
		return err
	}
	next := *c
	next.Stage = models.InferenceClosureObserved
	next.OwnerState = observation.State
	next.ClaimState = claims
	next.Observation, err = json.Marshal(observation)
	if err != nil {
		return err
	}
	return advanceInferenceClosure(ctx, store, c, next)
}

func (s *Service) settleInferenceClosure(ctx context.Context, store inferenceClosureStore, runtime inferenceClosureRuntime, c *models.InferenceClosureCheckpoint) error {
	var observation utility.InferenceClosureResponse
	if json.Unmarshal(c.Observation, &observation) != nil || !closureObservationMatches(c, &observation, models.InferenceClosureObserved) {
		return models.ErrInferenceClosureConflict
	}
	if err := runtime.SettleInferenceClosure(ctx, c.SessionID, c.SourceExecutionID, &observation, c.ClaimState); err != nil {
		return err
	}
	current, err := runtime.InferenceClosureStep(ctx, c.SessionID, c.SourceExecutionID, utility.InferenceClosureRequest{OperationID: c.OperationID, Action: "status"})
	if err != nil {
		return err
	}
	if !closureObservationMatches(c, current, models.InferenceClosureObserved) {
		return models.ErrInferenceClosureConflict
	}
	next := *c
	next.Stage = models.InferenceClosureSettled
	next.OwnerState = current.State
	next.ClaimsSettled = true
	return advanceInferenceClosure(ctx, store, c, next)
}

func (s *Service) commitInferenceClosure(ctx context.Context, store inferenceClosureStore, runtime inferenceClosureRuntime, c *models.InferenceClosureCheckpoint) error {
	if err := runtime.VerifyInferenceClosureClaims(ctx, c.SessionID, c.SourceExecutionID); err != nil {
		return err
	}
	current, err := runtime.InferenceClosureStep(ctx, c.SessionID, c.SourceExecutionID, utility.InferenceClosureRequest{OperationID: c.OperationID, Action: "status"})
	if err != nil {
		return err
	}
	if !closureObservationMatches(c, current, models.InferenceClosureCommitted) {
		if !closureObservationMatches(c, current, models.InferenceClosureObserved) {
			return models.ErrInferenceClosureConflict
		}
		current, err = runtime.InferenceClosureStep(ctx, c.SessionID, c.SourceExecutionID, utility.InferenceClosureRequest{OperationID: c.OperationID, Action: "commit", ExpectedState: current.State})
		if err != nil {
			return err
		}
	}
	if !closureObservationMatches(c, current, models.InferenceClosureCommitted) {
		return models.ErrInferenceClosureConflict
	}
	next := *c
	next.Stage = models.InferenceClosureCommitted
	next.OwnerState = current.State
	next.Receipt, err = json.Marshal(current.Snapshot.Closure)
	if err != nil {
		return err
	}
	return advanceInferenceClosure(ctx, store, c, next)
}

func advanceInferenceClosure(ctx context.Context, store inferenceClosureStore, current *models.InferenceClosureCheckpoint, next models.InferenceClosureCheckpoint) error {
	prior, err := json.Marshal(current)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return err
	}
	changed, err := store.AdvanceInferenceClosure(ctx, current.SessionID, current.OperationID, prior, encoded)
	if err != nil {
		return err
	}
	if !changed {
		return models.ErrInferenceClosureConflict
	}
	*current = next
	return nil
}

func closureObservationMatches(c *models.InferenceClosureCheckpoint, observation *utility.InferenceClosureResponse, stage string) bool {
	if observation == nil || !observation.Snapshot.Known || len(observation.State) == 0 {
		return false
	}
	proof := observation.Snapshot.Closure
	return proof != nil && proof.Version == 1 && proof.Stage == stage && proof.HistoricalContinuityKnown &&
		proof.OperationID == c.OperationID && proof.SessionID == c.SessionID && proof.IncarnationID == c.IncarnationID &&
		proof.SourceInstanceID == c.SourceExecutionID && proof.CandidateInstanceID == c.CandidateExecutionID
}

func (s *Service) stopCommittedInferenceSource(ctx context.Context, c *models.InferenceClosureCheckpoint) error {
	current, err := s.agentManager.GetExecutionIDForSession(ctx, c.SessionID)
	if errors.Is(err, agentruntime.ErrNoExecutionForSession) {
		return nil
	}
	if err != nil {
		return err
	}
	if current == c.CandidateExecutionID {
		return nil
	}
	if current != c.SourceExecutionID {
		return models.ErrInferenceClosureConflict
	}
	_, err = s.StopSessionAwaited(ctx, c.SessionID, c.SourceExecutionID, "inference scope closure")
	return err
}
