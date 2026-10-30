package orchestrator

import (
	"context"
	"errors"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/orchestrator/executor"
)

// AwaitedSessionStopResult separates accepted cancellation from observed teardown.
// Historical obligation resolution is deliberately unavailable in this result.
type AwaitedSessionStopResult struct {
	SessionID                string                            `json:"session_id"`
	ExecutionID              string                            `json:"execution_id"`
	Accepted                 bool                              `json:"accepted"`
	RuntimeStopReturned      bool                              `json:"runtime_stop_returned"`
	TeardownCompleted        bool                              `json:"teardown_completed"`
	RecordedTreeTerminal     bool                              `json:"recorded_tree_terminal"`
	LegacyObligationResolved bool                              `json:"legacy_obligation_resolved"`
	TargetChanged            bool                              `json:"target_changed"`
	Observation              *utility.InferenceStopObservation `json:"observation,omitempty"`
}

// StopSessionAwaited uses normal authorization and exact execution pinning.
func (s *Service) StopSessionAwaited(ctx context.Context, sessionID, expectedExecutionID, reason string) (*AwaitedSessionStopResult, error) {
	result := &AwaitedSessionStopResult{SessionID: sessionID, ExecutionID: expectedExecutionID}
	if expectedExecutionID == "" {
		return result, errors.New("expected_execution_id is required for awaited stop")
	}
	if err := s.authorizeSessionControl(ctx, sessionID); err != nil {
		return result, err
	}
	if s.executor == nil {
		return result, errors.New("session executor unavailable")
	}
	prepared, err := s.prepareAwaitedStop(ctx, sessionID, expectedExecutionID, reason)
	if err != nil {
		return result, err
	}
	result.Accepted = prepared.ExecutionID != ""
	if !result.Accepted {
		return result, executor.ErrExecutionNotFound
	}
	s.resetTransientRetryWithContext(ctx, sessionID, true)
	if s.lspLeases != nil {
		s.lspLeases.StopLSPLeasesForSession(sessionID)
	}
	collector := agentruntime.NewStopObservationCollector(expectedExecutionID)
	err = s.executor.StopExecution(agentruntime.WithStopObservation(ctx, collector), prepared.ExecutionID, reason, false)
	result.RuntimeStopReturned = err == nil
	result.Observation = collector.Result()
	result.applyObservation()
	if err != nil {
		return result, err
	}
	return s.finishAwaitedStop(ctx, result)
}

func (s *Service) finishAwaitedStop(ctx context.Context, result *AwaitedSessionStopResult) (*AwaitedSessionStopResult, error) {
	sessionID, expectedExecutionID := result.SessionID, result.ExecutionID
	if s.agentManager == nil {
		return result, errors.New("runtime identity observation unavailable")
	}
	current, lookupErr := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
	if lookupErr != nil && !errors.Is(lookupErr, agentruntime.ErrNoExecutionForSession) {
		return result, lookupErr
	}
	if current != "" && current != expectedExecutionID {
		result.TargetChanged = true
		return result, errors.New("session execution changed during awaited stop")
	}
	s.clearDynamicUnclassifiedStreakForStop(ctx, sessionID)
	s.settleBackgroundWork(ctx, sessionID, expectedExecutionID)
	return result, nil
}

func (s *Service) prepareAwaitedStop(ctx context.Context, sessionID, expectedExecutionID, reason string) (executor.SessionStopResult, error) {
	endCancel := s.beginCancelInFlight(sessionID)
	guard, release := s.acquireCancelInFlightGuard(sessionID)
	guard.Lock()
	result, err := s.executor.PrepareSessionStopForExecution(ctx, sessionID, expectedExecutionID, reason)
	guard.Unlock()
	release()
	endCancel()
	return result, err
}

func (r *AwaitedSessionStopResult) applyObservation() {
	if r.Observation == nil || r.Observation.ManagedStop == nil {
		return
	}
	receipt := r.Observation.ManagedStop
	r.TeardownCompleted = receipt.AdmissionClosed && receipt.OperationsDrained && receipt.ManagedProcessesStopped
	r.RecordedTreeTerminal = receipt.InferenceCensusKnown && receipt.RecordedChildrenTerminal
}
