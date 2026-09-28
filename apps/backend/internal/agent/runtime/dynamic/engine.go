package dynamic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agent/runtime/routingpolicy"
)

type EngineOption func(*Engine)

const (
	routeStatusStarting       = "starting"
	routeStatusActive         = "active"
	routeStatusActionRequired = "action_required"
	routeStatusRetrying       = "retrying"
)

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
	// retryClaims identifies retry launches owned by this process. A durable
	// "retrying" state can survive a restart without its in-memory owner, so
	// manual recovery may reclaim it only when this map does not contain the
	// same generation.
	retryClaims map[string]int64
}

func NewEngine(options ...EngineOption) *Engine {
	engine := &Engine{
		now:         time.Now,
		circuits:    NewCircuitRegistry(),
		states:      make(map[string]RouteState),
		probes:      make(map[string]ProbeLease),
		retryClaims: make(map[string]int64),
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
	return e.selectContext(ctx, sessionID, profile, expectedGeneration, excludeProfileID, "", "candidate_order")
}

// SelectContextWithReason is used by generation-fenced manual route actions
// so the immutable attempt row records why a candidate was selected.
func (e *Engine) SelectContextWithReason(
	ctx context.Context,
	sessionID string,
	profile Profile,
	expectedGeneration int64,
	excludeProfileID string,
	reason string,
) (RouteDecision, error) {
	return e.selectContext(ctx, sessionID, profile, expectedGeneration, excludeProfileID, "", reason)
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
	return e.selectContext(ctx, sessionID, profile, expectedGeneration, excludeProfileID, preferredProfileID, "candidate_order")
}

func (e *Engine) selectContext(
	ctx context.Context,
	sessionID string,
	profile Profile,
	expectedGeneration int64,
	excludeProfileID string,
	preferredProfileID string,
	reason string,
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
			Reason:             reason,
			Status:             routeStatusStarting,
		}
		nextState := RouteState{
			SessionID:          sessionID,
			LogicalProfileID:   profile.ID,
			ExecutionProfileID: candidate.ID,
			Generation:         generation,
			ProfileVersion:     profile.Version,
			Status:             routeStatusStarting,
			UpdatedAt:          now,
		}
		if exists && state.Status == routeStatusActionRequired &&
			state.LogicalProfileID == profile.ID && state.ExecutionProfileID == candidate.ID &&
			state.ProfileVersion == profile.Version {
			nextState.PolicyStateJSON = carryUnclassifiedStreak(state.PolicyStateJSON, profile.ID, candidate.ID, profile.Version, candidate.Policies.Unclassified)
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

// LoadState returns the current route state and hydrates it from the durable
// loader when this process has not handled the session before.
func (e *Engine) LoadState(ctx context.Context, sessionID string) (RouteState, bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.loadStateLocked(ctx, sessionID)
}

// OwnsRetryClaim reports whether this process still owns a retry launch for
// the session generation. The durable route row alone cannot answer this
// after a restart because the previous process owner is gone.
func (e *Engine) OwnsRetryClaim(sessionID string, generation int64) bool {
	if e == nil || sessionID == "" || generation <= 0 {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.retryClaims[sessionID] == generation
}

func (e *Engine) ActionFor(profile Profile, candidateID string, code routingerr.Code) Action {
	for _, candidate := range profile.Candidates {
		if candidate.ID != candidateID {
			continue
		}
		return actionForCandidate(candidate, code)
	}
	return ActionStop
}

func actionForCandidate(candidate Candidate, code routingerr.Code) Action {
	if candidate.Policies.Version == 0 {
		return actionForRules(candidate.Rules, code)
	}
	class := routingerr.ClassForCode(code)
	policy, ok := candidate.Policies.PolicyFor(class)
	if !ok {
		return ActionStop
	}
	if policy.Retry.Enabled {
		return ActionRetrySame
	}
	if policy.OnExhausted == routingpolicy.OutcomeSkip {
		return ActionTryNext
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
	if candidate, ok := candidateByID(profile, currentCandidateID); ok && candidate.Policies.Version != 0 {
		return e.applyPolicyFailure(ctx, sessionID, profile, expectedGeneration, currentCandidateID, candidate, failure)
	}
	action := e.ActionFor(profile, currentCandidateID, failure.Code)
	switch action {
	case ActionRetrySame:
		return e.selectContext(ctx, sessionID, profile, expectedGeneration, "", currentCandidateID, "retry")
	case ActionTryNext:
		return e.selectContext(ctx, sessionID, profile, expectedGeneration, currentCandidateID, "", "try_next")
	default:
		return RouteDecision{}, ErrNoEligibleCandidate
	}
}

// ApplyUnclassifiedFailureContext applies the repeated-identical-failure
// exception only when current task-owned evidence proves the narrow policy's
// admission conditions. Ordinary callers must continue to use
// ApplyFailureContext, which keeps unclassified failures fail-closed.
func (e *Engine) ApplyUnclassifiedFailureContext(
	ctx context.Context,
	sessionID string,
	profile Profile,
	expectedGeneration int64,
	currentCandidateID string,
	failure *routingerr.Error,
	evidence UnclassifiedFailureEvidence,
) (RouteDecision, error) {
	if e == nil || sessionID == "" || failure == nil ||
		!evidence.currentFor(sessionID, profile, currentCandidateID, expectedGeneration) {
		return RouteDecision{}, ErrStaleGeneration
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	state, err := e.currentUnclassifiedRouteState(ctx, sessionID, profile, expectedGeneration, currentCandidateID)
	if err != nil {
		return RouteDecision{}, err
	}
	policyState := decodePolicyState(state.PolicyStateJSON)
	if previous := policyState.Unclassified; previous.valid() && previous.LastAttemptID == evidence.AttemptID {
		return unclassifiedStreakDecision(state, previous), ErrRecoveryPending
	}

	candidate, threshold := candidateUnclassifiedThreshold(profile, currentCandidateID)
	fingerprint, fingerprintErr := unclassifiedFailureFingerprint(evidence, failure)
	if !unclassifiedFailureAdmitted(threshold, evidence, failure, fingerprintErr) {
		policyState.Unclassified = nil
		clearUnclassifiedFailureState(&policyState, failure)
		return e.persistUnclassifiedManual(ctx, state, state.PolicyStateJSON, policyState, failure, "unclassified_manual_recovery")
	}

	count := nextUnclassifiedFailureCount(policyState.Unclassified, profile, currentCandidateID, evidence, expectedGeneration, fingerprint, threshold)
	policyState.Unclassified = newUnclassifiedStreak(count, profile, currentCandidateID, evidence, expectedGeneration, fingerprint)
	clearUnclassifiedFailureState(&policyState, failure)
	if err := saveUnclassifiedPolicyJSON(&policyState, candidate.Policies); err != nil {
		return RouteDecision{}, err
	}
	if count < threshold {
		return e.persistUnclassifiedManual(ctx, state, state.PolicyStateJSON, policyState, failure, "unclassified_manual_recovery")
	}
	return e.routeToUnclassifiedSuccessor(
		ctx, state, profile, currentCandidateID, expectedGeneration, failure, evidence, policyState,
	)
}

func (e *Engine) currentUnclassifiedRouteState(
	ctx context.Context,
	sessionID string,
	profile Profile,
	expectedGeneration int64,
	currentCandidateID string,
) (RouteState, error) {
	state, exists, err := e.loadStateLocked(ctx, sessionID)
	if err != nil {
		return RouteState{}, err
	}
	if !exists || state.Generation != expectedGeneration || state.LogicalProfileID != profile.ID ||
		state.ExecutionProfileID != currentCandidateID || state.ProfileVersion != profile.Version {
		return RouteState{}, ErrStaleGeneration
	}
	if state.Status != routeStatusActive && state.Status != routeStatusStarting &&
		state.Status != routeStatusRetrying && state.Status != routeStatusActionRequired {
		return RouteState{}, ErrRecoveryPending
	}
	return state, nil
}

func decodePolicyState(raw string) PolicyState {
	var state PolicyState
	if raw != "" && json.Unmarshal([]byte(raw), &state) != nil {
		return PolicyState{}
	}
	return state
}

func candidateUnclassifiedThreshold(profile Profile, candidateID string) (Candidate, int64) {
	candidate, found := candidateByID(profile, candidateID)
	if !found || candidate.Policies.Unclassified == nil || !candidate.Policies.Unclassified.Enabled {
		return candidate, 0
	}
	return candidate, candidate.Policies.Unclassified.ConsecutiveFailureThreshold
}

func unclassifiedFailureAdmitted(
	threshold int64,
	evidence UnclassifiedFailureEvidence,
	failure *routingerr.Error,
	fingerprintErr error,
) bool {
	return threshold >= 2 && threshold <= 10 && evidence.permits(failure) && fingerprintErr == nil
}

func nextUnclassifiedFailureCount(
	previous *UnclassifiedStreak,
	profile Profile,
	candidateID string,
	evidence UnclassifiedFailureEvidence,
	generation int64,
	fingerprint string,
	threshold int64,
) int64 {
	count := int64(1)
	if previous.valid() && previous.LogicalProfileID == profile.ID &&
		previous.ExecutionProfileID == candidateID && previous.ProfileVersion == profile.Version &&
		previous.StepID == evidence.StepID && previous.StepUpdatedAt.Equal(evidence.StepUpdatedAt) &&
		previous.Fingerprint == fingerprint &&
		previous.Origin == evidence.Origin && previous.Phase == evidence.Phase &&
		previous.LastRouteGeneration <= generation {
		count = previous.Count + 1
	}
	if count > threshold {
		return threshold
	}
	return count
}

func newUnclassifiedStreak(
	count int64,
	profile Profile,
	candidateID string,
	evidence UnclassifiedFailureEvidence,
	generation int64,
	fingerprint string,
) *UnclassifiedStreak {
	return &UnclassifiedStreak{
		Version: 1, LogicalProfileID: profile.ID, ExecutionProfileID: candidateID,
		ProfileVersion: profile.Version, StepID: evidence.StepID, StepUpdatedAt: evidence.StepUpdatedAt,
		Fingerprint: fingerprint,
		Origin:      evidence.Origin, Phase: evidence.Phase, Count: count,
		LastAttemptID: evidence.AttemptID, LastRouteGeneration: generation,
	}
}

func clearUnclassifiedFailureState(state *PolicyState, failure *routingerr.Error) {
	state.FailureCode = failure.Code
	state.FailureClass = routingerr.ClassUnclassified
	state.CatalogueVersion = routingerr.CatalogueVersion
	state.PendingOutcome = routingpolicy.OutcomeStop
}

func saveUnclassifiedPolicyJSON(state *PolicyState, policies routingpolicy.Document) error {
	policyJSON, err := json.Marshal(policies)
	if err != nil {
		return err
	}
	state.PolicyJSON = string(policyJSON)
	return nil
}

func (e *Engine) routeToUnclassifiedSuccessor(
	ctx context.Context,
	state RouteState,
	profile Profile,
	currentCandidateID string,
	expectedGeneration int64,
	failure *routingerr.Error,
	evidence UnclassifiedFailureEvidence,
	policyState PolicyState,
) (RouteDecision, error) {
	successor, found := e.firstSelectableUnclassifiedSuccessor(profile, currentCandidateID, state.SessionID, expectedGeneration)
	if !found {
		return e.persistUnclassifiedManual(ctx, state, state.PolicyStateJSON, policyState, failure, "unclassified_candidates_exhausted")
	}
	decision, nextState := unclassifiedSuccessorRoute(state, profile, successor, expectedGeneration, failure, e.now())
	if persistedDecision, err := e.recordUnclassifiedSuccessor(
		ctx, expectedGeneration, decision, nextState, evidence, &policyState, state, failure,
	); err != nil {
		return persistedDecision, err
	}
	e.states[state.SessionID] = nextState
	delete(e.retryClaims, state.SessionID)
	return decision, nil
}

func (e *Engine) firstSelectableUnclassifiedSuccessor(
	profile Profile,
	currentCandidateID, sessionID string,
	generation int64,
) (Candidate, bool) {
	for index, item := range profile.Candidates {
		if item.ID != currentCandidateID {
			continue
		}
		for _, candidate := range profile.Candidates[index+1:] {
			if e.candidateSelectable(candidate, sessionID, generation+1, "", "", e.now()) {
				return candidate, true
			}
		}
		break
	}
	return Candidate{}, false
}

func unclassifiedSuccessorRoute(
	state RouteState,
	profile Profile,
	successor Candidate,
	expectedGeneration int64,
	failure *routingerr.Error,
	updatedAt time.Time,
) (RouteDecision, RouteState) {
	generation := expectedGeneration + 1
	decision := RouteDecision{
		SessionID: state.SessionID, LogicalProfileID: profile.ID,
		ExecutionProfileID: successor.ID, Generation: generation,
		ProfileVersion: profile.Version, Reason: "policy_skip",
		Status: routeStatusStarting, ErrorCode: failure.Code,
		ErrorClass:       routingerr.ClassUnclassified,
		CatalogueVersion: routingerr.CatalogueVersion,
		PendingOutcome:   routingpolicy.OutcomeSkip,
	}
	nextState := RouteState{
		SessionID: state.SessionID, LogicalProfileID: profile.ID,
		ExecutionProfileID: successor.ID, Generation: generation,
		ProfileVersion: profile.Version, Status: routeStatusStarting,
		UpdatedAt: updatedAt,
	}
	return decision, nextState
}

func (e *Engine) recordUnclassifiedSuccessor(
	ctx context.Context,
	expectedGeneration int64,
	decision RouteDecision,
	nextState RouteState,
	evidence UnclassifiedFailureEvidence,
	policyState *PolicyState,
	currentState RouteState,
	failure *routingerr.Error,
) (RouteDecision, error) {
	if e.persistence == nil {
		return decision, e.claimAndPersist(ctx, expectedGeneration, decision, nextState)
	}
	recorder, ok := e.persistence.(UnclassifiedRouteDecisionRecorder)
	if !ok {
		return e.persistUnclassifiedDecisionFailure(ctx, currentState, *policyState, failure, "unclassified_workflow_context_unavailable")
	}
	if err := recorder.RecordUnclassifiedRouteDecision(ctx, decision, nextState, evidence); err != nil {
		if errors.Is(err, ErrStaleGeneration) {
			return RouteDecision{}, err
		}
		return e.persistUnclassifiedDecisionFailure(ctx, currentState, *policyState, failure, workflowContextFailureReason(err))
	}
	return decision, nil
}

func (e *Engine) persistUnclassifiedDecisionFailure(
	ctx context.Context,
	state RouteState,
	policyState PolicyState,
	failure *routingerr.Error,
	reason string,
) (RouteDecision, error) {
	policyState.Unclassified = nil
	return e.persistUnclassifiedManual(ctx, state, state.PolicyStateJSON, policyState, failure, reason)
}

func workflowContextFailureReason(err error) string {
	if errors.Is(err, ErrUnclassifiedWorkflowContextChanged) {
		return "unclassified_workflow_context_changed"
	}
	return "unclassified_workflow_context_unavailable"
}

// ClaimUnclassifiedFallbackLaunch revalidates the task's workflow context at
// the automatic successor's final launch boundary. Durable task repositories
// must implement the transactional claim; a weaker read cannot authorize the
// launch.
func (e *Engine) ClaimUnclassifiedFallbackLaunch(
	ctx context.Context,
	decision RouteDecision,
	evidence UnclassifiedFailureEvidence,
) error {
	if e == nil || e.persistence == nil {
		return ErrUnclassifiedWorkflowContextUnavailable
	}
	claimer, ok := e.persistence.(UnclassifiedFallbackLaunchClaimer)
	if !ok {
		return ErrUnclassifiedWorkflowContextUnavailable
	}
	return claimer.ClaimUnclassifiedFallbackLaunch(ctx, decision, evidence)
}

func (e *Engine) persistUnclassifiedManual(
	ctx context.Context,
	state RouteState,
	expectedPolicyState string,
	policyState PolicyState,
	failure *routingerr.Error,
	reason string,
) (RouteDecision, error) {
	oldState := state.Status
	state.Status = routeStatusActionRequired
	state.PolicyStateJSON = string(mustJSON(policyState))
	state.UpdatedAt = e.now()
	if err := e.persistSnapshot(ctx, oldState, expectedPolicyState, state); err != nil {
		return RouteDecision{}, err
	}
	e.states[state.SessionID] = state
	decision := RouteDecision{
		SessionID: state.SessionID, LogicalProfileID: state.LogicalProfileID,
		ExecutionProfileID: state.ExecutionProfileID, Generation: state.Generation,
		ProfileVersion: state.ProfileVersion, Reason: reason, Status: state.Status,
		ErrorCode: failure.Code, ErrorClass: routingerr.ClassUnclassified,
		CatalogueVersion: routingerr.CatalogueVersion, PendingOutcome: routingpolicy.OutcomeStop,
	}
	return decision, fmt.Errorf("%w: %s", ErrRecoveryPending, reason)
}

func (e *Engine) persistSnapshot(ctx context.Context, expectedStatus, expectedPolicyState string, next RouteState) error {
	if e.persistence == nil {
		return nil
	}
	claimer, ok := e.persistence.(RouteStateSnapshotClaimer)
	if !ok {
		return ErrStatusClaimUnsupported
	}
	claimed, err := claimer.ClaimRouteStateFromSnapshot(ctx, next.Generation, expectedStatus, expectedPolicyState, next)
	if err != nil {
		return err
	}
	if !claimed {
		return ErrStaleGeneration
	}
	return nil
}

func unclassifiedStreakDecision(state RouteState, streak *UnclassifiedStreak) RouteDecision {
	return RouteDecision{
		SessionID: state.SessionID, LogicalProfileID: state.LogicalProfileID,
		ExecutionProfileID: state.ExecutionProfileID, Generation: state.Generation,
		ProfileVersion: state.ProfileVersion, Reason: "unclassified_duplicate",
		Status: state.Status, ErrorCode: routingerr.CodeUnknownProvider,
		ErrorClass: routingerr.ClassUnclassified, RetryOrdinal: streak.Count,
	}
}

func carryUnclassifiedStreak(
	raw, logicalProfileID, executionProfileID string,
	profileVersion int64,
	policy *routingpolicy.UnclassifiedPolicy,
) string {
	var previous PolicyState
	if policy == nil || !policy.Enabled || policy.ConsecutiveFailureThreshold < 2 ||
		policy.ConsecutiveFailureThreshold > 10 ||
		json.Unmarshal([]byte(raw), &previous) != nil || !previous.Unclassified.valid() ||
		previous.Unclassified.LogicalProfileID != logicalProfileID ||
		previous.Unclassified.ExecutionProfileID != executionProfileID ||
		previous.Unclassified.ProfileVersion != profileVersion ||
		previous.Unclassified.Count > policy.ConsecutiveFailureThreshold {
		return ""
	}
	return string(mustJSON(PolicyState{Unclassified: previous.Unclassified}))
}

func candidateByID(profile Profile, candidateID string) (Candidate, bool) {
	for _, candidate := range profile.Candidates {
		if candidate.ID == candidateID {
			return candidate, true
		}
	}
	return Candidate{}, false
}

func (e *Engine) applyPolicyFailure(
	ctx context.Context,
	sessionID string,
	profile Profile,
	expectedGeneration int64,
	currentCandidateID string,
	candidate Candidate,
	failure *routingerr.Error,
) (RouteDecision, error) {
	state, exists, policyState, evaluation, now, err := e.preparePolicyFailure(
		ctx, sessionID, expectedGeneration, candidate, failure,
	)
	if err != nil {
		return RouteDecision{}, err
	}

	if evaluation.Kind == routingpolicy.DecisionSkip {
		return e.selectContext(ctx, sessionID, profile, expectedGeneration, currentCandidateID, "", "policy_skip")
	}
	nextState := state
	if !exists {
		nextState = RouteState{
			SessionID:          sessionID,
			LogicalProfileID:   profile.ID,
			ExecutionProfileID: currentCandidateID,
			Generation:         expectedGeneration,
			ProfileVersion:     profile.Version,
		}
	}
	nextState.Status = string(evaluation.Kind)
	nextState.PolicyStateJSON = string(mustJSON(policyState))
	nextState.UpdatedAt = now
	if evaluation.Kind == routingpolicy.DecisionStop {
		nextState.Status = routeStatusActionRequired
	}
	if err := e.persistSameGeneration(ctx, expectedGeneration, state.Status, nextState); err != nil {
		return RouteDecision{}, err
	}
	e.mu.Lock()
	e.states[sessionID] = nextState
	e.mu.Unlock()
	var deadline *time.Time
	if !evaluation.Deadline.IsZero() {
		value := evaluation.Deadline
		deadline = &value
	}
	decision := RouteDecision{
		SessionID: sessionID, LogicalProfileID: profile.ID,
		ExecutionProfileID: currentCandidateID, Generation: expectedGeneration,
		ProfileVersion: profile.Version, Reason: string(evaluation.Kind),
		Status: nextState.Status, Deadline: deadline,
		ErrorCode: evaluation.Code, ErrorClass: evaluation.Class,
		CatalogueVersion: evaluation.CatalogueVersion,
		RetryOrdinal:     evaluation.RetryOrdinal, PendingOutcome: evaluation.PendingOutcome,
	}
	if evaluation.Kind == routingpolicy.DecisionStop {
		return decision, ErrNoEligibleCandidate
	}
	return decision, fmt.Errorf("%w: %s", ErrRecoveryPending, evaluation.Kind)
}

func (e *Engine) preparePolicyFailure(
	ctx context.Context,
	sessionID string,
	expectedGeneration int64,
	candidate Candidate,
	failure *routingerr.Error,
) (RouteState, bool, PolicyState, routingpolicy.Evaluation, time.Time, error) {
	state, exists, err := e.stateForFailure(ctx, sessionID)
	if err != nil {
		return RouteState{}, false, PolicyState{}, routingpolicy.Evaluation{}, time.Time{}, err
	}
	if exists && state.Generation != expectedGeneration {
		return RouteState{}, false, PolicyState{}, routingpolicy.Evaluation{}, time.Time{}, ErrStaleGeneration
	}
	policyState := PolicyState{}
	if exists && state.PolicyStateJSON != "" {
		if err := json.Unmarshal([]byte(state.PolicyStateJSON), &policyState); err != nil {
			return RouteState{}, false, PolicyState{}, routingpolicy.Evaluation{}, time.Time{}, fmt.Errorf("decode dynamic policy state: %w", err)
		}
	}
	policyState.Unclassified = nil
	now := e.now()
	failureClass := failure.Class
	if failureClass == "" {
		failureClass = routingerr.ClassForCode(failure.Code)
	}
	resetWaitUsed := policyState.ResetWaitUsed && policyState.FailureClass == failureClass
	if policyState.ResetWaitClasses != nil {
		resetWaitUsed = policyState.ResetWaitClasses[failureClass]
	}
	evaluation := routingpolicy.Evaluate(candidate.Policies, routingpolicy.EvaluationInput{
		Failure: failure, Now: now, RetryOrdinal: policyState.RetryOrdinal,
		ResetWaitUsed: resetWaitUsed, EffectSafe: failure.FallbackAllowed,
	})
	policyJSON, err := json.Marshal(candidate.Policies)
	if err != nil {
		return RouteState{}, false, PolicyState{}, routingpolicy.Evaluation{}, time.Time{}, fmt.Errorf("encode dynamic policy snapshot: %w", err)
	}
	policyState.FailureCode = evaluation.Code
	policyState.FailureClass = evaluation.Class
	policyState.CatalogueVersion = evaluation.CatalogueVersion
	policyState.PolicyJSON = string(policyJSON)
	policyState.RetryOrdinal = evaluation.RetryOrdinal
	if policyState.ResetWaitClasses == nil {
		policyState.ResetWaitClasses = make(map[routingerr.Class]bool)
	}
	if evaluation.Kind == routingpolicy.DecisionWaitForReset {
		policyState.ResetWaitClasses[evaluation.Class] = true
	}
	policyState.ResetWaitUsed = policyState.ResetWaitClasses[evaluation.Class]
	policyState.PendingOutcome = evaluation.PendingOutcome
	if evaluation.Deadline.IsZero() {
		policyState.Deadline = nil
	} else {
		deadline := evaluation.Deadline
		policyState.Deadline = &deadline
	}
	return state, exists, policyState, evaluation, now, nil
}

func (e *Engine) stateForFailure(ctx context.Context, sessionID string) (RouteState, bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.loadStateLocked(ctx, sessionID)
}

func (e *Engine) persistSameGeneration(
	ctx context.Context,
	expectedGeneration int64,
	expectedStatus string,
	state RouteState,
) error {
	if e.persistence == nil {
		return nil
	}
	if handled, err := e.persistInitialGeneration(ctx, expectedGeneration, expectedStatus, state); handled {
		return err
	}
	if claimer, ok := e.persistence.(GenerationStatusClaimer); ok {
		claimed, err := claimer.ClaimRouteStateFrom(ctx, expectedGeneration, expectedStatus, state)
		if err != nil {
			return err
		}
		if !claimed {
			return ErrStaleGeneration
		}
		return nil
	}
	if claimer, ok := e.persistence.(GenerationClaimer); ok {
		claimed, err := claimer.ClaimRouteState(ctx, expectedGeneration, state)
		if err != nil {
			return err
		}
		if !claimed {
			return ErrStaleGeneration
		}
		return nil
	}
	return e.persistence.SaveRouteState(ctx, state)
}

// persistInitialGeneration preserves the insert path for an adapter that
// observes a policy failure before the initial route row exists.
func (e *Engine) persistInitialGeneration(
	ctx context.Context,
	expectedGeneration int64,
	expectedStatus string,
	state RouteState,
) (bool, error) {
	if expectedGeneration != 0 || expectedStatus != "" {
		return false, nil
	}
	claimer, ok := e.persistence.(GenerationClaimer)
	if !ok {
		return false, nil
	}
	claimed, err := claimer.ClaimRouteState(ctx, expectedGeneration, state)
	if err != nil {
		return true, err
	}
	if !claimed {
		return true, ErrStaleGeneration
	}
	return true, nil
}

// persistExpectedStatus atomically updates a same-generation route status.
// Persistence without status-fencing support fails closed.
func (e *Engine) persistExpectedStatus(ctx context.Context, expectedGeneration int64, expectedStatus string, state RouteState) error {
	if e.persistence == nil {
		return nil
	}
	claimer, ok := e.persistence.(GenerationStatusClaimer)
	if !ok {
		return ErrStatusClaimUnsupported
	}
	claimed, err := claimer.ClaimRouteStateFrom(ctx, expectedGeneration, expectedStatus, state)
	if err != nil {
		return err
	}
	if !claimed {
		return ErrStaleGeneration
	}
	return nil
}

func mustJSON(value PolicyState) []byte {
	payload, err := json.Marshal(value)
	if err != nil {
		return []byte(`{}`)
	}
	return payload
}

// ResumePending advances a due retry/wait state to retrying. The caller still
// owns the concrete launch and must pass the returned generation through its
// normal launch fence.
func (e *Engine) ResumePending(ctx context.Context, sessionID string, expectedGeneration int64) (RouteDecision, error) {
	return e.resumePending(ctx, sessionID, expectedGeneration, false)
}

// ResumePendingNow is the generation-fenced manual "Retry now" operation.
// It bypasses the deadline but never bypasses the durable generation owner.
func (e *Engine) ResumePendingNow(ctx context.Context, sessionID string, expectedGeneration int64) (RouteDecision, error) {
	return e.resumePending(ctx, sessionID, expectedGeneration, true)
}

// CancelPending transitions a pending wait to manual recovery without
// advancing the route generation. Stop and cancel-wait both use this durable
// state transition; the caller chooses the reason shown to the user.
func (e *Engine) CancelPending(
	ctx context.Context,
	sessionID string,
	expectedGeneration int64,
	reason string,
) (RouteDecision, error) {
	state, exists, err := e.stateForFailure(ctx, sessionID)
	if err != nil {
		return RouteDecision{}, err
	}
	if !exists {
		return RouteDecision{}, ErrRouteStateNotFound
	}
	if state.Generation != expectedGeneration {
		return RouteDecision{}, ErrStaleGeneration
	}
	if state.Status != string(routingpolicy.DecisionRetry) &&
		state.Status != string(routingpolicy.DecisionWaitForReset) &&
		state.Status != routeStatusActionRequired {
		return RouteDecision{}, ErrRecoveryPending
	}
	var policyState PolicyState
	if state.PolicyStateJSON != "" {
		if err := json.Unmarshal([]byte(state.PolicyStateJSON), &policyState); err != nil {
			return RouteDecision{}, err
		}
	}
	policyState.PendingOutcome = routingpolicy.OutcomeStop
	expectedStatus := state.Status
	policyState.Unclassified = nil
	state.PolicyStateJSON = string(mustJSON(policyState))
	state.Status = routeStatusActionRequired
	state.UpdatedAt = e.now()
	if err := e.persistSameGeneration(ctx, expectedGeneration, expectedStatus, state); err != nil {
		return RouteDecision{}, err
	}
	e.mu.Lock()
	e.states[sessionID] = state
	e.mu.Unlock()
	return RouteDecision{
		SessionID: sessionID, LogicalProfileID: state.LogicalProfileID,
		ExecutionProfileID: state.ExecutionProfileID, Generation: state.Generation,
		ProfileVersion: state.ProfileVersion, Reason: reason, Status: state.Status,
		ErrorCode: policyState.FailureCode, ErrorClass: policyState.FailureClass,
		CatalogueVersion: policyState.CatalogueVersion, RetryOrdinal: policyState.RetryOrdinal,
		PendingOutcome: policyState.PendingOutcome, Deadline: policyState.Deadline,
	}, nil
}

// MarkActive records a successful launch. It accepts both a fresh starting
// route and a resumed retry. Other states are unchanged.
func (e *Engine) MarkActive(ctx context.Context, sessionID string, expectedGeneration int64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	state, exists, err := e.loadStateLocked(ctx, sessionID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrRouteStateNotFound
	}
	if state.Generation != expectedGeneration {
		return ErrStaleGeneration
	}
	if state.Status != routeStatusStarting && state.Status != routeStatusRetrying {
		return nil
	}
	expectedStatus := state.Status
	state.Status = routeStatusActive
	state.UpdatedAt = e.now()
	if err := e.persistSameGeneration(ctx, expectedGeneration, expectedStatus, state); err != nil {
		return err
	}
	e.states[sessionID] = state
	delete(e.retryClaims, sessionID)
	return nil
}

// ClearUnclassifiedStreak clears the current route's committed count after a
// successful output/effect/turn event has been verified against the live
// attempt by the caller.
func (e *Engine) ClearUnclassifiedStreak(
	ctx context.Context,
	sessionID string,
	expectedGeneration int64,
	candidateID string,
) error {
	return e.clearUnclassifiedStreak(ctx, sessionID, expectedGeneration, candidateID, "")
}

// ClearUnclassifiedStartupStreak clears only a startup streak after a
// confirmed boot-ready event. Launch acceptance alone is not initialization.
func (e *Engine) ClearUnclassifiedStartupStreak(
	ctx context.Context,
	sessionID string,
	expectedGeneration int64,
	candidateID string,
) error {
	return e.clearUnclassifiedStreak(ctx, sessionID, expectedGeneration, candidateID, UnclassifiedOriginAgentStartup)
}

func (e *Engine) clearUnclassifiedStreak(
	ctx context.Context,
	sessionID string,
	expectedGeneration int64,
	candidateID string,
	origin UnclassifiedFailureOrigin,
) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	state, exists, err := e.loadStateLocked(ctx, sessionID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrRouteStateNotFound
	}
	if state.Generation != expectedGeneration || state.ExecutionProfileID != candidateID {
		return ErrStaleGeneration
	}
	if state.PolicyStateJSON == "" {
		return nil
	}
	var policyState PolicyState
	if err := json.Unmarshal([]byte(state.PolicyStateJSON), &policyState); err != nil {
		return err
	}
	if policyState.Unclassified == nil || (origin != "" && policyState.Unclassified.Origin != origin) {
		return nil
	}
	oldStatus, oldPolicyState := state.Status, state.PolicyStateJSON
	policyState.Unclassified = nil
	state.PolicyStateJSON = string(mustJSON(policyState))
	state.UpdatedAt = e.now()
	if err := e.persistSnapshot(ctx, oldStatus, oldPolicyState, state); err != nil {
		return err
	}
	e.states[sessionID] = state
	return nil
}

// MarkActionRequired moves an unconfirmed launch to durable action_required.
// The transition is generation and status fenced. Active and already resolved
// routes are left unchanged.
func (e *Engine) MarkActionRequired(
	ctx context.Context,
	sessionID string,
	expectedGeneration int64,
	reason string,
) (RouteDecision, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	state, exists, err := e.loadStateLocked(ctx, sessionID)
	if err != nil {
		return RouteDecision{}, err
	}
	if !exists {
		return RouteDecision{}, ErrRouteStateNotFound
	}
	if state.Generation != expectedGeneration {
		return RouteDecision{}, ErrStaleGeneration
	}
	if state.Status == routeStatusStarting || state.Status == routeStatusRetrying {
		expectedStatus := state.Status
		state.Status = routeStatusActionRequired
		state.UpdatedAt = e.now()
		if err := e.persistSameGeneration(ctx, expectedGeneration, expectedStatus, state); err != nil {
			return RouteDecision{}, err
		}
		e.states[sessionID] = state
		delete(e.retryClaims, sessionID)
	}
	return RouteDecision{
		SessionID: sessionID, LogicalProfileID: state.LogicalProfileID,
		ExecutionProfileID: state.ExecutionProfileID, Generation: state.Generation,
		ProfileVersion: state.ProfileVersion, Reason: reason, Status: state.Status,
	}, nil
}

// ReclaimRetrying returns an orphaned durable retry claim to manual recovery.
// A retry claim owned by this process is not reclaimable because its launch
// may still cross the provider boundary.
func (e *Engine) ReclaimRetrying(ctx context.Context, sessionID string, expectedGeneration int64) (bool, error) {
	return e.reclaimRetrying(ctx, sessionID, expectedGeneration, false)
}

func (e *Engine) reclaimRetrying(
	ctx context.Context,
	sessionID string,
	expectedGeneration int64,
	allowOwned bool,
) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	state, exists, err := e.loadStateLocked(ctx, sessionID)
	if err != nil {
		return false, err
	}
	if !exists || state.Generation != expectedGeneration || state.Status != routeStatusRetrying {
		return false, nil
	}
	if !allowOwned && e.retryClaims[sessionID] == expectedGeneration {
		return false, ErrRecoveryPending
	}
	state.Status = routeStatusActionRequired
	state.UpdatedAt = e.now()
	if err := e.persistExpectedStatus(ctx, expectedGeneration, routeStatusRetrying, state); err != nil {
		return false, err
	}
	e.states[sessionID] = state
	delete(e.retryClaims, sessionID)
	return true, nil
}

// MarkRecoveryActionRequired returns an in-flight route to manual recovery.
// The launch owner calls this after a concrete launch failure, so it may
// reclaim a retry claim owned by this process.
func (e *Engine) MarkRecoveryActionRequired(ctx context.Context, sessionID string, expectedGeneration int64) error {
	_, err := e.reclaimRetrying(ctx, sessionID, expectedGeneration, true)
	return err
}

func (e *Engine) resumePending(
	ctx context.Context,
	sessionID string,
	expectedGeneration int64,
	force bool,
) (RouteDecision, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	state, exists, err := e.loadStateLocked(ctx, sessionID)
	if err != nil {
		return RouteDecision{}, err
	}
	if !exists {
		return RouteDecision{}, ErrRouteStateNotFound
	}
	if state.Generation != expectedGeneration {
		return RouteDecision{}, ErrStaleGeneration
	}
	if force {
		if state.Status == routeStatusRetrying {
			return RouteDecision{}, ErrRecoveryPending
		}
		if state.Status != string(routingpolicy.DecisionRetry) &&
			state.Status != string(routingpolicy.DecisionWaitForReset) &&
			state.Status != routeStatusActionRequired {
			return RouteDecision{}, ErrRecoveryPending
		}
	}
	if !force && state.Status != string(routingpolicy.DecisionRetry) && state.Status != string(routingpolicy.DecisionWaitForReset) {
		return RouteDecision{}, ErrRecoveryPending
	}
	observedStatus := state.Status
	var policyState PolicyState
	if state.PolicyStateJSON != "" {
		if err := json.Unmarshal([]byte(state.PolicyStateJSON), &policyState); err != nil {
			return RouteDecision{}, err
		}
	}
	now := e.now()
	if !force && policyState.Deadline != nil && now.Before(*policyState.Deadline) {
		return RouteDecision{}, ErrRecoveryNotDue
	}
	state.Status = routeStatusRetrying
	state.UpdatedAt = now
	if err := e.persistExpectedStatus(ctx, expectedGeneration, observedStatus, state); err != nil {
		return RouteDecision{}, err
	}
	e.states[sessionID] = state
	e.retryClaims[sessionID] = state.Generation
	return RouteDecision{
		SessionID: sessionID, LogicalProfileID: state.LogicalProfileID,
		ExecutionProfileID: state.ExecutionProfileID, Generation: state.Generation,
		ProfileVersion: state.ProfileVersion, Reason: "retry_due", Status: state.Status,
		ErrorCode: policyState.FailureCode, ErrorClass: policyState.FailureClass,
		CatalogueVersion: policyState.CatalogueVersion,
		RetryOrdinal:     policyState.RetryOrdinal, PendingOutcome: policyState.PendingOutcome,
	}, nil
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
