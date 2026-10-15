package dynamic

import (
	"encoding/json"
	"math/rand/v2"
	"sort"
	"time"
)

const (
	tierModeCost   = "cost"
	tierModeRandom = "random"
)

// WithTierRandomPick supplies a deterministic picker without exposing a wire seed.
func WithTierRandomPick(pick func(int) int) EngineOption {
	return func(e *Engine) { e.tierPick = pick }
}

type rankedTierRow struct {
	candidate Candidate
	entry     TierPreviewEntry
}

// TierPreview contains only public candidate identities and configured metadata.
type TierPreview struct {
	State       string             `json:"state"`
	CandidateID string             `json:"candidate_id,omitempty"`
	ObservedAt  time.Time          `json:"observed_at"`
	Considered  []TierPreviewEntry `json:"considered"`
}

type TierPreviewEntry struct {
	CandidateID string `json:"candidate_id"`
	Tier        int    `json:"tier"`
	Mode        string `json:"mode"`
	CostClass   string `json:"cost_class"`
	Eligible    bool   `json:"eligible"`
	Reason      string `json:"reason,omitempty"`
	Selected    bool   `json:"selected"`
}

func candidateCost(candidate Candidate) string {
	var selection struct {
		Model struct {
			Cost string `json:"cost"`
		} `json:"model"`
	}
	if json.Unmarshal(candidate.SelectionJSON, &selection) != nil {
		return "unknown"
	}
	switch selection.Model.Cost {
	case "free", "subscription", "metered":
		return selection.Model.Cost
	default:
		return "unknown"
	}
}

func configuredCostRank(cost string) int {
	switch cost {
	case "free":
		return 0
	case "subscription":
		return 1
	case "metered":
		return 2
	default:
		return 3
	}
}

// rankTierRows is pure: disabled/health/chain exclusions precede a random draw.
// It never changes the source rows or probes a provider.
func rankTierRows(tier orderedTier, number int, excluded map[string]bool, preferred string, unavailable map[string]string, pick func(int) int) []rankedTierRow {
	rows := make([]rankedTierRow, 0, len(tier.rows))
	for _, candidate := range tier.rows {
		reason := unavailable[candidate.ID]
		switch {
		case !candidate.Enabled:
			reason = "disabled"
		case excluded[candidate.ID] && candidate.ID != preferred:
			reason = "tried"
		case preferred != "" && candidate.ID != preferred:
			reason = "not_requested"
		}
		rows = append(rows, rankedTierRow{candidate: candidate, entry: TierPreviewEntry{
			CandidateID: candidate.ID, Tier: number, Mode: tier.mode, CostClass: candidateCost(candidate), Eligible: reason == "", Reason: reason,
		}})
	}
	if tier.mode == tierModeCost {
		sort.SliceStable(rows, func(i, j int) bool {
			return configuredCostRank(rows[i].entry.CostClass) < configuredCostRank(rows[j].entry.CostClass)
		})
	}
	if tier.mode == tierModeRandom {
		promoteRandomTierRow(rows, pick)
	}
	return rows
}

func promoteRandomTierRow(rows []rankedTierRow, pick func(int) int) {
	var eligible []int
	for index, row := range rows {
		if row.entry.Eligible {
			eligible = append(eligible, index)
		}
	}
	if len(eligible) < 2 {
		return
	}
	if pick == nil {
		pick = rand.IntN
	}
	index := pick(len(eligible))
	if index < 0 || index >= len(eligible) {
		index = 0
	}
	position := eligible[index]
	chosen := rows[position]
	copy(rows[1:position+1], rows[:position])
	rows[0] = chosen
}

// selectionAvailability is an observation, including whether an expired circuit
// can be probed. Only candidateSelectable may actually acquire that probe.
func (r *CircuitRegistry) selectionAvailability(candidate Candidate, now time.Time) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.circuits[accountBlockKey(candidate.ID)].held() {
		return "account_blocked"
	}
	entry := r.circuits[candidate.BindingKey]
	if candidate.BindingKey == "" || entry.state == CircuitClosed {
		return ""
	}
	// Match IsOpen before observing whether an exclusive probe is available.
	if entry.state != CircuitHalfOpen && !entry.held() && (entry.state != CircuitOpen || entry.until.IsZero()) {
		return ""
	}
	if entry.held() || now.Before(entry.until) {
		return "circuit_open"
	}
	if now.Before(entry.probeUntil) {
		return "circuit_probing"
	}
	return ""
}

func (e *Engine) tierAvailability(profile Profile, now time.Time) map[string]string {
	result := make(map[string]string, len(profile.Candidates))
	for _, candidate := range profile.Candidates {
		result[candidate.ID] = e.circuits.selectionAvailability(candidate, now)
	}
	return result
}

func (e *Engine) PreviewTiers(profile Profile) (TierPreview, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	preview := TierPreview{State: "disabled", ObservedAt: e.now(), Considered: []TierPreviewEntry{}}
	if !e.orderedTiers {
		return preview, nil
	}
	tiers, err := deriveOrderedTiers(profile.Candidates)
	if err != nil {
		return preview, err
	}
	preview.State = "no_candidates"
	unavailable := e.tierAvailability(profile, preview.ObservedAt)
	for index, tier := range tiers {
		for _, row := range rankTierRows(tier, index+1, nil, "", unavailable, e.tierPick) {
			entry := row.entry
			if entry.Eligible && preview.CandidateID == "" {
				entry.Selected = true
				preview.CandidateID = entry.CandidateID
			}
			preview.Considered = append(preview.Considered, entry)
		}
	}
	if len(preview.Considered) > 0 {
		preview.State = "no_eligible"
	}
	if preview.CandidateID != "" {
		preview.State = "selected"
	}
	return preview, nil
}
