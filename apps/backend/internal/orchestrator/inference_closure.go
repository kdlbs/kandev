package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

// StopForRestart uses closure only for configured shared inference, while an
// interrupted closure is always resumed even if configuration has changed.
func (s *Service) StopForRestart(ctx context.Context, sid string) (*models.InferenceClosureCheckpoint, bool, error) {
	all, finite := s.InferenceCapacityPolicy()
	return s.stopInferenceClosureForRestart(ctx, sid, all && finite)
}

func (s *Service) stopInferenceClosureForRestart(ctx context.Context, sid string, enabled bool) (*models.InferenceClosureCheckpoint, bool, error) {
	if err := s.authorizeSessionControl(ctx, sid); err != nil {
		return nil, true, err
	}
	store, ok := s.repo.(inferenceClosureStore)
	if !ok {
		return nil, false, nil
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
	prober, ok := s.agentManager.(interface {
		ProbeInferenceClosure(context.Context, string, string) error
	})
	if !ok {
		return nil, false, nil
	}
	executionID, err := s.agentManager.GetExecutionIDForSession(ctx, sid)
	if err != nil || executionID == "" {
		return nil, true, models.ErrInferenceClosureBlocked
	}
	if err = prober.ProbeInferenceClosure(ctx, sid, executionID); err != nil {
		if errors.Is(err, agentruntime.ErrInferenceOwnershipUnsupported) {
			return nil, false, nil
		}
		return nil, true, err
	}
	result, err := s.StopSessionForInferenceClosure(ctx, sid, executionID, uuid.NewString())
	return result, true, err
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

func (s *Service) beginInferenceClosure(ctx context.Context, store inferenceClosureStore, sid, executionID, operation string) (json.RawMessage, error) {
	// Session start/resume owns this lock through runtime admission. Take it
	// before the cancellation guard, matching the existing lifecycle order.
	releaseLifecycle, locked := s.acquireSessionLifecycleLockWithContext(ctx, sid)
	if !locked {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, models.ErrInferenceClosureConflict
	}
	defer releaseLifecycle()
	guard, err := s.lockCancelInFlightGuardWithContext(ctx, sid)
	if err != nil {
		return nil, err
	}
	defer guard.release()
	session, err := s.repo.GetTaskSession(ctx, sid)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, models.ErrInferenceClosureConflict
	}
	return store.BeginInferenceClosure(ctx, sid, operation, session.QueueIncarnationID, executionID)
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
