package dynamic

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

// Failed-route automatic retry is the durable bookkeeping behind the
// per-workspace opt-in that submits one unattended Retry route action for a
// route that a classified provider-capacity failure left in manual recovery.
// The engine only records, fences, and consumes the schedule; the
// orchestrator owns the timer and submits the route action through the same
// transaction a person uses.

// ErrFailedRouteRetryDeclined reports that a failed route may not be retried
// automatically: the route is not in a retryable recovery state, the chain
// already used its attempts, the same failure repeated, or no candidate can
// recover without an owner action.
var ErrFailedRouteRetryDeclined = errors.New("automatic failed-route retry declined")

// FailedRouteFailure identifies the classified failure that left a route in
// recovery: the candidate that failed and its code.
type FailedRouteFailure struct {
	ExecutionProfileID string          `json:"execution_profile_id"`
	Code               routingerr.Code `json:"code"`
}

// PendingFailedRouteRetry is one scheduled automatic retry. It belongs to the
// exhausted route generation it was scheduled for and is never carried to a
// later generation.
type PendingFailedRouteRetry struct {
	Generation int64              `json:"generation"`
	Due        time.Time          `json:"due"`
	Failure    FailedRouteFailure `json:"failure"`
}

// FailedRouteRetryChain records the automatic retries of one session since
// its last successful turn. It is carried across route generations so the
// cap and the repeated-failure stop survive the retry's own new claim and a
// backend restart.
type FailedRouteRetryChain struct {
	Attempts int                      `json:"attempts"`
	Failures []FailedRouteFailure     `json:"failures,omitempty"`
	Pending  *PendingFailedRouteRetry `json:"pending,omitempty"`
}

func (c *FailedRouteRetryChain) empty() bool {
	return c == nil || (c.Attempts == 0 && len(c.Failures) == 0 && c.Pending == nil)
}

// failedRouteRetryStatus reports whether a durable route status is a manual
// recovery state an automatic Retry may resume: action_required (a stopped
// candidate) or waiting (no eligible candidate remained).
func failedRouteRetryStatus(status string) bool {
	return status == routeStatusActionRequired || status == routeStatusWaiting
}

// FailedRouteRetryPending returns the schedule recorded in a route's policy
// state for its current generation. Restart recovery uses it to re-arm timers.
func FailedRouteRetryPending(state RouteState) (PendingFailedRouteRetry, bool) {
	if !failedRouteRetryStatus(state.Status) {
		return PendingFailedRouteRetry{}, false
	}
	chain := decodePolicyState(state.PolicyStateJSON).FailedRouteRetry
	if chain == nil || chain.Pending == nil || chain.Pending.Generation != state.Generation {
		return PendingFailedRouteRetry{}, false
	}
	return *chain.Pending, true
}

// carryFailedRouteRetry copies the chain's attempts and failures from the
// previous route state onto the policy state of a newly claimed generation. A
// pending schedule belongs to the old generation and is dropped.
func carryFailedRouteRetry(previousRaw, nextRaw string) string {
	previous := decodePolicyState(previousRaw).FailedRouteRetry
	if previous == nil || (previous.Attempts == 0 && len(previous.Failures) == 0) {
		return nextRaw
	}
	next := decodePolicyState(nextRaw)
	next.FailedRouteRetry = &FailedRouteRetryChain{
		Attempts: previous.Attempts,
		Failures: slices.Clone(previous.Failures),
	}
	return string(mustJSON(next))
}

// ScheduleFailedRouteRetry durably records one automatic retry for the route
// generation a classified failure left in recovery. The due time is never
// earlier than now+minDelay, nor than the earliest expiry of an open circuit
// of a candidate the retry could use. A schedule already recorded for the
// same generation is returned with scheduled=false, so a redelivered failure
// never schedules twice.
func (e *Engine) ScheduleFailedRouteRetry(
	ctx context.Context,
	sessionID string,
	profile Profile,
	expectedGeneration int64,
	failure FailedRouteFailure,
	maxAttempts int,
	minDelay time.Duration,
) (PendingFailedRouteRetry, bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	state, exists, err := e.loadStateLocked(ctx, sessionID)
	if err != nil {
		return PendingFailedRouteRetry{}, false, err
	}
	if !exists {
		return PendingFailedRouteRetry{}, false, ErrRouteStateNotFound
	}
	if state.Generation != expectedGeneration {
		return PendingFailedRouteRetry{}, false, ErrStaleGeneration
	}
	if !failedRouteRetryStatus(state.Status) {
		return PendingFailedRouteRetry{}, false, ErrFailedRouteRetryDeclined
	}
	policyState := decodePolicyState(state.PolicyStateJSON)
	chain := policyState.FailedRouteRetry
	if chain == nil {
		chain = &FailedRouteRetryChain{}
	}
	if chain.Pending != nil && chain.Pending.Generation == state.Generation {
		return *chain.Pending, false, nil
	}
	if chain.Attempts >= maxAttempts || slices.Contains(chain.Failures, failure) {
		return PendingFailedRouteRetry{}, false, ErrFailedRouteRetryDeclined
	}
	now := e.now()
	due, recoverable := e.failedRouteRetryDueLocked(profile, state, now.Add(minDelay))
	if !recoverable {
		return PendingFailedRouteRetry{}, false, ErrFailedRouteRetryDeclined
	}
	pending := PendingFailedRouteRetry{Generation: state.Generation, Due: due, Failure: failure}
	next := state
	updated := *chain
	updated.Pending = &pending
	policyState.FailedRouteRetry = &updated
	next.PolicyStateJSON = string(mustJSON(policyState))
	next.UpdatedAt = now
	if err := e.persistSnapshot(ctx, state.Status, state.PolicyStateJSON, next); err != nil {
		return PendingFailedRouteRetry{}, false, err
	}
	e.states[sessionID] = next
	return pending, true, nil
}

// failedRouteRetryDueLocked returns the earliest moment an automatic retry of
// the route can find a usable candidate. An action_required route retries its
// own candidate; a waiting route reselects from every enabled candidate. A
// candidate held until its owner releases it never recovers by waiting, so a
// route whose usable candidates are all held is not recoverable.
func (e *Engine) failedRouteRetryDueLocked(profile Profile, state RouteState, notBefore time.Time) (time.Time, bool) {
	var earliest time.Time
	usable := false
	for _, candidate := range profile.Candidates {
		if !candidate.Enabled || e.circuits.IsHeld(accountBlockKey(candidate.ID)) {
			continue
		}
		if state.Status == routeStatusActionRequired && candidate.ID != state.ExecutionProfileID {
			continue
		}
		if candidate.BindingKey != "" && e.circuits.IsHeld(candidate.BindingKey) {
			continue
		}
		until, open := e.circuits.OpenUntil(candidate.BindingKey)
		if !open {
			// A closed candidate is usable now; the minimum delay decides.
			return notBefore, true
		}
		usable = true
		if earliest.IsZero() || until.Before(earliest) {
			earliest = until
		}
	}
	if !usable {
		return time.Time{}, false
	}
	if earliest.After(notBefore) {
		return earliest, true
	}
	return notBefore, true
}

// ClaimFailedRouteRetry consumes the schedule recorded for expectedGeneration
// just before the automatic Retry is submitted: it counts the attempt and
// records its failure, then clears the schedule, in one snapshot-fenced
// write. Consuming first makes the retry at most once even across a crash. A
// schedule that is not yet due returns ErrRecoveryNotDue with its due time.
func (e *Engine) ClaimFailedRouteRetry(
	ctx context.Context,
	sessionID string,
	expectedGeneration int64,
) (PendingFailedRouteRetry, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	state, exists, err := e.loadStateLocked(ctx, sessionID)
	if err != nil {
		return PendingFailedRouteRetry{}, err
	}
	if !exists {
		return PendingFailedRouteRetry{}, ErrRouteStateNotFound
	}
	if state.Generation != expectedGeneration {
		return PendingFailedRouteRetry{}, ErrStaleGeneration
	}
	pending, ok := FailedRouteRetryPending(state)
	if !ok {
		return PendingFailedRouteRetry{}, ErrFailedRouteRetryDeclined
	}
	now := e.now()
	if now.Before(pending.Due) {
		return pending, ErrRecoveryNotDue
	}
	policyState := decodePolicyState(state.PolicyStateJSON)
	chain := *policyState.FailedRouteRetry
	chain.Attempts++
	chain.Failures = append(slices.Clone(chain.Failures), pending.Failure)
	chain.Pending = nil
	policyState.FailedRouteRetry = &chain
	next := state
	next.PolicyStateJSON = string(mustJSON(policyState))
	next.UpdatedAt = now
	if err := e.persistSnapshot(ctx, state.Status, state.PolicyStateJSON, next); err != nil {
		return PendingFailedRouteRetry{}, err
	}
	e.states[sessionID] = next
	return pending, nil
}

// ClearFailedRouteRetry ends a session's automatic retry chain after a
// successful turn, so a later failure starts a new chain. A route without a
// chain is left untouched and costs no write.
func (e *Engine) ClearFailedRouteRetry(ctx context.Context, sessionID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	state, exists, err := e.loadStateLocked(ctx, sessionID)
	if err != nil || !exists {
		return err
	}
	policyState := decodePolicyState(state.PolicyStateJSON)
	if policyState.FailedRouteRetry.empty() {
		return nil
	}
	policyState.FailedRouteRetry = nil
	next := state
	next.PolicyStateJSON = string(mustJSON(policyState))
	next.UpdatedAt = e.now()
	if err := e.persistSnapshot(ctx, state.Status, state.PolicyStateJSON, next); err != nil {
		return err
	}
	e.states[sessionID] = next
	return nil
}

// FailedRouteRetryChainFor returns a copy of the session's current chain, for
// diagnostics and tests.
func (e *Engine) FailedRouteRetryChainFor(ctx context.Context, sessionID string) (FailedRouteRetryChain, error) {
	state, exists, err := e.LoadState(ctx, sessionID)
	if err != nil || !exists {
		return FailedRouteRetryChain{}, err
	}
	var policyState PolicyState
	if state.PolicyStateJSON != "" {
		if err := json.Unmarshal([]byte(state.PolicyStateJSON), &policyState); err != nil {
			return FailedRouteRetryChain{}, err
		}
	}
	if policyState.FailedRouteRetry == nil {
		return FailedRouteRetryChain{}, nil
	}
	return *policyState.FailedRouteRetry, nil
}
