package dynamic

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// reasonPolicySkip is the route reason for a selection that skipped a candidate by policy.
const reasonPolicySkip = "policy_skip"

// reasonManualRetry is the route reason of a person's Retry of a route left
// waiting without an eligible candidate.
const reasonManualRetry = "manual_retry"

const (
	orderedTierMode = "order"
	orderedSameTier = "same_tier_next"
	orderedNextTier = "next_tier"
)

// WithOrderedTiers enables adjacency and durable traversal without replacing
// the existing generation, policy or circuit owners.
func WithOrderedTiers(enabled bool) EngineOption {
	return func(e *Engine) { e.orderedTiers = enabled }
}

// OrderedTiersEnabled reports the startup-composed feature value.
func (e *Engine) OrderedTiersEnabled() bool { return e != nil && e.orderedTiers }

type orderedSelection struct {
	JoinPrevious bool `json:"join_previous"`
	Tier         *struct {
		Mode      string `json:"mode"`
		OnFailure string `json:"on_failure"`
	} `json:"tier"`
}

type orderedTier struct {
	head      string
	mode      string
	direction string
	rows      []Candidate
}

// SelectionChain identifies an admitted transition chain by concrete IDs.
// Counters and profile revisions do not redefine which candidates were tried.
type SelectionChain struct {
	Version           int      `json:"version"`
	LogicalProfileID  string   `json:"logical_profile_id"`
	StartGeneration   int64    `json:"start_generation"`
	TriedCandidateIDs []string `json:"tried_candidate_ids"`
	LastCandidateID   string   `json:"last_candidate_id,omitempty"`
}

func deriveOrderedTiers(candidates []Candidate, usageAvailable bool) ([]orderedTier, error) {
	var tiers []orderedTier
	for _, candidate := range candidates {
		var selection orderedSelection
		if len(candidate.SelectionJSON) > 0 {
			if err := json.Unmarshal(candidate.SelectionJSON, &selection); err != nil {
				return nil, err
			}
		}
		if selection.JoinPrevious && len(tiers) > 0 {
			tiers[len(tiers)-1].rows = append(tiers[len(tiers)-1].rows, candidate)
			continue
		}
		mode, direction := orderedTierMode, orderedSameTier
		if selection.Tier != nil {
			if selection.Tier.Mode != "" {
				mode = selection.Tier.Mode
			}
			if selection.Tier.OnFailure != "" {
				direction = selection.Tier.OnFailure
			}
		}
		if !supportedTierMode(mode, usageAvailable) || (direction != orderedSameTier && direction != orderedNextTier) {
			return nil, fmt.Errorf("dynamic tier selection mode or direction is not supported by this increment")
		}
		tiers = append(tiers, orderedTier{head: candidate.ID, mode: mode, direction: direction, rows: []Candidate{candidate}})
	}
	return tiers, nil
}

type orderedPlan struct {
	rows  func(func(Candidate, error) bool)
	chain SelectionChain
	// observe builds the ranked facts of every remaining candidate in stored
	// order for the selection log. It is nil unless a tier ranks by usage, so
	// an order-only profile pays nothing and logs nothing. It reuses the
	// availability observed before this selection, so a probe acquired by the
	// decision itself cannot change what the log reports.
	observe func() []TierPreviewEntry
}

func (e *Engine) prepareOrderedPlan(ctx context.Context, state RouteState, profile Profile, generation int64, excluded, preferred, reason string) (orderedPlan, error) {
	if !e.orderedTiers {
		return orderedPlan{rows: func(yield func(Candidate, error) bool) {
			for _, candidate := range profile.Candidates {
				if !yield(candidate, nil) {
					return
				}
			}
		}}, nil
	}
	tiers, err := deriveOrderedTiers(profile.Candidates, e.tierUsageAvailable)
	if err != nil {
		return orderedPlan{}, err
	}
	chain, continuing := retainedSelectionChain(state, profile.ID, generation)
	if startsFreshManualWaitingRetry(state, excluded, preferred, reason) {
		// A new manual cycle keeps prior immutable attempts as history; it does
		// not claim successful output or release any provider/recovery guard.
		chain, continuing = retainedSelectionChain(RouteState{}, profile.ID, generation)
	}
	current := state.ExecutionProfileID
	if current == "" {
		current = chain.LastCandidateID
	}
	fallback := excluded != "" || preferred != "" || reason == "retry" || reason == reasonPolicySkip || reason == "try_next"
	if fallback && current != "" {
		chain = chain.withCandidate(current)
	}
	start := 0
	if continuing || fallback {
		start = orderedStartIndex(tiers, current, excluded != "", chain.TriedCandidateIDs)
	}
	now := e.now()
	unavailable := e.tierAvailability(profile, now)
	remaining := tiers[start:]
	return orderedPlan{
		chain: chain,
		rows: orderedRemainingRows(remaining, chain.TriedCandidateIDs, preferred, unavailable, e.tierPick, func() (map[string]int64, error) {
			return e.tierSelectionHistory(ctx, profile.ID, remaining)
		}, now),
		observe: tierObservationLog(remaining, chain.TriedCandidateIDs, preferred, unavailable, now),
	}, nil
}

// tierObservationLog returns the stored-order facts of every candidate the plan
// could still reach. Round-robin history is deliberately not read again: the
// log reports the saved order, not a second rotation.
func tierObservationLog(
	tiers []orderedTier,
	tried []string,
	preferred string,
	unavailable map[string]string,
	now time.Time,
) func() []TierPreviewEntry {
	if !anyUsageTier(tiers) {
		return nil
	}
	excluded := make(map[string]bool, len(tried))
	for _, id := range tried {
		excluded[id] = true
	}
	// Stored-order facts must not draw a second random sample; the log reports
	// the order the decision saw, not a new selection.
	storedOrder := func(int) int { return 0 }
	return func() []TierPreviewEntry {
		entries := make([]TierPreviewEntry, 0)
		for index, tier := range tiers {
			for _, row := range rankTierRows(tier, index+1, excluded, preferred, unavailable, storedOrder, nil, now) {
				entries = append(entries, row.entry)
			}
		}
		return entries
	}
}

func usageTierMode(mode string) bool {
	return mode == tierModePace || mode == tierModeHeadroom || mode == tierModeConsumptionRate
}

func anyUsageTier(tiers []orderedTier) bool {
	for _, tier := range tiers {
		if usageTierMode(tier.mode) {
			return true
		}
	}
	return false
}

func retainedSelectionChain(state RouteState, profileID string, generation int64) (SelectionChain, bool) {
	chain := SelectionChain{Version: 1, LogicalProfileID: profileID, StartGeneration: generation}
	previous := decodePolicyState(state.PolicyStateJSON).SelectionChain
	if previous == nil || previous.Version != 1 || previous.LogicalProfileID != profileID {
		return chain, false
	}
	chain = *previous
	chain.TriedCandidateIDs = append([]string(nil), previous.TriedCandidateIDs...)
	return chain, true
}

// startsFreshManualWaitingRetry reports a person's Retry of a route that is
// waiting without a candidate, so an exhausted traversal can start over.
func startsFreshManualWaitingRetry(state RouteState, excluded, preferred, reason string) bool {
	return reason == reasonManualRetry && state.Status == routeStatusWaiting &&
		state.ExecutionProfileID == "" && excluded == "" && preferred == ""
}

// Traverse lazily so an unreached round-robin tier cannot block an earlier
// eligible tier. One reached traversal shares a single history observation.
func orderedRemainingRows(tiers []orderedTier, triedIDs []string, preferred string, unavailable map[string]string, pick func(int) int, readHistory func() (map[string]int64, error), now time.Time) func(func(Candidate, error) bool) {
	return func(yield func(Candidate, error) bool) {
		tried := make(map[string]bool, len(triedIDs))
		for _, id := range triedIDs {
			tried[id] = true
		}
		var history map[string]int64
		var historyLoaded bool
		for index, tier := range tiers {
			// A round-robin tier with no eligible row is skipped without a
			// history read, so its read failure cannot block a later tier.
			if tier.mode == tierModeRoundRobin && !historyLoaded && tierHasEligibleRows(tier, tried, preferred, unavailable) {
				var err error
				history, err = readHistory()
				if err != nil {
					yield(Candidate{}, err)
					return
				}
				historyLoaded = true
			}
			for _, row := range rankTierRows(tier, index+1, tried, preferred, unavailable, pick, history, now) {
				if row.entry.Eligible && !yield(row.candidate, nil) {
					return
				}
			}
		}
	}
}

func locateTier(tiers []orderedTier, id string) (int, string, bool) {
	for index, tier := range tiers {
		for _, candidate := range tier.rows {
			if candidate.ID == id {
				return index, tier.direction, true
			}
		}
	}
	return 0, "", false
}

func orderedStartIndex(tiers []orderedTier, current string, skipping bool, tried []string) int {
	if index, direction, ok := locateTier(tiers, current); ok {
		if skipping && direction == orderedNextTier {
			return index + 1
		}
		return index
	}
	// The selected candidate may have been deleted from the saved order. Resume
	// at the furthest tier that still holds a tried candidate so the durable
	// chain cannot reopen an earlier tier.
	best, bestDirection := -1, ""
	for _, id := range tried {
		if index, direction, ok := locateTier(tiers, id); ok && index > best {
			best, bestDirection = index, direction
		}
	}
	if best < 0 {
		if len(tried) == 0 {
			return 0
		}
		// The chain's position is not established in the current saved order.
		// Fail closed instead of restarting at the first tier.
		return len(tiers)
	}
	if skipping && bestDirection == orderedNextTier {
		return best + 1
	}
	return best
}

func (c SelectionChain) withCandidate(id string) SelectionChain {
	if id == "" {
		return c
	}
	for _, tried := range c.TriedCandidateIDs {
		if tried == id {
			c.LastCandidateID = id
			return c
		}
	}
	c.TriedCandidateIDs = append(append([]string(nil), c.TriedCandidateIDs...), id)
	c.LastCandidateID = id
	return c
}

func (e *Engine) orderedPolicyState(raw string, chain SelectionChain) string {
	if !e.orderedTiers {
		return raw
	}
	state := decodePolicyState(raw)
	state.SelectionChain = &chain
	return string(mustJSON(state))
}

// ClearSelectionChain ends a chain only after a caller verifies current output
// or effects. Startup and failure-policy counter resets use separate methods.
func (e *Engine) ClearSelectionChain(ctx context.Context, sessionID string, generation int64, candidateID string) error {
	if !e.orderedTiers {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	state, exists, err := e.loadStateLocked(ctx, sessionID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrRouteStateNotFound
	}
	if state.Generation != generation || state.ExecutionProfileID != candidateID {
		return ErrStaleGeneration
	}
	policy := decodePolicyState(state.PolicyStateJSON)
	if policy.SelectionChain == nil {
		return nil
	}
	oldStatus, oldPolicy := state.Status, state.PolicyStateJSON
	policy.SelectionChain = nil
	state.PolicyStateJSON = string(mustJSON(policy))
	state.UpdatedAt = e.now()
	if err := e.persistSnapshot(ctx, oldStatus, oldPolicy, state); err != nil {
		return err
	}
	e.states[sessionID] = state
	return nil
}
