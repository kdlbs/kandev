package dynamic

import (
	"context"
	"encoding/json"
	"fmt"
)

// reasonPolicySkip is the route reason for a selection that skipped a candidate by policy.
const reasonPolicySkip = "policy_skip"

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

func deriveOrderedTiers(candidates []Candidate) ([]orderedTier, error) {
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
		if !supportedTierMode(mode) || (direction != orderedSameTier && direction != orderedNextTier) {
			return nil, fmt.Errorf("dynamic tier selection mode or direction is not supported by this increment")
		}
		tiers = append(tiers, orderedTier{head: candidate.ID, mode: mode, direction: direction, rows: []Candidate{candidate}})
	}
	return tiers, nil
}

type orderedPlan struct {
	rows  func(func(Candidate, error) bool)
	chain SelectionChain
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
	tiers, err := deriveOrderedTiers(profile.Candidates)
	if err != nil {
		return orderedPlan{}, err
	}
	chain, continuing := retainedSelectionChain(state, profile.ID, generation)
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
		start = orderedStartIndex(tiers, current, excluded != "")
	}
	unavailable := e.tierAvailability(profile, e.now())
	return orderedPlan{chain: chain, rows: orderedRemainingRows(tiers[start:], chain.TriedCandidateIDs, preferred, unavailable, e.tierPick, func() (map[string]int64, error) {
		return e.tierSelectionHistory(ctx, profile.ID, tiers[start:])
	})}, nil
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

// Traverse lazily so an unreached round-robin tier cannot block an earlier
// eligible tier. One reached traversal shares a single history observation.
func orderedRemainingRows(tiers []orderedTier, triedIDs []string, preferred string, unavailable map[string]string, pick func(int) int, readHistory func() (map[string]int64, error)) func(func(Candidate, error) bool) {
	return func(yield func(Candidate, error) bool) {
		tried := make(map[string]bool, len(triedIDs))
		for _, id := range triedIDs {
			tried[id] = true
		}
		var history map[string]int64
		var historyLoaded bool
		for index, tier := range tiers {
			if tier.mode == tierModeRoundRobin && !historyLoaded {
				var err error
				history, err = readHistory()
				if err != nil {
					yield(Candidate{}, err)
					return
				}
				historyLoaded = true
			}
			for _, row := range rankTierRows(tier, index+1, tried, preferred, unavailable, pick, history) {
				if row.entry.Eligible && !yield(row.candidate, nil) {
					return
				}
			}
		}
	}
}

func orderedStartIndex(tiers []orderedTier, current string, skipping bool) int {
	for index, tier := range tiers {
		for _, candidate := range tier.rows {
			if candidate.ID != current {
				continue
			}
			if skipping && tier.direction == orderedNextTier {
				return index + 1
			}
			return index
		}
	}
	return 0
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
