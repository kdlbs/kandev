package dynamic

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

type EngineOption func(*Engine)

func WithClock(now func() time.Time) EngineOption {
	return func(engine *Engine) {
		if now != nil {
			engine.now = now
			if engine.circuits != nil {
				engine.circuits.now = now
			}
		}
	}
}

func WithCircuitRegistry(registry *CircuitRegistry) EngineOption {
	return func(engine *Engine) {
		if registry != nil {
			engine.circuits = registry
		}
	}
}

func WithPersistence(persistence Persistence) EngineOption {
	return func(engine *Engine) {
		engine.persistence = persistence
	}
}

type Engine struct {
	mu          sync.Mutex
	now         func() time.Time
	circuits    *CircuitRegistry
	states      map[string]RouteState
	persistence Persistence
	loader      StateLoader
	probes      map[string]ProbeLease
}

func NewEngine(options ...EngineOption) *Engine {
	engine := &Engine{
		now:      time.Now,
		circuits: NewCircuitRegistry(),
		states:   make(map[string]RouteState),
		probes:   make(map[string]ProbeLease),
	}
	for _, option := range options {
		option(engine)
	}
	return engine
}

func (e *Engine) Circuits() *CircuitRegistry { return e.circuits }

// ContinuationPersistence exposes the durable handoff seam without exposing
// the engine's other persistence responsibilities to the conductor.
func (e *Engine) ContinuationPersistence() ContinuationPersistence {
	if e == nil || e.persistence == nil {
		return nil
	}
	persistence, _ := e.persistence.(ContinuationPersistence)
	return persistence
}

// WithStateLoader enables restart recovery for sessions that are not already
// present in the process-local state map.
func WithStateLoader(loader StateLoader) EngineOption {
	return func(engine *Engine) { engine.loader = loader }
}

// Select claims one route generation. The mutex protects the in-memory CAS;
// callers that need restart durability provide the same state through the
// task repository integration before invoking the engine again.
func (e *Engine) Select(sessionID string, profile Profile, expectedGeneration int64, excludeProfileID string) (RouteDecision, error) {
	return e.SelectContext(context.Background(), sessionID, profile, expectedGeneration, excludeProfileID)
}

func (e *Engine) SelectContext(ctx context.Context, sessionID string, profile Profile, expectedGeneration int64, excludeProfileID string) (RouteDecision, error) {
	return e.selectContext(ctx, sessionID, profile, expectedGeneration, excludeProfileID, "")
}

// SelectContextWithPreference retries the requested candidate when it remains
// eligible. A caller can still exclude that candidate for try-next behavior
// by using the ordinary SelectContext method.
func (e *Engine) SelectContextWithPreference(
	ctx context.Context,
	sessionID string,
	profile Profile,
	expectedGeneration int64,
	excludeProfileID string,
	preferredProfileID string,
) (RouteDecision, error) {
	return e.selectContext(ctx, sessionID, profile, expectedGeneration, excludeProfileID, preferredProfileID)
}

func (e *Engine) selectContext(
	ctx context.Context,
	sessionID string,
	profile Profile,
	expectedGeneration int64,
	excludeProfileID string,
	preferredProfileID string,
) (RouteDecision, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	state, exists, err := e.loadStateLocked(ctx, sessionID)
	if err != nil {
		return RouteDecision{}, err
	}
	currentGeneration := int64(0)
	if exists {
		currentGeneration = state.Generation
	}
	if expectedGeneration != currentGeneration {
		return RouteDecision{}, ErrStaleGeneration
	}
	generation := currentGeneration + 1
	now := e.now()
	for _, candidate := range profile.Candidates {
		if !e.candidateSelectable(candidate, sessionID, generation, excludeProfileID, preferredProfileID, now) {
			continue
		}
		decision := RouteDecision{
			SessionID:          sessionID,
			LogicalProfileID:   profile.ID,
			ExecutionProfileID: candidate.ID,
			Generation:         generation,
			ProfileVersion:     profile.Version,
			Reason:             "candidate_order",
		}
		nextState := RouteState{
			SessionID:          sessionID,
			LogicalProfileID:   profile.ID,
			ExecutionProfileID: candidate.ID,
			Generation:         generation,
			ProfileVersion:     profile.Version,
			Status:             "starting",
			UpdatedAt:          now,
		}
		if err := e.claimAndPersist(ctx, expectedGeneration, decision, nextState); err != nil {
			delete(e.states, sessionID)
			return RouteDecision{}, err
		}
		e.states[sessionID] = nextState
		return decision, nil
	}
	nextState := RouteState{
		SessionID:        sessionID,
		LogicalProfileID: profile.ID,
		Generation:       generation,
		ProfileVersion:   profile.Version,
		Status:           "waiting",
		UpdatedAt:        now,
	}
	if err := e.persistNoEligible(ctx, expectedGeneration, nextState); err != nil {
		delete(e.states, sessionID)
		return RouteDecision{}, err
	}
	e.states[sessionID] = nextState
	return RouteDecision{}, &NoEligibleCandidateError{
		SessionID: sessionID, LogicalProfile: profile.ID, Generation: generation,
	}
}

func (e *Engine) loadStateLocked(ctx context.Context, sessionID string) (RouteState, bool, error) {
	state, exists := e.states[sessionID]
	if exists || e.loader == nil {
		return state, exists, nil
	}
	loaded, err := e.loader.LoadRouteState(ctx, sessionID)
	if err != nil {
		return RouteState{}, false, err
	}
	if loaded == nil {
		return RouteState{}, false, nil
	}
	e.states[sessionID] = *loaded
	return *loaded, true, nil
}

const probeLeaseDuration = 30 * time.Second

func (e *Engine) candidateSelectable(candidate Candidate, sessionID string, generation int64, excludeProfileID, preferredProfileID string, now time.Time) bool {
	if !candidate.Enabled || candidate.ID == excludeProfileID {
		return false
	}
	if preferredProfileID != "" && candidate.ID != preferredProfileID {
		return false
	}
	if candidate.BindingKey == "" || !e.circuits.IsOpen(candidate.BindingKey, now) {
		return true
	}
	lease, ok := e.circuits.AcquireProbe(candidate.BindingKey, probeLeaseDuration)
	if !ok {
		return false
	}
	// The caller owns the generation fencing. The conductor releases this
	// lease after the concrete launch result is known.
	e.probes[probeKey(sessionID, generation, candidate.ID)] = lease
	return true
}

func probeKey(sessionID string, generation int64, candidateID string) string {
	return sessionID + ":" + fmt.Sprint(generation) + ":" + candidateID
}

func (e *Engine) persistNoEligible(ctx context.Context, expectedGeneration int64, state RouteState) error {
	if e.persistence == nil {
		return nil
	}
	claimer, ok := e.persistence.(GenerationClaimer)
	if !ok {
		return e.persistence.SaveRouteState(ctx, state)
	}
	claimed, err := claimer.ClaimRouteState(ctx, expectedGeneration, state)
	if err != nil {
		return err
	}
	if !claimed {
		return ErrStaleGeneration
	}
	return nil
}

func (e *Engine) persist(ctx context.Context, decision RouteDecision, state RouteState) error {
	if e.persistence == nil {
		return nil
	}
	if err := e.persistence.SaveRouteState(ctx, state); err != nil {
		return err
	}
	return e.persistence.AppendRouteAttempt(ctx, RouteAttempt{
		SessionID:          decision.SessionID,
		LogicalProfileID:   decision.LogicalProfileID,
		ExecutionProfileID: decision.ExecutionProfileID,
		Generation:         decision.Generation,
		ProfileVersion:     decision.ProfileVersion,
		Reason:             decision.Reason,
		CreatedAt:          state.UpdatedAt,
	})
}

func (e *Engine) claimAndPersist(ctx context.Context, expectedGeneration int64, decision RouteDecision, state RouteState) error {
	if e.persistence == nil {
		return nil
	}
	if recorder, ok := e.persistence.(DecisionRecorder); ok {
		return recorder.RecordRouteDecision(ctx, decision, state)
	}
	if claimer, ok := e.persistence.(GenerationClaimer); ok {
		claimed, err := claimer.ClaimRouteState(ctx, expectedGeneration, state)
		if err != nil {
			return err
		}
		if !claimed {
			return ErrStaleGeneration
		}
		return e.persistence.AppendRouteAttempt(ctx, RouteAttempt{
			SessionID: decision.SessionID, LogicalProfileID: decision.LogicalProfileID,
			ExecutionProfileID: decision.ExecutionProfileID, Generation: decision.Generation,
			ProfileVersion: decision.ProfileVersion, Reason: decision.Reason,
			CreatedAt: state.UpdatedAt,
		})
	}
	return e.persist(ctx, decision, state)
}

func (e *Engine) State(sessionID string) (RouteState, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	state, ok := e.states[sessionID]
	return state, ok
}

func (e *Engine) ActionFor(profile Profile, candidateID string, code routingerr.Code) Action {
	for _, candidate := range profile.Candidates {
		if candidate.ID == candidateID {
			return actionForRules(candidate.Rules, code)
		}
	}
	return ActionStop
}

// ApplyFailure translates a classified error into the next generation. A
// retry keeps the same candidate; try_next excludes it for this decision only.
func (e *Engine) ApplyFailure(
	sessionID string,
	profile Profile,
	expectedGeneration int64,
	currentCandidateID string,
	failure *routingerr.Error,
) (RouteDecision, error) {
	return e.ApplyFailureContext(context.Background(), sessionID, profile, expectedGeneration, currentCandidateID, failure)
}

// ApplyFailureContext applies a classified failure and persists the next
// route decision using the supplied context.
func (e *Engine) ApplyFailureContext(
	ctx context.Context,
	sessionID string,
	profile Profile,
	expectedGeneration int64,
	currentCandidateID string,
	failure *routingerr.Error,
) (RouteDecision, error) {
	if failure == nil {
		return RouteDecision{}, ErrNoEligibleCandidate
	}
	e.openCircuitForFailure(profile, currentCandidateID, failure)
	e.releaseProbeForFailure(sessionID, expectedGeneration, currentCandidateID)
	action := e.ActionFor(profile, currentCandidateID, failure.Code)
	switch action {
	case ActionRetrySame:
		return e.selectContext(ctx, sessionID, profile, expectedGeneration, "", currentCandidateID)
	case ActionTryNext:
		return e.selectContext(ctx, sessionID, profile, expectedGeneration, currentCandidateID, "")
	default:
		return RouteDecision{}, ErrNoEligibleCandidate
	}
}

// ReleaseProbe closes a successful half-open resource probe or applies the
// standard failure backoff. It is safe to call when the selected candidate did
// not hold a probe lease.
func (e *Engine) ReleaseProbe(decision RouteDecision, success bool) {
	if e == nil || e.circuits == nil {
		return
	}
	e.mu.Lock()
	key := probeKey(decision.SessionID, decision.Generation, decision.ExecutionProfileID)
	lease, ok := e.probes[key]
	if ok {
		delete(e.probes, key)
	}
	e.mu.Unlock()
	if ok {
		e.circuits.ReleaseProbe(lease, success, circuitBackoff)
	}
}

func (e *Engine) releaseProbeForFailure(sessionID string, generation int64, candidateID string) {
	e.ReleaseProbe(RouteDecision{SessionID: sessionID, Generation: generation, ExecutionProfileID: candidateID}, false)
}

func (e *Engine) openCircuitForFailure(profile Profile, candidateID string, failure *routingerr.Error) {
	if failure == nil || e.circuits == nil || !qualifiesForCircuit(failure.Code) {
		return
	}
	for _, candidate := range profile.Candidates {
		if candidate.ID != candidateID || candidate.BindingKey == "" {
			continue
		}
		until := e.now().Add(circuitBackoff)
		if failure.ResetHint != nil && failure.ResetHint.After(until) {
			until = *failure.ResetHint
		}
		e.circuits.Open(candidate.BindingKey, until, failure.Code)
		return
	}
}

const circuitBackoff = time.Minute

func qualifiesForCircuit(code routingerr.Code) bool {
	switch code {
	case routingerr.CodeAuthRequired, routingerr.CodeMissingCredentials,
		routingerr.CodeSubscriptionRequired, routingerr.CodeProviderNotConfigured,
		routingerr.CodeRateLimited, routingerr.CodeQuotaLimited,
		routingerr.CodeNetworkUnavailable, routingerr.CodeModelCapacity,
		routingerr.CodeProviderUnavailable, routingerr.CodeProviderOverloaded,
		routingerr.CodeModelUnavailable:
		return true
	default:
		return false
	}
}
