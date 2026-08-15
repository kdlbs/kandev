package dynamic

import (
	"context"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

type EngineOption func(*Engine)

func WithClock(now func() time.Time) EngineOption {
	return func(engine *Engine) {
		if now != nil {
			engine.now = now
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
}

func NewEngine(options ...EngineOption) *Engine {
	engine := &Engine{
		now:      time.Now,
		circuits: NewCircuitRegistry(),
		states:   make(map[string]RouteState),
	}
	for _, option := range options {
		option(engine)
	}
	return engine
}

func (e *Engine) Circuits() *CircuitRegistry { return e.circuits }

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
		if !e.candidateSelectable(candidate, excludeProfileID, preferredProfileID, now) {
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

func (e *Engine) candidateSelectable(candidate Candidate, excludeProfileID, preferredProfileID string, now time.Time) bool {
	if !candidate.Enabled || candidate.ID == excludeProfileID {
		return false
	}
	if preferredProfileID != "" && candidate.ID != preferredProfileID {
		return false
	}
	return candidate.BindingKey == "" || !e.circuits.IsOpen(candidate.BindingKey, now)
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
