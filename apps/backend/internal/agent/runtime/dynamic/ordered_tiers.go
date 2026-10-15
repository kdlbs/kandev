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
		if (mode != orderedTierMode && mode != tierModeCost && mode != tierModeRandom) || (direction != orderedSameTier && direction != orderedNextTier) {
			return nil, fmt.Errorf("dynamic tier selection mode or direction is not supported by this increment")
		}
		tiers = append(tiers, orderedTier{head: candidate.ID, mode: mode, direction: direction, rows: []Candidate{candidate}})
	}
	return tiers, nil
}

type orderedPlan struct {
	rows  []Candidate
	chain SelectionChain
}

func (e *Engine) prepareOrderedPlan(state RouteState, profile Profile, generation int64, excluded, preferred, reason string) (orderedPlan, error) {
	if !e.orderedTiers {
		return orderedPlan{rows: profile.Candidates}, nil
	}
	tiers, err := deriveOrderedTiers(profile.Candidates)
	if err != nil {
		return orderedPlan{}, err
	}
	chain := SelectionChain{Version: 1, LogicalProfileID: profile.ID, StartGeneration: generation}
	previous := decodePolicyState(state.PolicyStateJSON).SelectionChain
	continuing := previous != nil && previous.Version == 1 && previous.LogicalProfileID == profile.ID
	if continuing {
		chain = *previous
		chain.TriedCandidateIDs = append([]string(nil), previous.TriedCandidateIDs...)
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
		start = orderedStartIndex(tiers, current, excluded != "")
	}
	unavailable := e.tierAvailability(profile, e.now())
	return orderedPlan{chain: chain, rows: orderedRemainingRows(tiers[start:], chain.TriedCandidateIDs, preferred, unavailable, e.tierPick)}, nil
}

func orderedRemainingRows(tiers []orderedTier, triedIDs []string, preferred string, unavailable map[string]string, pick func(int) int) []Candidate {
	tried := make(map[string]bool, len(triedIDs))
	for _, id := range triedIDs {
		tried[id] = true
	}
	var rows []Candidate
	for index, tier := range tiers {
		for _, row := range rankTierRows(tier, index+1, tried, preferred, unavailable, pick) {
			if !row.entry.Eligible {
				continue
			}
			rows = append(rows, row.candidate)
		}
	}
	return rows
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
